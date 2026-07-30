package userdb

import (
	"bytes"
	"encoding/csv"
	"strings"
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

func TestCSVImportCreatesDatabaseFromHeadersAndRows(t *testing.T) {
	store, db := newTestStore(t)
	workspaceID := createWorkspace(t, db, "CSV")
	source := "\ufeffName,Website,Notes\nOpenPaw,https://openpaw.dev,\"Line one\nLine two\"\nNabu,,Agent\n"

	result, err := store.ImportCSV(workspaceID, "Favorite Sites.csv", strings.NewReader(source))
	if err != nil {
		t.Fatalf("import CSV: %v", err)
	}
	if result.Database.Name != "Favorite Sites" || result.ImportedRows != 2 {
		t.Fatalf("import result = %+v", result)
	}
	if len(result.Database.Tables) != 1 || result.Database.Tables[0].Name != "Favorite Sites" {
		t.Fatalf("imported tables = %+v", result.Database.Tables)
	}
	table := result.Database.Tables[0]
	if got := []string{table.Columns[0].Name, table.Columns[1].Name, table.Columns[2].Name}; got[0] != "Name" || got[1] != "Website" || got[2] != "Notes" {
		t.Fatalf("columns = %#v", got)
	}

	rows, err := store.NamedRows(workspaceID, table.ID, "", 10, 0)
	if err != nil {
		t.Fatalf("read imported rows: %v", err)
	}
	if len(rows.Records) != 2 || rows.Records[0]["Notes"] != "Line one\nLine two" || rows.Records[1]["Website"] != nil {
		t.Fatalf("records = %#v", rows.Records)
	}

	second, err := store.ImportCSV(workspaceID, "Favorite Sites.csv", strings.NewReader("Name\nOther\n"))
	if err != nil {
		t.Fatalf("second import CSV: %v", err)
	}
	if second.Database.Name != "Favorite Sites 2" {
		t.Fatalf("duplicate import name = %q", second.Database.Name)
	}
}

func TestCSVImportNormalizesHeadersAndExportRoundTrips(t *testing.T) {
	store, db := newTestStore(t)
	workspaceID := createWorkspace(t, db, "CSV Round Trip")

	result, err := store.ImportCSV(workspaceID, "data.csv", strings.NewReader("Name,name,,Value\nOne,Two,Three,4\n"))
	if err != nil {
		t.Fatalf("import CSV: %v", err)
	}
	table := result.Database.Tables[0]
	gotHeaders := []string{table.Columns[0].Name, table.Columns[1].Name, table.Columns[2].Name, table.Columns[3].Name}
	wantHeaders := []string{"Name", "name 2", "Column 3", "Value"}
	for index := range wantHeaders {
		if gotHeaders[index] != wantHeaders[index] {
			t.Fatalf("headers = %#v, want %#v", gotHeaders, wantHeaders)
		}
	}

	var output bytes.Buffer
	count, err := store.ExportTableCSV(workspaceID, table.ID, &output)
	if err != nil {
		t.Fatalf("export CSV: %v", err)
	}
	if count != 1 {
		t.Fatalf("exported rows = %d, want 1", count)
	}
	records, err := csv.NewReader(&output).ReadAll()
	if err != nil {
		t.Fatalf("parse export: %v", err)
	}
	if len(records) != 2 || strings.Join(records[0], "|") != strings.Join(wantHeaders, "|") ||
		strings.Join(records[1], "|") != "One|Two|Three|4" {
		t.Fatalf("exported records = %#v", records)
	}
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

	tableName := "Products"
	renamedTable, err := store.UpdateTable(firstWorkspace, created.Tables[0].ID, &tableName)
	if err != nil || renamedTable.Name != tableName {
		t.Fatalf("renamed table = %+v, %v", renamedTable, err)
	}
	refreshed, err := store.GetDatabase(firstWorkspace, created.ID)
	if err != nil {
		t.Fatalf("refresh renamed table: %v", err)
	}
	if len(refreshed.Tables) != 1 || refreshed.Tables[0].ID != created.Tables[0].ID ||
		refreshed.Tables[0].Name != tableName || len(refreshed.Tables[0].Columns) != 1 {
		t.Fatalf("renaming table changed its structure: %+v", refreshed.Tables)
	}
	if _, err := store.UpdateTable(secondWorkspace, created.Tables[0].ID, &tableName); err == nil {
		t.Fatal("table was writable from another workspace")
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

func TestListRowsSortedByTypedColumn(t *testing.T) {
	store, db := newTestStore(t)
	workspaceID := createWorkspace(t, db, "Sorted")
	databaseItem, err := store.CreateDatabase(workspaceID, "Products", "")
	if err != nil {
		t.Fatal(err)
	}
	table := databaseItem.Tables[0]
	nameColumn := table.Columns[0]
	priceColumn, err := store.CreateColumn(workspaceID, table.ID, "Price", "number", nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, values := range []map[string]interface{}{
		{nameColumn.ID: "Zulu", priceColumn.ID: 10},
		{nameColumn.ID: "alpha", priceColumn.ID: 2},
		{nameColumn.ID: "Beta"},
	} {
		if _, err := store.CreateRow(workspaceID, table.ID, values); err != nil {
			t.Fatal(err)
		}
	}

	ascending, err := store.ListRowsSorted(workspaceID, table.ID, "", 20, 0, priceColumn.ID, "asc")
	if err != nil {
		t.Fatal(err)
	}
	if got := []interface{}{
		ascending.Rows[0].Values[priceColumn.ID],
		ascending.Rows[1].Values[priceColumn.ID],
		ascending.Rows[2].Values[priceColumn.ID],
	}; got[0] != float64(2) || got[1] != float64(10) || got[2] != nil {
		t.Fatalf("ascending prices = %#v, want [2 10 nil]", got)
	}

	descending, err := store.ListRowsSorted(workspaceID, table.ID, "", 20, 0, nameColumn.ID, "desc")
	if err != nil {
		t.Fatal(err)
	}
	if got := []interface{}{
		descending.Rows[0].Values[nameColumn.ID],
		descending.Rows[1].Values[nameColumn.ID],
		descending.Rows[2].Values[nameColumn.ID],
	}; got[0] != "Zulu" || got[1] != "Beta" || got[2] != "alpha" {
		t.Fatalf("descending names = %#v, want [Zulu Beta alpha]", got)
	}

	if _, err := store.ListRowsSorted(workspaceID, table.ID, "", 20, 0, priceColumn.ID, "sideways"); err == nil {
		t.Fatal("invalid sort direction was accepted")
	}
	if _, err := store.ListRowsSorted(workspaceID, table.ID, "", 20, 0, "missing-column", "asc"); err == nil {
		t.Fatal("unknown sort column was accepted")
	}
}

func TestListRowsPagination(t *testing.T) {
	store, db := newTestStore(t)
	workspaceID := createWorkspace(t, db, "Paged")
	databaseItem, err := store.CreateDatabase(workspaceID, "People", "")
	if err != nil {
		t.Fatal(err)
	}
	table := databaseItem.Tables[0]
	nameColumn := table.Columns[0]
	for index := 1; index <= 23; index++ {
		if _, err := store.CreateRow(workspaceID, table.ID, map[string]interface{}{
			nameColumn.ID: index,
		}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := store.ListRows(workspaceID, table.ID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.ListRows(workspaceID, table.ID, "", 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 23 || first.Limit != 10 || first.Offset != 0 || len(first.Rows) != 10 {
		t.Fatalf("first page = %+v", first)
	}
	if third.Total != 23 || third.Limit != 10 || third.Offset != 20 || len(third.Rows) != 3 {
		t.Fatalf("third page = %+v", third)
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
