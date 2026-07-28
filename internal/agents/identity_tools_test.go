package agents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openpaw/openpaw/internal/database"
)

func newIdentityTestManager(t *testing.T) (*Manager, *database.DB, string) {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(
		"INSERT INTO agent_roles (id, slug, name, description, system_prompt, model, enabled, identity_initialized) VALUES ('a1', 'scout', 'Scout', '', '', 'sonnet', 1, 1)",
	); err != nil {
		t.Fatalf("insert agent role: %v", err)
	}

	dataDir := t.TempDir()
	m := &Manager{db: db, DataDir: dataDir, broadcast: func(string, interface{}) {}}
	agentDir := AgentDir(dataDir, "scout")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("create agent dir: %v", err)
	}
	return m, db, agentDir
}

func invokeIdentity(t *testing.T, m *Manager, name string, args map[string]interface{}) (string, bool) {
	t.Helper()
	h, ok := m.MakeIdentityToolHandlers("scout")[name]
	if !ok {
		t.Fatalf("tool %q not found", name)
	}
	raw, _ := json.Marshal(args)
	res := h(context.Background(), "", raw)
	return res.Output, res.IsError
}

func TestIdentityWriteAndReadRoundTrip(t *testing.T) {
	m, _, agentDir := newIdentityTestManager(t)

	body := "# Runbook\n\n## Lessons Learned\n- Ship the frontend before the binary.\n"
	if out, isErr := invokeIdentity(t, m, "my_identity_write", map[string]interface{}{
		"file": FileRunbook, "content": body,
	}); isErr {
		t.Fatalf("write failed: %s", out)
	}

	onDisk, err := os.ReadFile(filepath.Join(agentDir, FileRunbook))
	if err != nil {
		t.Fatalf("file was not written: %v", err)
	}
	if string(onDisk) != body {
		t.Errorf("on-disk content = %q, want %q", onDisk, body)
	}

	out, isErr := invokeIdentity(t, m, "my_identity_read", map[string]interface{}{"file": FileRunbook})
	if isErr {
		t.Fatalf("read failed: %s", out)
	}
	var got struct {
		Content string `json:"content"`
		Empty   bool   `json:"empty"`
		Purpose string `json:"purpose"`
	}
	json.Unmarshal([]byte(out), &got)
	if got.Content != body || got.Empty {
		t.Errorf("read back %q (empty=%v)", got.Content, got.Empty)
	}
	if got.Purpose == "" {
		t.Error("read returned no purpose, so the agent cannot explain the file to the user")
	}
}

// Append is the mode that matters for a runbook: an agent adding one lesson
// should not have to restate everything it already knew.
func TestIdentityWriteAppendPreservesExisting(t *testing.T) {
	m, _, agentDir := newIdentityTestManager(t)

	if err := os.WriteFile(filepath.Join(agentDir, FileRunbook), []byte("# Runbook\n\nOriginal rule.\n"), 0644); err != nil {
		t.Fatalf("seed runbook: %v", err)
	}

	if out, isErr := invokeIdentity(t, m, "my_identity_write", map[string]interface{}{
		"file": FileRunbook, "content": "New lesson.", "mode": "append",
	}); isErr {
		t.Fatalf("append failed: %s", out)
	}

	onDisk, _ := os.ReadFile(filepath.Join(agentDir, FileRunbook))
	if !strings.Contains(string(onDisk), "Original rule.") {
		t.Error("append discarded the existing content")
	}
	if !strings.Contains(string(onDisk), "New lesson.") {
		t.Error("append did not add the new content")
	}
}

// The enum in a tool schema is a hint, not a guarantee — models pass values
// outside it, and a filename is one "../" away from an arbitrary write to disk.
func TestIdentityToolsRefuseFilesOutsideTheAllowlist(t *testing.T) {
	m, _, agentDir := newIdentityTestManager(t)

	for _, name := range []string{
		"../../../etc/passwd",
		"../other-agent/SOUL.md",
		"/etc/hosts",
		"skills/evil/SKILL.md",
		"GOAL.md", // real file, but the gateway's — not this agent's to write
		"",
	} {
		if out, isErr := invokeIdentity(t, m, "my_identity_write", map[string]interface{}{
			"file": name, "content": "owned",
		}); !isErr {
			t.Errorf("write to %q was allowed: %s", name, out)
		}
		if out, isErr := invokeIdentity(t, m, "my_identity_read", map[string]interface{}{
			"file": name,
		}); !isErr {
			t.Errorf("read of %q was allowed: %s", name, out)
		}
	}

	// Nothing should have escaped the agent's own directory.
	if entries, err := os.ReadDir(agentDir); err == nil {
		for _, e := range entries {
			if identityFilePurpose(e.Name()) == "" {
				t.Errorf("unexpected file created in the agent dir: %s", e.Name())
			}
		}
	}
}

// These files are concatenated into the system prompt on every message, so an
// agent that writes an essay into one pays for it on every request forever.
func TestIdentityWriteEnforcesSizeCap(t *testing.T) {
	m, _, agentDir := newIdentityTestManager(t)

	out, isErr := invokeIdentity(t, m, "my_identity_write", map[string]interface{}{
		"file": FileSoul, "content": strings.Repeat("x", identityFileWriteCap+1),
	})
	if !isErr {
		t.Fatalf("oversized write was allowed: %s", out)
	}
	if _, err := os.Stat(filepath.Join(agentDir, FileSoul)); !os.IsNotExist(err) {
		t.Error("the oversized content was written despite the refusal")
	}
}

