package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/memory"
)

func (m *Manager) GatewayAnalyze(ctx context.Context, userMessage, threadID string, history []ThreadMessage, hints *GatewayRoutingHints) (*GatewayResponse, *llm.UsageInfo, error) {
	// Build dynamic agent list for gateway
	agentList := m.buildAgentList()
	gatewayPrompt := GatewayRoutingPromptFor(m.GatewayName())

	// Inject current date/time
	gatewayPrompt += fmt.Sprintf("\n\n## CURRENT TIME\n%s\n", time.Now().Format("Monday, January 2, 2006 at 3:04 PM MST"))

	if agentList != "" {
		gatewayPrompt += "\nAvailable specialist agents (use \"assigned_agent\" field with the slug):\n" + agentList
	}

	// Inject available tools info so gateway knows what tools exist for routing decisions
	if m.ToolMgr != nil {
		toolsSection := m.buildToolsPromptSection("")
		if toolsSection != "" {
			gatewayPrompt += "\n\n## SYSTEM TOOLS (read-only info for routing decisions)\n\n" + toolsSection
			gatewayPrompt += "\nWhen a user's request requires a tool (e.g. weather data, API calls), route to an agent that can use the tool — do NOT try to answer directly.\n"
		}
	}

	// Inject existing dashboards so gateway can match update requests
	dashSection := m.buildDashboardsPromptSection()
	if dashSection != "" {
		gatewayPrompt += "\n\n" + dashSection
	}

	// Inject user projects so gateway can resolve project references
	projectsSection := m.buildProjectsPromptSection()
	if projectsSection != "" {
		gatewayPrompt += "\n\n" + projectsSection
	}

	// Inject routing hints so gateway has full context
	if hints != nil {
		gatewayPrompt += "\n\n## ROUTING CONTEXT\n"
		if hints.LastResponder != "" {
			gatewayPrompt += fmt.Sprintf("- **Last responder**: `%s` (the agent who most recently replied in this thread)\n", hints.LastResponder)
		}
		if hints.MentionSlug != "" {
			gatewayPrompt += fmt.Sprintf("- **User @mentioned**: `%s` (the user explicitly tagged this agent)\n", hints.MentionSlug)
		}
		if len(hints.ThreadMembers) > 0 {
			gatewayPrompt += fmt.Sprintf("- **Thread members**: %s (agents already participating)\n", strings.Join(hints.ThreadMembers, ", "))
		}
	}

	// Build prompt with history context
	var promptBuilder strings.Builder
	promptBuilder.WriteString(gatewayPrompt)

	if len(history) > 0 {
		promptBuilder.WriteString("\n\nPrevious conversation:\n")
		for _, msg := range history {
			label := msg.Role
			if msg.AgentSlug != "" {
				label = fmt.Sprintf("assistant/%s", msg.AgentSlug)
			}
			promptBuilder.WriteString(fmt.Sprintf("[%s]: %s\n", label, msg.Content))
		}
		promptBuilder.WriteString(fmt.Sprintf("\nCurrent message:\n%s", userMessage))
	} else {
		promptBuilder.WriteString(fmt.Sprintf("\n\nUser message:\n%s", userMessage))
	}

	prompt := promptBuilder.String()

	// Inject todo list tools into gateway
	todoSection := buildTodoPromptSection(m.db)
	if todoSection != "" {
		prompt += "\n\n---\n\n" + todoSection
	}

	todoTools := BuildTodoToolDefs()
	todoHandlers := MakeTodoToolHandlers(m.db, "pounce", m.broadcast)

	result, err := m.client.RunAgentLoop(ctx, llm.AgentConfig{
		Model:         llm.ResolveModel(m.GatewayModel, llm.ModelHaiku),
		System:        "",
		MaxTurns:      3,
		ExtraTools:    todoTools,
		ExtraHandlers: todoHandlers,
		OnEvent: func(ev StreamEvent) {
			if ev.Type == EventTextDelta && ev.Text != "" {
				m.broadcast("gateway_thinking", map[string]interface{}{
					"thread_id": threadID,
					"text":      ev.Text,
				})
			}
		},
	}, prompt)
	if err != nil {
		return nil, nil, fmt.Errorf("gateway agent failed: %w", err)
	}

	usage := &llm.UsageInfo{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		CostUSD:      result.TotalCostUSD,
	}

	var resp GatewayResponse
	output := strings.TrimSpace(result.Text)

	jsonStart := strings.Index(output, "{")
	jsonEnd := strings.LastIndex(output, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		output = output[jsonStart : jsonEnd+1]
	}

	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		resp = GatewayResponse{
			Action:  "respond",
			Message: result.Text,
		}
	}

	return &resp, usage, nil
}

