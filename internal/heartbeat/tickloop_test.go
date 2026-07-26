package heartbeat

import (
	"testing"
	"time"

	"github.com/openpaw/openpaw/internal/database"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &Manager{db: db, config: DefaultConfig()}
}

func insertRun(t *testing.T, m *Manager, slug string, ago time.Duration) {
	t.Helper()
	m.db.Exec(
		"INSERT INTO heartbeat_executions (id, agent_role_slug, status, started_at) VALUES (?,?,'success',?)",
		slug+"-run", slug, time.Now().UTC().Add(-ago),
	)
}

// The loop used to wait a full interval before its first run, so a desktop app
// restarted more often than once an hour never ran a heartbeat at all.
func TestIsDue_NeverRunBefore(t *testing.T) {
	m := newTestManager(t)
	if !m.isDue("a", time.Hour) {
		t.Error("agent with no history is not due, want due")
	}
}

// Overdue while the app was closed — run shortly after boot, not an hour later.
func TestIsDue_OverdueRunsSoon(t *testing.T) {
	m := newTestManager(t)
	insertRun(t, m, "a", 3*time.Hour)
	if !m.isDue("a", time.Hour) {
		t.Error("agent 3h into an hourly interval is not due, want due")
	}
}

// A recent run means the schedule survives the restart instead of resetting.
func TestIsDue_RecentRunWaits(t *testing.T) {
	m := newTestManager(t)
	insertRun(t, m, "a", 20*time.Minute)
	if m.isDue("a", time.Hour) {
		t.Error("agent 20min into an hourly interval is due, want not due")
	}
}

// Each agent is scheduled off its own last run, which is the whole point of
// per-agent intervals: a slow agent must not hold a fast one back, and a fast
// one must not drag a slow one along with it.
func TestIsDue_PerAgentIndependent(t *testing.T) {
	m := newTestManager(t)
	insertRun(t, m, "fast", 31*time.Minute)
	insertRun(t, m, "slow", 31*time.Minute)

	if !m.isDue("fast", 30*time.Minute) {
		t.Error("fast agent past its 30m interval is not due, want due")
	}
	if m.isDue("slow", 24*time.Hour) {
		t.Error("slow agent 31m into a daily interval is due, want not due")
	}
}

func TestIsDue_ZeroIntervalDoesNotSpin(t *testing.T) {
	m := newTestManager(t)
	insertRun(t, m, "a", time.Minute)
	// 0 is "inherit"; an unset global must not degrade into a hot loop.
	if m.isDue("a", 0) {
		t.Error("agent 1min into a zero interval is due, want the 1h fallback")
	}
}

// A skip used to return before any execution row was written, so a heartbeat
// that could never run looked identical to one that was never scheduled.
func TestRecordSkip_IsVisibleAndDeduped(t *testing.T) {
	m := newTestManager(t)
	m.broadcast = func(string, interface{}) {}

	const reason = "HEARTBEAT.md is empty. Add instructions."

	m.recordSkip("nabu", reason)

	var count int
	var status, errText string
	m.db.QueryRow("SELECT COUNT(*) FROM heartbeat_executions WHERE agent_role_slug = 'nabu'").Scan(&count)
	if count != 1 {
		t.Fatalf("after first skip: %d rows, want 1", count)
	}
	m.db.QueryRow("SELECT status, error FROM heartbeat_executions WHERE agent_role_slug = 'nabu'").Scan(&status, &errText)
	if status != "skipped" {
		t.Errorf("status = %q, want skipped", status)
	}
	if errText != reason {
		t.Errorf("error = %q, want the reason", errText)
	}

	// Repeating the same skip must not add a row every interval forever.
	m.recordSkip("nabu", reason)
	m.recordSkip("nabu", reason)
	m.db.QueryRow("SELECT COUNT(*) FROM heartbeat_executions WHERE agent_role_slug = 'nabu'").Scan(&count)
	if count != 1 {
		t.Errorf("after repeats: %d rows, want 1 (deduped)", count)
	}

	// A different reason is new information and should be recorded.
	m.recordSkip("nabu", "HEARTBEAT.md could not be read.")
	m.db.QueryRow("SELECT COUNT(*) FROM heartbeat_executions WHERE agent_role_slug = 'nabu'").Scan(&count)
	if count != 2 {
		t.Errorf("after a new reason: %d rows, want 2", count)
	}
}

// Scheduling reads the latest execution to decide when an agent is next due, so
// a deduped skip must still move its timestamp. Otherwise a misconfigured agent
// stays permanently overdue and is retried on every tick forever.
func TestRecordSkip_DedupeStillAdvancesSchedule(t *testing.T) {
	m := newTestManager(t)
	m.broadcast = func(string, interface{}) {}

	const reason = "HEARTBEAT.md is empty."
	const interval = time.Hour

	m.recordSkip("nabu", reason)
	// Backdate it well past the interval, as if the app had been closed.
	m.db.Exec(
		"UPDATE heartbeat_executions SET started_at = ? WHERE agent_role_slug = 'nabu'",
		time.Now().UTC().Add(-3*interval),
	)
	if !m.isDue("nabu", interval) {
		t.Fatal("backdated skip is not due, want due")
	}

	// The retry skips again for the same reason — and must not stay due.
	m.recordSkip("nabu", reason)
	if m.isDue("nabu", interval) {
		t.Error("agent is still due right after a repeat skip, want the interval to restart")
	}
}
