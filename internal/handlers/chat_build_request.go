package handlers

import (
	"context"

	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/logger"
)

// RequestBuild files a build asked for by a specialist agent mid-conversation.
//
// It backs the request_build tool. Building used to be the gateway's alone, so
// an agent that had just spent a conversation working out exactly what a service
// should do could only tell the user to go and ask someone else for it — and if
// the gateway then misread the follow-up, the request was lost entirely.
//
// The request joins the normal path rather than a parallel one: same work order
// types, same confirmation card, same builder, same retarget when a "service"
// is really a dashboard. The only difference is who asked.
func (h *ChatHandler) RequestBuild(ctx context.Context, threadID, kind, title, description, requirements string) (string, error) {
	action := "build_tool"
	noun := "service"
	if kind == "dashboard" {
		action = "build_custom_dashboard"
		noun = "dashboard"
	}

	resp := &agents.GatewayResponse{
		Action: action,
		WorkOrder: &agents.GatewayWorkOrder{
			Title:        title,
			Description:  description,
			Requirements: requirements,
		},
	}

	// An existing service or dashboard by this name means update, not a second
	// copy alongside it. retarget handles the dashboard case; the tool lookup
	// here covers a service the agent described as new.
	h.retargetDashboardWorkOrder(resp)
	if resp.Action == "build_tool" {
		if toolID := h.findWorkOrderToolID(resp.WorkOrder); toolID != "" {
			resp.Action = "update_tool"
			resp.WorkOrder.ToolID = toolID
		}
	}
	if resp.Action == "build_custom_dashboard" || resp.Action == "build_dashboard" {
		noun = "dashboard"
	}

	userID := h.ownerUserID()
	h.addThreadMember(threadID, gatewayRoleSlug)

	// Confirmation is the user's guardrail on anything that writes code; an
	// agent asking on their behalf does not get to skip it.
	if h.isConfirmationEnabled() {
		h.saveConfirmationMessage(threadID, userID, resp)
		return "Filed a build request for \"" + title + "\". The user has been shown an approval card in this thread — tell them it is waiting, and stop. Do not call request_build again for this.", nil
	}

	go func() {
		buildCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		switch resp.Action {
		case "build_tool":
			h.handleBuildTool(buildCtx, threadID, userID, resp, 0, 0, 0)
		case "update_tool":
			h.handleUpdateTool(buildCtx, threadID, userID, resp, 0, 0, 0)
		case "build_dashboard":
			h.handleBuildDashboard(buildCtx, threadID, userID, resp)
		case "build_custom_dashboard":
			h.handleBuildCustomDashboard(buildCtx, threadID, userID, resp, 0, 0, 0)
		}
	}()

	logger.Info("Agent-requested build started: %s %q", resp.Action, title)
	return "The builder has started on the " + noun + " \"" + title + "\" and will report back in this thread. Tell the user it is underway, and stop — do not call request_build again for this.", nil
}

// ownerUserID returns the account to attribute an agent-initiated build to.
// There is no request to read a user from, and threads carry no owner, so this
// is the admin — the single account this self-hosted instance belongs to.
func (h *ChatHandler) ownerUserID() string {
	var userID string
	h.db.QueryRow("SELECT id FROM users ORDER BY created_at ASC LIMIT 1").Scan(&userID)
	return userID
}
