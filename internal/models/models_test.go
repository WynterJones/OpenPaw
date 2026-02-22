package models

import (
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	return data
}

func refTime(t time.Time) *time.Time { return &t }
func refString(s string) *string     { return &s }

var fixedTime = time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------------------
// User
// ---------------------------------------------------------------------------

func TestUserJSONRoundTrip(t *testing.T) {
	orig := User{
		ID:           "u-001",
		Username:     "alice",
		PasswordHash: "secret-hash",
		DisplayName:  "Alice",
		AvatarPath:   "/img/alice.png",
		CreatedAt:    fixedTime,
		UpdatedAt:    fixedTime,
	}

	data := mustMarshal(t, orig)

	var decoded User
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, orig.ID)
	}
	if decoded.Username != orig.Username {
		t.Errorf("Username mismatch: got %q, want %q", decoded.Username, orig.Username)
	}
	if decoded.DisplayName != orig.DisplayName {
		t.Errorf("DisplayName mismatch: got %q, want %q", decoded.DisplayName, orig.DisplayName)
	}
	if decoded.AvatarPath != orig.AvatarPath {
		t.Errorf("AvatarPath mismatch: got %q, want %q", decoded.AvatarPath, orig.AvatarPath)
	}
	if !decoded.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("CreatedAt mismatch: got %v, want %v", decoded.CreatedAt, orig.CreatedAt)
	}
	if !decoded.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("UpdatedAt mismatch: got %v, want %v", decoded.UpdatedAt, orig.UpdatedAt)
	}
}

func TestUserPasswordHashOmittedFromJSON(t *testing.T) {
	u := User{
		ID:           "u-002",
		Username:     "bob",
		PasswordHash: "super-secret",
		DisplayName:  "Bob",
	}

	data := mustMarshal(t, u)

	// The raw JSON must not contain the password hash value.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	if _, ok := raw["password_hash"]; ok {
		t.Error("password_hash should not appear in JSON output")
	}
	if _, ok := raw["PasswordHash"]; ok {
		t.Error("PasswordHash should not appear in JSON output")
	}

	// After round-trip the decoded PasswordHash must be empty.
	var decoded User
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.PasswordHash != "" {
		t.Errorf("PasswordHash should be empty after round-trip, got %q", decoded.PasswordHash)
	}
}

// ---------------------------------------------------------------------------
// Secret (also has json:"-" on EncryptedValue)
// ---------------------------------------------------------------------------

func TestSecretEncryptedValueOmittedFromJSON(t *testing.T) {
	s := Secret{
		ID:             "sec-001",
		Name:           "API_KEY",
		EncryptedValue: "encrypted-bytes",
		Description:    "My API key",
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}

	data := mustMarshal(t, s)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}
	if _, ok := raw["encrypted_value"]; ok {
		t.Error("encrypted_value should not appear in JSON output")
	}
	if _, ok := raw["EncryptedValue"]; ok {
		t.Error("EncryptedValue should not appear in JSON output")
	}
}

// ---------------------------------------------------------------------------
// Tool
// ---------------------------------------------------------------------------

func TestToolJSONRoundTrip(t *testing.T) {
	orig := Tool{
		ID:             "t-001",
		Name:           "weather",
		Description:    "Weather lookup",
		Type:           "mcp",
		Config:         `{"url":"http://example.com"}`,
		Enabled:        true,
		Status:         "running",
		Port:           9090,
		PID:            12345,
		Capabilities:   "read,write",
		OwnerAgentSlug: "agent-a",
		LibrarySlug:    "weather-lib",
		LibraryVersion: "1.0.0",
		SourceHash:     "abc123",
		BinaryHash:     "def456",
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
		DeletedAt:      nil,
	}

	data := mustMarshal(t, orig)

	var decoded Tool
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, orig.ID)
	}
	if decoded.Name != orig.Name {
		t.Errorf("Name mismatch: got %q, want %q", decoded.Name, orig.Name)
	}
	if decoded.Enabled != orig.Enabled {
		t.Errorf("Enabled mismatch: got %v, want %v", decoded.Enabled, orig.Enabled)
	}
	if decoded.Port != orig.Port {
		t.Errorf("Port mismatch: got %d, want %d", decoded.Port, orig.Port)
	}
	if decoded.DeletedAt != nil {
		t.Error("DeletedAt should be nil")
	}
}

