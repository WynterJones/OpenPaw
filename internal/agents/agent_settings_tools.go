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

// An agent's own settings, from inside a conversation.
//
// "Check in on this every half hour" is a thing users say to an agent, and the
// honest answer used to be a walk-through of the Agents page — open the agent,
// find the Heartbeat section, work out that the interval is in seconds, save.
// The agent knows what was asked; it should be the one to apply it.
//
// Deliberately scoped to the agent's OWN row. An agent editing its colleagues'
// configuration is a different and much worse idea: it turns one agent going
// wrong into a fleet going wrong, and nothing the user asked for needs it.
//
// Also deliberately excluded: `enabled` and `slug`. An agent that switches
// itself off mid-conversation cannot be switched back on by the conversation it
// was having, and a renamed slug orphans every schedule, thread membership and
// memory database keyed to the old one. Both are Agents-page decisions.
//
// The system prompt is excluded too. Identity-initialized agents already edit
// their own SOUL files through Read/Write in their agent directory, which is
// visible, versioned and reversible; a tool that silently rewrites an agent's
// instructions in the database is none of those things.

func BuildAgentSettingsToolDefs() []llm.ToolDef {
	getParams, _ := json.Marshal(map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	})

	updateParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"heartbeat_enabled": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether you wake up on your own to check in, without the user saying anything.",
			},
			"heartbeat_interval_sec": map[string]interface{}{
				"type":        "integer",
				"description": "Seconds between your check-ins. 0 means inherit the global interval. 1800 = every 30 minutes, 3600 = hourly.",
			},
			"heartbeat_max_turns": map[string]interface{}{
				"type":        "integer",
				"description": "How many tool-using turns one check-in may take. 0 inherits the global limit. Raise it if your check-ins do real work and keep running out.",
			},
			"heartbeat_timeout_sec": map[string]interface{}{
				"type":        "integer",
				"description": "Seconds one check-in may run before it is stopped. 0 inherits the global limit.",
			},
			"model": map[string]interface{}{
				"type":        "string",
				"description": "The model you run on. Only set this if the user names a model — do not guess an id.",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Your display name. Your slug is unaffected.",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "The one-line description of what you do, shown in the agent list and used for routing.",
			},
			"folder": map[string]interface{}{
				"type":        "string",
				"description": "The folder you are grouped under on the Agents page. Empty string removes the grouping.",
			},
		},
	})

	return []llm.ToolDef{
		{Type: "function", Function: llm.FunctionDef{
			Name: "my_settings_get",
			Description: "Read your own configuration: name, description, model, folder, and your heartbeat settings, " +
				"together with the global heartbeat settings yours are layered on top of. Read this before changing anything so you report accurate current values.",
			Parameters: getParams,
		}},
		{Type: "function", Function: llm.FunctionDef{
			Name: "my_settings_update",
			Description: "Change your own settings — most usefully your heartbeat (whether you check in on your own, and how often). " +
				"Only the fields you pass are changed. Confirm with the user before changing anything, and say what the value was before. " +
				"You cannot disable or rename yourself, or edit another agent; those are done on the Agents page.",
			Parameters: updateParams,
		}},
	}
}

func (m *Manager) MakeAgentSettingsToolHandlers(selfSlug string) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"my_settings_get":    m.handleMySettingsGet(selfSlug),
		"my_settings_update": m.handleMySettingsUpdate(selfSlug),
	}
}

type agentSettingsView struct {
	Slug                 string `json:"slug"`
	Name                 string `json:"name"`
	Description          string `json:"description"`
	Model                string `json:"model"`
	Folder               string `json:"folder,omitempty"`
	HeartbeatEnabled     bool   `json:"heartbeat_enabled"`
	HeartbeatIntervalSec int    `json:"heartbeat_interval_sec"`
	HeartbeatMaxTurns    int    `json:"heartbeat_max_turns"`
	HeartbeatTimeoutSec  int    `json:"heartbeat_timeout_sec"`
}

func (m *Manager) readAgentSettings(slug string) (agentSettingsView, error) {
	var v agentSettingsView
	err := m.db.QueryRow(
		`SELECT slug, name, description, model, folder, heartbeat_enabled,
		        heartbeat_interval_sec, heartbeat_max_turns, heartbeat_timeout_sec
		 FROM agent_roles WHERE slug = ?`, slug,
	).Scan(&v.Slug, &v.Name, &v.Description, &v.Model, &v.Folder, &v.HeartbeatEnabled,
		&v.HeartbeatIntervalSec, &v.HeartbeatMaxTurns, &v.HeartbeatTimeoutSec)
	return v, err
}

