package agents

import (
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
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

// interruptedNote is what a build left mid-flight by a crash or restart reports
// as its result, in the work order, the agent row and the chat.
const interruptedNote = "Interrupted — OpenPaw restarted while this was being built."

// ReapOrphanedWork closes out builds that were still in flight when the process
// last stopped.
//
// A work order goes in_progress and its agent row goes running the moment the
// builder spawns, and only the goroutine that owns them ever writes an end
// state. Anything still unfinished at boot cannot be running — the process that
// owned it is gone. Left alone those rows are permanent: ThreadStatus keeps
// reporting the thread active so the chat spins forever, and the active-chats
// indicator counts work that stopped happening days ago. The scheduler and
// heartbeat already reap their own runs exactly this way.
//
// awaiting_confirmation is deliberately left alone — that work order is waiting
// on the user, not on a process, and survives a restart intact.
func ReapOrphanedWork(db *database.DB) {
	now := time.Now().UTC()

	// Collected before the update so the affected threads can be told; after it
	// they are indistinguishable from any other failed build.
	type orphan struct{ threadID, title string }
	var orphans []orphan
	rows, err := db.Query(
		"SELECT thread_id, title FROM work_orders WHERE status IN (?, ?)",
		string(WorkOrderPending), string(WorkOrderInProgress),
	)
	if err == nil {
		for rows.Next() {
			var o orphan
			if rows.Scan(&o.threadID, &o.title) == nil {
				orphans = append(orphans, o)
			}
		}
		rows.Close()
	}

	if _, err := db.Exec(
		"UPDATE work_orders SET status = ?, result = ?, updated_at = ? WHERE status IN (?, ?)",
		string(WorkOrderFailed), interruptedNote, now,
		string(WorkOrderPending), string(WorkOrderInProgress),
	); err != nil {
		logger.Error("Failed to reap orphaned work orders: %v", err)
		return
	}

	if _, err := db.Exec(
		"UPDATE agents SET status = 'failed', error = ?, completed_at = ?, updated_at = ? WHERE status = 'running'",
		interruptedNote, now, now,
	); err != nil {
		logger.Error("Failed to reap orphaned agent runs: %v", err)
	}

	// The chat's last word was "🔨 Building X." — without this the thread just
	// stops mid-sentence and the user is left waiting on a build that died.
	for _, o := range orphans {
		if o.threadID == "" {
			continue
		}
		what := o.title
		if what == "" {
			what = "the last build"
		}
		db.Exec(
			`INSERT INTO chat_messages (id, thread_id, role, content, agent_role_slug, created_at)
			 VALUES (?, ?, 'assistant', ?, 'builder', ?)`,
			uuid.New().String(), o.threadID,
			"**"+what+"** was interrupted when OpenPaw restarted. Ask me again and I'll pick it back up.",
			now,
		)
		db.Exec("UPDATE chat_threads SET updated_at = ? WHERE id = ?", now, o.threadID)
	}

	if len(orphans) > 0 {
		logger.Info("Reaped %d interrupted build(s)", len(orphans))
	}
}

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
