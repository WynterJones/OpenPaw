package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/models"
)

func messageThreadTestRouter(h *ChatHandler) http.Handler {
	r := chi.NewRouter()
	r.Get("/chat/threads", h.ListThreads)
	r.Get("/chat/threads/{id}/messages", h.GetMessages)
	r.Get("/chat/messages/{messageId}/thread", h.GetMessageThread)
	r.Post("/chat/messages/{messageId}/thread", h.CreateMessageThread)
	return r
}

func seedMessageThreadAnchor(t *testing.T, h *ChatHandler, workspaceID string) (string, string) {
	t.Helper()
	threadID := "parent-" + strings.ReplaceAll(workspaceID, "-", "")
	messageID := "message-" + strings.ReplaceAll(workspaceID, "-", "")
	if _, err := h.db.Exec(
		`INSERT INTO chat_threads (id, title, workspace_id) VALUES (?, 'Parent chat', ?)`,
		threadID, workspaceID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(
		`INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug)
		 VALUES (?, ?, 'assistant', 'A detailed answer worth discussing', 'researcher')`,
		messageID, threadID,
	); err != nil {
		t.Fatal(err)
	}
	return threadID, messageID
}

func TestMessageThreadCreatesOneFocusedChildAndHidesItFromChatList(t *testing.T) {
	h := newTestHandler(t)
	parentID, messageID := seedMessageThreadAnchor(t, h, database.DefaultWorkspaceID)
	router := messageThreadTestRouter(h)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/chat/messages/"+messageID+"/thread", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			t.Fatalf("create attempt %d: status %d body %s", i+1, rec.Code, rec.Body.String())
		}
	}

	var childCount int
	if err := h.db.QueryRow(
		"SELECT COUNT(*) FROM chat_threads WHERE parent_thread_id = ? AND root_message_id = ?",
		parentID, messageID,
	).Scan(&childCount); err != nil {
		t.Fatal(err)
	}
	if childCount != 1 {
		t.Fatalf("expected exactly one focused child, got %d", childCount)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/chat/threads", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list threads: status %d body %s", rec.Code, rec.Body.String())
	}
	var listed []models.ChatThread
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != parentID {
		t.Fatalf("focused child leaked into main chat list: %#v", listed)
	}
}

func TestMessageThreadIsWorkspaceScopedAndCannotNest(t *testing.T) {
	h := newTestHandler(t)
	_, otherMessageID := seedMessageThreadAnchor(t, h, "other-workspace")
	router := messageThreadTestRouter(h)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/chat/messages/"+otherMessageID+"/thread", strings.NewReader(`{}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace anchor status = %d, want 404; body %s", rec.Code, rec.Body.String())
	}

	_, messageID := seedMessageThreadAnchor(t, h, database.DefaultWorkspaceID)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, httptest.NewRequest(http.MethodPost, "/chat/messages/"+messageID+"/thread", strings.NewReader(`{}`)))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create child: status %d body %s", createRec.Code, createRec.Body.String())
	}
	var child models.ChatThread
	if err := json.Unmarshal(createRec.Body.Bytes(), &child); err != nil {
		t.Fatal(err)
	}
	nestedMessageID := "nested-message"
	if _, err := h.db.Exec(
		`INSERT INTO chat_messages (id, thread_id, role, content)
		 VALUES (?, ?, 'user', 'Do not create a thread under this reply')`,
		nestedMessageID, child.ID,
	); err != nil {
		t.Fatal(err)
	}
	nestedRec := httptest.NewRecorder()
	router.ServeHTTP(nestedRec, httptest.NewRequest(http.MethodPost, "/chat/messages/"+nestedMessageID+"/thread", strings.NewReader(`{}`)))
	if nestedRec.Code != http.StatusBadRequest {
		t.Fatalf("nested thread status = %d, want 400; body %s", nestedRec.Code, nestedRec.Body.String())
	}
}

