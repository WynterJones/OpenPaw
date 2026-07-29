package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
)

const (
	maxThreadTitleLength = 50
	threadHistoryLimit   = 10
	gatewayHistoryLimit  = 4
)

// MemoryReflector reviews a finished exchange and saves what is worth
// remembering. Nil-safe at every call site — memory capture is an enhancement to
// a reply that has already been delivered, never a precondition for one.
type MemoryReflector interface {
	Reflect(agentSlug, userMessage, agentResponse string)
}

type ChatHandler struct {
	db               *database.DB
	agentManager     *agents.Manager
	toolsDir         string
	dataDir          string
	dashboardsDir    string
	threadCancels    sync.Map // map[threadID]context.CancelFunc
	compactingGuard  sync.Map // map[threadID]bool — prevents double-compaction
	multiAgentActive sync.Map // map[threadID]bool — suppresses per-agent agent_completed during multi-agent sequences
	roleCache        struct {
		sync.RWMutex
		roles     []struct{ slug, name string }
		expiresAt time.Time
	}

	// Reflector captures memories after each reply. Assigned after construction
	// (see server.go) because it depends on the agent manager this handler holds.
	Reflector MemoryReflector
}

func truncateStr(s string, max int, ellipsis bool) string {
	if len(s) <= max {
		return s
	}
	if ellipsis {
		return s[:max] + "..."
	}
	return s[:max]
}

func NewChatHandler(db *database.DB, agentManager *agents.Manager, toolsDir, dataDir string) *ChatHandler {
	dashboardsDir := filepath.Join(dataDir, "..", "dashboards")
	return &ChatHandler{db: db, agentManager: agentManager, toolsDir: toolsDir, dataDir: dataDir, dashboardsDir: dashboardsDir}
}

// parseAggregatedTimestamp handles timestamps returned by SQLite expressions
// such as MAX(scanned_at). Unlike a direct TIMESTAMP column, an aggregate has
// no declared type, so mattn/go-sqlite3 returns it as text instead of time.Time.
func parseAggregatedTimestamp(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	layouts := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}