// ProjectContext carries resolved project info from the gateway to the agent.
type ProjectContext struct {
	ProjectName string `json:"project_name"`
	Directory   string `json:"directory"`
	ToolID      string `json:"tool_id,omitempty"`
}

type GatewayResponse struct {
	Action         string            `json:"action"`
	Message        string            `json:"message"`
	ThreadTitle    string            `json:"thread_title,omitempty"`
	AssignedAgent  string            `json:"assigned_agent,omitempty"`
	WorkOrder      *GatewayWorkOrder `json:"work_order,omitempty"`
	MemoryNote     string            `json:"memory_note,omitempty"`
	ProjectContext *ProjectContext   `json:"project_context,omitempty"`
}

type GatewayWorkOrder struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Requirements string `json:"requirements"`
	Type         string `json:"type"`
	ToolID       string `json:"tool_id,omitempty"`
	DashboardID  string `json:"dashboard_id,omitempty"`
}

func (m *Manager) GatewaySummarize(ctx context.Context, workOrderID, builderOutput string) (string, error) {
	wo, err := GetWorkOrder(m.db, workOrderID)
	if err != nil {
		return "", fmt.Errorf("get work order for summary: %w", err)
	}

	output := builderOutput
	if len(output) > 20000 {
		output = output[:10000] + "\n\n... [output truncated] ...\n\n" + output[len(output)-10000:]
	}

	prompt := fmt.Sprintf(BuildSummaryPrompt, wo.Title, wo.Type, wo.Description, output)

	text, _, err := m.client.RunOneShot(ctx, llm.ResolveModel(m.GatewayModel, llm.ModelHaiku), "", prompt)
	if err != nil {
		return "", fmt.Errorf("gateway summarize failed: %w", err)
	}

	return strings.TrimSpace(text), nil
}

// RoleChatTools are the tools available to agents with identity files.
var RoleChatTools = []string{"Read", "Write", "Edit"}

