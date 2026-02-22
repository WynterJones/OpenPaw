package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/agentlibrary"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
)

type AgentLibraryHandler struct {
	db      *database.DB
	dataDir string
}

func NewAgentLibraryHandler(db *database.DB, dataDir string) *AgentLibraryHandler {
	return &AgentLibraryHandler{db: db, dataDir: dataDir}
}

func (h *AgentLibraryHandler) ListCatalog(w http.ResponseWriter, r *http.Request) {
	agents, err := agentlibrary.LoadRegistry()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load catalog")
		return
	}
	agents = agentlibrary.MarkInstalled(agents, h.db)

	category := r.URL.Query().Get("category")
	search := strings.ToLower(r.URL.Query().Get("q"))

	var filtered []agentlibrary.CatalogAgent
	for _, a := range agents {
		if category != "" && !strings.EqualFold(a.Category, category) {
			continue
		}
		if search != "" {
			match := strings.Contains(strings.ToLower(a.Name), search) ||
				strings.Contains(strings.ToLower(a.Description), search)
			if !match {
				for _, tag := range a.Tags {
					if strings.Contains(strings.ToLower(tag), search) {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}
		filtered = append(filtered, a)
	}

	if filtered == nil {
		filtered = []agentlibrary.CatalogAgent{}
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (h *AgentLibraryHandler) GetCatalogAgent(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	agent, err := agentlibrary.GetCatalogAgent(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "catalog agent not found")
		return
	}

	installed, installedSlug := agentlibrary.IsInstalled(h.db, slug)
	agent.Installed = installed

	resp := struct {
		agentlibrary.CatalogAgent
		InstalledSlug string `json:"installed_slug,omitempty"`
	}{
		CatalogAgent:  *agent,
		InstalledSlug: installedSlug,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AgentLibraryHandler) InstallCatalogAgent(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	if installed, _ := agentlibrary.IsInstalled(h.db, slug); installed {
		writeError(w, http.StatusConflict, "agent already installed")
		return
	}

	var existingID string
	err := h.db.QueryRow("SELECT id FROM agent_roles WHERE slug = ?", slug).Scan(&existingID)
	if err == nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("an agent with slug '%s' already exists", slug))
		return
	}

	agentSlug, err := agentlibrary.InstallAgent(h.db, slug, h.dataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("install failed: %v", err))
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "library_agent_installed", "agent", "agent_role", agentSlug, slug)

	var a models.AgentRole
	h.db.QueryRow(
		"SELECT id, slug, name, description, system_prompt, model, avatar_path, enabled, sort_order, is_preset, identity_initialized, heartbeat_enabled, library_slug, library_version, created_at, updated_at FROM agent_roles WHERE slug = ?",
		agentSlug,
	).Scan(&a.ID, &a.Slug, &a.Name, &a.Description, &a.SystemPrompt, &a.Model, &a.AvatarPath, &a.Enabled, &a.SortOrder, &a.IsPreset, &a.IdentityInitialized, &a.HeartbeatEnabled, &a.LibrarySlug, &a.LibraryVersion, &a.CreatedAt, &a.UpdatedAt)

	writeJSON(w, http.StatusCreated, a)
}
