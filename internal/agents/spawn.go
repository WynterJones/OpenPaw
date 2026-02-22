package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/models"
)

// spawnConfig holds the variable parts of a builder spawn.
type spawnConfig struct {
	agentType      string   // e.g. "tool_builder", "dashboard_builder"
	workDir        string   // working directory (empty for dashboard_builder)
	prompt         string   // fully constructed prompt
	tools          []string // LLM tools (nil for text-only builders)
	maxTurns       int      // max agent loop turns
	logToolCalls   bool     // whether to log tool start events
	suppressStream bool     // don't broadcast text deltas or save raw agent text (for JSON-only builders)
	postBuild      func(output string) string
	failureMsg     string // message prefix on failure
}

// spawnBuilder contains the common logic shared by all Spawn*Builder methods.
func (m *Manager) spawnBuilder(ctx context.Context, cfg spawnConfig, workOrder *models.WorkOrder, threadID string, placeholderMsgID string) (*models.Agent, error) {
	m.mu.RLock()
	activeCount := len(m.agents)
	m.mu.RUnlock()

	if activeCount >= maxConcurrentAgents {
		return nil, fmt.Errorf("max concurrent agents (%d) reached", maxConcurrentAgents)
	}

	agentID := generateAgentID()
	now := time.Now().UTC()
	agent := models.Agent{
		ID:          agentID,
		Type:        cfg.agentType,
		Status:      "running",
		Model:       m.BuilderModel,
		WorkOrderID: workOrder.ID,
		WorkingDir:  cfg.workDir,
		StartedAt:   &now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Insert agent record — include working_dir only if set
	if cfg.workDir != "" {
		_, err := m.db.Exec(
			`INSERT INTO agents (id, type, status, model, work_order_id, working_dir, started_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			agent.ID, agent.Type, agent.Status, agent.Model, agent.WorkOrderID, agent.WorkingDir,
			agent.StartedAt, agent.CreatedAt, agent.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert agent record: %w", err)
		}
	} else {
		_, err := m.db.Exec(
			`INSERT INTO agents (id, type, status, model, work_order_id, started_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			agent.ID, agent.Type, agent.Status, agent.Model, agent.WorkOrderID,
			agent.StartedAt, agent.CreatedAt, agent.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert agent record: %w", err)
		}
	}

	UpdateWorkOrderAgent(m.db, workOrder.ID, agentID)
	UpdateWorkOrderStatus(m.db, workOrder.ID, WorkOrderInProgress, "")

	agentCtx, cancel := context.WithTimeout(ctx, m.AgentTimeout())
	doneCh := make(chan struct{})
	var outputBuf strings.Builder

	ra := &runningAgent{
		agent:  agent,
		cancel: cancel,
		doneCh: doneCh,
	}

	m.mu.Lock()
	m.agents[agentID] = ra
	m.mu.Unlock()

	go func() {
		defer close(doneCh)
		defer cancel()

		agentCfg := llm.AgentConfig{
			Model:    llm.ResolveModel(m.BuilderModel, llm.ModelSonnet),
			Tools:    cfg.tools,
			WorkDir:  cfg.workDir,
			MaxTurns: cfg.maxTurns,
			OnEvent: func(event StreamEvent) {
				switch event.Type {
				case EventTextDelta:
					outputBuf.WriteString(event.Text)
				case EventToolStart:
					if cfg.logToolCalls {
						outputBuf.WriteString(fmt.Sprintf("\n[tool: %s]\n", event.ToolName))
						m.db.LogAudit("system", "builder_tool_call", "tool_call", "work_order", workOrder.ID, event.ToolName)
					}
				}

				// Don't broadcast text deltas for JSON-only builders (e.g. dashboard_builder)
				if cfg.suppressStream && event.Type == EventTextDelta {
					return
				}

				m.broadcast("agent_stream", map[string]interface{}{
					"agent_id":      agentID,
					"work_order_id": workOrder.ID,
					"thread_id":     threadID,
					"event":         event,
				})
			},
		}

		result, err := m.client.RunAgentLoop(agentCtx, agentCfg, cfg.prompt)

		output := outputBuf.String()
		status := "completed"
		errMsg := ""
		if err != nil {
			status = "failed"
			errMsg = err.Error()
		}

		m.updateAgentStatus(agentID, status, output, errMsg)

		if status == "completed" {
			m.db.LogAudit("system", "agent_completed", "agent", "work_order", workOrder.ID, cfg.agentType+" "+agentID)
		} else {
			m.db.LogAudit("system", "agent_failed", "agent", "work_order", workOrder.ID, cfg.agentType+" "+agentID+": "+errMsg)
		}

		woStatus := WorkOrderCompleted
		if status == "failed" {
			woStatus = WorkOrderFailed
		}
		UpdateWorkOrderStatus(m.db, workOrder.ID, woStatus, output)

		var chatMsg string
		var msgCost float64
		var msgInput, msgOutput int

		if status == "completed" {
			chatMsg = cfg.postBuild(output)
		} else {
			chatMsg = cfg.failureMsg
			if errMsg != "" {
				chatMsg += "\n\n**Error:** " + errMsg
			}
		}

		if result != nil {
			msgCost = result.TotalCostUSD
			msgInput = int(result.InputTokens)
			msgOutput = int(result.OutputTokens)
		}

		agentText := ""
		if result != nil && !cfg.suppressStream {
			agentText = strings.TrimSpace(result.Text)
		}
		m.saveBuildResult(threadID, agentText, chatMsg, "builder", msgCost, msgInput, msgOutput, placeholderMsgID)

		m.broadcast("agent_completed", map[string]interface{}{
			"agent_id":      agentID,
			"work_order_id": workOrder.ID,
			"thread_id":     threadID,
			"status":        status,
			"output":        output,
		})

		m.mu.Lock()
		delete(m.agents, agentID)
		m.mu.Unlock()

		if status == "completed" {
			logger.Success("Agent %s completed", agentID)
		} else {
			logger.Error("Agent %s failed: %s", agentID, errMsg)
		}
	}()

	m.db.LogAudit("system", "agent_spawned", "agent", "work_order", workOrder.ID, cfg.agentType+" "+agentID)
	logger.Info("Spawned %s agent %s for work order %s", cfg.agentType, agentID, workOrder.ID)
	return &agent, nil
}

func (m *Manager) SpawnToolBuilder(ctx context.Context, workOrder *models.WorkOrder, threadID, userID string, placeholderMsgID ...string) (*models.Agent, error) {
	toolDir := workOrder.TargetDir
	if toolDir == "" {
		toolDir = filepath.Join(m.toolsDir, workOrder.ToolID)
	}
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		return nil, fmt.Errorf("create tool directory: %w", err)
	}

	// Only scaffold for new builds — updates work with existing code
	if workOrder.Type != string(WorkOrderToolUpdate) {
		templateData := NewTemplateData(workOrder.ToolID, workOrder.Title, workOrder.Description)
		if err := ScaffoldToolDir(toolDir, templateData); err != nil {
			return nil, fmt.Errorf("scaffold tool directory: %w", err)
		}
	}

	var prompt string
	if workOrder.Type == string(WorkOrderToolUpdate) {
		prompt = fmt.Sprintf(ToolUpdaterPrompt, toolDir, workOrder.Description, workOrder.Requirements)
	} else {
		prompt = fmt.Sprintf(ToolBuilderPrompt, toolDir, workOrder.Description, workOrder.Requirements)
	}

	pID := ""
	if len(placeholderMsgID) > 0 {
		pID = placeholderMsgID[0]
	}

	return m.spawnBuilder(ctx, spawnConfig{
		agentType:    "tool_builder",
		workDir:      toolDir,
		prompt:       prompt,
		tools:        BuilderTools,
		maxTurns:     50,
		logToolCalls: true,
		postBuild: func(output string) string {
			return m.postBuildLifecycle(workOrder.ToolID, toolDir, workOrder.ID, output)
		},
		failureMsg: "Build failed. The agent encountered an error and could not finish.",
	}, workOrder, threadID, pID)
}

func (m *Manager) SpawnDashboardBuilder(ctx context.Context, workOrder *models.WorkOrder, threadID, userID string, placeholderMsgID ...string) (*models.Agent, error) {
	// Build available tools section for the prompt
	toolsSection := ""
	if m.ToolMgr != nil {
		ts := m.buildToolsPromptSection("")
		if ts != "" {
			toolsSection = "## Available Tools\n\n" + ts + "\nUse the tool IDs and endpoints above in your widget dataSource configurations.\n"
		}
	}

	prompt := fmt.Sprintf(DashboardBuilderPrompt, workOrder.Description, workOrder.Requirements, toolsSection)

	pID := ""
	if len(placeholderMsgID) > 0 {
		pID = placeholderMsgID[0]
	}

	return m.spawnBuilder(ctx, spawnConfig{
		agentType:      "dashboard_builder",
		workDir:        "",
		prompt:         prompt,
		tools:          nil,
		maxTurns:       1,
		logToolCalls:   false,
		suppressStream: true,
		postBuild: func(output string) string {
			return m.postBuildDashboard(workOrder, output)
		},
		failureMsg: "Dashboard build failed.",
	}, workOrder, threadID, pID)
}

func (m *Manager) SpawnCustomDashboardBuilder(ctx context.Context, workOrder *models.WorkOrder, threadID, userID string, placeholderMsgID ...string) (*models.Agent, error) {
	dashboardDir := workOrder.TargetDir
	if err := os.MkdirAll(dashboardDir, 0755); err != nil {
		return nil, fmt.Errorf("create dashboard directory: %w", err)
	}

	// Only scaffold for new builds — updates work with existing code
	if workOrder.Type != string(WorkOrderDashboardCustomUpdate) {
		templateData := DashboardTemplateData{
			DashboardID: workOrder.ToolID,
			Name:        workOrder.Title,
			Description: workOrder.Description,
		}
		if err := ScaffoldDashboardDir(dashboardDir, templateData); err != nil {
			return nil, fmt.Errorf("scaffold dashboard directory: %w", err)
		}
	}

	// Build available tools section for the prompt
	toolsSection := ""
	if m.ToolMgr != nil {
		ts := m.buildToolsPromptSection("")
		if ts != "" {
			toolsSection = "## Available Tools\n\n" + ts + "\nUse the tool IDs and endpoints above with OpenPaw.callTool(toolId, endpoint).\n"
		}
	}

	isUpdate := workOrder.Type == string(WorkOrderDashboardCustomUpdate)
	var prompt string
	if isUpdate {
		prompt = fmt.Sprintf(CustomDashboardUpdaterPrompt, dashboardDir, workOrder.Description, workOrder.Requirements, toolsSection)
	} else {
		prompt = fmt.Sprintf(CustomDashboardBuilderPrompt, dashboardDir, workOrder.Description, workOrder.Requirements, toolsSection)
	}

	pID := ""
	if len(placeholderMsgID) > 0 {
		pID = placeholderMsgID[0]
	}

	return m.spawnBuilder(ctx, spawnConfig{
		agentType:    "custom_dashboard_builder",
		workDir:      dashboardDir,
		prompt:       prompt,
		tools:        BuilderTools,
		maxTurns:     50,
		logToolCalls: true,
		postBuild: func(_ string) string {
			return m.postBuildCustomDashboard(workOrder, dashboardDir)
		},
		failureMsg: "Custom dashboard build failed.",
	}, workOrder, threadID, pID)
}
