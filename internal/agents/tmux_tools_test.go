package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	llm "github.com/openpaw/openpaw/internal/llm"
)

func TestBuildTmuxToolDefs_ShapeIsValid(t *testing.T) {
	defs := BuildTmuxToolDefs()
	if len(defs) != 9 {
		t.Fatalf("got %d tool defs, want 9", len(defs))
	}

	seen := map[string]bool{}
	for _, d := range defs {
		if d.Type != "function" {
			t.Errorf("%s: type = %q, want function", d.Function.Name, d.Type)
		}
		if d.Function.Description == "" {
			t.Errorf("%s: empty description", d.Function.Name)
		}
		// Providers reject a malformed schema outright, taking every other tool
		// on the run down with it.
		var schema map[string]interface{}
		if err := json.Unmarshal(d.Function.Parameters, &schema); err != nil {
			t.Errorf("%s: parameters are not valid JSON: %v", d.Function.Name, err)
		}
		if schema["type"] != "object" {
			t.Errorf("%s: schema type = %v, want object", d.Function.Name, schema["type"])
		}
		seen[d.Function.Name] = true
	}

	for _, want := range []string{
		"tmux_run", "tmux_list", "tmux_status", "tmux_logs", "tmux_send",
		"tmux_watch", "tmux_unwatch", "worktree_list", "worktree_remove",
	} {
		if !seen[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

// Every declared tool needs a handler behind it: a model that calls one with
// nothing wired up gets an error where it expected a result, and the tool it
// was told about is worse than no tool at all.
func TestMakeTmuxToolHandlers_CoversEveryDef(t *testing.T) {
	m := &Manager{}
	handlers := m.MakeTmuxToolHandlers("thread-1")

	for _, d := range BuildTmuxToolDefs() {
		if handlers[d.Function.Name] == nil {
			t.Errorf("tool %q is declared but has no handler", d.Function.Name)
		}
	}
	if len(handlers) != len(BuildTmuxToolDefs()) {
		t.Errorf("got %d handlers for %d tools", len(handlers), len(BuildTmuxToolDefs()))
	}
}

// The expensive habit this replaces is killing a working session to say one
// more thing to it, so the description has to rule that out explicitly.
func TestTmuxSendDef_SaysNotToRestartTheSession(t *testing.T) {
	desc := findToolDef(t, "tmux_send").Function.Description

	for _, want := range []string{"INSTEAD OF killing", "re-reads"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description does not mention %q:\n%s", want, desc)
		}
	}
}

// tmux_status returns the visible pane, which is where closing reports went to
// die. tmux_logs only earns its place if the agent knows that is what it's for.
func TestTmuxLogsDef_ExplainsWhatStatusMisses(t *testing.T) {
	desc := findToolDef(t, "tmux_logs").Function.Description

	if !strings.Contains(desc, "tmux_status") {
		t.Errorf("description does not contrast itself with tmux_status:\n%s", desc)
	}
	if !strings.Contains(strings.ToLower(desc), "report") {
		t.Errorf("description does not mention reading a finished agent's report:\n%s", desc)
	}
}

// Isolation is opt-in, so the model has to be told when opting in is required —
// otherwise it defaults to the shared tree, which is the race this prevents.
func TestTmuxRunDef_ExplainsWhenToIsolate(t *testing.T) {
	def := findToolDef(t, "tmux_run")

	var schema struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(def.Function.Parameters, &schema); err != nil {
		t.Fatalf("parameters are not valid JSON: %v", err)
	}

	worktree, ok := schema.Properties["worktree"]
	if !ok {
		t.Fatal("tmux_run has no worktree parameter")
	}
	if worktree.Type != "boolean" {
		t.Errorf("worktree type = %q, want boolean", worktree.Type)
	}
	if !strings.Contains(worktree.Description, "share a branch") {
		t.Errorf("worktree description does not say why a shared tree is unsafe:\n%s", worktree.Description)
	}
}

func findToolDef(t *testing.T, name string) llm.ToolDef {
	t.Helper()
	for _, d := range BuildTmuxToolDefs() {
		if d.Function.Name == name {
			return d
		}
	}
	t.Fatalf("no tool named %q", name)
	return llm.ToolDef{}
}

// The whole point of the tool is to stop agents promising to check back when
// they cannot, so the description has to say so.
func TestTmuxWatchDef_TellsTheAgentItCannotSelfCheck(t *testing.T) {
	desc := buildTmuxWatchDef().Function.Description
	for _, want := range []string{"turn ends", "do not", "promise"} {
		if !strings.Contains(strings.ToLower(desc), want) {
			t.Errorf("tmux_watch description missing %q: %s", want, desc)
		}
	}
}

func TestHandleTmuxWatch_RequiresSession(t *testing.T) {
	m := &Manager{}
	res := m.handleTmuxWatch("thread-1")(context.Background(), "", json.RawMessage(`{}`))
	if !res.IsError {
		t.Fatalf("expected an error result, got %+v", res)
	}
}

// A threadless run (a scheduled report) has nowhere to post an update, so the
// tool must say so rather than starting a watch that reports into the void.
func TestHandleTmuxWatch_RejectsThreadlessRun(t *testing.T) {
	called := false
	m := &Manager{TmuxWatchFn: func(string, string, int) error { called = true; return nil }}

	res := m.handleTmuxWatch("")(context.Background(), "", json.RawMessage(`{"session":"build"}`))
	if !res.IsError {
		t.Errorf("expected an error result, got %+v", res)
	}
	if called {
		t.Error("a watch was started for a run with no thread")
	}
}

func TestHandleTmuxWatch_UnavailableWhenNotWired(t *testing.T) {
	m := &Manager{}
	res := m.handleTmuxWatch("thread-1")(context.Background(), "", json.RawMessage(`{"session":"build"}`))
	if !res.IsError {
		t.Errorf("expected an error result when TmuxWatchFn is nil, got %+v", res)
	}
}

func TestHandleTmuxUnwatch_ReportsCount(t *testing.T) {
	cases := []struct {
		stopped int
		want    string
	}{
		{0, "no active watches"},
		{1, "Stopped 1 watch."},
		{3, "Stopped 3 watches."},
	}
	for _, c := range cases {
		m := &Manager{TmuxUnwatchFn: func(string, string) int { return c.stopped }}
		res := m.handleTmuxUnwatch("thread-1")(context.Background(), "", json.RawMessage(`{}`))
		if !strings.Contains(res.Output, c.want) {
			t.Errorf("stopped=%d output = %q, want it to contain %q", c.stopped, res.Output, c.want)
		}
	}
}

func TestHandleTmuxUnwatch_NoThreadIsNotAnError(t *testing.T) {
	m := &Manager{TmuxUnwatchFn: func(string, string) int { return 0 }}
	res := m.handleTmuxUnwatch("")(context.Background(), "", json.RawMessage(`{}`))
	if res.IsError {
		t.Errorf("unwatch with no thread should be a no-op, got %+v", res)
	}
}

func TestHandleTmuxRun_RequiresCommand(t *testing.T) {
	m := &Manager{}
	res := m.handleTmuxRun("thread-1")(context.Background(), "", json.RawMessage(`{}`))
	if !res.IsError {
		t.Fatalf("expected an error result, got %+v", res)
	}
}

// The default has to be "run it detached and tell me when it's done" — an
// unwatched session is work the agent starts and then forgets about.
func TestTmuxRunDef_DefaultsToWatching(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Default interface{} `json:"default"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(buildTmuxRunDef().Function.Parameters, &schema); err != nil {
		t.Fatalf("parameters are not valid JSON: %v", err)
	}
	if got := schema.Properties["watch"].Default; got != true {
		t.Errorf("watch default = %v, want true", got)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "command" {
		t.Errorf("required = %v, want [command]", schema.Required)
	}
}

// The description is what actually steers the choice at call time, so it has to
// say to prefer tmux over an inline run.
func TestTmuxRunDef_TellsTheAgentToPreferIt(t *testing.T) {
	desc := strings.ToLower(buildTmuxRunDef().Function.Description)
	for _, want := range []string{"prefer this", "turn ends", "inline"} {
		if !strings.Contains(desc, want) {
			t.Errorf("tmux_run description missing %q: %s", want, desc)
		}
	}
}

func TestFirstWords_BuildsReadableSessionName(t *testing.T) {
	if got := firstWords("npm run build --workspace web", 4); got != "npm-run-build---workspace" {
		t.Errorf("firstWords = %q", got)
	}
	if got := firstWords("ls", 4); got != "ls" {
		t.Errorf("firstWords = %q, want ls", got)
	}
}

// The prompt directive is only injected for CLI engines, so it has to name the
// tool and the reason inline runs die.
func TestTmuxPromptSection_NamesToolAndReason(t *testing.T) {
	s := buildTmuxPromptSection()
	for _, want := range []string{"tmux_run", "turn ends", "builds"} {
		if !strings.Contains(s, want) {
			t.Errorf("prompt section missing %q", want)
		}
	}
}
