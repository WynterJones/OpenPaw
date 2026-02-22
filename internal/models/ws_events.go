package models

// WebSocket event payload types for the most common broadcast messages.
// These replace map[string]interface{} for type safety in high-frequency broadcast calls.

// WSAgentStatus is the payload for "agent_status" broadcasts (status changes, routing indicators).
type WSAgentStatus struct {
	ThreadID      string `json:"thread_id"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	AgentRoleSlug string `json:"agent_role_slug,omitempty"`
}

// WSThreadUpdated is the payload for "thread_updated" broadcasts.
type WSThreadUpdated struct {
	ThreadID string `json:"thread_id"`
	Title    string `json:"title"`
}

// WSAgentCompleted is the payload for "agent_completed" broadcasts.
type WSAgentCompleted struct {
	ThreadID      string `json:"thread_id"`
	AgentRoleSlug string `json:"agent_role_slug,omitempty"`
	AgentID       string `json:"agent_id,omitempty"`
	WorkOrderID   string `json:"work_order_id,omitempty"`
	Status        string `json:"status,omitempty"`
	Output        string `json:"output,omitempty"`
}

// WSThreadMemberJoined is the payload for "thread_member_joined" broadcasts.
type WSThreadMemberJoined struct {
	ThreadID      string `json:"thread_id"`
	AgentRoleSlug string `json:"agent_role_slug"`
	Name          string `json:"name"`
	AvatarPath    string `json:"avatar_path"`
}

// WSThreadMemberRemoved is the payload for "thread_member_removed" broadcasts.
type WSThreadMemberRemoved struct {
	ThreadID      string `json:"thread_id"`
	AgentRoleSlug string `json:"agent_role_slug"`
}

// WSAgentStream is the payload for "agent_stream" broadcasts during builder execution.
type WSAgentStream struct {
	AgentID     string      `json:"agent_id"`
	WorkOrderID string      `json:"work_order_id"`
	ThreadID    string      `json:"thread_id"`
	Event       interface{} `json:"event"`
}
