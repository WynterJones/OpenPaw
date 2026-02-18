package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/models"
)

func (m *Manager) postBuildLifecycle(toolID, toolDir, workOrderID, builderOutput string) string {
	// 1. Read CAPABILITIES.md if it exists
	capPath := filepath.Join(toolDir, "CAPABILITIES.md")
	if capData, err := os.ReadFile(capPath); err == nil {
		now := time.Now().UTC()
		m.db.Exec("UPDATE tools SET capabilities = ?, updated_at = ? WHERE id = ?",
			string(capData), now, toolID)
	}

	// 2. Read manifest.json for endpoint info
	type manifestEndpoint struct {
		Method      string `json:"method"`
		Path        string `json:"path"`
		Description string `json:"description"`
	}
	var endpointList []manifestEndpoint
	manifestPath := filepath.Join(toolDir, "manifest.json")
	if manifestData, err := os.ReadFile(manifestPath); err == nil {
		var manifest map[string]json.RawMessage
		if json.Unmarshal(manifestData, &manifest) == nil {
			if eps, ok := manifest["endpoints"]; ok {
				json.Unmarshal(eps, &endpointList)
			}
		}
	}

	// 3. Compile
	if m.ToolMgr == nil {
		// No tool manager available — just summarize
		return m.fallbackSummary(workOrderID, builderOutput)
	}

	if err := m.ToolMgr.CompileTool(toolID); err != nil {
		now := time.Now().UTC()
		m.db.Exec("UPDATE tools SET status = 'error', updated_at = ? WHERE id = ?", now, toolID)
		return fmt.Sprintf("Build completed but **compilation failed**: %s", err.Error())
	}

	// 4. Start the tool
	if err := m.ToolMgr.StartTool(toolID); err != nil {
		now := time.Now().UTC()
		m.db.Exec("UPDATE tools SET status = 'error', updated_at = ? WHERE id = ?", now, toolID)
		return fmt.Sprintf("Build and compile succeeded but **failed to start**: %s", err.Error())
	}

	// 5. Wait for health (10s timeout)
	healthy := true
	if err := m.ToolMgr.WaitForHealth(toolID, 10*time.Second); err != nil {
		logger.Warn("Tool %s health check failed after build: %v", toolID, err)
		healthy = false
	}

	// 6. Update tool status and enable
	now := time.Now().UTC()
	if healthy {
		m.db.Exec("UPDATE tools SET status = 'running', enabled = 1, updated_at = ? WHERE id = ?", now, toolID)
	} else {
		m.db.Exec("UPDATE tools SET status = 'running', updated_at = ? WHERE id = ?", now, toolID)
	}

	// 7. Look up tool name
	var toolName string
	m.db.QueryRow("SELECT name FROM tools WHERE id = ?", toolID).Scan(&toolName)
	if toolName == "" {
		toolName = toolID
	}

	// 8. Build tool summary card JSON
	runtimeStatus := m.ToolMgr.GetStatus(toolID)
	port, _ := runtimeStatus["port"].(int)

	type cardEndpoint struct {
		Method      string `json:"method"`
		Path        string `json:"path"`
		Description string `json:"description"`
	}
	var cardEndpoints []cardEndpoint
	for _, ep := range endpointList {
		cardEndpoints = append(cardEndpoints, cardEndpoint{
			Method:      ep.Method,
			Path:        ep.Path,
			Description: ep.Description,
		})
	}

	cardData := map[string]interface{}{
		"__type":    "tool_summary",
		"tool_id":   toolID,
		"tool_name": toolName,
		"port":      port,
		"status":    "running",
		"healthy":   healthy,
		"endpoints": cardEndpoints,
	}
	cardJSON, _ := json.Marshal(cardData)

	return string(cardJSON)
}