func TestToolDeletedAtOmittedWhenNil(t *testing.T) {
	tool := Tool{ID: "t-002", Name: "tool-nil-deleted"}
	data := mustMarshal(t, tool)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}
	if _, ok := raw["deleted_at"]; ok {
		t.Error("deleted_at should be omitted when nil")
	}
}

func TestToolDeletedAtPresentWhenSet(t *testing.T) {
	deletedAt := fixedTime.Add(24 * time.Hour)
	tool := Tool{
		ID:        "t-003",
		Name:      "tool-deleted",
		DeletedAt: &deletedAt,
	}
	data := mustMarshal(t, tool)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}
	if _, ok := raw["deleted_at"]; !ok {
		t.Error("deleted_at should be present when set")
	}

	var decoded Tool
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.DeletedAt == nil {
		t.Fatal("decoded DeletedAt should not be nil")
	}
	if !decoded.DeletedAt.Equal(deletedAt) {
		t.Errorf("DeletedAt mismatch: got %v, want %v", *decoded.DeletedAt, deletedAt)
	}
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

func TestDashboardJSONRoundTrip(t *testing.T) {
	orig := Dashboard{
		ID:             "d-001",
		Name:           "System Overview",
		Description:    "Main dashboard",
		Layout:         `{"cols":3}`,
		Widgets:        `[{"id":"w1"}]`,
		DashboardType:  "system",
		OwnerAgentSlug: "admin-agent",
		BgImage:        "/img/bg.png",
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}

	data := mustMarshal(t, orig)

	var decoded Dashboard
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, orig.ID)
	}
	if decoded.Name != orig.Name {
		t.Errorf("Name mismatch: got %q, want %q", decoded.Name, orig.Name)
	}
	if decoded.Layout != orig.Layout {
		t.Errorf("Layout mismatch: got %q, want %q", decoded.Layout, orig.Layout)
	}
	if decoded.DashboardType != orig.DashboardType {
		t.Errorf("DashboardType mismatch: got %q, want %q", decoded.DashboardType, orig.DashboardType)
	}
}

// ---------------------------------------------------------------------------
// ChatThread
// ---------------------------------------------------------------------------

func TestChatThreadJSONRoundTrip(t *testing.T) {
	orig := ChatThread{
		ID:        "ct-001",
		Title:     "Hello world",
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime.Add(time.Hour),
	}

	data := mustMarshal(t, orig)

	var decoded ChatThread
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, orig.ID)
	}
	if decoded.Title != orig.Title {
		t.Errorf("Title mismatch: got %q, want %q", decoded.Title, orig.Title)
	}
	if !decoded.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("CreatedAt mismatch: got %v, want %v", decoded.CreatedAt, orig.CreatedAt)
	}
	if !decoded.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("UpdatedAt mismatch: got %v, want %v", decoded.UpdatedAt, orig.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// ChatMessage
// ---------------------------------------------------------------------------

func TestChatMessageJSONRoundTrip(t *testing.T) {
	widgetData := `{"chart":"bar"}`
	orig := ChatMessage{
		ID:            "cm-001",
		ThreadID:      "ct-001",
		Role:          "assistant",
		Content:       "Here is the data.",
		AgentRoleSlug: "helper",
		CostUSD:       0.0025,
		InputTokens:   150,
		OutputTokens:  200,
		WidgetData:    &widgetData,
		CreatedAt:     fixedTime,
	}

	data := mustMarshal(t, orig)

	var decoded ChatMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, orig.ID)
	}
	if decoded.Role != orig.Role {
		t.Errorf("Role mismatch: got %q, want %q", decoded.Role, orig.Role)
	}
	if decoded.CostUSD != orig.CostUSD {
		t.Errorf("CostUSD mismatch: got %f, want %f", decoded.CostUSD, orig.CostUSD)
	}
	if decoded.InputTokens != orig.InputTokens {
		t.Errorf("InputTokens mismatch: got %d, want %d", decoded.InputTokens, orig.InputTokens)
	}
	if decoded.OutputTokens != orig.OutputTokens {
		t.Errorf("OutputTokens mismatch: got %d, want %d", decoded.OutputTokens, orig.OutputTokens)
	}
	if decoded.WidgetData == nil {
		t.Fatal("WidgetData should not be nil")
	}
	if *decoded.WidgetData != widgetData {
		t.Errorf("WidgetData mismatch: got %q, want %q", *decoded.WidgetData, widgetData)
	}
}

func TestChatMessageWidgetDataOmittedWhenNil(t *testing.T) {
	msg := ChatMessage{ID: "cm-002", Role: "user", Content: "Hello"}
	data := mustMarshal(t, msg)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}
	if _, ok := raw["widget_data"]; ok {
		t.Error("widget_data should be omitted when nil")
	}
}

