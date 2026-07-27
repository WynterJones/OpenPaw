package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/tmux"
)

// Tools that let an agent inspect and watch tmux sessions.
//
// The watcher itself already existed but was only reachable from a button in
// the UI, so agents would confidently promise "I'll check back in 15 minutes"
// and then simply not — a turn is one request/response, and nothing survived
// the end of it. These tools hand the agent the mechanism behind that button,
// which turns the promise into a real server-side poll that reports back.
//
// tmux_status/tmux_list read through internal/tmux directly (no import cycle,
// it depends on nothing of ours). Starting a watch has to post a message into a
// thread, which lives in handlers — and handlers already imports agents — so it
// arrives as a callback on Manager instead.

// TmuxWatchStarter starts a server-side watch that reports into a chat thread.
type TmuxWatchStarter func(threadID, session string, intervalSeconds int) error

// TmuxWatchStopper cancels watches for a thread ("" session = all of them).
type TmuxWatchStopper func(threadID, session string) int

// BuildTmuxToolDefs returns the tmux tool definitions.
func BuildTmuxToolDefs() []llm.ToolDef {
	return []llm.ToolDef{
		buildTmuxRunDef(),
		buildTmuxListDef(),
		buildTmuxStatusDef(),
		buildTmuxWatchDef(),
		buildTmuxUnwatchDef(),
	}
}

// MakeTmuxToolHandlers builds the handlers for one run, with the thread baked
// in so a watch always reports back where the conversation is happening.
func (m *Manager) MakeTmuxToolHandlers(threadID string) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"tmux_run":     m.handleTmuxRun(threadID),
		"tmux_list":    handleTmuxList,
		"tmux_status":  handleTmuxStatus,
		"tmux_watch":   m.handleTmuxWatch(threadID),
		"tmux_unwatch": m.handleTmuxUnwatch(threadID),
	}
}

func emptyParams() json.RawMessage {
	p, _ := json.Marshal(map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	})
	return p
}

func sessionParams(extra map[string]interface{}, required ...string) json.RawMessage {
	props := map[string]interface{}{
		"session": map[string]interface{}{
			"type":        "string",
			"description": "The tmux session name, as returned by tmux_list.",
		},
	}
	for k, v := range extra {
		props[k] = v
	}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": props,
	}
	// A nil variadic marshals to `"required": null`, which is not a valid JSON
	// Schema. Claude Code validates every tool in tools/list and drops the WHOLE
	// MCP server on one failure, so this single field used to take every OpenPaw
	// tool down with it. Omit the key instead.
	if len(required) > 0 {
		schema["required"] = required
	}
	p, _ := json.Marshal(schema)
	return p
}

func buildTmuxRunDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to run, e.g. \"npm run build\" or \"go test ./...\".",
			},
			"session": map[string]interface{}{
				"type": "string",
				"description": "Optional name for the session, so you can find it again. " +
					"Letters, digits, dashes and underscores only. Defaults to a name derived from the command.",
			},
			"watch": map[string]interface{}{
				"type": "boolean",
				"description": "Report back into this chat when the command finishes or stalls. " +
					"Defaults to true — leave it on unless the user only wants the session started.",
				"default": true,
			},
		},
		"required": []string{"command"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name: "tmux_run",
			Description: "Run a long-running command — a build, test suite, dev server, install, migration, " +
				"deploy — in a detached tmux session. PREFER THIS over running such commands inline: your " +
				"turn ends when you reply, so an inline command that outlasts the turn is killed half-done, " +
				"and its output is lost. In tmux the work keeps going, the user can attach and watch it, and " +
				"you get told when it finishes. Use a normal inline command only for something quick whose " +
				"output you need in this same reply.",
			Parameters: params,
		},
	}
}

func buildTmuxListDef() llm.ToolDef {
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name: "tmux_list",
			Description: "List running tmux sessions, including any Claude Code or Codex sessions. " +
				"Use this to find out what long-running work is currently in flight.",
			Parameters: emptyParams(),
		},
	}
}

func buildTmuxStatusDef() llm.ToolDef {
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name: "tmux_status",
			Description: "Read the current state of one tmux session right now: its project, branch, " +
				"model, context usage, elapsed time, lines changed, and the last lines of output. " +
				"This is a single snapshot — for anything ongoing, use tmux_watch instead of calling " +
				"this repeatedly.",
			Parameters: sessionParams(nil, "session"),
		},
	}
}

