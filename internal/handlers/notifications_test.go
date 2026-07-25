package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/models"
)

func decodeTestJSON(t *testing.T, rec *httptest.ResponseRecorder, out interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
}

func newTestNotificationsHandler(t *testing.T) *NotificationsHandler {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &NotificationsHandler{db: db}
}

// openAsChat drives the handler through a router so chi's URL param is populated
// the same way it is in production.
func openAsChat(t *testing.T, h *NotificationsHandler, id string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/notifications/{id}/open-chat", h.OpenAsChat)
	req := httptest.NewRequest(http.MethodPost, "/notifications/"+id+"/open-chat", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestOpenAsChat_BuildsThreadFromReport(t *testing.T) {
	h := newTestNotificationsHandler(t)

	n, err := CreateNotification(h.db, models.NotificationInput{
		Title:           "Research Assistant: Morning digest",
		Body:            "Three items worth your attention",
		Detail:          "## Digest\n\nThree items worth your attention.",
		Prompt:          "Summarise overnight activity",
		SourceAgentSlug: "research",
		SourceType:      "schedule",
	})
	if err != nil {
		t.Fatalf("create notification: %v", err)
	}

	rec := openAsChat(t, h, n.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	var threadID string
	h.db.QueryRow("SELECT id FROM chat_threads ORDER BY created_at DESC LIMIT 1").Scan(&threadID)
	if threadID == "" {
		t.Fatal("no chat thread was created")
	}

	// The prompt must land as the user turn and the report as the reply, in that
	// order — otherwise a follow-up reply reads the conversation backwards.
	rows, err := h.db.Query(
		"SELECT role, content FROM chat_messages WHERE thread_id = ? ORDER BY created_at ASC", threadID)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	defer rows.Close()

	var got []struct{ role, content string }
	for rows.Next() {
		var role, content string
		rows.Scan(&role, &content)
		got = append(got, struct{ role, content string }{role, content})
	}

	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2: %+v", len(got), got)
	}
	if got[0].role != "user" || got[0].content != "Summarise overnight activity" {
		t.Errorf("first message = %+v, want the prompt as a user turn", got[0])
	}
	if got[1].role != "assistant" || got[1].content != "## Digest\n\nThree items worth your attention." {
		t.Errorf("second message = %+v, want the report as an assistant turn", got[1])
	}
}

// Clicking the button twice must not fork a second thread.
func TestOpenAsChat_IsIdempotent(t *testing.T) {
	h := newTestNotificationsHandler(t)

	n, _ := CreateNotification(h.db, models.NotificationInput{
		Title:  "Report",
		Detail: "Body of the report",
		Prompt: "Do the thing",
	})

	first := openAsChat(t, h, n.ID)
	second := openAsChat(t, h, n.ID)
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("statuses = %d, %d, want 200 both", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Errorf("second call returned a different thread:\n first  = %s\n second = %s",
			first.Body.String(), second.Body.String())
	}

	var threads int
	h.db.QueryRow("SELECT COUNT(*) FROM chat_threads").Scan(&threads)
	if threads != 1 {
		t.Errorf("thread count = %d, want 1", threads)
	}
}

// Opening marks the report read and links it, so the Inbox switches from
// "Open as chat" to "Open chat".
func TestOpenAsChat_LinksAndMarksRead(t *testing.T) {
	h := newTestNotificationsHandler(t)

	n, _ := CreateNotification(h.db, models.NotificationInput{Title: "R", Detail: "d", Prompt: "p"})
	openAsChat(t, h, n.ID)

	var link string
	var read bool
	h.db.QueryRow("SELECT link, read FROM notifications WHERE id = ?", n.ID).Scan(&link, &read)
	if link == "" {
		t.Error("link was not set after opening as chat")
	}
	if !read {
		t.Error("report was not marked read after opening as chat")
	}
}

// A heartbeat notification that only ever pointed at a chat has nothing to
// reconstruct — but the button should still take you to that chat.
func TestOpenAsChat_ReusesExistingThread(t *testing.T) {
	h := newTestNotificationsHandler(t)

	threadID := generateID()
	h.db.Exec("INSERT INTO chat_threads (id, title, created_at, updated_at) VALUES (?, 'Existing', datetime('now'), datetime('now'))", threadID)

	n, _ := CreateNotification(h.db, models.NotificationInput{
		Title: "Heartbeat note",
		Body:  "something happened",
		Link:  "/chat/" + threadID,
	})

	rec := openAsChat(t, h, n.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var threads int
	h.db.QueryRow("SELECT COUNT(*) FROM chat_threads").Scan(&threads)
	if threads != 1 {
		t.Errorf("thread count = %d, want 1 — a new thread was created instead of reusing", threads)
	}
}

// An empty report has nothing to open; that must be an error rather than a
// blank thread appearing in the chat list.
func TestOpenAsChat_RejectsEmptyReport(t *testing.T) {
	h := newTestNotificationsHandler(t)

	n, _ := CreateNotification(h.db, models.NotificationInput{Title: "Empty"})

	rec := openAsChat(t, h, n.ID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}

	var threads int
	h.db.QueryRow("SELECT COUNT(*) FROM chat_threads").Scan(&threads)
	if threads != 0 {
		t.Errorf("thread count = %d, want 0", threads)
	}
}

func TestList_FiltersBySourceAndState(t *testing.T) {
	h := newTestNotificationsHandler(t)

	CreateNotification(h.db, models.NotificationInput{Title: "sched", SourceType: "schedule"})
	CreateNotification(h.db, models.NotificationInput{Title: "beat", SourceType: "heartbeat"})
	archived, _ := CreateNotification(h.db, models.NotificationInput{Title: "old", SourceType: "schedule"})
	h.db.Exec("UPDATE notifications SET dismissed = 1 WHERE id = ?", archived.ID)

	list := func(query string) []models.Notification {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/notifications"+query, nil)
		rec := httptest.NewRecorder()
		h.List(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d for %q", rec.Code, query)
		}
		var out []models.Notification
		decodeTestJSON(t, rec, &out)
		return out
	}

	if got := list(""); len(got) != 2 {
		t.Errorf("default list = %d items, want 2 (archived excluded)", len(got))
	}
	if got := list("?source_type=schedule"); len(got) != 1 || got[0].Title != "sched" {
		t.Errorf("schedule filter = %+v, want just the schedule notification", got)
	}
	if got := list("?archived=true"); len(got) != 1 || got[0].Title != "old" {
		t.Errorf("archived list = %+v, want just the archived notification", got)
	}
}
