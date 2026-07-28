package agents

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/scheduler"
)

// Schedules, from inside a conversation.
//
// Setting up a recurring routine used to be a hand-off: the agent explained what
// it would do, then told the user to go to the Scheduler page, pick an agent,
// write the prompt again themselves and translate "every weekday morning" into
// cron. Most of the value was in the conversation that had just happened and
// none of it survived the trip. These tools close that loop — the agent that
// worked out what should recur is the one that files it.
//
// Everything here is scoped to prompt schedules (type='prompt'). Tool-action and
// dashboard-widget schedules are machinery the builder creates as a side effect
// of building something; they are not a thing to ask an agent for.

// SchedulerControl is the slice of the scheduler these tools need. An interface
// rather than the concrete type so a Manager without a scheduler wired in (tests,
// sub-agent runs) simply doesn't offer the tools.
type SchedulerControl interface {
	AddSchedule(cfg scheduler.ScheduleConfig)
	RemoveSchedule(id string)
	RunNow(cfg scheduler.ScheduleConfig)
}

// BuildScheduleToolDefs returns the schedule tools. canModify=false yields the
// read-only subset.
//
// Unattended runs get the read-only set. A scheduled run is itself the output of
// a schedule, and one whose prompt drifts even slightly towards "set up a
// follow-up check" would file a new schedule on every fire — each of which then
// fires and files its own. There is nobody watching to notice, and the failure
// grows rather than repeats. Reading the list is still useful: a report can say
// what is already automated.
func BuildScheduleToolDefs(canModify bool) []llm.ToolDef {
	listParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"agent_slug": map[string]interface{}{
				"type":        "string",
				"description": "Only show schedules that run this agent. Omit for all of them.",
			},
		},
	})

	createParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Short name shown in the Scheduler list and as the subject of the Inbox report, e.g. \"Morning competitor sweep\".",
			},
			"cron_expr": map[string]interface{}{
				"type":        "string",
				"description": "When to run. Standard 5-field cron (\"30 8 * * 1-5\" = 8:30am weekdays) or a descriptor (@daily, @hourly, @every 2h). Times are the server's local timezone.",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "What the agent should DO on each run, written as an instruction to it. Nobody is at the keyboard when this fires, so state the task completely — the agent cannot ask a follow-up question.",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Optional note about why this schedule exists.",
			},
			"agent_slug": map[string]interface{}{
				"type":        "string",
				"description": "Which agent runs it. Defaults to you. Must be an enabled agent.",
			},
			"post_in_this_chat": map[string]interface{}{
				"type":        "boolean",
				"description": "True posts each run into this conversation. False (the default) files the report in the Inbox instead, which is usually what you want — a recurring routine posting into a chat buries it.",
			},
		},
		"required": []string{"name", "cron_expr", "prompt"},
	})

	updateParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"schedule_id": map[string]interface{}{"type": "string", "description": "Id from schedule_list."},
			"name":        map[string]interface{}{"type": "string"},
			"cron_expr":   map[string]interface{}{"type": "string", "description": "New timing. Same format as schedule_create."},
			"prompt":      map[string]interface{}{"type": "string", "description": "New instruction for each run."},
			"description": map[string]interface{}{"type": "string"},
			"agent_slug":  map[string]interface{}{"type": "string", "description": "Hand the schedule to a different agent."},
			"enabled":     map[string]interface{}{"type": "boolean", "description": "False pauses it without deleting it."},
		},
		"required": []string{"schedule_id"},
	})

	idParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"schedule_id": map[string]interface{}{"type": "string", "description": "Id from schedule_list."},
		},
		"required": []string{"schedule_id"},
	})

	listDef := llm.ToolDef{Type: "function", Function: llm.FunctionDef{
		Name: "schedule_list",
		Description: "List the recurring schedules that exist, with their timing, which agent runs them, when they last ran and when they run next. " +
			"Check this before proposing new routines so you don't suggest something already set up, and to find the id of one to change.",
		Parameters: listParams,
	}}
	if !canModify {
		return []llm.ToolDef{listDef}
	}

	return []llm.ToolDef{
		listDef,
		{Type: "function", Function: llm.FunctionDef{
			Name: "schedule_create",
			Description: "Create a recurring schedule that runs an agent with a prompt on a cron. Use this when the user asks for something to happen regularly " +
				"(\"check this every morning\", \"weekly summary\", \"remind me each Friday\"). Confirm the timing and what it will do before creating it.",
			Parameters: createParams,
		}},
		{Type: "function", Function: llm.FunctionDef{
			Name:        "schedule_update",
			Description: "Change an existing schedule's timing, prompt, name, owning agent, or enabled state. Only the fields you pass are changed.",
			Parameters:  updateParams,
		}},
		{Type: "function", Function: llm.FunctionDef{
			Name:        "schedule_delete",
			Description: "Delete a schedule permanently. Prefer schedule_update with enabled=false if the user might want it back.",
			Parameters:  idParams,
		}},
		{Type: "function", Function: llm.FunctionDef{
			Name: "schedule_run_now",
			Description: "Run a schedule immediately, without waiting for its next slot. Use it to show the user what a routine they just set up actually produces. " +
				"The run happens in the background; its report lands in the Inbox (or the pinned chat) as usual.",
			Parameters: idParams,
		}},
	}
}

