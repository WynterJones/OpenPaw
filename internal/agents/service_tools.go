package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	llm "github.com/openpaw/openpaw/internal/llm"
)

// serviceLogTailBytes is how much of a service's stdout/stderr to read back. A
// failed start puts its reason in the last few lines; more than this is noise
// in a chat reply.
const serviceLogTailBytes = 4000

// BuildServiceControlToolDefs returns the service_control tool: restart,
// recompile, start, stop and status for a built service.
//
// Without it an agent could call a service but never fix one — "it's returning
// 502" ended with the user being sent to the Services page to press restart
// themselves, which is the one thing the agent could most easily have done.
func BuildServiceControlToolDefs() []llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"service": map[string]interface{}{
				"type":        "string",
				"description": "The service's ID, or its exact name from the services list.",
			},
			"action": map[string]interface{}{
				"type": "string",
				"enum": []string{"status", "restart", "recompile", "start", "stop"},
				"description": "status: is it running and healthy. restart: stop and start the current binary. " +
					"recompile: rebuild from source then restart — use after the source changed, or when a restart alone did not fix it. " +
					"start / stop: one direction only.",
			},
		},
		"required": []string{"service", "action"},
	})
	return []llm.ToolDef{{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "service_control",
			Description: "Check on a service and get it running again — status, restart, recompile, start, stop. Use this when a service call fails, when a service looks stopped, or to confirm a service is healthy before telling the user it works.",
			Parameters:  params,
		},
	}}
}

