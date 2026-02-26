package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

type Client struct {
	httpClient           *http.Client // streaming — no timeout (SSE needs unlimited time)
	nonStreamingClient   *http.Client // non-streaming — 30s timeout
	baseURL              string
	apiKey               string
	hasKey               bool
	mu                   sync.RWMutex
}

func NewClient(apiKey string) *Client {
	if apiKey == "" {
		apiKey = os.Getenv("OPENROUTER_API_KEY")
	}

	c := &Client{
		httpClient:         &http.Client{},
		nonStreamingClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:            "https://openrouter.ai/api/v1",
	}
	if apiKey != "" {
		c.apiKey = apiKey
		c.hasKey = true
	}
	return c
}

func (c *Client) UpdateAPIKey(apiKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if apiKey != "" {
		c.apiKey = apiKey
		c.hasKey = true
	} else {
		c.apiKey = ""
		c.hasKey = false
	}
}

func (c *Client) IsConfigured() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hasKey
}

func (c *Client) getAPIKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.apiKey
}

func (c *Client) ValidateKey(ctx context.Context) error {
	key := c.getAPIKey()
	if key == "" {
		return fmt.Errorf("API key not configured")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/models", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.nonStreamingClient.Do(req)
	if err != nil {
		return fmt.Errorf("API key validation failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("API key validation failed: unauthorized")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API key validation failed: status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func ResolveAPIKey(envKey, dbKey string) (key string, source string) {
	if envKey != "" {
		return envKey, "env"
	}
	if dbKey != "" {
		return dbKey, "database"
	}
	return "", "none"
}

// ChatMessage represents a message in OpenAI Chat Completions format.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatCompletionRequest is the request body for the chat completions endpoint.
type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Tools    []ToolDef     `json:"tools,omitempty"`
	Stream   bool          `json:"stream"`
	MaxTokens int64        `json:"max_tokens,omitempty"`
}

// prepareRequest builds an authenticated POST request to the chat completions endpoint.
func (c *Client) prepareRequest(ctx context.Context, reqBody ChatCompletionRequest) (*http.Request, error) {
	key := c.getAPIKey()
	if key == "" {
		return nil, fmt.Errorf("API client not configured")
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("HTTP-Referer", "https://openpaw.dev")
	req.Header.Set("X-Title", "OpenPaw")

	return req, nil
}

// doStreamRequest sends a streaming POST to the chat completions endpoint.
func (c *Client) doStreamRequest(ctx context.Context, reqBody ChatCompletionRequest) (*http.Response, error) {
	reqBody.Stream = true
	req, err := c.prepareRequest(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	return resp, nil
}

// doRequest sends a non-streaming POST to the chat completions endpoint.
func (c *Client) doRequest(ctx context.Context, reqBody ChatCompletionRequest) (*ChatCompletionResponse, error) {
	reqBody.Stream = false
	req, err := c.prepareRequest(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.nonStreamingClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	var result ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// doRawRequest sends a non-streaming POST with pre-marshaled JSON body to chat completions.
// Used for multimodal requests where the content field is an array rather than a string.
func (c *Client) doRawRequest(ctx context.Context, body []byte) (*ChatCompletionResponse, error) {
	key := c.getAPIKey()
	if key == "" {
		return nil, fmt.Errorf("API client not configured")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("HTTP-Referer", "https://openpaw.dev")
	req.Header.Set("X-Title", "OpenPaw")

	resp, err := c.nonStreamingClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	var result ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// ChatCompletionResponse is the response from the chat completions endpoint.
type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message      ChatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
}

// KeyInfo holds usage and limit data from the OpenRouter /key endpoint.
type KeyInfo struct {
	Label          string     `json:"label"`
	Limit          *float64   `json:"limit"`
	LimitRemaining *float64   `json:"limit_remaining"`
	Usage          float64    `json:"usage"`
	UsageMonthly   float64    `json:"usage_monthly"`
	IsFreeTier     bool       `json:"is_free_tier"`
	RateLimit      *RateLimit `json:"rate_limit,omitempty"`
}

type RateLimit struct {
	Requests int    `json:"requests"`
	Interval string `json:"interval"`
}

type keyInfoResponse struct {
	Data KeyInfo `json:"data"`
}

// GetKeyInfo retrieves usage and limit info for the configured API key.
func (c *Client) GetKeyInfo(ctx context.Context) (*KeyInfo, error) {
	key := c.getAPIKey()
	if key == "" {
		return nil, fmt.Errorf("API key not configured")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/key", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.nonStreamingClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("key info request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("key info failed: status %d: %s", resp.StatusCode, string(body))
	}

	var result keyInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode key info: %w", err)
	}

	return &result.Data, nil
}

// CreditsInfo holds account-level credit data from the OpenRouter /credits endpoint.
type CreditsInfo struct {
	TotalCredits float64 `json:"total_credits"`
	TotalUsage   float64 `json:"total_usage"`
}

type creditsResponse struct {
	Data CreditsInfo `json:"data"`
}

// GetCredits retrieves account-level credit info from OpenRouter.
// Returns nil without error if the endpoint is not accessible (e.g. standard key).
func (c *Client) GetCredits(ctx context.Context) (*CreditsInfo, error) {
	key := c.getAPIKey()
	if key == "" {
		return nil, fmt.Errorf("API key not configured")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/credits", nil)
	if err != nil {
		return nil, nil
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.nonStreamingClient.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, nil
	}

	var result creditsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil
	}

	return &result.Data, nil
}

// FetchModels retrieves available models from OpenRouter.
func (c *Client) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	return FetchModels(ctx, c.getAPIKey())
}

// GetCachedModels returns models from cache, fetching if stale.
func (c *Client) GetCachedModels(ctx context.Context) ([]ModelInfo, error) {
	return GetCachedModels(ctx, c.getAPIKey())
}
