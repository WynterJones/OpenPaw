package providers

import (
	"context"
	"strings"
	"testing"

	llm "github.com/openpaw/openpaw/internal/llm"
)

const codexFixture = `{"type":"thread.started","thread_id":"thread-abc"}
{"type":"turn.started"}
{"type":"item.started","item":{"id":"item_0","item_type":"command_execution","command":"ls","status":"in_progress"}}
{"type":"item.completed","item":{"id":"item_0","item_type":"command_execution","command":"ls","aggregated_output":"file.txt","exit_code":0,"status":"completed"}}
{"type":"item.completed","item":{"id":"item_1","item_type":"reasoning","text":"thinking..."}}
{"type":"item.completed","item":{"id":"item_2","item_type":"agent_message","text":"Found file.txt"}}
{"type":"turn.completed","usage":{"input_tokens":80,"cached_input_tokens":10,"output_tokens":30}}`

func TestCodexProviderRunAgentLoop(t *testing.T) {
	p := NewCodexProvider(nil, nil)
	p.binName = fakeCLI(t, codexFixture)
	p.probe.loggedIn = true

	var events []llm.StreamEvent
	cfg := llm.AgentConfig{
		Model:   "sonnet",
		OnEvent: func(ev llm.StreamEvent) { events = append(events, ev) },
	}

	result, err := p.RunAgentLoop(context.Background(), cfg, "list files")
	if err != nil {
		t.Fatalf("RunAgentLoop: %v", err)
	}

	if result.Text != "Found file.txt" {
		t.Errorf("Text = %q", result.Text)
	}
	if result.InputTokens != 90 || result.OutputTokens != 30 {
		t.Errorf("tokens = %d/%d, want 90/30", result.InputTokens, result.OutputTokens)
	}
	if result.NumTurns != 1 {
		t.Errorf("NumTurns = %d, want 1", result.NumTurns)
	}

	types := map[string]int{}
	for _, ev := range events {
		types[ev.Type]++
	}
	if types[llm.EventInit] != 1 || types[llm.EventTextDelta] != 1 ||
		types[llm.EventToolStart] != 1 || types[llm.EventToolEnd] != 1 ||
		types[llm.EventResult] != 1 {
		t.Errorf("unexpected event counts: %v", types)
	}
}

func TestCodexProviderFailureEvent(t *testing.T) {
	fixture := `{"type":"thread.started","thread_id":"t1"}
{"type":"turn.failed","error":{"message":"model overloaded"}}`
	p := NewCodexProvider(nil, nil)
	p.binName = fakeCLI(t, fixture)

	_, err := p.RunAgentLoop(context.Background(), llm.AgentConfig{}, "hi")
	if err == nil || !strings.Contains(err.Error(), "model overloaded") {
		t.Errorf("expected turn.failed error, got %v", err)
	}
}

func TestCodexResolveModel(t *testing.T) {
	p := NewCodexProvider(nil, nil)
	cases := []struct{ in, fallback, want string }{
		{"haiku", "", "gpt-5.4-mini"},
		{"anthropic/claude-sonnet-4-6", "", "gpt-5.4"},
		{"opus", "", "gpt-5.5"},
		{"fable", "", "gpt-5.5"},
		{"anthropic/claude-fable-5", "", "gpt-5.5"},
		{"gpt-5.4", "", "gpt-5.4"},
		{"gpt-5.5", "", "gpt-5.5"},
		{"", llm.ModelHaiku, "gpt-5.4-mini"},
	}
	for _, c := range cases {
		if got := p.ResolveModel(c.in, c.fallback); got != c.want {
			t.Errorf("ResolveModel(%q, %q) = %q, want %q", c.in, c.fallback, got, c.want)
		}
	}
}
