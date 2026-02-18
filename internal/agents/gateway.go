package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	llm "github.com/openpaw/openpaw/internal/llm"
)

func (m *Manager) GatewayAnalyze(ctx context.Context, userMessage, threadID string, history []ThreadMessage, hints *GatewayRoutingHints) (*GatewayResponse, *llm.UsageInfo, error) {
	// Build dynamic agent list for gateway
	agentList := m.buildAgentList()
	gatewayPrompt := GatewayPrompt

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

	result, err := m.client.RunAgentLoop(ctx, llm.AgentConfig{
		Model:    llm.ResolveModel(m.GatewayModel, llm.ModelHaiku),
		System:   "",
		MaxTurns: 1,
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

type GatewayResponse struct {
	Action        string            `json:"action"`
	Message       string            `json:"message"`
	ThreadTitle   string            `json:"thread_title,omitempty"`
	AssignedAgent string            `json:"assigned_agent,omitempty"`
	WorkOrder     *GatewayWorkOrder `json:"work_order,omitempty"`
	MemoryNote    string            `json:"memory_note,omitempty"`
}

type GatewayWorkOrder struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Requirements string `json:"requirements"`
	Type         string `json:"type"`
	ToolID       string `json:"tool_id,omitempty"`
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

	// Build history messages for multi-turn conversation
	var historyMsgs []llm.HistoryMessage
	for _, msg := range history {
		role := msg.Role
		if role == "system" {
			role = "assistant"
		}
		if role != "user" && role != "assistant" {
			continue
		}
		// Merge consecutive same-role messages
		if len(historyMsgs) > 0 && historyMsgs[len(historyMsgs)-1].Role == role {
			historyMsgs[len(historyMsgs)-1].Content += "\n\n" + msg.Content
		} else {
			historyMsgs = append(historyMsgs, llm.HistoryMessage{Role: role, Content: msg.Content})
		}
	}
	// Ensure history starts with user (API requirement)
	if len(historyMsgs) > 0 && historyMsgs[0].Role != "user" {
		historyMsgs = historyMsgs[1:]
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
			if ev.Type == EventToolEnd && ev.ToolOutput != "" {
				collector.Collect(ev.ToolName, ev.ToolID, ev.ToolOutput)
			}
			if ev.Type == EventToolStart {
				m.db.LogAudit("system", "agent_tool_call", "tool_call", "agent_role", agentRoleSlug, ev.ToolName)
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
func (m *Manager) SendScheduledPrompt(ctx context.Context, agentSlug, prompt string) (string, error) {
	// Look up agent role
	var systemPrompt, model string
	err := m.db.QueryRow(
		"SELECT system_prompt, model FROM agent_roles WHERE slug = ? AND enabled = 1",
		agentSlug,
	).Scan(&systemPrompt, &model)
	if err != nil {
		return "", fmt.Errorf("agent role %q not found or disabled: %w", agentSlug, err)
	}

	result, _, _, err := m.RoleChat(ctx, systemPrompt, model, nil, prompt, "", "", agentSlug)
	if err != nil {
		return "", fmt.Errorf("scheduled prompt failed: %w", err)
	}

	return result, nil
}
