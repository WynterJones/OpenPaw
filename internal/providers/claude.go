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

// ClaudeProvider runs inference through the local Claude Code CLI in headless
// mode (`claude -p --output-format stream-json`), using the user's Claude
// subscription auth instead of API billing. OpenPaw tools are exposed to the
// CLI via the MCP bridge.
type ClaudeProvider struct {
	binName    string // "claude"; overridable for tests
	store      llm.SessionStore
	registry   *mcp.Registry
	mcpBaseURL string // e.g. http://127.0.0.1:41295/api/v1/mcp/
	sem        chan struct{}
	probe      probeState
}

func NewClaudeProvider(store llm.SessionStore, registry *mcp.Registry) *ClaudeProvider {
	return &ClaudeProvider{
		binName:  "claude",
		store:    store,
		registry: registry,
		sem:      make(chan struct{}, maxConcurrentCLI),
	}
}

func (p *ClaudeProvider) SetMCPBaseURL(url string) { p.mcpBaseURL = url }

func (p *ClaudeProvider) Name() string { return llm.ProviderClaudeCode }

func (p *ClaudeProvider) IsConfigured() bool {
	ok, _, _ := p.probe.probe(p.binName, nil)
	return ok
}

// StatusInfo returns probe details for the settings UI.
func (p *ClaudeProvider) StatusInfo() map[string]interface{} {
	available, version, path := p.probe.probe(p.binName, nil)
	return map[string]interface{}{
		"available": available,
		"version":   version,
		"path":      path,
		"logged_in": available, // claude has no fast headless auth probe; errors surface per-request
	}
}

func (p *ClaudeProvider) ResolveModel(name, fallback string) string {
	return llm.TierForModel(name, fallback)
}

func (p *ClaudeProvider) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{
		{ID: "haiku", Name: "Claude Haiku (fast)"},
		{ID: "sonnet", Name: "Claude Sonnet (balanced)"},
		{ID: "opus", Name: "Claude Opus (most capable)"},
	}, nil
}

func (p *ClaudeProvider) RunAgentLoop(ctx context.Context, cfg llm.AgentConfig, userMessage string) (*llm.AgentResult, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Claude Code CLI not found — install it and run `claude` once to log in with your subscription")
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
		// Resume failed (session expired/evicted) — fall back to a fresh
		// session with full history replay, which is lossless.
		logger.Warn("claude-code resume failed (%v) — retrying with fresh session", err)
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

func (p *ClaudeProvider) runOnce(ctx context.Context, cfg llm.AgentConfig, userMessage, resumeID string) (*llm.AgentResult, string, error) {
	args := []string{"-p", "--output-format", "stream-json", "--verbose", "--strict-mcp-config"}

	model := llm.TierForModel(cfg.Model, "")
	args = append(args, "--model", model)

	// Note: recent Claude Code versions dropped --max-turns; runs are bounded
	// by the caller's context timeout (AgentTimeout) instead.
	if cfg.System != "" {
		args = append(args, "--system-prompt", cfg.System)
	}

	prompt := userMessage
	if resumeID != "" {
		args = append(args, "--resume", resumeID)
	} else {
		prompt = buildReplayPrompt(cfg.History, userMessage)
	}

	// Expose OpenPaw tools (memory, todos, delegation, image gen, browser...)
	// via the MCP bridge; native Read/Write/Edit/Bash come from the CLI itself.
	var mcpSession *mcp.Session
	allowed := append([]string{}, cfg.Tools...)
	if len(cfg.ExtraHandlers) > 0 && p.registry != nil && p.mcpBaseURL != "" {
		mcpSession = p.registry.Create(&mcp.Session{
			AgentSlug: sessionAgentSlug(cfg),
			ThreadID:  sessionThreadID(cfg),
			WorkDir:   cfg.WorkDir,
			Tools:     cfg.ExtraTools,
			Handlers:  cfg.ExtraHandlers,
		})
		defer p.registry.Release(mcpSession.Token)
		mcpConfig := fmt.Sprintf(`{"mcpServers":{"openpaw":{"type":"http","url":"%s%s"}}}`, p.mcpBaseURL, mcpSession.Token)
		args = append(args, "--mcp-config", mcpConfig)
		allowed = append(allowed, "mcp__openpaw")
	}
	if len(allowed) > 0 {
		args = append(args, "--allowedTools", strings.Join(allowed, ","))
	}

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
		textBuf    strings.Builder
		sessionID  string
		finalRes   *claudeLine
		toolNames  = map[string]string{}
		sawAnyLine bool
	)

	onLine := func(line []byte) {
		var cl claudeLine
		if err := json.Unmarshal(line, &cl); err != nil {
			return
		}
		sawAnyLine = true

		switch cl.Type {
		case "system":
			if cl.Subtype == "init" {
				if cl.SessionID != "" {
					sessionID = cl.SessionID
				}
				emit(llm.StreamEvent{Type: llm.EventInit, SessionID: cl.SessionID})
			}
		case "assistant":
			var msg anthropicMessage
			if err := json.Unmarshal(cl.Message, &msg); err != nil {
				return
			}
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					if block.Text == "" {
						continue
					}
					if textBuf.Len() > 0 {
						textBuf.WriteString("\n\n")
					}
					textBuf.WriteString(block.Text)
					emit(llm.StreamEvent{Type: llm.EventTextDelta, Text: block.Text})
				case "tool_use":
					toolNames[block.ID] = block.Name
					emit(llm.StreamEvent{
						Type:      llm.EventToolStart,
						ToolName:  block.Name,
						ToolID:    block.ID,
						ToolInput: flattenToolInput(block.Input),
					})
				}
			}
		case "user":
			var msg anthropicMessage
			if err := json.Unmarshal(cl.Message, &msg); err != nil {
				return
			}
			for _, block := range msg.Content {
				if block.Type != "tool_result" {
					continue
				}
				output := flattenToolResultContent(block.Content)
				if block.IsError && !strings.HasPrefix(output, "ERROR") {
					output = "ERROR: " + output
				}
				emit(llm.StreamEvent{
					Type:       llm.EventToolEnd,
					ToolName:   toolNames[block.ToolUseID],
					ToolID:     block.ToolUseID,
					ToolOutput: output,
				})
			}
		case "result":
			lineCopy := cl
			finalRes = &lineCopy
			if cl.SessionID != "" {
				sessionID = cl.SessionID
			}
		}
	}

	_, runErr := runJSONL(cmd, prompt, onLine)

	if runErr != nil && finalRes == nil {
		if !sawAnyLine {
			return nil, "", fmt.Errorf("claude-code failed to start: %w", runErr)
		}
		return nil, "", fmt.Errorf("claude-code run failed: %w", runErr)
	}

	text := strings.TrimSpace(textBuf.String())
	result := &llm.AgentResult{Text: text, StopReason: "stop"}

	if finalRes != nil {
		if text == "" {
			result.Text = strings.TrimSpace(finalRes.Result)
		}
		result.NumTurns = finalRes.NumTurns
		if finalRes.Usage != nil {
			result.InputTokens = finalRes.Usage.InputTokens + finalRes.Usage.CacheRead + finalRes.Usage.CacheCreate
			result.OutputTokens = finalRes.Usage.OutputTokens
		}
		switch finalRes.Subtype {
		case "success":
			result.StopReason = "stop"
		case "error_max_turns":
			result.StopReason = "max_turns"
		default:
			if finalRes.IsError {
				return result, sessionID, fmt.Errorf("claude-code error: %s", firstNonEmpty(finalRes.Result, finalRes.Subtype))
			}
		}
	}

	if mcpSession != nil {
		result.ImageURL = mcpSession.ImageURL()
	}

	emit(llm.StreamEvent{
		Type:     llm.EventResult,
		Result:   result.Text,
		NumTurns: result.NumTurns,
		Usage: &llm.ClaudeUsage{
			InputTokens:  int(result.InputTokens),
			OutputTokens: int(result.OutputTokens),
		},
	})

	return result, sessionID, nil
}

