package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// DescribeImage uses a vision model to analyze an image and return a text description.
func (c *Client) DescribeImage(ctx context.Context, model, imageDataURI, prompt string) (string, error) {
	if model == "" {
		model = "google/gemini-2.5-flash-preview"
	}

	content := []map[string]interface{}{
		{"type": "text", "text": prompt},
		{"type": "image_url", "image_url": map[string]interface{}{
			"url": imageDataURI,
		}},
	}

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
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.doRawRequest(ctx, body)
	if err != nil {
		return "", fmt.Errorf("vision request failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from vision model")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}
