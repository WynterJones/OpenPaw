package agents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openpaw/openpaw/internal/database"
)

// fakeToolMgr records the lifecycle calls made against it and lets each one be
// made to fail, so the order of stop/compile/start can be asserted.
type fakeToolMgr struct {
	calls     []string
	running   bool
	startErr  error
	healthErr error
	port      int
}

func (f *fakeToolMgr) CompileTool(string) error {
	f.calls = append(f.calls, "compile")
	return nil
}

func (f *fakeToolMgr) StartTool(string) error {
	f.calls = append(f.calls, "start")
	if f.startErr != nil {
		return f.startErr
	}
	f.running = true
	return nil
}

func (f *fakeToolMgr) StopTool(string) error {
	f.calls = append(f.calls, "stop")
	f.running = false
	return nil
}

func (f *fakeToolMgr) RestartTool(string) error {
	f.calls = append(f.calls, "restart")
	if f.startErr != nil {
		return f.startErr
	}
	f.running = true
	return nil
}

func (f *fakeToolMgr) WaitForHealth(string, time.Duration) error { return f.healthErr }

func (f *fakeToolMgr) GetStatus(string) map[string]interface{} {
	state := "stopped"
	if f.running {
		state = "running"
	}
	return map[string]interface{}{"status": state, "port": f.port}
}

func (f *fakeToolMgr) CallTool(string, string, []byte) ([]byte, error) { return nil, nil }
func (f *fakeToolMgr) CallToolWithContext(context.Context, string, string, []byte) ([]byte, error) {
	return nil, nil
}
func (f *fakeToolMgr) FetchFile(string, string) ([]byte, string, error) { return nil, "", nil }

func newServiceTestManager(t *testing.T, mgr *fakeToolMgr) (*Manager, *database.DB, string) {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	toolsDir := t.TempDir()
	m := NewManager(db, toolsDir, func(string, interface{}) {}, nil)
	m.ToolMgr = mgr

	if _, err := db.Exec(
		"INSERT INTO tools (id, name, description, type, config, enabled, status) VALUES ('tool-1', 'Weather Service', '', 'custom', '{}', 1, 'running')",
	); err != nil {
		t.Fatalf("insert tool: %v", err)
	}
	return m, db, toolsDir
}

func callServiceControl(t *testing.T, m *Manager, service, action string) string {
	t.Helper()
	handler := m.MakeServiceControlHandlers(database.DefaultWorkspaceID)["service_control"]
	input, _ := json.Marshal(map[string]string{"service": service, "action": action})
	return handler(context.Background(), "", input).Output
}

