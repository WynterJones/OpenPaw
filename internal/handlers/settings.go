package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/agents"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/secrets"
)

type SettingsHandler struct {
	db         *database.DB
	agentMgr   *agents.Manager
	secretsMgr *secrets.Manager
	client     *llm.Client
	dataDir    string
}

func NewSettingsHandler(db *database.DB, agentMgr *agents.Manager, secretsMgr *secrets.Manager, client *llm.Client, dataDir string) *SettingsHandler {
	return &SettingsHandler{db: db, agentMgr: agentMgr, secretsMgr: secretsMgr, client: client, dataDir: dataDir}
}

func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query("SELECT key, value FROM settings ORDER BY key ASC")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get settings")
		return
	}
	defer rows.Close()

	settings := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			logger.Warn("scan setting row: %v", err)
			continue
		}
		// Don't expose encrypted API key in general settings
		if key == "openrouter_api_key" {
			continue
		}
		settings[key] = value
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	for key, value := range req {
		// Don't allow setting API key through general settings
		if key == "openrouter_api_key" {
			continue
		}
		h.upsertSetting(key, value)
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "settings_updated", "settings", "settings", "", "")

	h.Get(w, r)
}

const defaultDesignConfig = `{
  "surface_0": "#000000",
  "surface_1": "#0a0a0a",
  "surface_2": "#141414",
  "surface_3": "#1f1f1f",
  "border_0": "#1a1a1a",
  "border_1": "#2a2a2a",
  "text_0": "#f5f5f5",
  "text_1": "#d4d4d4",
  "text_2": "#8a8a8a",
  "text_3": "#555555",
  "accent": "#E84BA5",
  "accent_hover": "#D43D95",
  "accent_muted": "rgba(232, 75, 165, 0.1)",
  "accent_text": "#F472B6",
  "danger": "#dc2626",
  "danger_hover": "#b91c1c",
  "font_family": "'Inter', system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
  "bg_image": ""
}`

func (h *SettingsHandler) GetDesign(w http.ResponseWriter, r *http.Request) {
	var value string
	err := h.db.QueryRow("SELECT value FROM settings WHERE key = 'design_config'").Scan(&value)
	if err != nil || value == "" {
		value = defaultDesignConfig
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"design":%s}`, value)
}

func (h *SettingsHandler) UpdateDesign(w http.ResponseWriter, r *http.Request) {
	var raw json.RawMessage
	if err := decodeJSON(r, &raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	value := string(raw)
	h.upsertSetting("design_config", value)

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "design_config_updated", "settings", "settings", "design_config", "")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(value))
}

func (h *SettingsHandler) GetModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"gateway_model":     h.agentMgr.GatewayModel,
		"builder_model":     h.agentMgr.BuilderModel,
		"max_turns":         h.agentMgr.MaxTurns,
		"agent_timeout_min": h.agentMgr.AgentTimeoutMin,
	})
}

func (h *SettingsHandler) UpdateModels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GatewayModel    string `json:"gateway_model"`
		BuilderModel    string `json:"builder_model"`
		MaxTurns        *int   `json:"max_turns"`
		AgentTimeoutMin *int   `json:"agent_timeout_min"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.GatewayModel != "" {
		model := agents.ParseModel(req.GatewayModel, llm.ModelHaiku)
		h.agentMgr.GatewayModel = model
		h.upsertSetting("gateway_model", model)
	}
	if req.BuilderModel != "" {
		model := agents.ParseModel(req.BuilderModel, llm.ModelSonnet)
		h.agentMgr.BuilderModel = model
		h.upsertSetting("builder_model", model)
	}
	if req.MaxTurns != nil && *req.MaxTurns > 0 {
		h.agentMgr.MaxTurns = *req.MaxTurns
		h.upsertSetting("max_turns", fmt.Sprintf("%d", *req.MaxTurns))
	}
	if req.AgentTimeoutMin != nil && *req.AgentTimeoutMin > 0 {
		h.agentMgr.AgentTimeoutMin = *req.AgentTimeoutMin
		h.upsertSetting("agent_timeout_min", fmt.Sprintf("%d", *req.AgentTimeoutMin))
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "model_settings_updated", "settings", "settings", "", "")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"gateway_model":     h.agentMgr.GatewayModel,
		"builder_model":     h.agentMgr.BuilderModel,
		"max_turns":         h.agentMgr.MaxTurns,
		"agent_timeout_min": h.agentMgr.AgentTimeoutMin,
	})
}

func (h *SettingsHandler) GetAPIKey(w http.ResponseWriter, r *http.Request) {
	source := resolveAPIKeySource(h.client)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"configured": source != "none",
		"source":     source,
	})
}

func (h *SettingsHandler) UpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	// Validate the key by making a test API call
	testClient := llm.NewClient(req.APIKey)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := testClient.ValidateKey(ctx); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid API key: "+err.Error())
		return
	}

	// Encrypt and store
	encrypted, err := h.secretsMgr.Encrypt(req.APIKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt API key")
		return
	}

	h.upsertSetting("openrouter_api_key", encrypted)

	// Hot-reload the client
	if h.client != nil {
		h.client.UpdateAPIKey(req.APIKey)
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "api_key_updated", "settings", "settings", "openrouter_api_key", "")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"configured": true,
		"source":     "database",
	})
}

func (h *SettingsHandler) AvailableModels(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || !h.client.IsConfigured() {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	models, err := h.client.GetCachedModels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch models: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func (h *SettingsHandler) UploadBackground(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(5 << 20)

	file, header, err := r.FormFile("background")
	if err != nil {
		writeError(w, http.StatusBadRequest, "background file required")
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
		writeError(w, http.StatusBadRequest, "background must be PNG, JPEG, or WebP")
		return
	}

	if !validateImageMagicBytes(file, ext) {
		writeError(w, http.StatusBadRequest, "file content does not match declared type")
		return
	}

	bgDir := filepath.Join(h.dataDir, "backgrounds")
	os.MkdirAll(bgDir, 0755)

	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	destPath := filepath.Join(bgDir, filename)

	dest, err := os.Create(destPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save background")
		return
	}
	defer dest.Close()

	if _, err := io.Copy(dest, file); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write background")
		return
	}

	bgURL := "/api/v1/uploads/backgrounds/" + filename

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "background_uploaded", "settings", "settings", "background", "")

	writeJSON(w, http.StatusOK, map[string]string{"url": bgURL})
}

func (h *SettingsHandler) DeleteBackground(w http.ResponseWriter, r *http.Request) {
	bgDir := filepath.Join(h.dataDir, "backgrounds")
	entries, err := os.ReadDir(bgDir)
	if err == nil {
		for _, e := range entries {
			os.Remove(filepath.Join(bgDir, e.Name()))
		}
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "background_deleted", "settings", "settings", "background", "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *SettingsHandler) ServeBackground(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if filename == "" || strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		http.NotFound(w, r)
		return
	}
	bgPath := filepath.Join(h.dataDir, "backgrounds", filename)
	http.ServeFile(w, r, bgPath)
}

func (h *SettingsHandler) upsertSetting(key, value string) {
	if _, err := h.db.Exec(
		"INSERT INTO settings (id, key, value) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		uuid.New().String(), key, value,
	); err != nil {
		logger.Error("Failed to upsert setting %s: %v", key, err)
	}
}

func generateID() string {
	return uuid.New().String()
}

func resolveAPIKeySource(client *llm.Client) string {
	if client == nil || !client.IsConfigured() {
		return "none"
	}
	if os.Getenv("OPENROUTER_API_KEY") != "" || os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "env"
	}
	return "database"
}
