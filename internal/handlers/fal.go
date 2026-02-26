package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/fal"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/secrets"
)

type FalHandler struct {
	db         *database.DB
	client     *fal.Client
	secretsMgr *secrets.Manager
	dataDir    string
}

func NewFalHandler(db *database.DB, client *fal.Client, secretsMgr *secrets.Manager, dataDir string) *FalHandler {
	return &FalHandler{db: db, client: client, secretsMgr: secretsMgr, dataDir: dataDir}
}

func (h *FalHandler) Status(w http.ResponseWriter, r *http.Request) {
	source := resolveFALKeySource(h.client)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"configured": source != "none",
		"source":     source,
	})
}

func (h *FalHandler) UpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	encrypted, err := h.secretsMgr.Encrypt(req.APIKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt API key")
		return
	}

	if _, err := h.db.Exec(
		"INSERT INTO settings (id, key, value) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		uuid.New().String(), "fal_api_key", encrypted,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save API key")
		return
	}

	if h.client != nil {
		h.client.UpdateAPIKey(req.APIKey)
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "fal_api_key_updated", "settings", "settings", "fal_api_key", "")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"configured": true,
		"source":     "database",
	})
}

func (h *FalHandler) Models(w http.ResponseWriter, r *http.Request) {
	models := []map[string]string{
		{"id": "flux-dev", "name": "FLUX.1 Dev", "description": "High-quality development model with excellent prompt adherence"},
		{"id": "flux-schnell", "name": "FLUX.1 Schnell", "description": "Fast generation model optimized for speed"},
		{"id": "flux-pro", "name": "FLUX Pro 1.1 Ultra", "description": "Professional-grade model with the highest quality output"},
	}
	writeJSON(w, http.StatusOK, models)
}

func (h *FalHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt    string  `json:"prompt"`
		Model     string  `json:"model"`
		ImageSize string  `json:"image_size"`
		ThreadID  string  `json:"thread_id"`
		MessageID string  `json:"message_id"`
		Seed      int     `json:"seed"`
		Steps     int     `json:"num_inference_steps"`
		Guidance  float64 `json:"guidance_scale"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	if req.Model == "" {
		req.Model = "flux-schnell"
	}

	if req.ImageSize == "" {
		req.ImageSize = "landscape_16_9"
	}

	if !h.client.IsConfigured() {
		writeError(w, http.StatusBadRequest, "FAL API key not configured")
		return
	}

	opts := fal.GenerateOpts{
		Seed:              req.Seed,
		NumInferenceSteps: req.Steps,
		GuidanceScale:     req.Guidance,
		NumImages:         1,
	}

	result, err := h.client.Generate(r.Context(), req.Model, req.Prompt, req.ImageSize, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "generation failed: "+err.Error())
		return
	}

	if len(result.Images) == 0 {
		writeError(w, http.StatusInternalServerError, "no images returned")
		return
	}

	img := result.Images[0]

	// Download the image from FAL to local storage
	mediaDir := filepath.Join(h.dataDir, "..", "media")
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

	sizeBytes, dlErr := downloadFile(img.URL, destPath)
	if dlErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to download image: "+dlErr.Error())
		return
	}

	// Insert media record
	now := time.Now().UTC()
	if _, err := h.db.Exec(
		`INSERT INTO media (id, thread_id, message_id, source, source_model, media_type, url, filename, mime_type, width, height, size_bytes, prompt, created_at)
		 VALUES (?, ?, ?, 'fal', ?, 'image', ?, ?, ?, ?, ?, ?, ?, ?)`,
		mediaID, req.ThreadID, req.MessageID, req.Model, img.URL, filename, mimeType,
		img.Width, img.Height, sizeBytes, req.Prompt, now,
	); err != nil {
		logger.Error("Failed to insert media record: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save media record")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "image_generated", "media", "media", mediaID, req.Model)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":        mediaID,
		"url":       img.URL,
		"width":     img.Width,
		"height":    img.Height,
		"prompt":    req.Prompt,
		"seed":      result.Seed,
		"local_url": fmt.Sprintf("/api/v1/media/%s/file", mediaID),
	})
}

func resolveFALKeySource(client *fal.Client) string {
	if client == nil || !client.IsConfigured() {
		return "none"
	}
	if os.Getenv("FAL_KEY") != "" {
		return "env"
	}
	return "database"
}

func downloadFile(url, destPath string) (int64, error) {
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