func buildTmuxWatchDef() llm.ToolDef {
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name: "tmux_watch",
			Description: "Watch a tmux session and report back into this chat when it finishes or stalls. " +
				"USE THIS whenever you want to check on a session later. Your turn ends when you reply, " +
				"so you cannot check back on your own — this schedules a server-side poll that outlives " +
				"the turn and posts the result here. Say that you have set it up, then stop; do not " +
				"promise to check back without calling this.",
			Parameters: sessionParams(map[string]interface{}{
				"interval_seconds": map[string]interface{}{
					"type": "integer",
					"description": "How often to check, in seconds. Defaults to 60. " +
						"Clamped to between 10 seconds and 15 minutes.",
					"default": 60,
				},
			}, "session"),
		},
	}
}

func buildTmuxUnwatchDef() llm.ToolDef {
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "tmux_unwatch",
			Description: "Stop watching a tmux session. Omit `session` to stop every watch on this chat.",
			Parameters:  sessionParams(nil),
		},
	}
}

// handleTmuxRun starts a command detached and, unless told otherwise, arms the
// watcher in the same call. Watching is the default because the two halves are
// only useful together: a session nobody reports on is work the agent started
// and then silently forgot.
func (m *Manager) handleTmuxRun(threadID string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var req struct {
			Command string `json:"command"`
			Session string `json:"session"`
			Watch   *bool  `json:"watch"`
		}
		json.Unmarshal(input, &req)

		if strings.TrimSpace(req.Command) == "" {
			return llm.ToolResult{Output: "command is required", IsError: true}
		}
		if !tmux.Available() {
			return llm.ToolResult{
				Output: "tmux is not installed on this machine, so this command cannot be run detached. " +
					"Run it inline instead, or ask the user to install tmux.",
			}
		}

		label := req.Session
		if strings.TrimSpace(label) == "" {
			label = firstWords(req.Command, 4)
		}
		name := uniqueSessionName(ctx, tmux.SessionName(label))

		// A detached Claude Code / Codex run has nobody to answer its approval
		// prompts, so they are turned off. Report the command that actually ran
		// rather than the one asked for — otherwise the agent tells the user
		// something different from what is in the pane.
		command := tmux.SkipPermissionPrompts(req.Command)

		if err := tmux.Start(ctx, name, workDir, command); err != nil {
			return llm.ToolResult{Output: "Failed to start the session: " + err.Error(), IsError: true}
		}

		out := fmt.Sprintf("Started %q in tmux, running: %s", name, command)

		// Default to watching; only an explicit false opts out.
		if req.Watch != nil && !*req.Watch {
			return llm.ToolResult{Output: out + "\nNot watching it — use tmux_status or tmux_watch to check on it."}
		}
		if threadID == "" || m.TmuxWatchFn == nil {
			return llm.ToolResult{Output: out + "\nReporting back is not available here, so check it with tmux_status."}
		}
		if err := m.TmuxWatchFn(threadID, name, 0); err != nil {
			return llm.ToolResult{Output: out + "\nCould not start watching it: " + err.Error()}
		}
		return llm.ToolResult{Output: out + "\nWatching it — I'll post here when it finishes or goes quiet. " +
			"Tell the user it is running and stop; do not wait on it in this turn."}
	}
}

// uniqueSessionName appends a suffix when the name is taken, so a second build
// doesn't fail just because the first one is still running.
func uniqueSessionName(ctx context.Context, base string) string {
	if !tmux.Exists(ctx, base) {
		return base
	}
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !tmux.Exists(ctx, candidate) {
			return candidate
		}
	}
	return base
}

// firstWords builds a readable session name out of the command itself.
func firstWords(command string, n int) string {
	fields := strings.Fields(command)
	if len(fields) > n {
		fields = fields[:n]
	}
	return strings.Join(fields, "-")
}

func handleTmuxList(ctx context.Context, _ string, _ json.RawMessage) llm.ToolResult {
	if !tmux.Available() {
		return llm.ToolResult{Output: "tmux is not installed on this machine."}
	}
	sessions, err := tmux.List(ctx)
	if err != nil {
		return llm.ToolResult{Output: "Failed to list tmux sessions: " + err.Error(), IsError: true}
	}
	if len(sessions) == 0 {
		return llm.ToolResult{Output: "No tmux sessions are running."}
	}

	var b strings.Builder
	for _, s := range sessions {
		fmt.Fprintf(&b, "- %s (%s, %d window(s)", s.Name, s.Kind, s.Windows)
		if s.Attached {
			b.WriteString(", attached")
		}
		b.WriteString(")")
		if s.Status != nil && s.Status.Project != "" {
			fmt.Fprintf(&b, " — %s", s.Status.Project)
			if s.Status.Branch != "" {
				fmt.Fprintf(&b, " (%s)", s.Status.Branch)
			}
		}
		b.WriteString("\n")
	}
	return llm.ToolResult{Output: strings.TrimRight(b.String(), "\n")}
}