// A heartbeat needs instructions AND the setting. Reporting only the half you
// just did leaves the user believing something is running when it is not.
func TestIdentityWriteHeartbeatWarnsWhenSettingIsOff(t *testing.T) {
	m, db, _ := newIdentityTestManager(t)

	out, isErr := invokeIdentity(t, m, "my_identity_write", map[string]interface{}{
		"file": FileHeartbeat, "content": "Check the deploy queue and report anything stuck.",
	})
	if isErr {
		t.Fatalf("write failed: %s", out)
	}
	var got struct {
		Warning string `json:"heartbeat_warning"`
	}
	json.Unmarshal([]byte(out), &got)
	if !strings.Contains(got.Warning, "OFF") {
		t.Errorf("heartbeat_warning = %q, want it to say the setting is off", got.Warning)
	}

	// With the setting on, the warning should turn into a confirmation.
	db.Exec("UPDATE agent_roles SET heartbeat_enabled = 1 WHERE slug = 'scout'")
	out, _ = invokeIdentity(t, m, "my_identity_write", map[string]interface{}{
		"file": FileHeartbeat, "content": "Check the deploy queue.",
	})
	var got2 struct {
		Warning string `json:"heartbeat_warning"`
		Note    string `json:"heartbeat_note"`
	}
	json.Unmarshal([]byte(out), &got2)
	if got2.Warning != "" || got2.Note == "" {
		t.Errorf("with the setting on, got warning=%q note=%q", got2.Warning, got2.Note)
	}
}

// The empty heartbeat file is the failure that produces no error anywhere the
// user looks — the wake-up happens, reads nothing, and records a skip.
func TestIdentityListFlagsEmptyHeartbeat(t *testing.T) {
	m, _, agentDir := newIdentityTestManager(t)

	out, isErr := invokeIdentity(t, m, "my_identity_list", nil)
	if isErr {
		t.Fatalf("list failed: %s", out)
	}
	var got struct {
		Files   []identityFileState `json:"files"`
		Warning string              `json:"heartbeat_warning"`
	}
	json.Unmarshal([]byte(out), &got)

	if len(got.Files) != len(identityFileDocs) {
		t.Fatalf("listed %d files, want %d", len(got.Files), len(identityFileDocs))
	}
	if got.Warning == "" {
		t.Error("an empty HEARTBEAT.md was not flagged")
	}
	for _, f := range got.Files {
		if f.Purpose == "" {
			t.Errorf("%s listed with no purpose", f.Name)
		}
	}

	// Once written, the warning goes away.
	os.WriteFile(filepath.Join(agentDir, FileHeartbeat), []byte("Check things."), 0644)
	out, _ = invokeIdentity(t, m, "my_identity_list", nil)
	got.Warning = ""
	json.Unmarshal([]byte(out), &got)
	if got.Warning != "" {
		t.Errorf("warning persisted after the file was filled: %q", got.Warning)
	}
}

// The three conditions a working heartbeat needs, each of which fails silently.
func TestHeartbeatEffectCoversAllThreeConditions(t *testing.T) {
	on := map[string]interface{}{"enabled": true, "interval_sec": 900}
	off := map[string]interface{}{"enabled": false, "interval_sec": 900}
	agentOn := agentSettingsView{HeartbeatEnabled: true, HeartbeatIntervalSec: 1800}

	if got := heartbeatEffect(agentSettingsView{}, on, false); !strings.Contains(got, "heartbeat is off") {
		t.Errorf("agent switch off: %q", got)
	}
	if got := heartbeatEffect(agentOn, off, false); !strings.Contains(got, "app-wide") {
		t.Errorf("global switch off: %q", got)
	}
	if got := heartbeatEffect(agentOn, on, true); !strings.Contains(got, "HEARTBEAT.md is empty") {
		t.Errorf("empty instructions: %q", got)
	}
	if got := heartbeatEffect(agentOn, on, false); !strings.Contains(got, "1800") {
		t.Errorf("all three satisfied: %q", got)
	}
}

// A missing file has to count as empty — that is what the heartbeat itself does
// with it, and disagreeing would report a working heartbeat that skips.
func TestHeartbeatInstructionsEmptyTreatsMissingAsEmpty(t *testing.T) {
	m, _, agentDir := newIdentityTestManager(t)

	if !m.heartbeatInstructionsEmpty("scout") {
		t.Error("a missing HEARTBEAT.md was not treated as empty")
	}
	os.WriteFile(filepath.Join(agentDir, FileHeartbeat), []byte("   \n\t\n"), 0644)
	if !m.heartbeatInstructionsEmpty("scout") {
		t.Error("a whitespace-only HEARTBEAT.md was not treated as empty")
	}
	os.WriteFile(filepath.Join(agentDir, FileHeartbeat), []byte("Check the queue."), 0644)
	if m.heartbeatInstructionsEmpty("scout") {
		t.Error("a filled HEARTBEAT.md was reported as empty")
	}
}