// MakeServiceControlHandlers returns the service_control handler.
func (m *Manager) MakeServiceControlHandlers(workspaceID string) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"service_control": func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
			var params struct {
				Service string `json:"service"`
				Action  string `json:"action"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
			}
			if m.ToolMgr == nil {
				return llm.ToolResult{Output: "Services are not available in this context.", IsError: true}
			}

			toolID, name := m.resolveService(params.Service, workspaceID)
			if toolID == "" {
				return llm.ToolResult{
					Output:  fmt.Sprintf("No service named %q. Use the exact name or ID from the services list.", params.Service),
					IsError: true,
				}
			}

			switch strings.ToLower(strings.TrimSpace(params.Action)) {
			case "status":
				return llm.ToolResult{Output: m.serviceStatusLine(toolID, name)}

			case "stop":
				if err := m.ToolMgr.StopTool(toolID); err != nil {
					return llm.ToolResult{Output: fmt.Sprintf("%s was not running.", name)}
				}
				return llm.ToolResult{Output: fmt.Sprintf("Stopped %s.", name)}

			case "start":
				if err := m.ToolMgr.StartTool(toolID); err != nil {
					return llm.ToolResult{Output: fmt.Sprintf("Could not start %s: %s%s", name, err, m.serviceLogTail(toolID)), IsError: true}
				}
				return llm.ToolResult{Output: m.verifyService(toolID, name, "Started")}

			case "restart":
				if err := m.ToolMgr.RestartTool(toolID); err != nil {
					return llm.ToolResult{Output: fmt.Sprintf("Could not restart %s: %s%s", name, err, m.serviceLogTail(toolID)), IsError: true}
				}
				return llm.ToolResult{Output: m.verifyService(toolID, name, "Restarted")}

			case "recompile":
				// Stop first: the old process holds both the port the new one
				// needs and the binary the compiler is about to overwrite.
				_ = m.ToolMgr.StopTool(toolID)
				if err := m.ToolMgr.CompileTool(toolID); err != nil {
					return llm.ToolResult{Output: fmt.Sprintf("%s failed to compile: %s", name, err), IsError: true}
				}
				if err := m.ToolMgr.StartTool(toolID); err != nil {
					return llm.ToolResult{Output: fmt.Sprintf("%s compiled but would not start: %s%s", name, err, m.serviceLogTail(toolID)), IsError: true}
				}
				return llm.ToolResult{Output: m.verifyService(toolID, name, "Recompiled and restarted")}

			default:
				return llm.ToolResult{Output: "action must be status, restart, recompile, start or stop.", IsError: true}
			}
		},
	}
}

// verifyService waits for the health endpoint and reports what actually
// happened. Reporting "restarted" on a process that came up and immediately
// fell over is how a broken service gets described to the user as fixed.
func (m *Manager) verifyService(toolID, name, did string) string {
	if err := m.ToolMgr.WaitForHealth(toolID, 10*time.Second); err != nil {
		return fmt.Sprintf(
			"%s %s, but it is not answering its health check: %s\nIt is running, so this may be a bad endpoint or a crash on the first request.%s",
			did, name, err, m.serviceLogTail(toolID),
		)
	}
	return fmt.Sprintf("%s %s. %s", did, name, m.serviceStatusLine(toolID, name))
}

// serviceStatusLine describes a service's current runtime state.
func (m *Manager) serviceStatusLine(toolID, name string) string {
	status := m.ToolMgr.GetStatus(toolID)
	state, _ := status["status"].(string)
	if state == "" {
		state = "unknown"
	}
	port, _ := status["port"].(int)

	line := fmt.Sprintf("%s is %s", name, state)
	if port > 0 {
		line += fmt.Sprintf(" on port %d", port)
	}
	if state != "running" {
		return line + "." + m.serviceLogTail(toolID)
	}
	if err := m.ToolMgr.WaitForHealth(toolID, 3*time.Second); err != nil {
		return line + ", but not answering /health." + m.serviceLogTail(toolID)
	}
	return line + " and healthy."
}

// resolveService maps an ID or a name to (id, name).
func (m *Manager) resolveService(ref, workspaceID string) (string, string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", ""
	}
	var id, name string
	m.db.QueryRow(
		`SELECT id, name FROM tools
		 WHERE id = ? AND deleted_at IS NULL
		   AND (workspace_id IS NULL OR workspace_id = ?)`,
		ref, workspaceID,
	).Scan(&id, &name)
	if id != "" {
		return id, name
	}
	m.db.QueryRow(
		`SELECT id, name FROM tools
		 WHERE LOWER(name) = LOWER(?) AND deleted_at IS NULL
		   AND (workspace_id IS NULL OR workspace_id = ?)
		 LIMIT 1`,
		ref, workspaceID,
	).Scan(&id, &name)
	if id != "" {
		return id, name
	}
	m.db.QueryRow(
		`SELECT id, name FROM tools
		 WHERE LOWER(name) LIKE '%' || LOWER(?) || '%' AND deleted_at IS NULL
		   AND (workspace_id IS NULL OR workspace_id = ?)
		 LIMIT 1`,
		ref, workspaceID,
	).Scan(&id, &name)
	return id, name
}

// serviceLogTail returns the end of a service's log, formatted for a chat
// reply, or "" when there is nothing to show. A failed start is unactionable
// without it — "exit status 1" says nothing about the missing env var that
// caused it.
func (m *Manager) serviceLogTail(toolID string) string {
	path := filepath.Join(m.toolsDir, toolID, "tool.log")
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return ""
	}
	size := info.Size()
	offset := int64(0)
	if size > serviceLogTailBytes {
		offset = size - serviceLogTailBytes
	}
	buf := make([]byte, size-offset)
	if _, err := f.ReadAt(buf, offset); err != nil {
		return ""
	}

	out := strings.TrimSpace(string(buf))
	if offset > 0 {
		// Drop the partial first line the offset almost certainly landed inside.
		if i := strings.IndexByte(out, '\n'); i >= 0 {
			out = out[i+1:]
		}
	}
	if strings.TrimSpace(out) == "" {
		return ""
	}
	return "\n\nLast output from the service:\n```\n" + strings.TrimSpace(out) + "\n```"
}

// buildServiceControlPromptSection tells an agent that a stopped or unhealthy
// service is something it can fix rather than report.
func buildServiceControlPromptSection() string {
	return "## KEEPING SERVICES RUNNING\n" +
		"`service_control` lets you check a service and get it working again: `status`, `restart`, `recompile`, `start`, `stop`.\n\n" +
		"- A service call that fails, times out, or returns a 502 usually means the process is stopped or wedged. Check `status`, then `restart`. Do not report the failure to the user until you have tried.\n" +
		"- `recompile` rebuilds from source and restarts. Use it after a service's code changed, or when a plain restart didn't take.\n" +
		"- Before telling the user a service works, confirm it — `status` reports whether it is answering its health check, not just whether a process exists.\n" +
		"- If it still won't run, the tool hands back the service's own log output. Read it and say what is actually wrong (a missing secret, a port clash, a crash on boot) rather than \"it failed to start\"."
}
