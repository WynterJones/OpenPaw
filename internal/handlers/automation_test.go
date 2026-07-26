package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openpaw/openpaw/internal/database"
)

func newAutomationHandler(t *testing.T) *AutomationHandler {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &AutomationHandler{db: db}
}

func activeAutomation(t *testing.T, h *AutomationHandler) []runningAutomation {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Active(rec, httptest.NewRequest(http.MethodGet, "/api/automation/active", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out []runningAutomation
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v (body %s)", err, rec.Body.String())
	}
	return out
}

func TestActiveAutomationEmpty(t *testing.T) {
	h := newAutomationHandler(t)

	got := activeAutomation(t, h)
	if len(got) != 0 {
		t.Fatalf("got %d runs, want 0", len(got))
	}
	// An empty result must serialize as [] so the UI's Array.isArray guard holds.
	rec := httptest.NewRecorder()
	h.Active(rec, httptest.NewRequest(http.MethodGet, "/api/automation/active", nil))
	if body := rec.Body.String(); body != "[]\n" && body != "[]" {
		t.Fatalf("empty body = %q, want []", body)
	}
}

func TestActiveAutomationReportsRunningWork(t *testing.T) {
	h := newAutomationHandler(t)
	now := time.Now().UTC()

	if _, err := h.db.Exec(
		"INSERT INTO agent_roles (id, slug, name, system_prompt) VALUES (?, ?, ?, ?)",
		"role-1", "researcher", "Research Assistant", "",
	); err != nil {
		t.Fatalf("insert agent role: %v", err)
	}
	if _, err := h.db.Exec(
		"INSERT INTO schedules (id, name, cron_expr, type, agent_role_slug, prompt_content) VALUES (?, ?, ?, 'prompt', ?, ?)",
		"sched-1", "Morning digest", "0 0 9 * * *", "researcher", "go",
	); err != nil {
		t.Fatalf("insert schedule: %v", err)
	}
	h.db.Exec(
		"INSERT INTO schedule_executions (id, schedule_id, status, started_at) VALUES (?, ?, 'running', ?)",
		"exec-run", "sched-1", now,
	)
	h.db.Exec(
		"INSERT INTO schedule_executions (id, schedule_id, status, started_at) VALUES (?, ?, 'success', ?)",
		"exec-done", "sched-1", now,
	)
	h.db.Exec(
		"INSERT INTO heartbeat_executions (id, agent_role_slug, status, started_at) VALUES (?, ?, 'running', ?)",
		"hb-run", "researcher", now,
	)

	got := activeAutomation(t, h)
	if len(got) != 2 {
		t.Fatalf("got %d runs, want 2: %+v", len(got), got)
	}

	sched, hb := got[0], got[1]
	if sched.Kind != "schedule" || sched.ID != "exec-run" {
		t.Errorf("schedule entry = %+v", sched)
	}
	if sched.Label != "Morning digest" || sched.Detail != "Research Assistant" {
		t.Errorf("schedule label/detail = %q/%q", sched.Label, sched.Detail)
	}
	if hb.Kind != "heartbeat" || hb.ID != "hb-run" || hb.Label != "Research Assistant" {
		t.Errorf("heartbeat entry = %+v", hb)
	}
}

// A row abandoned while the process stayed up must not pin the indicator on
// forever — boot-time reaping can't help there, so the cutoff has to.
func TestActiveAutomationIgnoresStaleRuns(t *testing.T) {
	h := newAutomationHandler(t)
	stale := time.Now().UTC().Add(-staleRunAfter - time.Minute)

	h.db.Exec(
		"INSERT INTO heartbeat_executions (id, agent_role_slug, status, started_at) VALUES (?, ?, 'running', ?)",
		"hb-stale", "ghost", stale,
	)

	if got := activeAutomation(t, h); len(got) != 0 {
		t.Fatalf("got %d runs, want 0: %+v", len(got), got)
	}
}
