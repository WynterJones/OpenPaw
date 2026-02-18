package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type ImageResult struct {
	Base64        string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt"`
}

type imageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n"`
	Size           string `json:"size"`
	ResponseFormat string `json:"response_format"`
}

type imageResponse struct {
	Data []ImageResult `json:"data"`
}

func (c *Client) GenerateImage(ctx context.Context, model, prompt, size string) (*ImageResult, error) {
	key := c.getAPIKey()
	if key == "" {
		return nil, fmt.Errorf("API client not configured")
	}

	if model == "" {
		model = "openai/dall-e-3"
	}
	if size == "" {
		size = "1024x1024"
	}

	reqBody := imageRequest{
		Model:          model,
		Prompt:         prompt,
		N:              1,
		Size:           size,
		ResponseFormat: "b64_json",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("HTTP-Referer", "https://openpaw.dev")
	req.Header.Set("X-Title", "OpenPaw")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("image generation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("image generation failed (status %d): %s", resp.StatusCode, string(errBody))
	}

	var result imageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode image response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no image data returned")
	}

	return &result.Data[0], nil
}