func (h *ChatHandler) ListThreads(w http.ResponseWriter, r *http.Request) {
	limit := 100
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &limit); n == 1 && err == nil && limit > 0 {
			if limit > 500 {
				limit = 500
			}
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}
	// ?pinned=1 / ?pinned=0 backs the All Chats / Pinned tabs; omitted returns both.
	pinnedFilter := ""
	args := []interface{}{activeWorkspaceID(h.db)}
	switch r.URL.Query().Get("pinned") {
	case "1", "true":
		pinnedFilter = " AND t.pinned = 1"
	case "0", "false":
		pinnedFilter = " AND t.pinned = 0"
	}
	args = append(args, limit, offset)

	// The dream_scans join is grouped by thread rather than joined directly: a
	// conversation several agents took part in has one scan row per agent, and
	// a plain join would duplicate the thread once per scan.
	rows, err := h.db.Query(
		`SELECT t.id, t.title, COALESCE(c.cost, 0), t.pinned, t.created_at, t.updated_at, d.scanned_at
		 FROM chat_threads t
		 LEFT JOIN (SELECT thread_id, SUM(cost_usd) AS cost FROM chat_messages GROUP BY thread_id) c ON c.thread_id = t.id
		 LEFT JOIN (SELECT thread_id, MAX(scanned_at) AS scanned_at FROM dream_scans GROUP BY thread_id) d ON d.thread_id = t.id
		 WHERE t.workspace_id = ?`+pinnedFilter+`
		 ORDER BY t.updated_at DESC LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list threads")
		return
	}
	defer rows.Close()

	threads := []models.ChatThread{}
	for rows.Next() {
		var t models.ChatThread
		var dreamedAt sql.NullString
		if err := rows.Scan(&t.ID, &t.Title, &t.TotalCostUSD, &t.Pinned, &t.CreatedAt, &t.UpdatedAt, &dreamedAt); err != nil {
			logger.Error("Failed to scan chat thread: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to scan thread")
			return
		}
		if dreamedAt.Valid {
			t.Dreamed = true
			if at, err := parseAggregatedTimestamp(dreamedAt.String); err == nil {
				t.DreamedAt = &at
			} else {
				// A damaged scan marker must never hide an otherwise healthy
				// conversation. Keep the dreamed badge and omit only its time.
				logger.Warn("Chat %s has an unreadable dream timestamp: %v", t.ID, err)
			}
		}
		threads = append(threads, t)
	}
	writeJSON(w, http.StatusOK, threads)
}

// ActiveThreadIds returns all thread IDs that currently have active work orders or streaming agents.
func (h *ChatHandler) ActiveThreadIds(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		`SELECT DISTINCT thread_id FROM work_orders WHERE status IN (?, ?)`,
		string(agents.WorkOrderPending), string(agents.WorkOrderInProgress),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query active threads")
		return
	}
	defer rows.Close()

	seen := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			seen[id] = true
		}
	}

	// Also include threads with active routing (non-builder chats)
	h.threadCancels.Range(func(key, _ interface{}) bool {
		if id, ok := key.(string); ok {
			seen[id] = true
		}
		return true
	})

	active := make([]string, 0, len(seen))
	for id := range seen {
		active = append(active, id)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active_thread_ids": active,
	})
}

func (h *ChatHandler) CreateThread(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		req.Title = "New Chat"
	}

	id := generateID()
	now := time.Now().UTC()

	_, err := h.db.Exec(
		"INSERT INTO chat_threads (id, title, workspace_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		id, req.Title, activeWorkspaceID(h.db), now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create thread")
		return
	}

	// The Gateway is a participant of every chat — add it up front so a new,
	// empty thread already shows Gateway in "Agents in chat".
	h.addThreadMember(id, "builder")

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "chat_thread_created", "chat", "chat_thread", id, req.Title)

	writeJSON(w, http.StatusCreated, models.ChatThread{
		ID:        id,
		Title:     req.Title,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// archiveMessageCosts moves the cost/token totals of specific messages from the
// live counters to the archived ones. Compaction removes only part of a thread,
// so archiving the whole thread's costs would double-count the messages that
// are still present.
func archiveMessageCosts(db *database.DB, messageIDs []string) {
	if len(messageIDs) == 0 {
		return
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(messageIDs)), ",")
	args := make([]interface{}, len(messageIDs))
	for i, id := range messageIDs {
		args[i] = id
	}

	var cost float64
	var inTok, outTok int
	db.QueryRow(
		"SELECT COALESCE(SUM(cost_usd),0), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0) FROM chat_messages WHERE id IN ("+placeholders+")",
		args...,
	).Scan(&cost, &inTok, &outTok)

	if cost <= 0 && inTok <= 0 && outTok <= 0 {
		return
	}
	if _, err := db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'archived_cost_usd'", cost); err != nil {
		logger.Error("Failed to archive cost: %v", err)
	}
	if _, err := db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'archived_input_tokens'", float64(inTok)); err != nil {
		logger.Error("Failed to archive input tokens: %v", err)
	}
	if _, err := db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'archived_output_tokens'", float64(outTok)); err != nil {
		logger.Error("Failed to archive output tokens: %v", err)
	}
	db.Exec("UPDATE system_stats SET value = value - ? WHERE key = 'live_cost_usd'", cost)
	db.Exec("UPDATE system_stats SET value = value - ? WHERE key = 'live_input_tokens'", float64(inTok))
	db.Exec("UPDATE system_stats SET value = value - ? WHERE key = 'live_output_tokens'", float64(outTok))
}

func archiveThreadCosts(db *database.DB, threadID string) {
	var cost float64
	var inTok, outTok int
	db.QueryRow(
		"SELECT COALESCE(SUM(cost_usd),0), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0) FROM chat_messages WHERE thread_id = ?",
		threadID,
	).Scan(&cost, &inTok, &outTok)

	if cost > 0 || inTok > 0 || outTok > 0 {
		if _, err := db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'archived_cost_usd'", cost); err != nil {
			logger.Error("Failed to archive cost: %v", err)
		}
		if _, err := db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'archived_input_tokens'", float64(inTok)); err != nil {
			logger.Error("Failed to archive input tokens: %v", err)
		}
		if _, err := db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'archived_output_tokens'", float64(outTok)); err != nil {
			logger.Error("Failed to archive output tokens: %v", err)
		}
		// Decrement live counters since these messages are being removed
		db.Exec("UPDATE system_stats SET value = value - ? WHERE key = 'live_cost_usd'", cost)
		db.Exec("UPDATE system_stats SET value = value - ? WHERE key = 'live_input_tokens'", float64(inTok))
		db.Exec("UPDATE system_stats SET value = value - ? WHERE key = 'live_output_tokens'", float64(outTok))
	}
}

func (h *ChatHandler) DeleteThread(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Archive cost/token stats before deletion
	archiveThreadCosts(h.db, id)

	// Delete reactions, members, messages, then the thread
	if _, err := h.db.Exec("DELETE FROM chat_message_reactions WHERE message_id IN (SELECT id FROM chat_messages WHERE thread_id = ?)", id); err != nil {
		logger.Error("Failed to delete thread reactions: %v", err)
	}
	if _, err := h.db.Exec("DELETE FROM thread_members WHERE thread_id = ?", id); err != nil {
		logger.Error("Failed to delete thread members: %v", err)
	}
	if _, err := h.db.Exec("DELETE FROM chat_messages WHERE thread_id = ?", id); err != nil {
		logger.Error("Failed to delete thread messages: %v", err)
	}
	h.db.DeleteThreadProviderSessions(id)

	result, err := h.db.Exec("DELETE FROM chat_threads WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete thread")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "chat_thread_deleted", "chat", "chat_thread", id, "")

	w.WriteHeader(http.StatusNoContent)
}

func (h *ChatHandler) UpdateThread(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Title string `json:"title"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	now := time.Now().UTC()
	result, err := h.db.Exec(
		"UPDATE chat_threads SET title = ?, updated_at = ? WHERE id = ?",
		req.Title, now, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update thread")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": id, "title": req.Title})
}

func (h *ChatHandler) ThreadStatus(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")

	// Check for active work orders on this thread
	var woID, woStatus, woTitle, woType string
	var agentID string
	err := h.db.QueryRow(
		`SELECT wo.id, wo.status, wo.title, wo.type, wo.agent_id
		 FROM work_orders wo WHERE wo.thread_id = ? ORDER BY wo.created_at DESC LIMIT 1`,
		threadID,
	).Scan(&woID, &woStatus, &woTitle, &woType, &agentID)

	status := map[string]interface{}{
		"thread_id": threadID,
		"active":    false,
	}

	if err == nil && (woStatus == string(agents.WorkOrderPending) || woStatus == string(agents.WorkOrderInProgress)) {
		status["active"] = true
		status["work_order_id"] = woID
		status["work_order_status"] = woStatus
		status["work_order_title"] = woTitle
		status["work_order_type"] = woType
		status["agent_id"] = agentID
	} else if err == nil {
		status["work_order_id"] = woID
		status["work_order_status"] = woStatus
		status["work_order_title"] = woTitle
	}

	// A cancel func is stored for the whole routing lifecycle (analyzing →
	// thinking → streaming → tool calls), so this marks the thread active during
	// the early thinking phase too — before any stream state or work order exists.
	// That lets a client that navigated away and back re-show the thinking state.
	if _, processing := h.threadCancels.Load(threadID); processing {
		status["active"] = true
	}

	// Include streaming state if the agent is actively streaming
	if ss := h.agentManager.GetStreamState(threadID); ss != nil && ss.Active {
		status["active"] = true
		status["stream_state"] = ss
	}

	writeJSON(w, http.StatusOK, status)
}

