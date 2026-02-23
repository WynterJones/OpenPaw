package handlers

import (
	"context"
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
)

type ChatHandler struct {
	db               *database.DB
	agentManager     *agents.Manager
	toolsDir         string
	dataDir          string
	dashboardsDir    string
	threadCancels    sync.Map // map[threadID]context.CancelFunc
	compactingGuard  sync.Map // map[threadID]bool — prevents double-compaction
	roleCache        struct {
		sync.RWMutex
		roles     []struct{ slug, name string }
		expiresAt time.Time
	}
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
	rows, err := h.db.Query(
		`SELECT t.id, t.title, COALESCE(c.cost, 0), t.created_at, t.updated_at
		 FROM chat_threads t
		 LEFT JOIN (SELECT thread_id, SUM(cost_usd) AS cost FROM chat_messages GROUP BY thread_id) c ON c.thread_id = t.id
		 ORDER BY t.updated_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list threads")
		return
	}
	defer rows.Close()

	threads := []models.ChatThread{}
	for rows.Next() {
		var t models.ChatThread
		if err := rows.Scan(&t.ID, &t.Title, &t.TotalCostUSD, &t.CreatedAt, &t.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan thread")
			return
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
		"INSERT INTO chat_threads (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)",
		id, req.Title, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create thread")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "chat_thread_created", "chat", "chat_thread", id, req.Title)

	writeJSON(w, http.StatusCreated, models.ChatThread{
		ID:        id,
		Title:     req.Title,
		CreatedAt: now,
		UpdatedAt: now,
	})
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

	// Delete members, messages, then the thread
	if _, err := h.db.Exec("DELETE FROM thread_members WHERE thread_id = ?", id); err != nil {
		logger.Error("Failed to delete thread members: %v", err)
	}
	if _, err := h.db.Exec("DELETE FROM chat_messages WHERE thread_id = ?", id); err != nil {
		logger.Error("Failed to delete thread messages: %v", err)
	}

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

	// Include streaming state if the agent is actively streaming
	if ss := h.agentManager.GetStreamState(threadID); ss != nil && ss.Active {
		status["active"] = true
		status["stream_state"] = ss
	}

	writeJSON(w, http.StatusOK, status)
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
		"SELECT id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, widget_data, created_at FROM chat_messages WHERE thread_id = ? ORDER BY created_at ASC",
		threadID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	defer rows.Close()

	messages := []models.ChatMessage{}
	for rows.Next() {
		var m models.ChatMessage
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Role, &m.Content, &m.AgentRoleSlug, &m.CostUSD, &m.InputTokens, &m.OutputTokens, &m.WidgetData, &m.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan message")
			return
		}
		messages = append(messages, m)
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
		Role          string `json:"role"`
		Content       string `json:"content"`
		AgentRoleSlug string `json:"agent_role_slug"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
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
		go h.handleAgentRouting(threadID, req.Content, userID, req.AgentRoleSlug, isFirstMsg)
	}

	writeJSON(w, http.StatusCreated, userMsg)
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
	var totalContentLen int
	err := h.db.QueryRow(
		`SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		        COUNT(*), COALESCE(SUM(LENGTH(content)), 0)
		 FROM chat_messages WHERE thread_id = ?`, threadID,
	).Scan(&totalCost, &totalInput, &totalOutput, &msgCount, &totalContentLen)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get thread stats")
		return
	}

	contextUsed := totalContentLen / 4
	contextLimit := h.getEffectiveContextLimit()

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

