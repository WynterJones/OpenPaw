package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/mcp"
)

// Codex model IDs per canonical tier. ChatGPT-subscription accounts only
// support the mainline gpt-5.x models (verified live against codex-cli
// 0.139: gpt-5.4-mini, gpt-5.4, and gpt-5.5 work; -codex variants do not).
var codexModels = map[string]string{
	"haiku":  "gpt-5.4-mini",
	"sonnet": "gpt-5.4",
	"opus":   "gpt-5.5",
	"fable":  "gpt-5.5",
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
		{ID: "gpt-5.4-mini", Name: "GPT-5.4 Mini (fast)"},
		{ID: "gpt-5.4", Name: "GPT-5.4 (balanced)"},
		{ID: "gpt-5.5", Name: "GPT-5.5 (latest, most capable)"},
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
		resumeID = p.store.GetProviderSession(cfg.Session.ThreadID, cfg.Session.AgentSlug, p.Name())
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

	if cfg.Session != nil && p.store != nil && sessionID != "" {
		p.store.PutProviderSession(cfg.Session.ThreadID, cfg.Session.AgentSlug, p.Name(), sessionID)
	}
	return result, nil
}

func (p *CodexProvider) runOnce(ctx context.Context, cfg llm.AgentConfig, userMessage, resumeID string) (*llm.AgentResult, string, error) {
	args := []string{"exec"}
	if resumeID != "" {
		args = append(args, "resume", resumeID)
	}
	args = append(args, "--json", "--skip-git-repo-check")

	if model := p.ResolveModel(cfg.Model, ""); model != "" {
		args = append(args, "-m", model)
	}

	// Mirror the trust level of the native loop: sandboxed agents get
	// workspace-write (cwd-restricted), unsandboxed agents get full access.
	if len(cfg.SandboxPaths) > 0 {
		args = append(args, "--sandbox", "workspace-write")
	} else {
		args = append(args, "--sandbox", "danger-full-access")
	}

	var mcpSession *mcp.Session
	if len(cfg.ExtraHandlers) > 0 && p.registry != nil && p.mcpBaseURL != "" {
		mcpSession = p.registry.Create(&mcp.Session{
			AgentSlug: sessionAgentSlug(cfg),
			ThreadID:  sessionThreadID(cfg),
			WorkDir:   cfg.WorkDir,
			Tools:     cfg.ExtraTools,
			Handlers:  cfg.ExtraHandlers,
		})
		defer p.registry.Release(mcpSession.Token)
		args = append(args, "-c", fmt.Sprintf(`mcp_servers.openpaw.url=%q`, p.mcpBaseURL+mcpSession.Token))
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

	// "-" makes codex read the prompt from stdin (avoids ARG_MAX limits).
	args = append(args, "-")

	cmd := exec.CommandContext(ctx, p.binName, args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
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
