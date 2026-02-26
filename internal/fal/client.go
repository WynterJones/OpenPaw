package fal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Client is a FAL API client for image generation.
type Client struct {
	apiKey     string
	httpClient *http.Client
	mu         sync.RWMutex
}

// NewClient creates a new FAL client with the given API key.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// UpdateAPIKey hot-reloads the API key.
func (c *Client) UpdateAPIKey(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apiKey = key
}

// IsConfigured returns true if an API key is set.
func (c *Client) IsConfigured() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.apiKey != ""
}

// Generate sends an image generation request to the FAL API.
func (c *Client) Generate(ctx context.Context, model, prompt, imageSize string, opts GenerateOpts) (*Result, error) {
	c.mu.RLock()
	key := c.apiKey
	c.mu.RUnlock()

	if key == "" {
		return nil, fmt.Errorf("FAL API key not configured")
	}

	endpoint, ok := SupportedModels[model]
	if !ok {
		return nil, fmt.Errorf("unsupported model: %s", model)
	}

	reqBody := GenerateRequest{
		Prompt:    prompt,
		ImageSize: imageSize,
	}
	if opts.NumInferenceSteps > 0 {
		reqBody.NumInferenceSteps = opts.NumInferenceSteps
	}
	if opts.GuidanceScale > 0 {
		reqBody.GuidanceScale = opts.GuidanceScale
	}
	if opts.Seed > 0 {
		reqBody.Seed = opts.Seed
	}
	if opts.NumImages > 0 {
		reqBody.NumImages = opts.NumImages
	}
	if opts.OutputFormat != "" {
		reqBody.OutputFormat = opts.OutputFormat
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("https://fal.run/%s", endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Key "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FAL API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("FAL API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var genResp GenerateResponse
	if err := json.Unmarshal(respBody, &genResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &Result{
		Images: genResp.Images,
		Seed:   genResp.Seed,
		Prompt: genResp.Prompt,
	}, nil
}
