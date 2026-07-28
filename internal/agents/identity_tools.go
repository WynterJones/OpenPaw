package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	llm "github.com/openpaw/openpaw/internal/llm"
)

// An agent's own identity files, by name.
//
// Identity-initialized agents already have Read/Write/Edit sandboxed to their
// directory, so they *could* always edit these. In practice they didn't: the
// files were described in one paragraph of a long system prompt, addressed by
// absolute path, and HEARTBEAT.md was not mentioned at all. Asking an agent to
// "update your runbook" got a description of what it would write.
//
// Naming the files as tools makes them a thing the agent can see it has, rather
// than a path it has to remember. The generic file tools still work and are
// still needed for skills; these are the discoverable front door for the five
// files that define the agent.

// identityFileDoc describes one editable file. The blurbs are written for the
// agent to relay to the user, because "what is my BOOT file?" is a question the
// user will ask the agent rather than the docs.
type identityFileDoc struct {
	Name    string `json:"file"`
	Purpose string `json:"purpose"`
}

var identityFileDocs = []identityFileDoc{
	{FileSoul, "Your personality and core identity — who you are and how you speak. Part of your system prompt on every message."},
	{FileRunbook, "Your operational playbook: session rules, response style, lessons learned, process notes. This is the one you should be evolving as you learn what works."},
	{FileUser, "What you know about the user's preferences and working style."},
	{FileBoot, "What you do at the start of a new session."},
	{FileHeartbeat, "What you do when you wake up on your own, with nobody present. IMPORTANT: if this file is empty your heartbeat does nothing — every wake-up is skipped, however the heartbeat setting is configured."},
}

func identityFilePurpose(name string) string {
	for _, d := range identityFileDocs {
		if d.Name == name {
			return d.Purpose
		}
	}
	return ""
}

// identityFileWriteCap bounds one file. These are concatenated into the system
// prompt on every single message, so an agent that writes an essay into its own
// SOUL.md pays for it on every request from then on, forever.
const identityFileWriteCap = 32 * 1024

func BuildIdentityToolDefs() []llm.ToolDef {
	noParams, _ := json.Marshal(map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{},
	})

	fileEnum := make([]string, 0, len(identityFileDocs))
	for _, d := range identityFileDocs {
		fileEnum = append(fileEnum, d.Name)
	}

	readParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file": map[string]interface{}{
				"type": "string", "enum": fileEnum,
				"description": "Which of your files to read.",
			},
		},
		"required": []string{"file"},
	})

	writeParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file": map[string]interface{}{
				"type": "string", "enum": fileEnum,
				"description": "Which of your files to write.",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Markdown content.",
			},
			"mode": map[string]interface{}{
				"type": "string", "enum": []string{"replace", "append"},
				"description": "\"replace\" (default) overwrites the whole file — read it first so you don't discard something. \"append\" adds to the end, which is usually right for adding a lesson to your runbook.",
			},
		},
		"required": []string{"file", "content"},
	})

	return []llm.ToolDef{
		{Type: "function", Function: llm.FunctionDef{
			Name: "my_identity_list",
			Description: "List your own identity files — SOUL, RUNBOOK, USER, BOOT and HEARTBEAT — with what each one is for and whether it currently has anything in it. " +
				"Use this when the user asks what defines you, what you do on a heartbeat, or asks you to change how you behave.",
			Parameters: noParams,
		}},
		{Type: "function", Function: llm.FunctionDef{
			Name:        "my_identity_read",
			Description: "Read one of your own identity files. Always read before writing, so you can show the user what is there now and not overwrite something you did not know about.",
			Parameters:  readParams,
		}},
		{Type: "function", Function: llm.FunctionDef{
			Name: "my_identity_write",
			Description: "Write one of your own identity files. This is how you change your own behaviour — your personality (SOUL), your working rules and lessons (RUNBOOK), " +
				"your startup routine (BOOT), or what you do when you wake up on your own (HEARTBEAT). Show the user what you intend to write and get agreement before writing it.",
			Parameters: writeParams,
		}},
	}
}