func (m *Manager) MakeScheduleToolHandlers(selfSlug, threadID string, canModify bool) map[string]llm.ToolHandler {
	handlers := map[string]llm.ToolHandler{
		"schedule_list": m.handleScheduleList(),
	}
	if !canModify {
		return handlers
	}
	handlers["schedule_create"] = m.handleScheduleCreate(selfSlug, threadID)
	handlers["schedule_update"] = m.handleScheduleUpdate()
	handlers["schedule_delete"] = m.handleScheduleDelete()
	handlers["schedule_run_now"] = m.handleScheduleRunNow()
	return handlers
}

type scheduleView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CronExpr    string `json:"cron_expr"`
	Prompt      string `json:"prompt"`
	AgentSlug   string `json:"agent_slug"`
	Enabled     bool   `json:"enabled"`
	PostsToChat bool   `json:"posts_to_chat"`
	LastRun     string `json:"last_run,omitempty"`
	NextRun     string `json:"next_run,omitempty"`
}

func (m *Manager) handleScheduleList() llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			AgentSlug string `json:"agent_slug"`
		}
		json.Unmarshal(input, &params)

		query := `SELECT id, name, description, cron_expr, prompt_content, agent_role_slug,
		                 enabled, thread_id, last_run_at, next_run_at
		          FROM schedules WHERE type = 'prompt'`
		var args []interface{}
		if slug := strings.TrimSpace(params.AgentSlug); slug != "" {
			query += " AND agent_role_slug = ?"
			args = append(args, slug)
		}
		query += " ORDER BY created_at DESC LIMIT 100"

		rows, err := m.db.Query(query, args...)
		if err != nil {
			return llm.ToolResult{Output: "Could not read schedules: " + err.Error(), IsError: true}
		}
		defer rows.Close()

		schedules := []scheduleView{}
		for rows.Next() {
			var s scheduleView
			var threadID string
			var lastRun, nextRun sql.NullTime
			if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.CronExpr, &s.Prompt,
				&s.AgentSlug, &s.Enabled, &threadID, &lastRun, &nextRun); err != nil {
				continue
			}
			s.PostsToChat = threadID != ""
			if lastRun.Valid {
				s.LastRun = lastRun.Time.Local().Format("Mon 2 Jan 3:04 PM")
			}
			if nextRun.Valid {
				s.NextRun = nextRun.Time.Local().Format("Mon 2 Jan 3:04 PM")
			}
			schedules = append(schedules, s)
		}

		if len(schedules) == 0 {
			return llm.ToolResult{Output: `{"schedules":[],"note":"Nothing is scheduled yet."}`}
		}
		out, _ := json.Marshal(map[string]interface{}{"schedules": schedules})
		return llm.ToolResult{Output: string(out)}
	}
}

