package agents

import (
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/models"
)

type WorkOrderType string

const (
	WorkOrderToolBuild            WorkOrderType = "tool_build"
	WorkOrderToolUpdate           WorkOrderType = "tool_update"
	WorkOrderDashboardBuild       WorkOrderType = "dashboard_build"
	WorkOrderDashboardCustomBuild  WorkOrderType = "dashboard_custom_build"
	WorkOrderDashboardCustomUpdate WorkOrderType = "dashboard_custom_build_update"
)

type WorkOrderStatus string

const (
	WorkOrderPending               WorkOrderStatus = "pending"
	WorkOrderInProgress            WorkOrderStatus = "in_progress"
	WorkOrderCompleted             WorkOrderStatus = "completed"
	WorkOrderFailed                WorkOrderStatus = "failed"
	WorkOrderAwaitingConfirmation  WorkOrderStatus = "awaiting_confirmation"
	WorkOrderCancelled             WorkOrderStatus = "cancelled"
)

func CreateWorkOrder(db *database.DB, woType WorkOrderType, title, description, requirements, targetDir, toolID, threadID, createdBy string) (*models.WorkOrder, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	wo := &models.WorkOrder{
		ID:           id,
		Type:         string(woType),
		Status:       string(WorkOrderPending),
		Title:        title,
		Description:  description,
		Requirements: requirements,
		TargetDir:    targetDir,
		ToolID:       toolID,
		ThreadID:     threadID,
		CreatedBy:    createdBy,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err := db.Exec(
		`INSERT INTO work_orders (id, type, status, title, description, requirements, target_dir, tool_id, thread_id, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wo.ID, wo.Type, wo.Status, wo.Title, wo.Description, wo.Requirements,
		wo.TargetDir, wo.ToolID, wo.ThreadID, wo.CreatedBy, wo.CreatedAt, wo.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return wo, nil
}

func CreateWorkOrderWithStatus(db *database.DB, woType WorkOrderType, title, description, requirements, targetDir, toolID, threadID, createdBy string, status WorkOrderStatus) (*models.WorkOrder, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	wo := &models.WorkOrder{
		ID:           id,
		Type:         string(woType),
		Status:       string(status),
		Title:        title,
		Description:  description,
		Requirements: requirements,
		TargetDir:    targetDir,
		ToolID:       toolID,
		ThreadID:     threadID,
		CreatedBy:    createdBy,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err := db.Exec(
		`INSERT INTO work_orders (id, type, status, title, description, requirements, target_dir, tool_id, thread_id, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wo.ID, wo.Type, wo.Status, wo.Title, wo.Description, wo.Requirements,
		wo.TargetDir, wo.ToolID, wo.ThreadID, wo.CreatedBy, wo.CreatedAt, wo.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return wo, nil
}

func UpdateWorkOrderStatus(db *database.DB, id string, status WorkOrderStatus, result string) error {
	now := time.Now().UTC()
	_, err := db.Exec(
		"UPDATE work_orders SET status = ?, result = ?, updated_at = ? WHERE id = ?",
		string(status), result, now, id,
	)
	return err
}

func UpdateWorkOrderAgent(db *database.DB, workOrderID, agentID string) error {
	now := time.Now().UTC()
	_, err := db.Exec(
		"UPDATE work_orders SET agent_id = ?, updated_at = ? WHERE id = ?",
		agentID, now, workOrderID,
	)
	return err
}

func GetWorkOrder(db *database.DB, id string) (*models.WorkOrder, error) {
	var wo models.WorkOrder
	err := db.QueryRow(
		`SELECT id, type, status, title, description, requirements, target_dir, tool_id, thread_id, agent_id, result, created_by, created_at, updated_at
		 FROM work_orders WHERE id = ?`, id,
	).Scan(&wo.ID, &wo.Type, &wo.Status, &wo.Title, &wo.Description, &wo.Requirements,
		&wo.TargetDir, &wo.ToolID, &wo.ThreadID, &wo.AgentID, &wo.Result, &wo.CreatedBy,
		&wo.CreatedAt, &wo.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &wo, nil
}
