package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	ModelHaiku  = "anthropic/claude-haiku-4.5"
	ModelSonnet = "anthropic/claude-sonnet-5"
	ModelOpus   = "anthropic/claude-opus-5"
	ModelFable  = "anthropic/claude-fable-5"
)

// CLIContextWindow is the session context window for the CLI subscription
// providers (Claude Code / Codex) with the 1M-context capability — matches the
// frontend's CLI_CONTEXT_LIMIT so the usage bar and auto-compaction agree.
const CLIContextWindow = 1_000_000

// Legacy short name -> OpenRouter model ID mapping
var legacyModels = map[string]string{
	"haiku":  ModelHaiku,
	"sonnet": ModelSonnet,
	"opus":   ModelOpus,
	"fable":  ModelFable,
}

func ResolveModel(name string, fallback string) string {
	if name == "" {
		return fallback
	}
	if mapped, ok := legacyModels[name]; ok {
		return mapped
	}
	if name == "auto" {
		return "openrouter/auto"
	}
	// If it contains a slash, treat as a full OpenRouter model ID
	if strings.Contains(name, "/") {
		return name
	}
	return fallback
}

// staticContextWindows covers models the OpenRouter cache can't resolve:
// CLI provider tier names and OpenPaw's dash-style Anthropic IDs (OpenRouter's
// canonical IDs use dots). Values verified against OpenRouter metadata.
var staticContextWindows = map[string]int{
	// Claude Code CLI tiers (standard subscription session window; fable is 1M)
	"haiku":  200_000,
	"sonnet": 200_000,
	"opus":   200_000,
	"fable":  1_000_000,
	// OpenPaw's OpenRouter model IDs (API-served windows)
	ModelHaiku:  200_000,
	ModelSonnet: 1_000_000,
	ModelOpus:   1_000_000,
	ModelFable:  1_000_000,
	// Codex CLI models (ChatGPT-account)
	"gpt-5.4-mini": 400_000,
	"gpt-5.4":      1_050_000,
	"gpt-5.5":      1_050_000,
}

// dashVersionRe matches a trailing dash-separated version ("-4-5") so it can
// be normalized to OpenRouter's dotted form ("-4.5") for cache lookups.
var dashVersionRe = regexp.MustCompile(`-(\d+)-(\d+)$`)

func ContextWindowForModel(model string) int {
	if cached := globalModelCache.get(model); cached != nil && cached.ContextLength > 0 {
		return cached.ContextLength
	}
	// OpenPaw historically uses dash-style Anthropic IDs (claude-haiku-4-5)
	// but OpenRouter's catalog keys are dotted (claude-haiku-4.5).
	if dotted := dashVersionRe.ReplaceAllString(model, "-$1.$2"); dotted != model {
		if cached := globalModelCache.get(dotted); cached != nil && cached.ContextLength > 0 {
			return cached.ContextLength
		}
	}
	if w, ok := staticContextWindows[strings.ToLower(model)]; ok {
		return w
	}
	return 200_000
}

func MaxTokensForModel(model string) int64 {
	switch model {
	case ModelOpus, ModelFable:
		return 32000
	case ModelSonnet:
		return 16000
	case ModelHaiku:
		return 8192
	default:
		if cached := globalModelCache.get(model); cached != nil {
			if cached.TopProvider.MaxCompletionTokens > 0 {
				return int64(cached.TopProvider.MaxCompletionTokens)
			}
		}
		return 8192
	}
}

// ModelInfo represents a model from the OpenRouter /models endpoint.
type ModelInfo struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	ContextLength int          `json:"context_length"`
	Pricing       ModelPricing `json:"pricing"`
	TopProvider   struct {
		MaxCompletionTokens int `json:"max_completion_tokens"`
	} `json:"top_provider"`
	Architecture struct {
		Modality         string   `json:"modality"`
		Tokenizer        string   `json:"tokenizer"`
		InstructType     string   `json:"instruct_type"`
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	Description string `json:"description"`
}

// EmitsImages reports whether the model can return generated images. Newer
// OpenRouter payloads say so in output_modalities; older ones only carry the
// combined modality string ("text+image->text+image"), so both are checked.
func (m ModelInfo) EmitsImages() bool {
	for _, mod := range m.Architecture.OutputModalities {
		if strings.EqualFold(mod, "image") {
			return true
		}
	}
	if idx := strings.Index(m.Architecture.Modality, "->"); idx >= 0 {
		return strings.Contains(m.Architecture.Modality[idx:], "image")
	}
	return false
}