func (p *ClaudeProvider) RunOneShot(ctx context.Context, model, system, prompt string) (string, *llm.UsageInfo, error) {
	if !p.IsConfigured() {
		return "", nil, fmt.Errorf("Claude Code CLI not found — install it and run `claude` once to log in with your subscription")
	}
	if err := acquireSem(ctx, p.sem); err != nil {
		return "", nil, err
	}
	defer func() { <-p.sem }()

	args := []string{"-p", "--output-format", "json", "--strict-mcp-config",
		"--model", llm.TierForModel(model, "")}
	if system != "" {
		args = append(args, "--system-prompt", system)
	}

	cmd := exec.CommandContext(ctx, p.binName, args...)

	var finalRes *claudeLine
	_, runErr := runJSONL(cmd, prompt, func(line []byte) {
		var cl claudeLine
		if json.Unmarshal(line, &cl) == nil && cl.Type == "result" {
			lineCopy := cl
			finalRes = &lineCopy
		}
	})
	if runErr != nil && finalRes == nil {
		return "", nil, fmt.Errorf("claude-code one-shot failed: %w", runErr)
	}
	if finalRes == nil {
		return "", nil, fmt.Errorf("claude-code one-shot produced no result")
	}
	if finalRes.IsError {
		return "", nil, fmt.Errorf("claude-code error: %s", firstNonEmpty(finalRes.Result, finalRes.Subtype))
	}

	usage := &llm.UsageInfo{}
	if finalRes.Usage != nil {
		usage.InputTokens = finalRes.Usage.InputTokens + finalRes.Usage.CacheRead + finalRes.Usage.CacheCreate
		usage.OutputTokens = finalRes.Usage.OutputTokens
	}
	return strings.TrimSpace(finalRes.Result), usage, nil
}

// claudeLine is one JSONL line of `claude -p --output-format stream-json`.
// Unknown fields and line types are ignored for forward compatibility.
type claudeLine struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	SessionID string          `json:"session_id"`
	Message   json.RawMessage `json:"message"`
	Result    string          `json:"result"`
	IsError   bool            `json:"is_error"`
	NumTurns  int             `json:"num_turns"`
	Usage     *claudeUsage    `json:"usage"`
}

type claudeUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	CacheRead    int64 `json:"cache_read_input_tokens"`
	CacheCreate  int64 `json:"cache_creation_input_tokens"`
}

type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// flattenToolResultContent handles both string and content-block-array forms
// of a tool_result content field.
func flattenToolResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []anthropicBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}

func sessionAgentSlug(cfg llm.AgentConfig) string {
	if cfg.Session != nil {
		return cfg.Session.AgentSlug
	}
	return ""
}

func sessionThreadID(cfg llm.AgentConfig) string {
	if cfg.Session != nil {
		return cfg.Session.ThreadID
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "unknown error"
}