func TestServiceHandlersRejectAnotherWorkspace(t *testing.T) {
	fake := &fakeToolMgr{}
	m, db, _ := newServiceTestManager(t, fake)
	otherWorkspace := "11111111-1111-1111-1111-111111111111"
	if _, err := db.Exec(
		"INSERT INTO workspaces (id, name) VALUES (?, 'Other')", otherWorkspace,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO tools (id, name, description, type, config, enabled, status, workspace_id)
		 VALUES ('other-tool', 'Private Other Service', '', 'custom', '{}', 1, 'running', ?)`,
		otherWorkspace,
	); err != nil {
		t.Fatal(err)
	}

	control := m.MakeServiceControlHandlers(database.DefaultWorkspaceID)["service_control"]
	input, _ := json.Marshal(map[string]string{"service": "other-tool", "action": "status"})
	result := control(context.Background(), "", input)
	if !result.IsError || !strings.Contains(result.Output, "No service") {
		t.Fatalf("cross-workspace service control result = %#v", result)
	}

	call := m.makeCallToolHandler(database.DefaultWorkspaceID)
	callInput, _ := json.Marshal(map[string]string{
		"tool_id": "other-tool", "endpoint": "/private", "method": "GET",
	})
	result = call(context.Background(), "", callInput)
	if !result.IsError || !strings.Contains(result.Output, "not available in this workspace") {
		t.Fatalf("cross-workspace service call result = %#v", result)
	}

	prompt := m.buildToolsPromptSection("", database.DefaultWorkspaceID)
	if strings.Contains(prompt, "Private Other Service") {
		t.Fatalf("cross-workspace service leaked into prompt: %s", prompt)
	}
}

// Rebuilding a service that is still running failed on the port the old process
// held: "Build and compile succeeded but failed to start: service already
// running". The old binary kept serving, so a successful update was reported as
// a failure and silently never took effect.
func TestPostBuild_StopsTheOldProcessBeforeRebuilding(t *testing.T) {
	fake := &fakeToolMgr{running: true, port: 9106}
	m, _, toolsDir := newServiceTestManager(t, fake)
	if err := os.MkdirAll(filepath.Join(toolsDir, "tool-1"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	m.postBuildLifecycle("tool-1", filepath.Join(toolsDir, "tool-1"), "wo-1", "")

	want := []string{"stop", "compile", "start"}
	if strings.Join(fake.calls, ",") != strings.Join(want, ",") {
		t.Errorf("lifecycle = %v, want %v", fake.calls, want)
	}
}

// A service that will not start is unactionable without its own output: "exit
// status 1" says nothing about the missing secret behind it.
func TestServiceControl_ReportsTheServiceLogOnFailure(t *testing.T) {
	fake := &fakeToolMgr{startErr: errFailedToStart}
	m, _, toolsDir := newServiceTestManager(t, fake)

	dir := filepath.Join(toolsDir, "tool-1")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tool.log"), []byte("panic: OPENWEATHER_KEY is not set\n"), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	out := callServiceControl(t, m, "Weather Service", "restart")
	if !strings.Contains(out, "OPENWEATHER_KEY is not set") {
		t.Errorf("failure did not include the service log:\n%s", out)
	}
}

// Recompiling has to stop the old process first — it holds both the port the
// new one needs and the binary the compiler is about to overwrite.
func TestServiceControl_RecompileStopsFirst(t *testing.T) {
	fake := &fakeToolMgr{running: true, port: 9106}
	m, _, _ := newServiceTestManager(t, fake)

	callServiceControl(t, m, "tool-1", "recompile")

	want := []string{"stop", "compile", "start"}
	if strings.Join(fake.calls, ",") != strings.Join(want, ",") {
		t.Errorf("recompile = %v, want %v", fake.calls, want)
	}
}

// "Restarted" on a process that came up and immediately fell over is how a
// broken service gets described to the user as fixed.
func TestServiceControl_UnhealthyRestartIsNotReportedAsSuccess(t *testing.T) {
	fake := &fakeToolMgr{running: true, port: 9106, healthErr: errNoHealth}
	m, _, _ := newServiceTestManager(t, fake)

	out := callServiceControl(t, m, "Weather Service", "restart")
	if !strings.Contains(out, "not answering its health check") {
		t.Errorf("unhealthy restart reported as clean:\n%s", out)
	}
}

func TestServiceControl_ResolvesByNameAndRejectsUnknown(t *testing.T) {
	fake := &fakeToolMgr{running: true, port: 9106}
	m, _, _ := newServiceTestManager(t, fake)

	if out := callServiceControl(t, m, "weather service", "status"); !strings.Contains(out, "Weather Service is running") {
		t.Errorf("case-insensitive name did not resolve: %s", out)
	}
	if out := callServiceControl(t, m, "Nonexistent", "status"); !strings.Contains(out, "No service named") {
		t.Errorf("unknown service was not rejected: %s", out)
	}
}

var (
	errFailedToStart = &staticErr{"service already running on port 9106"}
	errNoHealth      = &staticErr{"health check timed out after 10s"}
)

type staticErr struct{ msg string }

func (e *staticErr) Error() string { return e.msg }