func TestFocusedThreadHistoryContainsAnchorButNotParentConversation(t *testing.T) {
	h := newTestHandler(t)
	parentID, messageID := seedMessageThreadAnchor(t, h, database.DefaultWorkspaceID)
	if _, err := h.db.Exec(
		`INSERT INTO chat_messages (id, thread_id, role, content)
		 VALUES ('unrelated-parent-message', ?, 'user', 'SECRET PARENT CONTEXT')`,
		parentID,
	); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	messageThreadTestRouter(h).ServeHTTP(
		rec,
		httptest.NewRequest(http.MethodPost, "/chat/messages/"+messageID+"/thread", strings.NewReader(`{}`)),
	)
	var child models.ChatThread
	if err := json.Unmarshal(rec.Body.Bytes(), &child); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(
		`INSERT INTO chat_messages (id, thread_id, role, content)
		 VALUES ('focused-reply', ?, 'user', 'Question inside the focused thread')`,
		child.ID,
	); err != nil {
		t.Fatal(err)
	}

	history := h.fetchThreadHistory(child.ID)
	joined := ""
	for _, message := range history {
		joined += "\n" + message.Content
	}
	if !strings.Contains(joined, "A detailed answer worth discussing") {
		t.Fatalf("focused history omitted anchor: %s", joined)
	}
	if !strings.Contains(joined, "Question inside the focused thread") {
		t.Fatalf("focused history omitted reply: %s", joined)
	}
	if strings.Contains(joined, "SECRET PARENT CONTEXT") {
		t.Fatalf("focused history leaked unrelated parent context: %s", joined)
	}
}

func TestGetMessagesIncludesFocusedThreadReplyCount(t *testing.T) {
	h := newTestHandler(t)
	parentID, messageID := seedMessageThreadAnchor(t, h, database.DefaultWorkspaceID)
	createRec := httptest.NewRecorder()
	messageThreadTestRouter(h).ServeHTTP(
		createRec,
		httptest.NewRequest(http.MethodPost, "/chat/messages/"+messageID+"/thread", strings.NewReader(`{}`)),
	)
	var child models.ChatThread
	if err := json.Unmarshal(createRec.Body.Bytes(), &child); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"reply-one", "reply-two"} {
		if _, err := h.db.Exec(
			"INSERT INTO chat_messages (id, thread_id, role, content) VALUES (?, ?, 'user', 'reply')",
			id, child.ID,
		); err != nil {
			t.Fatal(err)
		}
	}

	rec := httptest.NewRecorder()
	messageThreadTestRouter(h).ServeHTTP(
		rec,
		httptest.NewRequest(http.MethodGet, "/chat/threads/"+parentID+"/messages", nil),
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("get parent messages: status %d body %s", rec.Code, rec.Body.String())
	}
	var messages []models.ChatMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("got %d parent messages, want 1", len(messages))
	}
	if messages[0].ChildThreadID != child.ID || messages[0].ThreadReplyCount != 2 {
		t.Fatalf("thread metadata = id %q count %d", messages[0].ChildThreadID, messages[0].ThreadReplyCount)
	}
}

func TestAgentMentionBranchesOnceThenStaysInFocusedThread(t *testing.T) {
	h := newTestHandler(t)
	parentID, rootMessageID := seedMessageThreadAnchor(t, h, database.DefaultWorkspaceID)

	childID, err := h.agentMentionThread(parentID, rootMessageID, "Please ask @reviewer to check this")
	if err != nil {
		t.Fatal(err)
	}
	if childID == parentID {
		t.Fatal("top-level agent mention did not branch into a focused thread")
	}

	replyMessageID := "agent-reply-inside-child"
	if _, err := h.db.Exec(
		`INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug)
		 VALUES (?, ?, 'assistant', 'I will ask @researcher here', 'reviewer')`,
		replyMessageID, childID,
	); err != nil {
		t.Fatal(err)
	}
	nextTarget, err := h.agentMentionThread(childID, replyMessageID, "I will ask @researcher here")
	if err != nil {
		t.Fatal(err)
	}
	if nextTarget != childID {
		t.Fatalf("mention inside focused thread targeted %q, want existing child %q", nextTarget, childID)
	}

	var grandchildren int
	if err := h.db.QueryRow(
		"SELECT COUNT(*) FROM chat_threads WHERE parent_thread_id = ?", childID,
	).Scan(&grandchildren); err != nil {
		t.Fatal(err)
	}
	if grandchildren != 0 {
		t.Fatalf("created %d nested threads, want none", grandchildren)
	}
}
