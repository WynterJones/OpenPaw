package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
)

// DreamingManager is the interface for the dreaming manager (avoids circular
// imports, matching how the heartbeat manager is wired).
type DreamingManager interface {
	GetConfig() map[string]string
	UpdateConfig(cfg map[string]string) error
	RunNow()
	IsDreaming() bool
}

type DreamingHandler struct {
	db  *database.DB
	mgr DreamingManager
}

func NewDreamingHandler(db *database.DB, mgr DreamingManager) *DreamingHandler {
	return &DreamingHandler{db: db, mgr: mgr}
}

// available reports whether a dreaming manager was wired in. Dreaming is not
// optional in the shipped binary, but the handler is constructed unconditionally
// — answering 503 beats panicking the request if that ever stops being true.
func (h *DreamingHandler) available(w http.ResponseWriter) bool {
	if h.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "Dreaming is not available")
		return false
	}
	return true
}

func (h *DreamingHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	writeJSON(w, http.StatusOK, h.mgr.GetConfig())
}

func (h *DreamingHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	var req map[string]string
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.mgr.UpdateConfig(req); err != nil {
		// A rejected cron expression is the user's typo, not a server fault —
		// answering 500 would send them looking for a broken install.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "dreaming_config_updated", "dreaming", "settings", "dreaming", "")

	writeJSON(w, http.StatusOK, h.mgr.GetConfig())
}

func (h *DreamingHandler) RunNow(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	if h.mgr.IsDreaming() {
		writeError(w, http.StatusConflict, "A dream is already in progress")
		return
	}

	h.mgr.RunNow()

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "dreaming_run_now", "dreaming", "settings", "dreaming", "")

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// ListRuns returns the dream history, newest first.
func (h *DreamingHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	where, args := "1=1", []interface{}{}
	if agent := r.URL.Query().Get("agent"); agent != "" {
		where += " AND agent_slug = ?"
		args = append(args, agent)
	}
	args = append(args, limit)

	rows, err := h.db.Query(
		`SELECT r.id, r.agent_slug, COALESCE(a.name, r.agent_slug), r.status,
		        r.threads_scanned, r.facts_found, r.memories_added, r.memories_updated,
		        r.memories_pruned, r.summary, r.error, r.started_at, r.finished_at
		 FROM dream_runs r
		 LEFT JOIN agent_roles a ON a.slug = r.agent_slug
		 WHERE `+where+`
		 ORDER BY r.started_at DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list dream runs")
		return
	}
	defer rows.Close()

	runs := []models.DreamRun{}
	for rows.Next() {
		var run models.DreamRun
		var finishedAt sql.NullTime
		if err := rows.Scan(&run.ID, &run.AgentRoleSlug, &run.AgentName, &run.Status,
			&run.ThreadsScanned, &run.FactsFound, &run.MemoriesAdded, &run.MemoriesUpdated,
			&run.MemoriesPruned, &run.Summary, &run.Error, &run.StartedAt, &finishedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan dream run")
			return
		}
		if finishedAt.Valid {
			t := finishedAt.Time
			run.FinishedAt = &t
		}
		runs = append(runs, run)
	}

	writeJSON(w, http.StatusOK, runs)
}
