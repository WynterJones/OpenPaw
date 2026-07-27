package handlers

import (
	"testing"
)

// Every route lands on handleRoleChatWithDepth, and the slug reaching it can
// come from a stale UI selection, a hand-typed @mention, one agent naming
// another, or the gateway. Any of those can name an agent that was renamed,
// disabled or deleted — and the answer used to be an apology that ended the
// turn with the user's message dropped.
func TestThreadFallbackAgent(t *testing.T) {
	db := newTestDB(t)
	h := &ChatHandler{db: db}

	insertAgentRole(t, db, "atlas", "Atlas", true)
	insertAgentRole(t, db, "retired", "Retired Helper", false)

	if _, err := db.Exec("INSERT INTO chat_threads (id, title) VALUES ('t1', 'Chat')"); err != nil {
		t.Fatalf("insert thread: %v", err)
	}

	t.Run("no members falls back to any enabled agent", func(t *testing.T) {
		if got := h.threadFallbackAgent("t1"); got != "atlas" {
			t.Errorf("got %q, want atlas", got)
		}
	})

	t.Run("a disabled thread member is skipped", func(t *testing.T) {
		// A thread keeps its members after an agent is switched off, so the
		// member list alone is not a safe answer.
		if _, err := db.Exec("INSERT INTO thread_members (thread_id, agent_role_slug) VALUES ('t1', 'retired')"); err != nil {
			t.Fatalf("insert member: %v", err)
		}
		if got := h.threadFallbackAgent("t1"); got != "atlas" {
			t.Errorf("got %q, want atlas — the disabled member should be skipped", got)
		}
	})

	t.Run("an enabled thread member is preferred", func(t *testing.T) {
		insertAgentRole(t, db, "whiskers", "Whiskers", true)
		if _, err := db.Exec("INSERT INTO thread_members (thread_id, agent_role_slug) VALUES ('t1', 'whiskers')"); err != nil {
			t.Fatalf("insert member: %v", err)
		}
		if got := h.threadFallbackAgent("t1"); got != "whiskers" {
			t.Errorf("got %q, want whiskers — the thread's own agent should answer", got)
		}
	})
}

// With no agents at all there is nothing to fall back to, and the reply should
// say what to do rather than blaming a missing role.
func TestThreadFallbackAgent_NoAgents(t *testing.T) {
	db := newTestDB(t)
	h := &ChatHandler{db: db}
	if got := h.threadFallbackAgent("t1"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
