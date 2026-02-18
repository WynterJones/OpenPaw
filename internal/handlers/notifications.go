package handlers

import (
	"net/http"
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

func (h *NotificationsHandler) List(w http.ResponseWriter, r *http.Request) {
	unreadOnly := r.URL.Query().Get("unread") == "true"

	query := `SELECT id, title, body, priority, source_agent_slug, source_type, link, read, dismissed, created_at
		FROM notifications WHERE dismissed = 0`
	if unreadOnly {
		query += " AND read = 0"
	}
	query += " ORDER BY created_at DESC LIMIT 100"

	rows, err := h.db.Query(query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list notifications")
		return
	}
	defer rows.Close()

	notifications := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.ID, &n.Title, &n.Body, &n.Priority, &n.SourceAgentSlug, &n.SourceType, &n.Link, &n.Read, &n.Dismissed, &n.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan notification")
			return
		}
		notifications = append(notifications, n)
	}
	writeJSON(w, http.StatusOK, notifications)
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

// CreateNotification is used by the heartbeat system to insert a notification.
func CreateNotification(db *database.DB, title, body, priority, sourceAgentSlug, sourceType, link string) (*models.Notification, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	if priority == "" {
		priority = "normal"
	}
	if sourceType == "" {
		sourceType = "heartbeat"
	}

	_, err := db.Exec(
		`INSERT INTO notifications (id, title, body, priority, source_agent_slug, source_type, link, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, title, body, priority, sourceAgentSlug, sourceType, link, now,
	)
	if err != nil {
		return nil, err
	}

	return &models.Notification{
		ID:              id,
		Title:           title,
		Body:            body,
		Priority:        priority,
		SourceAgentSlug: sourceAgentSlug,
		SourceType:      sourceType,
		Link:            link,
		CreatedAt:       now,
	}, nil
}