// ActiveThreads lists every thread currently being processed (thinking /
// streaming), across all workspaces, so the UI can show a global "active chats"
// indicator. Source of truth is the per-thread cancel funcs stored for the whole
// routing lifecycle.
func (h *ChatHandler) ActiveThreads(w http.ResponseWriter, r *http.Request) {
	type activeThread struct {
		ThreadID    string `json:"thread_id"`
		Title       string `json:"title"`
		WorkspaceID string `json:"workspace_id,omitempty"`
		AgentSlug   string `json:"agent_slug,omitempty"`
		Streaming   bool   `json:"streaming"`
	}

	// A thread is active if it's mid-routing (cancel func present) OR a builder
	// agent is currently RUNNING for it (tool/dashboard build) — the latter runs
	// in its own goroutine, e.g. after an Approve, so it isn't in the cancel set.
	// We key off a running AGENT (not work-order status) so stale/orphaned work
	// orders from earlier attempts don't keep a finished chat showing as "working".
	seen := map[string]bool{}
	var ids []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	h.threadCancels.Range(func(k, _ interface{}) bool {
		if id, ok := k.(string); ok {
			add(id)
		}
		return true
	})
	if rows, err := h.db.Query(
		`SELECT DISTINCT wo.thread_id FROM work_orders wo
		 JOIN agents a ON a.work_order_id = wo.id
		 WHERE a.status = 'running' AND wo.thread_id != ''`,
	); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				add(id)
			}
		}
	}

	out := []activeThread{}
	for _, id := range ids {
		at := activeThread{ThreadID: id, Title: "New chat"}
		var title, wsID string
		if err := h.db.QueryRow("SELECT COALESCE(title, ''), COALESCE(workspace_id, '') FROM chat_threads WHERE id = ?", id).Scan(&title, &wsID); err == nil {
			if title != "" {
				at.Title = title
			}
			at.WorkspaceID = wsID
		}
		if ss := h.agentManager.GetStreamState(id); ss != nil && ss.Active {
			at.Streaming = true
			at.AgentSlug = ss.AgentSlug
		}
		out = append(out, at)
	}

	writeJSON(w, http.StatusOK, out)
}

func (h *ChatHandler) GetMessages(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")

	var exists string
	err := h.db.QueryRow("SELECT id FROM chat_threads WHERE id = ?", threadID).Scan(&exists)
	if err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	rows, err := h.db.Query(
		"SELECT id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, widget_data, image_url, tool_calls_json, stopped, created_at FROM chat_messages WHERE thread_id = ? ORDER BY created_at ASC",
		threadID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	defer rows.Close()

	messages := []models.ChatMessage{}
	var messageIDs []string
	for rows.Next() {
		var m models.ChatMessage
		var tcJSON *string
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Role, &m.Content, &m.AgentRoleSlug, &m.CostUSD, &m.InputTokens, &m.OutputTokens, &m.WidgetData, &m.ImageURL, &tcJSON, &m.Stopped, &m.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan message")
			return
		}
		if tcJSON != nil && *tcJSON != "" {
			m.ToolCalls = json.RawMessage(*tcJSON)
		}
		messages = append(messages, m)
		messageIDs = append(messageIDs, m.ID)
	}

	// Batch-load reactions for all messages
	if len(messageIDs) > 0 {
		placeholders := strings.Repeat("?,", len(messageIDs))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]interface{}, len(messageIDs))
		for i, id := range messageIDs {
			args[i] = id
		}
		rRows, err := h.db.Query(
			fmt.Sprintf("SELECT message_id, emoji, source, COUNT(*) as count FROM chat_message_reactions WHERE message_id IN (%s) GROUP BY message_id, emoji, source ORDER BY MIN(created_at) ASC", placeholders),
			args...,
		)
		if err == nil {
			defer rRows.Close()
			reactionMap := make(map[string][]models.Reaction)
			for rRows.Next() {
				var msgID string
				var r models.Reaction
				if rRows.Scan(&msgID, &r.Emoji, &r.Source, &r.Count) == nil {
					reactionMap[msgID] = append(reactionMap[msgID], r)
				}
			}
			for i := range messages {
				if rxns, ok := reactionMap[messages[i].ID]; ok {
					messages[i].Reactions = rxns
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, messages)
}

func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")

	var exists string
	err := h.db.QueryRow("SELECT id FROM chat_threads WHERE id = ?", threadID).Scan(&exists)
	if err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	var req struct {
		Role          string   `json:"role"`
		Content       string   `json:"content"`
		AgentRoleSlug string   `json:"agent_role_slug"`
		Tools         []string `json:"tools"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	// Pinned threads are archived: kept verbatim as a reference, never added to.
	if h.threadIsPinned(threadID) {
		writeError(w, http.StatusConflict, "this chat is pinned and read-only — unpin it to continue the conversation")
		return
	}
	if req.Role == "" {
		req.Role = "user"
	}

	// Save user message
	userMsgID := generateID()
	now := time.Now().UTC()

	_, err = h.db.Exec(
		"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		userMsgID, threadID, req.Role, req.Content, req.AgentRoleSlug, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}

	if _, err = h.db.Exec("UPDATE chat_threads SET updated_at = ? WHERE id = ?", now, threadID); err != nil {
		logger.Error("Failed to update thread timestamp: %v", err)
	}

	userMsg := models.ChatMessage{
		ID:            userMsgID,
		ThreadID:      threadID,
		Role:          req.Role,
		Content:       req.Content,
		AgentRoleSlug: req.AgentRoleSlug,
		CreatedAt:     now,
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "user_message_sent", "chat", "chat_thread", threadID, req.Content)

	// If agent manager is available and this is a user message, route to appropriate handler
	if h.agentManager != nil && req.Role == "user" {
		var isFirstMsg bool
		var msgCount int
		h.db.QueryRow("SELECT COUNT(*) FROM chat_messages WHERE thread_id = ?", threadID).Scan(&msgCount)
		isFirstMsg = msgCount == 1
		// Tools attached via the `#` picker are appended to the routed content
		// only — the stored message stays exactly what the user typed.
		routedContent := req.Content + h.attachedToolsSection(req.Tools, threadID)
		go h.handleAgentRouting(threadID, routedContent, userID, req.AgentRoleSlug, isFirstMsg)
	}

	writeJSON(w, http.StatusCreated, userMsg)
}

