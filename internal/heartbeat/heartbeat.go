package heartbeat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/memory"
)

type BroadcastFunc func(msgType string, payload interface{})

type Config struct {
	Enabled     bool   `json:"enabled"`
	IntervalSec int    `json:"interval_sec"`
	Timezone    string `json:"timezone"`
	ActiveStart string `json:"active_start"`
	ActiveEnd   string `json:"active_end"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:     false,
		IntervalSec: 3600,
		Timezone:    "UTC",
		ActiveStart: "09:00",
		ActiveEnd:   "22:00",
	}
}

type Manager struct {
	db        *database.DB
	agentMgr  *agents.Manager
	broadcast BroadcastFunc
	dataDir   string

	config Config
	mu     sync.RWMutex

	running  atomic.Bool
	stopCh   chan struct{}
	stopped  chan struct{}
	started  bool
}

func New(db *database.DB, agentMgr *agents.Manager, broadcast BroadcastFunc, dataDir string) *Manager {
	return &Manager{
		db:        db,
		agentMgr:  agentMgr,
		broadcast: broadcast,
		dataDir:   dataDir,
		config:    DefaultConfig(),
		stopCh:    make(chan struct{}),
		stopped:   make(chan struct{}),
	}
}

// LoadConfig reads heartbeat settings from the database.
func (m *Manager) LoadConfig() {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := DefaultConfig()

	rows, err := m.db.Query(
		"SELECT key, value FROM settings WHERE key IN ('heartbeat_enabled', 'heartbeat_interval_sec', 'heartbeat_timezone', 'heartbeat_active_start', 'heartbeat_active_end')",
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var key, val string
			if rows.Scan(&key, &val) != nil || val == "" {
				continue
			}
			switch key {
			case "heartbeat_enabled":
				cfg.Enabled = val == "true" || val == "1"
			case "heartbeat_interval_sec":
				if v := parseInt(val); v > 0 {
					cfg.IntervalSec = v
				}
			case "heartbeat_timezone":
				cfg.Timezone = val
			case "heartbeat_active_start":
				cfg.ActiveStart = val
			case "heartbeat_active_end":
				cfg.ActiveEnd = val
			}
		}
	}

	m.config = cfg
}

func parseInt(s string) int {
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}

// GetConfig returns the current config as a string map for the API.
func (m *Manager) GetConfig() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	enabled := "false"
	if m.config.Enabled {
		enabled = "true"
	}

	return map[string]string{
		"heartbeat_enabled":      enabled,
		"heartbeat_interval_sec": fmt.Sprintf("%d", m.config.IntervalSec),
		"heartbeat_timezone":     m.config.Timezone,
		"heartbeat_active_start": m.config.ActiveStart,
		"heartbeat_active_end":   m.config.ActiveEnd,
	}
}

// UpdateConfig saves new settings and hot-reloads.
func (m *Manager) UpdateConfig(cfg map[string]string) error {
	for key, val := range cfg {
		m.db.Exec(
			"INSERT INTO settings (id, key, value) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
			"hb-"+key, key, val, val,
		)
	}

	m.LoadConfig()

	// Restart the tick loop if running
	if m.started {
		m.Stop()
		m.Start()
	}

	return nil
}

// Start begins the heartbeat tick loop.
func (m *Manager) Start() {
	m.mu.RLock()
	enabled := m.config.Enabled
	m.mu.RUnlock()

	if !enabled {
		logger.Info("Heartbeat disabled, not starting")
		return
	}

	m.stopCh = make(chan struct{})
	m.stopped = make(chan struct{})
	m.started = true

	go m.tickLoop()
	logger.Success("Heartbeat manager started")
}

// Stop halts the tick loop.
func (m *Manager) Stop() {
	if !m.started {
		return
	}
	close(m.stopCh)
	<-m.stopped
	m.started = false
	logger.Info("Heartbeat manager stopped")
}

// IsRunning returns whether a heartbeat cycle is currently executing.
func (m *Manager) IsRunning() bool {
	return m.running.Load()
}

// RunNow triggers an immediate heartbeat cycle, bypassing enabled/active-hours checks.
func (m *Manager) RunNow() {
	m.runCycleForced()
}

func (m *Manager) tickLoop() {
	defer close(m.stopped)

	for {
		m.mu.RLock()
		interval := time.Duration(m.config.IntervalSec) * time.Second
		m.mu.RUnlock()

		select {
		case <-m.stopCh:
			return
		case <-time.After(interval):
			m.runCycle()
		}
	}
}

func (m *Manager) runCycle() {
	m.runCycleInternal(false)
}

func (m *Manager) runCycleForced() {
	m.runCycleInternal(true)
}

func (m *Manager) runCycleInternal(forced bool) {
	// Skip if already running
	if !m.running.CompareAndSwap(false, true) {
		logger.Warn("Heartbeat cycle skipped — previous cycle still running")
		return
	}
	defer func() {
		m.running.Store(false)
		m.broadcast("heartbeat_cycle_done", map[string]interface{}{})
	}()

	m.mu.RLock()
	cfg := m.config
	m.mu.RUnlock()

	if !forced && !cfg.Enabled {
		logger.Info("Heartbeat cycle skipped — disabled")
		return
	}

	if !forced && !m.isWithinActiveHours(cfg) {
		logger.Info("Heartbeat skipped — outside active hours (%s-%s %s)", cfg.ActiveStart, cfg.ActiveEnd, cfg.Timezone)
		return
	}

	// Get enabled agents with heartbeat enabled
	rows, err := m.db.Query(
		"SELECT slug, model FROM agent_roles WHERE enabled = 1 AND heartbeat_enabled = 1 ORDER BY sort_order ASC",
	)
	if err != nil {
		logger.Error("Heartbeat: failed to query agents: %v", err)
		return
	}
	defer rows.Close()

	type agentInfo struct {
		slug, model string
	}
	var agentList []agentInfo
	for rows.Next() {
		var a agentInfo
		if err := rows.Scan(&a.slug, &a.model); err != nil {
			logger.Error("Heartbeat: failed to scan agent row: %v", err)
			continue
		}
		agentList = append(agentList, a)
	}

	if len(agentList) == 0 {
		logger.Info("Heartbeat cycle: no agents with heartbeat enabled")
		return
	}

	logger.Info("Heartbeat cycle starting for %d agent(s)", len(agentList))

	// Execute sequentially
	for _, agent := range agentList {
		select {
		case <-m.stopCh:
			return
		default:
		}

		m.executeForAgent(agent.slug, agent.model)
	}

	logger.Info("Heartbeat cycle complete")
}

func (m *Manager) isWithinActiveHours(cfg Config) bool {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)
	startH, startM := parseTime(cfg.ActiveStart)
	endH, endM := parseTime(cfg.ActiveEnd)

	currentMinutes := now.Hour()*60 + now.Minute()
	startMinutes := startH*60 + startM
	endMinutes := endH*60 + endM

	if startMinutes <= endMinutes {
		return currentMinutes >= startMinutes && currentMinutes < endMinutes
	}
	// Wraps midnight
	return currentMinutes >= startMinutes || currentMinutes < endMinutes
}

func parseTime(s string) (int, int) {
	var h, m int
	fmt.Sscanf(s, "%d:%d", &h, &m)
	return h, m
}

func (m *Manager) executeForAgent(slug, model string) {
	// Read HEARTBEAT.md
	hbPath := filepath.Join(agents.AgentDir(m.dataDir, slug), agents.FileHeartbeat)
	heartbeatContent, err := os.ReadFile(hbPath)
	if err != nil {
		logger.Warn("Heartbeat: skipping %s — cannot read HEARTBEAT.md: %v", slug, err)
		return
	}
	if strings.TrimSpace(string(heartbeatContent)) == "" {
		logger.Info("Heartbeat: skipping %s — HEARTBEAT.md is empty", slug)
		return
	}

	logger.Info("Heartbeat: executing for agent %s (model: %s)", slug, model)

	execID := uuid.New().String()
	now := time.Now().UTC()

	// Insert execution record
	m.db.Exec(
		`INSERT INTO heartbeat_executions (id, agent_role_slug, status, started_at) VALUES (?, ?, 'running', ?)`,
		execID, slug, now,
	)

	m.broadcast("heartbeat_started", map[string]interface{}{
		"agent_slug": slug,
	})

	// Assemble system prompt
	systemPrompt, err := agents.AssembleSystemPrompt(m.dataDir, slug)
	if err != nil {
		m.finishExecution(execID, "failed", "[]", "", err.Error(), 0, 0, 0)
		return
	}

	// Inject SQLite memory into system prompt
	if m.agentMgr.MemoryMgr != nil {
		m.agentMgr.MemoryMgr.EnsureMigrated(slug)
		if memSection := m.agentMgr.MemoryMgr.BuildMemoryPromptSection(slug); memSection != "" {
			systemPrompt += "\n\n---\n\n" + memSection
		}
	}

	// Add heartbeat-mode instructions
	systemPrompt += fmt.Sprintf(`

---

## HEARTBEAT MODE

You are running in **heartbeat mode** — a periodic, autonomous check-in.

Current time: %s

### Tools

- **create_notification**: Send a notification to the user. Parameters: title (required), body, priority (low/normal/high), link
- **create_chat**: Start a NEW chat thread. Parameters: title (required), message (required)
- **send_message**: Send a message to an EXISTING chat thread. Parameters: thread_id (required), message (required). Returns error if thread was deleted — fall back to create_chat.
- **no_action**: Signal that you have nothing to do right now. Parameters: reason (required)
- **Memory tools**: memory_save, memory_search, memory_list, memory_update, memory_forget, memory_stats

### Chat Continuation Strategy

1. **First**, use memory_search to look for an existing thread ID (search for "thread" or the topic).
2. **If found**, use send_message with that thread_id to continue the conversation.
3. **If not found** (or send_message returns an error), use create_chat to start a new thread, then use memory_save to store the thread ID for next time. Include the topic in the memory content so you can find it later.

Read your heartbeat instructions below and decide what actions to take. If there's nothing to do, use no_action.
Keep actions concise and purposeful. You have a maximum of 5 turns.`, time.Now().Format("Monday, January 2, 2006 at 3:04 PM MST"))

	// Run agent loop with heartbeat tools
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var actionsTaken []string

	extraTools := []llm.ToolDef{
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "create_notification",
				Description: "Send a notification to the user",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","description":"Notification title"},"body":{"type":"string","description":"Notification body"},"priority":{"type":"string","enum":["low","normal","high"],"description":"Priority level"},"link":{"type":"string","description":"Optional link URL"}},"required":["title"]}`),
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "create_chat",
				Description: "Start a new chat thread with an initial message",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","description":"Thread title"},"message":{"type":"string","description":"Initial message content"}},"required":["title","message"]}`),
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "send_message",
				Description: "Send a message to an existing chat thread. Use this to continue a conversation instead of creating a new thread.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"thread_id":{"type":"string","description":"ID of the existing chat thread"},"message":{"type":"string","description":"Message content to send"}},"required":["thread_id","message"]}`),
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "no_action",
				Description: "Signal that there is nothing to do right now",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"reason":{"type":"string","description":"Why no action is needed"}},"required":["reason"]}`),
			},
		},
	}

	// Append memory tools if available
	if m.agentMgr.MemoryMgr != nil {
		extraTools = append(extraTools, memory.BuildMemoryToolDefs()...)
	}

	extraHandlers := map[string]llm.ToolHandler{
		"create_notification": func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
			var params struct {
				Title    string `json:"title"`
				Body     string `json:"body"`
				Priority string `json:"priority"`
				Link     string `json:"link"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
			}
			if params.Title == "" {
				return llm.ToolResult{Output: "title is required", IsError: true}
			}

			notif, err := m.createNotification(params.Title, params.Body, params.Priority, slug, "heartbeat", params.Link)
			if err != nil {
				return llm.ToolResult{Output: "Failed to create notification: " + err.Error(), IsError: true}
			}

			m.broadcast("notification_created", notif)
			actionsTaken = append(actionsTaken, "create_notification: "+params.Title)
			return llm.ToolResult{Output: fmt.Sprintf("Notification created: %s", notif["id"])}
		},
		"create_chat": func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
			var params struct {
				Title   string `json:"title"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
			}
			if params.Title == "" || params.Message == "" {
				return llm.ToolResult{Output: "title and message are required", IsError: true}
			}

			threadID, err := m.createChatThread(params.Title, params.Message, slug)
			if err != nil {
				return llm.ToolResult{Output: "Failed to create chat: " + err.Error(), IsError: true}
			}

			m.broadcast("thread_created", map[string]interface{}{
				"thread_id": threadID,
				"title":     params.Title,
				"agent":     slug,
			})

			notif, nErr := m.createNotification(params.Title, params.Message, "normal", slug, "heartbeat", "/chat/"+threadID)
			if nErr == nil {
				m.broadcast("notification_created", notif)
			}

			actionsTaken = append(actionsTaken, "create_chat: "+params.Title)
			return llm.ToolResult{Output: fmt.Sprintf("Chat thread created: %s", threadID)}
		},
		"send_message": func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
			var params struct {
				ThreadID string `json:"thread_id"`
				Message  string `json:"message"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
			}
			if params.ThreadID == "" || params.Message == "" {
				return llm.ToolResult{Output: "thread_id and message are required", IsError: true}
			}

			// Validate thread exists
			var threadTitle string
			err := m.db.QueryRow("SELECT title FROM chat_threads WHERE id = ?", params.ThreadID).Scan(&threadTitle)
			if err != nil {
				return llm.ToolResult{Output: "Thread not found — it may have been deleted. Use create_chat instead.", IsError: true}
			}

			// Insert message
			msgID := uuid.New().String()
			now := time.Now().UTC()
			_, err = m.db.Exec(
				`INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, created_at)
				 VALUES (?, ?, 'assistant', ?, ?, 0, 0, 0, ?)`,
				msgID, params.ThreadID, params.Message, slug, now,
			)
			if err != nil {
				return llm.ToolResult{Output: "Failed to send message: " + err.Error(), IsError: true}
			}

			// Update thread timestamp
			m.db.Exec("UPDATE chat_threads SET updated_at = ? WHERE id = ?", now, params.ThreadID)

			// Ensure agent is a thread member
			m.db.Exec(
				"INSERT OR IGNORE INTO thread_members (thread_id, agent_role_slug, joined_at) VALUES (?, ?, ?)",
				params.ThreadID, slug, now,
			)

			// Broadcast WebSocket events
			m.broadcast("thread_updated", map[string]interface{}{
				"thread_id": params.ThreadID,
				"title":     threadTitle,
				"agent":     slug,
			})
			m.broadcast("agent_completed", map[string]interface{}{
				"thread_id": params.ThreadID,
				"agent":     slug,
			})

			// Create notification
			notif, nErr := m.createNotification(threadTitle, params.Message, "normal", slug, "heartbeat", "/chat/"+params.ThreadID)
			if nErr == nil {
				m.broadcast("notification_created", notif)
			}

			actionsTaken = append(actionsTaken, "send_message: "+threadTitle)
			return llm.ToolResult{Output: fmt.Sprintf("Message sent to thread %s", params.ThreadID)}
		},
		"no_action": func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
			var params struct {
				Reason string `json:"reason"`
			}
			json.Unmarshal(input, &params)
			actionsTaken = append(actionsTaken, "no_action: "+params.Reason)
			return llm.ToolResult{Output: "OK, no action taken"}
		},
	}

	// Register memory handlers
	if m.agentMgr.MemoryMgr != nil {
		for name, handler := range m.agentMgr.MemoryMgr.MakeMemoryHandlers(slug) {
			extraHandlers[name] = handler
		}
	}

	result, err := m.agentMgr.Client().RunAgentLoop(ctx, llm.AgentConfig{
		Model:         llm.ResolveModel(model, llm.ModelSonnet),
		System:        systemPrompt,
		MaxTurns:      5,
		ExtraTools:    extraTools,
		ExtraHandlers: extraHandlers,
	}, string(heartbeatContent))

	status := "completed"
	errMsg := ""
	var costUSD float64
	var inputTokens, outputTokens int
	output := ""

	if err != nil {
		status = "failed"
		errMsg = err.Error()
	}
	if result != nil {
		costUSD = result.TotalCostUSD
		inputTokens = int(result.InputTokens)
		outputTokens = int(result.OutputTokens)
		output = strings.TrimSpace(result.Text)
	}

	actionsJSON, _ := json.Marshal(actionsTaken)
	m.finishExecution(execID, status, string(actionsJSON), output, errMsg, costUSD, inputTokens, outputTokens)

	m.broadcast("heartbeat_completed", map[string]interface{}{
		"agent_slug":    slug,
		"status":        status,
		"actions_taken": actionsTaken,
	})

	m.db.LogAudit("system", "heartbeat_executed", "heartbeat", "agent_role", slug,
		fmt.Sprintf("status=%s actions=%d tokens=%d+%d cost=$%.4f", status, len(actionsTaken), inputTokens, outputTokens, costUSD))

	logger.Info("Heartbeat %s for %s: %d actions, $%.4f", status, slug, len(actionsTaken), costUSD)
}

