package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
)

type AgentTasksHandler struct {
	db *database.DB
}

func NewAgentTasksHandler(db *database.DB) *AgentTasksHandler {
	return &AgentTasksHandler{db: db}
}

func (h *AgentTasksHandler) AllCounts(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		"SELECT agent_role_slug, COUNT(*) FROM agent_tasks WHERE status != 'done' GROUP BY agent_role_slug",
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query task counts")
		return
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var slug string
		var count int
		if rows.Scan(&slug, &count) != nil {
			continue
		}
		counts[slug] = count
	}
	writeJSON(w, http.StatusOK, counts)
}

func (h *AgentTasksHandler) List(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	rows, err := h.db.Query(
		"SELECT id, agent_role_slug, title, description, status, sort_order, created_at, updated_at FROM agent_tasks WHERE agent_role_slug = ? ORDER BY sort_order ASC, created_at ASC",
		slug,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	defer rows.Close()

	type task struct {
		ID        string `json:"id"`
		Slug      string `json:"agent_role_slug"`
		Title     string `json:"title"`
		Desc      string `json:"description"`
		Status    string `json:"status"`
		SortOrder int    `json:"sort_order"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	tasks := []task{}
	for rows.Next() {
		var t task
		var createdAt, updatedAt time.Time
		if rows.Scan(&t.ID, &t.Slug, &t.Title, &t.Desc, &t.Status, &t.SortOrder, &createdAt, &updatedAt) != nil {
			continue
		}
		t.CreatedAt = createdAt.Format(time.RFC3339)
		t.UpdatedAt = updatedAt.Format(time.RFC3339)
		tasks = append(tasks, t)
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (h *AgentTasksHandler) Create(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.Status == "" {
		req.Status = "backlog"
	}

	validStatuses := map[string]bool{"backlog": true, "doing": true, "blocked": true, "done": true}
	if !validStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	var maxOrder int
	h.db.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM agent_tasks WHERE agent_role_slug = ? AND status = ?", slug, req.Status).Scan(&maxOrder)

	_, err := h.db.Exec(
		"INSERT INTO agent_tasks (id, agent_role_slug, title, description, status, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		id, slug, req.Title, req.Description, req.Status, maxOrder+1, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}

	h.db.LogAudit("system", "task_created", "agent_task", "agent_role", slug, "title="+req.Title)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":              id,
		"agent_role_slug": slug,
		"title":           req.Title,
		"description":     req.Description,
		"status":          req.Status,
		"sort_order":      maxOrder + 1,
		"created_at":      now.Format(time.RFC3339),
		"updated_at":      now.Format(time.RFC3339),
	})
}

func (h *AgentTasksHandler) Counts(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	rows, err := h.db.Query(
		"SELECT status, COUNT(*) FROM agent_tasks WHERE agent_role_slug = ? GROUP BY status",
		slug,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query counts")
		return
	}
	defer rows.Close()

	counts := map[string]int{"backlog": 0, "doing": 0, "blocked": 0, "done": 0}
	for rows.Next() {
		var status string
		var count int
		if rows.Scan(&status, &count) != nil {
			continue
		}
		counts[status] = count
	}
	writeJSON(w, http.StatusOK, counts)
}

func (h *AgentTasksHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []struct {
			ID        string `json:"id"`
			SortOrder int    `json:"sort_order"`
		} `json:"items"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()
	for _, item := range req.Items {
		h.db.Exec(
			"UPDATE agent_tasks SET sort_order = ?, updated_at = ? WHERE id = ?",
			item.SortOrder, now, item.ID,
		)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AgentTasksHandler) Update(w http.ResponseWriter, r *http.Request) {
	taskId := chi.URLParam(r, "taskId")

	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
		SortOrder   *int    `json:"sort_order"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Status != nil {
		validStatuses := map[string]bool{"backlog": true, "doing": true, "blocked": true, "done": true}
		if !validStatuses[*req.Status] {
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
	}

	now := time.Now().UTC()

	if req.Title != nil {
		h.db.Exec("UPDATE agent_tasks SET title = ?, updated_at = ? WHERE id = ?", *req.Title, now, taskId)
	}
	if req.Description != nil {
		h.db.Exec("UPDATE agent_tasks SET description = ?, updated_at = ? WHERE id = ?", *req.Description, now, taskId)
	}
	if req.Status != nil {
		h.db.Exec("UPDATE agent_tasks SET status = ?, updated_at = ? WHERE id = ?", *req.Status, now, taskId)
	}
	if req.SortOrder != nil {
		h.db.Exec("UPDATE agent_tasks SET sort_order = ?, updated_at = ? WHERE id = ?", *req.SortOrder, now, taskId)
	}

	var t struct {
		ID        string    `json:"id"`
		Slug      string    `json:"agent_role_slug"`
		Title     string    `json:"title"`
		Desc      string    `json:"description"`
		Status    string    `json:"status"`
		SortOrder int       `json:"sort_order"`
		CreatedAt time.Time `json:"-"`
		UpdatedAt time.Time `json:"-"`
	}
	err := h.db.QueryRow(
		"SELECT id, agent_role_slug, title, description, status, sort_order, created_at, updated_at FROM agent_tasks WHERE id = ?",
		taskId,
	).Scan(&t.ID, &t.Slug, &t.Title, &t.Desc, &t.Status, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":              t.ID,
		"agent_role_slug": t.Slug,
		"title":           t.Title,
		"description":     t.Desc,
		"status":          t.Status,
		"sort_order":      t.SortOrder,
		"created_at":      t.CreatedAt.Format(time.RFC3339),
		"updated_at":      t.UpdatedAt.Format(time.RFC3339),
	})
}

func (h *AgentTasksHandler) Delete(w http.ResponseWriter, r *http.Request) {
	taskId := chi.URLParam(r, "taskId")

	result, err := h.db.Exec("DELETE FROM agent_tasks WHERE id = ?", taskId)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete task")
		return
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *AgentTasksHandler) ClearDone(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	result, err := h.db.Exec(
		"DELETE FROM agent_tasks WHERE agent_role_slug = ? AND status = 'done'",
		slug,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear done tasks")
		return
	}

	n, _ := result.RowsAffected()
	writeJSON(w, http.StatusOK, map[string]int{"deleted": int(n)})
}
