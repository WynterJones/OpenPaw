package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
	"github.com/openpaw/openpaw/internal/toolmgr"
)

type ToolsHandler struct {
	db           *database.DB
	agentManager *agents.Manager
	toolMgr      *toolmgr.Manager
	toolsDir     string
}

func NewToolsHandler(db *database.DB, agentManager *agents.Manager, toolMgr *toolmgr.Manager, toolsDir string) *ToolsHandler {
	return &ToolsHandler{db: db, agentManager: agentManager, toolMgr: toolMgr, toolsDir: toolsDir}
}

func (h *ToolsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		"SELECT id, name, description, type, config, enabled, status, port, pid, capabilities, owner_agent_slug, library_slug, library_version, source_hash, binary_hash, created_at, updated_at FROM tools WHERE deleted_at IS NULL ORDER BY created_at DESC",
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tools")
		return
	}
	defer rows.Close()

	tools := []models.Tool{}
	for rows.Next() {
		var t models.Tool
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Type, &t.Config, &t.Enabled, &t.Status, &t.Port, &t.PID, &t.Capabilities, &t.OwnerAgentSlug, &t.LibrarySlug, &t.LibraryVersion, &t.SourceHash, &t.BinaryHash, &t.CreatedAt, &t.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan tool")
			return
		}
		tools = append(tools, t)
	}
	writeJSON(w, http.StatusOK, tools)
}

func (h *ToolsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Config      string `json:"config"`
		AutoBuild   bool   `json:"auto_build"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Type == "" {
		req.Type = "generic"
	}
	if req.Config == "" {
		req.Config = "{}"
	}

	id := generateID()
	now := time.Now().UTC()

	status := "active"
	enabled := true
	if req.AutoBuild {
		status = "building"
		enabled = false
	}

	_, err := h.db.Exec(
		"INSERT INTO tools (id, name, description, type, config, enabled, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id, req.Name, req.Description, req.Type, req.Config, enabled, status, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create tool")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "tool_created", "tool", "tool", id, req.Name)

	tool := models.Tool{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		Config:      req.Config,
		Enabled:     enabled,
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// If auto_build is requested, spawn a builder agent
	if req.AutoBuild && h.agentManager != nil {
		go h.spawnToolBuilder(id, req.Name, req.Description, req.Config, userID)
	}

	writeJSON(w, http.StatusCreated, tool)
}

func (h *ToolsHandler) spawnToolBuilder(toolID, name, description, config, userID string) {
	toolDir := filepath.Join(h.toolsDir, toolID)
	wo, err := agents.CreateWorkOrder(h.db, agents.WorkOrderToolBuild,
		name, description, config, toolDir, toolID, "", userID,
	)
	if err != nil {
		return
	}

	_, err = h.agentManager.SpawnToolBuilder(context.Background(), wo, "", userID)
	if err != nil {
		now := time.Now().UTC()
		if _, dbErr := h.db.Exec("UPDATE tools SET status = 'error', updated_at = ? WHERE id = ?", now, toolID); dbErr != nil {
			logger.Error("Failed to update tool status: %v", dbErr)
		}
	}
}

func (h *ToolsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var t models.Tool
	err := h.db.QueryRow(
		"SELECT id, name, description, type, config, enabled, status, port, pid, capabilities, owner_agent_slug, library_slug, library_version, source_hash, binary_hash, created_at, updated_at FROM tools WHERE id = ? AND deleted_at IS NULL",
		id,
	).Scan(&t.ID, &t.Name, &t.Description, &t.Type, &t.Config, &t.Enabled, &t.Status, &t.Port, &t.PID, &t.Capabilities, &t.OwnerAgentSlug, &t.LibrarySlug, &t.LibraryVersion, &t.SourceHash, &t.BinaryHash, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}

	// Read manifest from disk if available
	var manifest json.RawMessage
	manifestPath := filepath.Join(h.toolsDir, id, "manifest.json")
	if data, err := os.ReadFile(manifestPath); err == nil {
		manifest = data
	}

	resp := struct {
		models.Tool
		Manifest json.RawMessage `json:"manifest,omitempty"`
	}{Tool: t, Manifest: manifest}

	writeJSON(w, http.StatusOK, resp)
}

func (h *ToolsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var existing models.Tool
	err := h.db.QueryRow(
		"SELECT id FROM tools WHERE id = ? AND deleted_at IS NULL", id,
	).Scan(&existing.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Type        *string `json:"type"`
		Config      *string `json:"config"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()
	if req.Name != nil {
		h.db.Exec("UPDATE tools SET name = ?, updated_at = ? WHERE id = ?", *req.Name, now, id)
	}
	if req.Description != nil {
		h.db.Exec("UPDATE tools SET description = ?, updated_at = ? WHERE id = ?", *req.Description, now, id)
	}
	if req.Type != nil {
		h.db.Exec("UPDATE tools SET type = ?, updated_at = ? WHERE id = ?", *req.Type, now, id)
	}
	if req.Config != nil {
		h.db.Exec("UPDATE tools SET config = ?, updated_at = ? WHERE id = ?", *req.Config, now, id)
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "tool_updated", "tool", "tool", id, "")

	var t models.Tool
	h.db.QueryRow(
		"SELECT id, name, description, type, config, enabled, status, port, pid, capabilities, owner_agent_slug, library_slug, library_version, source_hash, binary_hash, created_at, updated_at FROM tools WHERE id = ?", id,
	).Scan(&t.ID, &t.Name, &t.Description, &t.Type, &t.Config, &t.Enabled, &t.Status, &t.Port, &t.PID, &t.Capabilities, &t.OwnerAgentSlug, &t.LibrarySlug, &t.LibraryVersion, &t.SourceHash, &t.BinaryHash, &t.CreatedAt, &t.UpdatedAt)

	writeJSON(w, http.StatusOK, t)
}

