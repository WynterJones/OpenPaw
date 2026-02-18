package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	ModelHaiku  = "anthropic/claude-haiku-4-5"
	ModelSonnet = "anthropic/claude-sonnet-4-6"
	ModelOpus   = "anthropic/claude-opus-4-6"
)

// Legacy short name -> OpenRouter model ID mapping
var legacyModels = map[string]string{
	"haiku":  ModelHaiku,
	"sonnet": ModelSonnet,
	"opus":   ModelOpus,
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

func ContextWindowForModel(model string) int {
	if cached := globalModelCache.get(model); cached != nil {
		return cached.ContextLength
	}
	return 200_000
}

func MaxTokensForModel(model string) int64 {
	switch model {
	case ModelOpus:
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
		Modality     string `json:"modality"`
		Tokenizer    string `json:"tokenizer"`
		InstructType string `json:"instruct_type"`
	} `json:"architecture"`
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
