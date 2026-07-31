package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/mcp"
)

// Codex model IDs per canonical tier. These match the current Codex CLI
// catalog: Luna is optimized for speed, Terra for everyday balance, and Sol
// for the most capable coding work.
var codexModels = map[string]string{
	"haiku":  "gpt-5.6-luna",
	"sonnet": "gpt-5.6-terra",
	"opus":   "gpt-5.6-sol",
	"fable":  "gpt-5.6-sol",
}

// CodexProvider runs inference through the OpenAI Codex CLI in headless mode
// (`codex exec --json`), using the user's ChatGPT subscription auth.
type CodexProvider struct {
	binName    string // "codex"; overridable for tests
	store      llm.SessionStore
	registry   *mcp.Registry
	mcpBaseURL string
	sem        chan struct{}
	probe      probeState
	// workspaceDir resolves the active workspace's real files dir at exec time
	// so the shelled-out CLI runs with its cwd there. Nil = fall back to cfg.WorkDir.
	workspaceDir func() string
}

func NewCodexProvider(store llm.SessionStore, registry *mcp.Registry) *CodexProvider {
	return &CodexProvider{
		binName:  "codex",
		store:    store,
		registry: registry,
		sem:      make(chan struct{}, maxConcurrentCLI),
	}
}

func (p *CodexProvider) SetMCPBaseURL(url string) { p.mcpBaseURL = url }

// SetWorkspaceDirFunc injects a resolver for the active workspace's files dir.
func (p *CodexProvider) SetWorkspaceDirFunc(fn func() string) { p.workspaceDir = fn }

// resolveWorkDir picks the cwd for the shelled-out CLI, preferring the
// thread's own workspace dir (threadWorkspaceDir, created if missing) so
// concurrent chats in different workspaces don't share a cwd; then the
// global active-workspace resolver; then cfg.WorkDir (fallback).
func (p *CodexProvider) resolveWorkDir(threadWorkspaceDir, fallback string) string {
	if threadWorkspaceDir != "" {
		if err := os.MkdirAll(threadWorkspaceDir, 0755); err != nil {
			logger.Warn("codex: failed to create workspace dir %s: %v", threadWorkspaceDir, err)
		}
		return threadWorkspaceDir
	}
	if p.workspaceDir != nil {
		if dir := p.workspaceDir(); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				logger.Warn("codex: failed to create workspace dir %s: %v", dir, err)
			}
			return dir
		}
	}
	return fallback
}

func (p *CodexProvider) Name() string { return llm.ProviderCodex }

func (p *CodexProvider) IsConfigured() bool {
	ok, _, _ := p.probe.probe(p.binName, codexLoginCheck)
	return ok
}

// StatusInfo returns probe details for the settings UI.
func (p *CodexProvider) StatusInfo() map[string]interface{} {
	available, version, path := p.probe.probe(p.binName, codexLoginCheck)
	return map[string]interface{}{
		"available": available,
		"version":   version,
		"path":      path,
		"logged_in": p.probe.isLoggedIn(),
	}
}

func codexLoginCheck(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	return exec.CommandContext(ctx, path, "login", "status").Run() == nil
}

func (p *CodexProvider) ResolveModel(name, fallback string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	// Pass through anything already codex/GPT-shaped
	if strings.Contains(n, "codex") || strings.HasPrefix(n, "gpt-") || strings.HasPrefix(n, "o3") || strings.HasPrefix(n, "o4") {
		return name
	}
	return codexModels[llm.TierForModel(name, fallback)]
}

func (p *CodexProvider) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{
		{ID: "gpt-5.6-sol", Name: "GPT-5.6 Sol (frontier)"},
		{ID: "gpt-5.6-terra", Name: "GPT-5.6 Terra (balanced)"},
		{ID: "gpt-5.6-luna", Name: "GPT-5.6 Luna (fast)"},
		{ID: "gpt-5.5", Name: "GPT-5.5"},
		{ID: "gpt-5.4", Name: "GPT-5.4"},
		{ID: "gpt-5.4-mini", Name: "GPT-5.4 Mini (fast)"},
	}, nil
}

