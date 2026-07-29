package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openpaw/openpaw/internal/models"
)

// A dream stores scanned_at as a TIMESTAMP, but SQLite aggregate expressions
// such as MAX(scanned_at) lose the column's declared type and are returned by
// the driver as text. The chat list must still load after the first dream;
// otherwise the frontend interprets the failed request as an empty workspace.
func TestListThreadsAfterDreamScan(t *testing.T) {
	db := newTestDB(t)
	h := &ChatHandler{db: db}

	updatedAt := time.Date(2026, 7, 29, 5, 40, 54, 0, time.UTC)
	scannedAt := time.Date(2026, 7, 29, 7, 2, 18, 0, time.UTC)
	if _, err := db.Exec(
		`INSERT INTO chat_threads (id, title, workspace_id, pinned, created_at, updated_at)
		 VALUES ('thread-1', 'Still here', ?, 1, ?, ?)`,
		db.ActiveWorkspaceID(), updatedAt.Add(-time.Hour), updatedAt,
	); err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO dream_scans
		 (id, agent_slug, thread_id, facts_found, last_message_at, scanned_at)
		 VALUES ('scan-1', 'builder', 'thread-1', 3, ?, ?)`,
		updatedAt, scannedAt,
	); err != nil {
		t.Fatalf("insert dream scan: %v", err)
	}

	for _, path := range []string{"/chat/threads", "/chat/threads?pinned=1"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ListThreads(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
			}

			var threads []models.ChatThread
			if err := json.Unmarshal(rec.Body.Bytes(), &threads); err != nil {
				t.Fatalf("decode response: %v — body %s", err, rec.Body.String())
			}
			if len(threads) != 1 {
				t.Fatalf("threads = %d, want 1", len(threads))
			}
			got := threads[0]
			if !got.Pinned || !got.Dreamed || got.DreamedAt == nil {
				t.Fatalf("pinned/dreamed metadata was lost: %+v", got)
			}
			if !got.DreamedAt.Equal(scannedAt) {
				t.Errorf("dreamed_at = %s, want %s", got.DreamedAt, scannedAt)
			}
		})
	}
}
