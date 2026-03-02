package agents

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	llm "github.com/openpaw/openpaw/internal/llm"
)

func BuildGenerateImageDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{"type": "string", "description": "The image generation prompt describing what to create"},
			"model":  map[string]interface{}{"type": "string", "description": "Model override. Leave empty for default (Gemini 3 Pro Image). Fallbacks: sourceful/riverflow-v2-pro, bytedance-seed/seedream-4.5"},
			"size":   map[string]interface{}{"type": "string", "description": "Image size: 1024x1024 (default), 1536x1024, 1024x1536"},
			"images": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional array of input image URLs for visual reference. Use local paths like /api/v1/media/{id}/file for media library images, /api/v1/uploads/avatars/{filename} for uploaded avatars, or /avatars/avatar-N.webp for preset avatars."},
		},
		"required": []string{"prompt"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "generate_image",
			Description: "Generate an image from a text prompt via OpenRouter. Uses Gemini 3 Pro Image with automatic fallback to alternative models on failure.",
			Parameters:  params,
		},
	}
}

func (m *Manager) makeGenerateImageHandler() llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var req struct {
			Prompt string   `json:"prompt"`
			Model  string   `json:"model"`
			Size   string   `json:"size"`
			Images []string `json:"images"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return llm.ToolResult{Output: "ERROR: Invalid input: " + err.Error(), IsError: true}
		}

		if req.Prompt == "" {
			return llm.ToolResult{Output: "ERROR: prompt is required", IsError: true}
		}

		return m.generateImage(ctx, req.Prompt, req.Model, req.Size, req.Images)
	}
}

func (m *Manager) generateImage(ctx context.Context, prompt, model, size string, images []string) llm.ToolResult {
	if m.client == nil || !m.client.IsConfigured() {
		return llm.ToolResult{Output: "ERROR: OpenRouter API key not configured. Set it up in Settings.", IsError: true}
	}

	if size == "" {
		size = "1024x1024"
	}

	// Resolve local image URLs to base64 data URIs
	var resolvedImages []string
	for _, img := range images {
		resolved, err := llm.ResolveImageToBase64(m.DataDir, img, m.FrontendFS)
		if err != nil {
			return llm.ToolResult{Output: "ERROR: Failed to resolve image " + img + ": " + err.Error(), IsError: true}
		}
		resolvedImages = append(resolvedImages, resolved)
	}

	// If a specific model was requested, try only that model
	if model != "" {
		return m.tryGenerateImage(ctx, prompt, model, size, resolvedImages)
	}

	// Try models in fallback order
	models := llm.ImageGenModels
	var lastErr string
	for _, mdl := range models {
		result := m.tryGenerateImage(ctx, prompt, mdl, size, resolvedImages)
		if !result.IsError {
			return result
		}
		lastErr = result.Output
	}

	return llm.ToolResult{
		Output:  fmt.Sprintf("ERROR: All image generation models failed. Last error: %s", lastErr),
		IsError: true,
	}
}

func (m *Manager) tryGenerateImage(ctx context.Context, prompt, model, size string, resolvedImages []string) llm.ToolResult {
	result, err := m.client.GenerateImage(ctx, model, prompt, size, resolvedImages)
	if err != nil {
		return llm.ToolResult{Output: fmt.Sprintf("ERROR [model=%s]: %s", model, err.Error()), IsError: true}
	}

	if result.Base64 == "" {
		return llm.ToolResult{Output: fmt.Sprintf("ERROR [model=%s]: No image data returned", model), IsError: true}
	}

	// Decode base64 and save to media directory
	imgData, err := base64.StdEncoding.DecodeString(result.Base64)
	if err != nil {
		return llm.ToolResult{Output: fmt.Sprintf("ERROR [model=%s]: Failed to decode image data: %s", model, err.Error()), IsError: true}
	}

	mediaDir := filepath.Join(m.DataDir, "..", "media")
	os.MkdirAll(mediaDir, 0755)

	mediaID := uuid.New().String()
	filename := mediaID + ".png"
	destPath := filepath.Join(mediaDir, filename)

	if err := os.WriteFile(destPath, imgData, 0644); err != nil {
		return llm.ToolResult{Output: "ERROR: Failed to save image: " + err.Error(), IsError: true}
	}

	width, height := parseSizeDimensions(size)

	now := time.Now().UTC()
	m.db.Exec(
		`INSERT INTO media (id, source, source_model, media_type, url, filename, mime_type, width, height, size_bytes, prompt, created_at)
		 VALUES (?, 'openrouter', ?, 'image', '', ?, 'image/png', ?, ?, ?, ?, ?)`,
		mediaID, model, filename, width, height, len(imgData), prompt, now,
	)

	localURL := fmt.Sprintf("/api/v1/media/%s/file", mediaID)
	displayPrompt := result.RevisedPrompt
	if displayPrompt == "" {
		displayPrompt = prompt
	}

	return llm.ToolResult{
		Output: fmt.Sprintf("Image generated successfully!\n\n![%s](%s)\n\nModel: %s\nSize: %s\nLocal URL: %s",
			displayPrompt, localURL, model, size, localURL),
		ImageURL: localURL,
	}
}

func parseSizeDimensions(size string) (int, int) {
	var w, h int
	if n, _ := fmt.Sscanf(size, "%dx%d", &w, &h); n == 2 && w > 0 && h > 0 {
		return w, h
	}
	return 1024, 1024
}
