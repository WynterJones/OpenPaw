package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/agents"
)

type SkillsHandler struct {
	dataDir string
}

func NewSkillsHandler(dataDir string) *SkillsHandler {
	return &SkillsHandler{dataDir: dataDir}
}

func (h *SkillsHandler) List(w http.ResponseWriter, r *http.Request) {
	skills, err := agents.ListGlobalSkills(h.dataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list skills")
		return
	}
	// Strip full content from list response — return metadata only
	for i := range skills {
		skills[i].Content = ""
	}
	writeJSON(w, http.StatusOK, skills)
}

func (h *SkillsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Content     string `json:"content"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Ensure frontmatter if description provided but content lacks it
	content := req.Content
	if req.Description != "" {
		meta, _ := agents.ParseFrontmatter(content)
		if meta.Description == "" {
			_, body := agents.ParseFrontmatter(content)
			content = agents.BuildFrontmatter(req.Name, req.Description, body)
		}
	}

	if err := agents.WriteGlobalSkill(h.dataDir, req.Name, content); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create skill: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, agents.BuildSkillFromFile(req.Name, content))
}

func (h *SkillsHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	content, err := agents.GetGlobalSkill(h.dataDir, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	writeJSON(w, http.StatusOK, agents.BuildSkillFromFile(name, content))
}

func (h *SkillsHandler) Update(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := agents.WriteGlobalSkill(h.dataDir, name, req.Content); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update skill: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, agents.Skill{
		Name:    name,
		Content: req.Content,
	})
}

func (h *SkillsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := agents.DeleteGlobalSkill(h.dataDir, name); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete skill")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "skill deleted"})
}

// Agent skill endpoints

func (h *SkillsHandler) ListAgentSkills(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	skills, err := agents.ListAgentSkills(h.dataDir, slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agent skills")
		return
	}
	// Strip full content from list response
	for i := range skills {
		skills[i].Content = ""
	}
	writeJSON(w, http.StatusOK, skills)
}

func (h *SkillsHandler) AddSkillToAgent(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "skill name is required")
		return
	}

	if err := agents.AddSkillToAgent(h.dataDir, req.Name, slug); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add skill: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "skill added to agent"})
}

func (h *SkillsHandler) UpdateAgentSkill(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	name := chi.URLParam(r, "name")
	var req struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	filename := "skills/" + name + "/SKILL.md"
	if err := agents.WriteIdentityFile(h.dataDir, slug, filename, req.Content); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update agent skill: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, agents.Skill{Name: name, Content: req.Content})
}

func (h *SkillsHandler) RemoveSkillFromAgent(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	name := chi.URLParam(r, "name")

	if err := agents.RemoveSkillFromAgent(h.dataDir, name, slug); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove skill: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "skill removed from agent"})
}

func (h *SkillsHandler) PublishAgentSkill(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	name := chi.URLParam(r, "name")

	if err := agents.PublishAgentSkill(h.dataDir, name, slug); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to publish skill: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "skill published to global"})
}
