package models

import "time"

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	DisplayName  string    `json:"display_name"`
	AvatarPath   string    `json:"avatar_path"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Tool struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Type           string     `json:"type"`
	Config         string     `json:"config"`
	Enabled        bool       `json:"enabled"`
	Status         string     `json:"status"`
	Port           int        `json:"port"`
	PID            int        `json:"pid"`
	Capabilities   string     `json:"capabilities"`
	OwnerAgentSlug string     `json:"owner_agent_slug"`
	LibrarySlug    string     `json:"library_slug"`
	LibraryVersion string     `json:"library_version"`
	SourceHash     string     `json:"source_hash"`
	BinaryHash     string     `json:"binary_hash"`
	Folder         string     `json:"folder"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

type ToolIntegrity struct {
	ID         string    `json:"id"`
	ToolID     string    `json:"tool_id"`
	Filename   string    `json:"filename"`
	FileHash   string    `json:"file_hash"`
	FileSize   int64     `json:"file_size"`
	RecordedAt time.Time `json:"recorded_at"`
}

type Secret struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	EncryptedValue string    `json:"-"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Schedule struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	CronExpr            string     `json:"cron_expr"`
	ToolID              string     `json:"tool_id"`
	Action              string     `json:"action"`
	Payload             string     `json:"payload"`
	Enabled             bool       `json:"enabled"`
	Type                string     `json:"type"`
	AgentRoleSlug       string     `json:"agent_role_slug"`
	PromptContent       string     `json:"prompt_content"`
	ThreadID            string     `json:"thread_id"`
	DashboardID         string     `json:"dashboard_id"`
	WidgetID            string     `json:"widget_id"`
	BrowserSessionID    string     `json:"browser_session_id"`
	BrowserInstructions string     `json:"browser_instructions"`
	LastRunAt           *time.Time `json:"last_run_at,omitempty"`
	NextRunAt           *time.Time `json:"next_run_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type ScheduleExecution struct {
	ID         string     `json:"id"`
	ScheduleID string     `json:"schedule_id"`
	Status     string     `json:"status"`
	Output     string     `json:"output"`
	Error      string     `json:"error"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type Dashboard struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Layout         string    `json:"layout"`
	Widgets        string    `json:"widgets"`
	DashboardType  string    `json:"dashboard_type"`
	OwnerAgentSlug string    `json:"owner_agent_slug"`
	BgImage        string    `json:"bg_image"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type DashboardDataPoint struct {
	ID          string    `json:"id"`
	DashboardID string    `json:"dashboard_id"`
	WidgetID    string    `json:"widget_id"`
	ToolID      string    `json:"tool_id"`
	Endpoint    string    `json:"endpoint"`
	Data        string    `json:"data"`
	CollectedAt time.Time `json:"collected_at"`
}

type AgentToolAccess struct {
	ID            string    `json:"id"`
	AgentRoleSlug string    `json:"agent_role_slug"`
	ToolID        string    `json:"tool_id"`
	GrantedAt     time.Time `json:"granted_at"`
}

type AuditLog struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Category  string    `json:"category"`
	Target    string    `json:"target"`
	TargetID  string    `json:"target_id"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"created_at"`
}

type ChatThread struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	TotalCostUSD float64   `json:"total_cost_usd"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ChatMessage struct {
	ID            string    `json:"id"`
	ThreadID      string    `json:"thread_id"`
	Role          string    `json:"role"`
	Content       string    `json:"content"`
	AgentRoleSlug string    `json:"agent_role_slug"`
	CostUSD       float64   `json:"cost_usd"`
	InputTokens   int       `json:"input_tokens"`
	OutputTokens  int       `json:"output_tokens"`
	WidgetData    *string   `json:"widget_data,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type Settings struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Agent struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	Model       string     `json:"model"`
	WorkOrderID string     `json:"work_order_id"`
	PID         int        `json:"pid"`
	WorkingDir  string     `json:"working_dir"`
	Output      string     `json:"output"`
	Error       string     `json:"error,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type AgentRole struct {
	ID                   string    `json:"id"`
	Slug                 string    `json:"slug"`
	Name                 string    `json:"name"`
	Description          string    `json:"description"`
	SystemPrompt         string    `json:"system_prompt"`
	Model                string    `json:"model"`
	AvatarPath           string    `json:"avatar_path"`
	Enabled              bool      `json:"enabled"`
	SortOrder            int       `json:"sort_order"`
	IsPreset             bool      `json:"is_preset"`
	IdentityInitialized  bool      `json:"identity_initialized"`
	HeartbeatEnabled     bool      `json:"heartbeat_enabled"`
	LibrarySlug          string    `json:"library_slug"`
	LibraryVersion       string    `json:"library_version"`
	Folder               string    `json:"folder"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type Notification struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Body            string    `json:"body"`
	Priority        string    `json:"priority"`
	SourceAgentSlug string    `json:"source_agent_slug"`
	SourceType      string    `json:"source_type"`
	Link            string    `json:"link"`
	Read            bool      `json:"read"`
	Dismissed       bool      `json:"dismissed"`
	CreatedAt       time.Time `json:"created_at"`
}

type HeartbeatExecution struct {
	ID            string     `json:"id"`
	AgentRoleSlug string     `json:"agent_role_slug"`
	Status        string     `json:"status"`
	ActionsTaken  string     `json:"actions_taken"`
	Output        string     `json:"output"`
	Error         string     `json:"error"`
	CostUSD       float64    `json:"cost_usd"`
	InputTokens   int        `json:"input_tokens"`
	OutputTokens  int        `json:"output_tokens"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

type ThreadMember struct {
	ThreadID      string    `json:"thread_id"`
	AgentRoleSlug string    `json:"agent_role_slug"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	AvatarPath    string    `json:"avatar_path"`
	JoinedAt      time.Time `json:"joined_at"`
}

type WorkOrder struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Requirements string    `json:"requirements"`
	TargetDir    string    `json:"target_dir"`
	ToolID       string    `json:"tool_id"`
	ThreadID     string    `json:"thread_id"`
	AgentID      string    `json:"agent_id"`
	Result       string    `json:"result"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ContextFolder struct {
	ID        string    `json:"id"`
	ParentID  *string   `json:"parent_id,omitempty"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ContextFile struct {
	ID         string    `json:"id"`
	FolderID   *string   `json:"folder_id,omitempty"`
	Name       string    `json:"name"`
	Filename   string    `json:"filename"`
	MimeType   string    `json:"mime_type"`
	SizeBytes  int64     `json:"size_bytes"`
	IsAboutYou bool      `json:"is_about_you"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ChatAttachment struct {
	ID           string    `json:"id"`
	MessageID    string    `json:"message_id"`
	Filename     string    `json:"filename"`
	OriginalName string    `json:"original_name"`
	MimeType     string    `json:"mime_type"`
	SizeBytes    int64     `json:"size_bytes"`
	CreatedAt    time.Time `json:"created_at"`
}
