package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/userdb"
)

func TestExportDatabases(t *testing.T) {
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	store := userdb.NewStore(db)
	created, err := store.CreateDatabase(database.DefaultWorkspaceID, "Bookmarks", "Favorite sites")
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	table := created.Tables[0]
	urlColumn, err := store.CreateColumn(database.DefaultWorkspaceID, table.ID, "URL", "url", nil)
	if err != nil {
		t.Fatalf("create column: %v", err)
	}
	if _, err := store.CreateRow(database.DefaultWorkspaceID, table.ID, map[string]interface{}{
		table.Columns[0].ID: "OpenPaw",
		urlColumn.ID:        "https://openpaw.dev",
	}); err != nil {
		t.Fatalf("create row: %v", err)
	}

	destDir := t.TempDir()
	files, databases, tables, columns, rows, err := exportDatabases(db, destDir)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(files) != 1 || databases != 1 || tables != 1 || columns != 2 || rows != 1 {
		t.Fatalf("counts = files:%d databases:%d tables:%d columns:%d rows:%d", len(files), databases, tables, columns, rows)
	}

	raw, err := os.ReadFile(filepath.Join(destDir, "databases.json"))
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	var exported struct {
		Databases []map[string]interface{} `json:"databases"`
		Columns   []map[string]interface{} `json:"columns"`
		Rows      []struct {
			Values map[string]interface{} `json:"values"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &exported); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if exported.Databases[0]["name"] != "Bookmarks" || exported.Columns[1]["name"] != "URL" {
		t.Fatalf("unexpected metadata: %#v %#v", exported.Databases, exported.Columns)
	}
	if exported.Rows[0].Values[urlColumn.ID] != "https://openpaw.dev" {
		t.Fatalf("unexpected row values: %#v", exported.Rows[0].Values)
	}
}