// pinSummaryPrompt asks for a longer, reference-grade write-up than compaction's
// summary. A pinned chat is meant to be read instead of the transcript, so this
// favours completeness over brevity — the opposite trade from compaction, which
// exists to shrink a live context window.
const pinSummaryPrompt = `Write a detailed reference summary of this conversation. It will be read later INSTEAD of the transcript, so be thorough — several paragraphs, and use markdown headings and bullet lists.

Cover:
- What the conversation set out to do, and the outcome.
- Every decision made, and the reasoning behind it.
- Concrete specifics worth keeping: file paths, commands, names, versions, numbers, code snippets.
- Problems hit and how they were resolved.
- Anything left unfinished or explicitly deferred.

Do not editorialise and do not add a preamble — start directly with the summary.

---

%s`

// PinThread archives a thread: it becomes read-only and gains a long-form
// summary so it stays useful as a reference.
func (h *ChatHandler) PinThread(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")

	var exists string
	if err := h.db.QueryRow("SELECT id FROM chat_threads WHERE id = ?", threadID).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	if h.agentManager == nil {
		writeError(w, http.StatusServiceUnavailable, "no AI provider configured")
		return
	}

	transcript, err := h.threadTranscript(threadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if strings.TrimSpace(transcript) == "" {
		writeError(w, http.StatusBadRequest, "this chat has no messages to summarise")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	provider := h.agentManager.Provider()
	summary, usage, err := provider.RunOneShot(
		ctx,
		provider.ResolveModel(h.agentManager.GatewayModel, llm.ModelSonnet),
		"", fmt.Sprintf(pinSummaryPrompt, transcript),
	)
	summary = strings.TrimSpace(summary)
	if err != nil || summary == "" {
		// Pin anyway — the archive matters more than the summary, and the UI
		// falls back to the transcript. A retry can fill it in later.
		if err != nil {
			logger.Warn("pin summary failed for thread %s: %v", threadID, err)
		}
	}

	now := time.Now().UTC()
	if _, err := h.db.Exec(
		"UPDATE chat_threads SET pinned = 1, pinned_at = ?, pin_summary = ? WHERE id = ?",
		now, summary, threadID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to pin thread")
		return
	}

	if usage != nil && (usage.CostUSD > 0 || usage.InputTokens > 0) {
		h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'live_cost_usd'", usage.CostUSD)
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "chat_thread_pinned", "chat", "chat_thread", threadID, "")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "pinned",
		"pin_summary": summary,
	})
}

// GetPin returns a thread's pin state and summary, so the chat view can render
// the archive banner and summary card without loading the whole thread list.
func (h *ChatHandler) GetPin(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")

	var pinned bool
	var summary string
	var pinnedAt sql.NullTime
	err := h.db.QueryRow(
		"SELECT pinned, COALESCE(pin_summary, ''), pinned_at FROM chat_threads WHERE id = ?", threadID,
	).Scan(&pinned, &summary, &pinnedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	out := map[string]interface{}{"pinned": pinned, "pin_summary": summary}
	if pinnedAt.Valid {
		out["pinned_at"] = pinnedAt.Time
	}
	writeJSON(w, http.StatusOK, out)
}

// UnpinThread returns a thread to normal, editable use. The summary is kept so
// re-pinning doesn't have to pay for it again.
func (h *ChatHandler) UnpinThread(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")
	if _, err := h.db.Exec("UPDATE chat_threads SET pinned = 0, pinned_at = NULL WHERE id = ?", threadID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unpin thread")
		return
	}
	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "chat_thread_unpinned", "chat", "chat_thread", threadID, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "unpinned"})
}

// threadTranscript renders a thread as "[role]: content" lines.
func (h *ChatHandler) threadTranscript(threadID string) (string, error) {
	rows, err := h.db.Query(
		"SELECT role, content FROM chat_messages WHERE thread_id = ? ORDER BY created_at ASC",
		threadID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to load messages: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
	}
	return sb.String(), nil
}

// threadIsPinned reports whether a thread is archived and therefore read-only.
func (h *ChatHandler) threadIsPinned(threadID string) bool {
	var pinned bool
	h.db.QueryRow("SELECT pinned FROM chat_threads WHERE id = ?", threadID).Scan(&pinned)
	return pinned
}

