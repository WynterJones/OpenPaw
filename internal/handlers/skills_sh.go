package handlers

import (
	"net/http"
	"strings"

	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/skillssh"
)

type SkillsShHandler struct {
	db      *database.DB
	dataDir string
	client  *skillssh.Client
}

func NewSkillsShHandler(db *database.DB, dataDir string) *SkillsShHandler {
	return &SkillsShHandler{
		db:      db,
		dataDir: dataDir,
		client:  skillssh.NewClient(),
	}
}

func (h *SkillsShHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	results, err := h.client.Search(query)
	if err != nil {
		writeError(w, http.StatusBadGateway, "skills.sh search failed")
		return
	}

	type resultForFrontend struct {
		ID        string `json:"id"`
		SkillID   string `json:"skill_id"`
		Name      string `json:"name"`
		Installs  int    `json:"installs"`
		Source    string `json:"source"`
		Installed bool   `json:"installed"`
	}

	out := make([]resultForFrontend, 0, len(results))
	for _, s := range results {
		name := skillssh.SanitizeSkillName(s.SkillID)
		installed := false
		if name != "" {
			_, err := agents.GetGlobalSkill(h.dataDir, name)
			installed = err == nil
		}
		out = append(out, resultForFrontend{
			ID:        s.ID,
			SkillID:   s.SkillID,
			Name:      s.Name,
			Installs:  s.Installs,
			Source:    s.Source,
			Installed: installed,
		})
	}

	writeJSON(w, http.StatusOK, out)
}

func (h *SkillsShHandler) GetSkill(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	skillID := r.URL.Query().Get("skill")
	if source == "" || skillID == "" {
		writeError(w, http.StatusBadRequest, "source and skill parameters required")
		return
	}

	content, err := h.client.FetchSkillContent(source, skillID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch skill from GitHub")
		return
	}

	meta, body := agents.ParseFrontmatter(content)

	name := skillssh.SanitizeSkillName(skillID)
	_, checkErr := agents.GetGlobalSkill(h.dataDir, name)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"skill_id":    skillID,
		"name":        meta.Name,
		"source":      source,
		"description": meta.Description,
		"content":     content,
		"body":        body,
		"installed":   checkErr == nil,
	})
}

type installSkillsShRequest struct {
	Source  string `json:"source"`
	SkillID string `json:"skill_id"`
}

func (h *SkillsShHandler) Install(w http.ResponseWriter, r *http.Request) {
	var req installSkillsShRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Source == "" || req.SkillID == "" {
		writeError(w, http.StatusBadRequest, "source and skill_id are required")
		return
	}

	content, err := h.client.FetchSkillContent(req.Source, req.SkillID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch skill from GitHub")
		return
	}

	if len(content) > 1024*1024 {
		writeError(w, http.StatusBadRequest, "skill content exceeds 1MB limit")
		return
	}
	if strings.ContainsRune(content, 0) {
		writeError(w, http.StatusBadRequest, "skill content contains invalid characters")
		return
	}

	name := skillssh.SanitizeSkillName(req.SkillID)
	if !agents.IsValidSkillName(name) {
		writeError(w, http.StatusBadRequest, "skill name is invalid after sanitization")
		return
	}

	if _, err := agents.GetGlobalSkill(h.dataDir, name); err == nil {
		writeError(w, http.StatusConflict, "skill already installed")
		return
	}

	if err := agents.WriteGlobalSkill(h.dataDir, name, content); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write skill")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "skillssh_skill_installed", "skill", "skill", name, req.Source+"/"+req.SkillID)

	skill := agents.BuildSkillFromFile(name, content)
	writeJSON(w, http.StatusCreated, skill)
}