func handleTmuxStatus(ctx context.Context, _ string, input json.RawMessage) llm.ToolResult {
	var req struct {
		Session string `json:"session"`
	}
	json.Unmarshal(input, &req)
	if req.Session == "" {
		return llm.ToolResult{Output: "session is required", IsError: true}
	}
	if !tmux.Exists(ctx, req.Session) {
		return llm.ToolResult{Output: fmt.Sprintf(
			"There is no tmux session named %q — it may have finished already.", req.Session)}
	}

	pane, err := tmux.Capture(ctx, req.Session)
	if err != nil {
		return llm.ToolResult{Output: "Failed to read the session: " + err.Error(), IsError: true}
	}
	return llm.ToolResult{Output: tmux.Describe(req.Session, pane)}
}

func (m *Manager) handleTmuxWatch(threadID string) llm.ToolHandler {
	return func(ctx context.Context, _ string, input json.RawMessage) llm.ToolResult {
		var req struct {
			Session   string `json:"session"`
			IntervalS int    `json:"interval_seconds"`
		}
		json.Unmarshal(input, &req)

		if req.Session == "" {
			return llm.ToolResult{Output: "session is required", IsError: true}
		}
		if threadID == "" {
			return llm.ToolResult{
				Output:  "This run has no chat thread, so there is nowhere to report back to.",
				IsError: true,
			}
		}
		if m.TmuxWatchFn == nil {
			return llm.ToolResult{Output: "Watching tmux sessions is not available.", IsError: true}
		}
		if !tmux.Exists(ctx, req.Session) {
			return llm.ToolResult{Output: fmt.Sprintf(
				"There is no tmux session named %q, so there is nothing to watch.", req.Session)}
		}

		if err := m.TmuxWatchFn(threadID, req.Session, req.IntervalS); err != nil {
			return llm.ToolResult{Output: "Failed to start watching: " + err.Error(), IsError: true}
		}
		return llm.ToolResult{Output: fmt.Sprintf(
			"Now watching %q. I'll post an update in this chat when it finishes or goes quiet. "+
				"Tell the user it's set up — do not promise to check again yourself.", req.Session)}
	}
}

func (m *Manager) handleTmuxUnwatch(threadID string) llm.ToolHandler {
	return func(_ context.Context, _ string, input json.RawMessage) llm.ToolResult {
		var req struct {
			Session string `json:"session"`
		}
		json.Unmarshal(input, &req)

		if m.TmuxUnwatchFn == nil || threadID == "" {
			return llm.ToolResult{Output: "There are no watches to stop."}
		}
		switch n := m.TmuxUnwatchFn(threadID, req.Session); n {
		case 0:
			return llm.ToolResult{Output: "There were no active watches on this chat."}
		case 1:
			return llm.ToolResult{Output: "Stopped 1 watch."}
		default:
			return llm.ToolResult{Output: fmt.Sprintf("Stopped %d watches.", n)}
		}
	}
}

// buildTmuxPromptSection tells CLI-engine agents to put long work in tmux.
//
// Injected only for Claude Code / Codex, which have a real shell in the
// workspace. The default matters because the failure is silent: an inline build
// that outlasts the turn is killed partway with nothing to show, and the agent
// reports success on work that never finished.
func buildTmuxPromptSection() string {
	return `## LONG-RUNNING WORK — USE TMUX

You are running on a CLI engine with a real shell, and tmux is available. Your turn ends when you reply, and anything still running inline is killed with it.

**Default to ` + "`tmux_run`" + ` for work that takes more than a few seconds**: builds, test suites, dev servers, installs, migrations, deploys, long scripts. It starts the command detached, keeps it alive past this turn, lets the user attach and watch it, and reports back here when it finishes.

- ` + "`tmux_run`" + ` — start it. Watching is on by default; leave it on.
- ` + "`tmux_list`" + ` / ` + "`tmux_status`" + ` — see what is already running.
- ` + "`tmux_watch`" + ` — report back on a session you did not start.

Claude Code and Codex started this way run with their approval prompts turned off automatically — nobody is watching a detached pane to answer one. Do not add permission flags yourself.

Run a command inline only when it is quick AND you need its output in this same reply (` + "`git status`, `ls`, reading a file" + `). When you start something in tmux, say so and finish your reply — do not sit and wait on it.`
}
