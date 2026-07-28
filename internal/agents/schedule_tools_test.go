package agents

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/scheduler"
)

// fakeScheduler records what would have been registered with cron, so the tools
// can be tested without standing up a real scheduler.
type fakeScheduler struct {
	added   []scheduler.ScheduleConfig
	removed []string
	ran     []string
}

func (f *fakeScheduler) AddSchedule(cfg scheduler.ScheduleConfig) { f.added = append(f.added, cfg) }
func (f *fakeScheduler) RemoveSchedule(id string)                 { f.removed = append(f.removed, id) }
func (f *fakeScheduler) RunNow(cfg scheduler.ScheduleConfig)      { f.ran = append(f.ran, cfg.ID) }

func newScheduleTestManager(t *testing.T) (*Manager, *database.DB, *fakeScheduler) {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(
		"INSERT INTO agent_roles (id, slug, name, description, system_prompt, enabled) VALUES ('a1', 'scout', 'Scout', '', '', 1)",
	); err != nil {
		t.Fatalf("insert agent role: %v", err)
	}

	sched := &fakeScheduler{}
	m := &Manager{db: db, Scheduler: sched, broadcast: func(string, interface{}) {}}
	return m, db, sched
}

func invoke(t *testing.T, m *Manager, name string, args map[string]interface{}, canModify bool) (string, bool) {
	t.Helper()
	handlers := m.MakeScheduleToolHandlers("scout", "thread-1", canModify)
	h, ok := handlers[name]
	if !ok {
		t.Fatalf("tool %q is not available (canModify=%v)", name, canModify)
	}
	raw, _ := json.Marshal(args)
	res := h(context.Background(), "", raw)
	return res.Output, res.IsError
}