func (p *CodexProvider) RunAgentLoop(ctx context.Context, cfg llm.AgentConfig, userMessage string) (*llm.AgentResult, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Codex CLI not found — install it and run `codex login` to use your ChatGPT subscription")
	}
	if err := acquireSem(ctx, p.sem); err != nil {
		return nil, err
	}
	defer func() { <-p.sem }()

	var resumeID string
	if cfg.Session != nil && p.store != nil {
		storedID := p.store.GetProviderSession(cfg.Session.ThreadID, cfg.Session.AgentSlug, p.Name())
		if len(cfg.ExtraHandlers) == 0 {
			resumeID = storedID
		} else if storedID != "" {
			// A resumed Codex thread keeps the MCP binding from its original
			// turn, while OpenPaw bridge credentials are deliberately
			// single-run. Codex can list tools from the replacement bridge but
			// executes them against the expired binding, reporting "user
			// cancelled MCP tool call". Start tool-enabled turns fresh and
			// replay OpenPaw history instead.
			p.store.DeleteProviderSession(cfg.Session.ThreadID, cfg.Session.AgentSlug, p.Name())
		}
	}

	result, sessionID, err := p.runOnce(ctx, cfg, userMessage, resumeID)
	if err != nil && resumeID != "" && ctx.Err() == nil {
		logger.Warn("codex resume failed (%v) — retrying with fresh session", err)
		p.store.DeleteProviderSession(cfg.Session.ThreadID, cfg.Session.AgentSlug, p.Name())
		result, sessionID, err = p.runOnce(ctx, cfg, userMessage, "")
	}
	if err != nil {
		return result, err
	}

	if cfg.Session != nil && p.store != nil && sessionID != "" && len(cfg.ExtraHandlers) == 0 {
		p.store.PutProviderSession(cfg.Session.ThreadID, cfg.Session.AgentSlug, p.Name(), sessionID)
	}
	return result, nil
}

