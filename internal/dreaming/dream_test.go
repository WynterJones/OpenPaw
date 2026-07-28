package dreaming

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/memory"
)

func newTestManager(t *testing.T) (*Manager, *database.DB) {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	mem := memory.NewManager(t.TempDir())
	t.Cleanup(mem.Close)

	m := New(db, mem, nil, nil)
	return m, db
}

func seedThread(t *testing.T, db *database.DB, threadID, slug string, msgTimes ...time.Time) {
	t.Helper()
	if _, err := db.Exec(
		"INSERT INTO chat_threads (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)",
		threadID, "Thread "+threadID, msgTimes[0], msgTimes[len(msgTimes)-1],
	); err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	for i, at := range msgTimes {
		if _, err := db.Exec(
			`INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, created_at)
			 VALUES (?, ?, 'assistant', ?, ?, ?)`,
			uuid.New().String(), threadID, "message", slug, at,
		); err != nil {
			t.Fatalf("insert message %d: %v", i, err)
		}
	}
}

// A dream must not pay to re-read a conversation that has not changed — that
// re-read is the single largest cost in the feature, and it would recur nightly
// forever.
func TestUnscannedThreadsSkipsUnchangedChats(t *testing.T) {
	m, db := newTestManager(t)

	base := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	seedThread(t, db, "t-quiet", "scout", base)
	seedThread(t, db, "t-continued", "scout", base, base.Add(time.Hour))
	seedThread(t, db, "t-fresh", "scout", base)

	// Both scanned up to `base`. Only the one that gained a later message
	// afterwards should come back.
	for _, id := range []string{"t-quiet", "t-continued"} {
		m.recordScan("scout", threadRef{ID: id, UpdatedAt: base}, 1)
	}

	got, err := m.unscannedThreads("scout", 10)
	if err != nil {
		t.Fatalf("unscannedThreads: %v", err)
	}

	seen := map[string]bool{}
	for _, tr := range got {
		seen[tr.ID] = true
	}
	if seen["t-quiet"] {
		t.Error("re-scanned a chat that had not changed since the last dream")
	}
	if !seen["t-continued"] {
		t.Error("skipped a chat that gained new messages after being scanned")
	}
	if !seen["t-fresh"] {
		t.Error("skipped a chat that had never been scanned")
	}
}

// Chats another agent held are not this agent's to remember.
func TestUnscannedThreadsIsScopedToTheAgent(t *testing.T) {
	m, db := newTestManager(t)

	now := time.Now().UTC().Truncate(time.Second)
	seedThread(t, db, "t-scout", "scout", now)
	seedThread(t, db, "t-other", "builder", now)

	got, err := m.unscannedThreads("scout", 10)
	if err != nil {
		t.Fatalf("unscannedThreads: %v", err)
	}
	if len(got) != 1 || got[0].ID != "t-scout" {
		t.Fatalf("expected only the scout's own chat, got %+v", got)
	}
}

// recordScan runs on every scanned thread, including ones scanned before, so
// the second write has to update rather than collide with the unique index.
func TestRecordScanIsIdempotent(t *testing.T) {
	m, db := newTestManager(t)

	at := time.Now().UTC().Truncate(time.Second)
	m.recordScan("scout", threadRef{ID: "t-1", UpdatedAt: at}, 2)
	m.recordScan("scout", threadRef{ID: "t-1", UpdatedAt: at.Add(time.Hour)}, 5)

	var rows, facts int
	db.QueryRow("SELECT COUNT(*), COALESCE(MAX(facts_found), 0) FROM dream_scans WHERE thread_id = 't-1'").Scan(&rows, &facts)
	if rows != 1 {
		t.Errorf("expected 1 scan row after rescanning, got %d", rows)
	}
	if facts != 5 {
		t.Errorf("expected the rescan to replace facts_found with 5, got %d", facts)
	}
}

// applyOps is the only path in the app that deletes memories with nobody
// watching, so its guards matter more than the model's judgement.
func TestApplyOpsGuardsDeletions(t *testing.T) {
	m, _ := newTestManager(t)

	critical, err := m.mem.Add("scout", memory.Record{
		Content: "Always deploy from the release branch", Importance: 10,
	})
	if err != nil {
		t.Fatalf("seed critical memory: %v", err)
	}
	ordinary, err := m.mem.Add("scout", memory.Record{
		Content: "The staging URL is staging.example.com", Importance: 4,
	})
	if err != nil {
		t.Fatalf("seed ordinary memory: %v", err)
	}

	existing, err := m.mem.Recent("scout", 100)
	if err != nil {
		t.Fatalf("read seeded memories: %v", err)
	}

	ops := consolidateOps{}
	ops.Forget = append(ops.Forget,
		struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		}{ID: critical, Reason: "model thinks it is stale"},
		struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		}{ID: ordinary, Reason: "superseded"},
		struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		}{ID: "not-a-real-id", Reason: "hallucinated"},
	)

	_, _, pruned := m.applyOps("scout", ops, existing)
	if pruned != 1 {
		t.Fatalf("expected exactly 1 deletion, got %d", pruned)
	}

	left, err := m.mem.Recent("scout", 100)
	if err != nil {
		t.Fatalf("re-read memories: %v", err)
	}
	if len(left) != 1 || left[0].ID != critical {
		t.Fatalf("expected only the protected memory to survive, got %+v", left)
	}
}

