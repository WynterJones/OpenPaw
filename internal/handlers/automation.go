package handlers

import (
	"net/http"
	"time"

	"github.com/openpaw/openpaw/internal/database"
)

// AutomationHandler exposes what the background automation (scheduled routines
// and agent heartbeats) is doing right now, so the UI can show that the system
// is awake and working even when the user isn't in a chat or terminal.
type AutomationHandler struct {
	db *database.DB
}

func NewAutomationHandler(db *database.DB) *AutomationHandler {
	return &AutomationHandler{db: db}
}

// staleRunAfter bounds how long a row may claim to be running before we stop
// believing it. Interrupted rows are normally reaped at boot; this is the
// backstop for a row abandoned while the process stays up, so that one bad run
// can't leave the indicator stuck on forever. Kept well clear of the default
// 60-minute agent timeout so a genuinely long run still shows.
const staleRunAfter = 3 * time.Hour

type runningAutomation struct {
	Kind      string `json:"kind"` // "schedule" | "heartbeat"
	ID        string `json:"id"`   // execution id
	Label     string `json:"label"`
	Detail    string `json:"detail,omitempty"`
	StartedAt string `json:"started_at"`
}

// Active lists every schedule execution and heartbeat cycle currently in
// flight. Read-only and identity-free by design: it reports that work is
// happening, not how to reach it.
func (h *AutomationHandler) Active(w http.ResponseWriter, r *http.Request) {
	cutoff := time.Now().UTC().Add(-staleRunAfter)
	out := []runningAutomation{}

	if rows, err := h.db.Query(
		`SELECT se.id, COALESCE(s.name, ''), COALESCE(ar.name, s.agent_role_slug, ''), se.started_at
		 FROM schedule_executions se
		 LEFT JOIN schedules s ON s.id = se.schedule_id
		 LEFT JOIN agent_roles ar ON ar.slug = s.agent_role_slug
		 WHERE se.status = 'running' AND se.started_at >= ?
		 ORDER BY se.started_at ASC`,
		cutoff,
	); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, name, agent string
			var startedAt time.Time
			if rows.Scan(&id, &name, &agent, &startedAt) != nil {
				continue
			}
			if name == "" {
				name = "Scheduled routine"
			}
			out = append(out, runningAutomation{
				Kind:      "schedule",
				ID:        id,
				Label:     name,
				Detail:    agent,
				StartedAt: startedAt.UTC().Format(time.RFC3339),
			})
		}
	}

	if rows, err := h.db.Query(
		`SELECT he.id, COALESCE(ar.name, he.agent_role_slug), he.started_at
		 FROM heartbeat_executions he
		 LEFT JOIN agent_roles ar ON ar.slug = he.agent_role_slug
		 WHERE he.status = 'running' AND he.started_at >= ?
		 ORDER BY he.started_at ASC`,
		cutoff,
	); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, agent string
			var startedAt time.Time
			if rows.Scan(&id, &agent, &startedAt) != nil {
				continue
			}
			if agent == "" {
				agent = "Agent"
			}
			out = append(out, runningAutomation{
				Kind:      "heartbeat",
				ID:        id,
				Label:     agent,
				Detail:    "heartbeat",
				StartedAt: startedAt.UTC().Format(time.RFC3339),
			})
		}
	}

	writeJSON(w, http.StatusOK, out)
}
