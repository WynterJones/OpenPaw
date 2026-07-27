package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
)

// newBuildTestHandler wires a real agent manager, since filing a build
// broadcasts thread-member and message events.
func newBuildTestHandler(t *testing.T, db *database.DB) *ChatHandler {
	t.Helper()
	mgr := agents.NewManager(db, t.TempDir(), func(string, interface{}) {}, nil)
	return NewChatHandler(db, mgr, t.TempDir(), t.TempDir())
}

func confirmationCard(t *testing.T, db *database.DB, threadID string) map[string]string {
	t.Helper()
	rows, err := db.Query(
		"SELECT content FROM chat_messages WHERE thread_id = ? ORDER BY created_at DESC", threadID,
	)
	if err != nil {
		t.Fatalf("read messages: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var content string
		if rows.Scan(&content) != nil {
			continue
		}
		var card map[string]string
		if json.Unmarshal([]byte(content), &card) == nil && card["__type"] == "confirmation" {
			return card
		}
	}
	t.Fatalf("no confirmation card filed on thread %s", threadID)
	return nil
}

// An agent that has just worked out what a service should do can file the build
// itself. Before this it could only tell the user to go and ask the Gateway,
// which then had to re-derive the whole spec from the conversation — the point
// where "I asked it to build the service and it said it couldn't" came from.
func TestRequestBuild_FilesAConfirmationCard(t *testing.T) {
	db := newTestDB(t)
	h := newBuildTestHandler(t, db)

	if _, err := db.Exec("INSERT INTO chat_threads (id, title) VALUES ('t1', 'Feedback API')"); err != nil {
		t.Fatalf("insert thread: %v", err)
	}

	out, err := h.RequestBuild(
		context.Background(), "t1", "service",
		"MarketingSecretsAI Feedback Lookup",
		"Wraps the admin feedback API.",
		"GET /reports/{id} returns one report; GET /reports?section=... lists the latest.",
	)
	if err != nil {
		t.Fatalf("RequestBuild: %v", err)
	}

	// The agent must be told to stop, or it files the same build on every turn.
	if !strings.Contains(strings.ToLower(out), "stop") {
		t.Errorf("tool output does not tell the agent to stop: %q", out)
	}

	card := confirmationCard(t, db, "t1")
	if card["action"] != "build_tool" {
		t.Errorf("action = %q, want build_tool", card["action"])
	}
	if card["action_label"] != "New Service" {
		t.Errorf("label = %q, want New Service", card["action_label"])
	}
	if card["title"] != "MarketingSecretsAI Feedback Lookup" {
		t.Errorf("title = %q", card["title"])
	}

	// The requirements have to reach the work order — the builder never sees
	// the conversation they were gathered in.
	var requirements string
	db.QueryRow("SELECT requirements FROM work_orders WHERE id = ?", card["work_order_id"]).Scan(&requirements)
	if !strings.Contains(requirements, "GET /reports/{id}") {
		t.Errorf("requirements did not reach the work order: %q", requirements)
	}
}

// A "service" whose name is an existing dashboard is a dashboard update, the
// same as when the gateway asks. An agent gets the same correction.
func TestRequestBuild_RetargetsAnExistingDashboard(t *testing.T) {
	db := newTestDB(t)
	h := newBuildTestHandler(t, db)

	if _, err := db.Exec("INSERT INTO chat_threads (id, title) VALUES ('t1', 'Stack')"); err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	insertTestDashboard(t, db, "dash-1", "Revenue Overview", "custom")

	if _, err := h.RequestBuild(
		context.Background(), "t1", "service", "Revenue Overview", "", "Add a monthly total.",
	); err != nil {
		t.Fatalf("RequestBuild: %v", err)
	}

	card := confirmationCard(t, db, "t1")
	if card["action"] != "build_custom_dashboard" {
		t.Errorf("action = %q, want build_custom_dashboard", card["action"])
	}
	if card["action_label"] != "Update Dashboard" {
		t.Errorf("label = %q, want Update Dashboard", card["action_label"])
	}
}

// Naming a service that already exists is an update, not a second copy of it
// sitting alongside the first.
func TestRequestBuild_ExistingServiceBecomesAnUpdate(t *testing.T) {
	db := newTestDB(t)
	h := newBuildTestHandler(t, db)

	if _, err := db.Exec("INSERT INTO chat_threads (id, title) VALUES ('t1', 'Weather')"); err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO tools (id, name, description, type, config, enabled, status) VALUES ('tool-1', 'Weather Service', '', 'custom', '{}', 1, 'ready')",
	); err != nil {
		t.Fatalf("insert tool: %v", err)
	}

	if _, err := h.RequestBuild(
		context.Background(), "t1", "service", "Weather Service", "", "Add a forecast endpoint.",
	); err != nil {
		t.Fatalf("RequestBuild: %v", err)
	}

	card := confirmationCard(t, db, "t1")
	if card["action"] != "update_tool" {
		t.Errorf("action = %q, want update_tool", card["action"])
	}
	if card["action_label"] != "Update Service" {
		t.Errorf("label = %q, want Update Service", card["action_label"])
	}
}
