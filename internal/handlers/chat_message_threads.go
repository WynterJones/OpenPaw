package handlers

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
)

// messageThreadAnchor validates that a message belongs to a top-level chat in
// the active workspace. Focused threads cannot be nested; their context stays
// understandable and mirrors Slack's one-thread-per-message behavior.
func (h *ChatHandler) messageThreadAnchor(messageID string) (parentThreadID, content string, err error) {
	var parentID, parentParentID, body string
	err = h.db.QueryRow(
		`SELECT parent.id, COALESCE(parent.parent_thread_id, ''), message.content
		 FROM chat_messages message
		 JOIN chat_threads parent ON parent.id = message.thread_id
		 WHERE message.id = ? AND parent.workspace_id = ?`,
		messageID, activeWorkspaceID(h.db),
	).Scan(&parentID, &parentParentID, &body)
	if err != nil {
		return "", "", err
	}
	if parentParentID != "" {
		return "", "", errNestedMessageThread
	}
	return parentID, body, nil
}

var errNestedMessageThread = &messageThreadError{"threads cannot be nested"}

type messageThreadError struct{ message string }

func (e *messageThreadError) Error() string { return e.message }

func focusedThreadTitle(content string) string {
	title := strings.Join(strings.Fields(content), " ")
	if title == "" {
		return "Focused thread"
	}
	const max = 44
	if len(title) > max {
		title = title[:max] + "…"
	}
	return "Thread: " + title
}

func (h *ChatHandler) findMessageThread(messageID string) (*models.ChatThread, int, error) {
	var thread models.ChatThread
	var replyCount int
	err := h.db.QueryRow(
		`SELECT child.id, child.title, child.parent_thread_id,
		        child.root_message_id, COALESCE(SUM(reply.cost_usd), 0),
		        child.pinned, child.created_at, child.updated_at, COUNT(reply.id)
		 FROM chat_threads child
		 LEFT JOIN chat_messages reply ON reply.thread_id = child.id
		 WHERE child.root_message_id = ?
		 GROUP BY child.id`,
		messageID,
	).Scan(
		&thread.ID, &thread.Title, &thread.ParentThreadID,
		&thread.RootMessageID, &thread.TotalCostUSD, &thread.Pinned,
		&thread.CreatedAt, &thread.UpdatedAt, &replyCount,
	)
	if err != nil {
		return nil, 0, err
	}
	return &thread, replyCount, nil
}

func (h *ChatHandler) ensureMessageThread(parentThreadID, messageID, content, workspaceID string) (*models.ChatThread, bool, error) {
	if thread, _, err := h.findMessageThread(messageID); err == nil {
		return thread, false, nil
	} else if err != sql.ErrNoRows {
		return nil, false, err
	}

	id := generateID()
	now := time.Now().UTC()
	result, err := h.db.Exec(
		`INSERT OR IGNORE INTO chat_threads
		    (id, title, workspace_id, parent_thread_id, root_message_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, focusedThreadTitle(content), workspaceID,
		parentThreadID, messageID, now, now,
	)
	if err != nil {
		return nil, false, err
	}
	created, _ := result.RowsAffected()
	thread, _, err := h.findMessageThread(messageID)
	if err != nil {
		return nil, false, err
	}
	if created > 0 {
		h.addThreadMember(thread.ID, "builder")
	}
	return thread, created > 0, nil
}

// notifyMessageThreadUpdated keeps the parent transcript's reply badge current
// whether a reply came from the user or from an agent-to-agent handoff.
func (h *ChatHandler) notifyMessageThreadUpdated(threadID string) {
	var parentThreadID, rootMessageID string
	if err := h.db.QueryRow(
		`SELECT parent_thread_id, root_message_id FROM chat_threads
		 WHERE id = ? AND parent_thread_id != '' AND root_message_id != ''`,
		threadID,
	).Scan(&parentThreadID, &rootMessageID); err != nil {
		return
	}
	var count int
	h.db.QueryRow("SELECT COUNT(*) FROM chat_messages WHERE thread_id = ?", threadID).Scan(&count)
	h.db.Exec("UPDATE chat_threads SET updated_at = ? WHERE id = ?", time.Now().UTC(), parentThreadID)
	if h.agentManager != nil {
		h.agentManager.Broadcast("message_thread_updated", map[string]interface{}{
			"thread_id":       parentThreadID,
			"child_thread_id": threadID,
			"root_message_id": rootMessageID,
			"reply_count":     count,
		})
	}
}

// GetMessageThread returns the focused child chat, if one has actually been
// started. Merely opening the panel does not create an empty database row.
func (h *ChatHandler) GetMessageThread(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageId")
	if _, _, err := h.messageThreadAnchor(messageID); err != nil {
		if err == errNestedMessageThread {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusNotFound, "message not found")
		return
	}

	thread, replyCount, err := h.findMessageThread(messageID)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"thread":      nil,
			"reply_count": 0,
		})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load message thread")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"thread":      thread,
		"reply_count": replyCount,
	})
}

// CreateMessageThread creates (or returns) the one child chat anchored to a
// message. The child is a normal chat internally, so all existing routing,
// @mentions, streaming, tools, costs, and stop behavior work unchanged.
func (h *ChatHandler) CreateMessageThread(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageId")
	parentThreadID, content, err := h.messageThreadAnchor(messageID)
	if err != nil {
		if err == errNestedMessageThread {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusNotFound, "message not found")
		return
	}

	thread, created, err := h.ensureMessageThread(
		parentThreadID, messageID, content, activeWorkspaceID(h.db),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create message thread")
		return
	}
	if created {
		userID := middleware.GetUserID(r.Context())
		h.db.LogAudit(userID, "chat_message_thread_created", "chat", "chat_thread", thread.ID, parentThreadID)
		writeJSON(w, http.StatusCreated, thread)
		return
	}
	writeJSON(w, http.StatusOK, thread)
}