func (m *Manager) handleScheduleCreate(selfSlug, threadID string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		if m.Scheduler == nil {
			return llm.ToolResult{Output: "The scheduler is not available in this run.", IsError: true}
		}

		var params struct {
			Name           string `json:"name"`
			CronExpr       string `json:"cron_expr"`
			Prompt         string `json:"prompt"`
			Description    string `json:"description"`
			AgentSlug      string `json:"agent_slug"`
			PostInThisChat bool   `json:"post_in_this_chat"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}

		params.Name = strings.TrimSpace(params.Name)
		params.Prompt = strings.TrimSpace(params.Prompt)
		if params.Name == "" || params.Prompt == "" {
			return llm.ToolResult{Output: "name and prompt are both required.", IsError: true}
		}

		// Validated before anything is written: a schedule with an unparseable
		// expression saves happily and then simply never fires, which reads to
		// the user as the feature being broken rather than the cron being wrong.
		cronExpr, err := scheduler.NormalizeCron(params.CronExpr)
		if err != nil {
			return llm.ToolResult{Output: err.Error(), IsError: true}
		}

		agentSlug := strings.TrimSpace(params.AgentSlug)
		if agentSlug == "" {
			agentSlug = selfSlug
		}
		var found string
		if err := m.db.QueryRow(
			"SELECT slug FROM agent_roles WHERE slug = ? AND enabled = 1", agentSlug,
		).Scan(&found); err != nil {
			return llm.ToolResult{
				Output:  fmt.Sprintf("There is no enabled agent with the slug %q. Use schedule_list or ask the user which agent should run this.", agentSlug),
				IsError: true,
			}
		}

		// An unpinned schedule files its report in the Inbox. Pinning it to this
		// thread is opt-in because a routine posting into a conversation every
		// morning buries the conversation.
		targetThread := ""
		if params.PostInThisChat {
			targetThread = threadID
		}

		id := uuid.New().String()
		now := time.Now().UTC()
		workspaceID := m.threadWorkspaceID(threadID)
		var workspacePtr *string
		if workspaceID != "" {
			workspacePtr = &workspaceID
		}

		if _, err := m.db.Exec(
			`INSERT INTO schedules (id, name, description, cron_expr, tool_id, action, payload, enabled,
			                        type, agent_role_slug, prompt_content, thread_id, workspace_id, provider, created_at, updated_at)
			 VALUES (?, ?, ?, ?, '', '', '{}', 1, 'prompt', ?, ?, ?, ?, '', ?, ?)`,
			id, params.Name, params.Description, cronExpr,
			agentSlug, params.Prompt, targetThread, workspacePtr, now, now,
		); err != nil {
			return llm.ToolResult{Output: "Could not save the schedule: " + err.Error(), IsError: true}
		}

		m.Scheduler.AddSchedule(scheduler.ScheduleConfig{
			ID:            id,
			CronExpr:      cronExpr,
			AgentRoleSlug: agentSlug,
			PromptContent: params.Prompt,
			ThreadID:      targetThread,
			WorkspaceID:   workspaceID,
		})

		m.db.LogAudit("agent:"+selfSlug, "schedule_created", "schedule", "schedule", id, params.Name)
		m.broadcast("schedules_changed", map[string]interface{}{"id": id, "action": "created"})

		result := map[string]interface{}{
			"schedule_id": id,
			"name":        params.Name,
			"cron_expr":   cronExpr,
			"agent_slug":  agentSlug,
			"delivery":    "Inbox report",
		}
		if targetThread != "" {
			result["delivery"] = "posted into this chat"
		}
		if next, err := scheduler.NextRun(cronExpr); err == nil {
			result["first_run"] = next.Local().Format("Mon 2 Jan 3:04 PM")
		}
		out, _ := json.Marshal(result)
		return llm.ToolResult{Output: string(out)}
	}
}

func (m *Manager) handleScheduleUpdate() llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		if m.Scheduler == nil {
			return llm.ToolResult{Output: "The scheduler is not available in this run.", IsError: true}
		}

		var params struct {
			ScheduleID  string  `json:"schedule_id"`
			Name        *string `json:"name"`
			CronExpr    *string `json:"cron_expr"`
			Prompt      *string `json:"prompt"`
			Description *string `json:"description"`
			AgentSlug   *string `json:"agent_slug"`
			Enabled     *bool   `json:"enabled"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if strings.TrimSpace(params.ScheduleID) == "" {
			return llm.ToolResult{Output: "schedule_id is required. Use schedule_list to find it.", IsError: true}
		}

		var sType string
		if err := m.db.QueryRow(
			"SELECT type FROM schedules WHERE id = ?", params.ScheduleID,
		).Scan(&sType); err != nil {
			return llm.ToolResult{Output: "No schedule with that id. Use schedule_list to find it.", IsError: true}
		}
		// Tool-action and dashboard schedules are generated machinery whose
		// payload these tools cannot express — editing one through here would
		// half-rewrite it.
		if sType != "prompt" {
			return llm.ToolResult{
				Output:  fmt.Sprintf("That schedule is a %q schedule, created automatically to drive a service or dashboard. It has to be changed where it came from.", sType),
				IsError: true,
			}
		}

		sets := []string{"updated_at = ?"}
		args := []interface{}{time.Now().UTC()}

		if params.Name != nil {
			sets, args = append(sets, "name = ?"), append(args, strings.TrimSpace(*params.Name))
		}
		if params.Description != nil {
			sets, args = append(sets, "description = ?"), append(args, *params.Description)
		}
		if params.Prompt != nil {
			sets, args = append(sets, "prompt_content = ?"), append(args, strings.TrimSpace(*params.Prompt))
		}
		if params.CronExpr != nil {
			expr, err := scheduler.NormalizeCron(*params.CronExpr)
			if err != nil {
				return llm.ToolResult{Output: err.Error(), IsError: true}
			}
			sets, args = append(sets, "cron_expr = ?"), append(args, expr)
		}
		if params.AgentSlug != nil {
			slug := strings.TrimSpace(*params.AgentSlug)
			var found string
			if err := m.db.QueryRow("SELECT slug FROM agent_roles WHERE slug = ? AND enabled = 1", slug).Scan(&found); err != nil {
				return llm.ToolResult{Output: fmt.Sprintf("There is no enabled agent with the slug %q.", slug), IsError: true}
			}
			sets, args = append(sets, "agent_role_slug = ?"), append(args, slug)
		}
		if params.Enabled != nil {
			sets, args = append(sets, "enabled = ?"), append(args, *params.Enabled)
		}

		if len(sets) == 1 {
			return llm.ToolResult{Output: "Nothing to change — pass at least one field.", IsError: true}
		}

		args = append(args, params.ScheduleID)
		if _, err := m.db.Exec("UPDATE schedules SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil {
			return llm.ToolResult{Output: "Could not update the schedule: " + err.Error(), IsError: true}
		}

		// Re-register from the saved row, not from the request. The live cron
		// entry carries the prompt, agent and thread as well as the timing, so an
		// edit that isn't re-registered keeps running the old version until the
		// next restart — which looks exactly like the edit not saving.
		cfg, enabled, err := m.scheduleConfig(params.ScheduleID)
		if err != nil {
			return llm.ToolResult{Output: "Saved, but could not re-read the schedule to restart it: " + err.Error(), IsError: true}
		}
		m.Scheduler.RemoveSchedule(params.ScheduleID)
		if enabled {
			m.Scheduler.AddSchedule(cfg)
		}

		m.db.LogAudit("agent", "schedule_updated", "schedule", "schedule", params.ScheduleID, "")
		m.broadcast("schedules_changed", map[string]interface{}{"id": params.ScheduleID, "action": "updated"})

		result := map[string]interface{}{
			"schedule_id": params.ScheduleID,
			"updated":     true,
			"enabled":     enabled,
			"cron_expr":   cfg.CronExpr,
		}
		if !enabled {
			result["note"] = "Paused — it will not run until it is enabled again."
		} else if next, err := scheduler.NextRun(cfg.CronExpr); err == nil {
			result["next_run"] = next.Local().Format("Mon 2 Jan 3:04 PM")
		}
		out, _ := json.Marshal(result)
		return llm.ToolResult{Output: string(out)}
	}
}