func (m *Manager) finishExecution(execID, status, actionsTaken, output, errMsg string, costUSD float64, inputTokens, outputTokens int) {
	now := time.Now().UTC()
	m.db.Exec(
		`UPDATE heartbeat_executions SET status = ?, actions_taken = ?, output = ?, error = ?, cost_usd = ?, input_tokens = ?, output_tokens = ?, finished_at = ? WHERE id = ?`,
		status, actionsTaken, output, errMsg, costUSD, inputTokens, outputTokens, now, execID,
	)
}

func (m *Manager) createNotification(title, body, priority, sourceAgentSlug, sourceType, link string) (map[string]interface{}, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	if priority == "" {
		priority = "normal"
	}

	_, err := m.db.Exec(
		`INSERT INTO notifications (id, title, body, priority, source_agent_slug, source_type, link, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, title, body, priority, sourceAgentSlug, sourceType, link, now,
	)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"id":                id,
		"title":             title,
		"body":              body,
		"priority":          priority,
		"source_agent_slug": sourceAgentSlug,
		"link":              link,
		"created_at":        now.Format(time.RFC3339),
	}, nil
}

func (m *Manager) createChatThread(title, message, agentSlug string) (string, error) {
	threadID := uuid.New().String()
	msgID := uuid.New().String()
	now := time.Now().UTC()

	// Create thread
	_, err := m.db.Exec(
		"INSERT INTO chat_threads (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)",
		threadID, title, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("create thread: %w", err)
	}

	// Create initial message from the agent
	_, err = m.db.Exec(
		`INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, created_at)
		 VALUES (?, ?, 'assistant', ?, ?, 0, 0, 0, ?)`,
		msgID, threadID, message, agentSlug, now,
	)
	if err != nil {
		return "", fmt.Errorf("create message: %w", err)
	}

	// Add agent as thread member
	m.db.Exec(
		"INSERT OR IGNORE INTO thread_members (thread_id, agent_role_slug, joined_at) VALUES (?, ?, ?)",
		threadID, agentSlug, now,
	)

	return threadID, nil
}