func (m *Manager) MakeIdentityToolHandlers(selfSlug string) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"my_identity_list":  m.handleIdentityList(selfSlug),
		"my_identity_read":  m.handleIdentityRead(selfSlug),
		"my_identity_write": m.handleIdentityWrite(selfSlug),
	}
}

// resolveIdentityFile maps a requested filename to a path inside the agent's own
// directory, refusing anything not on the list.
//
// The enum in the tool schema is a hint, not a guarantee — models pass values
// outside an enum, and a filename is one "../" away from being an arbitrary
// write to disk. The allowlist is checked here because this is the only place
// that can actually be relied on.
func (m *Manager) resolveIdentityFile(slug, name string) (string, error) {
	name = strings.TrimSpace(name)
	if identityFilePurpose(name) == "" {
		var allowed []string
		for _, d := range identityFileDocs {
			allowed = append(allowed, d.Name)
		}
		return "", fmt.Errorf("%q is not one of your identity files. Choose one of: %s", name, strings.Join(allowed, ", "))
	}
	return filepath.Join(AgentDir(m.DataDir, slug), name), nil
}

type identityFileState struct {
	Name    string `json:"file"`
	Purpose string `json:"purpose"`
	Bytes   int    `json:"bytes"`
	Empty   bool   `json:"empty"`
}

func (m *Manager) identityFileStates(slug string) []identityFileState {
	dir := AgentDir(m.DataDir, slug)
	states := make([]identityFileState, 0, len(identityFileDocs))
	for _, d := range identityFileDocs {
		content, err := os.ReadFile(filepath.Join(dir, d.Name))
		trimmed := ""
		if err == nil {
			trimmed = strings.TrimSpace(string(content))
		}
		states = append(states, identityFileState{
			Name:    d.Name,
			Purpose: d.Purpose,
			Bytes:   len(trimmed),
			Empty:   trimmed == "",
		})
	}
	return states
}

func (m *Manager) handleIdentityList(selfSlug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		states := m.identityFileStates(selfSlug)

		result := map[string]interface{}{
			"files": states,
			"note":  "Changes to these take effect from your NEXT message — the prompt for this one was already assembled.",
		}
		// The heartbeat is the one place where an empty file silently disables a
		// feature the user believes they switched on, so it is called out rather
		// than left for the agent to infer from a byte count.
		for _, s := range states {
			if s.Name == FileHeartbeat && s.Empty {
				result["heartbeat_warning"] = "Your HEARTBEAT.md is empty, so every heartbeat is skipped even if the setting is on. " +
					"Write instructions into it to make your heartbeat do anything."
			}
		}
		out, _ := json.Marshal(result)
		return llm.ToolResult{Output: string(out)}
	}
}

