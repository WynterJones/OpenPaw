package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/logger"
)

const (
	maxSubAgentTasks     = 5
	maxConcurrentSubAg   = 5
	subAgentMaxTurns     = 10
	subAgentTimeoutMin   = 10
)

// delegateAgentInfo holds info about an agent available for delegation.
type delegateAgentInfo struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// delegateTaskInput is the parsed input from the delegate_task tool call.
type delegateTaskInput struct {
	Tasks []struct {
		AgentSlug string `json:"agent_slug"`
		Task      string `json:"task"`
	} `json:"tasks"`
}

// delegateTaskResult is the result from a single sub-agent.
type delegateTaskResult struct {
	AgentSlug string `json:"agent_slug"`
	AgentName string `json:"agent_name"`
	Task      string `json:"task"`
	Status    string `json:"status"`
	Result    string `json:"result"`
	Error     string `json:"error,omitempty"`
	CostUSD   float64 `json:"cost_usd"`
}

// getAvailableAgentsForDelegation returns enabled agents that can be delegated to,
// excluding the parent agent itself.
func (m *Manager) getAvailableAgentsForDelegation(parentSlug string) []delegateAgentInfo {
	rows, err := m.db.Query(
		"SELECT slug, name, description FROM agent_roles WHERE enabled = 1 AND slug != ? ORDER BY sort_order ASC",
		parentSlug,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var agents []delegateAgentInfo
	for rows.Next() {
		var a delegateAgentInfo
		if err := rows.Scan(&a.Slug, &a.Name, &a.Description); err != nil {
			continue
		}
		agents = append(agents, a)
	}
	return agents
}

// buildDelegationPromptSection generates the system prompt section that teaches
// agents about delegation and lists available agents.
func buildDelegationPromptSection(agents []delegateAgentInfo) string {
	var sb strings.Builder
	sb.WriteString("## DELEGATION\n\n")
	sb.WriteString("You can delegate tasks to specialist agents who work **in parallel** using the `delegate_task` tool.\n\n")
	sb.WriteString("Use `delegate_task` when:\n")
	sb.WriteString("- Multiple independent subtasks would benefit from specialist expertise\n")
	sb.WriteString("- Parallel execution would save time (e.g. research + analysis simultaneously)\n")
	sb.WriteString("- A task falls outside your area of expertise\n\n")
	sb.WriteString("**Important guidelines:**\n")
	sb.WriteString("- Each sub-agent receives ONLY the task description (no conversation history)\n")
	sb.WriteString("- Write clear, self-contained task descriptions with all necessary context\n")
	sb.WriteString("- Sub-agents return text results — synthesize them into your response\n")
	sb.WriteString("- Maximum 5 tasks per delegation call\n\n")
	sb.WriteString("Available agents for delegation:\n")
	for _, a := range agents {
		sb.WriteString(fmt.Sprintf("- **%s** (slug: `%s`) — %s\n", a.Name, a.Slug, a.Description))
	}
	return sb.String()
}

// makeDelegateTaskHandler creates the tool handler for delegate_task.
func (m *Manager) makeDelegateTaskHandler(threadID, parentSlug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params delegateTaskInput
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}

		if len(params.Tasks) == 0 {
			return llm.ToolResult{Output: "No tasks provided", IsError: true}
		}
		if len(params.Tasks) > maxSubAgentTasks {
			return llm.ToolResult{
				Output:  fmt.Sprintf("Too many tasks: %d (max %d)", len(params.Tasks), maxSubAgentTasks),
				IsError: true,
			}
		}

		// Check global concurrent sub-agent limit
		current := atomic.LoadInt32(&m.activeSubAgents)
		if int(current)+len(params.Tasks) > maxConcurrentSubAg {
			return llm.ToolResult{
				Output:  fmt.Sprintf("Too many concurrent sub-agents: %d active + %d requested (max %d)", current, len(params.Tasks), maxConcurrentSubAg),
				IsError: true,
			}
		}

		// Validate all agent slugs exist and are enabled
		availableAgents := m.getAvailableAgentsForDelegation(parentSlug)
		agentMap := make(map[string]delegateAgentInfo)
		for _, a := range availableAgents {
			agentMap[a.Slug] = a
		}

		for _, task := range params.Tasks {
			if _, ok := agentMap[task.AgentSlug]; !ok {
				return llm.ToolResult{
					Output:  fmt.Sprintf("Agent '%s' not found or not available for delegation", task.AgentSlug),
					IsError: true,
				}
			}
		}

		// Broadcast delegation started
		m.broadcast("subagent_status", map[string]interface{}{
			"thread_id":   threadID,
			"parent_slug": parentSlug,
			"status":      "delegation_started",
			"task_count":  len(params.Tasks),
		})

		// Spawn all sub-agents in parallel
		var wg sync.WaitGroup
		results := make([]delegateTaskResult, len(params.Tasks))

		for i, task := range params.Tasks {
			wg.Add(1)
			atomic.AddInt32(&m.activeSubAgents, 1)

			go func(idx int, agentSlug, taskDesc string) {
				defer wg.Done()
				defer atomic.AddInt32(&m.activeSubAgents, -1)

				agentInfo := agentMap[agentSlug]
				subAgentID := uuid.New().String()
				startedAt := time.Now().UTC()

				// Record sub-agent task in DB
				m.db.Exec(
					`INSERT INTO subagent_tasks (id, thread_id, parent_agent_slug, agent_slug, task_description, status, started_at, created_at)
					 VALUES (?, ?, ?, ?, ?, 'running', ?, ?)`,
					subAgentID, threadID, parentSlug, agentSlug, taskDesc, startedAt, startedAt,
				)

				// Broadcast sub-agent started
				m.broadcast("subagent_status", map[string]interface{}{
					"thread_id":    threadID,
					"parent_slug":  parentSlug,
					"subagent_id":  subAgentID,
					"agent_slug":   agentSlug,
					"agent_name":   agentInfo.Name,
					"status":       "started",
					"task_summary": truncateStr(taskDesc, 100, true),
				})

				result, err := m.roleChatSubAgent(ctx, agentSlug, taskDesc, threadID, parentSlug, subAgentID)

				completedAt := time.Now().UTC()
				taskResult := delegateTaskResult{
					AgentSlug: agentSlug,
					AgentName: agentInfo.Name,
					Task:      taskDesc,
				}

				if err != nil {
					taskResult.Status = "failed"
					taskResult.Error = err.Error()
					m.db.Exec(
						"UPDATE subagent_tasks SET status = 'failed', error = ?, completed_at = ? WHERE id = ?",
						err.Error(), completedAt, subAgentID,
					)
				} else {
					taskResult.Status = "completed"
					taskResult.Result = result.text
					taskResult.CostUSD = result.costUSD
					m.db.Exec(
						"UPDATE subagent_tasks SET status = 'completed', result_text = ?, cost_usd = ?, input_tokens = ?, output_tokens = ?, completed_at = ? WHERE id = ?",
						result.text, result.costUSD, result.inputTokens, result.outputTokens, completedAt, subAgentID,
					)
				}

				results[idx] = taskResult

				// Broadcast sub-agent completed/failed
				preview := taskResult.Result
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				m.broadcast("subagent_status", map[string]interface{}{
					"thread_id":      threadID,
					"parent_slug":    parentSlug,
					"subagent_id":    subAgentID,
					"agent_slug":     agentSlug,
					"agent_name":     agentInfo.Name,
					"status":         taskResult.Status,
					"task_summary":   truncateStr(taskDesc, 100, true),
					"result_preview": preview,
					"cost_usd":       taskResult.CostUSD,
				})
			}(i, task.AgentSlug, task.Task)
		}

		wg.Wait()

		// Build combined result for parent agent
		outputJSON, _ := json.Marshal(results)

		m.broadcast("subagent_status", map[string]interface{}{
			"thread_id":   threadID,
			"parent_slug": parentSlug,
			"status":      "delegation_completed",
			"task_count":  len(params.Tasks),
		})

		logger.Info("Delegation from %s completed: %d tasks", parentSlug, len(params.Tasks))
		return llm.ToolResult{Output: string(outputJSON)}
	}
}