// postBuildCustomDashboard verifies the dashboard files and saves/updates the DB record.
func (m *Manager) postBuildCustomDashboard(workOrder *models.WorkOrder, dashboardDir string) string {
	// Verify index.html exists
	indexPath := filepath.Join(dashboardDir, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		return "Custom dashboard builder did not produce an index.html file."
	}

	dashboardID := workOrder.ToolID // We store dashboardID in ToolID field
	now := time.Now().UTC()

	// Check if updating an existing dashboard
	var existingID string
	m.db.QueryRow("SELECT id FROM dashboards WHERE id = ?", dashboardID).Scan(&existingID)

	action := "created"
	if existingID != "" {
		// Update existing
		m.db.Exec(
			"UPDATE dashboards SET description = ?, dashboard_type = 'custom', updated_at = ? WHERE id = ?",
			workOrder.Description, now, existingID,
		)
		action = "updated"
	} else {
		// Insert new
		m.db.Exec(
			"INSERT INTO dashboards (id, name, description, layout, widgets, dashboard_type, owner_agent_slug, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			dashboardID, workOrder.Title, workOrder.Description, "{}", "[]", "custom", "builder", now, now,
		)
	}

	m.db.LogAudit("system", "dashboard_"+action, "dashboard", "dashboard", dashboardID, workOrder.Title)
	return fmt.Sprintf("Custom dashboard **%s** %s. View it on the Dashboards page.", workOrder.Title, action)
}

// postBuildDashboard parses the builder output JSON and saves the dashboard to DB.
func (m *Manager) postBuildDashboard(workOrder *models.WorkOrder, builderOutput string) string {
	// Extract JSON from output (may be wrapped in ```json fences)
	jsonStr := extractJSON(builderOutput)
	if jsonStr == "" {
		return "Dashboard builder did not produce valid JSON output."
	}

	var config struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Layout      json.RawMessage `json:"layout"`
		Widgets     json.RawMessage `json:"widgets"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &config); err != nil {
		return "Failed to parse dashboard configuration: " + err.Error()
	}
	if config.Name == "" {
		config.Name = workOrder.Title
	}

	layoutStr := "{}"
	if len(config.Layout) > 0 {
		layoutStr = string(config.Layout)
	}
	widgetsStr := "[]"
	if len(config.Widgets) > 0 {
		widgetsStr = string(config.Widgets)
	}

	// Check if updating an existing dashboard
	var existingID string
	m.db.QueryRow(
		"SELECT id FROM dashboards WHERE name = ?", config.Name,
	).Scan(&existingID)

	now := time.Now().UTC()
	dashboardID := existingID
	action := "created"

	if existingID != "" {
		// Update existing
		m.db.Exec(
			"UPDATE dashboards SET description = ?, layout = ?, widgets = ?, updated_at = ? WHERE id = ?",
			config.Description, layoutStr, widgetsStr, now, existingID,
		)
		action = "updated"
	} else {
		// Insert new
		dashboardID = uuid.New().String()
		m.db.Exec(
			"INSERT INTO dashboards (id, name, description, layout, widgets, owner_agent_slug, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			dashboardID, config.Name, config.Description, layoutStr, widgetsStr, "builder", now, now,
		)
	}

	// Count widgets
	var widgets []json.RawMessage
	json.Unmarshal([]byte(widgetsStr), &widgets)
	widgetCount := len(widgets)

	// Create schedules for widgets with refreshInterval
	m.createDashboardSchedules(dashboardID, widgetsStr)

	m.db.LogAudit("system", "dashboard_"+action, "dashboard", "dashboard", dashboardID, config.Name)

	return fmt.Sprintf("Dashboard **%s** %s with %d widgets.", config.Name, action, widgetCount)
}

// createDashboardSchedules creates cron schedules for widgets with refreshInterval > 0.
func (m *Manager) createDashboardSchedules(dashboardID, widgetsJSON string) {
	// Clean up old schedules for this dashboard
	m.db.Exec("DELETE FROM schedules WHERE dashboard_id = ?", dashboardID)

	var widgets []struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		DataSource *struct {
			Type            string `json:"type"`
			ToolID          string `json:"toolId"`
			Endpoint        string `json:"endpoint"`
			RefreshInterval int    `json:"refreshInterval"`
		} `json:"dataSource"`
	}
	if err := json.Unmarshal([]byte(widgetsJSON), &widgets); err != nil {
		return
	}

	for _, w := range widgets {
		if w.DataSource == nil || w.DataSource.Type != "tool" || w.DataSource.RefreshInterval <= 0 {
			continue
		}

		cronExpr := intervalToCron(w.DataSource.RefreshInterval)
		schedID := uuid.New().String()
		now := time.Now().UTC()

		m.db.Exec(
			`INSERT INTO schedules (id, name, description, cron_expr, tool_id, action, payload, enabled,
			                        type, agent_role_slug, prompt_content, dashboard_id, widget_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			schedID,
			fmt.Sprintf("Dashboard: %s", w.Title),
			fmt.Sprintf("Auto-collect data for widget %s", w.ID),
			cronExpr,
			w.DataSource.ToolID,
			strings.TrimPrefix(w.DataSource.Endpoint, "/"),
			"{}",
			true,
			"tool_action",
			"",
			"",
			dashboardID,
			w.ID,
			now,
			now,
		)
	}
}

