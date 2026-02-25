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

	m.mu.RLock()
	enabled := m.config.Enabled
	m.mu.RUnlock()

	if m.started {
		// Restart the tick loop to pick up new settings
		m.Stop()
		if enabled {
			m.Start()
		}
	} else if enabled {
		// Heartbeat was disabled at boot but just got enabled — start it
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

// findRecentThread looks up the most recent active thread for this agent.
func (m *Manager) findRecentThread(slug string, maxAge time.Duration) (threadID, threadTitle string) {
	cutoff := time.Now().UTC().Add(-maxAge).Format("2006-01-02 15:04:05")
	err := m.db.QueryRow(
		`SELECT ct.id, ct.title FROM chat_threads ct
		 JOIN thread_members tm ON ct.id = tm.thread_id
		 WHERE tm.agent_role_slug = ? AND ct.updated_at > ?
		 ORDER BY ct.updated_at DESC LIMIT 1`,
		slug, cutoff,
	).Scan(&threadID, &threadTitle)
	if err != nil {
		return "", ""
	}
	return threadID, threadTitle
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

	// Look up existing thread server-side (no reliance on agent memory_search)
	existingThreadID, existingThreadTitle := m.findRecentThread(slug, 7*24*time.Hour)

	// Build thread continuation instructions
	var threadInstructions string
	if existingThreadID != "" {
		threadInstructions = fmt.Sprintf(
			"You have an active thread: ID=%s title=%q. You MUST use send_message with this thread_id. DO NOT call create_chat.",
			existingThreadID, existingThreadTitle,
		)
	} else {
		threadInstructions = "You have no active thread. Use create_chat if you need to communicate with the user."
	}

	// Build task board summary
	taskBoardSummary := m.buildTaskBoardSummary(slug)
	todoSummary := m.buildHeartbeatTodoSummary()

	// Add heartbeat-mode instructions
	systemPrompt += fmt.Sprintf(`

---

## HEARTBEAT MODE

You are running in **heartbeat mode** — a periodic, autonomous wake-up. You have a maximum of **5 turns**, so be focused and efficient.

Current time: %s

### Your Workflow

Every heartbeat, follow this loop:

1. **Check your task board** (shown below). Look at what's in "doing" and "blocked" first.
2. **Pick 1-2 tasks to advance.** Move a "backlog" item to "doing" before you start working on it. When you finish, move it to "done".
3. **If a task is blocked**, note WHY in a chat message to the user so they can help unblock you.
4. **If you completed something significant** (finished a multi-step task, resolved a blocked item, accomplished a goal), send a message in chat to let the user know what was done and what you learned.
5. **If your board is empty** (no backlog, no doing, no blocked), message the user to ask for more work or suggest tasks based on your role and recent context.
6. **If everything is quiet** and you have nothing actionable, use no_action with a clear reason.

### Writing Good Tasks

When creating tasks for yourself, write them so your future self (waking up fresh) can understand and act on them:

- **Title**: Short, action-oriented. Start with a verb. e.g. "Review error logs from last 24h", "Draft weekly summary for user", "Check if API key rotation is due"
- **Description**: Include enough context that you don't need to remember anything. Add specifics: file paths, thread IDs, dates, what "done" looks like.
- **Break big work into steps**: Instead of "Reorganize everything", create: "List files that need cleanup" → "Draft reorganization plan" → "Send plan to user for approval"
- **Use status correctly**:
  - **backlog**: Queued for later. You'll get to it in a future heartbeat.
  - **doing**: You're actively working on this RIGHT NOW in this heartbeat cycle.
  - **blocked**: You can't proceed without user input, external info, or another task finishing. Always explain the blocker in the description or in a chat message.
  - **done**: Completed. Move tasks here when finished.

### When to Communicate

- **Send a chat message** when: you completed something the user should know about, you're blocked and need help, you have no tasks and want direction, or you have a suggestion or finding worth sharing.
- **Send a notification** for: quick FYI items that don't need a conversation (e.g. "Daily check complete, all good").
- **Prefer send_message** over create_chat when you have an active thread — it keeps the conversation in one place.

### Tools

- **task_list**: List all your current tasks. Call this to refresh your view.
- **task_create**: Create a task. Parameters: title (required), description, status (default: backlog)
- **task_move**: Move a task to a new column. Parameters: task_id (required), status (required)
- **create_chat**: Start a NEW chat thread. Parameters: title (required), message (required)
- **send_message**: Send a message to an EXISTING thread. Parameters: thread_id (required), message (required). If the thread was deleted, fall back to create_chat.
- **create_notification**: Quick notification to the user. Parameters: title (required), body, priority (low/normal/high), link
- **no_action**: Nothing to do. Parameters: reason (required)
- **Memory tools**: memory_save, memory_search, memory_list, memory_update, memory_forget, memory_stats — use these to remember things across heartbeats.
- **todo_list_all**: List all user todo lists
- **todo_list_items**: View items in a todo list
- **todo_add_item**: Add an item to a todo list
- **todo_check_item**: Mark a todo item as completed
- **todo_uncheck_item**: Mark a todo item as incomplete

### Thread Status

%s
%s
%s
Now read your heartbeat instructions below and take action.`, time.Now().Format("Monday, January 2, 2006 at 3:04 PM MST"), threadInstructions, taskBoardSummary, todoSummary)

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
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "task_create",
				Description: "Create a task on your Kanban board",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","description":"Task title"},"description":{"type":"string","description":"Task description"},"status":{"type":"string","enum":["backlog","doing","blocked","done"],"description":"Initial status (default: backlog)"}},"required":["title"]}`),
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "task_move",
				Description: "Move a task to a different status column",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"string","description":"ID of the task to move"},"status":{"type":"string","enum":["backlog","doing","blocked","done"],"description":"New status"}},"required":["task_id","status"]}`),
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "task_list",
				Description: "List all your current tasks",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
	}

	// Append todo tools
	extraTools = append(extraTools, agents.BuildTodoToolDefs()...)

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

			// Auto-save thread ID to memory for future heartbeat lookups
			if m.agentMgr.MemoryMgr != nil {
				m.agentMgr.MemoryMgr.SaveNote(slug,
					fmt.Sprintf("Heartbeat thread: ID=%s title=%q", threadID, params.Title),
					"heartbeat")
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
		"task_create": func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
			var params struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				Status      string `json:"status"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
			}
			if params.Title == "" {
				return llm.ToolResult{Output: "title is required", IsError: true}
			}
			if params.Status == "" {
				params.Status = "backlog"
			}
			id := uuid.New().String()
			now := time.Now().UTC()
			var maxOrder int
			m.db.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM agent_tasks WHERE agent_role_slug = ? AND status = ?", slug, params.Status).Scan(&maxOrder)
			_, err := m.db.Exec(
				"INSERT INTO agent_tasks (id, agent_role_slug, title, description, status, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
				id, slug, params.Title, params.Description, params.Status, maxOrder+1, now, now,
			)
			if err != nil {
				return llm.ToolResult{Output: "Failed to create task: " + err.Error(), IsError: true}
			}
			actionsTaken = append(actionsTaken, "task_create: "+params.Title)
			return llm.ToolResult{Output: fmt.Sprintf("Task created: %s (status: %s)", id, params.Status)}
		},
		"task_move": func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
			var params struct {
				TaskID string `json:"task_id"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
			}
			if params.TaskID == "" || params.Status == "" {
				return llm.ToolResult{Output: "task_id and status are required", IsError: true}
			}
			validStatuses := map[string]bool{"backlog": true, "doing": true, "blocked": true, "done": true}
			if !validStatuses[params.Status] {
				return llm.ToolResult{Output: "invalid status — use backlog, doing, blocked, or done", IsError: true}
			}
			now := time.Now().UTC()
			result, err := m.db.Exec("UPDATE agent_tasks SET status = ?, updated_at = ? WHERE id = ? AND agent_role_slug = ?", params.Status, now, params.TaskID, slug)
			if err != nil {
				return llm.ToolResult{Output: "Failed to move task: " + err.Error(), IsError: true}
			}
			n, _ := result.RowsAffected()
			if n == 0 {
				return llm.ToolResult{Output: "Task not found", IsError: true}
			}
			actionsTaken = append(actionsTaken, "task_move: "+params.TaskID+" → "+params.Status)
			return llm.ToolResult{Output: fmt.Sprintf("Task moved to %s", params.Status)}
		},
		"task_list": func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
			return llm.ToolResult{Output: m.buildTaskBoardSummary(slug)}
		},
	}

	// Register memory handlers
	if m.agentMgr.MemoryMgr != nil {
		for name, handler := range m.agentMgr.MemoryMgr.MakeMemoryHandlers(slug) {
			extraHandlers[name] = handler
		}
	}

	// Register todo handlers
	for name, handler := range agents.MakeTodoToolHandlers(m.db, slug, m.broadcast) {
		extraHandlers[name] = handler
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

func (m *Manager) buildTaskBoardSummary(slug string) string {
	rows, err := m.db.Query(
		"SELECT id, title, description, status FROM agent_tasks WHERE agent_role_slug = ? AND status != 'done' ORDER BY CASE status WHEN 'doing' THEN 0 WHEN 'blocked' THEN 1 WHEN 'backlog' THEN 2 END, sort_order ASC",
		slug,
	)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var doing, blocked, backlog []string
	for rows.Next() {
		var id, title, description, status string
		if rows.Scan(&id, &title, &description, &status) != nil {
			continue
		}
		line := fmt.Sprintf("- **%s** (id: %s)", title, id)
		if description != "" {
			// Truncate long descriptions
			desc := description
			if len(desc) > 120 {
				desc = desc[:120] + "..."
			}
			line += "\n  " + desc
		}
		switch status {
		case "doing":
			doing = append(doing, line)
		case "blocked":
			blocked = append(blocked, line)
		default:
			backlog = append(backlog, line)
		}
	}

	if len(doing) == 0 && len(blocked) == 0 && len(backlog) == 0 {
		return "\n### Task Board\n\nYour board is empty. Consider creating tasks for yourself based on your role, or message the user to ask what you should work on.\n"
	}

	var sb strings.Builder
	sb.WriteString("\n### Task Board\n")
	if len(doing) > 0 {
		sb.WriteString("\n**DOING** (pick up where you left off):\n")
		sb.WriteString(strings.Join(doing, "\n"))
		sb.WriteString("\n")
	}
	if len(blocked) > 0 {
		sb.WriteString("\n**BLOCKED** (needs user help or external input):\n")
		sb.WriteString(strings.Join(blocked, "\n"))
		sb.WriteString("\n")
	}
	if len(backlog) > 0 {
		sb.WriteString("\n**BACKLOG** (ready to start):\n")
		sb.WriteString(strings.Join(backlog, "\n"))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m *Manager) buildHeartbeatTodoSummary() string {
	rows, err := m.db.Query(`
		SELECT tl.name,
			(SELECT COUNT(*) FROM todo_items WHERE list_id = tl.id) as total,
			(SELECT COUNT(*) FROM todo_items WHERE list_id = tl.id AND completed = 1) as done
		FROM todo_lists tl ORDER BY tl.sort_order ASC`)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var name string
		var total, done int
		if rows.Scan(&name, &total, &done) != nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s (%d items, %d done)", name, total, done))
	}
	if len(lines) == 0 {
		return ""
	}
	return "\n### User Todo Lists\nThe user has todo lists you can manage with todo_* tools:\n" + strings.Join(lines, "\n") + "\n"
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
