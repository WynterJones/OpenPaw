package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/memory"
	"github.com/openpaw/openpaw/internal/tmux"
)

func (m *Manager) GatewayAnalyze(ctx context.Context, userMessage, threadID string, history []ThreadMessage, hints *GatewayRoutingHints) (*GatewayResponse, *llm.UsageInfo, error) {
	// Build dynamic agent list for gateway
	workspaceID := m.threadWorkspaceID(threadID)
	agentList := m.buildAgentList(workspaceID)
	gatewayPrompt := GatewayRoutingPromptFor(m.GatewayName())

	// Inject gateway identity (SOUL, USER, GOAL, memory)
	gatewayIdentity := AssembleGatewayContext(m.DataDir, m.MemoryMgr)
	if gatewayIdentity != "" {
		gatewayPrompt = gatewayIdentity + "\n\n---\n\n" + gatewayPrompt
	}

	// Inject current date/time
	gatewayPrompt += fmt.Sprintf("\n\n## CURRENT TIME\n%s\n", time.Now().Format("Monday, January 2, 2006 at 3:04 PM MST"))

	if agentList != "" {
		gatewayPrompt += "\nAvailable specialist agents (use \"assigned_agent\" field with the slug):\n" + agentList
	}

	// Inject available tools info so gateway knows what tools exist for routing decisions
	if m.ToolMgr != nil {
		toolsSection := m.buildToolsPromptSection("", workspaceID)
		if toolsSection != "" {
			gatewayPrompt += "\n\n## SYSTEM SERVICES (read-only info for routing decisions)\n\n" + toolsSection
			gatewayPrompt += "\nWhen a user's request requires a service (e.g. weather data, API calls), route to an agent that can use the service — do NOT try to answer directly.\n"
		}
	}

	// The gateway holds no studio tools itself — it routes. Without this it
	// would answer "make me a picture of X" conversationally and nothing would
	// ever be generated.
	gatewayPrompt += buildStudioRoutingNote(m.MediaRegistry)

	// Inject existing dashboards so gateway can match update requests
	dashSection := m.buildDashboardsPromptSection(workspaceID)
	if dashSection != "" {
		gatewayPrompt += "\n\n" + dashSection
	}

	// Inject user projects so gateway can resolve project references
	projectsSection := m.buildProjectsPromptSection(workspaceID)
	if projectsSection != "" {
		gatewayPrompt += "\n\n" + projectsSection
	}

	// Inject routing hints so gateway has full context
	if hints != nil {
		gatewayPrompt += "\n\n## ROUTING CONTEXT\n"
		if hints.LastResponder != "" {
			gatewayPrompt += fmt.Sprintf("- **Last responder**: `%s` (the agent who most recently replied in this thread)\n", hints.LastResponder)
		}
		if len(hints.MentionSlugs) > 1 {
			gatewayPrompt += fmt.Sprintf("- **User @mentioned multiple agents**: %s\n", strings.Join(hints.MentionSlugs, ", "))
		} else if hints.MentionSlug != "" {
			gatewayPrompt += fmt.Sprintf("- **User @mentioned**: `%s` (the user explicitly tagged this agent)\n", hints.MentionSlug)
		}
		if len(hints.ThreadMembers) > 0 {
			gatewayPrompt += fmt.Sprintf("- **Thread members**: %s (agents already participating)\n", strings.Join(hints.ThreadMembers, ", "))
		}
	}

	// Build prompt with history context
	var promptBuilder strings.Builder
	promptBuilder.WriteString(gatewayPrompt)

	if len(history) > 0 {
		promptBuilder.WriteString("\n\nPrevious conversation:\n")
		for _, msg := range history {
			label := msg.Role
			if msg.AgentSlug != "" {
				label = fmt.Sprintf("assistant/%s", msg.AgentSlug)
			}
			promptBuilder.WriteString(fmt.Sprintf("[%s]: %s\n", label, msg.Content))
		}
		promptBuilder.WriteString(fmt.Sprintf("\nCurrent message:\n%s", userMessage))
	} else {
		promptBuilder.WriteString(fmt.Sprintf("\n\nUser message:\n%s", userMessage))
	}

	prompt := promptBuilder.String()

	// Inject todo list tools into gateway
	todoSection := buildTodoPromptSection(m.db)
	if todoSection != "" {
		prompt += "\n\n---\n\n" + todoSection
	}

	var outputText string
	var usage *llm.UsageInfo

	if provider := m.Provider(); provider.Name() != llm.ProviderOpenRouter {
		// CLI providers (Claude Code / Codex): a single one-shot call with a
		// strict JSON instruction. No tools — todo tools are an OpenRouter-loop
		// nicety the routing decision doesn't depend on.
		text, u, err := provider.RunOneShot(ctx,
			provider.ResolveModel(m.GatewayModel, llm.ModelHaiku), "",
			prompt+"\n\nRespond with ONLY a single JSON object as specified by the instructions above — no prose, no markdown fences.")
		if err != nil {
			return nil, nil, fmt.Errorf("gateway agent failed: %w", err)
		}
		outputText = text
		usage = u
		if usage == nil {
			usage = &llm.UsageInfo{}
		}
	} else {
		todoTools := BuildTodoToolDefs()
		todoHandlers := MakeTodoToolHandlers(m.db, "pounce", m.broadcast)

		result, err := m.client.RunAgentLoop(ctx, llm.AgentConfig{
			Model:         llm.ResolveModel(m.GatewayModel, llm.ModelHaiku),
			System:        "",
			MaxTurns:      3,
			ExtraTools:    todoTools,
			ExtraHandlers: todoHandlers,
			OnEvent: func(ev StreamEvent) {
				if ev.Type == EventTextDelta && ev.Text != "" {
					m.broadcast("gateway_thinking", map[string]interface{}{
						"thread_id": threadID,
						"text":      ev.Text,
					})
				}
			},
		}, prompt)
		if err != nil {
			return nil, nil, fmt.Errorf("gateway agent failed: %w", err)
		}

		outputText = result.Text
		usage = &llm.UsageInfo{
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
			CostUSD:      result.TotalCostUSD,
		}
	}

	var resp GatewayResponse
	output := strings.TrimSpace(outputText)

	jsonStart := strings.Index(output, "{")
	jsonEnd := strings.LastIndex(output, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		output = output[jsonStart : jsonEnd+1]
	}

	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		resp = GatewayResponse{
			Action:  "respond",
			Message: outputText,
		}
	}

	// Normalize AssignedAgents ↔ AssignedAgent for backward compatibility
	if len(resp.AssignedAgents) == 0 && resp.AssignedAgent != "" {
		resp.AssignedAgents = []string{resp.AssignedAgent}
	}
	if len(resp.AssignedAgents) > 0 && resp.AssignedAgent == "" {
		resp.AssignedAgent = resp.AssignedAgents[0]
	}

	return &resp, usage, nil
}

