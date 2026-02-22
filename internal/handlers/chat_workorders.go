package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
)

func (h *ChatHandler) saveConfirmationMessage(threadID, userID string, resp *agents.GatewayResponse) {
	// Determine work order type — all dashboards are custom
	var woType agents.WorkOrderType
	switch resp.Action {
	case "build_tool":
		woType = agents.WorkOrderToolBuild
	case "update_tool":
		woType = agents.WorkOrderToolUpdate
	case "build_dashboard", "build_custom_dashboard":
		woType = agents.WorkOrderDashboardCustomBuild
	default:
		return
	}

	// For dashboard actions, stash dashboard_id in the work order's tool_id field
	// so it survives the confirmation round-trip
	toolID := ""
	if resp.WorkOrder.DashboardID != "" {
		toolID = resp.WorkOrder.DashboardID
	}

	wo, err := agents.CreateWorkOrderWithStatus(h.db, woType,
		resp.WorkOrder.Title, resp.WorkOrder.Description, resp.WorkOrder.Requirements,
		"", toolID, threadID, userID, agents.WorkOrderAwaitingConfirmation,
	)
	if err != nil {
		h.saveAssistantMessage(threadID, "", "Failed to create work order: "+err.Error(), 0, 0, 0)
		return
	}

	// Build action label — distinguish new vs update
	isUpdate := resp.WorkOrder.DashboardID != "" || resp.Action == "update_tool"
	actionLabel := resp.Action
	switch resp.Action {
	case "build_tool":
		actionLabel = "New Tool"
	case "update_tool":
		actionLabel = "Update Tool"
	case "build_dashboard", "build_custom_dashboard":
		if isUpdate {
			actionLabel = "Update Dashboard"
		} else {
			actionLabel = "New Dashboard"
		}
	}

	// Build confirmation card using json.Marshal for safe encoding
	cardData := map[string]string{
		"__type":        "confirmation",
		"action":        resp.Action,
		"action_label":  actionLabel,
		"title":         resp.WorkOrder.Title,
		"description":   resp.WorkOrder.Description,
		"work_order_id": wo.ID,
		"message_id":    "",
		"status":        "pending",
	}
	cardBytes, err := json.Marshal(cardData)
	if err != nil {
		h.saveAssistantMessage(threadID, "", "Failed to build confirmation card: "+err.Error(), 0, 0, 0)
		return
	}
	cardJSON := string(cardBytes)

	msgID := h.saveAssistantMessage(threadID, "", cardJSON, 0, 0, 0)

	// Update message content with the real message ID
	cardData["message_id"] = msgID
	updatedBytes, _ := json.Marshal(cardData)
	if _, err := h.db.Exec("UPDATE chat_messages SET content = ? WHERE id = ?", string(updatedBytes), msgID); err != nil {
		logger.Error("Failed to update confirmation message ID: %v", err)
	}

	h.agentManager.Broadcast("agent_status", map[string]interface{}{
		"thread_id": threadID,
		"status":    "message_saved",
		"message":   "",
	})
}

