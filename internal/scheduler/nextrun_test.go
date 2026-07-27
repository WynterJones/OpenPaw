package scheduler

import (
	"database/sql"
	"testing"
	"time"

	"github.com/openpaw/openpaw/internal/database"
)

// hourly fires at the top of every hour (leading field is seconds — the
// scheduler is built with cron.WithSeconds()).
const hourly = "0 0 * * * *"

func newSchedulerTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertSchedule(t *testing.T, db *database.DB, id string, nextRun *time.Time) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO schedules (id, name, cron_expr, tool_id, enabled, type, agent_role_slug, prompt_content, next_run_at)
		 VALUES (?, ?, ?, '', 1, 'prompt', 'researcher', 'go', ?)`,
		id, "Daily research", hourly, nextRun,
	); err != nil {
		t.Fatalf("insert schedule: %v", err)
	}
}

func nextRunOf(t *testing.T, db *database.DB, id string) sql.NullTime {
	t.Helper()
	var next sql.NullTime
	if err := db.QueryRow("SELECT next_run_at FROM schedules WHERE id = ?", id).Scan(&next); err != nil {
		t.Fatalf("read next_run_at: %v", err)
	}
	return next
}

// next_run_at was never written by anything, so the Scheduler page's "Next run"
// read a permanently NULL column and every active schedule said "Not scheduled".
func TestAddSchedule_RecordsNextRun(t *testing.T) {
	db := newSchedulerTestDB(t)
	s := New(db)
	insertSchedule(t, db, "s1", nil)

	s.AddSchedule(ScheduleConfig{ID: "s1", CronExpr: hourly, AgentRoleSlug: "researcher", PromptContent: "go"})

	next := nextRunOf(t, db, "s1")
	if !next.Valid {
		t.Fatal("next_run_at still NULL after registering the schedule")
	}
	if !next.Time.After(time.Now().UTC()) {
		t.Errorf("next_run_at = %v, want a time in the future", next.Time)
	}

	// A schedule with no stored due time has nothing to have missed.
	var missed int
	db.QueryRow("SELECT COUNT(*) FROM schedule_executions WHERE status = 'missed'").Scan(&missed)
	if missed != 0 {
		t.Errorf("recorded %d missed runs for a brand new schedule", missed)
	}

	// A paused schedule must not keep advertising a run that won't happen.
	s.RemoveSchedule("s1")
	if nextRunOf(t, db, "s1").Valid {
		t.Error("next_run_at survived removal")
	}
}

// cron has no catch-up: a run due while the machine was asleep simply never
// happened, and left no trace anywhere for the user to find.
func TestAddSchedule_RecordsMissedRuns(t *testing.T) {
	db := newSchedulerTestDB(t)
	s := New(db)

	// Due three hours ago — an hourly schedule missed roughly three fires.
	due := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Hour)
	insertSchedule(t, db, "s1", &due)

	s.AddSchedule(ScheduleConfig{ID: "s1", CronExpr: hourly, AgentRoleSlug: "researcher", PromptContent: "go"})

	var count int
	db.QueryRow("SELECT COUNT(*) FROM schedule_executions WHERE schedule_id = 's1' AND status = 'missed'").Scan(&count)

	var errMsg string
	var startedAt time.Time
	db.QueryRow(
		"SELECT error, started_at FROM schedule_executions WHERE schedule_id = 's1' AND status = 'missed' LIMIT 1",
	).Scan(&errMsg, &startedAt)

	// One row for the whole gap — a minutely schedule offline overnight would
	// otherwise bury the history under hundreds of identical entries.
	if count != 1 {
		t.Fatalf("recorded %d missed-run rows, want 1", count)
	}
	if errMsg == "" {
		t.Error("missed run has no explanation")
	}
	if !startedAt.UTC().Truncate(time.Second).Equal(due.Truncate(time.Second)) {
		t.Errorf("started_at = %v, want the time it was due (%v)", startedAt, due)
	}

	// And the schedule is caught up afterwards, so the next boot doesn't report
	// the same gap again.
	next := nextRunOf(t, db, "s1")
	if !next.Valid || !next.Time.After(time.Now().UTC()) {
		t.Errorf("next_run_at = %v, want a future time", next)
	}
	s.RemoveSchedule("s1")
	s.AddSchedule(ScheduleConfig{ID: "s1", CronExpr: hourly, AgentRoleSlug: "researcher", PromptContent: "go"})
	db.QueryRow("SELECT COUNT(*) FROM schedule_executions WHERE schedule_id = 's1' AND status = 'missed'").Scan(&count)
	if count != 1 {
		t.Errorf("re-registering reported the gap again (%d rows)", count)
	}
}

// An expression cron can't parse must not leave a stale next-run time claiming
// the schedule is going to fire.
func TestSetNextRun_ClearsOnBadExpression(t *testing.T) {
	db := newSchedulerTestDB(t)
	s := New(db)
	future := time.Now().UTC().Add(time.Hour)
	insertSchedule(t, db, "s1", &future)

	s.setNextRun("s1", "not a cron expression")

	if nextRunOf(t, db, "s1").Valid {
		t.Error("next_run_at kept a value for an unparseable expression")
	}
}
