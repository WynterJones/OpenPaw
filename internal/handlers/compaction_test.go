package handlers

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
)

// newTestHandler builds a ChatHandler backed by a real migrated SQLite database
// in a temp dir. agentManager is left nil — the compaction path under test takes
// its summarizer by injection, so no provider is needed.
func newTestHandler(t *testing.T) *ChatHandler {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &ChatHandler{db: db}
}

func createTestThread(t *testing.T, h *ChatHandler) string {
	t.Helper()
	id := generateID()
	now := time.Now().UTC()
	if _, err := h.db.Exec(
		"INSERT INTO chat_threads (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)",
		id, "Test Thread", now, now,
	); err != nil {
		t.Fatalf("create thread: %v", err)
	}
	return id
}

// addMessage appends a message one second after the previous one so ordering is
// deterministic regardless of how fast the test runs.
func addMessage(t *testing.T, h *ChatHandler, threadID, role, content string, inputTokens int, seq int) string {
	t.Helper()
	id := generateID()
	created := time.Date(2026, 1, 1, 0, 0, seq, 0, time.UTC)
	if _, err := h.db.Exec(
		"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id, threadID, role, content, "", 0.0, inputTokens, 0, created,
	); err != nil {
		t.Fatalf("insert message: %v", err)
	}
	return id
}

type storedMessage struct {
	ID      string
	Role    string
	Content string
}

func loadMessages(t *testing.T, h *ChatHandler, threadID string) []storedMessage {
	t.Helper()
	rows, err := h.db.Query(
		"SELECT id, role, content FROM chat_messages WHERE thread_id = ? ORDER BY created_at ASC, id ASC",
		threadID,
	)
	if err != nil {
		t.Fatalf("load messages: %v", err)
	}
	defer rows.Close()

	var out []storedMessage
	for rows.Next() {
		var m storedMessage
		if err := rows.Scan(&m.ID, &m.Role, &m.Content); err != nil {
			t.Fatalf("scan message: %v", err)
		}
		out = append(out, m)
	}
	return out
}

// stubSummarizer returns a fixed summary and records the transcript it saw.
func stubSummarizer(summary string, seen *string) summarizeFunc {
	return func(_ context.Context, transcript string) (string, *llm.UsageInfo, error) {
		if seen != nil {
			*seen = transcript
		}
		return summary, &llm.UsageInfo{InputTokens: 100, OutputTokens: 20}, nil
	}
}

// (a) Normal operation: a thread under the retain floor must not compact.
func TestCompaction_NotNeededBelowThreshold(t *testing.T) {
	h := newTestHandler(t)
	threadID := createTestThread(t, h)
	for i := 0; i < compactRetainMessages; i++ {
		addMessage(t, h, threadID, "user", fmt.Sprintf("message %d", i), 0, i)
	}

	err := h.compactThreadWith(context.Background(), threadID, stubSummarizer("summary", nil))
	if err == nil {
		t.Fatal("expected compaction to be refused for a short thread, got nil error")
	}
	if msgs := loadMessages(t, h, threadID); len(msgs) != compactRetainMessages {
		t.Fatalf("messages must be untouched, want %d got %d", compactRetainMessages, len(msgs))
	}
}

// (b) The exact boundary where compaction triggers.
func TestCompactionNeeded_ThresholdBoundary(t *testing.T) {
	const limit = 1000
	tests := []struct {
		name      string
		used      int
		threshold int
		want      bool
	}{
		{"just below threshold", 849, 85, false},
		{"exactly at threshold", 850, 85, true},
		{"just above threshold", 851, 85, true},
		{"zero usage never triggers", 0, 85, false},
		{"zero limit never triggers", 500, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lim := limit
			if tc.name == "zero limit never triggers" {
				lim = 0
			}
			if got := compactionNeeded(tc.used, lim, tc.threshold); got != tc.want {
				t.Fatalf("compactionNeeded(%d, %d, %d) = %v, want %v", tc.used, lim, tc.threshold, got, tc.want)
			}
		})
	}
}