func (h *ChatHandler) ConfirmWork(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r.Context())

	var req struct {
		WorkOrderID string `json:"work_order_id"`
		MessageID   string `json:"message_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate work order
	wo, err := agents.GetWorkOrder(h.db, req.WorkOrderID)
	if err != nil {
		writeError(w, http.StatusNotFound, "work order not found")
		return
	}
	if wo.ThreadID != threadID {
		writeError(w, http.StatusBadRequest, "work order does not belong to this thread")
		return
	}
	if wo.Status != string(agents.WorkOrderAwaitingConfirmation) {
		writeError(w, http.StatusConflict, "work order is not awaiting confirmation")
		return
	}

	// Update work order status to pending
	agents.UpdateWorkOrderStatus(h.db, wo.ID, agents.WorkOrderPending, "")
	h.db.LogAudit(userID, "work_order_confirmed", "work_order", "work_order", wo.ID, wo.Title)

	// Update message content: set status to confirmed
	var msgContent string
	h.db.QueryRow("SELECT content FROM chat_messages WHERE id = ?", req.MessageID).Scan(&msgContent)
	if msgContent != "" {
		updatedContent := strings.Replace(msgContent, `"status":"pending"`, `"status":"confirmed"`, 1)
		if _, err := h.db.Exec("UPDATE chat_messages SET content = ? WHERE id = ?", updatedContent, req.MessageID); err != nil {
			logger.Error("Failed to update confirmation status: %v", err)
		}
	}

	h.agentManager.Broadcast("agent_status", map[string]interface{}{
		"thread_id": threadID,
		"status":    "message_saved",
		"message":   "",
	})

	// Dispatch builder in a goroutine with cancellable context
	confirmCtx, confirmCancel := context.WithCancel(context.Background())
	h.threadCancels.Store(threadID, confirmCancel)
	go func() {
		defer func() {
			confirmCancel()
			h.threadCancels.Delete(threadID)
		}()
		resp := &agents.GatewayResponse{
			Action: wo.Type,
			WorkOrder: &agents.GatewayWorkOrder{
				Title:        wo.Title,
				Description:  wo.Description,
				Requirements: wo.Requirements,
				DashboardID:  wo.ToolID,
			},
		}
		gwName := h.agentManager.GatewayName()
		buildingMsg := gwName + " is building..."
		switch wo.Type {
		case string(agents.WorkOrderToolBuild):
			resp.Action = "build_tool"
			h.broadcastStatus(threadID, "spawning", buildingMsg)
			h.handleBuildTool(confirmCtx, threadID, userID, resp)
		case string(agents.WorkOrderToolUpdate):
			resp.Action = "update_tool"
			h.broadcastStatus(threadID, "spawning", buildingMsg)
			h.handleUpdateTool(confirmCtx, threadID, userID, resp)
		case string(agents.WorkOrderDashboardBuild), string(agents.WorkOrderDashboardCustomBuild), string(agents.WorkOrderDashboardCustomUpdate):
			resp.Action = "build_custom_dashboard"
			h.broadcastStatus(threadID, "spawning", buildingMsg)
			h.handleBuildCustomDashboard(confirmCtx, threadID, userID, resp)
		}
	}()

	writeJSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

func (h *ChatHandler) RejectWork(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")

	var req struct {
		WorkOrderID string `json:"work_order_id"`
		MessageID   string `json:"message_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate work order
	wo, err := agents.GetWorkOrder(h.db, req.WorkOrderID)
	if err != nil {
		writeError(w, http.StatusNotFound, "work order not found")
		return
	}
	if wo.ThreadID != threadID {
		writeError(w, http.StatusBadRequest, "work order does not belong to this thread")
		return
	}
	if wo.Status != string(agents.WorkOrderAwaitingConfirmation) {
		writeError(w, http.StatusConflict, "work order is not awaiting confirmation")
		return
	}

	// Update work order status to cancelled
	agents.UpdateWorkOrderStatus(h.db, wo.ID, agents.WorkOrderCancelled, "User cancelled")
	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "work_order_rejected", "work_order", "work_order", wo.ID, wo.Title)

	// Update message content: set status to rejected
	var msgContent string
	h.db.QueryRow("SELECT content FROM chat_messages WHERE id = ?", req.MessageID).Scan(&msgContent)
	if msgContent != "" {
		updatedContent := strings.Replace(msgContent, `"status":"pending"`, `"status":"rejected"`, 1)
		if _, err := h.db.Exec("UPDATE chat_messages SET content = ? WHERE id = ?", updatedContent, req.MessageID); err != nil {
			logger.Error("Failed to update rejection status: %v", err)
		}
	}

	h.agentManager.Broadcast("agent_status", map[string]interface{}{
		"thread_id": threadID,
		"status":    "message_saved",
		"message":   "",
	})

	// Save follow-up message
	h.saveAssistantMessage(threadID, "", "No problem, cancelled.", 0, 0, 0)

	h.agentManager.Broadcast("agent_status", map[string]interface{}{
		"thread_id": threadID,
		"status":    "done",
		"message":   "",
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}
