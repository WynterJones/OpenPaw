package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/secrets"
	"github.com/openpaw/openpaw/internal/toolmgr"
)

type SecretsHandler struct {
	db      *database.DB
	manager *secrets.Manager
	toolMgr *toolmgr.Manager
}

func NewSecretsHandler(db *database.DB, manager *secrets.Manager, toolMgr *toolmgr.Manager) *SecretsHandler {
	return &SecretsHandler{db: db, manager: manager, toolMgr: toolMgr}
}

func (h *SecretsHandler) restartToolsUsing(secretName string) {
	if h.toolMgr == nil {
		return
	}
	toolIDs := h.toolMgr.FindToolsUsingSecret(secretName)
	for _, id := range toolIDs {
		logger.Info("Restarting tool %s after secret %q changed", id, secretName)
		if err := h.toolMgr.RestartTool(id); err != nil {
			logger.Warn("Failed to restart tool %s after secret change: %v", id, err)
		}
	}
}

func (h *SecretsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		`SELECT s.id, s.name, s.description, s.tool_id, COALESCE(t.name, ''), s.created_at, s.updated_at
		 FROM secrets s
		 LEFT JOIN tools t ON s.tool_id != '' AND s.tool_id = t.id
		 ORDER BY s.name ASC`,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list secrets")
		return
	}
	defer rows.Close()

	type secretEntry struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
		ToolID      string    `json:"tool_id"`
		ToolName    string    `json:"tool_name"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
	}

	list := []secretEntry{}
	for rows.Next() {
		var s secretEntry
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.ToolID, &s.ToolName, &s.CreatedAt, &s.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan secret")
			return
		}
		list = append(list, s)
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *SecretsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Value       string `json:"value"`
		Description string `json:"description"`
		ToolID      string `json:"tool_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Value == "" {
		writeError(w, http.StatusBadRequest, "name and value are required")
		return
	}

	encrypted, err := h.manager.Encrypt(req.Value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt secret")
		return
	}

	id := generateID()
	now := time.Now().UTC()

	_, err = h.db.Exec(
		"INSERT INTO secrets (id, name, encrypted_value, description, tool_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, req.Name, encrypted, req.Description, req.ToolID, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create secret")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "secret_created", "secret", "secret", id, req.Name)

	go h.restartToolsUsing(req.Name)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          id,
		"name":        req.Name,
		"description": req.Description,
		"tool_id":     req.ToolID,
		"created_at":  now,
		"updated_at":  now,
	})
}

func (h *SecretsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := h.db.Exec("DELETE FROM secrets WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete secret")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "secret not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "secret_deleted", "secret", "secret", id, "")

	writeJSON(w, http.StatusOK, map[string]string{"message": "secret deleted"})
}

func (h *SecretsHandler) Rotate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var secretName string
	err := h.db.QueryRow("SELECT name FROM secrets WHERE id = ?", id).Scan(&secretName)
	if err != nil {
		writeError(w, http.StatusNotFound, "secret not found")
		return
	}

	var req struct {
		Value  string  `json:"value"`
		ToolID *string `json:"tool_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()

	if req.Value != "" {
		encrypted, err := h.manager.Encrypt(req.Value)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt secret")
			return
		}
		h.db.Exec("UPDATE secrets SET encrypted_value = ?, updated_at = ? WHERE id = ?", encrypted, now, id)
	}

	if req.ToolID != nil {
		h.db.Exec("UPDATE secrets SET tool_id = ?, updated_at = ? WHERE id = ?", *req.ToolID, now, id)
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "secret_rotated", "secret", "secret", id, "")

	go h.restartToolsUsing(secretName)

	writeJSON(w, http.StatusOK, map[string]string{"message": "secret updated"})
}

func (h *SecretsHandler) CheckNames(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Names []string `json:"names"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	type secretStatus struct {
		Name        string `json:"name"`
		Exists      bool   `json:"exists"`
		Placeholder bool   `json:"placeholder"`
	}

	results := make([]secretStatus, 0, len(req.Names))
	for _, name := range req.Names {
		var encrypted string
		err := h.db.QueryRow("SELECT encrypted_value FROM secrets WHERE name = ?", name).Scan(&encrypted)
		if err != nil {
			results = append(results, secretStatus{Name: name, Exists: false, Placeholder: false})
			continue
		}
		isPlaceholder := false
		if decrypted, decErr := h.manager.Decrypt(encrypted); decErr == nil && decrypted == "REPLACE_ME" {
			isPlaceholder = true
		}
		results = append(results, secretStatus{Name: name, Exists: true, Placeholder: isPlaceholder})
	}

	writeJSON(w, http.StatusOK, results)
}

func (h *SecretsHandler) EnsurePlaceholders(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Secrets []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"secrets"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	created := 0
	for _, s := range req.Secrets {
		if s.Name == "" {
			continue
		}
		var exists int
		h.db.QueryRow("SELECT COUNT(*) FROM secrets WHERE name = ?", s.Name).Scan(&exists)
		if exists > 0 {
			continue
		}
		encrypted, err := h.manager.Encrypt("REPLACE_ME")
		if err != nil {
			continue
		}
		id := generateID()
		now := time.Now().UTC()
		desc := s.Description
		if desc == "" {
			desc = "Placeholder — replace with your real value"
		}
		h.db.Exec(
			"INSERT INTO secrets (id, name, encrypted_value, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			id, s.Name, encrypted, desc, now, now,
		)
		created++
	}

	userID := middleware.GetUserID(r.Context())
	if created > 0 {
		h.db.LogAudit(userID, "secrets_placeholders_created", "secret", "secret", "", "")
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"created": created})
}

func (h *SecretsHandler) Test(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var encrypted string
	err := h.db.QueryRow("SELECT encrypted_value FROM secrets WHERE id = ?", id).Scan(&encrypted)
	if err != nil {
		writeError(w, http.StatusNotFound, "secret not found")
		return
	}

	_, err = h.manager.Decrypt(encrypted)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   false,
			"message": "failed to decrypt secret",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":   true,
		"message": "secret is valid and can be decrypted",
	})
}