// globalHeartbeat reports the app-wide heartbeat settings an agent's overrides
// sit on top of.
//
// Without it an agent happily reports "your heartbeat is now on, every 30
// minutes" while the global heartbeat is switched off and nothing will ever
// fire. The per-agent flag alone is not sufficient, and that is invisible from
// the agent's own row.
func (m *Manager) globalHeartbeat() map[string]interface{} {
	out := map[string]interface{}{"enabled": false, "interval_sec": 0, "active_hours": ""}

	rows, err := m.db.Query(
		`SELECT key, value FROM settings WHERE key IN
		 ('heartbeat_enabled', 'heartbeat_interval_sec', 'heartbeat_active_start', 'heartbeat_active_end')`,
	)
	if err != nil {
		return out
	}
	defer rows.Close()

	var start, end string
	for rows.Next() {
		var key, val string
		if rows.Scan(&key, &val) != nil {
			continue
		}
		switch key {
		case "heartbeat_enabled":
			out["enabled"] = val == "true" || val == "1"
		case "heartbeat_interval_sec":
			var n int
			fmt.Sscanf(val, "%d", &n)
			out["interval_sec"] = n
		case "heartbeat_active_start":
			start = val
		case "heartbeat_active_end":
			end = val
		}
	}
	if start != "" || end != "" {
		out["active_hours"] = start + "–" + end
	}
	return out
}

// heartbeatInstructionsEmpty reports whether this agent's HEARTBEAT.md has
// anything in it. A missing file counts as empty, which is what the heartbeat
// itself does with it.
func (m *Manager) heartbeatInstructionsEmpty(slug string) bool {
	content, err := os.ReadFile(filepath.Join(AgentDir(m.DataDir, slug), FileHeartbeat))
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(content)) == ""
}

func (m *Manager) handleMySettingsGet(selfSlug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		v, err := m.readAgentSettings(selfSlug)
		if err != nil {
			return llm.ToolResult{Output: "Could not read your settings: " + err.Error(), IsError: true}
		}

		global := m.globalHeartbeat()
		result := map[string]interface{}{
			"me":                           v,
			"global_heartbeat":             global,
			"heartbeat_effect":             heartbeatEffect(v, global, m.heartbeatInstructionsEmpty(selfSlug)),
			"inherit_note":                 "A heartbeat value of 0 means you inherit the global setting.",
			"heartbeat_instructions_empty": m.heartbeatInstructionsEmpty(selfSlug),
			"not_settable_here":            []string{"enabled", "slug", "system_prompt"},
		}
		out, _ := json.Marshal(result)
		return llm.ToolResult{Output: string(out)}
	}
}

// heartbeatEffect states in one sentence whether this agent will actually wake
// up and do anything.
//
// Three separate things all have to be true, and each fails silently on its own:
// the agent's switch, the app-wide switch, and instructions in HEARTBEAT.md. The
// last one is the cruel one — the heartbeat runs, reads an empty file, and
// records a skip, so the user sees a feature they enabled producing nothing at
// all with no error anywhere they would look.
func heartbeatEffect(v agentSettingsView, global map[string]interface{}, instructionsEmpty bool) string {
	globalOn, _ := global["enabled"].(bool)
	switch {
	case !v.HeartbeatEnabled:
		return "You do not check in on your own — your heartbeat is off."
	case !globalOn:
		return "Your heartbeat is on, but the app-wide heartbeat is switched off, so nothing will fire. " +
			"The user has to enable it in Settings → General before your setting has any effect."
	case instructionsEmpty:
		return "Your heartbeat is on, but your HEARTBEAT.md is empty, so every wake-up is skipped and nothing happens. " +
			"Write what you should check into it with my_identity_write before telling the user this is working."
	default:
		interval := v.HeartbeatIntervalSec
		source := "your own interval"
		if interval == 0 {
			if n, ok := global["interval_sec"].(int); ok {
				interval = n
			}
			source = "the global interval"
		}
		return fmt.Sprintf("You check in roughly every %d seconds, using %s.", interval, source)
	}
}

