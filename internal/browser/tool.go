package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llm "github.com/openpaw/openpaw/internal/llm"
)

func BuildBrowserActionDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "The browser session ID to use (from AVAILABLE BROWSER SESSIONS)",
			},
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The browser action to perform",
				"enum":        []string{"navigate", "click", "type", "screenshot", "extract_text", "wait_element", "scroll", "back", "forward", "eval", "key_press"},
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS selector for the target element (for click, type, extract_text, wait_element)",
			},
			"value": map[string]interface{}{
				"type":        "string",
				"description": "URL for navigate, text for type, JavaScript for eval, key name for key_press",
			},
			"x": map[string]interface{}{
				"type":        "number",
				"description": "X coordinate for click (alternative to selector), or horizontal scroll amount",
			},
			"y": map[string]interface{}{
				"type":        "number",
				"description": "Y coordinate for click (alternative to selector), or vertical scroll amount",
			},
		},
		"required": []string{"session_id", "action"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "browser_action",
			Description: "Control a web browser session. Navigate to pages, click elements, type text, and extract content. Prefer 'extract_text' over 'screenshot' to read page content — it uses far fewer tokens. Screenshots are streamed to the Browser tab automatically; only request one when the human needs to see the page.",
			Parameters:  params,
		},
	}
}

func (m *Manager) MakeBrowserActionHandler() llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var req ActionRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if req.SessionID == "" {
			return llm.ToolResult{Output: "session_id is required", IsError: true}
		}
		if req.Action == "" {
			return llm.ToolResult{Output: "action is required", IsError: true}
		}

		result := m.ExecuteAction(ctx, req)

		// Broadcast the full result (with screenshot) to the UI via WebSocket
		if result.Screenshot != "" {
			m.broadcast("browser_screenshot", map[string]interface{}{
				"session_id": req.SessionID,
				"image":      result.Screenshot,
				"url":        result.URL,
				"title":      result.Title,
			})
		}

		// Strip the base64 screenshot before returning to the LLM to save tokens.
		// The screenshot is already visible in the Browser tab via WebSocket streaming.
		llmResult := ActionResult{
			Success: result.Success,
			Data:    result.Data,
			URL:     result.URL,
			Title:   result.Title,
			Error:   result.Error,
		}
		if result.Screenshot != "" {
			llmResult.Data += " (screenshot sent to browser viewer)"
		}

		resultJSON, err := json.Marshal(llmResult)
		if err != nil {
			return llm.ToolResult{Output: "Failed to marshal result: " + err.Error(), IsError: true}
		}

		if !result.Success {
			return llm.ToolResult{Output: string(resultJSON), IsError: true}
		}
		return llm.ToolResult{Output: string(resultJSON)}
	}
}

func (m *Manager) BuildSessionsPromptSection(agentRoleSlug string) string {
	sessions := m.ListSessions()
	if len(sessions) == 0 {
		return ""
	}

	var filtered []*Session
	for _, s := range sessions {
		if s.OwnerAgentSlug == "" || s.OwnerAgentSlug == agentRoleSlug {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## AVAILABLE BROWSER SESSIONS\n\n")
	sb.WriteString("You can control web browsers using the `browser_action` tool.\n")
	sb.WriteString("Use these sessions for tasks that require interacting with websites.\n\n")

	sb.WriteString("### IMPORTANT: Login Workflow\n\n")
	sb.WriteString("If you encounter a login page:\n")
	sb.WriteString("1. Take a screenshot so the human can see the login page\n")
	sb.WriteString("2. Tell the human: \"I've encountered a login page. Please take control of the browser, log in, then tell me to continue.\"\n")
	sb.WriteString("3. Wait for the human to say \"continue\" before proceeding\n\n")

	for _, s := range filtered {
		status := string(s.Status)
		sb.WriteString(fmt.Sprintf("### %s\n", s.Name))
		sb.WriteString(fmt.Sprintf("- **Session ID**: `%s`\n", s.ID))
		sb.WriteString(fmt.Sprintf("- **Status**: %s\n", status))
		if s.CurrentURL != "" {
			sb.WriteString(fmt.Sprintf("- **Current URL**: %s\n", s.CurrentURL))
		}
		if s.CurrentTitle != "" {
			sb.WriteString(fmt.Sprintf("- **Current Title**: %s\n", s.CurrentTitle))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