func (h *ToolsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now().UTC()
	result, err := h.db.Exec("UPDATE tools SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL", now, now, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete tool")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "tool_deleted", "tool", "tool", id, "")

	writeJSON(w, http.StatusOK, map[string]string{"message": "tool deleted"})
}

func (h *ToolsHandler) Call(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var t models.Tool
	err := h.db.QueryRow(
		"SELECT id, name, type, config, enabled FROM tools WHERE id = ? AND deleted_at IS NULL", id,
	).Scan(&t.ID, &t.Name, &t.Type, &t.Config, &t.Enabled)
	if err != nil {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}
	if !t.Enabled {
		writeError(w, http.StatusBadRequest, "tool is disabled")
		return
	}

	var req struct {
		Endpoint string `json:"endpoint"`
		Payload  string `json:"payload"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	endpoint := req.Endpoint
	if endpoint == "" {
		endpoint = "/"
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "tool_called", "tool", "tool", id, endpoint)

	if h.toolMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "tool manager not available")
		return
	}

	// If payload is a JSON object, append its fields as query parameters
	// so tools that use GET with query params (e.g. ?city=Kingston) work correctly.
	if req.Payload != "" {
		var params map[string]interface{}
		if json.Unmarshal([]byte(req.Payload), &params) == nil && len(params) > 0 {
			q := url.Values{}
			for k, v := range params {
				q.Set(k, fmt.Sprintf("%v", v))
			}
			if idx := len(endpoint); idx > 0 {
				sep := "?"
				for i := 0; i < len(endpoint); i++ {
					if endpoint[i] == '?' {
						sep = "&"
						break
					}
				}
				endpoint = endpoint + sep + q.Encode()
			}
		}
	}

	data, err := h.toolMgr.CallTool(id, endpoint, nil)
	if err != nil {
		logger.Error("Tool call failed for %s endpoint %s: %v", id, endpoint, err)
		writeError(w, http.StatusBadGateway, "tool call failed: "+err.Error())
		return
	}

	var parsed interface{}
	if json.Unmarshal(data, &parsed) == nil {
		writeJSON(w, http.StatusOK, parsed)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}
}

func (h *ToolsHandler) Enable(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now().UTC()
	result, err := h.db.Exec("UPDATE tools SET enabled = 1, updated_at = ? WHERE id = ? AND deleted_at IS NULL", now, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enable tool")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}
	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "tool_enabled", "tool", "tool", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"message": "tool enabled"})
}

func (h *ToolsHandler) Disable(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now().UTC()
	result, err := h.db.Exec("UPDATE tools SET enabled = 0, updated_at = ? WHERE id = ? AND deleted_at IS NULL", now, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disable tool")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}
	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "tool_disabled", "tool", "tool", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"message": "tool disabled"})
}

func (h *ToolsHandler) Compile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.toolMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "tool manager not available")
		return
	}

	var exists string
	if err := h.db.QueryRow("SELECT id FROM tools WHERE id = ? AND deleted_at IS NULL", id).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}

	if err := h.toolMgr.CompileTool(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "tool_compiled", "tool", "tool", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"message": "tool compiled"})
}

func (h *ToolsHandler) Start(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.toolMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "tool manager not available")
		return
	}

	var exists string
	if err := h.db.QueryRow("SELECT id FROM tools WHERE id = ? AND deleted_at IS NULL", id).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}

	if err := h.toolMgr.StartTool(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "tool_started", "tool", "tool", id, "")
	writeJSON(w, http.StatusOK, h.toolMgr.GetStatus(id))
}

func (h *ToolsHandler) Stop(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.toolMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "tool manager not available")
		return
	}

	if err := h.toolMgr.StopTool(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "tool_stopped", "tool", "tool", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"message": "tool stopped"})
}

func (h *ToolsHandler) Restart(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.toolMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "tool manager not available")
		return
	}

	var exists string
	if err := h.db.QueryRow("SELECT id FROM tools WHERE id = ? AND deleted_at IS NULL", id).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}

	if err := h.toolMgr.RestartTool(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "tool_restarted", "tool", "tool", id, "")
	writeJSON(w, http.StatusOK, h.toolMgr.GetStatus(id))
}

func (h *ToolsHandler) Status(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.toolMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "tool manager not available")
		return
	}
	writeJSON(w, http.StatusOK, h.toolMgr.GetStatus(id))
}

func (h *ToolsHandler) Proxy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.toolMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "tool manager not available")
		return
	}

	routeCtx := chi.RouteContext(r.Context())
	proxyPath := "/" + routeCtx.URLParam("*")
	if r.URL.RawQuery != "" {
		proxyPath += "?" + r.URL.RawQuery
	}

	resp, err := h.toolMgr.ProxyRequest(id, proxyPath)
	if err != nil {
		writeError(w, http.StatusBadGateway, "proxy failed: "+err.Error())
		return
	}

	if resp.ContentType != "" {
		w.Header().Set("Content-Type", resp.ContentType)
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(resp.Body)
}

func (h *ToolsHandler) WidgetJS(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.toolMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "tool manager not available")
		return
	}

	result, err := h.toolMgr.CallTool(id, "/widget.js", nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch widget.js: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(result)
}
