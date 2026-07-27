package agents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openpaw/openpaw/internal/database"
)

func newContextTestDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	dataDir := t.TempDir()
	db, err := database.New(dataDir)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, dataDir
}

func callContextTool(t *testing.T, handlers map[string]llmHandlerMap, name string, args map[string]interface{}) string {
	t.Helper()
	input, _ := json.Marshal(args)
	return handlers[name](context.Background(), "", input)
}

// llmHandlerMap keeps the test readable without importing the llm package's
// signature everywhere.
type llmHandlerMap func(ctx context.Context, workDir string, input json.RawMessage) string

func toolFuncs(t *testing.T, db *database.DB, dataDir string) map[string]llmHandlerMap {
	t.Helper()
	raw := MakeContextToolHandlers(db, dataDir, "atlas", nil)
	out := map[string]llmHandlerMap{}
	for name, h := range raw {
		handler := h
		out[name] = func(ctx context.Context, workDir string, input json.RawMessage) string {
			return handler(ctx, workDir, input).Output
		}
	}
	return out
}

// An agent could list documents and overwrite them but never see one, so
// "add a section to that doc" in a later conversation meant rewriting it blind.
func TestReadContextDocument_RoundTrip(t *testing.T) {
	db, dataDir := newContextTestDB(t)
	tools := toolFuncs(t, db, dataDir)

	created := callContextTool(t, tools, "create_context_document", map[string]interface{}{
		"name":    "Feedback Spec",
		"content": "# Feedback Spec\n\nOriginal body.",
	})
	var createdDoc struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(created), &createdDoc); err != nil {
		t.Fatalf("create returned %q: %v", created, err)
	}

	out := callContextTool(t, tools, "read_context_document", map[string]interface{}{"id": createdDoc.ID})
	var doc struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("read returned %q: %v", out, err)
	}
	if doc.Name != "Feedback Spec" || !strings.Contains(doc.Content, "Original body.") {
		t.Errorf("read back %+v", doc)
	}

	// Agents are shown document names, not IDs — a name has to work too.
	byName := callContextTool(t, tools, "read_context_document", map[string]interface{}{"id": "feedback spec"})
	if !strings.Contains(byName, "Original body.") {
		t.Errorf("lookup by name failed: %s", byName)
	}
}

// The whole point: the user edits these documents by hand between
// conversations, and the agent has to see what they wrote.
func TestReadContextDocument_SeesUserEdits(t *testing.T) {
	db, dataDir := newContextTestDB(t)
	tools := toolFuncs(t, db, dataDir)

	created := callContextTool(t, tools, "create_context_document", map[string]interface{}{
		"name":    "Notes",
		"content": "agent wrote this",
	})
	var createdDoc struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(created), &createdDoc)

	// Exactly what the Context tab does when the user saves an edit
	// (handlers/context.go UpdateFile writes this same path).
	path := filepath.Join(dataDir, "context", createdDoc.ID+".md")
	if err := os.WriteFile(path, []byte("the user rewrote this by hand"), 0644); err != nil {
		t.Fatalf("simulate user edit: %v", err)
	}

	out := callContextTool(t, tools, "read_context_document", map[string]interface{}{"id": createdDoc.ID})
	if !strings.Contains(out, "the user rewrote this by hand") {
		t.Errorf("read did not pick up the user's edit: %s", out)
	}
	if strings.Contains(out, "agent wrote this") {
		t.Errorf("read served stale content: %s", out)
	}
}

func TestReadContextDocument_UnknownAndBinary(t *testing.T) {
	db, dataDir := newContextTestDB(t)
	tools := toolFuncs(t, db, dataDir)

	if out := callContextTool(t, tools, "read_context_document", map[string]interface{}{"id": "nope"}); !strings.Contains(out, "No document") {
		t.Errorf("unknown document not reported: %s", out)
	}

	if _, err := db.Exec(
		"INSERT INTO context_files (id, name, filename, mime_type, size_bytes, is_about_you, created_at, updated_at) VALUES ('pdf-1', 'Report', 'pdf-1.pdf', 'application/pdf', 10, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
	); err != nil {
		t.Fatalf("insert pdf row: %v", err)
	}
	if out := callContextTool(t, tools, "read_context_document", map[string]interface{}{"id": "pdf-1"}); !strings.Contains(out, "cannot be read as text") {
		t.Errorf("binary file was not refused: %s", out)
	}
}
