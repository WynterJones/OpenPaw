package agents

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestInboxToolsCRUDArchiveAndWorkspaceIsolation(t *testing.T) {
	db, workspaceID := newDatabaseToolsTestDB(t)
	otherWorkspace := uuid.NewString()
	_, _ = db.Exec("INSERT INTO workspaces (id, name) VALUES (?, 'Other')", otherWorkspace)

	var broadcasts int
	first := MakeInboxToolHandlers(db, workspaceID, "reporter", func(_ string, _ interface{}) { broadcasts++ })
	second := MakeInboxToolHandlers(db, otherWorkspace, "other", nil)
	_, _ = db.Exec(`INSERT INTO notifications
		(id, title, body, detail, prompt, workspace_id, priority, source_agent_slug, source_type, source_id, link)
		VALUES ('legacy-post', 'Legacy report', '', 'Visible in the Inbox', '', '', 'normal', '', 'schedule', '', '')`)

	created := callDatabaseTool(t, first, "manage_inbox_post", `{
		"action":"create",
		"title":"Five useful sites",
		"body":"A short link report",
		"detail":"- https://example.com\\n- https://openpaw.dev",
		"source_type":"schedule"
	}`)
	if created.IsError {
		t.Fatalf("create failed: %s", created.Output)
	}
	var item struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(created.Output), &item); err != nil || item.ID == "" {
		t.Fatalf("created result = %s (%v)", created.Output, err)
	}

	list := callDatabaseTool(t, first, "list_inbox_posts", `{"search":"openpaw","limit":5}`)
	if list.IsError || !strings.Contains(list.Output, "Five useful sites") || !strings.Contains(list.Output, "https://openpaw.dev") {
		t.Fatalf("list result = %s", list.Output)
	}
	legacy := callDatabaseTool(t, first, "list_inbox_posts", `{"search":"Legacy report","limit":5}`)
	if legacy.IsError || !strings.Contains(legacy.Output, "Legacy report") {
		t.Fatalf("legacy Inbox post was hidden from active workspace: %s", legacy.Output)
	}
	otherList := callDatabaseTool(t, second, "list_inbox_posts", `{"status":"all"}`)
	if strings.Contains(otherList.Output, "Five useful sites") {
		t.Fatalf("cross-workspace inbox leak: %s", otherList.Output)
	}

	updated := callDatabaseTool(t, first, "manage_inbox_post",
		`{"action":"update","id":"`+item.ID+`","title":"Curated sites","priority":"high"}`)
	if updated.IsError {
		t.Fatalf("update failed: %s", updated.Output)
	}
	archived := callDatabaseTool(t, first, "manage_inbox_post", `{"action":"archive","id":"`+item.ID+`"}`)
	if archived.IsError {
		t.Fatalf("archive failed: %s", archived.Output)
	}
	active := callDatabaseTool(t, first, "list_inbox_posts", `{}`)
	if strings.Contains(active.Output, "Curated sites") {
		t.Fatalf("archived post remained active: %s", active.Output)
	}
	archive := callDatabaseTool(t, first, "list_inbox_posts", `{"status":"archived"}`)
	if !strings.Contains(archive.Output, "Curated sites") {
		t.Fatalf("archive did not contain post: %s", archive.Output)
	}

	deleted := callDatabaseTool(t, first, "manage_inbox_post", `{"action":"delete","id":"`+item.ID+`"}`)
	if deleted.IsError {
		t.Fatalf("delete failed: %s", deleted.Output)
	}
	archive = callDatabaseTool(t, first, "list_inbox_posts", `{"status":"all"}`)
	if strings.Contains(archive.Output, "Curated sites") {
		t.Fatalf("deleted post remained: %s", archive.Output)
	}
	if broadcasts < 4 {
		t.Fatalf("broadcasts = %d, want create/update/archive/delete", broadcasts)
	}
}

func TestInboxToolDefinitionsMatchHandlers(t *testing.T) {
	db, workspaceID := newDatabaseToolsTestDB(t)
	handlers := MakeInboxToolHandlers(db, workspaceID, "test", nil)
	defs := BuildInboxToolDefs()
	if len(defs) != 2 {
		t.Fatalf("got %d definitions, want 2", len(defs))
	}
	for _, def := range defs {
		if handlers[def.Function.Name] == nil {
			t.Errorf("definition %q has no handler", def.Function.Name)
		}
	}
}