func (m *Manager) handleIdentityRead(selfSlug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			File string `json:"file"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}

		path, err := m.resolveIdentityFile(selfSlug, params.File)
		if err != nil {
			return llm.ToolResult{Output: err.Error(), IsError: true}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				out, _ := json.Marshal(map[string]interface{}{
					"file": params.File, "content": "", "empty": true,
					"purpose": identityFilePurpose(params.File),
					"note":    "This file does not exist yet. Writing to it creates it.",
				})
				return llm.ToolResult{Output: string(out)}
			}
			return llm.ToolResult{Output: "Could not read " + params.File + ": " + err.Error(), IsError: true}
		}

		out, _ := json.Marshal(map[string]interface{}{
			"file":    params.File,
			"purpose": identityFilePurpose(params.File),
			"content": string(content),
			"empty":   strings.TrimSpace(string(content)) == "",
		})
		return llm.ToolResult{Output: string(out)}
	}
}

func (m *Manager) handleIdentityWrite(selfSlug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			File    string `json:"file"`
			Content string `json:"content"`
			Mode    string `json:"mode"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}

		path, err := m.resolveIdentityFile(selfSlug, params.File)
		if err != nil {
			return llm.ToolResult{Output: err.Error(), IsError: true}
		}

		existing, _ := os.ReadFile(path)
		content := params.Content
		if strings.EqualFold(strings.TrimSpace(params.Mode), "append") && strings.TrimSpace(string(existing)) != "" {
			content = strings.TrimRight(string(existing), "\n") + "\n\n" + strings.TrimSpace(params.Content) + "\n"
		}

		if len(content) > identityFileWriteCap {
			return llm.ToolResult{
				Output: fmt.Sprintf("That is %d bytes and the limit is %d. These files are part of your system prompt on every message, "+
					"so keeping them short is not a formality. Trim it and try again.", len(content), identityFileWriteCap),
				IsError: true,
			}
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return llm.ToolResult{Output: "Could not create your identity directory: " + err.Error(), IsError: true}
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return llm.ToolResult{Output: "Could not write " + params.File + ": " + err.Error(), IsError: true}
		}

		m.db.LogAudit("agent:"+selfSlug, "agent_identity_file_written", "agent", "agent_role", selfSlug, params.File)
		m.broadcast("agent_files_changed", map[string]interface{}{"slug": selfSlug, "file": params.File})

		result := map[string]interface{}{
			"file":          params.File,
			"written":       true,
			"bytes":         len(content),
			"previous_size": len(existing),
			"applies":       "from your next message — the prompt for this one was already assembled",
		}

		// Writing HEARTBEAT.md is half of making a heartbeat work; the setting is
		// the other half, and an agent that reports only its half leaves the user
		// believing something is running when it is not.
		if params.File == FileHeartbeat {
			var enabled bool
			m.db.QueryRow("SELECT heartbeat_enabled FROM agent_roles WHERE slug = ?", selfSlug).Scan(&enabled)
			switch {
			case strings.TrimSpace(content) == "":
				result["heartbeat_warning"] = "This file is now empty, so your heartbeat will skip every wake-up."
			case !enabled:
				result["heartbeat_warning"] = "Your heartbeat instructions are saved, but your heartbeat setting is OFF so nothing will run. " +
					"Use my_settings_update with heartbeat_enabled=true, and check my_settings_get to confirm the app-wide heartbeat is on too."
			default:
				result["heartbeat_note"] = "Your heartbeat is on and now has instructions. Check my_settings_get to confirm the app-wide heartbeat is also on."
			}
		}

		out, _ := json.Marshal(result)
		return llm.ToolResult{Output: string(out)}
	}
}

// buildIdentityPromptSection tells an agent its defining files are editable, by
// name, and what changing each one actually does.
func buildIdentityPromptSection() string {
	var b strings.Builder
	b.WriteString("## YOUR OWN IDENTITY FILES\n\n")
	b.WriteString("The files that define you are yours to change, through conversation: " +
		"`my_identity_list`, `my_identity_read`, `my_identity_write`.\n\n")
	for _, d := range identityFileDocs {
		fmt.Fprintf(&b, "- **%s** — %s\n", d.Name, d.Purpose)
	}
	b.WriteString("\nWhen the user asks you to change how you behave — your tone, your rules, what you do on startup, " +
		"what you check when you wake up — edit the relevant file rather than agreeing and carrying on unchanged.\n\n")
	b.WriteString("- Read before you write. `my_identity_write` in replace mode overwrites everything.\n")
	b.WriteString("- Show the user what you are about to write, and get agreement first. These change who you are.\n")
	b.WriteString("- Changes apply from your **next** message, not this one.\n")
	b.WriteString("- Your heartbeat needs BOTH instructions in HEARTBEAT.md and the setting switched on. " +
		"Empty file means every wake-up is skipped, no matter what the setting says — so when the user asks you to " +
		"start checking in, do both halves and confirm both.\n")
	return b.String()
}
