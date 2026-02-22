package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/skilllibrary"
)

type SkillLibraryHandler struct {
	db      *database.DB
	dataDir string
}

func NewSkillLibraryHandler(db *database.DB, dataDir string) *SkillLibraryHandler {
	return &SkillLibraryHandler{db: db, dataDir: dataDir}
}

func (h *SkillLibraryHandler) ListCatalog(w http.ResponseWriter, r *http.Request) {
	skills, err := skilllibrary.LoadRegistry()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load catalog")
		return
	}
	skills = skilllibrary.MarkInstalled(skills, h.dataDir)

	category := r.URL.Query().Get("category")
	search := strings.ToLower(r.URL.Query().Get("q"))

	var filtered []skilllibrary.CatalogSkill
	for _, s := range skills {
		if category != "" && !strings.EqualFold(s.Category, category) {
			continue
		}
		if search != "" {
			match := strings.Contains(strings.ToLower(s.Name), search) ||
				strings.Contains(strings.ToLower(s.Description), search)
			if !match {
				for _, tag := range s.Tags {
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
		filtered = append(filtered, s)
	}

	if filtered == nil {
		filtered = []skilllibrary.CatalogSkill{}
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (h *SkillLibraryHandler) GetCatalogSkill(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	skill, err := skilllibrary.GetCatalogSkill(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "catalog skill not found")
		return
	}

	skill.Installed = skilllibrary.IsInstalled(h.dataDir, slug)
	writeJSON(w, http.StatusOK, skill)
}

func (h *SkillLibraryHandler) InstallCatalogSkill(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	if skilllibrary.IsInstalled(h.dataDir, slug) {
		writeError(w, http.StatusConflict, "skill already installed")
		return
	}

	if err := skilllibrary.InstallSkill(h.dataDir, slug); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("install failed: %v", err))
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "library_skill_installed", "skill", "skill", slug, slug)

	content, _ := agents.GetGlobalSkill(h.dataDir, slug)
	skill := agents.BuildSkillFromFile(slug, content)
	writeJSON(w, http.StatusCreated, skill)
}
