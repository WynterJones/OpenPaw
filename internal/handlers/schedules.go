package handlers

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
	"github.com/openpaw/openpaw/internal/scheduler"
)

type SchedulesHandler struct {
	db        *database.DB
	scheduler *scheduler.Scheduler
}

func scheduleToConfig(id string, s models.Schedule) scheduler.ScheduleConfig {
	workspaceID := ""
	if s.WorkspaceID != nil {
		workspaceID = *s.WorkspaceID
	}
	return scheduler.ScheduleConfig{
		ID:            id,
		CronExpr:      s.CronExpr,
		AgentRoleSlug: s.AgentRoleSlug,
		PromptContent: s.PromptContent,
		ThreadID:      s.ThreadID,
		WorkspaceID:   workspaceID,
		Provider:      s.Provider,
	}
}

func NewSchedulesHandler(db *database.DB, sched *scheduler.Scheduler) *SchedulesHandler {
	return &SchedulesHandler{db: db, scheduler: sched}
}

func (h *SchedulesHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		`SELECT id, name, description, cron_expr, tool_id, action, payload, enabled,
		        type, agent_role_slug, prompt_content, thread_id, dashboard_id, widget_id,
		        workspace_id, provider, last_run_at, next_run_at, created_at, updated_at
		 FROM schedules ORDER BY created_at DESC`,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list schedules")
		return
	}
	defer rows.Close()

	schedules := []models.Schedule{}
	for rows.Next() {
		var s models.Schedule
		var workspaceID sql.NullString
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.CronExpr, &s.ToolID, &s.Action, &s.Payload, &s.Enabled,
			&s.Type, &s.AgentRoleSlug, &s.PromptContent, &s.ThreadID, &s.DashboardID, &s.WidgetID,
			&workspaceID, &s.Provider, &s.LastRunAt, &s.NextRunAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan schedule")
			return
		}
		if workspaceID.Valid && workspaceID.String != "" {
			ws := workspaceID.String
			s.WorkspaceID = &ws
		}
		schedules = append(schedules, s)
	}
	writeJSON(w, http.StatusOK, schedules)
}

