package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ImageGenModels = []string{
	"google/gemini-3-pro-image-preview",
	"sourceful/riverflow-v2-pro",
	"bytedance-seed/seedream-4.5",
}

type ImageResult struct {
	Base64        string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt"`
	// MimeType is what the model actually returned. Models ignore the format
	// hint and commonly reply with JPEG or WebP, so callers must not assume
	// PNG when naming the file.
	MimeType string `json:"mime_type"`
}

// mimeFromDataURI pulls the media type out of a "data:image/jpeg;base64,…"
// prefix, defaulting to PNG when the URI is malformed or unlabelled.
func mimeFromDataURI(uri string) string {
	if !strings.HasPrefix(uri, "data:") {
		return "image/png"
	}
	rest := uri[len("data:"):]
	end := strings.IndexAny(rest, ";,")
	if end <= 0 {
		return "image/png"
	}
	return rest[:end]
}

// GenerateImage sends an image generation request to OpenRouter with retry on 429 rate limits.
func (c *Client) GenerateImage(ctx context.Context, model, prompt, size string, images []string) (*ImageResult, error) {
	key := c.getAPIKey()
	if key == "" {
		return nil, fmt.Errorf("API client not configured")
	}

	if model == "" {
		model = ImageGenModels[0]
	}
	if size == "" {
		size = "1024x1024"
	}

	// Build user message content
	var content interface{}
	if len(images) > 0 {
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

	// Retry on 429 with exponential backoff
	const maxRetries = 3
	backoff := 2 * time.Second

	for attempt := range maxRetries {
		result, err := c.doImageRequest(ctx, key, body, prompt)
		if err == nil {
			return result, nil
		}

		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 {
			return nil, err
		}

		if attempt == maxRetries-1 {
			return nil, err
		}

		log.Printf("image gen model %s rate limited (attempt %d/%d), retrying in %v", model, attempt+1, maxRetries, backoff)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}

	return nil, fmt.Errorf("unreachable")
}

func (c *Client) doImageRequest(ctx context.Context, key string, body []byte, prompt string) (*ImageResult, error) {
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
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(errBody)}
	}

	// Parse chat completions response
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
				Images  []struct {
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
				return &ImageResult{
					Base64:        img.ImageURL.URL[idx+1:],
					RevisedPrompt: prompt,
					MimeType:      mimeFromDataURI(img.ImageURL.URL),
				}, nil
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
				return &ImageResult{Base64: part.B64JSON, RevisedPrompt: prompt, MimeType: "image/png"}, nil
			}
			if part.InlineData != nil && part.InlineData.Data != "" {
				mime := part.InlineData.MimeType
				if mime == "" {
					mime = "image/png"
				}
				return &ImageResult{Base64: part.InlineData.Data, RevisedPrompt: prompt, MimeType: mime}, nil
			}
			if part.ImageURL != nil && strings.HasPrefix(part.ImageURL.URL, "data:") {
				idx := strings.Index(part.ImageURL.URL, ",")
				if idx >= 0 {
					return &ImageResult{
						Base64:        part.ImageURL.URL[idx+1:],
						RevisedPrompt: prompt,
						MimeType:      mimeFromDataURI(part.ImageURL.URL),
					}, nil
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
		resolved, path, err := resolveFrontendAsset(dataDir, "avatars/"+filename, frontendFS)
		if err != nil {
			return "", fmt.Errorf("preset avatar not found: %s", urlPath)
		}
		if resolved != "" {
			return resolved, nil
		}
		filePath = path

	case isShippedAsset(urlPath):
		// Assets that ship with the frontend and are referenced by their public
		// URL — the preset backgrounds and the mascot. Same disk-then-embedded
		// lookup as preset avatars.
		resolved, path, err := resolveFrontendAsset(dataDir, strings.TrimPrefix(urlPath, "/"), frontendFS)
		if err != nil {
			return "", fmt.Errorf("shipped asset not found: %s", urlPath)
		}
		if resolved != "" {
			return resolved, nil
		}
		filePath = path

	default:
		return "", fmt.Errorf("unsupported local URL: %s", urlPath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read image file: %w", err)
	}

	return fmt.Sprintf("data:%s;base64,%s", MimeForPath(filePath), base64.StdEncoding.EncodeToString(data)), nil
}

// shippedAssetPrefixes are the frontend's own public files that may be inlined
// into a model request. It is an allowlist, not a directory walk: only assets
// the app itself references (preset backgrounds, the mascot) are reachable.
var shippedAssetPrefixes = []string{
	"/preset-bg/",
	"/cat-toolbar.webp",
	"/logo-transparent.png",
	"/icon.webp",
}

func isShippedAsset(urlPath string) bool {
	if strings.Contains(urlPath, "..") {
		return false
	}
	for _, p := range shippedAssetPrefixes {
		if strings.HasSuffix(p, "/") {
			if strings.HasPrefix(urlPath, p) && !strings.Contains(strings.TrimPrefix(urlPath, p), "/") {
				return true
			}
			continue
		}
		if urlPath == p {
			return true
		}
	}
	return false
}

// resolveFrontendAsset locates a file that ships with the frontend. In a dev
// checkout it is on disk; in the single-binary build it only exists inside the
// embedded FS, which cannot be handed back as a path — hence the two return
// modes: a ready data URI (embedded) or a filesystem path (disk) for the
// caller to read.
func resolveFrontendAsset(dataDir, rel string, frontendFS fs.FS) (dataURI string, filePath string, err error) {
	if rel == "" || strings.Contains(rel, "..") {
		return "", "", fmt.Errorf("invalid asset path %q", rel)
	}

	parts := strings.Split(rel, "/")
	diskRel := filepath.Join(parts...)
	candidates := []string{
		filepath.Join(dataDir, "..", "web", "frontend", "public", diskRel),
		filepath.Join(dataDir, "..", "web", "frontend", "dist", diskRel),
	}
	for _, c := range candidates {
		if info, statErr := os.Stat(c); statErr == nil && !info.IsDir() {
			return "", c, nil
		}
	}

	if frontendFS != nil {
		if data, readErr := fs.ReadFile(frontendFS, rel); readErr == nil {
			return fmt.Sprintf("data:%s;base64,%s", MimeForPath(rel), base64.StdEncoding.EncodeToString(data)), "", nil
		}
	}

	return "", "", fmt.Errorf("asset not found: %s", rel)
}

// MimeForPath guesses an image MIME type from a file extension, defaulting to
// PNG for anything unrecognised.
func MimeForPath(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	}
	return "image/png"
}
