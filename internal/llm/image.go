package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type ImageResult struct {
	Base64        string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt"`
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

	// Build user message content
	var content interface{}
	if len(images) > 0 {
		// Multipart content: text prompt + reference images
		parts := []map[string]interface{}{
			{"type": "text", "text": prompt},
		}
		for _, img := range images {
			parts = append(parts, map[string]interface{}{
				"type": "image_url",
				"image_url": map[string]interface{}{
					"url": img,
				},
			})
		}
		content = parts
	} else {
		content = prompt
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{"role": "user", "content": content},
		},
		"modalities": []string{"image", "text"},
		"max_tokens": 4096,
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

	// Parse chat completions response
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
				// OpenRouter returns images in a separate "images" field
				Images []struct {
					Type     string `json:"type"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"images"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned")
	}

	msg := chatResp.Choices[0].Message

	// Check the "images" field first (OpenRouter's standard format)
	for _, img := range msg.Images {
		if img.ImageURL.URL != "" && strings.HasPrefix(img.ImageURL.URL, "data:") {
			idx := strings.Index(img.ImageURL.URL, ",")
			if idx >= 0 {
				return &ImageResult{Base64: img.ImageURL.URL[idx+1:], RevisedPrompt: prompt}, nil
			}
		}
	}

	// Fall back to parsing content as an array of parts
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
	if err := json.Unmarshal(msg.Content, &parts); err == nil {
		for _, part := range parts {
			if part.B64JSON != "" {
				return &ImageResult{Base64: part.B64JSON, RevisedPrompt: prompt}, nil
			}
			if part.InlineData != nil && part.InlineData.Data != "" {
				return &ImageResult{Base64: part.InlineData.Data, RevisedPrompt: prompt}, nil
			}
			if part.ImageURL != nil && strings.HasPrefix(part.ImageURL.URL, "data:") {
				idx := strings.Index(part.ImageURL.URL, ",")
				if idx >= 0 {
					return &ImageResult{Base64: part.ImageURL.URL[idx+1:], RevisedPrompt: prompt}, nil
				}
			}
		}
		for _, part := range parts {
			if part.Text != "" {
				return nil, fmt.Errorf("model returned text instead of image: %s", part.Text)
			}
		}
	}

	// Try parsing content as a plain string (text-only response)
	var textContent string
	if err := json.Unmarshal(msg.Content, &textContent); err == nil {
		return nil, fmt.Errorf("model returned text instead of image: %s", textContent)
	}

	return nil, fmt.Errorf("could not extract image from response")
}

// ResolveImageToBase64 converts local image URLs to base64 data URIs.
// External URLs (http/https) are returned as-is.
// frontendFS is an optional embedded filesystem used as fallback for preset avatars.
func ResolveImageToBase64(dataDir string, urlPath string, frontendFS fs.FS) (string, error) {
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
		// Preset avatars - try disk first, then embedded filesystem
		filename := filepath.Base(urlPath)
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
		if filePath == "" && frontendFS != nil {
			// Fall back to embedded filesystem
			embeddedPath := "avatars/" + filename
			if data, err := fs.ReadFile(frontendFS, embeddedPath); err == nil {
				mimeType := "image/png"
				switch strings.ToLower(filepath.Ext(filename)) {
				case ".jpg", ".jpeg":
					mimeType = "image/jpeg"
				case ".webp":
					mimeType = "image/webp"
				case ".gif":
					mimeType = "image/gif"
				}
				return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)), nil
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
