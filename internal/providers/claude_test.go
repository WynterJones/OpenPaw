package providers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llm "github.com/openpaw/openpaw/internal/llm"
)

// fakeCLI writes an executable script that prints the given JSONL and returns
// its path. The script also responds to --version for the probe.
func fakeCLI(t *testing.T, jsonl string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-cli")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 1.0.0-test; exit 0; fi\ncat <<'EOF'\n" + jsonl + "\nEOF\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

const claudeFixture = `{"type":"system","subtype":"init","session_id":"sess-123"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Let me check."},{"type":"tool_use","id":"tu_1","name":"Bash","input":{"command":"ls"}}]}}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_1","content":"file.txt"}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done — found file.txt"}]}}
{"type":"result","subtype":"success","is_error":false,"result":"Done — found file.txt","session_id":"sess-123","num_turns":2,"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":20,"cache_creation_input_tokens":5}}`

func TestClaudeProviderRunAgentLoop(t *testing.T) {
	p := NewClaudeProvider(nil, nil)
	p.binName = fakeCLI(t, claudeFixture)

	var events []llm.StreamEvent
	cfg := llm.AgentConfig{
		Model:    "anthropic/claude-sonnet-4-6",
		MaxTurns: 10,
		OnEvent:  func(ev llm.StreamEvent) { events = append(events, ev) },
	}

	result, err := p.RunAgentLoop(context.Background(), cfg, "list files")
	if err != nil {
		t.Fatalf("RunAgentLoop: %v", err)
	}

	want := "Let me check.\n\nDone — found file.txt"
	if result.Text != want {
		t.Errorf("Text = %q, want %q", result.Text, want)
	}
	if result.InputTokens != 125 { // input + cache read + cache create
		t.Errorf("InputTokens = %d, want 125", result.InputTokens)
	}
	if result.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", result.OutputTokens)
	}
	if result.NumTurns != 2 {
		t.Errorf("NumTurns = %d, want 2", result.NumTurns)
	}
	if result.StopReason != "stop" {
		t.Errorf("StopReason = %q, want stop", result.StopReason)
	}
	if result.TotalCostUSD != 0 {
		t.Errorf("TotalCostUSD = %f, want 0 (subscription)", result.TotalCostUSD)
	}

	types := map[string]int{}
	for _, ev := range events {
		types[ev.Type]++
	}
	if types[llm.EventInit] != 1 || types[llm.EventTextDelta] != 2 ||
		types[llm.EventToolStart] != 1 || types[llm.EventToolEnd] != 1 ||
		types[llm.EventResult] != 1 {
		t.Errorf("unexpected event counts: %v", types)
	}

	for _, ev := range events {
		if ev.Type == llm.EventToolEnd {
			if ev.ToolName != "Bash" || ev.ToolOutput != "file.txt" {
				t.Errorf("tool end = %+v", ev)
			}
		}
	}
}

func TestClaudeProviderMaxTurnsStop(t *testing.T) {
	fixture := `{"type":"system","subtype":"init","session_id":"s1"}
{"type":"result","subtype":"error_max_turns","is_error":false,"result":"partial","session_id":"s1","num_turns":5}`
	p := NewClaudeProvider(nil, nil)
	p.binName = fakeCLI(t, fixture)

	result, err := p.RunAgentLoop(context.Background(), llm.AgentConfig{}, "hi")
	if err != nil {
		t.Fatalf("RunAgentLoop: %v", err)
	}
	if result.StopReason != "max_turns" {
		t.Errorf("StopReason = %q, want max_turns", result.StopReason)
	}
	if result.Text != "partial" {
		t.Errorf("Text = %q, want fallback to result field", result.Text)
	}
}

func TestClaudeProviderRunOneShot(t *testing.T) {
	fixture := `{"type":"result","subtype":"success","is_error":false,"result":"A short title","usage":{"input_tokens":10,"output_tokens":4}}`
	p := NewClaudeProvider(nil, nil)
	p.binName = fakeCLI(t, fixture)

	text, usage, err := p.RunOneShot(context.Background(), "haiku", "", "title please")
	if err != nil {
		t.Fatalf("RunOneShot: %v", err)
	}
	if text != "A short title" {
		t.Errorf("text = %q", text)
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 4 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestClaudeProviderNotInstalled(t *testing.T) {
	p := NewClaudeProvider(nil, nil)
	p.binName = "definitely-not-a-real-binary-xyz"

	if p.IsConfigured() {
		t.Fatal("IsConfigured should be false for missing binary")
	}
	if _, err := p.RunAgentLoop(context.Background(), llm.AgentConfig{}, "hi"); err == nil {
		t.Fatal("RunAgentLoop should error for missing binary")
	}
}

func TestBuildReplayPrompt(t *testing.T) {
	history := []llm.HistoryMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	out := buildReplayPrompt(history, "follow-up")
	for _, want := range []string{"hello", "hi there", "follow-up", "CONVERSATION SO FAR", "NEW MESSAGE"} {
		if !strings.Contains(out, want) {
			t.Errorf("replay prompt missing %q:\n%s", want, out)
		}
	}
	if got := buildReplayPrompt(nil, "solo"); got != "solo" {
		t.Errorf("empty history should pass message through, got %q", got)
	}
}
