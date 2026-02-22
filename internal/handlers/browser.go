package handlers

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/browser"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
)

type browserTaskResponse struct {
	ID            string  `json:"id"`
	SessionID     string  `json:"session_id"`
	ThreadID      string  `json:"thread_id"`
	AgentRoleSlug string  `json:"agent_role_slug"`
	Title         string  `json:"title"`
	Status        string  `json:"status"`
	Instructions  string  `json:"instructions,omitempty"`
	Result        string  `json:"result,omitempty"`
	ExtractedData string  `json:"extracted_data,omitempty"`
	Error         string  `json:"error"`
	StartedAt     *string `json:"started_at"`
	CompletedAt   *string `json:"completed_at"`
	CreatedAt     string  `json:"created_at"`
}

type BrowserHandler struct {
	db         *database.DB
	browserMgr *browser.Manager
}

func NewBrowserHandler(db *database.DB, browserMgr *browser.Manager) *BrowserHandler {
	return &BrowserHandler{db: db, browserMgr: browserMgr}
}

func (h *BrowserHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := h.browserMgr.ListSessions()

	type sessionResponse struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Status         string `json:"status"`
		Headless       bool   `json:"headless"`
		CurrentURL     string `json:"current_url"`
		CurrentTitle   string `json:"current_title"`
		OwnerAgentSlug string `json:"owner_agent_slug"`
		CreatedAt      string `json:"created_at"`
		UpdatedAt      string `json:"updated_at"`
	}

	result := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, sessionResponse{
			ID:             s.ID,
			Name:           s.Name,
			Status:         string(s.Status),
			Headless:       s.Headless,
			CurrentURL:     s.CurrentURL,
			CurrentTitle:   s.CurrentTitle,
			OwnerAgentSlug: s.OwnerAgentSlug,
			CreatedAt:      s.CreatedAt.Format(time.RFC3339),
			UpdatedAt:      s.UpdatedAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *BrowserHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"name"`
		Headless       *bool  `json:"headless"`
		OwnerAgentSlug string `json:"owner_agent_slug"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	headless := true
	if req.Headless != nil {
		headless = *req.Headless
	}

	session, err := h.browserMgr.CreateSession(req.Name, headless, req.OwnerAgentSlug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "browser_session_created", "browser", "browser_session", session.ID, session.Name)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":               session.ID,
		"name":             session.Name,
		"status":           string(session.Status),
		"headless":         session.Headless,
		"current_url":      session.CurrentURL,
		"current_title":    session.CurrentTitle,
		"owner_agent_slug": session.OwnerAgentSlug,
		"created_at":       session.CreatedAt.Format(time.RFC3339),
		"updated_at":       session.UpdatedAt.Format(time.RFC3339),
	})
}

func (h *BrowserHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	session, err := h.browserMgr.GetSession(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":               session.ID,
		"name":             session.Name,
		"status":           string(session.Status),
		"headless":         session.Headless,
		"current_url":      session.CurrentURL,
		"current_title":    session.CurrentTitle,
		"owner_agent_slug": session.OwnerAgentSlug,
		"created_at":       session.CreatedAt.Format(time.RFC3339),
		"updated_at":       session.UpdatedAt.Format(time.RFC3339),
	})
}

