package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
)

type ProjectsHandler struct {
	db *database.DB
}

func NewProjectsHandler(db *database.DB) *ProjectsHandler {
	return &ProjectsHandler{db: db}
}

type projectResponse struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Color     string             `json:"color"`
	Repos     []projectRepoResp  `json:"repos"`
	CreatedAt string             `json:"created_at"`
}

type projectRepoResp struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	Name       string `json:"name"`
	FolderPath string `json:"folder_path"`
	Command    string `json:"command"`
	SortOrder  int    `json:"sort_order"`
}

// List returns all projects with their repos.
func (h *ProjectsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query("SELECT id, name, color, created_at FROM projects ORDER BY created_at")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var projects []projectResponse
	for rows.Next() {
		var p projectResponse
		var createdAt time.Time
		if err := rows.Scan(&p.ID, &p.Name, &p.Color, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		p.CreatedAt = createdAt.Format(time.RFC3339)
		p.Repos = []projectRepoResp{}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if projects == nil {
		projects = []projectResponse{}
	}

	// Load repos for all projects
	repoRows, err := h.db.Query("SELECT id, project_id, name, folder_path, command, sort_order FROM project_repos ORDER BY sort_order")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer repoRows.Close()

	repoMap := make(map[string][]projectRepoResp)
	for repoRows.Next() {
		var repo projectRepoResp
		if err := repoRows.Scan(&repo.ID, &repo.ProjectID, &repo.Name, &repo.FolderPath, &repo.Command, &repo.SortOrder); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		repoMap[repo.ProjectID] = append(repoMap[repo.ProjectID], repo)
	}

	for i := range projects {
		if repos, ok := repoMap[projects[i].ID]; ok {
			projects[i].Repos = repos
		}
	}

	writeJSON(w, http.StatusOK, projects)
}

// Create creates a new project with optional repos.
func (h *ProjectsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
		Repos []struct {
			Name       string `json:"name"`
			FolderPath string `json:"folder_path"`
			Command    string `json:"command"`
		} `json:"repos"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	projectID := uuid.New().String()
	now := time.Now().UTC()

	_, err := h.db.Exec("INSERT INTO projects (id, name, color, created_at) VALUES (?, ?, ?, ?)",
		projectID, req.Name, req.Color, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := projectResponse{
		ID:        projectID,
		Name:      req.Name,
		Color:     req.Color,
		Repos:     []projectRepoResp{},
		CreatedAt: now.Format(time.RFC3339),
	}

	for i, repo := range req.Repos {
		repoID := uuid.New().String()
		repoName := repo.Name
		if repoName == "" {
			repoName = repo.FolderPath
		}
		_, err := h.db.Exec(
			"INSERT INTO project_repos (id, project_id, name, folder_path, command, sort_order) VALUES (?, ?, ?, ?, ?, ?)",
			repoID, projectID, repoName, repo.FolderPath, repo.Command, i,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp.Repos = append(resp.Repos, projectRepoResp{
			ID:         repoID,
			ProjectID:  projectID,
			Name:       repoName,
			FolderPath: repo.FolderPath,
			Command:    repo.Command,
			SortOrder:  i,
		})
	}

	writeJSON(w, http.StatusCreated, resp)
}

// Update updates a project and replaces its repos.
func (h *ProjectsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
		Repos []struct {
			Name       string `json:"name"`
			FolderPath string `json:"folder_path"`
			Command    string `json:"command"`
		} `json:"repos"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	res, err := h.db.Exec("UPDATE projects SET name = ?, color = ? WHERE id = ?", req.Name, req.Color, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	// Replace repos
	h.db.Exec("DELETE FROM project_repos WHERE project_id = ?", id)

	repos := []projectRepoResp{}
	for i, repo := range req.Repos {
		repoID := uuid.New().String()
		repoName := repo.Name
		if repoName == "" {
			repoName = repo.FolderPath
		}
		h.db.Exec(
			"INSERT INTO project_repos (id, project_id, name, folder_path, command, sort_order) VALUES (?, ?, ?, ?, ?, ?)",
			repoID, id, repoName, repo.FolderPath, repo.Command, i,
		)
		repos = append(repos, projectRepoResp{
			ID:         repoID,
			ProjectID:  id,
			Name:       repoName,
			FolderPath: repo.FolderPath,
			Command:    repo.Command,
			SortOrder:  i,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "updated",
		"repos":  repos,
	})
}

// Delete removes a project and its repos.
func (h *ProjectsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.db.Exec("DELETE FROM project_repos WHERE project_id = ?", id)
	res, err := h.db.Exec("DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