// compactThreadInternal performs the core compaction logic: loads messages,
// summarizes them via LLM, replaces all messages with the summary.
func (h *ChatHandler) compactThreadInternal(ctx context.Context, threadID string) error {
	rows, err := h.db.Query(
		"SELECT role, content FROM chat_messages WHERE thread_id = ? ORDER BY created_at ASC",
		threadID,
	)
	if err != nil {
		return fmt.Errorf("failed to load messages: %w", err)
	}
	defer rows.Close()

	var transcript strings.Builder
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			return fmt.Errorf("failed to scan message: %w", err)
		}
		transcript.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
	}

	if transcript.Len() == 0 {
		return fmt.Errorf("no messages to compact")
	}

	prompt := fmt.Sprintf(
		"Summarize this conversation concisely, preserving: key decisions, requirements, outcomes, and technical details needed to continue. Format as a clear readable summary.\n\n---\n\n%s",
		transcript.String(),
	)

	compactCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	summary, usage, err := h.agentManager.Client().RunOneShot(compactCtx, llm.ResolveModel(h.agentManager.GatewayModel, llm.ModelHaiku), "", prompt)
	if err != nil {
		return fmt.Errorf("summarization failed: %w", err)
	}

	archiveThreadCosts(h.db, threadID)

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM chat_messages WHERE thread_id = ?", threadID); err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}

	var costUSD float64
	var inTok, outTok int
	if usage != nil {
		costUSD = usage.CostUSD
		inTok = int(usage.InputTokens)
		outTok = int(usage.OutputTokens)
	}

	msgID := generateID()
	now := time.Now().UTC()
	if _, err := tx.Exec(
		"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		msgID, threadID, "system", strings.TrimSpace(summary), "", costUSD, inTok, outTok, now,
	); err != nil {
		return fmt.Errorf("failed to insert summary: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit compaction: %w", err)
	}

	if costUSD > 0 || inTok > 0 || outTok > 0 {
		h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'live_cost_usd'", costUSD)
		h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'live_input_tokens'", float64(inTok))
		h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'live_output_tokens'", float64(outTok))
	}

	if _, err = h.db.Exec("UPDATE chat_threads SET updated_at = ? WHERE id = ?", now, threadID); err != nil {
		logger.Error("Failed to update thread timestamp: %v", err)
	}

	return nil
}

// getEffectiveContextLimit returns the context limit override if set, otherwise the model default.
func (h *ChatHandler) getEffectiveContextLimit() int {
	if h.agentManager.ContextLimitOverride > 0 {
		return h.agentManager.ContextLimitOverride
	}
	model := ""
	if h.agentManager != nil {
		model = h.agentManager.BuilderModel
	}
	return llm.ContextWindowForModel(model)
}

// shouldAutoCompact checks whether auto-compaction should trigger for the given thread.
func (h *ChatHandler) shouldAutoCompact(threadID string) bool {
	if h.agentManager == nil || !h.agentManager.AutoCompactEnabled {
		return false
	}

	var totalContentLen int
	if err := h.db.QueryRow(
		"SELECT COALESCE(SUM(LENGTH(content)), 0) FROM chat_messages WHERE thread_id = ?", threadID,
	).Scan(&totalContentLen); err != nil {
		return false
	}

	contextUsed := totalContentLen / 4
	contextLimit := h.getEffectiveContextLimit()
	if contextLimit <= 0 {
		return false
	}

	ratio := float64(contextUsed) / float64(contextLimit) * 100
	return ratio >= float64(h.agentManager.AutoCompactThreshold)
}

// doAutoCompact runs compaction with a guard to prevent double-compaction on the same thread.
func (h *ChatHandler) doAutoCompact(ctx context.Context, threadID string) error {
	if _, loaded := h.compactingGuard.LoadOrStore(threadID, true); loaded {
		return nil // already compacting
	}
	defer h.compactingGuard.Delete(threadID)

	return h.compactThreadInternal(ctx, threadID)
}

func (h *ChatHandler) saveAssistantMessage(threadID, agentRoleSlug, content string, costUSD float64, inputTokens, outputTokens int, widgetData ...string) string {
	id := generateID()
	now := time.Now().UTC()
	var wd *string
	if len(widgetData) > 0 && widgetData[0] != "" {
		wd = &widgetData[0]
	}
	if _, err := h.db.Exec(
		"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, widget_data, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id, threadID, "assistant", content, agentRoleSlug, costUSD, inputTokens, outputTokens, wd, now,
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

	if stopped {
		h.saveAssistantMessage(threadID, "", "Stopped.", 0, 0, 0)
		h.broadcastStatus(threadID, "message_saved", "")
	}

	// Always broadcast done to reset the frontend
	h.broadcastStatus(threadID, "done", "")

	// Broadcast agent_completed to also clear streaming state
	h.agentManager.Broadcast("agent_completed", map[string]interface{}{
		"thread_id": threadID,
	})

	h.db.LogAudit(userID, "chat_thread_stopped", "chat", "chat_thread", threadID, "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}
