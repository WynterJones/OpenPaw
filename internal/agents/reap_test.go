package agents

import (
	"testing"

	"github.com/openpaw/openpaw/internal/database"
)

func newReapTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertWorkOrder(t *testing.T, db *database.DB, id, status, threadID string) {
	t.Helper()
	if _, err := db.Exec(
		"INSERT INTO work_orders (id, type, status, title, thread_id) VALUES (?, 'tool_build', ?, ?, ?)",
		id, status, "Weather Service", threadID,
	); err != nil {
		t.Fatalf("insert work order: %v", err)
	}
}

func statusOf(t *testing.T, db *database.DB, table, id string) string {
	t.Helper()
	var status string
	if err := db.QueryRow("SELECT status FROM "+table+" WHERE id = ?", id).Scan(&status); err != nil {
		t.Fatalf("read %s status: %v", table, err)
	}
	return status
}

// Nothing ever finished a work order whose process died, so the chat it belonged
// to reported itself active forever — a spinner that outlived the build by days.
// The scheduler and heartbeat already reap this way; builds did not.
func TestReapOrphanedWork(t *testing.T) {
	db := newReapTestDB(t)

	if _, err := db.Exec("INSERT INTO chat_threads (id, title) VALUES ('t1', 'Weather')"); err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	insertWorkOrder(t, db, "wo-inprogress", string(WorkOrderInProgress), "t1")
	insertWorkOrder(t, db, "wo-pending", string(WorkOrderPending), "")
	insertWorkOrder(t, db, "wo-awaiting", string(WorkOrderAwaitingConfirmation), "")
	insertWorkOrder(t, db, "wo-done", string(WorkOrderCompleted), "")
	if _, err := db.Exec(
		"INSERT INTO agents (id, type, status, work_order_id) VALUES ('a1', 'builder', 'running', 'wo-inprogress')",
	); err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	ReapOrphanedWork(db)

	for id, want := range map[string]string{
		"wo-inprogress": string(WorkOrderFailed),
		"wo-pending":    string(WorkOrderFailed),
		// Waiting on the user, not on a process — a restart doesn't invalidate it.
		"wo-awaiting": string(WorkOrderAwaitingConfirmation),
		"wo-done":     string(WorkOrderCompleted),
	} {
		if got := statusOf(t, db, "work_orders", id); got != want {
			t.Errorf("%s = %q, want %q", id, got, want)
		}
	}

	if got := statusOf(t, db, "agents", "a1"); got != "failed" {
		t.Errorf("agent status = %q, want failed", got)
	}

	// The thread's last word was "🔨 Building X." — it has to say what happened.
	var content string
	db.QueryRow(
		"SELECT content FROM chat_messages WHERE thread_id = 't1' ORDER BY created_at DESC LIMIT 1",
	).Scan(&content)
	if content == "" {
		t.Fatal("no message filed on the interrupted thread")
	}

	// Reaping twice must not file the news twice — every boot runs it.
	ReapOrphanedWork(db)
	var msgs int
	db.QueryRow("SELECT COUNT(*) FROM chat_messages WHERE thread_id = 't1'").Scan(&msgs)
	if msgs != 1 {
		t.Errorf("filed %d messages after a second reap, want 1", msgs)
	}
}
