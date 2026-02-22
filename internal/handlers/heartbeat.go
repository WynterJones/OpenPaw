package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
)

// HeartbeatManager is the interface for the heartbeat manager (avoids circular imports).
type HeartbeatManager interface {
	GetConfig() map[string]string
	UpdateConfig(cfg map[string]string) error
	RunNow()
	IsRunning() bool
}

type HeartbeatHandler struct {
	db  *database.DB
	mgr HeartbeatManager
}

func NewHeartbeatHandler(db *database.DB, mgr HeartbeatManager) *HeartbeatHandler {
	return &HeartbeatHandler{db: db, mgr: mgr}
}

func (h *HeartbeatHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.mgr.GetConfig()
	writeJSON(w, http.StatusOK, cfg)
}

func (h *HeartbeatHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.mgr.UpdateConfig(req); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update heartbeat config: "+err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "heartbeat_config_updated", "heartbeat", "settings", "heartbeat", "")

	writeJSON(w, http.StatusOK, h.mgr.GetConfig())
}

func (h *HeartbeatHandler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	limit := 20
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	where := "1=1"
	var args []interface{}

	if q := r.URL.Query().Get("q"); q != "" {
		where += " AND (agent_role_slug LIKE ? ESCAPE '\\' OR actions_taken LIKE ? ESCAPE '\\' OR output LIKE ? ESCAPE '\\' OR error LIKE ? ESCAPE '\\')"
		like := "%" + escapeLike(q) + "%"
		args = append(args, like, like, like, like)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	if agent := r.URL.Query().Get("agent"); agent != "" {
		where += " AND agent_role_slug = ?"
		args = append(args, agent)
	}

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM heartbeat_executions WHERE %s", where)
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count executions")
		return
	}

	query := fmt.Sprintf(
		`SELECT id, agent_role_slug, status, actions_taken, output, error, cost_usd, input_tokens, output_tokens, started_at, finished_at
		 FROM heartbeat_executions WHERE %s ORDER BY started_at DESC LIMIT ? OFFSET ?`, where)
	queryArgs := append(args, limit, offset)

	rows, err := h.db.Query(query, queryArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list heartbeat executions")
		return
	}
	defer rows.Close()

	executions := []models.HeartbeatExecution{}
	for rows.Next() {
		var e models.HeartbeatExecution
		if err := rows.Scan(&e.ID, &e.AgentRoleSlug, &e.Status, &e.ActionsTaken, &e.Output, &e.Error,
			&e.CostUSD, &e.InputTokens, &e.OutputTokens, &e.StartedAt, &e.FinishedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan execution")
			return
		}
		executions = append(executions, e)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": executions,
		"total": total,
	})
}

func (h *HeartbeatHandler) RunNow(w http.ResponseWriter, r *http.Request) {
	if h.mgr.IsRunning() {
		writeError(w, http.StatusConflict, "heartbeat cycle already running")
		return
	}

	go h.mgr.RunNow()

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "heartbeat_run_now", "heartbeat", "heartbeat", "manual", "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "triggered"})
}