// ProjectContext carries resolved project info from the gateway to the agent.
type ProjectContext struct {
	ProjectName string `json:"project_name"`
	Directory   string `json:"directory"`
	ToolID      string `json:"tool_id,omitempty"`
}

type GatewayResponse struct {
	Action         string            `json:"action"`
	Message        string            `json:"message"`
	ThreadTitle    string            `json:"thread_title,omitempty"`
	AssignedAgent  string            `json:"assigned_agent,omitempty"`
	AssignedAgents []string          `json:"assigned_agents,omitempty"`
	WorkOrder      *GatewayWorkOrder `json:"work_order,omitempty"`
	MemoryNote     string            `json:"memory_note,omitempty"`
	ProjectContext *ProjectContext   `json:"project_context,omitempty"`
}

type GatewayWorkOrder struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Requirements string `json:"requirements"`
	Type         string `json:"type"`
	ToolID       string `json:"tool_id,omitempty"`
	DashboardID  string `json:"dashboard_id,omitempty"`
}

func (m *Manager) GatewaySummarize(ctx context.Context, workOrderID, builderOutput string) (string, error) {
	wo, err := GetWorkOrder(m.db, workOrderID)
	if err != nil {
		return "", fmt.Errorf("get work order for summary: %w", err)
	}

	output := builderOutput
	if len(output) > 20000 {
		output = output[:10000] + "\n\n... [output truncated] ...\n\n" + output[len(output)-10000:]
	}

	prompt := fmt.Sprintf(BuildSummaryPrompt, wo.Title, wo.Type, wo.Description, output)

	provider := m.Provider()
	text, _, err := provider.RunOneShot(ctx, provider.ResolveModel(m.GatewayModel, llm.ModelHaiku), "", prompt)
	if err != nil {
		return "", fmt.Errorf("gateway summarize failed: %w", err)
	}

	return strings.TrimSpace(text), nil
}