// attachedToolsSection resolves tool IDs attached via the `#` picker into a
// directive the agent can act on. Tools are re-checked against the thread's
// workspace (plus globals) so a stale or foreign ID can't smuggle in a tool the
// thread isn't entitled to. Returns "" when nothing valid is attached.
func (h *ChatHandler) attachedToolsSection(toolIDs []string, threadID string) string {
	if len(toolIDs) == 0 {
		return ""
	}

	var wsID string
	if err := h.db.QueryRow("SELECT COALESCE(workspace_id, '') FROM chat_threads WHERE id = ?", threadID).Scan(&wsID); err != nil {
		wsID = h.db.ActiveWorkspaceID()
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(toolIDs)), ",")
	args := make([]interface{}, 0, len(toolIDs)+1)
	for _, id := range toolIDs {
		args = append(args, id)
	}
	args = append(args, wsID)

	rows, err := h.db.Query(
		"SELECT name, description, status FROM tools WHERE id IN ("+placeholders+
			") AND deleted_at IS NULL AND (workspace_id IS NULL OR workspace_id = ?) ORDER BY name ASC",
		args...,
	)
	if err != nil {
		logger.Error("Failed to resolve attached tools: %v", err)
		return ""
	}
	defer rows.Close()

	var sb strings.Builder
	count := 0
	for rows.Next() {
		var name, description, status string
		if err := rows.Scan(&name, &description, &status); err != nil {
			continue
		}
		count++
		sb.WriteString(fmt.Sprintf("- **%s** (status: %s)", name, status))
		if description != "" {
			sb.WriteString(" — " + description)
		}
		sb.WriteString("\n")
	}
	if count == 0 {
		return ""
	}

	return "\n\n---\n**Attached tools** — the user explicitly attached these OpenPaw platform tools to this message. " +
		"Use the `call_tool` tool to invoke them, preferring them over other tools where they fit the request:\n" + sb.String()
}

func (h *ChatHandler) ThreadStats(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")

	var exists string
	if err := h.db.QueryRow("SELECT id FROM chat_threads WHERE id = ?", threadID).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	var totalCost float64
	var totalInput, totalOutput, msgCount int
	err := h.db.QueryRow(
		`SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		        COUNT(*)
		 FROM chat_messages WHERE thread_id = ?`, threadID,
	).Scan(&totalCost, &totalInput, &totalOutput, &msgCount)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get thread stats")
		return
	}

	// Same measure the auto-compact trigger uses, so the percentage the UI shows
	// always matches the one that decides whether to compact.
	contextUsed := h.threadContextUsed(threadID)
	contextLimit := h.getEffectiveContextLimit(threadID)

	autoCompactEnabled := false
	autoCompactThreshold := 85
	if h.agentManager != nil {
		autoCompactEnabled = h.agentManager.AutoCompactEnabled
		autoCompactThreshold = h.agentManager.AutoCompactThreshold
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_cost_usd":         totalCost,
		"total_input_tokens":     totalInput,
		"total_output_tokens":    totalOutput,
		"message_count":          msgCount,
		"context_used_tokens":    contextUsed,
		"context_limit_tokens":   contextLimit,
		"auto_compact_enabled":   autoCompactEnabled,
		"auto_compact_threshold": autoCompactThreshold,
	})
}