func (h *SchedulesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		CronExpr      string `json:"cron_expr"`
		AgentRoleSlug string `json:"agent_role_slug"`
		PromptContent string `json:"prompt_content"`
		ThreadID      string `json:"thread_id"`
		WorkspaceID   string `json:"workspace_id"`
		Provider      string `json:"provider"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.CronExpr == "" {
		writeError(w, http.StatusBadRequest, "name and cron_expr are required")
		return
	}
	if req.AgentRoleSlug == "" || req.PromptContent == "" {
		writeError(w, http.StatusBadRequest, "agent_role_slug and prompt_content are required")
		return
	}

	var agentExists string
	if err := h.db.QueryRow("SELECT slug FROM agent_roles WHERE slug = ? AND enabled = 1", req.AgentRoleSlug).Scan(&agentExists); err != nil {
		writeError(w, http.StatusBadRequest, "agent role not found or disabled")
		return
	}

	// Validate thread_id if provided
	if req.ThreadID != "" {
		var threadExists string
		if err := h.db.QueryRow("SELECT id FROM chat_threads WHERE id = ?", req.ThreadID).Scan(&threadExists); err != nil {
			writeError(w, http.StatusBadRequest, "chat thread not found")
			return
		}
	}

	id := generateID()
	now := time.Now().UTC()

	// nil = global (nullable column); a set value scopes the schedule's runs.
	var workspacePtr *string
	if req.WorkspaceID != "" {
		workspacePtr = &req.WorkspaceID
	}

	_, err := h.db.Exec(
		`INSERT INTO schedules (id, name, description, cron_expr, tool_id, action, payload, enabled,
		                        type, agent_role_slug, prompt_content, thread_id, workspace_id, provider, created_at, updated_at)
		 VALUES (?, ?, ?, ?, '', '', '{}', ?, 'prompt', ?, ?, ?, ?, ?, ?, ?)`,
		id, req.Name, req.Description, req.CronExpr, true,
		req.AgentRoleSlug, req.PromptContent, req.ThreadID, workspacePtr, req.Provider, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create schedule")
		return
	}

	h.scheduler.AddSchedule(scheduler.ScheduleConfig{
		ID:            id,
		CronExpr:      req.CronExpr,
		AgentRoleSlug: req.AgentRoleSlug,
		PromptContent: req.PromptContent,
		ThreadID:      req.ThreadID,
		WorkspaceID:   req.WorkspaceID,
		Provider:      req.Provider,
	})

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "schedule_created", "schedule", "schedule", id, req.Name)

	writeJSON(w, http.StatusCreated, models.Schedule{
		ID:            id,
		Name:          req.Name,
		Description:   req.Description,
		CronExpr:      req.CronExpr,
		Type:          "prompt",
		AgentRoleSlug: req.AgentRoleSlug,
		PromptContent: req.PromptContent,
		ThreadID:      req.ThreadID,
		WorkspaceID:   workspacePtr,
		Provider:      req.Provider,
		Enabled:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
}

func (h *SchedulesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var exists string
	err := h.db.QueryRow("SELECT id FROM schedules WHERE id = ?", id).Scan(&exists)
	if err != nil {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}

	var req struct {
		Name          *string `json:"name"`
		Description   *string `json:"description"`
		CronExpr      *string `json:"cron_expr"`
		PromptContent *string `json:"prompt_content"`
		AgentRoleSlug *string `json:"agent_role_slug"`
		ThreadID      *string `json:"thread_id"`
		WorkspaceID   *string `json:"workspace_id"`
		Provider      *string `json:"provider"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()
	setClauses := []string{"updated_at = ?"}
	args := []interface{}{now}

	if req.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		setClauses = append(setClauses, "description = ?")
		args = append(args, *req.Description)
	}
	if req.CronExpr != nil {
		setClauses = append(setClauses, "cron_expr = ?")
		args = append(args, *req.CronExpr)
	}
	if req.PromptContent != nil {
		setClauses = append(setClauses, "prompt_content = ?")
		args = append(args, *req.PromptContent)
	}
	if req.AgentRoleSlug != nil {
		setClauses = append(setClauses, "agent_role_slug = ?")
		args = append(args, *req.AgentRoleSlug)
	}
	if req.ThreadID != nil {
		setClauses = append(setClauses, "thread_id = ?")
		args = append(args, *req.ThreadID)
	}
	if req.Provider != nil {
		setClauses = append(setClauses, "provider = ?")
		args = append(args, *req.Provider)
	}
	if req.WorkspaceID != nil {
		// Empty string clears the target (nullable = global).
		if *req.WorkspaceID == "" {
			setClauses = append(setClauses, "workspace_id = NULL")
		} else {
			setClauses = append(setClauses, "workspace_id = ?")
			args = append(args, *req.WorkspaceID)
		}
	}

	args = append(args, id)
	query := "UPDATE schedules SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	if _, err := h.db.Exec(query, args...); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update schedule")
		return
	}

	// Reload the live cron entry whenever any field it carries changed.
	//
	// This used to reload only on cron_expr or workspace_id, but the entry also
	// holds the agent, prompt and target thread — so editing the prompt saved to
	// the database and then kept running the old prompt until the next restart,
	// which looks exactly like the edit not saving at all.
	if req.CronExpr != nil || req.WorkspaceID != nil || req.AgentRoleSlug != nil ||
		req.PromptContent != nil || req.ThreadID != nil || req.Provider != nil {
		var s models.Schedule
		var workspaceID sql.NullString
		h.db.QueryRow(
			"SELECT cron_expr, enabled, agent_role_slug, prompt_content, thread_id, workspace_id, provider FROM schedules WHERE id = ?", id,
		).Scan(&s.CronExpr, &s.Enabled, &s.AgentRoleSlug, &s.PromptContent, &s.ThreadID, &workspaceID, &s.Provider)
		if workspaceID.Valid && workspaceID.String != "" {
			ws := workspaceID.String
			s.WorkspaceID = &ws
		}
		if s.Enabled {
			h.scheduler.RemoveSchedule(id)
			h.scheduler.AddSchedule(scheduleToConfig(id, s))
		}
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "schedule_updated", "schedule", "schedule", id, "")

	var s models.Schedule
	var workspaceID sql.NullString
	h.db.QueryRow(
		`SELECT id, name, description, cron_expr, tool_id, action, payload, enabled,
		        type, agent_role_slug, prompt_content, thread_id, dashboard_id, widget_id,
		        workspace_id, provider, last_run_at, next_run_at, created_at, updated_at
		 FROM schedules WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &s.Description, &s.CronExpr, &s.ToolID, &s.Action, &s.Payload, &s.Enabled,
		&s.Type, &s.AgentRoleSlug, &s.PromptContent, &s.ThreadID, &s.DashboardID, &s.WidgetID,
		&workspaceID, &s.Provider, &s.LastRunAt, &s.NextRunAt, &s.CreatedAt, &s.UpdatedAt)
	if workspaceID.Valid && workspaceID.String != "" {
		ws := workspaceID.String
		s.WorkspaceID = &ws
	}

	writeJSON(w, http.StatusOK, s)
}

func (h *SchedulesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := h.db.Exec("DELETE FROM schedules WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete schedule")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}

	h.scheduler.RemoveSchedule(id)

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "schedule_deleted", "schedule", "schedule", id, "")

	writeJSON(w, http.StatusOK, map[string]string{"message": "schedule deleted"})
}

func (h *SchedulesHandler) RunNow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var s models.Schedule
	var workspaceID sql.NullString
	err := h.db.QueryRow(
		`SELECT id, agent_role_slug, prompt_content, thread_id, workspace_id
		 FROM schedules WHERE id = ?`, id,
	).Scan(&s.ID, &s.AgentRoleSlug, &s.PromptContent, &s.ThreadID, &workspaceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if workspaceID.Valid && workspaceID.String != "" {
		ws := workspaceID.String
		s.WorkspaceID = &ws
	}

	h.scheduler.RunNow(scheduleToConfig(id, s))

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "schedule_run_now", "schedule", "schedule", id, "")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":     "schedule triggered",
		"schedule_id": id,
	})
}

func (h *SchedulesHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var enabled bool
	err := h.db.QueryRow("SELECT enabled FROM schedules WHERE id = ?", id).Scan(&enabled)
	if err != nil {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}

	newEnabled := !enabled
	now := time.Now().UTC()
	if _, err := h.db.Exec("UPDATE schedules SET enabled = ?, updated_at = ? WHERE id = ?", newEnabled, now, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to toggle schedule")
		return
	}

	if newEnabled {
		var s models.Schedule
		var workspaceID sql.NullString
		h.db.QueryRow(
			"SELECT cron_expr, agent_role_slug, prompt_content, thread_id, workspace_id FROM schedules WHERE id = ?", id,
		).Scan(&s.CronExpr, &s.AgentRoleSlug, &s.PromptContent, &s.ThreadID, &workspaceID)
		if workspaceID.Valid && workspaceID.String != "" {
			ws := workspaceID.String
			s.WorkspaceID = &ws
		}
		h.scheduler.AddSchedule(scheduleToConfig(id, s))
	} else {
		h.scheduler.RemoveSchedule(id)
	}

	userID := middleware.GetUserID(r.Context())
	action := "schedule_enabled"
	if !newEnabled {
		action = "schedule_disabled"
	}
	h.db.LogAudit(userID, action, "schedule", "schedule", id, "")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "schedule toggled",
		"enabled": newEnabled,
	})
}

func (h *SchedulesHandler) Executions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	rows, err := h.db.Query(
		`SELECT id, schedule_id, status, output, error, started_at, finished_at
		 FROM schedule_executions WHERE schedule_id = ? ORDER BY started_at DESC LIMIT 50`, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list executions")
		return
	}
	defer rows.Close()

	executions := []models.ScheduleExecution{}
	for rows.Next() {
		var e models.ScheduleExecution
		if err := rows.Scan(&e.ID, &e.ScheduleID, &e.Status, &e.Output, &e.Error, &e.StartedAt, &e.FinishedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan execution")
			return
		}
		executions = append(executions, e)
	}
	writeJSON(w, http.StatusOK, executions)
}