// GatewayOneShot runs a single toolless completion on the gateway's model.
//
// Exposed for background passes that are summarisation rather than agency —
// memory capture and the nightly dreaming consolidation — so they run on the
// one model already configured for that kind of work instead of on whichever
// (often far more expensive) model each individual agent happens to use.
func (m *Manager) GatewayOneShot(ctx context.Context, system, prompt string) (string, error) {
	provider := m.Provider()
	text, _, err := provider.RunOneShot(ctx, provider.ResolveModel(m.GatewayModel, llm.ModelHaiku), system, prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

// RoleChatTools are the in-app FS tools available to agents with identity files.
// These operate in the agent's own identity dir (SOUL/RUNBOOK/memory live there)
// and are intentionally sandboxed to it. Real coding work in the active
// workspace's files dir happens through the CLI providers (Claude Code / Codex),
// whose shelled-out process cwd is set to that workspace dir.
var RoleChatTools = []string{"Read", "Write", "Edit"}

// workspaceExtraDirs returns the absolute paths of every external directory
// attached to a workspace that still exists on disk. Queried directly here
// (rather than via internal/handlers) to avoid an import cycle — handlers
// already imports agents.
func (m *Manager) workspaceExtraDirs(workspaceID string) []string {
	rows, err := m.db.Query("SELECT path FROM workspace_directories WHERE workspace_id = ? ORDER BY created_at ASC", workspaceID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var dirs []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			continue
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			dirs = append(dirs, path)
		}
	}
	return dirs
}

// threadWorkspaceID resolves the workspace a chat thread belongs to, so a
// running agent operates on the thread's own workspace rather than whichever
// workspace happens to be globally active — otherwise two concurrent chats in
// different workspaces would both resolve to the same (active) one. Falls
// back to the default workspace when threadID is empty or unresolvable (e.g.
// scheduled/sub-agent runs that don't have a chat thread).
// workspaceCtxKey carries a workspace for runs that have no chat thread to
// resolve one from. Unexported type so nothing else can collide with the key.
type workspaceCtxKey struct{}

// WithWorkspace tags a context with the workspace a threadless run belongs to.
func WithWorkspace(ctx context.Context, workspaceID string) context.Context {
	return context.WithValue(ctx, workspaceCtxKey{}, workspaceID)
}

func workspaceFromContext(ctx context.Context) string {
	ws, _ := ctx.Value(workspaceCtxKey{}).(string)
	return ws
}

type unattendedCtxKey struct{}

// WithUnattended marks a run that nobody is watching — a scheduled routine
// rather than a chat turn.
//
// Carried on the context for the same reason as the provider pin: the fact has
// to survive down to where tools are assembled, and every intermediate would
// otherwise need a parameter it does not care about. Tools whose failure mode
// compounds without a person present (creating schedules, most of all) consult
// it and offer only their read-only half.
func WithUnattended(ctx context.Context) context.Context {
	return context.WithValue(ctx, unattendedCtxKey{}, true)
}

func isUnattended(ctx context.Context) bool {
	v, _ := ctx.Value(unattendedCtxKey{}).(bool)
	return v
}

type providerCtxKey struct{}

// WithProvider pins a run to a named engine regardless of which one is active.
//
// Carried on the context rather than threaded through every signature because
// the override has to survive the whole call chain — RoleChat, delegation, tool
// handlers — and every intermediate would otherwise need a parameter it does
// not care about.
func WithProvider(ctx context.Context, name string) context.Context {
	if name == "" {
		return ctx
	}
	return context.WithValue(ctx, providerCtxKey{}, name)
}

func providerFromContext(ctx context.Context) string {
	name, _ := ctx.Value(providerCtxKey{}).(string)
	return name
}

// providerFor resolves the engine for a run: the context's pin if it names one
// that is actually usable, otherwise the active engine.
//
// An unusable pin falls back rather than failing. A schedule pinned to Codex
// still has to produce its report on a machine where Codex was uninstalled —
// silently running on the active engine beats an unattended routine that stops
// reporting and never says why.
func (m *Manager) providerFor(ctx context.Context) llm.Provider {
	name := providerFromContext(ctx)
	if name == "" || m.Providers == nil {
		return m.Provider()
	}
	if p := m.Providers.Get(name); p != nil && p.IsConfigured() {
		return p
	}
	logger.Warn("engine %q is not available — running on the active engine instead", name)
	return m.Provider()
}

func (m *Manager) threadWorkspaceID(threadID string) string {
	if threadID == "" {
		return database.DefaultWorkspaceID
	}
	var id string
	if err := m.db.QueryRow("SELECT workspace_id FROM chat_threads WHERE id = ?", threadID).Scan(&id); err != nil || id == "" {
		return database.DefaultWorkspaceID
	}
	return id
}

// buildWorkspacePromptSection tells the agent which workspace it's operating in.
// Resolved per run (from the thread's own workspace) so switching workspaces
// changes what the agent is told and concurrent chats in different workspaces
// don't cross-contaminate. The working-directory guidance is only included for
// CLI providers (Claude Code / Codex), whose process cwd is the workspace files
// dir; for the OpenRouter loop (sandboxed to the identity dir) only the name is
// stated, to avoid implying it writes into the workspace.
func (m *Manager) buildWorkspacePromptSection(providerName, workspaceID string) string {
	id := workspaceID
	var name string
	if err := m.db.QueryRow("SELECT name FROM workspaces WHERE id = ?", id).Scan(&name); err != nil || name == "" {
		name = "Default"
	}

	section := fmt.Sprintf("## Workspace\nYou are working in the %q workspace.", name)
	if providerName != llm.ProviderOpenRouter {
		dir := filepath.Join(m.DataDir, "workspaces", id, "files")
		section += fmt.Sprintf(" Its working directory is `%s` — this is your current working directory, so clone repos, create files, and run commands here. Files you create appear in this workspace's Directory.", dir)

		if extraDirs := m.workspaceExtraDirs(id); len(extraDirs) > 0 {
			section += "\n\nAdditional directories you can access in this workspace:\n"
			for _, d := range extraDirs {
				section += fmt.Sprintf("- %s\n", d)
			}
			section += "You may read and edit files in these directories as needed."
		}
		section += "\n\nKeep all filesystem discovery and commands inside this workspace and its explicitly attached directories. Do not inspect the user's home directory, ~/Library, Music, Photos, or other applications' data unless the user explicitly attaches that directory to this workspace."
	}
	return section
}

func (m *Manager) RoleChat(ctx context.Context, systemPrompt, model string, history []ThreadMessage, userMessage, threadID, agentDir, agentRoleSlug, agentName, avatarDescription, avatarPath string) (string, *llm.UsageInfo, string, string, string, error) {
	// A scheduled run may pin its own engine; everything else gets the active one.
	provider := m.providerFor(ctx)
	resolvedModel := provider.ResolveModel(model, llm.ModelSonnet)

	// Resolve the workspace from the thread being answered, not the global
	// active workspace, so concurrent chats in different workspaces don't
	// cross-contaminate (files dir, tools, attached dirs, CLI cwd).
	wsID := m.threadWorkspaceID(threadID)
	// Threadless runs (scheduled reports) have no thread to resolve from and
	// carry their workspace on the context instead.
	if threadID == "" {
		if ws := workspaceFromContext(ctx); ws != "" {
			wsID = ws
		}
	}

	// Build history messages for multi-turn conversation.
	// Messages from OTHER agents are re-attributed as user-role context so the
	// current agent doesn't mistakenly think it authored them.
	// Each message is prefixed with [msg_id:xxx] so agents can reference them for reactions.
	var historyMsgs []llm.HistoryMessage
	for _, msg := range history {
		role := msg.Role
		content := msg.Content
		isSummary := role == "system"
		if isSummary {
			// Compaction summaries stand in for messages that no longer exist.
			// Labelled so the agent treats them as established history rather
			// than as something it said verbatim.
			role = "assistant"
			content = "[Summary of earlier conversation — the original messages were compacted]:\n" + content
		}
		if role != "user" && role != "assistant" {
			continue
		}
		// Prepend message ID so agents can reference messages for reactions
		if msg.ID != "" && !isSummary {
			content = fmt.Sprintf("[msg_id:%s]\n%s", msg.ID, content)
		}
		// If an assistant message came from a different agent, present it as
		// third-party context so the current agent knows it didn't say this.
		if role == "assistant" && msg.AgentSlug != "" && msg.AgentSlug != agentRoleSlug {
			role = "user"
			content = fmt.Sprintf("[Message from @%s — not you]:\n%s", msg.AgentSlug, content)
		}
		// Merge consecutive same-role messages
		if len(historyMsgs) > 0 && historyMsgs[len(historyMsgs)-1].Role == role {
			historyMsgs[len(historyMsgs)-1].Content += "\n\n" + content
		} else {
			historyMsgs = append(historyMsgs, llm.HistoryMessage{Role: role, Content: content})
		}
	}
	// Ensure history starts with user (API requirement).
	// If it starts with assistant (e.g. heartbeat-initiated thread), convert the
	// leading assistant message to a user-role context block so it isn't dropped.
	if len(historyMsgs) > 0 && historyMsgs[0].Role != "user" {
		historyMsgs[0] = llm.HistoryMessage{
			Role:    "user",
			Content: "[Your previous message in this thread]:\n" + historyMsgs[0].Content,
		}
	}
	// Ensure history ends with assistant (so the new user message can follow)
	if len(historyMsgs) > 0 && historyMsgs[len(historyMsgs)-1].Role != "assistant" {
		historyMsgs = historyMsgs[:len(historyMsgs)-1]
	}

	collector := llm.NewWidgetCollector()

	// Append current date/time to system prompt
	systemPrompt += fmt.Sprintf("\n\nCurrent time: %s", time.Now().Format("Monday, January 2, 2006 at 3:04 PM MST"))

	// Tell the agent which workspace it's in (resolved per run, from the thread).
	systemPrompt += "\n\n---\n\n" + m.buildWorkspacePromptSection(provider.Name(), wsID)

	cfg := llm.AgentConfig{
		Model:   resolvedModel,
		System:  systemPrompt,
		History: historyMsgs,
		OnEvent: func(ev StreamEvent) {
			if ev.Type == EventToolStart {
				collector.TrackToolStart(ev.ToolName, ev.ToolID, ev.ToolInput)
				m.db.LogAudit("system", "agent_tool_call", "tool_call", "agent_role", agentRoleSlug, ev.ToolName)
			}
			if ev.Type == EventToolEnd {
				isError := strings.HasPrefix(ev.ToolOutput, "ERROR")
				collector.TrackToolEnd(ev.ToolID, isError)
				if ev.ToolOutput != "" {
					collector.Collect(ev.ToolName, ev.ToolID, ev.ToolOutput)
				}
			}
			if threadID == "" {
				return
			}
			// Accumulate stream state for recovery when switching threads
			switch ev.Type {
			case EventTextDelta:
				if ev.Text != "" {
					m.UpdateStreamText(threadID, agentRoleSlug, ev.Text)
				}
			case EventToolStart:
				m.UpdateStreamTool(threadID, StreamTool{Name: ev.ToolName, ID: ev.ToolID, Done: false})
			case EventToolEnd:
				m.UpdateStreamTool(threadID, StreamTool{Name: ev.ToolName, ID: ev.ToolID, Done: true})
			}
			m.broadcast("agent_stream", map[string]interface{}{
				"thread_id":       threadID,
				"agent_role_slug": agentRoleSlug,
				"event":           ev,
			})
		},
	}

	if agentDir != "" {
		cfg.Tools = RoleChatTools
		cfg.WorkDir = agentDir
		cfg.SandboxPaths = []string{agentDir}
		cfg.MaxTurns = m.MaxTurns
		if provider.Name() != llm.ProviderOpenRouter {
			cfg.ExtraDirs = m.workspaceExtraDirs(wsID)
			// CLI providers (Claude Code / Codex) shell out with their cwd set
			// to the workspace files dir — resolve it from the thread's own
			// workspace so concurrent chats in different workspaces don't
			// clobber each other's cwd.
			wsDir := filepath.Join(m.DataDir, "workspaces", wsID, "files")
			if err := os.MkdirAll(wsDir, 0755); err == nil {
				cfg.WorkspaceDir = wsDir
			}
		}
	} else {
		cfg.MaxTurns = 1
	}

	// Inject available tools into system prompt and add call_tool capability
	if m.ToolMgr != nil {
		toolsSection := m.buildToolsPromptSection(agentRoleSlug, wsID)
		if toolsSection != "" {
			cfg.System += "\n\n---\n\n" + toolsSection
			cfg.ExtraTools = append(cfg.ExtraTools, llm.BuildCallToolDef())
			cfg.ExtraHandlers = map[string]llm.ToolHandler{
				"call_tool": m.makeCallToolHandler(wsID),
			}

			// Being able to call a service but not fix one made every stopped
			// service the user's problem to go and solve on the Services page.
			cfg.ExtraTools = append(cfg.ExtraTools, BuildServiceControlToolDefs()...)
			for name, handler := range m.MakeServiceControlHandlers(wsID) {
				cfg.ExtraHandlers[name] = handler
			}
			cfg.System += "\n\n---\n\n" + buildServiceControlPromptSection()
		}
	}

	// tmux tools: an agent's turn ends when it replies, so "I'll check back on
	// that build" is only true if it can hand the checking to something that
	// outlives the turn. tmux_watch does exactly that.
	if tmux.Available() {
		cfg.ExtraTools = append(cfg.ExtraTools, BuildTmuxToolDefs()...)
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		for name, handler := range m.MakeTmuxToolHandlers(threadID) {
			cfg.ExtraHandlers[name] = handler
		}
		// Only the CLI engines can actually build — they have a real shell in
		// the workspace. Telling an OpenRouter agent to prefer tmux would just
		// describe a workflow it has no way to start.
		if provider.Name() != llm.ProviderOpenRouter {
			cfg.System += "\n\n---\n\n" + buildTmuxPromptSection()
		}
	}

	// Canvas: show a running dev server or a built page in the preview pane
	// beside the chat, so local work can be looked at without leaving it.
	// Slack-style reply threads intentionally do not get canvas tools: the
	// frontend makes Canvas and the thread panel mutually exclusive, and a
	// child-thread canvas event has no safe main-chat surface to replace.
	isReplyThread := false
	if threadID != "" {
		var parentThreadID string
		if err := m.db.QueryRow("SELECT COALESCE(parent_thread_id, '') FROM chat_threads WHERE id = ?", threadID).Scan(&parentThreadID); err == nil {
			isReplyThread = parentThreadID != ""
		}
	}
	if threadID != "" && !isReplyThread {
		cfg.ExtraTools = append(cfg.ExtraTools, BuildCanvasToolDefs()...)
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		for name, handler := range m.MakeCanvasToolHandlers(threadID, wsID, agentRoleSlug) {
			cfg.ExtraHandlers[name] = handler
		}
		cfg.System += "\n\n---\n\n" + buildCanvasPromptSection()
	}

	// Building is collaborative: the agent that gathered the spec files the work
	// order itself. Without this an agent mid-conversation could only answer
	// "I can't build that, ask the Gateway" — with all the context in hand.
	if threadID != "" && m.BuildRequestFn != nil {
		cfg.ExtraTools = append(cfg.ExtraTools, BuildRequestToolDefs()...)
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		for name, handler := range m.MakeBuildRequestHandler(threadID) {
			cfg.ExtraHandlers[name] = handler
		}
		cfg.System += "\n\n---\n\n" + buildRequestPromptSection()
	}

	// Inject memory tools so agents can save/search memories across conversations
	if m.MemoryMgr != nil {
		m.MemoryMgr.EnsureMigrated(agentRoleSlug)
		cfg.ExtraTools = append(cfg.ExtraTools, memory.BuildMemoryToolDefs()...)
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		for name, handler := range m.MemoryMgr.MakeMemoryHandlers(agentRoleSlug) {
			cfg.ExtraHandlers[name] = handler
		}
	}

	// Schedules: the conversation that works out a routine should be the one
	// that files it, rather than ending on directions to the Scheduler page.
	if m.Scheduler != nil {
		canSchedule := !isUnattended(ctx)
		cfg.ExtraTools = append(cfg.ExtraTools, BuildScheduleToolDefs(canSchedule)...)
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		for name, handler := range m.MakeScheduleToolHandlers(agentRoleSlug, threadID, canSchedule) {
			cfg.ExtraHandlers[name] = handler
		}
		cfg.System += "\n\n---\n\n" + m.buildSchedulePromptSection(canSchedule)
	}

	// Self-configuration — chiefly the heartbeat, which is the setting users ask
	// an agent to change in words ("check in every hour") far more often than
	// they go looking for it on the Agents page.
	if agentRoleSlug != "" {
		cfg.ExtraTools = append(cfg.ExtraTools, BuildAgentSettingsToolDefs()...)
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		for name, handler := range m.MakeAgentSettingsToolHandlers(agentRoleSlug) {
			cfg.ExtraHandlers[name] = handler
		}
		cfg.System += "\n\n---\n\n" + buildAgentSettingsPromptSection()
	}

	// The identity files themselves — SOUL, RUNBOOK, BOOT, USER, HEARTBEAT.
	//
	// Gated on agentDir because only identity-initialized agents have these
	// files; for the rest the whole personality is a system_prompt column and
	// AssembleSystemPrompt is never called, so writing the files would produce
	// something nothing reads.
	if agentRoleSlug != "" && agentDir != "" {
		cfg.ExtraTools = append(cfg.ExtraTools, BuildIdentityToolDefs()...)
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		for name, handler := range m.MakeIdentityToolHandlers(agentRoleSlug) {
			cfg.ExtraHandlers[name] = handler
		}
		cfg.System += "\n\n---\n\n" + buildIdentityPromptSection()
	}

	// Inject todo list tools
	todoSection := buildTodoPromptSection(m.db)
	if todoSection != "" {
		cfg.System += "\n\n---\n\n" + todoSection
	}
	cfg.ExtraTools = append(cfg.ExtraTools, BuildTodoToolDefs()...)
	if cfg.ExtraHandlers == nil {
		cfg.ExtraHandlers = map[string]llm.ToolHandler{}
	}
	for name, handler := range MakeTodoToolHandlers(m.db, agentRoleSlug, m.broadcast) {
		cfg.ExtraHandlers[name] = handler
	}

	// Secret names so an agent can answer "is that key set yet?" itself instead
	// of asking the user to go and look, plus get_secret for the cases where it
	// needs the value to do the work.
	cfg.System += "\n\n---\n\n" + buildSecretsPromptSection(m.db, m.SecretsMgr)
	cfg.ExtraTools = append(cfg.ExtraTools, BuildSecretToolDefs(m.SecretsMgr)...)
	for name, handler := range MakeSecretToolHandlers(m.db, m.SecretsMgr, agentRoleSlug) {
		cfg.ExtraHandlers[name] = handler
	}

	// Inject context-document tools so agents can create/update knowledge docs
	cfg.System += "\n\n---\n\n" + buildContextPromptSection(m.db, wsID)
	cfg.ExtraTools = append(cfg.ExtraTools, BuildContextToolDefs()...)
	if cfg.ExtraHandlers == nil {
		cfg.ExtraHandlers = map[string]llm.ToolHandler{}
	}
	for name, handler := range MakeContextToolHandlers(m.db, m.DataDir, wsID, agentRoleSlug, m.broadcast) {
		cfg.ExtraHandlers[name] = handler
	}

	// Workspace databases provide durable structured data for interactive chats
	// and unattended schedules alike. Scope by this run's workspace rather than
	// the globally active one so concurrent runs never cross-contaminate data.
	cfg.System += "\n\n---\n\n" + buildDatabasesPromptSection(m.db, wsID)
	cfg.ExtraTools = append(cfg.ExtraTools, BuildDatabaseToolDefs()...)
	for name, handler := range MakeDatabaseToolHandlers(m.db, wsID, agentRoleSlug, m.broadcast) {
		cfg.ExtraHandlers[name] = handler
	}

	// Inbox reports are workspace knowledge too: agents can mine recent reports,
	// save or update posts, and archive processed items without sending the user
	// back to the Inbox UI.
	cfg.System += "\n\n---\n\n" + buildInboxPromptSection()
	cfg.ExtraTools = append(cfg.ExtraTools, BuildInboxToolDefs()...)
	for name, handler := range MakeInboxToolHandlers(m.db, wsID, agentRoleSlug, m.broadcast) {
		cfg.ExtraHandlers[name] = handler
	}

	// Studio tools: browse the media library and generate into it. Separate
	// from generate_image below, which is the older single-shot image path —
	// these add folders, video and audio, and providers beyond OpenRouter.
	if studioDefs := BuildStudioToolDefs(m.MediaRegistry); len(studioDefs) > 0 {
		cfg.ExtraTools = append(cfg.ExtraTools, studioDefs...)
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		for name, handler := range m.MakeStudioToolHandlers(threadID) {
			cfg.ExtraHandlers[name] = handler
		}
		cfg.System += "\n\n---\n\n" + buildStudioPromptSection(m.MediaRegistry)
	}

	// Inject image generation tool (OpenRouter with model fallback chain)
	if m.client != nil && m.client.IsConfigured() {
		cfg.ExtraTools = append(cfg.ExtraTools, BuildGenerateImageDef())
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		cfg.ExtraHandlers["generate_image"] = m.makeGenerateImageHandler(provider.Name(), threadID)
		imageNote := "\n\n## IMAGE GENERATION\nYou can generate images using the `generate_image` tool. It uses OpenRouter with automatic model fallback. The tool supports an `images` parameter — pass local image URLs as visual references.\n"
		cfg.System += imageNote
		if avatarDescription != "" || avatarPath != "" {
			cfg.System += fmt.Sprintf("\n## YOUR VISUAL IDENTITY\nYour name is %s.", agentName)
			if avatarDescription != "" {
				cfg.System += " " + avatarDescription
			}
			if avatarPath != "" {
				cfg.System += fmt.Sprintf("\nYour avatar image is available at: %s", avatarPath)
				cfg.System += "\nWhen asked to create images of yourself, use the generate_image tool with your avatar URL in the images array for visual reference, combined with your description in the prompt."
			} else if avatarDescription != "" {
				cfg.System += "\nWhen asked to create images of yourself, incorporate this description into your image prompt."
			}
			cfg.System += "\n"
		}
	}

	// Inject avatar self-update tool so agents can change their own avatar
	cfg.ExtraTools = append(cfg.ExtraTools, BuildAvatarToolDef())
	for name, handler := range MakeAvatarToolHandler(m.db, m.DataDir, agentRoleSlug, m.broadcast) {
		cfg.ExtraHandlers[name] = handler
	}
	cfg.System += "\n\n" + buildAvatarPromptSection(m.db, agentName)

	// Inject delegate_task if other agents are available for delegation
	availableAgents := m.getAvailableAgentsForDelegation(agentRoleSlug, wsID)
	if len(availableAgents) > 0 {
		cfg.ExtraTools = append(cfg.ExtraTools, llm.BuildDelegateTaskDef())
		if cfg.ExtraHandlers == nil {
			cfg.ExtraHandlers = map[string]llm.ToolHandler{}
		}
		cfg.ExtraHandlers["delegate_task"] = m.makeDelegateTaskHandler(threadID, agentRoleSlug)
		cfg.System += "\n\n" + buildDelegationPromptSection(availableAgents)
	}

	// Inject reaction tools so agents can react to messages with emoji
	cfg.ExtraTools = append(cfg.ExtraTools, BuildReactionToolDefs()...)
	if cfg.ExtraHandlers == nil {
		cfg.ExtraHandlers = map[string]llm.ToolHandler{}
	}
	for name, handler := range MakeReactionToolHandlers(m.db, agentRoleSlug, m.broadcast) {
		cfg.ExtraHandlers[name] = handler
	}
	cfg.System += "\n\n" + buildReactionPromptSection()

	// Enable native session resume for CLI providers (per thread + agent)
	if threadID != "" && agentRoleSlug != "" {
		cfg.Session = &llm.SessionKey{ThreadID: threadID, AgentSlug: agentRoleSlug}
	}

	result, err := provider.RunAgentLoop(ctx, cfg, userMessage)
	if err != nil {
		return "", nil, "", "", "", fmt.Errorf("role chat failed: %w", err)
	}

	responseText := strings.TrimSpace(result.Text)

	// If the agent hit max turns or timed out while still working, append a notice
	if result.StopReason == "max_turns" {
		responseText += "\n\n---\n*I hit my turn limit before finishing. Say **continue** and I'll pick up where I left off.*"
	}
	if result.StopReason == "cancelled" && ctx.Err() == context.DeadlineExceeded {
		responseText += "\n\n---\n*I hit my time limit before finishing. Say **continue** and I'll pick up where I left off.*"
		// Return partial result without error so it gets saved
		usage := &llm.UsageInfo{
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
			CostUSD:      result.TotalCostUSD,
		}
		return responseText, usage, collector.JSON(), collector.ToolCallsJSON(), result.ImageURL, nil
	}

	usage := &llm.UsageInfo{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		CostUSD:      result.TotalCostUSD,
	}

	m.db.LogAudit("system", "agent_response", "agent", "agent_role", agentRoleSlug,
		fmt.Sprintf("%s tokens=%d+%d cost=$%.4f", agentRoleSlug, result.InputTokens, result.OutputTokens, result.TotalCostUSD))

	return responseText, usage, collector.JSON(), collector.ToolCallsJSON(), result.ImageURL, nil
}

// scheduledReportDirective tells the agent it is writing a report, not holding
// a conversation.
//
// A scheduled run has no one on the other end at the moment it happens. Without
// this, agents end reports the way they end chat turns — "want me to set up a
// recurring check?", "let me know which you'd prefer" — questions that are
// never answered, and in the threadless case cannot be, because the run
// produces a report rather than a conversation. It also stops agents proposing
// to schedule the very thing that is already scheduled.
func scheduledReportDirective(threadless bool) string {
	delivery := "This report is being posted into an existing chat thread."
	if threadless {
		delivery = "This report is filed in the user's Inbox. There is no conversation attached to it, " +
			"and any question you ask will go unanswered."
	}

	return "\n\n## THIS IS A SCHEDULED RUN\n\n" +
		"You were triggered by a schedule, not by a person. Nobody is waiting at a keyboard. " +
		delivery + "\n\n" +
		"Write a self-contained report:\n" +
		"- Do the work now with the tools and skills you have. Do not ask for permission or clarification first.\n" +
		"- Do not end with a question, an offer, or a call to action. No \"want me to…\", " +
		"\"should I…\", \"let me know if…\".\n" +
		"- Do not propose setting up a recurring check or a schedule — this run is already that schedule.\n" +
		"- If something blocked you, say plainly what it was and what you managed anyway. " +
		"That is the finding; report it rather than asking how to proceed.\n" +
		"- Lead with what matters. Assume the reader is skimming and may not remember the request.\n" +
		"- Use markdown. Keep it as short as the content allows.\n"
}

// SendScheduledPrompt sends a prompt to an agent role and returns the response.
// If threadID is provided, the message is persisted to that chat thread.
// If threadID is empty, a new thread is created.
func (m *Manager) SendScheduledPrompt(ctx context.Context, agentSlug, prompt, threadID, workspaceID, provider string) (response, usedThreadID string, err error) {
	// Pin the engine for this whole run, if the schedule chose one.
	ctx = WithProvider(ctx, provider)
	// Nobody is watching this one, so tools that compound badly unattended —
	// creating further schedules above all — drop to read-only.
	ctx = WithUnattended(ctx)

	var systemPrompt, model, agentProvider, agentName, avatarDescription, avatarPath, remoteProvider, remoteAgentID string
	var identityInitialized bool
	err = m.db.QueryRow(
		"SELECT system_prompt, model, provider, identity_initialized, name, avatar_description, avatar_path, remote_provider, remote_agent_id FROM agent_roles WHERE slug = ? AND enabled = 1",
		agentSlug,
	).Scan(&systemPrompt, &model, &agentProvider, &identityInitialized, &agentName, &avatarDescription, &avatarPath, &remoteProvider, &remoteAgentID)
	if err != nil {
		return "", "", fmt.Errorf("agent role %q not found or disabled: %w", agentSlug, err)
	}
	// A schedule-level engine is an explicit one-run override. Otherwise the
	// selected agent keeps its own engine, just as it does in interactive chat.
	if provider == "" {
		ctx = WithProvider(ctx, agentProvider)
	}

	now := time.Now().UTC()

	if workspaceID == "" {
		workspaceID = m.db.ActiveWorkspaceID()
	}

	// A schedule pinned to a thread keeps writing there — the user picked that
	// conversation deliberately. An unpinned schedule runs *threadless*: it used
	// to spawn a new chat on every tick, burying the chat list under runs nobody
	// asked to see. The report is filed in the Inbox instead, and a thread is
	// created only if the user opens it as a chat.
	var history []ThreadMessage
	if threadID != "" {
		userMsgID := uuid.New().String()
		m.db.Exec(
			"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, created_at) VALUES (?, ?, 'user', ?, ?, ?)",
			userMsgID, threadID, prompt, agentSlug, now,
		)
		m.db.Exec("UPDATE chat_threads SET updated_at = ? WHERE id = ?", now, threadID)

		rows, err := m.db.Query(
			"SELECT role, content FROM chat_messages WHERE thread_id = ? ORDER BY created_at ASC",
			threadID,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var tm ThreadMessage
				if rows.Scan(&tm.Role, &tm.Content) == nil {
					history = append(history, tm)
				}
			}
		}
	} else {
		// Without a thread there is nothing to resolve the workspace from, so
		// carry the schedule's workspace explicitly — otherwise the run would
		// silently operate on the default workspace's files and tools.
		ctx = WithWorkspace(ctx, workspaceID)
	}

	// If identity system is initialized, assemble prompt from files and set agentDir
	// so the agent gets tools and full max turns (same as normal chat).
	var agentDir string
	if identityInitialized {
		assembled, err := AssembleSystemPrompt(m.DataDir, agentSlug)
		if err == nil {
			systemPrompt = assembled
			agentDir = AgentDir(m.DataDir, agentSlug)
		}
	}

	systemPrompt += scheduledReportDirective(threadID == "")

	var result, widgetJSON, toolCallsJSON, imageURL string
	var usage *llm.UsageInfo
	var chatErr error
	if remoteProvider == openClawProvider {
		// Remote OpenClaw agent — proxy the turn; the remote side owns context
		result, chatErr = m.OpenClawChat(ctx, remoteAgentID, threadID, prompt)
	} else {
		result, usage, widgetJSON, toolCallsJSON, imageURL, chatErr = m.RoleChat(ctx, systemPrompt, model, history, prompt, threadID, agentDir, agentSlug, agentName, avatarDescription, avatarPath)
	}
	if chatErr != nil {
		return "", threadID, fmt.Errorf("scheduled prompt failed: %w", chatErr)
	}

	// Threadless run — the caller files the response as a report.
	if threadID == "" {
		return result, "", nil
	}

	// Save the assistant response to the thread
	assistMsgID := uuid.New().String()
	assistNow := time.Now().UTC()
	var costUSD float64
	var inputTokens, outputTokens int64
	if usage != nil {
		costUSD = usage.CostUSD
		inputTokens = usage.InputTokens
		outputTokens = usage.OutputTokens
	}
	var imgPtr *string
	if imageURL != "" {
		imgPtr = &imageURL
	}
	var wdPtr *string
	if widgetJSON != "" {
		wdPtr = &widgetJSON
	}
	var tcPtr *string
	if toolCallsJSON != "" {
		tcPtr = &toolCallsJSON
	}
	m.db.Exec(
		"INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, widget_data, image_url, tool_calls_json, created_at) VALUES (?, ?, 'assistant', ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		assistMsgID, threadID, result, agentSlug, costUSD, inputTokens, outputTokens, wdPtr, imgPtr, tcPtr, assistNow,
	)
	m.db.Exec("UPDATE chat_threads SET updated_at = ? WHERE id = ?", assistNow, threadID)

	return result, threadID, nil
}
