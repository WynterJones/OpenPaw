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

type codexTestSessionStore struct {
	sessionID string
	deletes   int
	puts      int
}

func (s *codexTestSessionStore) GetProviderSession(_, _, _ string) string {
	return s.sessionID
}

func (s *codexTestSessionStore) PutProviderSession(_, _, _, sessionID string) {
	s.sessionID = sessionID
	s.puts++
}

func (s *codexTestSessionStore) DeleteProviderSession(_, _, _ string) {
	s.sessionID = ""
	s.deletes++
}

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

func TestCodexToolTurnsReplayInsteadOfResuming(t *testing.T) {
	store := &codexTestSessionStore{sessionID: "expired-mcp-binding"}
	p := NewCodexProvider(store, nil)
	p.binName = fakeCLI(t, codexFixture)
	p.probe.loggedIn = true

	_, err := p.RunAgentLoop(context.Background(), llm.AgentConfig{
		Session:       &llm.SessionKey{ThreadID: "thread-1", AgentSlug: "builder"},
		ExtraHandlers: map[string]llm.ToolHandler{"tmux_run": nil},
	}, "continue")
	if err != nil {
		t.Fatalf("RunAgentLoop: %v", err)
	}
	if store.deletes != 1 {
		t.Errorf("provider session deletes = %d, want 1", store.deletes)
	}
	if store.puts != 0 || store.sessionID != "" {
		t.Errorf("tool turn persisted an unsafe Codex resume binding: puts=%d session=%q", store.puts, store.sessionID)
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
		{"haiku", "", "gpt-5.6-luna"},
		{"anthropic/claude-sonnet-4-6", "", "gpt-5.6-terra"},
		{"opus", "", "gpt-5.6-sol"},
		{"fable", "", "gpt-5.6-sol"},
		{"anthropic/claude-fable-5", "", "gpt-5.6-sol"},
		{"gpt-5.6-sol", "", "gpt-5.6-sol"},
		{"gpt-5.6-terra", "", "gpt-5.6-terra"},
		{"gpt-5.6-luna", "", "gpt-5.6-luna"},
		{"gpt-5.4", "", "gpt-5.4"},
		{"gpt-5.5", "", "gpt-5.5"},
		{"", llm.ModelHaiku, "gpt-5.6-luna"},
	}
	for _, c := range cases {
		if got := p.ResolveModel(c.in, c.fallback); got != c.want {
			t.Errorf("ResolveModel(%q, %q) = %q, want %q", c.in, c.fallback, got, c.want)
		}
	}
}

func TestCodexListModelsIncludesCurrentAndLegacyOptions(t *testing.T) {
	p := NewCodexProvider(nil, nil)
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}

	ids := make(map[string]bool, len(models))
	for _, model := range models {
		ids[model.ID] = true
	}
	for _, want := range []string{
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.4-mini",
	} {
		if !ids[want] {
			t.Errorf("ListModels missing %q", want)
		}
	}
}

func TestCodexBuildArgsUsesResumeCompatiblePolicy(t *testing.T) {
	p := NewCodexProvider(nil, nil)
	cfg := llm.AgentConfig{
		Model:        "sonnet",
		SandboxPaths: []string{"/workspace"},
	}

	args := p.buildArgs(cfg, "thread-resume-123", "http://127.0.0.1:41295/api/v1/mcp/token")
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"exec resume thread-resume-123",
		`sandbox_mode="workspace-write"`,
		`approval_policy="never"`,
		`sandbox_workspace_write.network_access=true`,
		`mcp_servers.openpaw.url="http://127.0.0.1:41295/api/v1/mcp/token"`,
		`mcp_servers.openpaw.default_tools_approval_mode="approve"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("resume args missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "--sandbox") {
		t.Errorf("resume args contain unsupported --sandbox flag:\n%s", joined)
	}
}

func TestCodexBuildArgsAllowsFullAccessForUnsandboxedAgents(t *testing.T) {
	p := NewCodexProvider(nil, nil)
	args := p.buildArgs(llm.AgentConfig{}, "", "")
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, `sandbox_mode="danger-full-access"`) {
		t.Errorf("fresh unsandboxed args do not grant full access:\n%s", joined)
	}
	if !strings.Contains(joined, `approval_policy="never"`) {
		t.Errorf("fresh args do not disable headless approval prompts:\n%s", joined)
	}
}
