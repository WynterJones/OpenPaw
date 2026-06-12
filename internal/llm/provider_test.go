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