// ---------------------------------------------------------------------------
// Agent
// ---------------------------------------------------------------------------

func TestAgentJSONRoundTrip(t *testing.T) {
	started := fixedTime
	completed := fixedTime.Add(5 * time.Minute)
	orig := Agent{
		ID:          "a-001",
		Type:        "builder",
		Status:      "completed",
		Model:       "claude-sonnet-4-20250514",
		WorkOrderID: "wo-001",
		PID:         54321,
		WorkingDir:  "/tmp/work",
		Output:      "Done.",
		Error:       "",
		StartedAt:   &started,
		CompletedAt: &completed,
		CreatedAt:   fixedTime,
		UpdatedAt:   fixedTime,
	}

	data := mustMarshal(t, orig)

	var decoded Agent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, orig.ID)
	}
	if decoded.Type != orig.Type {
		t.Errorf("Type mismatch: got %q, want %q", decoded.Type, orig.Type)
	}
	if decoded.PID != orig.PID {
		t.Errorf("PID mismatch: got %d, want %d", decoded.PID, orig.PID)
	}
	if decoded.StartedAt == nil {
		t.Fatal("StartedAt should not be nil")
	}
	if !decoded.StartedAt.Equal(started) {
		t.Errorf("StartedAt mismatch: got %v, want %v", *decoded.StartedAt, started)
	}
	if decoded.CompletedAt == nil {
		t.Fatal("CompletedAt should not be nil")
	}
	if !decoded.CompletedAt.Equal(completed) {
		t.Errorf("CompletedAt mismatch: got %v, want %v", *decoded.CompletedAt, completed)
	}
}

func TestAgentOptionalTimesOmittedWhenNil(t *testing.T) {
	agent := Agent{ID: "a-002", Type: "chat", Status: "pending"}
	data := mustMarshal(t, agent)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}
	if _, ok := raw["started_at"]; ok {
		t.Error("started_at should be omitted when nil")
	}
	if _, ok := raw["completed_at"]; ok {
		t.Error("completed_at should be omitted when nil")
	}
}

func TestAgentErrorOmittedWhenEmpty(t *testing.T) {
	agent := Agent{ID: "a-003", Type: "chat", Status: "running", Error: ""}
	data := mustMarshal(t, agent)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}
	if _, ok := raw["error"]; ok {
		t.Error("error should be omitted when empty string")
	}
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

func TestSettingsJSONRoundTrip(t *testing.T) {
	orig := Settings{
		ID:    "s-001",
		Key:   "theme",
		Value: "dark",
	}

	data := mustMarshal(t, orig)

	var decoded Settings
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, orig.ID)
	}
	if decoded.Key != orig.Key {
		t.Errorf("Key mismatch: got %q, want %q", decoded.Key, orig.Key)
	}
	if decoded.Value != orig.Value {
		t.Errorf("Value mismatch: got %q, want %q", decoded.Value, orig.Value)
	}
}

// ---------------------------------------------------------------------------
// WorkOrder
// ---------------------------------------------------------------------------

func TestWorkOrderJSONRoundTrip(t *testing.T) {
	orig := WorkOrder{
		ID:           "wo-001",
		Type:         "build",
		Status:       "completed",
		Title:        "Build weather tool",
		Description:  "Create a weather MCP tool",
		Requirements: "Must support Celsius and Fahrenheit",
		TargetDir:    "/tools/weather",
		ToolID:       "t-001",
		ThreadID:     "ct-001",
		AgentID:      "a-001",
		Result:       "Tool built successfully",
		CreatedBy:    "user-001",
		CreatedAt:    fixedTime,
		UpdatedAt:    fixedTime.Add(10 * time.Minute),
	}

	data := mustMarshal(t, orig)

	var decoded WorkOrder
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != orig.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, orig.ID)
	}
	if decoded.Title != orig.Title {
		t.Errorf("Title mismatch: got %q, want %q", decoded.Title, orig.Title)
	}
	if decoded.Status != orig.Status {
		t.Errorf("Status mismatch: got %q, want %q", decoded.Status, orig.Status)
	}
	if decoded.Requirements != orig.Requirements {
		t.Errorf("Requirements mismatch: got %q, want %q", decoded.Requirements, orig.Requirements)
	}
	if decoded.Result != orig.Result {
		t.Errorf("Result mismatch: got %q, want %q", decoded.Result, orig.Result)
	}
	if !decoded.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("UpdatedAt mismatch: got %v, want %v", decoded.UpdatedAt, orig.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// Schedule (optional time pointers)
// ---------------------------------------------------------------------------

func TestScheduleOptionalTimesNil(t *testing.T) {
	sched := Schedule{ID: "sch-001", Name: "daily-check"}
	data := mustMarshal(t, sched)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}
	if _, ok := raw["last_run_at"]; ok {
		t.Error("last_run_at should be omitted when nil")
	}
	if _, ok := raw["next_run_at"]; ok {
		t.Error("next_run_at should be omitted when nil")
	}
}

