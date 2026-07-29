package userdb

import (
	"testing"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
)

func newTestStore(t *testing.T) (*Store, *database.DB) {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db), db
}

func createWorkspace(t *testing.T, db *database.DB, name string) string {
	t.Helper()
	id := uuid.New().String()
	if _, err := db.Exec("INSERT INTO workspaces (id, name) VALUES (?, ?)", id, name); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	return id
}

func TestDatabaseCRUDAndWorkspaceIsolation(t *testing.T) {
	store, db := newTestStore(t)
	firstWorkspace := createWorkspace(t, db, "First")
	secondWorkspace := createWorkspace(t, db, "Second")

	created, err := store.CreateDatabase(firstWorkspace, "Projects", "Active projects")
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	if len(created.Tables) != 1 || len(created.Tables[0].Columns) != 1 {
		t.Fatalf("new database = %+v, want one starter table and column", created)
	}

	first, err := store.ListDatabases(firstWorkspace)
	if err != nil || len(first) != 1 {
		t.Fatalf("first workspace list = %+v, %v", first, err)
	}
	second, err := store.ListDatabases(secondWorkspace)
	if err != nil || len(second) != 0 {
		t.Fatalf("second workspace leaked rows = %+v, %v", second, err)
	}
	if _, err := store.GetDatabase(secondWorkspace, created.ID); err == nil {
		t.Fatal("database was readable from another workspace")
	}

	renamed := "Client Projects"
	updated, err := store.UpdateDatabase(firstWorkspace, created.ID, &renamed, nil)
	if err != nil || updated.Name != renamed {
		t.Fatalf("updated database = %+v, %v", updated, err)
	}
	if err := store.DeleteDatabase(firstWorkspace, created.ID); err != nil {
		t.Fatalf("delete database: %v", err)
	}
}

func TestColumnsRowsSearchAndNamedProjection(t *testing.T) {
	store, db := newTestStore(t)
	workspaceID := createWorkspace(t, db, "Projects")
	databaseItem, err := store.CreateDatabase(workspaceID, "Projects", "")
	if err != nil {
		t.Fatal(err)
	}
	table := databaseItem.Tables[0]
	nameColumn := table.Columns[0]
	statusColumn, err := store.CreateColumn(workspaceID, table.ID, "Status", "select", map[string]interface{}{
		"choices": []interface{}{"Idea", "Active", "Done"},
	})
	if err != nil {
		t.Fatal(err)
	}
	budgetColumn, err := store.CreateColumn(workspaceID, table.ID, "Budget", "number", nil)
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.CreateRow(workspaceID, table.ID, map[string]interface{}{
		nameColumn.ID:   "OpenPaw",
		statusColumn.ID: "Active",
		budgetColumn.ID: 1250,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateRow(workspaceID, table.ID, map[string]interface{}{
		nameColumn.ID:   "Old site",
		statusColumn.ID: "Done",
	})
	if err != nil {
		t.Fatal(err)
	}

	page, err := store.ListRows(workspaceID, table.ID, "openpaw", 20, 0)
	if err != nil || page.Total != 1 || page.Rows[0].ID != first.ID {
		t.Fatalf("search page = %+v, %v", page, err)
	}

	named, err := store.NamedRows(workspaceID, table.ID, "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(named.Records) != 2 || named.Records[0]["Name"] != "OpenPaw" || named.Records[0]["Status"] != "Active" {
		t.Fatalf("named projection = %+v", named)
	}
	if len(named.Rows) != 2 || len(named.Columns) != 3 {
		t.Fatalf("table projection = %+v", named)
	}

	if err := store.DeleteColumn(workspaceID, budgetColumn.ID); err != nil {
		t.Fatal(err)
	}
	afterDelete, err := store.ListRows(workspaceID, table.ID, "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := afterDelete.Rows[0].Values[budgetColumn.ID]; exists {
		t.Fatal("deleted column value remained in row JSON")
	}
}

func TestColumnNamesForAgentRows(t *testing.T) {
	store, db := newTestStore(t)
	workspaceID := createWorkspace(t, db, "Agent")
	databaseItem, _ := store.CreateDatabase(workspaceID, "Links", "")
	table := databaseItem.Tables[0]
	urlColumn, _ := store.CreateColumn(workspaceID, table.ID, "URL", "url", nil)

	values, err := store.ColumnIDsForNames(workspaceID, table.ID, map[string]interface{}{
		"name": "OpenPaw",
		"url":  "https://openpaw.dev",
	})
	if err != nil {
		t.Fatal(err)
	}
	if values[table.Columns[0].ID] != "OpenPaw" || values[urlColumn.ID] != "https://openpaw.dev" {
		t.Fatalf("converted values = %+v", values)
	}
	if _, err := store.ColumnIDsForNames(workspaceID, table.ID, map[string]interface{}{"Missing": "x"}); err == nil {
		t.Fatal("unknown column name was accepted")
	}
}
