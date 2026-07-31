package terminal

import (
	"testing"

	"github.com/openpaw/openpaw/internal/database"
)

func newWorkbenchTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func setActiveWorkspace(t *testing.T, db *database.DB, id string) {
	t.Helper()
	if _, err := db.Exec(
		"INSERT INTO settings (key, value) VALUES ('active_workspace_id', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		id,
	); err != nil {
		t.Fatalf("set active workspace: %v", err)
	}
}

func workbenchNames(t *testing.T, m *Manager) []string {
	t.Helper()
	wbs, err := m.ListWorkbenches()
	if err != nil {
		t.Fatalf("list workbenches: %v", err)
	}
	names := make([]string, 0, len(wbs))
	for _, wb := range wbs {
		names = append(names, wb.Name)
	}
	return names
}

// Terminal navigation is global. Changing the active data workspace must leave
// the same workbench tabs available instead of replacing a live terminal with
// the new-terminal screen.
func TestWorkbenches_RemainVisibleAcrossWorkspaceSwitches(t *testing.T) {
	db := newWorkbenchTestDB(t)
	m := NewManager(db, t.TempDir())

	const other = "11111111-1111-1111-1111-111111111111"

	// Created in the default workspace.
	if _, err := m.CreateWorkbench("Client A"); err != nil {
		t.Fatalf("create: %v", err)
	}

	setActiveWorkspace(t, db, other)
	if got := workbenchNames(t, m); len(got) != 1 || got[0] != "Client A" {
		t.Errorf("after workspace switch got %v, want [Client A]", got)
	}

	if _, err := m.CreateWorkbench("Side Project"); err != nil {
		t.Fatalf("create in other workspace: %v", err)
	}
	if got := workbenchNames(t, m); len(got) != 2 || got[0] != "Client A" || got[1] != "Side Project" {
		t.Errorf("got %v, want both global workbenches", got)
	}

	// Switching back leaves the exact same terminal navigation in place.
	setActiveWorkspace(t, db, database.DefaultWorkspaceID)
	if got := workbenchNames(t, m); len(got) != 2 || got[0] != "Client A" || got[1] != "Side Project" {
		t.Errorf("switching back changed global workbenches: %v", got)
	}
}

func TestEnsureDefaultWorkbench_IsGlobal(t *testing.T) {
	db := newWorkbenchTestDB(t)
	m := NewManager(db, t.TempDir())

	first, err := m.EnsureDefaultWorkbench()
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}

	// Idempotent within a workspace.
	again, err := m.EnsureDefaultWorkbench()
	if err != nil {
		t.Fatalf("ensure again: %v", err)
	}
	if again.ID != first.ID {
		t.Errorf("made a second workbench (%s then %s) for one workspace", first.ID, again.ID)
	}

	setActiveWorkspace(t, db, "22222222-2222-2222-2222-222222222222")
	other, err := m.EnsureDefaultWorkbench()
	if err != nil {
		t.Fatalf("ensure in other workspace: %v", err)
	}
	if other.ID != first.ID {
		t.Errorf("workspace switch created a second default workbench (%s, want %s)", other.ID, first.ID)
	}
}

// Legacy rows predate the workspace column being written and can hold NULL.
// Migration 067 pins them to the Default workspace; without it they would match
// no workspace at all and their terminals would drop out of the UI.
func TestListWorkbenches_LegacyRowsLandInDefault(t *testing.T) {
	db := newWorkbenchTestDB(t)
	m := NewManager(db, t.TempDir())

	if _, err := db.Exec(
		"INSERT INTO workbenches (id, name, workspace_id) VALUES ('legacy', 'Old', NULL)",
	); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	// Re-run what the migration does, since migrations ran before this insert.
	if _, err := db.Exec(
		"UPDATE workbenches SET workspace_id = ? WHERE workspace_id IS NULL OR workspace_id = ''",
		database.DefaultWorkspaceID,
	); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	if got := workbenchNames(t, m); len(got) != 1 || got[0] != "Old" {
		t.Errorf("got %v, want the legacy workbench in the default workspace", got)
	}
}