// (c) Retained history after compaction: summary first, recent tail verbatim.
func TestCompaction_RetainsRecentHistory(t *testing.T) {
	h := newTestHandler(t)
	threadID := createTestThread(t, h)

	const total = compactRetainMessages + 4
	var ids []string
	for i := 0; i < total; i++ {
		ids = append(ids, addMessage(t, h, threadID, "user", fmt.Sprintf("message %d", i), 0, i))
	}
	retainedIDs := ids[total-compactRetainMessages:]

	var transcript string
	if err := h.compactThreadWith(context.Background(), threadID, stubSummarizer("THE SUMMARY", &transcript)); err != nil {
		t.Fatalf("compaction failed: %v", err)
	}

	msgs := loadMessages(t, h, threadID)
	if len(msgs) != compactRetainMessages+1 {
		t.Fatalf("want %d messages after compaction, got %d", compactRetainMessages+1, len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "THE SUMMARY" {
		t.Fatalf("summary must be prepended, got role=%q content=%q", msgs[0].Role, msgs[0].Content)
	}
	// The retained tail must survive byte-for-byte, with its original IDs.
	for i, wantID := range retainedIDs {
		got := msgs[i+1]
		if got.ID != wantID {
			t.Fatalf("retained message %d: ID = %q, want %q", i, got.ID, wantID)
		}
		wantContent := fmt.Sprintf("message %d", total-compactRetainMessages+i)
		if got.Content != wantContent {
			t.Fatalf("retained message %d: content = %q, want %q", i, got.Content, wantContent)
		}
	}
	// Only the older messages should have been summarized.
	if !strings.Contains(transcript, "message 0") {
		t.Error("transcript should contain the oldest message")
	}
	if strings.Contains(transcript, fmt.Sprintf("message %d", total-1)) {
		t.Error("transcript must not contain retained messages")
	}
	// A watermark must be recorded so usage is measured from here on.
	var compactedAt any
	if err := h.db.QueryRow("SELECT compacted_at FROM chat_threads WHERE id = ?", threadID).Scan(&compactedAt); err != nil {
		t.Fatalf("read compacted_at: %v", err)
	}
	if compactedAt == nil {
		t.Fatal("compacted_at must be set after compaction")
	}
}

// (d) Multiple successive compaction cycles stay stable and never lose the tail.
func TestCompaction_MultipleCycles(t *testing.T) {
	h := newTestHandler(t)
	threadID := createTestThread(t, h)

	seq := 0
	for i := 0; i < compactRetainMessages+3; i++ {
		addMessage(t, h, threadID, "user", fmt.Sprintf("round0-%d", i), 0, seq)
		seq++
	}

	for cycle := 1; cycle <= 3; cycle++ {
		if err := h.compactThreadWith(context.Background(), threadID, stubSummarizer(fmt.Sprintf("summary-%d", cycle), nil)); err != nil {
			t.Fatalf("cycle %d failed: %v", cycle, err)
		}
		msgs := loadMessages(t, h, threadID)
		if len(msgs) != compactRetainMessages+1 {
			t.Fatalf("cycle %d: want %d messages, got %d", cycle, compactRetainMessages+1, len(msgs))
		}
		if msgs[0].Content != fmt.Sprintf("summary-%d", cycle) {
			t.Fatalf("cycle %d: newest summary must lead, got %q", cycle, msgs[0].Content)
		}
		// Exactly one summary must exist — cycles must not accumulate them.
		summaries := 0
		for _, m := range msgs {
			if m.Role == "system" {
				summaries++
			}
		}
		if summaries != 1 {
			t.Fatalf("cycle %d: want exactly 1 summary, got %d", cycle, summaries)
		}
		// Add fresh traffic so the next cycle has something to compact.
		for i := 0; i < 4; i++ {
			addMessage(t, h, threadID, "user", fmt.Sprintf("round%d-%d", cycle, i), 0, seq)
			seq++
		}
	}
}

// (e) Reported context usage after compaction ignores the pre-compaction peak.
func TestThreadContextUsed_AfterCompaction(t *testing.T) {
	h := newTestHandler(t)
	threadID := createTestThread(t, h)

	addMessage(t, h, threadID, "user", "hello", 0, 0)
	addMessage(t, h, threadID, "assistant", "hi", 900_000, 1)

	if got := h.threadContextUsed(threadID); got != 900_000 {
		t.Fatalf("before compaction: usage = %d, want 900000", got)
	}

	// Simulate compaction having happened after those messages.
	watermark := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	if _, err := h.db.Exec("UPDATE chat_threads SET compacted_at = ? WHERE id = ?", watermark, threadID); err != nil {
		t.Fatalf("set compacted_at: %v", err)
	}

	// The old peak predates the watermark, so it must no longer count —
	// otherwise the thread would re-compact on every subsequent turn.
	if got := h.threadContextUsed(threadID); got != 0 {
		t.Fatalf("after compaction: usage = %d, want 0", got)
	}

	// A new turn after the watermark is counted again.
	addMessage(t, h, threadID, "assistant", "post-compaction reply", 1_200, 120)
	if got := h.threadContextUsed(threadID); got != 1_200 {
		t.Fatalf("after new turn: usage = %d, want 1200", got)
	}
}

// Usage must include messages appended after the last measured assistant turn,
// so a large paste triggers compaction before it overflows the window.
func TestThreadContextUsed_IncludesPendingMessages(t *testing.T) {
	h := newTestHandler(t)
	threadID := createTestThread(t, h)

	addMessage(t, h, threadID, "assistant", "reply", 1_000, 0)
	pending := strings.Repeat("x", 300)
	addMessage(t, h, threadID, "user", pending, 0, 1)

	want := 1_000 + len(pending)/charsPerTokenEstimate
	if got := h.threadContextUsed(threadID); got != want {
		t.Fatalf("usage = %d, want %d (peak + pending estimate)", got, want)
	}
}

// A CLI provider reports usage for the whole agentic run — every internal turn
// plus cache reads — so a tool-heavy turn logs millions of "input tokens". That
// is spend, not context fill, and must not be reported as context usage.
func TestThreadContextUsed_IgnoresCumulativeToolCallUsage(t *testing.T) {
	h := newTestHandler(t)
	threadID := createTestThread(t, h)

	addMessage(t, h, threadID, "user", "build the thing", 0, 0)
	// 3.5M reported against a 1M window: impossible as a context measurement.
	addMessage(t, h, threadID, "assistant", strings.Repeat("y", 900), 3_567_638, 1)

	got := h.threadContextUsed(threadID)
	if got >= 3_000_000 {
		t.Fatalf("cumulative run usage leaked into context usage: got %d", got)
	}
	// Falls back to sizing the conversation instead.
	if got <= systemPromptTokenAllowance {
		t.Fatalf("expected an estimate above the system allowance, got %d", got)
	}
	if limit := h.getEffectiveContextLimit(threadID); limit > 0 && got > limit {
		t.Fatalf("estimate %d exceeds the window %d", got, limit)
	}
}

// A plausible per-request count is still preferred — it includes the system
// prompt and tools that a message-size estimate can only approximate.
func TestThreadContextUsed_KeepsPlausibleReportedCount(t *testing.T) {
	h := newTestHandler(t)
	threadID := createTestThread(t, h)

	addMessage(t, h, threadID, "user", "hi", 0, 0)
	addMessage(t, h, threadID, "assistant", "short reply", 250_000, 1)

	if got := h.threadContextUsed(threadID); got != 250_000 {
		t.Fatalf("usage = %d, want the reported 250000", got)
	}
}

// An empty summary must never destroy history.
func TestCompaction_EmptySummaryAborts(t *testing.T) {
	h := newTestHandler(t)
	threadID := createTestThread(t, h)
	total := compactRetainMessages + 3
	for i := 0; i < total; i++ {
		addMessage(t, h, threadID, "user", fmt.Sprintf("message %d", i), 0, i)
	}

	err := h.compactThreadWith(context.Background(), threadID, stubSummarizer("   \n  ", nil))
	if err == nil {
		t.Fatal("expected an error for an empty summary")
	}
	if msgs := loadMessages(t, h, threadID); len(msgs) != total {
		t.Fatalf("history must be intact after a failed summary, want %d got %d", total, len(msgs))
	}
}

// A failing summarizer must leave the thread untouched.
func TestCompaction_SummarizerErrorLeavesThreadIntact(t *testing.T) {
	h := newTestHandler(t)
	threadID := createTestThread(t, h)
	total := compactRetainMessages + 3
	for i := 0; i < total; i++ {
		addMessage(t, h, threadID, "user", fmt.Sprintf("message %d", i), 0, i)
	}

	failing := func(context.Context, string) (string, *llm.UsageInfo, error) {
		return "", nil, fmt.Errorf("provider exploded")
	}
	if err := h.compactThreadWith(context.Background(), threadID, failing); err == nil {
		t.Fatal("expected the summarizer error to propagate")
	}
	if msgs := loadMessages(t, h, threadID); len(msgs) != total {
		t.Fatalf("history must be intact, want %d got %d", total, len(msgs))
	}
}

// The guard must make a second concurrent compaction a no-op rather than
// letting two summarizations race on the same thread.
func TestCompaction_GuardBlocksConcurrentRun(t *testing.T) {
	h := newTestHandler(t)
	threadID := createTestThread(t, h)

	h.compactingGuard.Store(threadID, true)
	defer h.compactingGuard.Delete(threadID)

	if err := h.doAutoCompact(context.Background(), threadID); err != nil {
		t.Fatalf("an in-flight compaction should be a silent no-op, got %v", err)
	}
}

// Stopping a reply used to discard the streamed text and save the word
// "Stopped." instead. The partial answer is the thing worth keeping, so it is
// stored verbatim and flagged — the flag is what lets the UI badge it rather
// than pass a truncated reply off as a finished one.
func TestSaveStoppedMessage_KeepsPartialAndFlagsIt(t *testing.T) {
	h := newTestHandler(t)
	threadID := createTestThread(t, h)

	const partial = "Here is the first half of the answer"
	id := h.saveStoppedMessage(threadID, "researcher", partial)
	if id == "" {
		t.Fatal("no message id returned")
	}

	var content, agent string
	var stopped bool
	if err := h.db.QueryRow(
		"SELECT content, agent_role_slug, stopped FROM chat_messages WHERE id = ?", id,
	).Scan(&content, &agent, &stopped); err != nil {
		t.Fatalf("read back message: %v", err)
	}
	if content != partial {
		t.Errorf("content = %q, want the partial text unchanged", content)
	}
	if agent != "researcher" {
		t.Errorf("agent_role_slug = %q, want researcher — the badge names who was replying", agent)
	}
	if !stopped {
		t.Error("stopped flag not set, so the UI would render this as a complete reply")
	}
}

// An ordinary reply must not come back flagged, or every message would badge.
func TestSaveAssistantMessage_IsNotStopped(t *testing.T) {
	h := newTestHandler(t)
	threadID := createTestThread(t, h)

	id := h.saveAssistantMessage(threadID, "researcher", "A complete answer", 0, 0, 0)

	var stopped bool
	if err := h.db.QueryRow("SELECT stopped FROM chat_messages WHERE id = ?", id).Scan(&stopped); err != nil {
		t.Fatalf("read back message: %v", err)
	}
	if stopped {
		t.Error("a normal reply came back flagged as stopped")
	}
}
