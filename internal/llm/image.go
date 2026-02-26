package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

func (c *Client) GenerateImage(ctx context.Context, model, prompt, size string, images []string) (*ImageResult, error) {
	key := c.getAPIKey()
	if key == "" {
		return nil, fmt.Errorf("API client not configured")
	}

	if model == "" {
		model = "google/gemini-2.5-flash-image-preview"
	}
	if size == "" {
		size = "1024x1024"
	}

	// If input images are provided, use chat completions with image modality
	if len(images) > 0 {
		return c.generateImageWithInputs(ctx, key, model, prompt, size, images)
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

// generateImageWithInputs uses the chat completions endpoint with image modality
// to generate images that reference input images.
func (c *Client) generateImageWithInputs(ctx context.Context, key, model, prompt, size string, images []string) (*ImageResult, error) {
	// Build content array: text prompt + image_url entries
	content := []map[string]interface{}{
		{"type": "text", "text": prompt},
	}
	for _, img := range images {
		content = append(content, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url": img,
			},
		})
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{"role": "user", "content": content},
		},
		"modalities":      []string{"image", "text"},
		"response_format": map[string]interface{}{"type": "b64_json"},
		"max_tokens":      4096,
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("image generation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("image generation failed (status %d): %s", resp.StatusCode, string(errBody))
	}

	// Parse chat completions response — images may be in content array or inline_data
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned")
	}

	rawContent := chatResp.Choices[0].Message.Content

	// Try parsing content as an array of parts (multimodal response)
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url,omitempty"`
		InlineData *struct {
			MimeType string `json:"mime_type"`
			Data     string `json:"data"`
		} `json:"inline_data,omitempty"`
		B64JSON string `json:"b64_json,omitempty"`
	}
	if err := json.Unmarshal(rawContent, &parts); err == nil {
		for _, part := range parts {
			if part.B64JSON != "" {
				return &ImageResult{Base64: part.B64JSON, RevisedPrompt: prompt}, nil
			}
			if part.InlineData != nil && part.InlineData.Data != "" {
				return &ImageResult{Base64: part.InlineData.Data, RevisedPrompt: prompt}, nil
			}
			if part.ImageURL != nil && strings.HasPrefix(part.ImageURL.URL, "data:") {
				// Extract base64 from data URI
				idx := strings.Index(part.ImageURL.URL, ",")
				if idx >= 0 {
					return &ImageResult{Base64: part.ImageURL.URL[idx+1:], RevisedPrompt: prompt}, nil
				}
			}
		}
		// No image in parts, return text if available
		for _, part := range parts {
			if part.Text != "" {
				return nil, fmt.Errorf("model returned text instead of image: %s", part.Text)
			}
		}
	}

	// Try parsing content as a plain string (text-only response)
	var textContent string
	if err := json.Unmarshal(rawContent, &textContent); err == nil {
		return nil, fmt.Errorf("model returned text instead of image: %s", textContent)
	}

	return nil, fmt.Errorf("could not extract image from response")
}

// ResolveImageToBase64 converts local image URLs to base64 data URIs.
// External URLs (http/https) are returned as-is.
func ResolveImageToBase64(dataDir string, urlPath string) (string, error) {
	if strings.HasPrefix(urlPath, "http://") || strings.HasPrefix(urlPath, "https://") {
		return urlPath, nil
	}
	if strings.HasPrefix(urlPath, "data:") {
		return urlPath, nil
	}

	var filePath string
	switch {
	case strings.HasPrefix(urlPath, "/api/v1/media/"):
		// /api/v1/media/{id}/file → read from media directory
		parts := strings.Split(strings.TrimPrefix(urlPath, "/api/v1/media/"), "/")
		if len(parts) < 1 {
			return "", fmt.Errorf("invalid media URL: %s", urlPath)
		}
		mediaID := parts[0]
		mediaDir := filepath.Join(dataDir, "..", "media")
		// Find the file by ID prefix (could be .png, .jpg, .webp)
		matches, _ := filepath.Glob(filepath.Join(mediaDir, mediaID+".*"))
		if len(matches) == 0 {
			return "", fmt.Errorf("media file not found for ID %s", mediaID)
		}
		filePath = matches[0]

	case strings.HasPrefix(urlPath, "/api/v1/uploads/avatars/"):
		filename := filepath.Base(urlPath)
		filePath = filepath.Join(dataDir, "avatars", filename)

	case strings.HasPrefix(urlPath, "/avatars/"):
		// Preset avatars in frontend public directory
		filename := filepath.Base(urlPath)
		// Try the frontend public directory first (development), then dist
		candidates := []string{
			filepath.Join(dataDir, "..", "web", "frontend", "public", "avatars", filename),
			filepath.Join(dataDir, "..", "web", "frontend", "dist", "avatars", filename),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				filePath = c
				break
			}
		}
		if filePath == "" {
			return "", fmt.Errorf("preset avatar not found: %s", urlPath)
		}

	default:
		return "", fmt.Errorf("unsupported local URL: %s", urlPath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read image file: %w", err)
	}

	mimeType := "image/png"
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".webp":
		mimeType = "image/webp"
	case ".gif":
		mimeType = "image/gif"
	}

	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)), nil
}
