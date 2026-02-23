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
		Folder      string `json:"folder"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	content := req.Content
	meta, body := agents.ParseFrontmatter(content)
	if req.Description != "" && meta.Description == "" {
		meta.Description = req.Description
	}
	if meta.Name == "" {
		meta.Name = req.Name
	}
	if req.Folder != "" {
		meta.Folder = req.Folder
	}
	if meta.Description != "" || meta.Folder != "" {
		content = agents.BuildFrontmatterFromMeta(meta, body)
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
		Content string  `json:"content"`
		Folder  *string `json:"folder"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	content := req.Content

	// If folder is being updated, parse existing content and rebuild frontmatter
	if req.Folder != nil {
		// If no content provided, read existing
		if content == "" {
			existing, err := agents.GetGlobalSkill(h.dataDir, name)
			if err != nil {
				writeError(w, http.StatusNotFound, "skill not found")
				return
			}
			content = existing
		}
		meta, body := agents.ParseFrontmatter(content)
		meta.Folder = *req.Folder
		if meta.Name == "" {
			meta.Name = name
		}
		content = agents.BuildFrontmatterFromMeta(meta, body)
	}

	if err := agents.WriteGlobalSkill(h.dataDir, name, content); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update skill: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, agents.BuildSkillFromFile(name, content))
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
