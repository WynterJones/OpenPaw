package tmux

import "testing"

// Captured verbatim from a live Claude Code tmux session.
const claudePane = `     ◻ P1: Asset + Template + seeds
     ◻ P2: Sales page, onboarding, dashboard, chat shell
     ◻ P3: Generation pipeline
      … +1 pending, 1 completed
────────────────────────────────────────────────────────────────────────────────
❯
────────────────────────────────────────────────────────────────────────────────
  ╭╼ WynterAI (main) +23 | Rails
  ╰──╼ Opus 5 (1M context) | ━━──────── 22% (smart) | 39m22s | +2255/-1991
  ⏵⏵ auto mode on (shift+tab to cycle) · ← 1 agent`

func TestParseStatus_ClaudeStatusLine(t *testing.T) {
	st := ParseStatus(claudePane)
	if st == nil {
		t.Fatal("expected a parsed status")
	}

	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"Project", st.Project, "WynterAI"},
		{"Branch", st.Branch, "main"},
		{"Uncommitted", st.Uncommitted, 23},
		{"Framework", st.Framework, "Rails"},
		{"Model", st.Model, "Opus 5 (1M context)"},
		{"ContextPct", st.ContextPct, 22},
		{"Elapsed", st.Elapsed, "39m22s"},
		{"LinesAdded", st.LinesAdded, 2255},
		{"LinesRemoved", st.LinesRemoved, 1991},
		{"AutoMode", st.AutoMode, true},
		{"Agents", st.Agents, 1},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
		}
	}
}

// Without the "+23" suffix the branch must still parse — an unmodified repo is
// the common case, and greedily eating the token would swallow the branch.
func TestParseStatus_NoUncommittedChanges(t *testing.T) {
	st := ParseStatus("  ╭╼ OpenPaw (feature/pins) | Go\n  ╰──╼ Sonnet 5 | ─── 4% (smart) | 12s | +1/-0")
	if st == nil {
		t.Fatal("expected a parsed status")
	}
	if st.Project != "OpenPaw" {
		t.Errorf("Project = %q, want OpenPaw", st.Project)
	}
	if st.Branch != "feature/pins" {
		t.Errorf("Branch = %q, want feature/pins", st.Branch)
	}
	if st.Uncommitted != 0 {
		t.Errorf("Uncommitted = %d, want 0", st.Uncommitted)
	}
	if st.ContextPct != 4 {
		t.Errorf("ContextPct = %d, want 4", st.ContextPct)
	}
}

// A plain shell has no status block; callers rely on nil to fall back to the tail.
func TestParseStatus_PlainShell(t *testing.T) {
	if st := ParseStatus("$ ls -la\ntotal 8\ndrwxr-xr-x  4 user  staff  128 Jul 25 01:00 ."); st != nil {
		t.Fatalf("expected nil for a plain shell pane, got %+v", st)
	}
}

func TestDetectKind(t *testing.T) {
	if got := detectKind(claudePane); got != "claude" {
		t.Errorf("claude pane detected as %q", got)
	}
	if got := detectKind("$ echo hi\nhi"); got != "shell" {
		t.Errorf("shell pane detected as %q", got)
	}
}

func TestTail_DropsBlankLinesAndCaps(t *testing.T) {
	got := tail("a\n\n\nb\nc\n\nd\n", 2)
	if len(got) != 2 || got[0] != "c" || got[1] != "d" {
		t.Fatalf("tail = %#v, want [c d]", got)
	}
}
