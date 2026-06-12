package llm

import "testing"

func TestTierForModel(t *testing.T) {
	cases := []struct {
		name     string
		fallback string
		want     string
	}{
		{"haiku", "", "haiku"},
		{"sonnet", "", "sonnet"},
		{"opus", "", "opus"},
		{"fable", "", "fable"},
		{"anthropic/claude-haiku-4-5", "", "haiku"},
		{"anthropic/claude-sonnet-4-6", "", "sonnet"},
		{"anthropic/claude-opus-4-6", "", "opus"},
		{"anthropic/claude-fable-5", "", "fable"},
		{"", ModelFable, "fable"},
		{"", ModelHaiku, "haiku"},
		{"", ModelSonnet, "sonnet"},
		{"", "", "sonnet"},
		{"gpt-5.1-codex", ModelHaiku, "haiku"},
		{"google/gemini-2.0-flash-001", ModelSonnet, "sonnet"},
		{"openrouter/auto", "", "sonnet"},
	}
	for _, c := range cases {
		if got := TierForModel(c.name, c.fallback); got != c.want {
			t.Errorf("TierForModel(%q, %q) = %q, want %q", c.name, c.fallback, got, c.want)
		}
	}
}

func TestContextWindowForModel(t *testing.T) {
	cases := []struct {
		model string
		want  int
	}{
		// CLI tier names
		{"haiku", 200_000},
		{"sonnet", 200_000},
		{"fable", 1_000_000},
		// OpenPaw's dash-style OpenRouter IDs (static fallback, no cache)
		{ModelHaiku, 200_000},
		{ModelSonnet, 1_000_000},
		{ModelOpus, 1_000_000},
		{ModelFable, 1_000_000},
		// Codex models
		{"gpt-5.4-mini", 400_000},
		{"gpt-5.4", 1_050_000},
		{"gpt-5.5", 1_050_000},
		// Unknown falls back to 200k
		{"some/unknown-model", 200_000},
		{"", 200_000},
	}
	for _, c := range cases {
		if got := ContextWindowForModel(c.model); got != c.want {
			t.Errorf("ContextWindowForModel(%q) = %d, want %d", c.model, got, c.want)
		}
	}
}

func TestContextWindowDottedCacheLookup(t *testing.T) {
	// Simulate a populated OpenRouter cache keyed by the canonical dotted ID;
	// the dash-style constant must still resolve to it.
	globalModelCache.update([]ModelInfo{
		{ID: "anthropic/claude-haiku-4.5", ContextLength: 123_456},
	})
	defer globalModelCache.update(nil)

	if got := ContextWindowForModel("anthropic/claude-haiku-4-5"); got != 123_456 {
		t.Errorf("dash→dot cache lookup = %d, want 123456", got)
	}
}

func TestProviderRouter(t *testing.T) {
	client := NewClient("test-key")
	router := NewProviderRouter(client)

	if router.ActiveName() != ProviderOpenRouter {
		t.Fatalf("default active = %q, want openrouter", router.ActiveName())
	}
	if router.Active() != Provider(client) {
		t.Fatal("default active provider should be the OpenRouter client")
	}
	if err := router.SetActive("nonexistent"); err == nil {
		t.Fatal("SetActive with unknown provider should error")
	}
	if router.OpenRouter() != client {
		t.Fatal("OpenRouter() should always return the client")
	}
}