func (h *ChatHandler) CompactThread(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")

	var exists string
	if err := h.db.QueryRow("SELECT id FROM chat_threads WHERE id = ?", threadID).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	if err := h.compactThreadInternal(r.Context(), threadID); err != nil {
		writeError(w, http.StatusInternalServerError, "compaction failed: "+err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "chat_thread_compacted", "chat", "chat_thread", threadID, "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "compacted"})
}

// compactRetainMessages is how many of the most recent messages survive
// compaction verbatim. Everything older is replaced by a single summary.
// Keeping a live tail matters: compaction runs *after* the incoming user
// message is saved, so summarizing the entire thread would erase the message
// the user just sent, along with the immediate context the agent needs.
const compactRetainMessages = 6

// errCompactionInProgress is returned when a compaction is already running for
// the thread. Callers treat it as a no-op rather than a failure.
var errCompactionInProgress = errors.New("compaction already in progress")

// summarizeFunc produces the summary for a transcript. Injected so compaction
// can be tested without a live provider.
type summarizeFunc func(ctx context.Context, transcript string) (string, *llm.UsageInfo, error)

// compactThreadInternal compacts a thread using the configured LLM provider.
// The guard is held here (not just in doAutoCompact) so a manual compaction and
// an auto-compaction can never run against the same thread concurrently.
func (h *ChatHandler) compactThreadInternal(ctx context.Context, threadID string) error {
	if _, loaded := h.compactingGuard.LoadOrStore(threadID, true); loaded {
		return errCompactionInProgress
	}
	defer h.compactingGuard.Delete(threadID)

	return h.compactThreadWith(ctx, threadID, func(ctx context.Context, transcript string) (string, *llm.UsageInfo, error) {
		prompt := fmt.Sprintf(
			"Summarize this conversation concisely, preserving: key decisions, requirements, outcomes, and technical details needed to continue. Format as a clear readable summary.\n\n---\n\n%s",
			transcript,
		)
		compactCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		return h.agentManager.Provider().RunOneShot(
			compactCtx,
			h.agentManager.Provider().ResolveModel(h.agentManager.GatewayModel, llm.ModelHaiku),
			"", prompt,
		)
	})
}

// compactThreadWith summarizes the older half of a thread and replaces just
// those messages with a single summary, leaving the most recent
// compactRetainMessages intact. The summary is inserted at the timestamp of the
// oldest summarized message so it sorts ahead of the retained tail.
func (h *ChatHandler) compactThreadWith(ctx context.Context, threadID string, summarize summarizeFunc) error {
	type msgRow struct {
		ID        string
		Role      string
		Content   string
		CreatedAt time.Time
	}

	rows, err := h.db.Query(
		"SELECT id, role, content, created_at FROM chat_messages WHERE thread_id = ? ORDER BY created_at ASC, id ASC",
		threadID,
	)
	if err != nil {
		return fmt.Errorf("failed to load messages: %w", err)
	}
	var msgs []msgRow
	for rows.Next() {
		var m msgRow
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			rows.Close()
			return fmt.Errorf("failed to scan message: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("failed to read messages: %w", err)
	}
	// Closed explicitly rather than deferred: SQLite will not begin the write
	// transaction below while a read cursor is still open on the same table.
	rows.Close()

	if len(msgs) == 0 {
		return fmt.Errorf("no messages to compact")
	}
	// Nothing would be removed — summarizing here would burn a call and, worse,
	// replace recent context with a lossy paraphrase for no gain.
	if len(msgs) <= compactRetainMessages {
		return fmt.Errorf("thread too short to compact: %d message(s)", len(msgs))
	}

	older := msgs[:len(msgs)-compactRetainMessages]
	var transcript strings.Builder
	for _, m := range older {
		transcript.WriteString(fmt.Sprintf("[%s]: %s\n\n", m.Role, m.Content))
	}

	summary, usage, err := summarize(ctx, transcript.String())
	if err != nil {
		return fmt.Errorf("summarization failed: %w", err)
	}
	summary = strings.TrimSpace(summary)
	// An empty summary would destroy history and replace it with nothing, so
	// bail out before any deletion — the thread is left exactly as it was.
	if summary == "" {
		return fmt.Errorf("summarization returned an empty summary")
	}

	olderIDs := make([]string, len(older))
	for i, m := range older {
		olderIDs[i] = m.ID
	}
	archiveMessageCosts(h.db, olderIDs)

	var costUSD float64
	var inTok, outTok int
	if usage != nil {
		costUSD = usage.CostUSD
		inTok = int(usage.InputTokens)
		outTok = int(usage.OutputTokens)
	}

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(olderIDs)), ",")
	delArgs := make([]interface{}, len(olderIDs))
	for i, id := range olderIDs {
		delArgs[i] = id
	}
	if _, err := tx.Exec("DELETE FROM chat_messages WHERE id IN ("+placeholders+")", delArgs...); err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(
		"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		generateID(), threadID, "system", summary, "", costUSD, inTok, outTok, older[0].CreatedAt,
	); err != nil {
		return fmt.Errorf("failed to insert summary: %w", err)
	}

	// The watermark stops the retained tail's pre-compaction input_tokens from
	// counting against the context window and re-triggering compaction forever.
	if _, err := tx.Exec("UPDATE chat_threads SET compacted_at = ?, updated_at = ? WHERE id = ?", now, now, threadID); err != nil {
		return fmt.Errorf("failed to record compaction: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit compaction: %w", err)
	}

	if costUSD > 0 || inTok > 0 || outTok > 0 {
		h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'live_cost_usd'", costUSD)
		h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'live_input_tokens'", float64(inTok))
		h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'live_output_tokens'", float64(outTok))
	}

	return nil
}

// getEffectiveContextLimit returns the context limit override if set,
// otherwise the context window of the model actually used in the thread —
// the model of the agent who last responded — resolved through the active
// provider so tier names map to real models. Falls back to the builder model.
func (h *ChatHandler) getEffectiveContextLimit(threadID string) int {
	if h.agentManager == nil {
		return llm.ContextWindowForModel(llm.ModelSonnet)
	}
	if h.agentManager.ContextLimitOverride > 0 {
		return h.agentManager.ContextLimitOverride
	}

	// CLI subscription providers (Claude Code / Codex) run a 1M-token session
	// window — the per-model OpenRouter windows below don't apply to them, and
	// using the 200k tier default would auto-compact far too early (the UI shows
	// 1M via CLI_CONTEXT_LIMIT, so both must agree).
	if h.agentManager != nil {
		switch h.agentManager.Provider().Name() {
		case llm.ProviderClaudeCode, llm.ProviderCodex:
			return llm.CLIContextWindow
		}
	}

	// OpenRouter: use the LARGEST context window among the models the thread's
	// agents actually use, so a single small-model turn (e.g. the Gateway on
	// Haiku, 200k) doesn't collapse the limit and trigger early auto-compaction
	// on a thread that's really running a 1M-window model.
	best := 0
	if threadID != "" {
		rows, err := h.db.Query(
			`SELECT DISTINCT COALESCE(ar.model, '') FROM chat_messages cm
			 JOIN agent_roles ar ON ar.slug = cm.agent_role_slug
			 WHERE cm.thread_id = ? AND cm.role = 'assistant' AND cm.agent_role_slug != ''`, threadID,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var m string
				if rows.Scan(&m) == nil && m != "" && h.agentManager != nil {
					if w := llm.ContextWindowForModel(h.agentManager.Provider().ResolveModel(m, llm.ModelSonnet)); w > best {
						best = w
					}
				}
			}
		}
	}
	if best == 0 && h.agentManager != nil {
		best = llm.ContextWindowForModel(h.agentManager.Provider().ResolveModel(h.agentManager.BuilderModel, llm.ModelSonnet))
	}
	if best == 0 {
		best = llm.ContextWindowForModel(llm.ModelSonnet)
	}
	return best
}