// IsImageFirst reports whether images are the model's primary output. Purpose-
// built image models list "image" first in output_modalities; chat models that
// can also emit an image list "text" first. The distinction matters for picking
// a default — a text-first model asked for a picture often replies with words.
func (m ModelInfo) IsImageFirst() bool {
	if len(m.Architecture.OutputModalities) == 0 {
		return false
	}
	return strings.EqualFold(m.Architecture.OutputModalities[0], "image")
}

// AcceptsImageInput reports whether the model can take reference images
// alongside the prompt.
func (m ModelInfo) AcceptsImageInput() bool {
	for _, mod := range m.Architecture.InputModalities {
		if strings.EqualFold(mod, "image") {
			return true
		}
	}
	if idx := strings.Index(m.Architecture.Modality, "->"); idx > 0 {
		return strings.Contains(m.Architecture.Modality[:idx], "image")
	}
	return false
}

// IsRouter reports whether the id is one of OpenRouter's meta-models, which
// dispatch to some other model rather than generating anything themselves.
func (m ModelInfo) IsRouter() bool {
	return strings.HasPrefix(m.ID, "openrouter/auto")
}

type ModelPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
	Image      string `json:"image"`
}

// ModelCache holds cached model data from OpenRouter.
type ModelCache struct {
	mu        sync.RWMutex
	models    []ModelInfo
	byID      map[string]*ModelInfo
	fetchedAt time.Time
}

var globalModelCache = &ModelCache{
	byID: make(map[string]*ModelInfo),
}

func (mc *ModelCache) get(id string) *ModelInfo {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.byID[id]
}

func (mc *ModelCache) isStale() bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return time.Since(mc.fetchedAt) > 15*time.Minute || len(mc.models) == 0
}

func (mc *ModelCache) update(models []ModelInfo) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.models = models
	mc.byID = make(map[string]*ModelInfo, len(models))
	for i := range models {
		mc.byID[models[i].ID] = &models[i]
	}
	mc.fetchedAt = time.Now()
}

func (mc *ModelCache) all() []ModelInfo {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	out := make([]ModelInfo, len(mc.models))
	copy(out, mc.models)
	return out
}

// FetchModels retrieves available models from OpenRouter.
func FetchModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key required to fetch models")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/models?supported_parameters=tools", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("models endpoint returned %d", resp.StatusCode)
	}

	var result struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode models: %w", err)
	}

	globalModelCache.update(result.Data)
	return result.Data, nil
}

// GetCachedModels returns models from cache, fetching if stale.
func GetCachedModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	if !globalModelCache.isStale() {
		return globalModelCache.all(), nil
	}
	return FetchModels(ctx, apiKey)
}

// The catalog above is deliberately filtered to tool-capable models, because
// that is what the chat model picker should offer. Image-generation models
// don't support tools and are missing from it entirely, so Studio needs the
// unfiltered catalog and keeps it in its own cache.
var fullModelCache = &ModelCache{byID: make(map[string]*ModelInfo)}

// FetchAllModels retrieves the complete OpenRouter catalog, unfiltered.
func FetchAllModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key required to fetch models")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("models endpoint returned %d", resp.StatusCode)
	}

	var result struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode models: %w", err)
	}

	fullModelCache.update(result.Data)
	return result.Data, nil
}

// GetAllModels returns the full catalog from cache, fetching if stale.
func GetAllModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	if !fullModelCache.isStale() {
		return fullModelCache.all(), nil
	}
	return FetchAllModels(ctx, apiKey)
}

// GetImageModels returns catalog entries that can emit images, newest-looking
// first is not attempted — OpenRouter has no release date, so callers sort.
func GetImageModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	all, err := GetAllModels(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	var out []ModelInfo
	for _, m := range all {
		// Routers are excluded outright: "openrouter/auto" advertises image
		// output but decides at request time what to dispatch to, so asking it
		// for an image can come back as text — a bad thing to spend money on.
		if m.EmitsImages() && !m.IsRouter() {
			out = append(out, m)
		}
	}
	return out, nil
}
