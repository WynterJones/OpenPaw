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
	sb.WriteString("### CRITICAL: Workspace Restrictions & External File Operations\n\n")
	sb.WriteString("You are **locked to your own workspace directory**. You CANNOT read, write, create, or execute files in ANY other directory on the computer. ")
	sb.WriteString("Your Read, Write, Edit, and Bash tools are sandboxed — they will **fail silently or error** if you try to use them outside your workspace.\n\n")
	sb.WriteString("**When a user asks you to create files, build a project, generate code, scaffold a website, or do ANY file operation in an external directory, you MUST use a coding CLI tool via `call_tool`.** ")
	sb.WriteString("Do NOT say \"I'll create that for you\" and then attempt to use Write/Edit — it will not work. Do NOT tell the user you cannot do it — you CAN, by delegating to a coding tool.\n\n")
	sb.WriteString("#### How to handle external file/project requests:\n\n")
	sb.WriteString("1. **Identify which coding CLI tools are available** (look for Claude Code, Codex, or Gemini CLI in your tools list above)\n")
	sb.WriteString("2. **Use `call_tool`** with the `/implement` endpoint for creating/modifying files, or `/plan` for read-only analysis\n")
	sb.WriteString("3. **Pass the user's target directory** as the `directory` field and a detailed prompt as the `prompt` field\n\n")
	sb.WriteString("#### Examples:\n\n")
	sb.WriteString("User: \"Create a React website in /Users/me/projects/my-site\"\n")
	sb.WriteString("You: Use `call_tool` → tool_id: \"claude-code\" (or codex/gemini-cli), endpoint: \"/implement\", payload: {\"directory\": \"/Users/me/projects/my-site\", \"prompt\": \"Create a React website with...\"}\n\n")
	sb.WriteString("User: \"What's in /Users/me/projects/app?\"\n")
	sb.WriteString("You: Use `call_tool` → tool_id: \"claude-code\", endpoint: \"/plan\", payload: {\"directory\": \"/Users/me/projects/app\", \"prompt\": \"Analyze this project and describe its structure\"}\n\n")
	sb.WriteString("User: \"Fix the bug in /Users/me/code/server.js\"\n")
	sb.WriteString("You: Use `call_tool` → tool_id: \"claude-code\", endpoint: \"/implement\", payload: {\"directory\": \"/Users/me/code\", \"prompt\": \"Fix the bug in server.js...\"}\n\n")
	sb.WriteString("**NEVER attempt direct file access outside your workspace. ALWAYS delegate to a coding CLI tool.**\n\n")

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

// buildProjectsPromptSection queries the DB for user projects and their repos,
// resolving each repo's preferred coding CLI tool to a tool UUID.
func (m *Manager) buildProjectsPromptSection() string {
	rows, err := m.db.Query(
		`SELECT p.id, p.name, pr.name, pr.folder_path, pr.command
		 FROM projects p
		 JOIN project_repos pr ON pr.project_id = p.id
		 ORDER BY p.name, pr.sort_order`,
	)
	if err != nil {
		return ""
	}
	defer rows.Close()

	// Map command presets to library slugs
	commandToSlug := map[string]string{
		"claude": "claude-code",
		"codex":  "codex",
		"gemini": "gemini-cli",
	}

	// Cache tool UUID lookups by library_slug
	slugToToolID := map[string]string{}
	resolveToolID := func(librarySlug string) string {
		if tid, ok := slugToToolID[librarySlug]; ok {
			return tid
		}
		var tid string
		m.db.QueryRow(
			"SELECT id FROM tools WHERE library_slug = ? AND enabled = 1 AND deleted_at IS NULL LIMIT 1",
			librarySlug,
		).Scan(&tid)
		slugToToolID[librarySlug] = tid
		return tid
	}

	// Fallback: first available coding CLI tool
	fallbackToolID := func() string {
		for _, slug := range []string{"claude-code", "codex", "gemini-cli"} {
			if tid := resolveToolID(slug); tid != "" {
				return tid
			}
		}
		return ""
	}

	type repoEntry struct {
		repoName, folderPath, toolID string
	}
	type projectEntry struct {
		id, name string
		repos    []repoEntry
	}
	projectMap := map[string]*projectEntry{}
	var projectOrder []string

	for rows.Next() {
		var projID, projName, repoName, folderPath, command string
		if err := rows.Scan(&projID, &projName, &repoName, &folderPath, &command); err != nil {
			continue
		}
		p, ok := projectMap[projID]
		if !ok {
			p = &projectEntry{id: projID, name: projName}
			projectMap[projID] = p
			projectOrder = append(projectOrder, projID)
		}

		var toolID string
		if slug, ok := commandToSlug[command]; ok {
			toolID = resolveToolID(slug)
		}
		if toolID == "" {
			toolID = fallbackToolID()
		}

		p.repos = append(p.repos, repoEntry{
			repoName:   repoName,
			folderPath: folderPath,
			toolID:     toolID,
		})
	}

	if len(projectOrder) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## USER PROJECTS\n\n")
	sb.WriteString("When a user references a project by name, include \"project_context\" in your response.\n\n")

	for _, projID := range projectOrder {
		p := projectMap[projID]
		sb.WriteString(fmt.Sprintf("- **%s** (ID: `%s`)\n", p.name, p.id))
		for _, r := range p.repos {
			toolRef := "none"
			if r.toolID != "" {
				toolRef = fmt.Sprintf("`%s`", r.toolID)
			}
			sb.WriteString(fmt.Sprintf("  - \"%s\" → %s (tool: %s)\n", r.repoName, r.folderPath, toolRef))
		}
	}

	return sb.String()
}

// buildDashboardsPromptSection queries the DB for existing dashboards and builds a prompt section
// so the gateway knows about them and can include dashboard_id when updating.
func (m *Manager) buildDashboardsPromptSection() string {
	rows, err := m.db.Query(
		"SELECT id, name, description, dashboard_type FROM dashboards ORDER BY updated_at DESC LIMIT 20",
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