// threadContextUsed reports how much of the context window the thread's next
// request will consume, in tokens.
//
// Two sources are combined:
//
//   - The peak input_tokens of an assistant message, which is exact when it is
//     a real per-request count: it already includes the system prompt and tool
//     definitions the provider counted.
//   - A size estimate from the retained messages, used when that figure is not
//     a context measurement at all.
//
// The second case is not an edge case. CLI providers report usage for the WHOLE
// agentic run on their final result line — every internal turn's input plus
// cache reads and writes, summed. A turn with dozens of tool calls therefore
// reports millions of "input tokens" while the actual conversation is a fraction
// of the window, which showed up as "352% of 1M" with no way for the thread to
// be that large: the API would have rejected the request. Any figure above the
// window is cumulative spend, so fall back to measuring the conversation.
//
// Messages created at or before the last compaction are excluded: their counts
// describe a history that no longer exists, and counting them would keep the
// thread pinned above the threshold forever.
func (h *ChatHandler) threadContextUsed(threadID string) int {
	var compactedAt sql.NullTime
	h.db.QueryRow("SELECT compacted_at FROM chat_threads WHERE id = ?", threadID).Scan(&compactedAt)

	liveClause := ""
	liveArgs := []interface{}{threadID}
	if compactedAt.Valid {
		liveClause = " AND created_at > ?"
		liveArgs = append(liveArgs, compactedAt.Time)
	}

	// Size of the conversation that will actually be sent.
	var liveChars int
	h.db.QueryRow(
		"SELECT COALESCE(SUM(LENGTH(content)), 0) FROM chat_messages WHERE thread_id = ?"+liveClause,
		liveArgs...,
	).Scan(&liveChars)
	estimate := liveChars/charsPerTokenEstimate + systemPromptTokenAllowance

	args := append([]interface{}{}, liveArgs...)
	var reported int
	var peakAt sql.NullString
	err := h.db.QueryRow(
		`SELECT COALESCE(MAX(input_tokens), 0), COALESCE(MAX(created_at), '') FROM chat_messages
		 WHERE thread_id = ? AND role = 'assistant' AND input_tokens > 0`+liveClause,
		args...,
	).Scan(&reported, &peakAt)
	if err != nil || reported == 0 {
		if liveChars == 0 {
			return 0
		}
		return estimate
	}

	// Reject a figure that cannot describe a context window.
	if limit := h.getEffectiveContextLimit(threadID); limit > 0 && reported > limit {
		return estimate
	}

	// Add an estimate for anything appended after that turn — those tokens are
	// real and will be sent, but no provider has counted them yet.
	if peakAt.Valid && peakAt.String != "" {
		var pendingChars int
		h.db.QueryRow(
			`SELECT COALESCE(SUM(LENGTH(content)), 0) FROM chat_messages
			 WHERE thread_id = ? AND created_at > ?`, threadID, peakAt.String,
		).Scan(&pendingChars)
		reported += pendingChars / charsPerTokenEstimate
	}

	// A plausible reported count is authoritative — it already includes the
	// system prompt and tool definitions the estimate can only approximate — so
	// it is NOT floored by the estimate.
	return reported
}

// systemPromptTokenAllowance approximates the system prompt, tool definitions
// and identity files that ride along with every request but are not stored as
// chat messages. Only used when estimating from message sizes.
const systemPromptTokenAllowance = 4000

// charsPerTokenEstimate is the rough characters-per-token ratio used to size
// messages the provider has not tokenized yet. Deliberately conservative (real
// English prose runs ~4); under-estimating here would let a thread overflow.
const charsPerTokenEstimate = 3

// compactionNeeded reports whether usage has reached the configured threshold.
// The comparison is inclusive so a thread sitting exactly on the threshold
// compacts rather than waiting for the turn that overflows it.
func compactionNeeded(contextUsed, contextLimit, thresholdPercent int) bool {
	if contextUsed <= 0 || contextLimit <= 0 {
		return false
	}
	ratio := float64(contextUsed) / float64(contextLimit) * 100
	return ratio >= float64(thresholdPercent)
}

// shouldAutoCompact checks whether auto-compaction should trigger for the given thread.
func (h *ChatHandler) shouldAutoCompact(threadID string) bool {
	if h.agentManager == nil || !h.agentManager.AutoCompactEnabled {
		return false
	}
	// Below the retain floor there is nothing compaction could remove, so
	// triggering would loop on a thread it cannot shrink.
	var msgCount int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM chat_messages WHERE thread_id = ?", threadID).Scan(&msgCount); err != nil || msgCount <= compactRetainMessages {
		return false
	}
	return compactionNeeded(h.threadContextUsed(threadID), h.getEffectiveContextLimit(threadID), h.agentManager.AutoCompactThreshold)
}

// doAutoCompact runs compaction, treating an in-flight compaction as a no-op.
func (h *ChatHandler) doAutoCompact(ctx context.Context, threadID string) error {
	err := h.compactThreadInternal(ctx, threadID)
	if errors.Is(err, errCompactionInProgress) {
		return nil
	}
	return err
}

// saveStoppedMessage stores an interrupted reply, flagged so the UI can badge
// it instead of presenting a truncated answer as a finished one.
func (h *ChatHandler) saveStoppedMessage(threadID, agentRoleSlug, content string) string {
	id := h.saveAssistantMessage(threadID, agentRoleSlug, content, 0, 0, 0)
	if _, err := h.db.Exec("UPDATE chat_messages SET stopped = 1 WHERE id = ?", id); err != nil {
		logger.Error("Failed to flag stopped message: %v", err)
	}
	return id
}

