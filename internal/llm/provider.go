package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

const (
	ProviderOpenRouter = "openrouter"
	ProviderClaudeCode = "claude-code"
	ProviderCodex      = "codex"
)

// Provider abstracts an LLM backend. OpenRouter (the *Client) is the default;
// CLI-based providers (Claude Code, Codex) run inference through local
// subscription-authenticated binaries.
type Provider interface {
	Name() string
	IsConfigured() bool
	RunAgentLoop(ctx context.Context, cfg AgentConfig, userMessage string) (*AgentResult, error)
	RunOneShot(ctx context.Context, model, system, prompt string) (string, *UsageInfo, error)
	// ResolveModel translates a stored model string (legacy short name, full
	// OpenRouter ID, or provider-native ID) into this provider's native model
	// argument. fallback is an OpenRouter-style model ID (e.g. llm.ModelHaiku).
	ResolveModel(name, fallback string) string
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// SessionKey identifies a chat thread + agent pair for CLI providers that
// support native session resume. Nil means always run a fresh session.
type SessionKey struct {
	ThreadID  string
	AgentSlug string
}

// SessionStore persists CLI provider session IDs per thread+agent.
type SessionStore interface {
	GetProviderSession(threadID, agentSlug, provider string) string
	PutProviderSession(threadID, agentSlug, provider, sessionID string)
	DeleteProviderSession(threadID, agentSlug, provider string)
}

// Client (OpenRouter) implements Provider.

func (c *Client) Name() string { return ProviderOpenRouter }

func (c *Client) ResolveModel(name, fallback string) string {
	return ResolveModel(name, fallback)
}

func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return c.GetCachedModels(ctx)
}

// TierForModel maps any stored model string to a canonical cross-provider
// tier: "haiku", "sonnet", or "opus". Unknown names fall back to the tier of
// the fallback model ID, defaulting to sonnet.
func TierForModel(name, fallback string) string {
	if tier := tierOf(name); tier != "" {
		return tier
	}
	if tier := tierOf(fallback); tier != "" {
		return tier
	}
	return "sonnet"
}

func tierOf(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case n == "":
		return ""
	case strings.Contains(n, "haiku"):
		return "haiku"
	case strings.Contains(n, "fable"), strings.Contains(n, "mythos"):
		return "fable"
	case strings.Contains(n, "opus"):
		return "opus"
	case strings.Contains(n, "sonnet"):
		return "sonnet"
	}
	return ""
}

// ProviderRouter holds all registered providers and the active selection.
// The OpenRouter client is always reachable (image generation, balance)
// regardless of which provider is active.
type ProviderRouter struct {
	mu         sync.RWMutex
	active     string
	openrouter *Client
	providers  map[string]Provider
}

func NewProviderRouter(openrouter *Client) *ProviderRouter {
	r := &ProviderRouter{
		active:     ProviderOpenRouter,
		openrouter: openrouter,
		providers:  map[string]Provider{ProviderOpenRouter: openrouter},
	}
	return r
}

func (r *ProviderRouter) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

func (r *ProviderRouter) Active() Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.providers[r.active]; ok {
		return p
	}
	return r.openrouter
}

func (r *ProviderRouter) ActiveName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

func (r *ProviderRouter) Get(name string) Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[name]
}

func (r *ProviderRouter) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

func (r *ProviderRouter) SetActive(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[name]; !ok {
		return fmt.Errorf("unknown provider %q", name)
	}
	r.active = name
	return nil
}

func (r *ProviderRouter) OpenRouter() *Client {
	return r.openrouter
}
