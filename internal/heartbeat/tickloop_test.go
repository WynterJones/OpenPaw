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

// The loop used to wait a full interval before its first run, so a desktop app
// restarted more often than once an hour never ran a heartbeat at all.
func TestFirstDelay_NeverRunBefore(t *testing.T) {
	m := newTestManager(t)
	if got := m.firstDelay(time.Hour); got != settleDelay {
		t.Errorf("firstDelay with no history = %v, want %v", got, settleDelay)
	}
}

// Overdue while the app was closed — run shortly after boot, not an hour later.
func TestFirstDelay_OverdueRunsSoon(t *testing.T) {
	m := newTestManager(t)
	m.db.Exec(
		"INSERT INTO heartbeat_executions (id, agent_role_slug, status, started_at) VALUES ('x','a','success',?)",
		time.Now().UTC().Add(-3*time.Hour),
	)
	if got := m.firstDelay(time.Hour); got != settleDelay {
		t.Errorf("firstDelay when overdue = %v, want %v", got, settleDelay)
	}
}

// A recent run means the schedule survives the restart instead of resetting.
func TestFirstDelay_RecentRunWaitsRemainder(t *testing.T) {
	m := newTestManager(t)
	m.db.Exec(
		"INSERT INTO heartbeat_executions (id, agent_role_slug, status, started_at) VALUES ('x','a','success',?)",
		time.Now().UTC().Add(-20*time.Minute),
	)
	got := m.firstDelay(time.Hour)
	// ~40 minutes remaining; allow slack for test execution time.
	if got < 38*time.Minute || got > 41*time.Minute {
		t.Errorf("firstDelay 20min into an hourly interval = %v, want ~40m", got)
	}
}

func TestFirstDelay_ZeroIntervalDoesNotSpin(t *testing.T) {
	m := newTestManager(t)
	if got := m.firstDelay(0); got < settleDelay {
		t.Errorf("firstDelay(0) = %v, want at least %v", got, settleDelay)
	}
}