func (m *Manager) RoleChat(ctx context.Context, systemPrompt, model string, history []ThreadMessage, userMessage, threadID, agentDir, agentRoleSlug string) (string, *llm.UsageInfo, string, error) {
	resolvedModel := llm.ResolveModel(model, llm.ModelSonnet)

	// Build history messages for multi-turn conversation.
	// Messages from OTHER agents are re-attributed as user-role context so the
	// current agent doesn't mistakenly think it authored them.
	var historyMsgs []llm.HistoryMessage
	for _, msg := range history {
		role := msg.Role
		content := msg.Content
		if role == "system" {
			role = "assistant"
		}
		if role != "user" && role != "assistant" {
			continue
		}
		// If an assistant message came from a different agent, present it as
		// third-party context so the current agent knows it didn't say this.
		if role == "assistant" && msg.AgentSlug != "" && msg.AgentSlug != agentRoleSlug {
			role = "user"
			content = fmt.Sprintf("[Message from @%s — not you]:\n%s", msg.AgentSlug, content)
		}
		// Merge consecutive same-role messages
		if len(historyMsgs) > 0 && historyMsgs[len(historyMsgs)-1].Role == role {
			historyMsgs[len(historyMsgs)-1].Content += "\n\n" + content
		} else {
			historyMsgs = append(historyMsgs, llm.HistoryMessage{Role: role, Content: content})
		}
	}
	// Ensure history starts with user (API requirement).
	// If it starts with assistant (e.g. heartbeat-initiated thread), convert the
	// leading assistant message to a user-role context block so it isn't dropped.
	if len(historyMsgs) > 0 && historyMsgs[0].Role != "user" {
		historyMsgs[0] = llm.HistoryMessage{
			Role:    "user",
			Content: "[Your previous message in this thread]:\n" + historyMsgs[0].Content,
		}
	}
	// Ensure history ends with assistant (so the new user message can follow)
	if len(historyMsgs) > 0 && historyMsgs[len(historyMsgs)-1].Role != "assistant" {
		historyMsgs = historyMsgs[:len(historyMsgs)-1]
	}

	collector := llm.NewWidgetCollector()

	// Append current date/time to system prompt
	systemPrompt += fmt.Sprintf("\n\nCurrent time: %s", time.Now().Format("Monday, January 2, 2006 at 3:04 PM MST"))

	cfg := llm.AgentConfig{
		Model:   resolvedModel,
		System:  systemPrompt,
		History: historyMsgs,
		OnEvent: func(ev StreamEvent) {
			if ev.Type == EventToolStart {
				collector.TrackToolStart(ev.ToolName, ev.ToolID, ev.ToolInput)
				m.db.LogAudit("system", "agent_tool_call", "tool_call", "agent_role", agentRoleSlug, ev.ToolName)
			}
			if ev.Type == EventToolEnd && ev.ToolOutput != "" {
				collector.Collect(ev.ToolName, ev.ToolID, ev.ToolOutput)
			}
			if threadID == "" {
				return
			}
			m.broadcast("agent_stream", map[string]interface{}{
				"thread_id":       threadID,
				"agent_role_slug": agentRoleSlug,
				"event":           ev,
			})
		},
	}

	if agentDir != "" {
		cfg.Tools = RoleChatTools
		cfg.WorkDir = agentDir
		cfg.SandboxPaths = []string{agentDir}
		cfg.MaxTurns = m.MaxTurns
	} else {
		cfg.MaxTurns = 1
	}

	// Inject available tools into system prompt and add call_tool capability
	if m.ToolMgr != nil {
		toolsSection := m.buildToolsPromptSection(agentRoleSlug)
		if toolsSection != "" {
			cfg.System += "\n\n---\n\n" + toolsSection
			cfg.ExtraTools = append(cfg.ExtraTools, llm.BuildCallToolDef())
			cfg.ExtraHandlers = map[string]llm.ToolHandler{
				"call_tool": m.makeCallToolHandler(),
			}
		}
	}

	// Inject memory tools so agents can save/search memories across conversations
	if m.MemoryMgr != nil {
		m.MemoryMgr.EnsureMigrated(agentRoleSlug)
		cfg.ExtraTools = append(cfg.ExtraTools, memory.BuildMemoryToolDefs()...)
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		for name, handler := range m.MemoryMgr.MakeMemoryHandlers(agentRoleSlug) {
			cfg.ExtraHandlers[name] = handler
		}
	}

	// Inject todo list tools
	todoSection := buildTodoPromptSection(m.db)
	if todoSection != "" {
		cfg.System += "\n\n---\n\n" + todoSection
	}
	cfg.ExtraTools = append(cfg.ExtraTools, BuildTodoToolDefs()...)
	if cfg.ExtraHandlers == nil {
		cfg.ExtraHandlers = map[string]llm.ToolHandler{}
	}
	for name, handler := range MakeTodoToolHandlers(m.db, agentRoleSlug, m.broadcast) {
		cfg.ExtraHandlers[name] = handler
	}

	// Inject image generation tool (uses Gemini Flash by default, FAL optional)
	if m.client != nil && m.client.IsConfigured() {
		falAvailable := m.FalClient != nil && m.FalClient.IsConfigured()
		cfg.ExtraTools = append(cfg.ExtraTools, BuildGenerateImageDef(falAvailable))
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		cfg.ExtraHandlers["generate_image"] = m.makeGenerateImageHandler()
		falNote := ""
		if falAvailable {
			falNote = " You also have access to FAL FLUX models — but ONLY use provider 'fal' when the user explicitly requests FAL or FLUX."
		}
		cfg.System += "\n\n## IMAGE GENERATION\nYou can generate images using the `generate_image` tool. It uses Gemini Flash by default." + falNote + "\n"
	}

	// Inject delegate_task if other agents are available for delegation
	availableAgents := m.getAvailableAgentsForDelegation(agentRoleSlug)
	if len(availableAgents) > 0 {
		cfg.ExtraTools = append(cfg.ExtraTools, llm.BuildDelegateTaskDef())
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		cfg.ExtraHandlers["delegate_task"] = m.makeDelegateTaskHandler(threadID, agentRoleSlug)
		cfg.System += "\n\n" + buildDelegationPromptSection(availableAgents)
	}

	result, err := m.client.RunAgentLoop(ctx, cfg, userMessage)
	if err != nil {
		return "", nil, "", fmt.Errorf("role chat failed: %w", err)
	}

	responseText := strings.TrimSpace(result.Text)

	// If the agent hit max turns or timed out while still working, append a notice
	if result.StopReason == "max_turns" {
		responseText += "\n\n---\n*I hit my turn limit before finishing. Say **continue** and I'll pick up where I left off.*"
	}
	if result.StopReason == "cancelled" && ctx.Err() == context.DeadlineExceeded {
		responseText += "\n\n---\n*I hit my time limit before finishing. Say **continue** and I'll pick up where I left off.*"
		// Return partial result without error so it gets saved
		usage := &llm.UsageInfo{
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
			CostUSD:      result.TotalCostUSD,
		}
		return responseText, usage, collector.JSON(), nil
	}

	usage := &llm.UsageInfo{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		CostUSD:      result.TotalCostUSD,
	}

	m.db.LogAudit("system", "agent_response", "agent", "agent_role", agentRoleSlug,
		fmt.Sprintf("%s tokens=%d+%d cost=$%.4f", agentRoleSlug, result.InputTokens, result.OutputTokens, result.TotalCostUSD))

	return responseText, usage, collector.JSON(), nil
}

