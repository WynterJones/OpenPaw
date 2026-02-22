package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/models"
)

type LogsHandler struct {
	db *database.DB
}

func NewLogsHandler(db *database.DB) *LogsHandler {
	return &LogsHandler{db: db}
}

func (h *LogsHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := 100
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	var conditions []string
	args := []interface{}{}

	if action := r.URL.Query().Get("action"); action != "" {
		conditions = append(conditions, "a.action = ?")
		args = append(args, action)
	}
	if category := r.URL.Query().Get("category"); category != "" {
		conditions = append(conditions, "a.category = ?")
		args = append(args, category)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := `SELECT COUNT(*) FROM audit_logs a ` + where
	var total int
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count logs")
		return
	}

	query := `SELECT a.id, a.user_id, COALESCE(u.username, a.user_id), a.action, a.category, a.target, a.target_id, a.details, a.created_at
		FROM audit_logs a LEFT JOIN users u ON a.user_id = u.id
		` + where + ` ORDER BY a.created_at DESC LIMIT ? OFFSET ?`
	queryArgs := append(args, limit, offset)

	rows, err := h.db.Query(query, queryArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list logs")
		return
	}
	defer rows.Close()

	logs := []models.AuditLog{}
	for rows.Next() {
		var l models.AuditLog
		var username sql.NullString
		if err := rows.Scan(&l.ID, &l.UserID, &username, &l.Action, &l.Category, &l.Target, &l.TargetID, &l.Details, &l.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan log")
			return
		}
		if username.Valid {
			l.Username = username.String
		} else {
			l.Username = l.UserID
		}
		logs = append(logs, l)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs":  logs,
		"total": total,
	})
}

func (h *LogsHandler) Stats(w http.ResponseWriter, r *http.Request) {
	// Backfill live counters if they are zero but chat_messages exist
	h.backfillLiveCounters()

	var archivedCost, archivedInTok, archivedOutTok, archivedActivity float64
	h.db.QueryRow("SELECT COALESCE(value,0) FROM system_stats WHERE key='archived_cost_usd'").Scan(&archivedCost)
	h.db.QueryRow("SELECT COALESCE(value,0) FROM system_stats WHERE key='archived_input_tokens'").Scan(&archivedInTok)
	h.db.QueryRow("SELECT COALESCE(value,0) FROM system_stats WHERE key='archived_output_tokens'").Scan(&archivedOutTok)
	h.db.QueryRow("SELECT COALESCE(value,0) FROM system_stats WHERE key='archived_activity_count'").Scan(&archivedActivity)

	var liveCost, liveInTok, liveOutTok float64
	h.db.QueryRow("SELECT COALESCE(value,0) FROM system_stats WHERE key='live_cost_usd'").Scan(&liveCost)
	h.db.QueryRow("SELECT COALESCE(value,0) FROM system_stats WHERE key='live_input_tokens'").Scan(&liveInTok)
	h.db.QueryRow("SELECT COALESCE(value,0) FROM system_stats WHERE key='live_output_tokens'").Scan(&liveOutTok)

	var liveActivity int
	h.db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&liveActivity)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_cost_usd": archivedCost + liveCost,
		"total_tokens":   int(archivedInTok) + int(archivedOutTok) + int(liveInTok) + int(liveOutTok),
		"total_activity": int(archivedActivity) + liveActivity,
	})
}

// backfillLiveCounters does a one-time scan of chat_messages to populate live counters
// when the counters are zero but messages with costs exist (upgrade from pre-counter schema).
func (h *LogsHandler) backfillLiveCounters() {
	var liveCost float64
	h.db.QueryRow("SELECT COALESCE(value,0) FROM system_stats WHERE key='live_cost_usd'").Scan(&liveCost)
	if liveCost > 0 {
		return // already populated
	}

	var scanCost float64
	var scanIn, scanOut int
	h.db.QueryRow("SELECT COALESCE(SUM(cost_usd),0), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0) FROM chat_messages").Scan(&scanCost, &scanIn, &scanOut)
	if scanCost == 0 && scanIn == 0 && scanOut == 0 {
		return // no data to backfill
	}

	h.db.Exec("UPDATE system_stats SET value = ? WHERE key = 'live_cost_usd'", scanCost)
	h.db.Exec("UPDATE system_stats SET value = ? WHERE key = 'live_input_tokens'", float64(scanIn))
	h.db.Exec("UPDATE system_stats SET value = ? WHERE key = 'live_output_tokens'", float64(scanOut))
}

func (h *LogsHandler) ToolLogs(w http.ResponseWriter, r *http.Request) {
	toolID := chi.URLParam(r, "id")

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	rows, err := h.db.Query(
		`SELECT a.id, a.user_id, COALESCE(u.username, a.user_id), a.action, a.category, a.target, a.target_id, a.details, a.created_at
			FROM audit_logs a LEFT JOIN users u ON a.user_id = u.id
			WHERE a.target = 'tool' AND a.target_id = ? ORDER BY a.created_at DESC LIMIT ?`,
		toolID, limit,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tool logs")
		return
	}
	defer rows.Close()

	logs := []models.AuditLog{}
	for rows.Next() {
		var l models.AuditLog
		var username sql.NullString
		if err := rows.Scan(&l.ID, &l.UserID, &username, &l.Action, &l.Category, &l.Target, &l.TargetID, &l.Details, &l.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan log")
			return
		}
		if username.Valid {
			l.Username = username.String
		} else {
			l.Username = l.UserID
		}
		logs = append(logs, l)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"logs": logs})
}