func (h *ChatHandler) saveAssistantMessage(threadID, agentRoleSlug, content string, costUSD float64, inputTokens, outputTokens int, extras ...string) string {
	id := generateID()
	now := time.Now().UTC()
	var wd *string
	if len(extras) > 0 && extras[0] != "" {
		wd = &extras[0]
	}
	var imgURL *string
	if len(extras) > 1 && extras[1] != "" {
		imgURL = &extras[1]
	}
	var tcJSON *string
	if len(extras) > 2 && extras[2] != "" {
		tcJSON = &extras[2]
	}
	if _, err := h.db.Exec(
		"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, widget_data, image_url, tool_calls_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id, threadID, "assistant", content, agentRoleSlug, costUSD, inputTokens, outputTokens, wd, imgURL, tcJSON, now,
	); err != nil {
		logger.Error("Failed to save assistant message: %v", err)
	}
	if _, err := h.db.Exec("UPDATE chat_threads SET updated_at = ? WHERE id = ?", now, threadID); err != nil {
		logger.Error("Failed to update thread timestamp: %v", err)
	}
	// Increment running counters for LogStats
	if costUSD > 0 || inputTokens > 0 || outputTokens > 0 {
		h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'live_cost_usd'", costUSD)
		h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'live_input_tokens'", float64(inputTokens))
		h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'live_output_tokens'", float64(outputTokens))
	}
	return id
}

func (h *ChatHandler) ToggleReaction(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageId")

	var req struct {
		Emoji  string `json:"emoji"`
		Source string `json:"source"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Emoji == "" {
		writeError(w, http.StatusBadRequest, "emoji is required")
		return
	}
	if req.Source == "" {
		req.Source = "user"
	}

	// Verify message exists and get thread ID
	var threadID string
	if err := h.db.QueryRow("SELECT thread_id FROM chat_messages WHERE id = ?", messageID).Scan(&threadID); err != nil {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}

	// Toggle: check if reaction exists
	var existingID string
	err := h.db.QueryRow(
		"SELECT id FROM chat_message_reactions WHERE message_id = ? AND emoji = ? AND source = ?",
		messageID, req.Emoji, req.Source,
	).Scan(&existingID)

	if err == nil {
		// Exists — delete it
		h.db.Exec("DELETE FROM chat_message_reactions WHERE id = ?", existingID)
	} else {
		// Doesn't exist — insert
		id := generateID()
		h.db.Exec(
			"INSERT INTO chat_message_reactions (id, message_id, emoji, source) VALUES (?, ?, ?, ?)",
			id, messageID, req.Emoji, req.Source,
		)
	}

	// Load updated reactions for this message
	reactions := h.loadReactionsForMessage(messageID)

	// Broadcast via WebSocket
	h.agentManager.Broadcast("message_reacted", models.WSMessageReacted{
		ThreadID:  threadID,
		MessageID: messageID,
		Reactions: reactions,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message_id": messageID,
		"reactions":  reactions,
	})
}

func (h *ChatHandler) loadReactionsForMessage(messageID string) []models.Reaction {
	rows, err := h.db.Query(
		"SELECT emoji, source, COUNT(*) as count FROM chat_message_reactions WHERE message_id = ? GROUP BY emoji, source ORDER BY MIN(created_at) ASC",
		messageID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var reactions []models.Reaction
	for rows.Next() {
		var r models.Reaction
		if err := rows.Scan(&r.Emoji, &r.Source, &r.Count); err != nil {
			continue
		}
		reactions = append(reactions, r)
	}
	return reactions
}

func (h *ChatHandler) isConfirmationEnabled() bool {
	var val string
	err := h.db.QueryRow("SELECT value FROM settings WHERE key = 'confirmation_enabled'").Scan(&val)
	if err != nil {
		return true // default: enabled
	}
	return val != "false"
}

func (h *ChatHandler) StopThread(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r.Context())
	stopped := false

	// Snapshot the half-written reply BEFORE cancelling. Cancelling wakes the
	// routing goroutine, which clears the stream state on its way out — read it
	// afterwards and the text is sometimes already gone.
	var partial string
	var partialAgent string
	if st := h.agentManager.GetStreamState(threadID); st != nil {
		partial = strings.TrimSpace(st.Text)
		partialAgent = st.AgentSlug
	}

	// 1. Cancel the routing goroutine (gateway/role chat)
	if cancelVal, ok := h.threadCancels.LoadAndDelete(threadID); ok {
		if cancel, ok := cancelVal.(context.CancelFunc); ok {
			cancel()
			stopped = true
		}
	}

	// 2. Stop any active builder agent associated with this thread
	var agentID, woID, woStatus string
	err := h.db.QueryRow(
		`SELECT wo.agent_id, wo.id, wo.status FROM work_orders wo
		 WHERE wo.thread_id = ? AND wo.status IN ('pending', 'in_progress')
		 ORDER BY wo.created_at DESC LIMIT 1`,
		threadID,
	).Scan(&agentID, &woID, &woStatus)
	if err == nil && agentID != "" {
		if stopErr := h.agentManager.StopAgent(agentID); stopErr == nil {
			stopped = true
		}
	} else if err == nil && woStatus == string(agents.WorkOrderPending) {
		agents.UpdateWorkOrderStatus(h.db, woID, agents.WorkOrderCancelled, "stopped by user")
		stopped = true
	}

	// Keep whatever was written. The old behaviour discarded it and saved the
	// word "Stopped." instead, which threw away the only thing the user
	// actually wanted — the answer as far as it got.
	if stopped {
		if partial != "" {
			h.saveStoppedMessage(threadID, partialAgent, partial)
		} else {
			// Nothing streamed yet — say so plainly rather than leaving the
			// turn with no trace that it was interrupted.
			h.saveStoppedMessage(threadID, partialAgent, "_Stopped before a reply was written._")
		}
		h.broadcastStatus(threadID, "message_saved", "")
	}

	h.agentManager.ClearStreamState(threadID)

	// Always broadcast done to reset the frontend
	h.broadcastStatus(threadID, "done", "")

	// Broadcast agent_completed to also clear streaming state
	h.agentManager.Broadcast("agent_completed", map[string]interface{}{
		"thread_id": threadID,
	})

	h.db.LogAudit(userID, "chat_thread_stopped", "chat", "chat_thread", threadID, "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}
