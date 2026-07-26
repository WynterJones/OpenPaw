package handlers

import (
	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
)

// Per-provider model selection.
//
// A model id only means something to the provider it came from:
// "anthropic/claude-sonnet-5" is an OpenRouter route, "sonnet" is a Claude Code
// tier, "gpt-5.5" is a Codex model. One shared setting meant switching provider
// carried the previous provider's id across, where it was silently coerced to
// that provider's default — so picking a model, switching away, and switching
// back quietly lost the choice. Each provider now remembers its own.

// providerModelKey is the settings key holding a provider's chosen model.
// The legacy un-suffixed keys are kept as OpenRouter's, so existing installs
// keep the model they already had.
func providerModelKey(role, provider string) string {
	if provider == llm.ProviderOpenRouter || provider == "" {
		return role // "gateway_model" / "builder_model"
	}
	return role + "_" + provider
}

// ProviderModels is the pair of models a provider runs with.
type ProviderModels struct {
	Gateway string `json:"gateway_model"`
	Builder string `json:"builder_model"`
}

// LoadProviderModels reads a provider's saved models, falling back to that
// provider's own defaults when nothing has been chosen yet. Returning the
// provider's default rather than a global one keeps CLI providers on their tier
// names instead of an OpenRouter route they cannot use.
func LoadProviderModels(db *database.DB, providers *llm.ProviderRouter, provider string) ProviderModels {
	var out ProviderModels
	db.QueryRow("SELECT value FROM settings WHERE key = ?", providerModelKey("gateway_model", provider)).Scan(&out.Gateway)
	db.QueryRow("SELECT value FROM settings WHERE key = ?", providerModelKey("builder_model", provider)).Scan(&out.Builder)

	gwFallback, bldFallback := defaultModelsFor(provider)
	if out.Gateway == "" {
		out.Gateway = gwFallback
	}
	if out.Builder == "" {
		out.Builder = bldFallback
	}

	// Resolve through the provider so a stale or foreign id becomes something
	// it can actually run, rather than failing at call time.
	if providers != nil {
		if p := providers.Get(provider); p != nil {
			out.Gateway = p.ResolveModel(out.Gateway, gwFallback)
			out.Builder = p.ResolveModel(out.Builder, bldFallback)
		}
	}
	return out
}

// defaultModelsFor is the sensible starting pair for a provider. Both roles get
// the mid tier: the gateway does routing reasoning on every message, and the
// cheapest tier was measurably worse at it.
func defaultModelsFor(provider string) (gateway, builder string) {
	switch provider {
	case "claude-code":
		return "sonnet", "sonnet"
	case "codex":
		return "gpt-5.5", "gpt-5.5"
	default:
		return llm.ModelSonnet, llm.ModelSonnet
	}
}

// ApplyProviderModels points the agent manager at a provider's models. Called
// on startup and whenever the active provider changes, so a switch takes effect
// without a restart.
func ApplyProviderModels(db *database.DB, providers *llm.ProviderRouter, provider string, set func(gateway, builder string)) ProviderModels {
	m := LoadProviderModels(db, providers, provider)
	if set != nil {
		set(m.Gateway, m.Builder)
	}
	return m
}
