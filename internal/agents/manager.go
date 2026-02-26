package agents

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/fal"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/models"
)

const maxConcurrentAgents = 5

type BroadcastFunc func(msgType string, payload interface{})

// ToolManager is the interface for the tool process manager (avoids circular imports).
type ToolManager interface {
	CompileTool(toolID string) error
	StartTool(toolID string) error
	WaitForHealth(toolID string, timeout time.Duration) error
	GetStatus(toolID string) map[string]interface{}
	CallTool(toolID, endpoint string, payload []byte) ([]byte, error)
	CallToolWithContext(ctx context.Context, toolID, endpoint string, payload []byte) ([]byte, error)
}

// MemoryManager is the interface for the per-agent memory system (avoids circular imports).
type MemoryManager interface {
	SaveNote(slug, note, source string) error
	EnsureMigrated(slug string)
	BuildMemoryPromptSection(slug string) string
	MakeMemoryHandlers(slug string) map[string]llm.ToolHandler
	Close()
}

// BrowserManager is the interface for the browser session manager (avoids circular imports).
// The agents package stores a reference but doesn't call methods on it directly;
// the server package passes it to handlers that use the concrete type.
type BrowserManager interface{}


// StreamState tracks the live streaming output of an active agent for a thread.
type StreamState struct {
	mu        sync.Mutex   `json:"-"`
	Active    bool         `json:"active"`
	Text      string       `json:"text"`
	AgentSlug string       `json:"agent_slug"`
	Tools     []StreamTool `json:"tools"`
}

type StreamTool struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	Done   bool   `json:"done"`
	Detail string `json:"detail,omitempty"`
}

type Manager struct {
	db              *database.DB
	toolsDir        string
	DataDir         string
	DashboardsDir   string
	agents          map[string]*runningAgent
	mu              sync.RWMutex
	broadcast       BroadcastFunc
	client          *llm.Client
	GatewayModel    string
	BuilderModel    string
	MaxTurns        int
	AgentTimeoutMin int
	AutoCompactEnabled   bool
	AutoCompactThreshold int  // percentage (0-100), default 85
	ContextLimitOverride int  // 0 = use model default
	ToolMgr              ToolManager
	MemoryMgr            MemoryManager
	BrowserMgr           BrowserManager
	FalClient            *fal.Client
	NotifyFn             func(title, body, priority, sourceAgentSlug, sourceType, link string)
	manifestCache        sync.Map // map[toolID][]byte
	streamStates    sync.Map // map[threadID]*StreamState
	activeSubAgents int32    // atomic counter for concurrent sub-agents
}

type runningAgent struct {
	agent  models.Agent
	cancel func()
	doneCh chan struct{}
}

func NewManager(db *database.DB, toolsDir string, broadcast BroadcastFunc, client *llm.Client) *Manager {
	return &Manager{
		db:                   db,
		toolsDir:             toolsDir,
		agents:               make(map[string]*runningAgent),
		broadcast:            broadcast,
		client:               client,
		GatewayModel:         llm.ModelHaiku,
		BuilderModel:         llm.ModelSonnet,
		MaxTurns:             300,
		AgentTimeoutMin:      60,
		AutoCompactEnabled:   true,
		AutoCompactThreshold: 85,
		ContextLimitOverride: 0,
	}
}

func ParseModel(s string, fallback string) string {
	return llm.ResolveModel(s, fallback)
}

func (m *Manager) AgentTimeout() time.Duration {
	if m.AgentTimeoutMin <= 0 {
		return 60 * time.Minute
	}
	return time.Duration(m.AgentTimeoutMin) * time.Minute
}

func (m *Manager) Broadcast(msgType string, payload interface{}) {
	m.broadcast(msgType, payload)
}

func (m *Manager) Client() *llm.Client {
	return m.client
}

// GatewayName returns the configured gateway name from the builder role, falling back to "Pounce".
func (m *Manager) GatewayName() string {
	var name string
	if err := m.db.QueryRow("SELECT name FROM agent_roles WHERE slug = 'builder'").Scan(&name); err == nil && name != "" {
		return name
	}
	return "Pounce"
}

