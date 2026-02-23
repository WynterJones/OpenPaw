package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/models"
)

func (h *ChatHandler) loadRolesCache() []struct{ slug, name string } {
	h.roleCache.RLock()
	if time.Now().Before(h.roleCache.expiresAt) {
		roles := h.roleCache.roles
		h.roleCache.RUnlock()
		return roles
	}
	h.roleCache.RUnlock()

	h.roleCache.Lock()
	defer h.roleCache.Unlock()

	// Double-check after acquiring write lock
	if time.Now().Before(h.roleCache.expiresAt) {
		return h.roleCache.roles
	}

	rows, err := h.db.Query("SELECT slug, name FROM agent_roles WHERE enabled = 1 ORDER BY sort_order ASC")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var roles []struct{ slug, name string }
	for rows.Next() {
		var r struct{ slug, name string }
		if err := rows.Scan(&r.slug, &r.name); err != nil {
			logger.Warn("scan role cache row: %v", err)
			continue
		}
		roles = append(roles, r)
	}

	h.roleCache.roles = roles
	h.roleCache.expiresAt = time.Now().Add(30 * time.Second)
	return roles
}

var mentionRegex = regexp.MustCompile(`@([a-z0-9]+(?:-[a-z0-9]+)*)`)

func (h *ChatHandler) extractMention(content string) string {
	matches := mentionRegex.FindStringSubmatch(content)
	if len(matches) < 2 {
		return ""
	}
	slug := matches[1]

	// Verify the slug matches a known enabled agent
	roles := h.loadRolesCache()
	for _, r := range roles {
		if r.slug == slug {
			return slug
		}
	}
	return ""
}

func (h *ChatHandler) setThreadTitle(threadID, title string) {
	title = truncateStr(strings.TrimSpace(title), maxThreadTitleLength, false)
	now := time.Now().UTC()
	if _, err := h.db.Exec("UPDATE chat_threads SET title = ?, updated_at = ? WHERE id = ?", title, now, threadID); err != nil {
		logger.Error("Failed to set thread title: %v", err)
	}
	h.agentManager.Broadcast("thread_updated", models.WSThreadUpdated{
		ThreadID: threadID,
		Title:    title,
	})
}

func (h *ChatHandler) broadcastStatus(threadID, status, message string) {
	h.agentManager.Broadcast("agent_status", models.WSAgentStatus{
		ThreadID: threadID,
		Status:   status,
		Message:  message,
	})
}

func (h *ChatHandler) broadcastRoutingIndicator(threadID, agentSlug string) {
	var name string
	h.db.QueryRow("SELECT name FROM agent_roles WHERE slug = ?", agentSlug).Scan(&name)
	if name == "" {
		name = agentSlug
	}
	h.agentManager.Broadcast("agent_status", models.WSAgentStatus{
		ThreadID:      threadID,
		Status:        "routing",
		Message:       fmt.Sprintf("Routing to @%s...", name),
		AgentRoleSlug: agentSlug,
	})
}

// beginAgentWork broadcasts the routing indicator followed by "thinking" status.
// Used when handing off to a specialist agent for conversation.
func (h *ChatHandler) beginAgentWork(threadID, agentSlug string) {
	h.broadcastRoutingIndicator(threadID, agentSlug)
	h.broadcastStatus(threadID, "thinking", "Thinking...")
}

// endAgentWork broadcasts the "message_saved" and "done" status sequence.
// Used after saving a direct response (non-streaming) to signal the frontend.
func (h *ChatHandler) endAgentWork(threadID string) {
	h.broadcastStatus(threadID, "message_saved", "")
	h.broadcastStatus(threadID, "done", "")
}