// intervalToCron converts a refresh interval in seconds to a cron expression.
func intervalToCron(seconds int) string {
	if seconds <= 0 {
		return "0 */5 * * * *" // default 5 min
	}
	minutes := seconds / 60
	if minutes <= 0 {
		minutes = 1
	}
	if minutes < 60 {
		return fmt.Sprintf("0 */%d * * * *", minutes)
	}
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("0 0 */%d * * *", hours)
	}
	return "0 0 0 * * *" // daily
}

// extractJSON finds and returns the first JSON object in the text.
func extractJSON(text string) string {
	// Remove markdown json fences
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		if idx := strings.LastIndex(text, "```"); idx >= 0 {
			text = text[:idx]
		}
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		if idx := strings.LastIndex(text, "```"); idx >= 0 {
			text = text[:idx]
		}
		text = strings.TrimSpace(text)
	}

	// Find first { and last }
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		candidate := text[start : end+1]
		if json.Valid([]byte(candidate)) {
			return candidate
		}
	}
	return ""
}

func (m *Manager) fallbackSummary(workOrderID, builderOutput string) string {
	summaryCtx, summaryCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer summaryCancel()
	summary, err := m.GatewaySummarize(summaryCtx, workOrderID, builderOutput)
	if err != nil || summary == "" {
		return "Build complete!"
	}
	return summary
}

// saveBuildResult persists the agent's text output and the post-build result card
// as separate messages so the card doesn't replace the streamed text.
func (m *Manager) saveBuildResult(threadID, agentText, chatMsg, roleSlug string, msgCost float64, msgInput, msgOutput int, placeholderMsgID string) {
	if threadID == "" {
		return
	}
	msgNow := time.Now().UTC()

	if agentText != "" && chatMsg != agentText {
		// Agent produced text AND we have a separate result — save both
		if placeholderMsgID != "" {
			m.db.Exec(
				"UPDATE chat_messages SET content = ?, cost_usd = ?, input_tokens = ?, output_tokens = ?, created_at = ? WHERE id = ?",
				agentText, msgCost, msgInput, msgOutput, msgNow, placeholderMsgID,
			)
		} else {
			m.db.Exec(
				"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
				uuid.New().String(), threadID, "assistant", agentText, roleSlug, msgCost, msgInput, msgOutput, msgNow,
			)
		}
		cardNow := msgNow.Add(time.Millisecond)
		m.db.Exec(
			"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			uuid.New().String(), threadID, "assistant", chatMsg, roleSlug, 0, 0, 0, cardNow,
		)
		m.db.Exec("UPDATE chat_threads SET updated_at = ? WHERE id = ?", cardNow, threadID)
	} else {
		// No separate text — save chatMsg only (failure case or empty output)
		if placeholderMsgID != "" {
			m.db.Exec(
				"UPDATE chat_messages SET content = ?, cost_usd = ?, input_tokens = ?, output_tokens = ?, created_at = ? WHERE id = ?",
				chatMsg, msgCost, msgInput, msgOutput, msgNow, placeholderMsgID,
			)
		} else {
			m.db.Exec(
				"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
				uuid.New().String(), threadID, "assistant", chatMsg, roleSlug, msgCost, msgInput, msgOutput, msgNow,
			)
		}
		m.db.Exec("UPDATE chat_threads SET updated_at = ? WHERE id = ?", msgNow, threadID)
	}
}