func (p *CodexProvider) runOnce(ctx context.Context, cfg llm.AgentConfig, userMessage, resumeID string) (*llm.AgentResult, string, error) {
	var mcpSession *mcp.Session
	var mcpURL string
	if len(cfg.ExtraHandlers) > 0 && p.registry != nil && p.mcpBaseURL != "" {
		mcpSession = p.registry.Create(&mcp.Session{
			AgentSlug: sessionAgentSlug(cfg),
			ThreadID:  sessionThreadID(cfg),
			WorkDir:   cfg.WorkDir,
			Tools:     cfg.ExtraTools,
			Handlers:  cfg.ExtraHandlers,
		})
		defer p.registry.Release(mcpSession.Token)
		mcpURL = p.mcpBaseURL + mcpSession.Token
	}

	// Codex has no system-prompt flag: embed system + history in the prompt.
	prompt := userMessage
	if resumeID == "" {
		var sb strings.Builder
		if cfg.System != "" {
			sb.WriteString("## SYSTEM INSTRUCTIONS (follow these for the whole conversation)\n\n")
			sb.WriteString(cfg.System)
			sb.WriteString("\n\n")
		}
		sb.WriteString(buildReplayPrompt(cfg.History, userMessage))
		prompt = sb.String()
	}

	args := p.buildArgs(cfg, resumeID, mcpURL)

	cmd := exec.CommandContext(ctx, p.binName, args...)
	if dir := p.resolveWorkDir(cfg.WorkspaceDir, cfg.WorkDir); dir != "" {
		cmd.Dir = dir
	}

	emit := func(ev llm.StreamEvent) {
		if cfg.OnEvent != nil {
			cfg.OnEvent(ev)
		}
	}

	var (
		textBuf      strings.Builder
		sessionID    string
		inputTokens  int64
		outputTokens int64
		runFailure   string
		numTurns     int
	)

	onLine := func(line []byte) {
		var ev codexEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return
		}

		switch ev.Type {
		case "thread.started":
			if ev.ThreadID != "" {
				sessionID = ev.ThreadID
			}
			emit(llm.StreamEvent{Type: llm.EventInit, SessionID: ev.ThreadID})
		case "turn.started":
			numTurns++
		case "item.started":
			if ev.Item == nil {
				return
			}
			if name, input := codexToolInfo(ev.Item); name != "" {
				emit(llm.StreamEvent{
					Type:      llm.EventToolStart,
					ToolName:  name,
					ToolID:    ev.Item.ID,
					ToolInput: input,
				})
			}
		case "item.completed":
			if ev.Item == nil {
				return
			}
			switch ev.Item.ItemType() {
			case "agent_message", "assistant_message":
				if ev.Item.Text == "" {
					return
				}
				if textBuf.Len() > 0 {
					textBuf.WriteString("\n\n")
				}
				textBuf.WriteString(ev.Item.Text)
				emit(llm.StreamEvent{Type: llm.EventTextDelta, Text: ev.Item.Text})
			case "reasoning":
				// internal reasoning — not surfaced
			default:
				if name, _ := codexToolInfo(ev.Item); name != "" {
					emit(llm.StreamEvent{
						Type:       llm.EventToolEnd,
						ToolName:   name,
						ToolID:     ev.Item.ID,
						ToolOutput: ev.Item.AggregatedOutput,
					})
				}
			}
		case "turn.completed":
			if ev.Usage != nil {
				inputTokens += ev.Usage.InputTokens + ev.Usage.CachedInputTokens
				outputTokens += ev.Usage.OutputTokens
			}
		case "turn.failed", "error":
			if ev.Error != nil && ev.Error.Message != "" {
				runFailure = ev.Error.Message
			} else if ev.Message != "" {
				runFailure = ev.Message
			} else {
				runFailure = "codex run failed"
			}
		}
	}

	_, runErr := runJSONL(cmd, prompt, onLine)

	text := strings.TrimSpace(textBuf.String())

	if runFailure != "" {
		return nil, "", fmt.Errorf("codex error: %s", runFailure)
	}
	if runErr != nil && text == "" {
		return nil, "", fmt.Errorf("codex run failed: %w", runErr)
	}

	result := &llm.AgentResult{
		Text:         text,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		NumTurns:     numTurns,
		StopReason:   "stop",
	}
	if mcpSession != nil {
		result.ImageURL = mcpSession.ImageURL()
	}

	emit(llm.StreamEvent{
		Type:     llm.EventResult,
		Result:   text,
		NumTurns: numTurns,
		Usage: &llm.ClaudeUsage{
			InputTokens:  int(inputTokens),
			OutputTokens: int(outputTokens),
		},
	})

	return result, sessionID, nil
}

// buildArgs assembles one Codex invocation. Execution policy is passed through
// config overrides instead of `--sandbox`: `codex exec` accepts that flag, but
// `codex exec resume` does not. The old flag made every resumed OpenPaw turn
// fail immediately and silently fall back to a fresh Codex session.
func (p *CodexProvider) buildArgs(cfg llm.AgentConfig, resumeID, mcpURL string) []string {
	args := []string{"exec"}
	if resumeID != "" {
		args = append(args, "resume", resumeID)
	}
	args = append(args, "--json", "--skip-git-repo-check")

	if model := p.ResolveModel(cfg.Model, ""); model != "" {
		args = append(args, "-m", model)
	}

	// Approval is always "never" because this is a headless process: there is
	// no terminal where a user can approve an MCP or shell action.
	scopedProfile := runtime.GOOS == "darwin" && cfg.WorkspaceDir != ""
	if scopedProfile {
		// Permission profiles can deny reads outside workspace roots, unlike the
		// legacy workspace-write sandbox (which still reads most of the disk).
		// Ignore user config so an older sandbox_mode setting cannot silently
		// override this profile and re-open protected app/media directories.
		args = append(args, "--ignore-user-config")
		args = append(args, codexWorkspacePermissionArgs(cfg)...)
	} else {
		sandboxMode := "danger-full-access"
		if len(cfg.SandboxPaths) > 0 {
			sandboxMode = "workspace-write"
		}
		args = append(args, "-c", fmt.Sprintf(`sandbox_mode=%q`, sandboxMode))
		if sandboxMode == "workspace-write" && mcpURL != "" {
			args = append(args, "-c", `sandbox_workspace_write.network_access=true`)
		}
	}
	args = append(args, "-c", `approval_policy="never"`)

	if mcpURL != "" {
		args = append(args,
			"-c", fmt.Sprintf(`mcp_servers.openpaw.url=%q`, mcpURL),
			// OpenPaw is already the user-controlled host application and its
			// tool handlers enforce their own scope. Codex 5.6 otherwise asks
			// for a separate MCP approval for side-effecting tools; with a
			// headless "never" approval policy that becomes an immediate
			// client cancellation before the request reaches the bridge.
			"-c", `mcp_servers.openpaw.default_tools_approval_mode="approve"`,
		)
	}

	// "-" makes codex read the prompt from stdin (avoids ARG_MAX limits).
	return append(args, "-")
}