func (h *ChatHandler) handleAgentRouting(threadID, content, userID, agentRoleSlug string, isFirstMsg bool) {
	// Create a cancellable parent context for the entire routing lifecycle
	parentCtx, parentCancel := context.WithCancel(context.Background())
	h.threadCancels.Store(threadID, parentCancel)
	defer func() {
		parentCancel()
		h.threadCancels.Delete(threadID)
	}()

	// Priority 1: Explicit agent selection from UI dropdown (not gateway or empty)
	if agentRoleSlug != "" && agentRoleSlug != "gateway" {
		if isFirstMsg {
			go h.generateThreadTitle(threadID, content)
		}
		h.db.LogAudit(userID, "routing_explicit_agent", "agent", "agent_role", agentRoleSlug, agentRoleSlug)
		h.addThreadMember(threadID, agentRoleSlug)
		h.beginAgentWork(threadID, agentRoleSlug)
		roleChatCtx, roleChatCancel := context.WithTimeout(parentCtx, h.agentManager.AgentTimeout())
		defer roleChatCancel()
		h.handleRoleChatWithDepth(roleChatCtx, threadID, content, agentRoleSlug, 0)
		return
	}

	// Priority 2: Check for bootstrap mode (first-time onboarding)
	if agents.GatewayHasBootstrap(h.dataDir) {
		h.broadcastStatus(threadID, "analyzing", "Setting up...")
		history := h.fetchThreadHistory(threadID)
		gatewayCtx, gatewayCancel := context.WithTimeout(parentCtx, h.agentManager.AgentTimeout())
		defer gatewayCancel()
		resp, usage, err := h.agentManager.GatewayAnalyzeBootstrap(gatewayCtx, content, threadID, history)
		if err != nil {
			if parentCtx.Err() != nil {
				return
			}
			h.saveAssistantMessage(threadID, "", "I'm sorry, I encountered an error during setup: "+err.Error(), 0, 0, 0)
			h.broadcastStatus(threadID, "done", "")
			return
		}

		var gatewayCostUSD float64
		var gatewayInTok, gatewayOutTok int
		if usage != nil {
			gatewayCostUSD = usage.CostUSD
			gatewayInTok = int(usage.InputTokens)
			gatewayOutTok = int(usage.OutputTokens)
		}

		if isFirstMsg && resp.ThreadTitle != "" {
			h.setThreadTitle(threadID, resp.ThreadTitle)
		}

		if resp.Action == "bootstrap_complete" && resp.WorkOrder != nil {
			h.handleBootstrapComplete(threadID, resp, gatewayCostUSD, gatewayInTok, gatewayOutTok)
		} else if resp.Action == "guide" || (resp.WorkOrder == nil && resp.AssignedAgent == "") {
			// Continue bootstrap onboarding conversation
			h.addThreadMember(threadID, "pounce")
			h.broadcastRoutingIndicator(threadID, "pounce")
			msg := resp.Message
			if msg == "" {
				msg = "Welcome! I'm getting set up. Tell me a bit about yourself!"
			}
			h.saveAssistantMessage(threadID, "pounce", msg, gatewayCostUSD, gatewayInTok, gatewayOutTok)
			h.endAgentWork(threadID)
		} else {
			// User made a functional request (build, route, etc.) during bootstrap —
			// auto-exit bootstrap mode and handle normally.
			agents.DeleteGatewayBootstrap(h.dataDir)
			h.handleGatewayAction(parentCtx, threadID, content, userID, resp, gatewayCostUSD, gatewayInTok, gatewayOutTok)
		}
		return
	}

	// Priority 3: ALL other messages go through the gateway with routing hints
	h.broadcastStatus(threadID, "analyzing", "Analyzing your request...")

	// Build routing hints for the gateway
	hints := &agents.GatewayRoutingHints{
		LastResponder: h.getLastResponder(threadID),
		MentionSlug:   h.extractMention(content),
		ThreadMembers: h.getThreadMemberSlugs(threadID),
	}

	// Fetch recent thread history for multi-turn context
	history := h.fetchThreadHistory(threadID)

	gatewayCtx, gatewayCancel := context.WithTimeout(parentCtx, h.agentManager.AgentTimeout())
	defer gatewayCancel()

	resp, usage, err := h.agentManager.GatewayAnalyze(gatewayCtx, content, threadID, history, hints)
	if err != nil {
		if parentCtx.Err() != nil {
			return // stopped by user
		}
		h.saveAssistantMessage(threadID, "", "I'm sorry, I encountered an error processing your request: "+err.Error(), 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	// Auto-set thread title from gateway response on first message
	if isFirstMsg {
		if resp.ThreadTitle != "" {
			h.setThreadTitle(threadID, resp.ThreadTitle)
		} else {
			go h.generateThreadTitle(threadID, content)
		}
	}

	// Track gateway cost but don't attribute it to gateway message
	var gatewayCostUSD float64
	var gatewayInTok, gatewayOutTok int
	if usage != nil {
		gatewayCostUSD = usage.CostUSD
		gatewayInTok = int(usage.InputTokens)
		gatewayOutTok = int(usage.OutputTokens)
	}

	h.handleGatewayAction(parentCtx, threadID, content, userID, resp, gatewayCostUSD, gatewayInTok, gatewayOutTok)
}

// handleGatewayAction processes a parsed gateway response — routing to agents,
// spawning builders, or delivering guide/fallback messages.
func (h *ChatHandler) handleGatewayAction(parentCtx context.Context, threadID, content, userID string, resp *agents.GatewayResponse, gatewayCostUSD float64, gatewayInTok, gatewayOutTok int) {
	// Save memory note if the gateway detected something worth remembering
	if resp.MemoryNote != "" {
		if h.agentManager.MemoryMgr != nil {
			go func(note string) {
				h.agentManager.MemoryMgr.SaveNote("gateway", note, "gateway")
			}(resp.MemoryNote)
		} else {
			go agents.SaveGatewayMemoryNote(h.dataDir, resp.MemoryNote)
		}
	}

	// If a work order is needed, spawn the appropriate agent (attributed to gateway/builder).
	if resp.WorkOrder != nil {
		h.addThreadMember(threadID, "builder")
		majorActions := map[string]bool{"build_tool": true, "update_tool": true, "build_dashboard": true, "build_custom_dashboard": true}
		if majorActions[resp.Action] && h.isConfirmationEnabled() {
			h.saveConfirmationMessage(threadID, userID, resp)
			h.broadcastStatus(threadID, "done", "")
			return
		}
		gwName := h.agentManager.GatewayName()
		buildingMsg := gwName + " is building..."
		switch resp.Action {
		case "build_tool":
			h.broadcastRoutingIndicator(threadID, "builder")
			h.broadcastStatus(threadID, "spawning", buildingMsg)
			h.handleBuildTool(parentCtx, threadID, userID, resp)
		case "update_tool":
			h.broadcastRoutingIndicator(threadID, "builder")
			h.broadcastStatus(threadID, "spawning", buildingMsg)
			h.handleUpdateTool(parentCtx, threadID, userID, resp)
		case "build_dashboard", "build_custom_dashboard":
			h.broadcastRoutingIndicator(threadID, "builder")
			h.broadcastStatus(threadID, "spawning", buildingMsg)
			h.handleBuildCustomDashboard(parentCtx, threadID, userID, resp)
		case "create_agent":
			h.handleCreateAgent(threadID, userID, resp)
		case "create_skill":
			h.handleCreateSkill(threadID, resp)
		}
	} else if resp.Action == "guide" && resp.Message != "" {
		// Gateway is proactively guiding the user
		h.db.LogAudit(userID, "routing_gateway", "agent", "agent_role", "pounce", "guide")
		h.addThreadMember(threadID, "pounce")
		h.broadcastRoutingIndicator(threadID, "pounce")
		h.saveAssistantMessage(threadID, "pounce", resp.Message, gatewayCostUSD, gatewayInTok, gatewayOutTok)
		h.endAgentWork(threadID)
	} else if resp.AssignedAgent != "" && (resp.Action == "respond" || resp.Action == "route") {
		// Gateway assigned a specialist agent — hand off (no gateway message saved)
		h.db.LogAudit(userID, "routing_gateway", "agent", "agent_role", resp.AssignedAgent, resp.Action+" -> "+resp.AssignedAgent)
		h.addThreadMember(threadID, resp.AssignedAgent)
		h.beginAgentWork(threadID, resp.AssignedAgent)
		roleChatCtx, roleChatCancel := context.WithTimeout(parentCtx, h.agentManager.AgentTimeout())
		defer roleChatCancel()
		h.handleRoleChatWithDepth(roleChatCtx, threadID, content, resp.AssignedAgent, 0)
	} else {
		// No assigned agent and no work order — use gateway message if available, else fallback
		fallbackMsg := resp.Message
		if fallbackMsg == "" {
			fallbackMsg = "I'm not sure how to help with that. You can tag an agent directly with **@**, ask me to build a tool or dashboard, or say **hi** and I'll show you what's available."
		}
		h.addThreadMember(threadID, "pounce")
		h.broadcastRoutingIndicator(threadID, "pounce")
		h.saveAssistantMessage(threadID, "pounce", fallbackMsg, gatewayCostUSD, gatewayInTok, gatewayOutTok)
		h.endAgentWork(threadID)
	}
}

const maxMentionDepth = 3

func (h *ChatHandler) handleRoleChatWithDepth(ctx context.Context, threadID, content, agentRoleSlug string, depth int) {
	// Look up the role from the database
	var systemPrompt, model string
	var identityInitialized bool
	err := h.db.QueryRow(
		"SELECT system_prompt, model, identity_initialized FROM agent_roles WHERE slug = ? AND enabled = 1",
		agentRoleSlug,
	).Scan(&systemPrompt, &model, &identityInitialized)
	if err != nil {
		h.saveAssistantMessage(threadID, agentRoleSlug, "I'm sorry, I couldn't find that agent role or it's disabled.", 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	// If identity system is initialized, assemble prompt from files
	var agentDir string
	if identityInitialized {
		assembled, err := agents.AssembleSystemPrompt(h.dataDir, agentRoleSlug)
		if err == nil {
			systemPrompt = assembled
			agentDir = agents.AgentDir(h.dataDir, agentRoleSlug)
		}
	}

	// Inject About You context from context files
	aboutYou := GetAboutYouContent(h.db, h.dataDir)
	if aboutYou != "" {
		systemPrompt += "\n\n## USER CONTEXT (About You)\n" + aboutYou
	}

	// Inject thread member awareness so agents know who else is in the conversation
	memberContext := h.buildThreadMemberContext(threadID, agentRoleSlug)
	if memberContext != "" {
		systemPrompt += "\n\n" + memberContext
	}

	// Inject SQLite memory into system prompt (mirrors heartbeat.go behavior)
	if h.agentManager.MemoryMgr != nil {
		h.agentManager.MemoryMgr.EnsureMigrated(agentRoleSlug)
		if memSection := h.agentManager.MemoryMgr.BuildMemoryPromptSection(agentRoleSlug); memSection != "" {
			systemPrompt += "\n\n---\n\n" + memSection
		}
	}

	// Fetch thread history for multi-turn context (excludes current message)
	history := h.fetchThreadHistory(threadID)
	// Remove the last message (it's the current user message we're about to send separately)
	if len(history) > 0 && history[len(history)-1].Role == "user" {
		history = history[:len(history)-1]
	}

	response, usage, widgetJSON, err := h.agentManager.RoleChat(ctx, systemPrompt, model, history, content, threadID, agentDir, agentRoleSlug)
	if err != nil {
		h.saveAssistantMessage(threadID, agentRoleSlug, "I'm sorry, I encountered an error: "+err.Error(), 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	var costUSD float64
	var inTok, outTok int
	if usage != nil {
		costUSD = usage.CostUSD
		inTok = int(usage.InputTokens)
		outTok = int(usage.OutputTokens)
	}
	h.saveAssistantMessage(threadID, agentRoleSlug, response, costUSD, inTok, outTok, widgetJSON)

	// Notify frontend the message is saved before clearing streaming state
	h.broadcastStatus(threadID, "message_saved", "")

	h.agentManager.Broadcast("agent_completed", models.WSAgentCompleted{
		ThreadID:      threadID,
		AgentRoleSlug: agentRoleSlug,
	})

	// Check for agent-to-agent @mentions in the response (with depth limit)
	if depth < maxMentionDepth {
		if mentionedSlug := h.extractMention(response); mentionedSlug != "" && mentionedSlug != agentRoleSlug {
			h.evaluateAgentMention(ctx, threadID, agentRoleSlug, mentionedSlug, response, depth)
		}
	}
}

func (h *ChatHandler) handleBuildTool(ctx context.Context, threadID, userID string, resp *agents.GatewayResponse) {
	// Create a tool record
	toolID := uuid.New().String()
	now := time.Now().UTC()
	if _, err := h.db.Exec(
		"INSERT INTO tools (id, name, description, type, config, enabled, status, owner_agent_slug, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		toolID, resp.WorkOrder.Title, resp.WorkOrder.Description, "custom", "{}", false, "building", "builder", now, now,
	); err != nil {
		logger.Error("Failed to insert tool record: %v", err)
	}

	toolDir := filepath.Join(h.toolsDir, toolID)
	wo, err := agents.CreateWorkOrder(h.db, agents.WorkOrderToolBuild,
		resp.WorkOrder.Title, resp.WorkOrder.Description, resp.WorkOrder.Requirements,
		toolDir, toolID, threadID, userID,
	)
	if err != nil {
		h.saveAssistantMessage(threadID, "", "Failed to create work order: "+err.Error(), 0, 0, 0)
		h.endAgentWork(threadID)
		return
	}

	h.db.LogAudit(userID, "work_order_created", "work_order", "work_order", wo.ID, "build_tool: "+resp.WorkOrder.Title)
	placeholderID := h.saveAssistantMessage(threadID, "builder", "🔨 Building **"+resp.WorkOrder.Title+"**. This may take a few minutes.", 0, 0, 0)

	_, err = h.agentManager.SpawnToolBuilder(context.Background(), wo, threadID, userID, placeholderID)
	if err != nil {
		h.saveAssistantMessage(threadID, "", "Build failed: "+err.Error(), 0, 0, 0)
		h.endAgentWork(threadID)
	}
}

func (h *ChatHandler) handleUpdateTool(ctx context.Context, threadID, userID string, resp *agents.GatewayResponse) {
	// Look up the existing tool by ID (if gateway provided it) or by name
	var toolID string
	if resp.WorkOrder.ToolID != "" {
		toolID = resp.WorkOrder.ToolID
	} else {
		h.db.QueryRow(
			"SELECT id FROM tools WHERE name = ? AND deleted_at IS NULL LIMIT 1",
			resp.WorkOrder.Title,
		).Scan(&toolID)
	}
	if toolID == "" {
		// Try case-insensitive partial match as fallback
		h.db.QueryRow(
			"SELECT id FROM tools WHERE LOWER(name) LIKE '%' || LOWER(?) || '%' AND deleted_at IS NULL LIMIT 1",
			resp.WorkOrder.Title,
		).Scan(&toolID)
	}
	if toolID == "" {
		h.saveAssistantMessage(threadID, "", "Could not find an existing tool named **"+resp.WorkOrder.Title+"** to update.", 0, 0, 0)
		h.endAgentWork(threadID)
		return
	}

	toolDir := filepath.Join(h.toolsDir, toolID)

	wo, err := agents.CreateWorkOrder(h.db, agents.WorkOrderToolUpdate,
		resp.WorkOrder.Title, resp.WorkOrder.Description, resp.WorkOrder.Requirements,
		toolDir, toolID, threadID, userID,
	)
	if err != nil {
		h.saveAssistantMessage(threadID, "", "Failed to create work order: "+err.Error(), 0, 0, 0)
		h.endAgentWork(threadID)
		return
	}

	h.db.LogAudit(userID, "work_order_created", "work_order", "work_order", wo.ID, "update_tool: "+resp.WorkOrder.Title)
	placeholderID := h.saveAssistantMessage(threadID, "builder", "🔧 Updating **"+resp.WorkOrder.Title+"**. This may take a few minutes.", 0, 0, 0)

	_, err = h.agentManager.SpawnToolBuilder(context.Background(), wo, threadID, userID, placeholderID)
	if err != nil {
		h.saveAssistantMessage(threadID, "", "Update failed: "+err.Error(), 0, 0, 0)
		h.endAgentWork(threadID)
	}
}

func (h *ChatHandler) handleBuildDashboard(ctx context.Context, threadID, userID string, resp *agents.GatewayResponse) {
	// Check for existing dashboard to enable updates — prefer explicit ID from gateway
	var existingConfig string
	var existingName string
	var existingID string
	if resp.WorkOrder.DashboardID != "" {
		h.db.QueryRow(
			"SELECT id, name, widgets FROM dashboards WHERE id = ?",
			resp.WorkOrder.DashboardID,
		).Scan(&existingID, &existingName, &existingConfig)
	}
	if existingConfig == "" {
		h.db.QueryRow(
			"SELECT id, name, widgets FROM dashboards WHERE name LIKE ? ESCAPE '\\' LIMIT 1",
			"%"+escapeLike(resp.WorkOrder.Title)+"%",
		).Scan(&existingID, &existingName, &existingConfig)
	}

	requirements := resp.WorkOrder.Requirements
	if existingConfig != "" {
		requirements += "\n\nEXISTING DASHBOARD CONFIG (modify this, do not start from scratch):\nName: " + existingName + "\nWidgets: " + existingConfig
	}

	// Pass existingID as toolID so postBuildDashboard can match by ID
	wo, err := agents.CreateWorkOrder(h.db, agents.WorkOrderDashboardBuild,
		resp.WorkOrder.Title, resp.WorkOrder.Description, requirements,
		"", existingID, threadID, userID,
	)
	if err != nil {
		h.saveAssistantMessage(threadID, "", "Failed to create work order: "+err.Error(), 0, 0, 0)
		h.endAgentWork(threadID)
		return
	}

	h.db.LogAudit(userID, "work_order_created", "work_order", "work_order", wo.ID, "build_dashboard: "+resp.WorkOrder.Title)
	placeholderID := h.saveAssistantMessage(threadID, "builder", "Building the **"+resp.WorkOrder.Title+"** dashboard...", 0, 0, 0)

	_, err = h.agentManager.SpawnDashboardBuilder(context.Background(), wo, threadID, userID, placeholderID)
	if err != nil {
		h.saveAssistantMessage(threadID, "", "Dashboard build failed: "+err.Error(), 0, 0, 0)
		h.endAgentWork(threadID)
	}
}

func (h *ChatHandler) handleBuildCustomDashboard(ctx context.Context, threadID, userID string, resp *agents.GatewayResponse) {
	dashboardID := uuid.New().String()
	dashboardDir := filepath.Join(h.dashboardsDir, dashboardID)

	// Check for existing custom dashboard — prefer explicit ID from gateway
	var existingID string
	if resp.WorkOrder.DashboardID != "" {
		var checkID string
		err := h.db.QueryRow(
			"SELECT id FROM dashboards WHERE id = ? AND dashboard_type = 'custom'",
			resp.WorkOrder.DashboardID,
		).Scan(&checkID)
		if err == nil {
			existingID = checkID
		}
	}
	if existingID == "" {
		h.db.QueryRow(
			"SELECT id FROM dashboards WHERE name LIKE ? ESCAPE '\\' AND dashboard_type = 'custom' LIMIT 1",
			"%"+escapeLike(resp.WorkOrder.Title)+"%",
		).Scan(&existingID)
	}

	woType := agents.WorkOrderDashboardCustomBuild
	if existingID != "" {
		dashboardID = existingID
		dashboardDir = filepath.Join(h.dashboardsDir, existingID)
		woType = agents.WorkOrderDashboardCustomUpdate
	}

	wo, err := agents.CreateWorkOrder(h.db, woType,
		resp.WorkOrder.Title, resp.WorkOrder.Description, resp.WorkOrder.Requirements,
		dashboardDir, dashboardID, threadID, userID,
	)
	if err != nil {
		h.saveAssistantMessage(threadID, "", "Failed to create work order: "+err.Error(), 0, 0, 0)
		h.endAgentWork(threadID)
		return
	}

	h.db.LogAudit(userID, "work_order_created", "work_order", "work_order", wo.ID, "build_custom_dashboard: "+resp.WorkOrder.Title)
	placeholderID := h.saveAssistantMessage(threadID, "builder", "Building custom dashboard **"+resp.WorkOrder.Title+"**. This may take a few minutes.", 0, 0, 0)

	_, err = h.agentManager.SpawnCustomDashboardBuilder(context.Background(), wo, threadID, userID, placeholderID)
	if err != nil {
		h.saveAssistantMessage(threadID, "", "Custom dashboard build failed: "+err.Error(), 0, 0, 0)
		h.endAgentWork(threadID)
	}
}

func (h *ChatHandler) fetchThreadHistory(threadID string) []agents.ThreadMessage {
	rows, err := h.db.Query(
		fmt.Sprintf(
			"SELECT role, content, agent_role_slug FROM (SELECT role, content, agent_role_slug, created_at FROM chat_messages WHERE thread_id = ? ORDER BY created_at DESC LIMIT %d) sub ORDER BY created_at ASC",
			threadHistoryLimit,
		),
		threadID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var msgs []agents.ThreadMessage
	for rows.Next() {
		var role, content, agentSlug string
		if err := rows.Scan(&role, &content, &agentSlug); err != nil {
			continue
		}
		msgs = append(msgs, agents.ThreadMessage{Role: role, Content: content, AgentSlug: agentSlug})
	}

	return msgs
}

func (h *ChatHandler) handleCreateAgent(threadID, userID string, resp *agents.GatewayResponse) {
	if resp.WorkOrder == nil {
		h.broadcastStatus(threadID, "done", "")
		return
	}

	// Parse requirements JSON
	var reqs struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
		Model       string `json:"model"`
		Soul        string `json:"soul"`
	}
	if err := json.Unmarshal([]byte(resp.WorkOrder.Requirements), &reqs); err != nil {
		h.saveAssistantMessage(threadID, "", "Failed to parse agent requirements: "+err.Error(), 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	if reqs.Name == "" {
		h.saveAssistantMessage(threadID, "", "Agent name is required.", 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	// Auto-generate slug from name if missing
	if reqs.Slug == "" {
		reqs.Slug = strings.ToLower(strings.ReplaceAll(reqs.Name, " ", "-"))
	}

	// Validate slug format (uses package-level slugRegex from agent_roles.go)
	if !slugRegex.MatchString(reqs.Slug) {
		h.saveAssistantMessage(threadID, "", "Invalid agent slug format. Must be lowercase alphanumeric with hyphens.", 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	// Check for duplicate slug
	var existingID string
	if err := h.db.QueryRow("SELECT id FROM agent_roles WHERE slug = ?", reqs.Slug).Scan(&existingID); err == nil {
		h.saveAssistantMessage(threadID, "", fmt.Sprintf("An agent with slug `%s` already exists.", reqs.Slug), 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	if reqs.Model == "" {
		reqs.Model = "sonnet"
	}

	// Get next sort order
	var maxSort int
	h.db.QueryRow("SELECT COALESCE(MAX(sort_order), 0) FROM agent_roles").Scan(&maxSort)

	id := uuid.New().String()
	now := time.Now().UTC()

	_, err := h.db.Exec(
		`INSERT INTO agent_roles (id, slug, name, description, system_prompt, model, avatar_path, enabled, sort_order, is_preset, identity_initialized, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, '', 1, ?, 0, 1, ?, ?)`,
		id, reqs.Slug, reqs.Name, reqs.Description, reqs.Soul, reqs.Model, maxSort+1, now, now,
	)
	if err != nil {
		h.saveAssistantMessage(threadID, "", "Failed to create agent: "+err.Error(), 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	// Initialize identity files
	if err := agents.InitAgentDir(h.dataDir, reqs.Slug, reqs.Name, reqs.Soul); err != nil {
		h.saveAssistantMessage(threadID, "", "Agent created but failed to initialize identity files: "+err.Error(), 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	h.saveAssistantMessage(threadID, "", fmt.Sprintf("Created agent **%s** (`%s`). You can customize it further in the agent settings.", reqs.Name, reqs.Slug), 0, 0, 0)
	h.broadcastStatus(threadID, "done", "")
}

func (h *ChatHandler) handleCreateSkill(threadID string, resp *agents.GatewayResponse) {
	if resp.WorkOrder == nil {
		h.broadcastStatus(threadID, "done", "")
		return
	}

	// Parse requirements JSON
	var reqs struct {
		Name        string `json:"name"`
		Content     string `json:"content"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(resp.WorkOrder.Requirements), &reqs); err != nil {
		h.saveAssistantMessage(threadID, "", "Failed to parse skill requirements: "+err.Error(), 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	if reqs.Name == "" || reqs.Content == "" {
		h.saveAssistantMessage(threadID, "", "Skill name and content are required.", 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	if !agents.IsValidSkillName(reqs.Name) {
		h.saveAssistantMessage(threadID, "", fmt.Sprintf("Invalid skill name `%s`. Must be lowercase alphanumeric with hyphens.", reqs.Name), 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	// Ensure frontmatter wrapping if content lacks it
	content := reqs.Content
	meta, body := agents.ParseFrontmatter(content)
	if meta.Description == "" && reqs.Description != "" {
		content = agents.BuildFrontmatter(reqs.Name, reqs.Description, body)
	}

	if err := agents.WriteGlobalSkill(h.dataDir, reqs.Name, content); err != nil {
		h.saveAssistantMessage(threadID, "", "Failed to create skill: "+err.Error(), 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	h.saveAssistantMessage(threadID, "", fmt.Sprintf("Created skill **%s**. You can install it on any agent from their settings page.", reqs.Name), 0, 0, 0)
	h.broadcastStatus(threadID, "done", "")
}

func (h *ChatHandler) handleBootstrapComplete(threadID string, resp *agents.GatewayResponse, costUSD float64, inTok, outTok int) {
	// Parse requirements JSON from the bootstrap response
	var reqs struct {
		Name string `json:"name"`
		Soul string `json:"soul"`
		User string `json:"user"`
	}
	if err := json.Unmarshal([]byte(resp.WorkOrder.Requirements), &reqs); err != nil {
		h.saveAssistantMessage(threadID, "pounce", "Setup encountered an error. Let's try again!", 0, 0, 0)
		h.broadcastStatus(threadID, "done", "")
		return
	}

	// Write SOUL.md with the gathered personality info
	if reqs.Soul != "" {
		soulContent := fmt.Sprintf("# Soul\n\nYou are %s, the OpenPaw Gateway.\n\n%s", reqs.Name, reqs.Soul)
		agents.WriteGatewayFile(h.dataDir, "SOUL.md", soulContent)
	}

	// Write USER.md with what we learned about the user
	if reqs.User != "" {
		agents.WriteGatewayFile(h.dataDir, "USER.md", fmt.Sprintf("# User\n\n%s", reqs.User))
	}

	// Delete BOOTSTRAP.md to exit onboarding mode
	agents.DeleteGatewayBootstrap(h.dataDir)

	// Optionally update the gateway name in the builder agent_role
	if reqs.Name != "" {
		if _, err := h.db.Exec("UPDATE agent_roles SET name = ? WHERE slug = 'builder'", reqs.Name); err != nil {
			logger.Error("Failed to update builder name: %v", err)
		}
	}

	// Send confirmation
	confirmMsg := resp.Message
	if confirmMsg == "" {
		name := reqs.Name
		if name == "" {
			name = h.agentManager.GatewayName()
		}
		confirmMsg = fmt.Sprintf("All set! I'm **%s**, your gateway to OpenPaw. How can I help?", name)
	}

	h.addThreadMember(threadID, "pounce")
	h.broadcastRoutingIndicator(threadID, "pounce")
	h.saveAssistantMessage(threadID, "pounce", confirmMsg, costUSD, inTok, outTok)
	h.endAgentWork(threadID)
}