// An id the model invented must not reach the database — on its own it matches
// nothing, but an id half-remembered from elsewhere would overwrite real memory.
func TestApplyOpsRejectsUnknownUpdateIDs(t *testing.T) {
	m, _ := newTestManager(t)

	real, err := m.mem.Add("scout", memory.Record{Content: "Original wording", Importance: 5})
	if err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	existing, _ := m.mem.Recent("scout", 100)

	ops := consolidateOps{
		Update: []memory.Record{
			{ID: real, Content: "Sharpened wording"},
			{ID: "ghost-id", Content: "Should not be written"},
		},
	}

	_, updated, _ := m.applyOps("scout", ops, existing)
	if updated != 1 {
		t.Fatalf("expected 1 update, got %d", updated)
	}

	after, _ := m.mem.Recent("scout", 100)
	if len(after) != 1 || after[0].Content != "Sharpened wording" {
		t.Fatalf("update did not apply as expected: %+v", after)
	}
}

// The add path must not undo what consolidation just merged by re-adding a
// memory that already exists verbatim.
func TestApplyOpsSkipsDuplicateAdds(t *testing.T) {
	m, _ := newTestManager(t)

	if _, err := m.mem.Add("scout", memory.Record{Content: "Wynter prefers Go", Importance: 7}); err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	existing, _ := m.mem.Recent("scout", 100)

	ops := consolidateOps{
		Add: []memory.Record{
			{Content: "wynter prefers go."},        // same fact, different punctuation
			{Content: "Wynter deploys on Fridays"}, // genuinely new
		},
	}

	added, _, _ := m.applyOps("scout", ops, existing)
	if added != 1 {
		t.Fatalf("expected 1 add (the duplicate should be skipped), got %d", added)
	}
}

// Models asked for bare JSON return it fenced, prefaced, or both often enough
// that every reply here goes through this first.
func TestExtractJSON(t *testing.T) {
	want := `{"memories":[]}`
	cases := []struct {
		name, in string
	}{
		{"bare", want},
		{"fenced", "```json\n" + want + "\n```"},
		{"unlabelled fence", "```\n" + want + "\n```"},
		{"prefaced", "Here is the result:\n" + want},
		{"trailing prose", want + "\n\nLet me know if you need more."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractJSON(tc.in); got != want {
				t.Errorf("extractJSON(%q) = %q, want %q", tc.in, got, want)
			}
		})
	}
}

// The defaults are the shipped behaviour, and they are the whole cost story:
// both switches on would read every conversation twice, once per message and
// once per night. Flipping either by accident is expensive rather than broken,
// which is exactly the kind of change that slips through review.
func TestDefaultsRunExactlyOneCapturePath(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("dreaming should be on by default — it is the only capture path once the reflex is off")
	}
	if cfg.ReflexEnabled {
		t.Error("the reflex should be off by default; with dreaming on it re-reads the same chats at a call per message")
	}
	if cfg.CronExpr != CronDaily {
		t.Errorf("default cron = %q, want %q", cfg.CronExpr, CronDaily)
	}
	if cfg.ReviewLimit != 100 {
		t.Errorf("default review limit = %d, want 100", cfg.ReviewLimit)
	}
}

// A typo'd cron must be refused at save time. Accepting it and falling back at
// load would leave the user believing a schedule is in place that isn't.
func TestUpdateConfigRejectsBadCron(t *testing.T) {
	m, _ := newTestManager(t)

	if err := m.UpdateConfig(map[string]string{"dreaming_cron": "every tuesday-ish"}); err == nil {
		t.Fatal("expected an unparseable cron expression to be rejected")
	}
	if got := m.GetConfig()["dreaming_cron"]; got != CronDaily {
		t.Errorf("a rejected cron changed the stored value to %q", got)
	}

	if err := m.UpdateConfig(map[string]string{"dreaming_cron": CronWeekly}); err != nil {
		t.Fatalf("valid cron rejected: %v", err)
	}
	if got := m.GetConfig()["dreaming_cron"]; got != CronWeekly {
		t.Errorf("dreaming_cron = %q, want %q", got, CronWeekly)
	}
}
