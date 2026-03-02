package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

var visionModels = []string{
	"google/gemini-3-flash-preview",
	"google/gemini-2.5-flash",
}

// DescribeImage uses a vision model to analyze an image and return a text description.
// It retries on 429 rate limits with exponential backoff, then falls back to alternative models.
func (c *Client) DescribeImage(ctx context.Context, model, imageDataURI, prompt string) (string, error) {
	models := visionModels
	if model != "" {
		models = []string{model}
	}

	content := []map[string]interface{}{
		{"type": "text", "text": prompt},
		{"type": "image_url", "image_url": map[string]interface{}{
			"url": imageDataURI,
		}},
	}

	var lastErr error
	for _, m := range models {
		resp, err := c.describeImageWithRetry(ctx, m, content)
		if err != nil {
			lastErr = err
			log.Printf("vision model %s failed: %v, trying next", m, err)
			continue
		}
		if len(resp.Choices) == 0 {
			lastErr = fmt.Errorf("no response from vision model %s", m)
			continue
		}
		return strings.TrimSpace(resp.Choices[0].Message.Content), nil
	}

	return "", fmt.Errorf("vision request failed (all models exhausted): %w", lastErr)
}

func (c *Client) describeImageWithRetry(ctx context.Context, model string, content interface{}) (*ChatCompletionResponse, error) {
	rawReq := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{"role": "user", "content": content},
		},
		"max_tokens": 512,
		"stream":     false,
	}

	body, err := json.Marshal(rawReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	const maxRetries = 3
	backoff := time.Second

	for attempt := range maxRetries {
		resp, err := c.doRawRequest(ctx, body)
		if err == nil {
			return resp, nil
		}

		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 {
			return nil, err
		}

		if attempt == maxRetries-1 {
			return nil, err
		}

		log.Printf("vision model %s rate limited (attempt %d/%d), retrying in %v", model, attempt+1, maxRetries, backoff)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}

	return nil, fmt.Errorf("unreachable")
}
