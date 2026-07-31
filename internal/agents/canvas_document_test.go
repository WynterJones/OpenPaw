package agents

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/models"
)

func TestCanvasShowDocument_IsWorkspaceScoped(t *testing.T) {
	db, _ := newContextTestDB(t)
	const otherWorkspace = "workspace-canvas-other"
	if _, err := db.Exec("INSERT INTO workspaces (id, name) VALUES (?, ?)", otherWorkspace, "Other"); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO context_files (id, name, filename, mime_type, size_bytes, is_about_you, workspace_id, created_at, updated_at)
		 VALUES ('other-doc', 'Other PRD', 'other.md', 'text/markdown', 1, 0, ?, ?, ?)`,
		otherWorkspace, now, now,
	); err != nil {
		t.Fatalf("insert context document: %v", err)
	}

	var opened *models.WSCanvasOpen
	manager := &Manager{db: db, broadcast: func(kind string, payload interface{}) {
		if kind == "canvas_open" {
			event := payload.(models.WSCanvasOpen)
			opened = &event
		}
	}}
	input, _ := json.Marshal(map[string]string{"id": "other-doc"})

	defaultHandler := manager.MakeCanvasToolHandlers("thread-1", database.DefaultWorkspaceID, "atlas")["canvas_show_document"]
	if result := defaultHandler(context.Background(), "", input); !result.IsError {
		t.Fatalf("document leaked into default workspace: %+v", result)
	}
	if opened != nil {
		t.Fatal("canvas event broadcast for inaccessible document")
	}

	otherHandler := manager.MakeCanvasToolHandlers("thread-1", otherWorkspace, "atlas")["canvas_show_document"]
	if result := otherHandler(context.Background(), "", input); result.IsError {
		t.Fatalf("show document failed: %+v", result)
	}
	if opened == nil || opened.DocumentID != "other-doc" || opened.ThreadID != "thread-1" || opened.URL != "" {
		t.Fatalf("unexpected canvas event: %+v", opened)
	}
}