func (m *Manager) handleScheduleDelete() llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		if m.Scheduler == nil {
			return llm.ToolResult{Output: "The scheduler is not available in this run.", IsError: true}
		}

		var params struct {
			ScheduleID string `json:"schedule_id"`
		}
		json.Unmarshal(input, &params)
		if strings.TrimSpace(params.ScheduleID) == "" {
			return llm.ToolResult{Output: "schedule_id is required.", IsError: true}
		}

		var name, sType string
		if err := m.db.QueryRow(
			"SELECT name, type FROM schedules WHERE id = ?", params.ScheduleID,
		).Scan(&name, &sType); err != nil {
			return llm.ToolResult{Output: "No schedule with that id.", IsError: true}
		}
		if sType != "prompt" {
			return llm.ToolResult{
				Output:  fmt.Sprintf("That is a %q schedule driving a service or dashboard — deleting it here would break what depends on it.", sType),
				IsError: true,
			}
		}

		if _, err := m.db.Exec("DELETE FROM schedules WHERE id = ?", params.ScheduleID); err != nil {
			return llm.ToolResult{Output: "Could not delete the schedule: " + err.Error(), IsError: true}
		}
		m.Scheduler.RemoveSchedule(params.ScheduleID)

		m.db.LogAudit("agent", "schedule_deleted", "schedule", "schedule", params.ScheduleID, name)
		m.broadcast("schedules_changed", map[string]interface{}{"id": params.ScheduleID, "action": "deleted"})

		out, _ := json.Marshal(map[string]interface{}{"deleted": true, "name": name})
		return llm.ToolResult{Output: string(out)}
	}
}