func TestScheduleOptionalTimesPresent(t *testing.T) {
	lastRun := fixedTime
	nextRun := fixedTime.Add(24 * time.Hour)
	sched := Schedule{
		ID:        "sch-002",
		Name:      "hourly-ping",
		LastRunAt: &lastRun,
		NextRunAt: &nextRun,
	}
	data := mustMarshal(t, sched)

	var decoded Schedule
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.LastRunAt == nil {
		t.Fatal("LastRunAt should not be nil")
	}
	if !decoded.LastRunAt.Equal(lastRun) {
		t.Errorf("LastRunAt mismatch: got %v, want %v", *decoded.LastRunAt, lastRun)
	}
	if decoded.NextRunAt == nil {
		t.Fatal("NextRunAt should not be nil")
	}
	if !decoded.NextRunAt.Equal(nextRun) {
		t.Errorf("NextRunAt mismatch: got %v, want %v", *decoded.NextRunAt, nextRun)
	}
}

// ---------------------------------------------------------------------------
// ContextFolder / ContextFile (optional *string pointers)
// ---------------------------------------------------------------------------

func TestContextFolderParentIDNil(t *testing.T) {
	folder := ContextFolder{ID: "cf-001", Name: "Root"}
	data := mustMarshal(t, folder)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}
	if _, ok := raw["parent_id"]; ok {
		t.Error("parent_id should be omitted when nil")
	}
}

func TestContextFolderParentIDPresent(t *testing.T) {
	folder := ContextFolder{ID: "cf-002", Name: "Child", ParentID: refString("cf-001")}
	data := mustMarshal(t, folder)

	var decoded ContextFolder
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.ParentID == nil {
		t.Fatal("ParentID should not be nil")
	}
	if *decoded.ParentID != "cf-001" {
		t.Errorf("ParentID mismatch: got %q, want %q", *decoded.ParentID, "cf-001")
	}
}

func TestContextFileFolderIDNil(t *testing.T) {
	file := ContextFile{ID: "cfi-001", Name: "notes.txt"}
	data := mustMarshal(t, file)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}
	if _, ok := raw["folder_id"]; ok {
		t.Error("folder_id should be omitted when nil")
	}
}

// ---------------------------------------------------------------------------
// HeartbeatExecution (optional *time.Time)
// ---------------------------------------------------------------------------

func TestHeartbeatExecutionFinishedAtOmittedWhenNil(t *testing.T) {
	hb := HeartbeatExecution{ID: "hb-001", Status: "running"}
	data := mustMarshal(t, hb)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}
	if _, ok := raw["finished_at"]; ok {
		t.Error("finished_at should be omitted when nil")
	}
}

// ---------------------------------------------------------------------------
// ScheduleExecution (optional *time.Time)
// ---------------------------------------------------------------------------

func TestScheduleExecutionFinishedAtOmittedWhenNil(t *testing.T) {
	se := ScheduleExecution{ID: "se-001", Status: "running"}
	data := mustMarshal(t, se)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}
	if _, ok := raw["finished_at"]; ok {
		t.Error("finished_at should be omitted when nil")
	}
}

// ---------------------------------------------------------------------------
// Time fields marshal to valid JSON
// ---------------------------------------------------------------------------

func TestTimeFieldsMarshalToValidJSON(t *testing.T) {
	u := User{
		ID:        "u-time",
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime.Add(48 * time.Hour),
	}

	data := mustMarshal(t, u)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	createdStr, ok := raw["created_at"].(string)
	if !ok {
		t.Fatal("created_at should be a JSON string")
	}
	if _, err := time.Parse(time.RFC3339Nano, createdStr); err != nil {
		t.Errorf("created_at is not a valid RFC3339 time: %v", err)
	}

	updatedStr, ok := raw["updated_at"].(string)
	if !ok {
		t.Fatal("updated_at should be a JSON string")
	}
	if _, err := time.Parse(time.RFC3339Nano, updatedStr); err != nil {
		t.Errorf("updated_at is not a valid RFC3339 time: %v", err)
	}
}