type subAgentResult struct {
	text         string
	costUSD      float64
	inputTokens  int64
	outputTokens int64
}

// roleChatSubAgent runs a constrained agent loop for a delegated sub-task.
func (m *Manager) roleChatSubAgent(ctx context.Context, agentSlug, task, threadID, parentSlug, subAgentID string) (*subAgentResult, error) {
	// Look up agent role
	var systemPrompt, model string
	var identityInitialized bool
	err := m.db.QueryRow(
		"SELECT system_prompt, model, identity_initialized FROM agent_roles WHERE slug = ? AND enabled = 1",
		agentSlug,
	).Scan(&systemPrompt, &model, &identityInitialized)
	if err != nil {
		return nil, fmt.Errorf("agent role %q not found or disabled: %w", agentSlug, err)
	}

	// If identity system is initialized, use assembled prompt
	if identityInitialized {
		assembled, err := AssembleSystemPrompt(m.DataDir, agentSlug)
		if err == nil {
			systemPrompt = assembled
		}
	}

	// Add delegation preamble
	systemPrompt += "\n\n## DELEGATED TASK MODE\nYou have been delegated a specific task by another agent. Complete it concisely and return your findings. Focus only on the task below — do not ask follow-up questions."

	// Add current time
	systemPrompt += fmt.Sprintf("\n\nCurrent time: %s", time.Now().Format("Monday, January 2, 2006 at 3:04 PM MST"))

	// Sub-agent timeout
	subCtx, cancel := context.WithTimeout(ctx, time.Duration(subAgentTimeoutMin)*time.Minute)
	defer cancel()

	cfg := llm.AgentConfig{
		Model:    llm.ResolveModel(model, llm.ModelSonnet),
		System:   systemPrompt,
		MaxTurns: subAgentMaxTurns,
		OnEvent: func(ev StreamEvent) {
			// Broadcast sub-agent streaming events
			if ev.Type == EventTextDelta && ev.Text != "" {
				m.broadcast("subagent_stream", map[string]interface{}{
					"thread_id":   threadID,
					"parent_slug": parentSlug,
					"subagent_id": subAgentID,
					"agent_slug":  agentSlug,
					"text":        ev.Text,
				})
			}
		},
	}

	// Sub-agents get call_tool (for HTTP tools) but NOT delegate_task (no recursion)
	if m.ToolMgr != nil {
		toolsSection := m.buildToolsPromptSection(agentSlug)
		if toolsSection != "" {
			cfg.System += "\n\n---\n\n" + toolsSection
			cfg.ExtraTools = append(cfg.ExtraTools, llm.BuildCallToolDef())
			cfg.ExtraHandlers = map[string]llm.ToolHandler{
				"call_tool": m.makeCallToolHandler(),
			}
		}
	}

	result, err := m.client.RunAgentLoop(subCtx, cfg, task)
	if err != nil {
		return nil, fmt.Errorf("sub-agent %s failed: %w", agentSlug, err)
	}

	return &subAgentResult{
		text:         strings.TrimSpace(result.Text),
		costUSD:      result.TotalCostUSD,
		inputTokens:  result.InputTokens,
		outputTokens: result.OutputTokens,
	}, nil
}

// truncateStr truncates a string to maxLen, optionally adding an ellipsis.
func truncateStr(s string, maxLen int, ellipsis bool) string {
	if len(s) <= maxLen {
		return s
	}
	if ellipsis && maxLen > 3 {
		return s[:maxLen-3] + "..."
	}
	return s[:maxLen]
}
