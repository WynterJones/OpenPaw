package terminal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openpaw/openpaw/internal/database"
)

func TestOpenSessionsRestoreAfterManagerRestart(t *testing.T) {
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer db.Close()

	defaultDir := t.TempDir()
	sessionDir := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(sessionDir, 0o755); err != nil {
		t.Fatalf("create session directory: %v", err)
	}

	first := NewManager(db, defaultDir)
	workbench, err := first.EnsureDefaultWorkbench()
	if err != nil {
		t.Fatalf("ensure workbench: %v", err)
	}

	created, err := first.CreateSession(
		"Project shell",
		101,
		37,
		"#ec4899",
		workbench.ID,
		sessionDir,
		"",
	)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	first.Shutdown()

	var saved int
	if err := db.QueryRow("SELECT COUNT(*) FROM terminal_sessions WHERE id = ?", created.ID).Scan(&saved); err != nil {
		t.Fatalf("count saved session: %v", err)
	}
	if saved != 1 {
		t.Fatalf("saved rows = %d, want 1", saved)
	}

	second := NewManager(db, defaultDir)
	defer second.Shutdown()

	restored := second.GetSession(created.ID)
	if restored == nil {
		t.Fatal("open terminal was not restored")
	}
	if restored.Title != "Project shell" {
		t.Errorf("title = %q, want Project shell", restored.Title)
	}
	if restored.Cwd != sessionDir {
		t.Errorf("cwd = %q, want %q", restored.Cwd, sessionDir)
	}
	if restored.Cols != 101 || restored.Rows != 37 {
		t.Errorf("size = %dx%d, want 101x37", restored.Cols, restored.Rows)
	}
	if restored.Color != "#ec4899" {
		t.Errorf("color = %q, want #ec4899", restored.Color)
	}
	if restored.WorkbenchID != workbench.ID {
		t.Errorf("workbench = %q, want %q", restored.WorkbenchID, workbench.ID)
	}

	if err := second.DestroySession(created.ID); err != nil {
		t.Fatalf("close restored session: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM terminal_sessions WHERE id = ?", created.ID).Scan(&saved); err != nil {
		t.Fatalf("count closed session: %v", err)
	}
	if saved != 0 {
		t.Errorf("explicitly closed session left %d saved rows", saved)
	}
}