// ---------------------------------------------------------------------------
// WebSocket event types
// ---------------------------------------------------------------------------

func TestWSAgentStatusMarshal(t *testing.T) {
	ev := WSAgentStatus{
		ThreadID:      "ct-100",
		Status:        "thinking",
		Message:       "Analyzing request...",
		AgentRoleSlug: "helper",
	}

	data := mustMarshal(t, ev)

	var decoded WSAgentStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ThreadID != ev.ThreadID {
		t.Errorf("ThreadID mismatch: got %q, want %q", decoded.ThreadID, ev.ThreadID)
	}
	if decoded.Status != ev.Status {
		t.Errorf("Status mismatch: got %q, want %q", decoded.Status, ev.Status)
	}
	if decoded.Message != ev.Message {
		t.Errorf("Message mismatch: got %q, want %q", decoded.Message, ev.Message)
	}
	if decoded.AgentRoleSlug != ev.AgentRoleSlug {
		t.Errorf("AgentRoleSlug mismatch: got %q, want %q", decoded.AgentRoleSlug, ev.AgentRoleSlug)
	}
}

func TestWSAgentStatusOmitsEmptyAgentRoleSlug(t *testing.T) {
	ev := WSAgentStatus{ThreadID: "ct-101", Status: "idle"}
	data := mustMarshal(t, ev)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}
	if _, ok := raw["agent_role_slug"]; ok {
		t.Error("agent_role_slug should be omitted when empty")
	}
}

func TestWSThreadUpdatedMarshal(t *testing.T) {
	ev := WSThreadUpdated{
		ThreadID: "ct-200",
		Title:    "Updated Title",
	}

	data := mustMarshal(t, ev)

	var decoded WSThreadUpdated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ThreadID != ev.ThreadID {
		t.Errorf("ThreadID mismatch: got %q, want %q", decoded.ThreadID, ev.ThreadID)
	}
	if decoded.Title != ev.Title {
		t.Errorf("Title mismatch: got %q, want %q", decoded.Title, ev.Title)
	}
}

func TestWSAgentCompletedMarshal(t *testing.T) {
	ev := WSAgentCompleted{
		ThreadID:      "ct-300",
		AgentRoleSlug: "builder",
		AgentID:       "a-100",
		WorkOrderID:   "wo-100",
		Status:        "completed",
		Output:        "Build successful",
	}

	data := mustMarshal(t, ev)

	var decoded WSAgentCompleted
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ThreadID != ev.ThreadID {
		t.Errorf("ThreadID mismatch: got %q, want %q", decoded.ThreadID, ev.ThreadID)
	}
	if decoded.AgentRoleSlug != ev.AgentRoleSlug {
		t.Errorf("AgentRoleSlug mismatch: got %q, want %q", decoded.AgentRoleSlug, ev.AgentRoleSlug)
	}
	if decoded.Output != ev.Output {
		t.Errorf("Output mismatch: got %q, want %q", decoded.Output, ev.Output)
	}
}

func TestWSAgentCompletedOmitsEmptyOptionalFields(t *testing.T) {
	ev := WSAgentCompleted{ThreadID: "ct-301"}
	data := mustMarshal(t, ev)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	for _, field := range []string{"agent_role_slug", "agent_id", "work_order_id", "status", "output"} {
		if _, ok := raw[field]; ok {
			t.Errorf("%s should be omitted when empty", field)
		}
	}
}

func TestWSThreadMemberJoinedMarshal(t *testing.T) {
	ev := WSThreadMemberJoined{
		ThreadID:      "ct-400",
		AgentRoleSlug: "researcher",
		Name:          "Research Bot",
		AvatarPath:    "/img/researcher.png",
	}

	data := mustMarshal(t, ev)

	var decoded WSThreadMemberJoined
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ThreadID != ev.ThreadID {
		t.Errorf("ThreadID mismatch: got %q, want %q", decoded.ThreadID, ev.ThreadID)
	}
	if decoded.Name != ev.Name {
		t.Errorf("Name mismatch: got %q, want %q", decoded.Name, ev.Name)
	}
	if decoded.AvatarPath != ev.AvatarPath {
		t.Errorf("AvatarPath mismatch: got %q, want %q", decoded.AvatarPath, ev.AvatarPath)
	}
}

