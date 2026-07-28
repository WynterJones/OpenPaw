package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openpaw/openpaw/internal/database"
)

func newSettingsTestManager(t *testing.T) (*Manager, *database.DB) {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, a := range []struct{ id, slug, name string }{
		{"a1", "scout", "Scout"},
		{"a2", "builder-bot", "Builder Bot"},
	} {
		if _, err := db.Exec(
			"INSERT INTO agent_roles (id, slug, name, description, system_prompt, model, enabled) VALUES (?, ?, ?, '', '', 'sonnet', 1)",
			a.id, a.slug, a.name,
		); err != nil {
			t.Fatalf("insert agent role: %v", err)
		}
	}

	return &Manager{db: db, broadcast: func(string, interface{}) {}}, db
}

func invokeSettings(t *testing.T, m *Manager, name string, args map[string]interface{}) (string, bool) {
	t.Helper()
	h, ok := m.MakeAgentSettingsToolHandlers("scout")[name]
	if !ok {
		t.Fatalf("tool %q not found", name)
	}
	raw, _ := json.Marshal(args)
	res := h(context.Background(), "", raw)
	return res.Output, res.IsError
}

func setGlobalHeartbeat(t *testing.T, db *database.DB, enabled string, interval int) {
	t.Helper()
	for key, val := range map[string]string{
		"heartbeat_enabled":      enabled,
		"heartbeat_interval_sec": "900",
	} {
		if _, err := db.Exec(
			"INSERT INTO settings (id, key, value) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
			"t-"+key, key, val,
		); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
}

func TestMySettingsUpdateAppliesHeartbeat(t *testing.T) {
	m, db := newSettingsTestManager(t)
	setGlobalHeartbeat(t, db, "true", 900)

	out, isErr := invokeSettings(t, m, "my_settings_update", map[string]interface{}{
		"heartbeat_enabled":      true,
		"heartbeat_interval_sec": 1800,
	})
	if isErr {
		t.Fatalf("my_settings_update failed: %s", out)
	}

	var enabled bool
	var interval int
	db.QueryRow(
		"SELECT heartbeat_enabled, heartbeat_interval_sec FROM agent_roles WHERE slug = 'scout'",
	).Scan(&enabled, &interval)
	if !enabled || interval != 1800 {
		t.Errorf("saved heartbeat = enabled:%v interval:%d, want true / 1800", enabled, interval)
	}

	// The reply has to carry the previous value too — an agent that only reports
	// the new one cannot tell the user what it changed.
	var result struct {
		Before map[string]interface{} `json:"before"`
		After  map[string]interface{} `json:"after"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode result: %v — %s", err, out)
	}
	if result.Before["heartbeat_enabled"] != false {
		t.Errorf("before.heartbeat_enabled = %v, want false", result.Before["heartbeat_enabled"])
	}
	if result.After["heartbeat_enabled"] != true {
		t.Errorf("after.heartbeat_enabled = %v, want true", result.After["heartbeat_enabled"])
	}
}

// Turning on a per-agent heartbeat while the app-wide one is off does nothing.
// An agent that reports success without saying so has told the user something
// false, and the user only finds out by noticing the check-ins never happen.
func TestMySettingsReportsGlobalHeartbeatOff(t *testing.T) {
	m, db := newSettingsTestManager(t)
	setGlobalHeartbeat(t, db, "false", 900)

	out, isErr := invokeSettings(t, m, "my_settings_update", map[string]interface{}{
		"heartbeat_enabled": true,
	})
	if isErr {
		t.Fatalf("my_settings_update failed: %s", out)
	}

	var result struct {
		HeartbeatEffect string `json:"heartbeat_effect"`
	}
	json.Unmarshal([]byte(out), &result)
	if !strings.Contains(result.HeartbeatEffect, "app-wide heartbeat is switched off") {
		t.Errorf("heartbeat_effect = %q, want it to say the global heartbeat is off", result.HeartbeatEffect)
	}
}

// "Check in every few minutes" is a thing users say, and a model will happily
// turn it into 1 second — which pins the machine running check-ins back to back.
func TestMySettingsClampsHeartbeatInterval(t *testing.T) {
	m, db := newSettingsTestManager(t)

	if out, isErr := invokeSettings(t, m, "my_settings_update", map[string]interface{}{
		"heartbeat_interval_sec": 1,
	}); isErr {
		t.Fatalf("my_settings_update failed: %s", out)
	}
	var interval int
	db.QueryRow("SELECT heartbeat_interval_sec FROM agent_roles WHERE slug = 'scout'").Scan(&interval)
	if interval != 60 {
		t.Errorf("interval = %d, want it clamped up to the 60s floor", interval)
	}

	// 0 is the "inherit the global value" sentinel and must survive clamping.
	if out, isErr := invokeSettings(t, m, "my_settings_update", map[string]interface{}{
		"heartbeat_interval_sec": 0,
	}); isErr {
		t.Fatalf("my_settings_update failed: %s", out)
	}
	db.QueryRow("SELECT heartbeat_interval_sec FROM agent_roles WHERE slug = 'scout'").Scan(&interval)
	if interval != 0 {
		t.Errorf("interval = %d, want 0 preserved as the inherit sentinel", interval)
	}
}

// An agent editing its colleagues turns one agent going wrong into a fleet
// going wrong, and nothing asked for needs it.
func TestMySettingsOnlyTouchesTheCallingAgent(t *testing.T) {
	m, db := newSettingsTestManager(t)

	if out, isErr := invokeSettings(t, m, "my_settings_update", map[string]interface{}{
		"heartbeat_enabled": true,
		"name":              "Scout Renamed",
	}); isErr {
		t.Fatalf("my_settings_update failed: %s", out)
	}

	var otherName string
	var otherHeartbeat bool
	db.QueryRow(
		"SELECT name, heartbeat_enabled FROM agent_roles WHERE slug = 'builder-bot'",
	).Scan(&otherName, &otherHeartbeat)
	if otherName != "Builder Bot" || otherHeartbeat {
		t.Errorf("another agent was modified: name=%q heartbeat=%v", otherName, otherHeartbeat)
	}

	// The slug is the key every schedule, thread membership and memory database
	// is filed under, so a rename must not move it.
	var slug string
	db.QueryRow("SELECT slug FROM agent_roles WHERE id = 'a1'").Scan(&slug)
	if slug != "scout" {
		t.Errorf("slug changed to %q — schedules and memory keyed to the old one would be orphaned", slug)
	}
}

// An agent that switches itself off cannot be switched back on by the
// conversation it was having.
func TestMySettingsCannotDisableSelf(t *testing.T) {
	m, db := newSettingsTestManager(t)

	// `enabled` is not in the schema, so passing it must be ignored rather than
	// silently written.
	invokeSettings(t, m, "my_settings_update", map[string]interface{}{
		"enabled":           false,
		"heartbeat_enabled": true,
	})

	var enabled bool
	db.QueryRow("SELECT enabled FROM agent_roles WHERE slug = 'scout'").Scan(&enabled)
	if !enabled {
		t.Error("the agent disabled itself")
	}

	for _, def := range BuildAgentSettingsToolDefs() {
		if strings.Contains(string(def.Function.Parameters), `"enabled"`) {
			t.Errorf("%s advertises an `enabled` parameter", def.Function.Name)
		}
		if strings.Contains(string(def.Function.Parameters), `"slug"`) {
			t.Errorf("%s advertises a `slug` parameter", def.Function.Name)
		}
		if strings.Contains(string(def.Function.Parameters), `"system_prompt"`) {
			t.Errorf("%s advertises a `system_prompt` parameter", def.Function.Name)
		}
	}
}

func TestMySettingsUpdateRejectsEmptyPayload(t *testing.T) {
	m, _ := newSettingsTestManager(t)
	if out, isErr := invokeSettings(t, m, "my_settings_update", map[string]interface{}{}); !isErr {
		t.Errorf("expected an empty update to be refused, got: %s", out)
	}
}

func TestMySettingsGetReportsGlobalContext(t *testing.T) {
	m, db := newSettingsTestManager(t)
	setGlobalHeartbeat(t, db, "true", 900)

	out, isErr := invokeSettings(t, m, "my_settings_get", map[string]interface{}{})
	if isErr {
		t.Fatalf("my_settings_get failed: %s", out)
	}

	var result struct {
		Me              agentSettingsView      `json:"me"`
		GlobalHeartbeat map[string]interface{} `json:"global_heartbeat"`
		HeartbeatEffect string                 `json:"heartbeat_effect"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode result: %v — %s", err, out)
	}
	if result.Me.Slug != "scout" {
		t.Errorf("reported slug = %q, want the calling agent", result.Me.Slug)
	}
	if result.GlobalHeartbeat["enabled"] != true {
		t.Errorf("global heartbeat not reported: %+v", result.GlobalHeartbeat)
	}
	if result.HeartbeatEffect == "" {
		t.Error("heartbeat_effect was empty — the agent has nothing plain to tell the user")
	}
}
