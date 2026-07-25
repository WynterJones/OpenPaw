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
	p, _ := json.Marshal(map[string]interface{}{
		"type":       "object",
		"properties": props,
		"required":   required,
	})
	return p
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