func (m *Manager) buildAgentList() string {
	rows, err := m.db.Query(
		`SELECT ar.slug, ar.name, ar.description, COALESCE(GROUP_CONCAT(t.name), '')
		 FROM agent_roles ar
		 LEFT JOIN agent_tool_access ata ON ata.agent_role_slug = ar.slug
		 LEFT JOIN tools t ON t.id = ata.tool_id
		 WHERE ar.enabled = 1
		 GROUP BY ar.slug, ar.name, ar.description
		 ORDER BY ar.sort_order ASC`,
	)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var sb strings.Builder
	for rows.Next() {
		var slug, name, desc, toolsCSV string
		if err := rows.Scan(&slug, &name, &desc, &toolsCSV); err != nil {
			logger.Warn("scan agent role row: %v", err)
			continue
		}
		sb.WriteString(fmt.Sprintf("- %s (slug: %s): %s", name, slug, desc))
		if toolsCSV != "" {
			sb.WriteString(fmt.Sprintf(" [tools: %s]", toolsCSV))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func CheckAPIKey(client *llm.Client) {
	if client == nil || !client.IsConfigured() {
		logger.Warn("OpenRouter API key not configured — chat agents will not work")
		logger.Warn("Set OPENROUTER_API_KEY env var or configure via Settings")
		return
	}
	logger.Success("OpenRouter API key configured")
}

// ThreadMessage represents a message from the chat thread history.
type ThreadMessage struct {
	ID        string
	Role      string
	Content   string
	AgentSlug string
}

// GatewayRoutingHints provides context to the gateway for smarter routing decisions.
type GatewayRoutingHints struct {
	LastResponder string   // slug of agent who last responded in the thread
	MentionSlug   string   // slug if user @mentioned an agent in this message
	ThreadMembers []string // slugs of all agents currently in the thread
}

func (m *Manager) updateAgentStatus(agentID, status, output, errMsg string) {
	now := time.Now().UTC()
	if status == "completed" || status == "failed" {
		m.db.Exec(
			"UPDATE agents SET status = ?, output = ?, error = ?, completed_at = ?, updated_at = ? WHERE id = ?",
			status, output, errMsg, now, now, agentID,
		)
	} else {
		m.db.Exec(
			"UPDATE agents SET status = ?, output = ?, error = ?, updated_at = ? WHERE id = ?",
			status, output, errMsg, now, agentID,
		)
	}
}

func (m *Manager) StopAgent(agentID string) error {
	m.mu.Lock()
	ra, exists := m.agents[agentID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("agent not found or not running")
	}
	delete(m.agents, agentID)
	m.mu.Unlock()

	ra.cancel()
	<-ra.doneCh

	m.updateAgentStatus(agentID, "stopped", "", "stopped by user")

	if ra.agent.WorkOrderID != "" {
		UpdateWorkOrderStatus(m.db, ra.agent.WorkOrderID, WorkOrderFailed, "agent stopped by user")
	}

	logger.Warn("Stopped agent %s", agentID)
	return nil
}

func (m *Manager) ListActiveAgents() []models.Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]models.Agent, 0, len(m.agents))
	for _, ra := range m.agents {
		result = append(result, ra.agent)
	}
	return result
}

func (m *Manager) ListAllAgents() ([]models.Agent, error) {
	rows, err := m.db.Query(
		`SELECT id, type, status, model, work_order_id, pid, working_dir, output, error, started_at, completed_at, created_at, updated_at
		 FROM agents ORDER BY created_at DESC LIMIT 50`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []models.Agent
	for rows.Next() {
		var a models.Agent
		if err := rows.Scan(&a.ID, &a.Type, &a.Status, &a.Model, &a.WorkOrderID,
			&a.PID, &a.WorkingDir, &a.Output, &a.Error, &a.StartedAt, &a.CompletedAt,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, nil
}

func (m *Manager) GetAgent(agentID string) (*models.Agent, error) {
	var a models.Agent
	err := m.db.QueryRow(
		`SELECT id, type, status, model, work_order_id, pid, working_dir, output, error, started_at, completed_at, created_at, updated_at
		 FROM agents WHERE id = ?`, agentID,
	).Scan(&a.ID, &a.Type, &a.Status, &a.Model, &a.WorkOrderID,
		&a.PID, &a.WorkingDir, &a.Output, &a.Error, &a.StartedAt, &a.CompletedAt,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (m *Manager) Shutdown() {
	m.mu.Lock()
	agentsCopy := make(map[string]*runningAgent)
	for k, v := range m.agents {
		agentsCopy[k] = v
	}
	m.agents = make(map[string]*runningAgent)
	m.mu.Unlock()

	for id, ra := range agentsCopy {
		ra.cancel()
		<-ra.doneCh
		m.updateAgentStatus(id, "stopped", "", "server shutdown")
		logger.Info("Stopped agent %s during shutdown", id)
	}
}

func generateAgentID() string {
	return uuid.New().String()
}

// GetStreamState returns a snapshot of the current streaming state for a thread, or nil if not streaming.
func (m *Manager) GetStreamState(threadID string) *StreamState {
	v, ok := m.streamStates.Load(threadID)
	if !ok {
		return nil
	}
	state := v.(*StreamState)
	state.mu.Lock()
	snapshot := &StreamState{
		Active:    state.Active,
		Text:      state.Text,
		AgentSlug: state.AgentSlug,
		Tools:     make([]StreamTool, len(state.Tools)),
	}
	copy(snapshot.Tools, state.Tools)
	state.mu.Unlock()
	return snapshot
}

// UpdateStreamText appends text to the streaming state for a thread.
func (m *Manager) UpdateStreamText(threadID, agentSlug, text string) {
	v, _ := m.streamStates.LoadOrStore(threadID, &StreamState{Active: true, AgentSlug: agentSlug})
	state := v.(*StreamState)
	state.mu.Lock()
	state.Text += text
	state.AgentSlug = agentSlug
	state.Active = true
	state.mu.Unlock()
}

// UpdateStreamTool updates or adds a tool in the streaming state for a thread.
func (m *Manager) UpdateStreamTool(threadID string, tool StreamTool) {
	v, _ := m.streamStates.LoadOrStore(threadID, &StreamState{Active: true})
	state := v.(*StreamState)
	state.mu.Lock()
	for i, t := range state.Tools {
		if t.ID == tool.ID {
			state.Tools[i] = tool
			state.mu.Unlock()
			return
		}
	}
	state.Tools = append(state.Tools, tool)
	state.mu.Unlock()
}

// ClearStreamState removes the streaming state for a thread.
func (m *Manager) ClearStreamState(threadID string) {
	m.streamStates.Delete(threadID)
}

// GatewayAnalyzeBootstrap is a convenience wrapper around GatewayAnalyze with no routing hints.
func (m *Manager) GatewayAnalyzeBootstrap(ctx context.Context, userMessage, threadID string, history []ThreadMessage) (*GatewayResponse, *llm.UsageInfo, error) {
	return m.GatewayAnalyze(ctx, userMessage, threadID, history, nil)
}