// codexWorkspacePermissionArgs creates an invocation-local permission profile
// with workspace-only filesystem access. The profile-defined roots supplement
// the current cwd, which Codex already treats as a runtime workspace root.
func codexWorkspacePermissionArgs(cfg llm.AgentConfig) []string {
	roots := mergeUnique([]string{cfg.WorkDir}, cfg.ExtraDirs)
	entries := make([]string, 0, len(roots))
	for _, root := range roots {
		clean := filepath.Clean(root)
		if clean == "." || !filepath.IsAbs(clean) || clean == filepath.Clean(cfg.WorkspaceDir) {
			continue
		}
		entries = append(entries, fmt.Sprintf("%q=true", clean))
	}

	args := []string{
		"-c", `default_permissions="openpaw-workspace"`,
		"-c", `permissions.openpaw-workspace.extends=":workspace"`,
		"-c", `permissions.openpaw-workspace.filesystem={":root"="deny",":minimal"="read"}`,
		"-c", `permissions.openpaw-workspace.network.enabled=true`,
		"-c", `permissions.openpaw-workspace.network.domains={"*"="allow","127.0.0.1"="allow","localhost"="allow"}`,
	}
	if len(entries) > 0 {
		args = append(args,
			"-c", fmt.Sprintf("permissions.openpaw-workspace.workspace_roots={%s}", strings.Join(entries, ",")),
		)
	}
	return args
}

func (p *CodexProvider) RunOneShot(ctx context.Context, model, system, prompt string) (string, *llm.UsageInfo, error) {
	cfg := llm.AgentConfig{Model: model, System: system, MaxTurns: 1}
	result, _, err := p.runOnce(ctx, cfg, prompt, "")
	if err != nil {
		return "", nil, err
	}
	return result.Text, &llm.UsageInfo{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
	}, nil
}

// codexEvent is one JSONL line of `codex exec --json`. Field names are kept
// tolerant: unknown event/item types are ignored.
type codexEvent struct {
	Type     string      `json:"type"`
	ThreadID string      `json:"thread_id"`
	Item     *codexItem  `json:"item"`
	Usage    *codexUsage `json:"usage"`
	Message  string      `json:"message"`
	Error    *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type codexItem struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	LegacyItemType   string `json:"item_type"`
	Text             string `json:"text"`
	Command          string `json:"command"`
	AggregatedOutput string `json:"aggregated_output"`
	Server           string `json:"server"`
	Tool             string `json:"tool"`
	Status           string `json:"status"`
}

func (i *codexItem) ItemType() string {
	if i.Type != "" {
		return i.Type
	}
	return i.LegacyItemType
}

type codexUsage struct {
	InputTokens       int64 `json:"input_tokens"`
	CachedInputTokens int64 `json:"cached_input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
}

// codexToolInfo maps a codex item to a tool name + input for stream events.
// Returns "" for non-tool items (messages, reasoning).
func codexToolInfo(item *codexItem) (string, map[string]interface{}) {
	switch item.ItemType() {
	case "command_execution":
		return "Bash", map[string]interface{}{"command": item.Command}
	case "mcp_tool_call":
		name := item.Tool
		if name == "" {
			name = "mcp_tool"
		}
		return name, nil
	case "file_change":
		return "Edit", nil
	case "web_search":
		return "WebSearch", nil
	}
	return "", nil
}