func (h *BrowserHandler) UpdateSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	session, err := h.browserMgr.GetSession(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		Name           string `json:"name"`
		OwnerAgentSlug string `json:"owner_agent_slug"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()
	if req.Name != "" {
		session.Name = req.Name
	}
	if req.OwnerAgentSlug != "" {
		session.OwnerAgentSlug = req.OwnerAgentSlug
	}

	h.db.Exec(
		"UPDATE browser_sessions SET name = ?, owner_agent_slug = ?, updated_at = ? WHERE id = ?",
		session.Name, session.OwnerAgentSlug, now, id,
	)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":               session.ID,
		"name":             session.Name,
		"status":           string(session.Status),
		"owner_agent_slug": session.OwnerAgentSlug,
		"updated_at":       now.Format(time.RFC3339),
	})
}

func (h *BrowserHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.browserMgr.DeleteSession(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "browser_session_deleted", "browser", "browser_session", id, "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *BrowserHandler) StartSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.browserMgr.StartSession(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "browser_session_started", "browser", "browser_session", id, "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (h *BrowserHandler) StopSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.browserMgr.StopSession(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "browser_session_stopped", "browser", "browser_session", id, "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (h *BrowserHandler) ExecuteAction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req browser.ActionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SessionID = id

	result := h.browserMgr.ExecuteAction(r.Context(), req)

	if !result.Success {
		writeJSON(w, http.StatusOK, result)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *BrowserHandler) GetScreenshot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	data, err := h.browserMgr.GetLastScreenshot(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if data == nil {
		writeError(w, http.StatusNotFound, "no screenshot available")
		return
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	writeJSON(w, http.StatusOK, map[string]string{
		"image": encoded,
	})
}

func (h *BrowserHandler) TakeControl(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.browserMgr.TakeHumanControl(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "browser_human_control", "browser", "browser_session", id, "took control")

	writeJSON(w, http.StatusOK, map[string]string{"status": "human"})
}

func (h *BrowserHandler) ReleaseControl(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.browserMgr.ReleaseHumanControl(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "browser_release_control", "browser", "browser_session", id, "released control")

	writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

func (h *BrowserHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	rows, err := h.db.Query(
		"SELECT id, session_id, thread_id, agent_role_slug, title, status, instructions, result, extracted_data, error, started_at, completed_at, created_at FROM browser_tasks WHERE session_id = ? ORDER BY created_at DESC LIMIT 50",
		id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	defer rows.Close()

	tasks := []browserTaskResponse{}
	for rows.Next() {
		var t browserTaskResponse
		var startedAt, completedAt *time.Time
		if err := rows.Scan(&t.ID, &t.SessionID, &t.ThreadID, &t.AgentRoleSlug, &t.Title, &t.Status, &t.Instructions, &t.Result, &t.ExtractedData, &t.Error, &startedAt, &completedAt, &t.CreatedAt); err != nil {
			logger.Error("Failed to scan browser task: %v", err)
			continue
		}
		if startedAt != nil {
			s := startedAt.Format(time.RFC3339)
			t.StartedAt = &s
		}
		if completedAt != nil {
			s := completedAt.Format(time.RFC3339)
			t.CompletedAt = &s
		}
		tasks = append(tasks, t)
	}

	writeJSON(w, http.StatusOK, tasks)
}

func (h *BrowserHandler) ListAllTasks(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		"SELECT id, session_id, thread_id, agent_role_slug, title, status, error, started_at, completed_at, created_at FROM browser_tasks ORDER BY created_at DESC LIMIT 50",
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	defer rows.Close()

	tasks := []browserTaskResponse{}
	for rows.Next() {
		var t browserTaskResponse
		var startedAt, completedAt *time.Time
		if err := rows.Scan(&t.ID, &t.SessionID, &t.ThreadID, &t.AgentRoleSlug, &t.Title, &t.Status, &t.Error, &startedAt, &completedAt, &t.CreatedAt); err != nil {
			logger.Error("Failed to scan browser task summary: %v", err)
			continue
		}
		if startedAt != nil {
			s := startedAt.Format(time.RFC3339)
			t.StartedAt = &s
		}
		if completedAt != nil {
			s := completedAt.Format(time.RFC3339)
			t.CompletedAt = &s
		}
		tasks = append(tasks, t)
	}

	writeJSON(w, http.StatusOK, tasks)
}

func (h *BrowserHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "taskId")

	var t browserTaskResponse

	var startedAt, completedAt *time.Time
	err := h.db.QueryRow(
		"SELECT id, session_id, thread_id, agent_role_slug, title, status, instructions, result, extracted_data, error, started_at, completed_at, created_at FROM browser_tasks WHERE id = ?",
		id,
	).Scan(&t.ID, &t.SessionID, &t.ThreadID, &t.AgentRoleSlug, &t.Title, &t.Status, &t.Instructions, &t.Result, &t.ExtractedData, &t.Error, &startedAt, &completedAt, &t.CreatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if startedAt != nil {
		s := startedAt.Format(time.RFC3339)
		t.StartedAt = &s
	}
	if completedAt != nil {
		s := completedAt.Format(time.RFC3339)
		t.CompletedAt = &s
	}

	writeJSON(w, http.StatusOK, t)
}

func (h *BrowserHandler) GetTaskActions(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskId")

	rows, err := h.db.Query(
		"SELECT id, session_id, action, selector, value, success, error, url_before, url_after, created_at FROM browser_action_log WHERE task_id = ? ORDER BY created_at ASC",
		taskID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list actions")
		return
	}
	defer rows.Close()

	type actionEntry struct {
		ID        string `json:"id"`
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
		Selector  string `json:"selector"`
		Value     string `json:"value"`
		Success   bool   `json:"success"`
		Error     string `json:"error"`
		URLBefore string `json:"url_before"`
		URLAfter  string `json:"url_after"`
		CreatedAt string `json:"created_at"`
	}

	actions := []actionEntry{}
	for rows.Next() {
		var a actionEntry
		var success int
		if err := rows.Scan(&a.ID, &a.SessionID, &a.Action, &a.Selector, &a.Value, &success, &a.Error, &a.URLBefore, &a.URLAfter, &a.CreatedAt); err != nil {
			logger.Error("Failed to scan browser action: %v", err)
			continue
		}
		a.Success = success == 1
		actions = append(actions, a)
	}

	writeJSON(w, http.StatusOK, actions)
}
