package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/auth"
	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
	"github.com/openpaw/openpaw/internal/secrets"
)

func validatePassword(password string) string {
	if len(password) < 8 {
		return "password must be at least 8 characters"
	}
	var hasUpper, hasLower, hasDigit bool
	for _, c := range password {
		switch {
		case 'A' <= c && c <= 'Z':
			hasUpper = true
		case 'a' <= c && c <= 'z':
			hasLower = true
		case '0' <= c && c <= '9':
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return "password must contain uppercase, lowercase, and a digit"
	}
	return ""
}

type AuthHandler struct {
	db      *database.DB
	auth    *auth.Service
	dataDir string
}

func NewAuthHandler(db *database.DB, authService *auth.Service, dataDir string) *AuthHandler {
	return &AuthHandler{db: db, auth: authService, dataDir: dataDir}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		RememberMe bool   `json:"remember_me"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}

	var user models.User
	err := h.db.QueryRow(
		"SELECT id, username, password_hash, display_name, avatar_path, created_at, updated_at FROM users WHERE username = ?",
		req.Username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.DisplayName, &user.AvatarPath, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := h.auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		h.db.LogAudit("", "login_failed", "auth", "user", "", "Failed login attempt for user: "+req.Username)
		return
	}

	ttl := 24 * time.Hour
	maxAge := 86400
	if req.RememberMe {
		ttl = 30 * 24 * time.Hour
		maxAge = 30 * 86400
	}

	token, err := h.auth.GenerateTokenWithTTL(user.ID, user.Username, ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "openpaw_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   maxAge,
	})

	h.db.LogAudit(user.ID, "login", "auth", "user", user.ID, "User logged in")

	middleware.SetCSRFCookie(w, r)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": user,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	http.SetCookie(w, &http.Cookie{
		Name:     "openpaw_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	h.db.LogAudit(userID, "logout", "auth", "user", userID, "User logged out")
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validatePassword(req.NewPassword); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var currentHash string
	err := h.db.QueryRow("SELECT password_hash FROM users WHERE id = ?", userID).Scan(&currentHash)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := h.auth.CheckPassword(currentHash, req.CurrentPassword); err != nil {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := h.auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	_, err = h.db.Exec("UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", newHash, time.Now().UTC(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	h.db.LogAudit(userID, "password_changed", "auth", "user", userID, "")
	writeJSON(w, http.StatusOK, map[string]string{"message": "password changed"})
}

func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req struct {
		Username string `json:"username"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(req.Username) < 3 {
		writeError(w, http.StatusBadRequest, "username must be at least 3 characters")
		return
	}

	// Check if username is already taken by another user
	var existingID string
	err := h.db.QueryRow("SELECT id FROM users WHERE username = ? AND id != ?", req.Username, userID).Scan(&existingID)
	if err == nil {
		writeError(w, http.StatusConflict, "username is already taken")
		return
	}

	now := time.Now().UTC()
	_, err = h.db.Exec("UPDATE users SET username = ?, updated_at = ? WHERE id = ?", req.Username, now, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	h.db.LogAudit(userID, "profile_updated", "auth", "user", userID, "Username changed to "+req.Username)
	writeJSON(w, http.StatusOK, map[string]string{"message": "profile updated"})
}

func (h *AuthHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	// Delete all data including user account. Order: standalone/child tables
	// first, then parent tables. CASCADE FKs auto-delete: tool_integrity,
	// schedule_executions, thread_members, chat_attachments,
	// dashboard_data_points, browser_tasks, browser_action_log.
	tablesToDelete := []string{
		"agent_tool_access",
		"context_files",
		"context_folders",
		"browser_sessions",
		"notifications",
		"heartbeat_executions",
		"chat_messages",
		"chat_threads",
		"work_orders",
		"agents",
		"schedules",
		"secrets",
		"dashboards",
		"agent_roles",
		"audit_logs",
		"settings",
		"tools",
		"system_stats",
		"users",
	}
	for _, table := range tablesToDelete {
		h.db.Exec("DELETE FROM " + table)
	}

	// Clear all filesystem data
	for _, dir := range []string{"skills", "agents", "gateway", "context", "browser_sessions", "avatars"} {
		dirPath := filepath.Join(h.dataDir, dir)
		os.RemoveAll(dirPath)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "openpaw_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	h.db.LogAudit(userID, "account_deleted", "auth", "user", userID, "Account and all data deleted")
	writeJSON(w, http.StatusOK, map[string]string{"message": "account deleted"})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var user models.User
	err := h.db.QueryRow(
		"SELECT id, username, display_name, avatar_path, created_at, updated_at FROM users WHERE id = ?",
		userID,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.AvatarPath, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"user": user})
}

func (h *AuthHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	r.ParseMultipartForm(5 << 20)

	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeError(w, http.StatusBadRequest, "avatar file required")
		return
	}
	defer file.Close()

	ct := header.Header.Get("Content-Type")
	var ext string
	switch {
	case strings.HasPrefix(ct, "image/png"):
		ext = ".png"
	case strings.HasPrefix(ct, "image/jpeg"):
		ext = ".jpg"
	case strings.HasPrefix(ct, "image/webp"):
		ext = ".webp"
	default:
		writeError(w, http.StatusBadRequest, "avatar must be PNG, JPEG, or WebP")
		return
	}

	// Validate file content with magic bytes
	if !validateImageMagicBytes(file, ext) {
		writeError(w, http.StatusBadRequest, "file content does not match declared type")
		return
	}

	uploadsDir := filepath.Join(h.dataDir, "avatars")
	os.MkdirAll(uploadsDir, 0755)

	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	destPath := filepath.Join(uploadsDir, filename)

	dest, err := os.Create(destPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save avatar")
		return
	}
	defer dest.Close()

	if _, err := io.Copy(dest, file); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write avatar")
		return
	}

	avatarURL := "/api/v1/uploads/avatars/" + filename
	_, err = h.db.Exec("UPDATE users SET avatar_path = ?, updated_at = ? WHERE id = ?", avatarURL, time.Now().UTC(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update avatar")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"avatar_path": avatarURL})
}

type SetupHandler struct {
	db         *database.DB
	auth       *auth.Service
	secretsMgr *secrets.Manager
	client     *llm.Client
}

func NewSetupHandler(db *database.DB, authService *auth.Service, secretsMgr *secrets.Manager, client *llm.Client) *SetupHandler {
	return &SetupHandler{db: db, auth: authService, secretsMgr: secretsMgr, client: client}
}

func (h *SetupHandler) Status(w http.ResponseWriter, r *http.Request) {
	hasAdmin, err := h.db.HasAdminUser()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check setup status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needs_setup": !hasAdmin})
}

func (h *SetupHandler) Init(w http.ResponseWriter, r *http.Request) {
	hasAdmin, err := h.db.HasAdminUser()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check setup status")
		return
	}
	if hasAdmin {
		writeError(w, http.StatusConflict, "admin user already exists")
		return
	}

	var req struct {
		Username     string   `json:"username"`
		Password     string   `json:"password"`
		DisplayName  string   `json:"display_name"`
		EnabledRoles []string `json:"enabled_roles"`
		APIKey       string   `json:"api_key"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	if msg := validatePassword(req.Password); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	hash, err := h.auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	id := generateID()
	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Username
	}
	now := time.Now().UTC()

	_, err = h.db.Exec(
		"INSERT INTO users (id, username, password_hash, display_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, req.Username, hash, displayName, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create admin user")
		return
	}

	// Save API key if provided
	if req.APIKey != "" && h.secretsMgr != nil {
		testClient := llm.NewClient(req.APIKey)
		valCtx, valCancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer valCancel()
		if err := testClient.ValidateKey(valCtx); err == nil {
			encrypted, encErr := h.secretsMgr.Encrypt(req.APIKey)
			if encErr == nil {
				keyID := generateID()
				h.db.Exec("INSERT INTO settings (id, key, value) VALUES (?, ?, ?)", keyID, "openrouter_api_key", encrypted)
				if h.client != nil {
					h.client.UpdateAPIKey(req.APIKey)
				}
			}
		}
	}

	// Seed preset agent roles
	agents.SeedPresetRoles(h.db, req.EnabledRoles)

	token, err := h.auth.GenerateToken(id, req.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "openpaw_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   86400,
	})

	h.db.LogAudit(id, "setup_init", "auth", "user", id, "Admin user created during setup")

	middleware.SetCSRFCookie(w, r)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"user": models.User{
			ID:          id,
			Username:    req.Username,
			DisplayName: displayName,
			AvatarPath:  "",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	})
}