func TestWSThreadMemberRemovedMarshal(t *testing.T) {
	ev := WSThreadMemberRemoved{
		ThreadID:      "ct-500",
		AgentRoleSlug: "old-agent",
	}

	data := mustMarshal(t, ev)

	var decoded WSThreadMemberRemoved
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ThreadID != ev.ThreadID {
		t.Errorf("ThreadID mismatch: got %q, want %q", decoded.ThreadID, ev.ThreadID)
	}
	if decoded.AgentRoleSlug != ev.AgentRoleSlug {
		t.Errorf("AgentRoleSlug mismatch: got %q, want %q", decoded.AgentRoleSlug, ev.AgentRoleSlug)
	}
}

func TestWSAgentStreamMarshal(t *testing.T) {
	ev := WSAgentStream{
		AgentID:     "a-200",
		WorkOrderID: "wo-200",
		ThreadID:    "ct-600",
		Event:       map[string]string{"type": "progress", "detail": "Step 3 of 5"},
	}

	data := mustMarshal(t, ev)

	var decoded WSAgentStream
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.AgentID != ev.AgentID {
		t.Errorf("AgentID mismatch: got %q, want %q", decoded.AgentID, ev.AgentID)
	}
	if decoded.WorkOrderID != ev.WorkOrderID {
		t.Errorf("WorkOrderID mismatch: got %q, want %q", decoded.WorkOrderID, ev.WorkOrderID)
	}
	if decoded.ThreadID != ev.ThreadID {
		t.Errorf("ThreadID mismatch: got %q, want %q", decoded.ThreadID, ev.ThreadID)
	}
	// Event round-trips as map[string]interface{} via JSON
	eventMap, ok := decoded.Event.(map[string]interface{})
	if !ok {
		t.Fatalf("Event should decode to map[string]interface{}, got %T", decoded.Event)
	}
	if eventMap["type"] != "progress" {
		t.Errorf("Event.type mismatch: got %v, want %q", eventMap["type"], "progress")
	}
}

func TestWSAgentStreamNilEvent(t *testing.T) {
	ev := WSAgentStream{
		AgentID:     "a-201",
		WorkOrderID: "wo-201",
		ThreadID:    "ct-601",
		Event:       nil,
	}

	data := mustMarshal(t, ev)

	var decoded WSAgentStream
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Event != nil {
		t.Errorf("Event should be nil, got %v", decoded.Event)
	}
}

// ---------------------------------------------------------------------------
// Unmarshal from external JSON (simulates API input)
// ---------------------------------------------------------------------------

func TestUnmarshalUserFromExternalJSON(t *testing.T) {
	input := `{
		"id": "ext-001",
		"username": "external",
		"display_name": "External User",
		"avatar_path": "",
		"created_at": "2025-06-15T12:00:00Z",
		"updated_at": "2025-06-15T12:00:00Z"
	}`

	var u User
	if err := json.Unmarshal([]byte(input), &u); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if u.ID != "ext-001" {
		t.Errorf("ID mismatch: got %q, want %q", u.ID, "ext-001")
	}
	if u.Username != "external" {
		t.Errorf("Username mismatch: got %q, want %q", u.Username, "external")
	}
	if u.PasswordHash != "" {
		t.Errorf("PasswordHash should remain empty from JSON input, got %q", u.PasswordHash)
	}
	if !u.CreatedAt.Equal(fixedTime) {
		t.Errorf("CreatedAt mismatch: got %v, want %v", u.CreatedAt, fixedTime)
	}
}

func TestUnmarshalToolWithDeletedAtFromJSON(t *testing.T) {
	input := `{
		"id": "ext-t-001",
		"name": "deleted-tool",
		"description": "",
		"type": "mcp",
		"config": "{}",
		"enabled": false,
		"status": "stopped",
		"port": 0,
		"pid": 0,
		"capabilities": "",
		"owner_agent_slug": "",
		"library_slug": "",
		"library_version": "",
		"source_hash": "",
		"binary_hash": "",
		"created_at": "2025-06-15T12:00:00Z",
		"updated_at": "2025-06-15T12:00:00Z",
		"deleted_at": "2025-06-16T12:00:00Z"
	}`

	var tool Tool
	if err := json.Unmarshal([]byte(input), &tool); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if tool.DeletedAt == nil {
		t.Fatal("DeletedAt should not be nil")
	}

	expected := fixedTime.Add(24 * time.Hour)
	if !tool.DeletedAt.Equal(expected) {
		t.Errorf("DeletedAt mismatch: got %v, want %v", *tool.DeletedAt, expected)
	}
}
