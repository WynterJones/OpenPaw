package tmux

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

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

func TestSessionName_StripsTmuxSeparators(t *testing.T) {
	cases := map[string]string{
		"npm run build":   "npm-run-build",
		"go test ./...":   "go-test",
		"deploy:prod":     "deploy-prod",
		"  spaced  out  ": "spaced--out",
		"!!!":             "op-task",
		"":                "op-task",
	}
	for in, want := range cases {
		if got := SessionName(in); got != want {
			t.Errorf("SessionName(%q) = %q, want %q", in, got, want)
		}
	}
	// tmux targets break on "." and ":", so they must never survive.
	for _, in := range []string{"a.b", "a:b", "a/b"} {
		if got := SessionName(in); strings.ContainsAny(got, ".:/") {
			t.Errorf("SessionName(%q) = %q, still contains a separator", in, got)
		}
	}
}

// Exercises the real tmux binary: a detached session must actually start, run
// its command, and still be inspectable afterwards.
func TestStart_RunsDetachedAndSurvivesTheCommand(t *testing.T) {
	if !Available() {
		t.Skip("tmux not installed")
	}
	ctx := context.Background()
	name := "openpaw-test-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	t.Cleanup(func() { run(ctx, "kill-session", "-t", name) })

	// echo is the hard case: it finishes almost instantly, so any setup that
	// races the command loses the session before the output can be read.
	if err := Start(ctx, name, t.TempDir(), "echo hello-from-tmux"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for the command to finish, then assert the session outlived it.
	var pane string
	for i := 0; i < 100; i++ {
		if dead, _, ok := Finished(ctx, name); ok && dead {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !Exists(ctx, name) {
		t.Fatal("session vanished once its command exited — output and status are lost")
	}
	dead, status, ok := Finished(ctx, name)
	if !ok || !dead {
		t.Fatalf("Finished = (dead %v, ok %v), want a dead pane", dead, ok)
	}
	if status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}

	pane, _ = Capture(ctx, name)
	if !strings.Contains(pane, "hello-from-tmux") {
		t.Fatalf("command output never appeared in pane: %q", pane)
	}

	// A second Start on the same name must not silently hijack the first.
	if err := Start(ctx, name, "", "echo again"); err == nil {
		t.Error("Start on an existing session should fail")
	}
}

func TestStart_RejectsEmptyInput(t *testing.T) {
	if !Available() {
		t.Skip("tmux not installed")
	}
	ctx := context.Background()
	if err := Start(ctx, "", "", "echo hi"); err == nil {
		t.Error("expected an error for an empty session name")
	}
	if err := Start(ctx, "openpaw-test-noop", "", "   "); err == nil {
		t.Error("expected an error for an empty command")
	}
}

// The watcher decides "done" from this, so a non-zero exit has to come back
// intact rather than as a bare "it stopped".
func TestFinished_ReportsExitStatus(t *testing.T) {
	if !Available() {
		t.Skip("tmux not installed")
	}
	ctx := context.Background()
	name := "openpaw-test-exit-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	t.Cleanup(func() { run(ctx, "kill-session", "-t", name) })

	if err := Start(ctx, name, t.TempDir(), "exit 3"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var dead, ok bool
	var status int
	for i := 0; i < 60; i++ {
		if dead, status, ok = Finished(ctx, name); ok && dead {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ok || !dead {
		t.Fatalf("Finished = (dead %v, ok %v), want a dead pane", dead, ok)
	}
	if status != 3 {
		t.Errorf("exit status = %d, want 3", status)
	}
}

// A live session must not read as finished, or every watch would report
// completion on its first check.
func TestFinished_LiveSessionIsNotDead(t *testing.T) {
	if !Available() {
		t.Skip("tmux not installed")
	}
	ctx := context.Background()
	name := "openpaw-test-live-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	t.Cleanup(func() { run(ctx, "kill-session", "-t", name) })

	if err := Start(ctx, name, t.TempDir(), "sleep 30"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if dead, _, ok := Finished(ctx, name); !ok || dead {
		t.Errorf("Finished = (dead %v, ok %v), want a live pane", dead, ok)
	}
}

// Regression: the field separator used to be \x1f, which tmux's format
// expansion rewrites to a literal "_". Every line then failed to split into
// four parts and was skipped, so List returned nothing while sessions were
// plainly running — and tmux_list told agents there was no work in flight.
func TestList_FindsARunningSession(t *testing.T) {
	if !Available() {
		t.Skip("tmux not installed")
	}
	ctx := context.Background()
	name := "openpaw-test-list-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	t.Cleanup(func() { run(ctx, "kill-session", "-t", name) })

	if err := Start(ctx, name, t.TempDir(), "sleep 30"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	sessions, err := List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, s := range sessions {
		if s.Name != name {
			continue
		}
		if s.Windows != 1 {
			t.Errorf("Windows = %d, want 1", s.Windows)
		}
		if s.Created.IsZero() {
			t.Error("Created was not parsed")
		}
		if s.Attached {
			t.Error("a detached session reported as attached")
		}
		return
	}
	t.Fatalf("session %q missing from List (%d sessions returned)", name, len(sessions))
}
