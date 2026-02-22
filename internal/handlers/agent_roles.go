package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/models"
)

type AgentRolesHandler struct {
	db      *database.DB
	dataDir string
}

func NewAgentRolesHandler(db *database.DB, dataDir string) *AgentRolesHandler {
	return &AgentRolesHandler{db: db, dataDir: dataDir}
}

var slugRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (h *AgentRolesHandler) List(w http.ResponseWriter, r *http.Request) {
	enabledOnly := r.URL.Query().Get("enabled") == "true"

	query := "SELECT id, slug, name, description, system_prompt, model, avatar_path, enabled, sort_order, is_preset, identity_initialized, heartbeat_enabled, library_slug, library_version, created_at, updated_at FROM agent_roles"
	if enabledOnly {
		query += " WHERE enabled = 1"
	}
	query += " ORDER BY sort_order ASC"

	rows, err := h.db.Query(query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agent roles")
		return
	}
	defer rows.Close()

	roles := []models.AgentRole{}
	for rows.Next() {
		var role models.AgentRole
		if err := rows.Scan(&role.ID, &role.Slug, &role.Name, &role.Description, &role.SystemPrompt, &role.Model, &role.AvatarPath, &role.Enabled, &role.SortOrder, &role.IsPreset, &role.IdentityInitialized, &role.HeartbeatEnabled, &role.LibrarySlug, &role.LibraryVersion, &role.CreatedAt, &role.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan agent role")
			return
		}
		roles = append(roles, role)
	}
	writeJSON(w, http.StatusOK, roles)
}

func (h *AgentRolesHandler) Get(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	var role models.AgentRole
	err := h.db.QueryRow(
		"SELECT id, slug, name, description, system_prompt, model, avatar_path, enabled, sort_order, is_preset, identity_initialized, heartbeat_enabled, library_slug, library_version, created_at, updated_at FROM agent_roles WHERE slug = ?",
		slug,
	).Scan(&role.ID, &role.Slug, &role.Name, &role.Description, &role.SystemPrompt, &role.Model, &role.AvatarPath, &role.Enabled, &role.SortOrder, &role.IsPreset, &role.IdentityInitialized, &role.HeartbeatEnabled, &role.LibrarySlug, &role.LibraryVersion, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent role not found")
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (h *AgentRolesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		Slug         string `json:"slug"`
		Description  string `json:"description"`
		SystemPrompt string `json:"system_prompt"`
		Model        string `json:"model"`
		AvatarPath   string `json:"avatar_path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Slug == "" {
		req.Slug = strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))
	}
	if !slugRegex.MatchString(req.Slug) {
		writeError(w, http.StatusBadRequest, "slug must be lowercase alphanumeric with hyphens")
		return
	}
	if req.Model == "" {
		req.Model = "anthropic/claude-haiku-4-5"
	}

	// Check for duplicate slug
	var exists string
	err := h.db.QueryRow("SELECT id FROM agent_roles WHERE slug = ?", req.Slug).Scan(&exists)
	if err == nil {
		writeError(w, http.StatusConflict, "an agent with this slug already exists")
		return
	}

	// Get next sort order
	var maxSort int
	h.db.QueryRow("SELECT COALESCE(MAX(sort_order), 0) FROM agent_roles").Scan(&maxSort)

	id := uuid.New().String()
	now := time.Now().UTC()

	_, err = h.db.Exec(
		`INSERT INTO agent_roles (id, slug, name, description, system_prompt, model, avatar_path, enabled, sort_order, is_preset, identity_initialized, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, 0, 1, ?, ?)`,
		id, req.Slug, req.Name, req.Description, req.SystemPrompt, req.Model,
		req.AvatarPath, maxSort+1, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create agent role")
		return
	}

	// Auto-initialize identity files
	if err := agents.InitAgentDir(h.dataDir, req.Slug, req.Name, req.SystemPrompt); err != nil {
		logger.Error("Failed to init identity files for %s: %v", req.Slug, err)
	}

	role := models.AgentRole{
		ID:                  id,
		Slug:                req.Slug,
		Name:                req.Name,
		Description:         req.Description,
		SystemPrompt:        req.SystemPrompt,
		Model:               req.Model,
		AvatarPath:          req.AvatarPath,
		Enabled:             true,
		SortOrder:           maxSort + 1,
		IsPreset:            false,
		IdentityInitialized: true,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	writeJSON(w, http.StatusCreated, role)
}

func (h *AgentRolesHandler) Update(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	var existing models.AgentRole
	err := h.db.QueryRow(
		"SELECT id, slug, name, description, system_prompt, model, avatar_path, enabled, sort_order, is_preset, identity_initialized, heartbeat_enabled, library_slug, library_version, created_at, updated_at FROM agent_roles WHERE slug = ?",
		slug,
	).Scan(&existing.ID, &existing.Slug, &existing.Name, &existing.Description, &existing.SystemPrompt, &existing.Model, &existing.AvatarPath, &existing.Enabled, &existing.SortOrder, &existing.IsPreset, &existing.IdentityInitialized, &existing.HeartbeatEnabled, &existing.LibrarySlug, &existing.LibraryVersion, &existing.CreatedAt, &existing.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent role not found")
		return
	}

	var req struct {
		Name             *string `json:"name"`
		Description      *string `json:"description"`
		SystemPrompt     *string `json:"system_prompt"`
		Model            *string `json:"model"`
		AvatarPath       *string `json:"avatar_path"`
		HeartbeatEnabled *bool   `json:"heartbeat_enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.SystemPrompt != nil {
		existing.SystemPrompt = *req.SystemPrompt
	}
	if req.Model != nil {
		existing.Model = *req.Model
	}
	if req.AvatarPath != nil {
		existing.AvatarPath = *req.AvatarPath
	}
	if req.HeartbeatEnabled != nil {
		existing.HeartbeatEnabled = *req.HeartbeatEnabled
	}

	if existing.Name == "" {
		writeError(w, http.StatusBadRequest, "name cannot be empty")
		return
	}

	now := time.Now().UTC()
	_, err = h.db.Exec(
		`UPDATE agent_roles SET name = ?, description = ?, system_prompt = ?, model = ?, avatar_path = ?, heartbeat_enabled = ?, updated_at = ? WHERE slug = ?`,
		existing.Name, existing.Description, existing.SystemPrompt, existing.Model, existing.AvatarPath, existing.HeartbeatEnabled, now, slug,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update agent role")
		return
	}

	existing.UpdatedAt = now
	writeJSON(w, http.StatusOK, existing)
}

func (h *AgentRolesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	var exists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM agent_roles WHERE slug = ?)", slug).Scan(&exists)
	if err != nil || !exists {
		writeError(w, http.StatusNotFound, "agent role not found")
		return
	}

	// Remove from all thread memberships (messages are intentionally kept)
	h.db.Exec("DELETE FROM thread_members WHERE agent_role_slug = ?", slug)

	// Remove tool access grants
	h.db.Exec("DELETE FROM agent_tool_access WHERE agent_role_slug = ?", slug)

	_, err = h.db.Exec("DELETE FROM agent_roles WHERE slug = ?", slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete agent role")
		return
	}

	// Clean up agent identity directory
	agentDir := agents.AgentDir(h.dataDir, slug)
	if err := os.RemoveAll(agentDir); err != nil {
		logger.Error("Failed to remove agent dir %s: %v", agentDir, err)
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "agent role deleted"})
}

