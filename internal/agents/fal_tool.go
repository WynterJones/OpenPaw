package agents

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/fal"
	llm "github.com/openpaw/openpaw/internal/llm"
)

func BuildGenerateImageDef(falAvailable bool) llm.ToolDef {
	providerDesc := "Provider: gemini (default, uses Gemini Flash via OpenRouter)"
	providerEnum := []string{"gemini"}
	if falAvailable {
		providerDesc = "Provider: gemini (default, uses Gemini Flash via OpenRouter) or fal (uses FLUX models, only when user explicitly asks for FAL/FLUX)"
		providerEnum = []string{"gemini", "fal"}
	}

	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt":   map[string]interface{}{"type": "string", "description": "The image generation prompt describing what to create"},
			"provider": map[string]interface{}{"type": "string", "description": providerDesc, "enum": providerEnum},
			"model":    map[string]interface{}{"type": "string", "description": "Model override. For fal: flux-dev, flux-schnell, flux-pro. Leave empty for default."},
			"size":     map[string]interface{}{"type": "string", "description": "Image size. For gemini: 1024x1024, 1536x1024, 1024x1536. For fal: square_hd, landscape_16_9, portrait_16_9, etc."},
			"images":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional array of input image URLs for reference. Use local paths like /api/v1/media/{id}/file for media library images, /api/v1/uploads/avatars/{filename} for uploaded avatars, or /avatars/avatar-N.webp for preset avatars. The model will use these as visual reference alongside your text prompt."},
		},
		"required": []string{"prompt"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "generate_image",
			Description: "Generate an image from a text prompt. Uses Gemini Flash by default. Only use provider 'fal' if the user explicitly asks for FAL or FLUX models.",
			Parameters:  params,
		},
	}
}

// Kept for backwards compat if referenced elsewhere
func BuildGenerateImageFalDef() llm.ToolDef {
	return BuildGenerateImageDef(true)
}

