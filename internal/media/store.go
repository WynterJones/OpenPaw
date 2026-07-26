package media

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
)

// SaveMeta is the bookkeeping attached to a generated asset.
type SaveMeta struct {
	Provider    string
	Model       string
	Prompt      string
	Kind        Kind
	WorkspaceID string
	FolderID    string
	ThreadID    string
	// Source attributes the generation to whoever asked for it — "studio" for
	// the UI, or the agent's provider name when a tool call made it. It keeps
	// the existing media library's source filter meaningful.
	Source string
}

// Dir returns the on-disk media directory. It sits beside the data directory
// rather than inside it, matching where agent-generated images already land.
func Dir(dataDir string) string {
	return filepath.Join(dataDir, "..", "media")
}

// Record is a stored asset as returned to the frontend.
type Record struct {
	ID          string `json:"id"`
	MediaType   string `json:"media_type"`
	Provider    string `json:"provider"`
	SourceModel string `json:"source_model"`
	Prompt      string `json:"prompt"`
	Filename    string `json:"filename"`
	MimeType    string `json:"mime_type"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	DurationMS  int    `json:"duration_ms"`
	SizeBytes   int    `json:"size_bytes"`
	FolderID    string `json:"folder_id"`
	WorkspaceID string `json:"workspace_id"`
	ThreadID    string `json:"thread_id"`
	CreatedAt   string `json:"created_at"`
	LocalURL    string `json:"local_url"`
}

// Save writes the asset to disk and records it in the media table, so Studio
// output appears in the media library and in chat like any other asset.
func Save(db *database.DB, dataDir string, asset *Asset, meta SaveMeta) (*Record, error) {
	if asset == nil || len(asset.Data) == 0 {
		return nil, fmt.Errorf("nothing to save")
	}

	dir := Dir(dataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create media directory: %w", err)
	}

	id := uuid.New().String()
	ext := asset.Ext
	if ext == "" {
		ext = extensionFor(asset.MimeType, meta.Kind)
	}
	filename := id + ext

	if err := os.WriteFile(filepath.Join(dir, filename), asset.Data, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write media file: %w", err)
	}

	source := meta.Source
	if source == "" {
		source = "studio"
	}
	prompt := meta.Prompt
	if asset.RevisedPrompt != "" {
		prompt = asset.RevisedPrompt
	}

	now := time.Now().UTC()
	_, err := db.Exec(
		`INSERT INTO media (id, thread_id, message_id, source, source_model, media_type, url, filename,
		                    mime_type, width, height, size_bytes, prompt, metadata, created_at,
		                    folder_id, workspace_id, provider, duration_ms)
		 VALUES (?, ?, '', ?, ?, ?, '', ?, ?, ?, ?, ?, ?, '{}', ?, ?, ?, ?, ?)`,
		id, meta.ThreadID, source, meta.Model, string(meta.Kind), filename,
		asset.MimeType, asset.Width, asset.Height, len(asset.Data), prompt, now,
		meta.FolderID, meta.WorkspaceID, meta.Provider, asset.DurationMS,
	)
	if err != nil {
		// The row is the thing that makes the file reachable, so a failed
		// insert leaves an orphan — clean it up rather than leaking disk.
		os.Remove(filepath.Join(dir, filename))
		return nil, fmt.Errorf("failed to record media: %w", err)
	}

	return &Record{
		ID:          id,
		MediaType:   string(meta.Kind),
		Provider:    meta.Provider,
		SourceModel: meta.Model,
		Prompt:      prompt,
		Filename:    filename,
		MimeType:    asset.MimeType,
		Width:       asset.Width,
		Height:      asset.Height,
		DurationMS:  asset.DurationMS,
		SizeBytes:   len(asset.Data),
		FolderID:    meta.FolderID,
		WorkspaceID: meta.WorkspaceID,
		ThreadID:    meta.ThreadID,
		CreatedAt:   now.Format(time.RFC3339),
		LocalURL:    fmt.Sprintf("/api/v1/media/%s/file", id),
	}, nil
}
