package agents

import (
	"context"
	"testing"

	llm "github.com/openpaw/openpaw/internal/llm"
)

type agentProviderStub struct {
	name       string
	configured bool
}

func (p *agentProviderStub) Name() string                       { return p.name }
func (p *agentProviderStub) IsConfigured() bool                 { return p.configured }
func (p *agentProviderStub) ResolveModel(name, _ string) string { return name }
func (p *agentProviderStub) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}
func (p *agentProviderStub) RunAgentLoop(context.Context, llm.AgentConfig, string) (*llm.AgentResult, error) {
	return &llm.AgentResult{}, nil
}
func (p *agentProviderStub) RunOneShot(context.Context, string, string, string) (string, *llm.UsageInfo, error) {
	return "", nil, nil
}

func TestProviderForSelectsAgentEngineWithoutChangingGlobalDefault(t *testing.T) {
	openrouter := llm.NewClient("")
	router := llm.NewProviderRouter(openrouter)
	claude := &agentProviderStub{name: llm.ProviderClaudeCode, configured: true}
	codex := &agentProviderStub{name: llm.ProviderCodex, configured: true}
	router.Register(claude)
	router.Register(codex)

	manager := &Manager{client: openrouter, Providers: router}
	if got := manager.ProviderFor(llm.ProviderClaudeCode); got != claude {
		t.Fatalf("Claude agent got %T, want configured Claude provider", got)
	}
	if got := manager.ProviderFor(llm.ProviderCodex); got != codex {
		t.Fatalf("Codex agent got %T, want configured Codex provider", got)
	}
	if router.ActiveName() != llm.ProviderOpenRouter {
		t.Fatalf("agent selection changed global provider to %q", router.ActiveName())
	}
}

func TestProviderForFallsBackWhenAgentEngineUnavailable(t *testing.T) {
	openrouter := llm.NewClient("")
	router := llm.NewProviderRouter(openrouter)
	router.Register(&agentProviderStub{name: llm.ProviderCodex, configured: false})
	manager := &Manager{client: openrouter, Providers: router}

	if got := manager.ProviderFor(llm.ProviderCodex); got != openrouter {
		t.Fatalf("unavailable agent provider did not fall back to active provider")
	}
	if got := manager.ProviderFor(""); got != openrouter {
		t.Fatalf("blank agent provider did not inherit active provider")
	}
}