func (m *Manager) makeGenerateImageHandler() llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var req struct {
			Prompt   string   `json:"prompt"`
			Provider string   `json:"provider"`
			Model    string   `json:"model"`
			Size     string   `json:"size"`
			Images   []string `json:"images"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}

		if req.Prompt == "" {
			return llm.ToolResult{Output: "prompt is required", IsError: true}
		}

		if req.Provider == "" {
			req.Provider = "gemini"
		}

		switch req.Provider {
		case "fal":
			return m.generateImageFal(ctx, req.Prompt, req.Model, req.Size)
		default:
			return m.generateImageGemini(ctx, req.Prompt, req.Model, req.Size, req.Images)
		}
	}
}

// Kept for backwards compat
func (m *Manager) makeGenerateImageFalHandler() llm.ToolHandler {
	return m.makeGenerateImageHandler()
}

func (m *Manager) generateImageGemini(ctx context.Context, prompt, model, size string, images []string) llm.ToolResult {
	if m.client == nil || !m.client.IsConfigured() {
		return llm.ToolResult{Output: "OpenRouter API key not configured. Set it up in Settings.", IsError: true}
	}

	if model == "" {
		model = "google/gemini-2.5-flash-image-preview"
	}
	if size == "" {
		size = "1024x1024"
	}

	// Resolve local image URLs to base64 data URIs
	var resolvedImages []string
	for _, img := range images {
		resolved, err := llm.ResolveImageToBase64(m.DataDir, img, m.FrontendFS)
		if err != nil {
			return llm.ToolResult{Output: "Failed to resolve image " + img + ": " + err.Error(), IsError: true}
		}
		resolvedImages = append(resolvedImages, resolved)
	}

	result, err := m.client.GenerateImage(ctx, model, prompt, size, resolvedImages)
	if err != nil {
		return llm.ToolResult{Output: "Image generation failed: " + err.Error(), IsError: true}
	}

	if result.Base64 == "" {
		return llm.ToolResult{Output: "No image data returned", IsError: true}
	}

	// Decode base64 and save to media directory
	imgData, err := base64.StdEncoding.DecodeString(result.Base64)
	if err != nil {
		return llm.ToolResult{Output: "Failed to decode image data: " + err.Error(), IsError: true}
	}

	mediaDir := filepath.Join(m.DataDir, "..", "media")
	os.MkdirAll(mediaDir, 0755)

	mediaID := uuid.New().String()
	filename := mediaID + ".png"
	destPath := filepath.Join(mediaDir, filename)

	if err := os.WriteFile(destPath, imgData, 0644); err != nil {
		return llm.ToolResult{Output: "Failed to save image: " + err.Error(), IsError: true}
	}

	// Parse dimensions from size string
	width, height := parseSizeDimensions(size)

	now := time.Now().UTC()
	m.db.Exec(
		`INSERT INTO media (id, source, source_model, media_type, url, filename, mime_type, width, height, size_bytes, prompt, created_at)
		 VALUES (?, 'gemini', ?, 'image', '', ?, 'image/png', ?, ?, ?, ?, ?)`,
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
	}
}

func (m *Manager) generateImageFal(ctx context.Context, prompt, model, size string) llm.ToolResult {
	if m.FalClient == nil || !m.FalClient.IsConfigured() {
		return llm.ToolResult{Output: "FAL API key not configured. Ask the user to set it up in Settings > FAL AI, or omit provider to use Gemini instead.", IsError: true}
	}

	if model == "" {
		model = "flux-schnell"
	}
	if size == "" {
		size = "landscape_16_9"
	}

	opts := fal.GenerateOpts{
		NumImages: 1,
	}

	result, err := m.FalClient.Generate(ctx, model, prompt, size, opts)
	if err != nil {
		return llm.ToolResult{Output: "Image generation failed: " + err.Error(), IsError: true}
	}

	if len(result.Images) == 0 {
		return llm.ToolResult{Output: "No images were returned", IsError: true}
	}

	img := result.Images[0]

	mediaDir := filepath.Join(m.DataDir, "..", "media")
	os.MkdirAll(mediaDir, 0755)

	ext := ".jpg"
	mimeType := "image/jpeg"
	if img.ContentType != "" {
		switch {
		case strings.Contains(img.ContentType, "png"):
			ext = ".png"
			mimeType = "image/png"
		case strings.Contains(img.ContentType, "webp"):
			ext = ".webp"
			mimeType = "image/webp"
		}
	}

	mediaID := uuid.New().String()
	filename := mediaID + ext
	destPath := filepath.Join(mediaDir, filename)

	sizeBytes, dlErr := downloadFalImage(img.URL, destPath)
	if dlErr != nil {
		return llm.ToolResult{Output: "Failed to download image: " + dlErr.Error(), IsError: true}
	}

	now := time.Now().UTC()
	m.db.Exec(
		`INSERT INTO media (id, source, source_model, media_type, url, filename, mime_type, width, height, size_bytes, prompt, created_at)
		 VALUES (?, 'fal', ?, 'image', ?, ?, ?, ?, ?, ?, ?, ?)`,
		mediaID, model, img.URL, filename, mimeType,
		img.Width, img.Height, sizeBytes, prompt, now,
	)

	localURL := fmt.Sprintf("/api/v1/media/%s/file", mediaID)

	return llm.ToolResult{
		Output: fmt.Sprintf("Image generated successfully!\n\n![%s](%s)\n\nModel: %s (FAL)\nSize: %dx%d\nLocal URL: %s",
			prompt, localURL, model, img.Width, img.Height, localURL),
	}
}

func parseSizeDimensions(size string) (int, int) {
	parts := strings.SplitN(size, "x", 2)
	if len(parts) == 2 {
		w, h := 0, 0
		fmt.Sscanf(parts[0], "%d", &w)
		fmt.Sscanf(parts[1], "%d", &h)
		if w > 0 && h > 0 {
			return w, h
		}
	}
	return 1024, 1024
}

func downloadFalImage(url, destPath string) (int64, error) {
	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("write file: %w", err)
	}

	return n, nil
}