func (h *AgentRolesHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	var currentEnabled bool
	err := h.db.QueryRow("SELECT enabled FROM agent_roles WHERE slug = ?", slug).Scan(&currentEnabled)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent role not found")
		return
	}

	now := time.Now().UTC()
	newEnabled := !currentEnabled
	_, err = h.db.Exec("UPDATE agent_roles SET enabled = ?, updated_at = ? WHERE slug = ?", newEnabled, now, slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to toggle agent role")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"slug": slug, "enabled": newEnabled})
}

func (h *AgentRolesHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(5 << 20) // 5MB max

	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeError(w, http.StatusBadRequest, "avatar file required")
		return
	}
	defer file.Close()

	// Validate content type
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

	// Create uploads directory
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
	writeJSON(w, http.StatusOK, map[string]string{"avatar_path": avatarURL})
}

func (h *AgentRolesHandler) ServeAvatar(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	// Sanitize filename to prevent path traversal
	filename = filepath.Base(filename)

	filePath := filepath.Join(h.dataDir, "avatars", filename)
	http.ServeFile(w, r, filePath)
}

func (h *AgentRolesHandler) SeedPresets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EnabledRoles []string `json:"enabled_roles"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := agents.SeedPresetRoles(h.db, req.EnabledRoles); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to seed agent roles")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "agent roles seeded"})
}

// Identity file endpoints

func (h *AgentRolesHandler) GetFiles(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	files, err := agents.ReadAllFiles(h.dataDir, slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read agent files: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (h *AgentRolesHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	filename := chi.URLParam(r, "*")
	if filename == "" {
		writeError(w, http.StatusBadRequest, "filename is required")
		return
	}

	content, err := agents.ReadIdentityFile(h.dataDir, slug, filename)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"filename": filename, "content": content})
}

func (h *AgentRolesHandler) UpdateFile(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	filename := chi.URLParam(r, "*")
	if filename == "" {
		writeError(w, http.StatusBadRequest, "filename is required")
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := agents.WriteIdentityFile(h.dataDir, slug, filename, req.Content); err != nil {
		writeError(w, http.StatusBadRequest, "failed to write file: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"filename": filename, "status": "saved"})
}

func (h *AgentRolesHandler) InitFiles(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	var name, systemPrompt string
	err := h.db.QueryRow(
		"SELECT name, system_prompt FROM agent_roles WHERE slug = ?", slug,
	).Scan(&name, &systemPrompt)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent role not found")
		return
	}

	if err := agents.InitAgentDir(h.dataDir, slug, name, systemPrompt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize identity files: "+err.Error())
		return
	}

	h.db.Exec("UPDATE agent_roles SET identity_initialized = 1 WHERE slug = ?", slug)

	writeJSON(w, http.StatusOK, map[string]string{"status": "initialized"})
}

func (h *AgentRolesHandler) ListMemory(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	files, err := agents.ListMemoryFiles(h.dataDir, slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list memory files")
		return
	}
	writeJSON(w, http.StatusOK, files)
}

// Gateway file endpoints

func (h *AgentRolesHandler) GetGatewayFiles(w http.ResponseWriter, r *http.Request) {
	files, err := agents.ReadGatewayFiles(h.dataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read gateway files: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (h *AgentRolesHandler) GetGatewayFile(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "*")
	if filename == "" {
		writeError(w, http.StatusBadRequest, "filename is required")
		return
	}

	content, err := agents.ReadGatewayFile(h.dataDir, filename)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"filename": filename, "content": content})
}

func (h *AgentRolesHandler) UpdateGatewayFile(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "*")
	if filename == "" {
		writeError(w, http.StatusBadRequest, "filename is required")
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := agents.WriteGatewayFile(h.dataDir, filename, req.Content); err != nil {
		writeError(w, http.StatusBadRequest, "failed to write file: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"filename": filename, "status": "saved"})
}

func (h *AgentRolesHandler) GetGatewayMemory(w http.ResponseWriter, r *http.Request) {
	files, err := agents.ListGatewayMemoryFiles(h.dataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list gateway memory files")
		return
	}
	writeJSON(w, http.StatusOK, files)
}

// Tool access management

func (h *AgentRolesHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	type agentTool struct {
		models.Tool
		AccessType string `json:"access_type"` // "owned" or "granted"
	}

	var tools []agentTool

	// Owned tools
	rows, err := h.db.Query(
		"SELECT id, name, description, type, config, enabled, status, owner_agent_slug, created_at, updated_at FROM tools WHERE owner_agent_slug = ? AND deleted_at IS NULL ORDER BY created_at DESC",
		slug,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tools")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var t models.Tool
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Type, &t.Config, &t.Enabled, &t.Status, &t.OwnerAgentSlug, &t.CreatedAt, &t.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan tool")
			return
		}
		tools = append(tools, agentTool{Tool: t, AccessType: "owned"})
	}

	// Granted tools
	grantRows, err := h.db.Query(
		`SELECT t.id, t.name, t.description, t.type, t.config, t.enabled, t.status, t.owner_agent_slug, t.created_at, t.updated_at
		 FROM tools t
		 INNER JOIN agent_tool_access ata ON ata.tool_id = t.id
		 WHERE ata.agent_role_slug = ? AND t.deleted_at IS NULL
		 ORDER BY ata.granted_at DESC`,
		slug,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list granted tools")
		return
	}
	defer grantRows.Close()

	for grantRows.Next() {
		var t models.Tool
		if err := grantRows.Scan(&t.ID, &t.Name, &t.Description, &t.Type, &t.Config, &t.Enabled, &t.Status, &t.OwnerAgentSlug, &t.CreatedAt, &t.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan granted tool")
			return
		}
		tools = append(tools, agentTool{Tool: t, AccessType: "granted"})
	}

	if tools == nil {
		tools = []agentTool{}
	}
	writeJSON(w, http.StatusOK, tools)
}

func (h *AgentRolesHandler) GrantToolAccess(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	toolID := chi.URLParam(r, "toolId")

	id := uuid.New().String()
	now := time.Now().UTC()

	_, err := h.db.Exec(
		"INSERT OR IGNORE INTO agent_tool_access (id, agent_role_slug, tool_id, granted_at) VALUES (?, ?, ?, ?)",
		id, slug, toolID, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to grant access")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "granted"})
}

func (h *AgentRolesHandler) RevokeToolAccess(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	toolID := chi.URLParam(r, "toolId")

	result, err := h.db.Exec(
		"DELETE FROM agent_tool_access WHERE agent_role_slug = ? AND tool_id = ?",
		slug, toolID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke access")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "access grant not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *AgentRolesHandler) UpdateToolOwner(w http.ResponseWriter, r *http.Request) {
	toolID := chi.URLParam(r, "id")

	var req struct {
		OwnerAgentSlug string `json:"owner_agent_slug"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()
	result, err := h.db.Exec(
		"UPDATE tools SET owner_agent_slug = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL",
		req.OwnerAgentSlug, now, toolID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update owner")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "owner_agent_slug": req.OwnerAgentSlug})
}