func (m *Manager) handleMySettingsUpdate(selfSlug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			HeartbeatEnabled  *bool   `json:"heartbeat_enabled"`
			HeartbeatInterval *int    `json:"heartbeat_interval_sec"`
			HeartbeatMaxTurns *int    `json:"heartbeat_max_turns"`
			HeartbeatTimeout  *int    `json:"heartbeat_timeout_sec"`
			Model             *string `json:"model"`
			Name              *string `json:"name"`
			Description       *string `json:"description"`
			Folder            *string `json:"folder"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}

		before, err := m.readAgentSettings(selfSlug)
		if err != nil {
			return llm.ToolResult{Output: "Could not read your current settings: " + err.Error(), IsError: true}
		}

		sets := []string{"updated_at = ?"}
		args := []interface{}{time.Now().UTC()}

		// Heartbeat numbers are clamped rather than rejected. A model asked for
		// "every few minutes" will happily propose 1 second, which would pin the
		// machine running check-ins back to back forever.
		addInt := func(column string, v *int, min, max int) {
			if v == nil {
				return
			}
			n := *v
			if n != 0 { // 0 is the sentinel for "inherit the global value"
				if n < min {
					n = min
				}
				if n > max {
					n = max
				}
			}
			sets, args = append(sets, column+" = ?"), append(args, n)
		}

		if params.HeartbeatEnabled != nil {
			sets, args = append(sets, "heartbeat_enabled = ?"), append(args, *params.HeartbeatEnabled)
		}
		addInt("heartbeat_interval_sec", params.HeartbeatInterval, 60, 86400)
		addInt("heartbeat_max_turns", params.HeartbeatMaxTurns, 1, 100)
		addInt("heartbeat_timeout_sec", params.HeartbeatTimeout, 30, 3600)

		if params.Name != nil {
			name := strings.TrimSpace(*params.Name)
			if name == "" {
				return llm.ToolResult{Output: "Your name cannot be empty.", IsError: true}
			}
			sets, args = append(sets, "name = ?"), append(args, name)
		}
		if params.Description != nil {
			sets, args = append(sets, "description = ?"), append(args, strings.TrimSpace(*params.Description))
		}
		if params.Folder != nil {
			sets, args = append(sets, "folder = ?"), append(args, strings.TrimSpace(*params.Folder))
		}
		if params.Model != nil {
			model := strings.TrimSpace(*params.Model)
			if model == "" {
				return llm.ToolResult{Output: "Model cannot be empty. Omit the field to leave it unchanged.", IsError: true}
			}
			sets, args = append(sets, "model = ?"), append(args, model)
		}

		if len(sets) == 1 {
			return llm.ToolResult{Output: "Nothing to change — pass at least one setting.", IsError: true}
		}

		args = append(args, selfSlug)
		if _, err := m.db.Exec("UPDATE agent_roles SET "+strings.Join(sets, ", ")+" WHERE slug = ?", args...); err != nil {
			return llm.ToolResult{Output: "Could not save your settings: " + err.Error(), IsError: true}
		}

		after, err := m.readAgentSettings(selfSlug)
		if err != nil {
			return llm.ToolResult{Output: "Saved, but could not read the settings back: " + err.Error(), IsError: true}
		}

		m.db.LogAudit("agent:"+selfSlug, "agent_self_settings_updated", "agent", "agent_role", selfSlug, "")
		m.broadcast("agent_roles_changed", map[string]interface{}{"slug": selfSlug})

		result := map[string]interface{}{
			"updated":          true,
			"before":           before,
			"after":            after,
			"heartbeat_effect": heartbeatEffect(after, m.globalHeartbeat(), m.heartbeatInstructionsEmpty(selfSlug)),
		}
		// The heartbeat reads each agent's overrides fresh on every tick, so a
		// change lands on the next one with nothing to restart.
		if params.HeartbeatEnabled != nil || params.HeartbeatInterval != nil ||
			params.HeartbeatMaxTurns != nil || params.HeartbeatTimeout != nil {
			result["applies"] = "from your next check-in — nothing needs restarting"
		}
		if params.Model != nil {
			result["applies_model"] = "from your next message; this reply is still on the old model"
		}
		out, _ := json.Marshal(result)
		return llm.ToolResult{Output: string(out)}
	}
}

// buildAgentSettingsPromptSection tells an agent it can adjust its own setup.
func buildAgentSettingsPromptSection() string {
	return "## YOUR OWN SETTINGS\n\n" +
		"You can read and change your own configuration with `my_settings_get` and `my_settings_update` — " +
		"most usefully your **heartbeat**: whether you wake up and check in on your own, and how often.\n\n" +
		"When the user asks you to \"check in every hour\", \"stop waking up\", \"be more/less active\", or asks " +
		"what your current setup is, use these rather than describing where the Agents page is.\n\n" +
		"- Read before you write, and tell the user what the value was as well as what it becomes.\n" +
		"- A heartbeat needs BOTH your own switch and the app-wide one. `my_settings_get` reports whether the " +
		"global one is on; if it is off, say so plainly instead of implying the change took effect.\n" +
		"- An interval of 0 means you inherit the global setting rather than never running.\n" +
		"- You can only change your own settings, and not your enabled state or slug. For another agent, or to " +
		"switch one off, point the user at the Agents page.\n"
}