func (m *Manager) handleScheduleRunNow() llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		if m.Scheduler == nil {
			return llm.ToolResult{Output: "The scheduler is not available in this run.", IsError: true}
		}

		var params struct {
			ScheduleID string `json:"schedule_id"`
		}
		json.Unmarshal(input, &params)
		if strings.TrimSpace(params.ScheduleID) == "" {
			return llm.ToolResult{Output: "schedule_id is required.", IsError: true}
		}

		cfg, _, err := m.scheduleConfig(params.ScheduleID)
		if err != nil {
			return llm.ToolResult{Output: "No schedule with that id.", IsError: true}
		}
		m.Scheduler.RunNow(cfg)

		m.db.LogAudit("agent", "schedule_run_now", "schedule", "schedule", params.ScheduleID, "")

		out, _ := json.Marshal(map[string]interface{}{
			"started": true,
			"note":    "Running in the background. The report will appear in the Inbox (or the pinned chat) when it finishes — it will not come back through this tool.",
		})
		return llm.ToolResult{Output: string(out)}
	}
}

// scheduleConfig reads a schedule back into the shape the cron registry wants.
func (m *Manager) scheduleConfig(id string) (scheduler.ScheduleConfig, bool, error) {
	var cfg scheduler.ScheduleConfig
	var enabled bool
	var workspaceID sql.NullString
	err := m.db.QueryRow(
		`SELECT cron_expr, agent_role_slug, prompt_content, thread_id, workspace_id, provider, enabled
		 FROM schedules WHERE id = ?`, id,
	).Scan(&cfg.CronExpr, &cfg.AgentRoleSlug, &cfg.PromptContent, &cfg.ThreadID,
		&workspaceID, &cfg.Provider, &enabled)
	if err != nil {
		return cfg, false, err
	}
	cfg.ID = id
	cfg.WorkspaceID = workspaceID.String
	return cfg, enabled, nil
}

// buildSchedulePromptSection tells an agent it can schedule things, and what a
// schedule that actually works looks like.
//
// The guidance matters more than the tool list. A scheduled run has nobody at
// the other end, so the failure mode is a prompt written as a conversational
// opener — "check if there's anything new and let me know what you'd like me to
// do" — which produces a daily report asking a question that is never answered.
func (m *Manager) buildSchedulePromptSection(canModify bool) string {
	var b strings.Builder
	b.WriteString("## SCHEDULES (recurring routines)\n\n")
	if !canModify {
		b.WriteString("`schedule_list` shows the recurring routines that are set up. " +
			"You cannot create or change schedules during an unattended run — if this run turns up something " +
			"that ought to recur, say so in your report and let the user set it up.\n")
		if section := m.existingSchedulesSection(); section != "" {
			b.WriteString("\n" + section)
		}
		return b.String()
	}
	b.WriteString("You can set up work that repeats on its own: `schedule_list`, `schedule_create`, " +
		"`schedule_update`, `schedule_delete`, `schedule_run_now`.\n\n")
	b.WriteString("When the user describes anything recurring — \"every morning\", \"each Friday\", " +
		"\"keep an eye on\", \"remind me weekly\" — offer to schedule it rather than describing how they could.\n\n")

	b.WriteString("Writing the prompt for a run:\n")
	b.WriteString("- Nobody is present when it fires. Write a complete instruction, not a conversation opener. " +
		"It cannot ask a clarifying question, and any question it ends on goes unanswered.\n")
	b.WriteString("- Say what to produce, not just what to look at. \"Check the repo\" yields a run that checked the repo.\n")
	b.WriteString("- Reports land in the Inbox by default. Only set `post_in_this_chat` if the user wants this " +
		"specific conversation to keep receiving them.\n\n")

	b.WriteString("Timing: plain 5-field cron is fine (`30 8 * * 1-5` = 8:30am on weekdays), as are `@daily` and " +
		"`@every 6h`. Times are the server's local timezone. Always tell the user in words when it will run, " +
		"and confirm before creating it.\n")

	if section := m.existingSchedulesSection(); section != "" {
		b.WriteString("\n" + section)
	}
	return b.String()
}

// existingSchedulesSection lists what is already scheduled.
//
// Present in the prompt rather than left to a schedule_list call because the
// common request is "what should I automate?", and an agent answering that
// without knowing what already runs proposes things the user set up months ago.
func (m *Manager) existingSchedulesSection() string {
	rows, err := m.db.Query(
		`SELECT name, cron_expr, agent_role_slug, enabled FROM schedules
		 WHERE type = 'prompt' ORDER BY created_at DESC LIMIT 25`,
	)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var name, cronExpr, slug string
		var enabled bool
		if rows.Scan(&name, &cronExpr, &slug, &enabled) != nil {
			continue
		}
		state := ""
		if !enabled {
			state = " — paused"
		}
		lines = append(lines, fmt.Sprintf("- **%s** (`%s`, runs @%s)%s", name, cronExpr, slug, state))
	}
	if len(lines) == 0 {
		return "Nothing is scheduled yet.\n"
	}
	return "Already scheduled — don't propose these again:\n" + strings.Join(lines, "\n") + "\n"
}
