package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	llm "github.com/openpaw/openpaw/internal/llm"
)

// buildToolsPromptSection queries the DB for enabled tools and builds a system prompt section.
// If agentRoleSlug is provided, it filters to only tools granted to that agent (if grants exist).
func (m *Manager) buildToolsPromptSection(agentRoleSlug string) string {
	// Check if this agent has explicit tool grants
	var grantedToolIDs map[string]bool
	if agentRoleSlug != "" {
		grantRows, err := m.db.Query(
			"SELECT tool_id FROM agent_tool_access WHERE agent_role_slug = ?", agentRoleSlug,
		)
		if err == nil {
			defer grantRows.Close()
			grantedToolIDs = make(map[string]bool)
			for grantRows.Next() {
				var toolID string
				if err := grantRows.Scan(&toolID); err == nil {
					grantedToolIDs[toolID] = true
				}
			}
			// If no grants found, show all tools (permissive default)
			if len(grantedToolIDs) == 0 {
				grantedToolIDs = nil
			}
		}
	}

	rows, err := m.db.Query(
		"SELECT id, name, description, status, port FROM tools WHERE enabled = 1 AND deleted_at IS NULL",
	)
	if err != nil {
		return ""
	}
	defer rows.Close()

	type toolInfo struct {
		ID, Name, Description, Status string
		Port                          int
	}
	var tools []toolInfo
	for rows.Next() {
		var t toolInfo
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Status, &t.Port); err != nil {
			log.Printf("[warn] scan tool info row: %v", err)
			continue
		}
		// Filter by grants if the agent has explicit grants
		if grantedToolIDs != nil && !grantedToolIDs[t.ID] {
			continue
		}
		tools = append(tools, t)
	}
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## AVAILABLE TOOLS\n\n")
	sb.WriteString("You have custom tools installed. Use the `call_tool` tool to invoke them.\nTools must have status 'running' to be called. If a tool is not running, let the user know they need to start it from the Tools page.\n\n")
	sb.WriteString("### IMPORTANT: Tool Result Display\n\n")
	sb.WriteString("When a tool returns data, a **visual widget component** is automatically rendered below your message showing the full data (table, chart, key-value pairs, etc.).\n")
	sb.WriteString("**Do NOT repeat or reformat the tool data** in your text response (no markdown tables, no bullet-point lists of the same values).\n")
	sb.WriteString("Instead, provide a brief **conversational insight or commentary** about the data and reference the widget. For example:\n")
	sb.WriteString("- \"Pretty cold out there! Here's the current weather for your area:\" (widget renders below)\n")
	sb.WriteString("- \"Looks like sales are trending up this quarter — take a look:\" (widget renders below)\n")
	sb.WriteString("- \"All systems healthy. Full status:\" (widget renders below)\n\n")
	sb.WriteString("Keep your text response short and insightful — the widget handles the data display.\n\n")

	for _, t := range tools {
		// Check runtime status from tool manager for accuracy
		runtimeStatus := m.ToolMgr.GetStatus(t.ID)
		if rs, ok := runtimeStatus["status"].(string); ok && rs == "running" {
			t.Status = "running"
			if p, ok := runtimeStatus["port"].(int); ok && p > 0 {
				t.Port = p
			}
		}

		sb.WriteString(fmt.Sprintf("### %s\n", t.Name))
		sb.WriteString(fmt.Sprintf("- **ID**: `%s`\n", t.ID))
		sb.WriteString(fmt.Sprintf("- **Status**: %s\n", t.Status))
		if t.Description != "" {
			sb.WriteString(fmt.Sprintf("- **Description**: %s\n", t.Description))
		}

		// Read manifest for endpoint details (cached)
		manifestPath := filepath.Join(m.toolsDir, t.ID, "manifest.json")
		var data []byte
		if cached, ok := m.manifestCache.Load(t.ID); ok {
			data = cached.([]byte)
		} else if raw, err := os.ReadFile(manifestPath); err == nil {
			data = raw
			m.manifestCache.Store(t.ID, raw)
		}
		if data != nil {
			var manifest struct {
				Endpoints []struct {
					Method      string `json:"method"`
					Path        string `json:"path"`
					Description string `json:"description"`
					QueryParams []struct {
						Name        string `json:"name"`
						Type        string `json:"type"`
						Required    bool   `json:"required"`
						Description string `json:"description"`
					} `json:"query_params"`
				} `json:"endpoints"`
			}
			if json.Unmarshal(data, &manifest) == nil && len(manifest.Endpoints) > 0 {
				sb.WriteString("- **Endpoints**:\n")
				for _, ep := range manifest.Endpoints {
					if ep.Path == "/health" {
						continue
					}
					sb.WriteString(fmt.Sprintf("  - `%s %s` — %s\n", ep.Method, ep.Path, ep.Description))
					for _, qp := range ep.QueryParams {
						req := ""
						if qp.Required {
							req = ", **required**"
						}
						sb.WriteString(fmt.Sprintf("    - `%s` (%s%s) — %s\n", qp.Name, qp.Type, req, qp.Description))
					}
				}
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// buildDashboardsPromptSection queries the DB for existing dashboards and builds a prompt section
// so the gateway knows about them and can include dashboard_id when updating.
func (m *Manager) buildDashboardsPromptSection() string {
	rows, err := m.db.Query(
		"SELECT id, name, description, dashboard_type FROM dashboards WHERE deleted_at IS NULL ORDER BY updated_at DESC LIMIT 20",
	)
	if err != nil {
		return ""
	}
	defer rows.Close()

	type dashInfo struct {
		ID, Name, Description, DashboardType string
	}
	var dashboards []dashInfo
	for rows.Next() {
		var d dashInfo
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.DashboardType); err != nil {
			continue
		}
		dashboards = append(dashboards, d)
	}
	if len(dashboards) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## EXISTING DASHBOARDS\n\n")
	sb.WriteString("When updating an existing dashboard, include its ID as `dashboard_id` in the work_order.\n\n")
	for _, d := range dashboards {
		desc := d.Description
		if desc == "" {
			desc = "No description"
		}
		sb.WriteString(fmt.Sprintf("- **%s** (ID: `%s`, type: %s) — %s\n", d.Name, d.ID, d.DashboardType, desc))
	}

	return sb.String()
}

// makeCallToolHandler returns a tool handler closure that invokes custom tools via the ToolManager.
func (m *Manager) makeCallToolHandler() llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			ToolID   string `json:"tool_id"`
			Endpoint string `json:"endpoint"`
			Method   string `json:"method"`
			Payload  string `json:"payload"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.ToolID == "" || params.Endpoint == "" {
			return llm.ToolResult{Output: "tool_id and endpoint are required", IsError: true}
		}

		var payload []byte
		if params.Payload != "" {
			payload = []byte(params.Payload)
		}

		result, err := m.ToolMgr.CallTool(params.ToolID, params.Endpoint, payload)
		if err != nil {
			return llm.ToolResult{Output: "Tool call failed: " + err.Error(), IsError: true}
		}

		// Inject __tool_uuid so WidgetCollector can map to the real tool DB ID
		output := injectToolUUID(string(result), params.ToolID)
		return llm.ToolResult{Output: output}
	}
}

// injectToolUUID adds __tool_uuid to a JSON object response so the widget system
// can resolve the tool's DB UUID (distinct from the opaque API call ID).
func injectToolUUID(output, toolID string) string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return output
	}
	uuidJSON, _ := json.Marshal(toolID)
	raw["__tool_uuid"] = uuidJSON
	b, err := json.Marshal(raw)
	if err != nil {
		return output
	}
	return string(b)
}