// SendScheduledPrompt sends a prompt to an agent role and returns the response.
// If threadID is provided, the message is persisted to that chat thread.
// If threadID is empty, a new thread is created.
func (m *Manager) SendScheduledPrompt(ctx context.Context, agentSlug, prompt, threadID string) (response, usedThreadID string, err error) {
	var systemPrompt, model string
	var identityInitialized bool
	err = m.db.QueryRow(
		"SELECT system_prompt, model, identity_initialized FROM agent_roles WHERE slug = ? AND enabled = 1",
		agentSlug,
	).Scan(&systemPrompt, &model, &identityInitialized)
	if err != nil {
		return "", "", fmt.Errorf("agent role %q not found or disabled: %w", agentSlug, err)
	}

	now := time.Now().UTC()

	// Create thread if none provided
	if threadID == "" {
		threadID = uuid.New().String()
		m.db.Exec(
			"INSERT INTO chat_threads (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)",
			threadID, "Scheduled: "+prompt[:min(len(prompt), 40)], now, now,
		)
	}

	// Save the user message to the thread
	userMsgID := uuid.New().String()
	m.db.Exec(
		"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, created_at) VALUES (?, ?, 'user', ?, ?, ?)",
		userMsgID, threadID, prompt, agentSlug, now,
	)
	m.db.Exec("UPDATE chat_threads SET updated_at = ? WHERE id = ?", now, threadID)

	// Load thread history for context
	var history []ThreadMessage
	rows, err := m.db.Query(
		"SELECT role, content FROM chat_messages WHERE thread_id = ? ORDER BY created_at ASC",
		threadID,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var tm ThreadMessage
			if rows.Scan(&tm.Role, &tm.Content) == nil {
				history = append(history, tm)
			}
		}
	}

	// If identity system is initialized, assemble prompt from files and set agentDir
	// so the agent gets tools and full max turns (same as normal chat).
	var agentDir string
	if identityInitialized {
		assembled, err := AssembleSystemPrompt(m.DataDir, agentSlug)
		if err == nil {
			systemPrompt = assembled
			agentDir = AgentDir(m.DataDir, agentSlug)
		}
	}

	result, usage, _, chatErr := m.RoleChat(ctx, systemPrompt, model, history, prompt, threadID, agentDir, agentSlug)
	if chatErr != nil {
		return "", threadID, fmt.Errorf("scheduled prompt failed: %w", chatErr)
	}

	// Save the assistant response to the thread
	assistMsgID := uuid.New().String()
	assistNow := time.Now().UTC()
	var costUSD float64
	var inputTokens, outputTokens int64
	if usage != nil {
		costUSD = usage.CostUSD
		inputTokens = usage.InputTokens
		outputTokens = usage.OutputTokens
	}
	m.db.Exec(
		"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, created_at) VALUES (?, ?, 'assistant', ?, ?, ?, ?, ?, ?)",
		assistMsgID, threadID, result, agentSlug, costUSD, inputTokens, outputTokens, assistNow,
	)
	m.db.Exec("UPDATE chat_threads SET updated_at = ? WHERE id = ?", assistNow, threadID)

	return result, threadID, nil
}
