package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/models"
)

type NotificationsHandler struct {
	db *database.DB
}

func NewNotificationsHandler(db *database.DB) *NotificationsHandler {
	return &NotificationsHandler{db: db}
}

const notificationColumns = `id, title, body, detail, prompt, workspace_id, priority,
	source_agent_slug, source_type, source_id, link, read, dismissed, created_at`

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanNotification(s rowScanner, n *models.Notification) error {
	return s.Scan(&n.ID, &n.Title, &n.Body, &n.Detail, &n.Prompt, &n.WorkspaceID, &n.Priority,
		&n.SourceAgentSlug, &n.SourceType, &n.SourceID, &n.Link, &n.Read, &n.Dismissed, &n.CreatedAt)
}

// List returns notifications newest-first.
//
// Defaults match the bell dropdown: unarchived only, capped at 100. The Inbox
// passes explicit filters — ?unread=true, ?source_type=schedule|heartbeat,
// ?archived=true to read the archive instead of the live list, and ?limit= for
// a deeper page than the bell needs.
func (h *NotificationsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	where := []string{}
	args := []interface{}{}

	if q.Get("archived") == "true" {
		where = append(where, "dismissed = 1")
	} else {
		where = append(where, "dismissed = 0")
	}
	if q.Get("unread") == "true" {
		where = append(where, "read = 0")
	}
	if st := q.Get("source_type"); st != "" {
		where = append(where, "source_type = ?")
		args = append(args, st)
	}

	// Clamped so a bad or hostile ?limit can't ask for the whole table.
	limit := 100
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		limit = min(v, 500)
	}

	query := `SELECT ` + notificationColumns + `
		FROM notifications WHERE ` + strings.Join(where, " AND ") +
		" ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list notifications")
		return
	}
	defer rows.Close()

	notifications := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		if err := scanNotification(rows, &n); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan notification")
			return
		}
		notifications = append(notifications, n)
	}
	writeJSON(w, http.StatusOK, notifications)
}

// Restore un-archives a notification, so archiving from the Inbox is undoable.
func (h *NotificationsHandler) Restore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := h.db.Exec("UPDATE notifications SET dismissed = 0 WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to restore notification")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

// MarkUnread flips a notification back to unread.
func (h *NotificationsHandler) MarkUnread(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := h.db.Exec("UPDATE notifications SET read = 0 WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark notification unread")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unread"})
}

func (h *NotificationsHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	var count int
	err := h.db.QueryRow("SELECT COUNT(*) FROM notifications WHERE read = 0 AND dismissed = 0").Scan(&count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count notifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

func (h *NotificationsHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := h.db.Exec("UPDATE notifications SET read = 1 WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark notification read")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "read"})
}

func (h *NotificationsHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	_, err := h.db.Exec("UPDATE notifications SET read = 1 WHERE read = 0")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark all notifications read")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "all_read"})
}

func (h *NotificationsHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := h.db.Exec("UPDATE notifications SET dismissed = 1 WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to dismiss notification")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "dismissed"})
}

func (h *NotificationsHandler) DismissAll(w http.ResponseWriter, r *http.Request) {
	_, err := h.db.Exec("UPDATE notifications SET dismissed = 1 WHERE dismissed = 0")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to dismiss all notifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "all_dismissed"})
}

// CreateNotification inserts a notification. Used by the scheduler and the
// heartbeat system to file a report into the Inbox.
func CreateNotification(db *database.DB, in models.NotificationInput) (*models.Notification, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	if in.Priority == "" {
		in.Priority = "normal"
	}
	if in.SourceType == "" {
		in.SourceType = "heartbeat"
	}

	_, err := db.Exec(
		`INSERT INTO notifications (id, title, body, detail, prompt, workspace_id, priority,
			source_agent_slug, source_type, source_id, link, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.Title, in.Body, in.Detail, in.Prompt, in.WorkspaceID, in.Priority,
		in.SourceAgentSlug, in.SourceType, in.SourceID, in.Link, now,
	)
	if err != nil {
		return nil, err
	}

	return &models.Notification{
		ID:              id,
		Title:           in.Title,
		Body:            in.Body,
		Detail:          in.Detail,
		Prompt:          in.Prompt,
		WorkspaceID:     in.WorkspaceID,
		Priority:        in.Priority,
		SourceAgentSlug: in.SourceAgentSlug,
		SourceType:      in.SourceType,
		SourceID:        in.SourceID,
		Link:            in.Link,
		CreatedAt:       now,
	}, nil
}

// OpenAsChat turns a report into a real chat thread on demand.
//
// Scheduled runs are threadless — the report is filed in the Inbox and nothing
// clutters the chat list unless the user actually wants to continue the
// conversation. This reconstructs that conversation (the prompt as the user
// turn, the report as the agent's reply) so replying picks up with full
// context. Calling it twice returns the thread created the first time.
func (h *NotificationsHandler) OpenAsChat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var n models.Notification
	err := scanNotification(h.db.QueryRow(
		`SELECT `+notificationColumns+` FROM notifications WHERE id = ?`, id), &n)
	if err != nil {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}

	// Already opened (or it always pointed at a thread) — reuse it, so the
	// button is idempotent rather than spawning a thread per click.
	if strings.HasPrefix(n.Link, "/chat/") {
		threadID := strings.TrimPrefix(n.Link, "/chat/")
		var exists int
		h.db.QueryRow("SELECT COUNT(*) FROM chat_threads WHERE id = ?", threadID).Scan(&exists)
		if exists > 0 {
			writeJSON(w, http.StatusOK, map[string]string{"thread_id": threadID})
			return
		}
	}

	if n.Detail == "" && n.Prompt == "" {
		writeError(w, http.StatusBadRequest, "this notification has no report to open as a chat")
		return
	}

	workspaceID := n.WorkspaceID
	if workspaceID == "" {
		workspaceID = h.db.ActiveWorkspaceID()
	}

	threadID := uuid.New().String()
	now := time.Now().UTC()
	title := n.Title
	if len(title) > 80 {
		title = title[:80]
	}

	if _, err := h.db.Exec(
		"INSERT INTO chat_threads (id, title, workspace_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		threadID, title, workspaceID, now, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create chat thread")
		return
	}

	if n.Prompt != "" {
		h.db.Exec(
			"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, created_at) VALUES (?, ?, 'user', ?, ?, ?)",
			uuid.New().String(), threadID, n.Prompt, n.SourceAgentSlug, now,
		)
	}
	if n.Detail != "" {
		// One microsecond later so the reply always sorts after the prompt.
		h.db.Exec(
			"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, created_at) VALUES (?, ?, 'assistant', ?, ?, ?)",
			uuid.New().String(), threadID, n.Detail, n.SourceAgentSlug, now.Add(time.Microsecond),
		)
	}
	if n.SourceAgentSlug != "" {
		h.db.Exec(
			"INSERT OR IGNORE INTO thread_members (thread_id, agent_role_slug, joined_at) VALUES (?, ?, ?)",
			threadID, n.SourceAgentSlug, now,
		)
	}

	// Point the notification at the thread so the Inbox now offers "Open chat"
	// and a second click can't fork a duplicate.
	h.db.Exec("UPDATE notifications SET link = ?, read = 1 WHERE id = ?", "/chat/"+threadID, id)

	h.db.LogAudit("user", "notification_open_chat", "user", "notification", id,
		"opened report as chat thread "+threadID)

	writeJSON(w, http.StatusOK, map[string]string{"thread_id": threadID})
}