// The whole point of the tool is that a routine set up in conversation actually
// runs — which means both a database row and a live cron entry.
func TestScheduleCreateRegistersWithCron(t *testing.T) {
	m, db, sched := newScheduleTestManager(t)

	out, isErr := invoke(t, m, "schedule_create", map[string]interface{}{
		"name":      "Morning sweep",
		"cron_expr": "30 8 * * 1-5",
		"prompt":    "Summarise anything new in the repo since yesterday.",
	}, true)
	if isErr {
		t.Fatalf("schedule_create failed: %s", out)
	}

	var result struct {
		ScheduleID string `json:"schedule_id"`
		CronExpr   string `json:"cron_expr"`
		AgentSlug  string `json:"agent_slug"`
		Delivery   string `json:"delivery"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode result: %v — %s", err, out)
	}

	// Five-field input must be stored in the six-field form the cron runs.
	if result.CronExpr != "0 30 8 * * 1-5" {
		t.Errorf("stored cron = %q, want the normalized 6-field form", result.CronExpr)
	}
	if result.AgentSlug != "scout" {
		t.Errorf("agent_slug = %q, want it to default to the calling agent", result.AgentSlug)
	}

	var name, cronExpr, prompt, sType, threadID string
	var enabled bool
	if err := db.QueryRow(
		"SELECT name, cron_expr, prompt_content, type, thread_id, enabled FROM schedules WHERE id = ?",
		result.ScheduleID,
	).Scan(&name, &cronExpr, &prompt, &sType, &threadID, &enabled); err != nil {
		t.Fatalf("schedule was not saved: %v", err)
	}
	if name != "Morning sweep" || sType != "prompt" || !enabled {
		t.Errorf("saved row is wrong: name=%q type=%q enabled=%v", name, sType, enabled)
	}
	// Unpinned by default: a recurring routine posting into this chat every
	// morning would bury the conversation.
	if threadID != "" {
		t.Errorf("thread_id = %q, want it unpinned unless post_in_this_chat was set", threadID)
	}

	if len(sched.added) != 1 {
		t.Fatalf("registered %d cron entries, want 1 — a saved schedule that isn't registered never fires", len(sched.added))
	}
	if sched.added[0].CronExpr != "0 30 8 * * 1-5" || sched.added[0].PromptContent != prompt {
		t.Errorf("registered config does not match the saved row: %+v", sched.added[0])
	}
}

func TestScheduleCreatePinsToChatOnRequest(t *testing.T) {
	m, db, _ := newScheduleTestManager(t)

	out, isErr := invoke(t, m, "schedule_create", map[string]interface{}{
		"name":              "Standup",
		"cron_expr":         "@daily",
		"prompt":            "Post the standup summary.",
		"post_in_this_chat": true,
	}, true)
	if isErr {
		t.Fatalf("schedule_create failed: %s", out)
	}

	var result struct {
		ScheduleID string `json:"schedule_id"`
	}
	json.Unmarshal([]byte(out), &result)

	var threadID string
	db.QueryRow("SELECT thread_id FROM schedules WHERE id = ?", result.ScheduleID).Scan(&threadID)
	if threadID != "thread-1" {
		t.Errorf("thread_id = %q, want the current thread", threadID)
	}
}

// A schedule with a broken expression saves happily and then never fires, which
// reads as the feature being broken rather than the input.
func TestScheduleCreateRejectsBadCron(t *testing.T) {
	m, db, sched := newScheduleTestManager(t)

	out, isErr := invoke(t, m, "schedule_create", map[string]interface{}{
		"name":      "Whenever",
		"cron_expr": "every morning-ish",
		"prompt":    "Do the thing.",
	}, true)
	if !isErr {
		t.Fatalf("expected an unparseable cron to be rejected, got: %s", out)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM schedules").Scan(&count)
	if count != 0 {
		t.Errorf("a rejected schedule still wrote %d row(s)", count)
	}
	if len(sched.added) != 0 {
		t.Errorf("a rejected schedule still registered %d cron entrie(s)", len(sched.added))
	}
}

func TestScheduleCreateRejectsUnknownAgent(t *testing.T) {
	m, db, _ := newScheduleTestManager(t)

	out, isErr := invoke(t, m, "schedule_create", map[string]interface{}{
		"name":       "Ghost job",
		"cron_expr":  "@daily",
		"prompt":     "Do the thing.",
		"agent_slug": "nobody",
	}, true)
	if !isErr {
		t.Fatalf("expected an unknown agent to be rejected, got: %s", out)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM schedules").Scan(&count)
	if count != 0 {
		t.Errorf("a rejected schedule still wrote %d row(s)", count)
	}
}

// The live cron entry carries the prompt and agent as well as the timing, so an
// edit that isn't re-registered keeps running the old version — which looks
// exactly like the edit not saving.
func TestScheduleUpdateReRegisters(t *testing.T) {
	m, _, sched := newScheduleTestManager(t)

	out, _ := invoke(t, m, "schedule_create", map[string]interface{}{
		"name": "Sweep", "cron_expr": "@daily", "prompt": "Old instruction.",
	}, true)
	var created struct {
		ScheduleID string `json:"schedule_id"`
	}
	json.Unmarshal([]byte(out), &created)
	sched.added = nil

	out, isErr := invoke(t, m, "schedule_update", map[string]interface{}{
		"schedule_id": created.ScheduleID,
		"prompt":      "New instruction.",
	}, true)
	if isErr {
		t.Fatalf("schedule_update failed: %s", out)
	}

	if len(sched.removed) != 1 || len(sched.added) != 1 {
		t.Fatalf("expected the cron entry to be replaced, got removed=%d added=%d", len(sched.removed), len(sched.added))
	}
	if sched.added[0].PromptContent != "New instruction." {
		t.Errorf("re-registered with the old prompt %q", sched.added[0].PromptContent)
	}
}

// Pausing must take the entry out of cron, not just flip a column.
func TestScheduleUpdateDisableRemovesFromCron(t *testing.T) {
	m, _, sched := newScheduleTestManager(t)

	out, _ := invoke(t, m, "schedule_create", map[string]interface{}{
		"name": "Sweep", "cron_expr": "@daily", "prompt": "Do it.",
	}, true)
	var created struct {
		ScheduleID string `json:"schedule_id"`
	}
	json.Unmarshal([]byte(out), &created)
	sched.added = nil

	if out, isErr := invoke(t, m, "schedule_update", map[string]interface{}{
		"schedule_id": created.ScheduleID,
		"enabled":     false,
	}, true); isErr {
		t.Fatalf("schedule_update failed: %s", out)
	}

	if len(sched.removed) != 1 {
		t.Errorf("a paused schedule was not removed from cron")
	}
	if len(sched.added) != 0 {
		t.Errorf("a paused schedule was re-registered with cron and will still fire")
	}
}

// Dashboard and service schedules carry a payload these tools cannot express;
// editing one through here would half-rewrite it and break what depends on it.
func TestScheduleToolsRefuseNonPromptSchedules(t *testing.T) {
	m, db, _ := newScheduleTestManager(t)

	if _, err := db.Exec(
		`INSERT INTO schedules (id, name, cron_expr, tool_id, action, payload, enabled, type)
		 VALUES ('s-widget', 'Widget refresh', '0 */5 * * * *', 't1', 'refresh', '{}', 1, 'tool')`,
	); err != nil {
		t.Fatalf("insert tool schedule: %v", err)
	}

	if out, isErr := invoke(t, m, "schedule_update", map[string]interface{}{
		"schedule_id": "s-widget", "cron_expr": "@daily",
	}, true); !isErr {
		t.Errorf("expected update of a tool schedule to be refused, got: %s", out)
	}
	if out, isErr := invoke(t, m, "schedule_delete", map[string]interface{}{
		"schedule_id": "s-widget",
	}, true); !isErr {
		t.Errorf("expected delete of a tool schedule to be refused, got: %s", out)
	}

	var stillThere int
	db.QueryRow("SELECT COUNT(*) FROM schedules WHERE id = 's-widget'").Scan(&stillThere)
	if stillThere != 1 {
		t.Error("the tool schedule was modified or deleted despite the refusal")
	}
}

// An unattended run is itself the output of a schedule. One that could create
// schedules would file a new one on every fire, each of which then fires and
// files its own, with nobody watching.
func TestUnattendedRunsCannotModifySchedules(t *testing.T) {
	m, _, _ := newScheduleTestManager(t)

	readOnly := m.MakeScheduleToolHandlers("scout", "", false)
	for _, name := range []string{"schedule_create", "schedule_update", "schedule_delete", "schedule_run_now"} {
		if _, ok := readOnly[name]; ok {
			t.Errorf("%s is reachable in an unattended run", name)
		}
	}
	if _, ok := readOnly["schedule_list"]; !ok {
		t.Error("schedule_list should still be readable in an unattended run")
	}

	defs := BuildScheduleToolDefs(false)
	if len(defs) != 1 || defs[0].Function.Name != "schedule_list" {
		t.Errorf("unattended tool defs = %+v, want schedule_list only", defs)
	}
	if full := BuildScheduleToolDefs(true); len(full) != 5 {
		t.Errorf("attended tool defs = %d, want 5", len(full))
	}
}

// The context marker is what carries "nobody is watching" down to where tools
// are assembled; if it stops propagating, the guard above silently turns off.
func TestUnattendedContextMarker(t *testing.T) {
	if isUnattended(context.Background()) {
		t.Error("a plain context should not be marked unattended")
	}
	if !isUnattended(WithUnattended(context.Background())) {
		t.Error("WithUnattended did not mark the context")
	}
}
