package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/llm"
)

func newDatabaseToolsTestDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	workspaceID := uuid.New().String()
	if _, err := db.Exec("INSERT INTO workspaces (id, name) VALUES (?, 'Test')", workspaceID); err != nil {
		t.Fatal(err)
	}
	return db, workspaceID
}

func callDatabaseTool(t *testing.T, handlers map[string]llm.ToolHandler, name, input string) llm.ToolResult {
	t.Helper()
	handler := handlers[name]
	if handler == nil {
		t.Fatalf("tool %q is not registered", name)
	}
	return handler(context.Background(), "", json.RawMessage(input))
}

func TestDatabaseToolsCreateQueryAndMutateRows(t *testing.T) {
	db, workspaceID := newDatabaseToolsTestDB(t)
	var broadcasts int
	handlers := MakeDatabaseToolHandlers(db, workspaceID, "reporter", func(kind string, _ interface{}) {
		if kind == "database_updated" {
			broadcasts++
		}
	})

	created := callDatabaseTool(t, handlers, "create_database", `{
		"name":"Projects",
		"tables":[{
			"name":"Projects",
			"columns":[
				{"name":"Name","type":"text"},
				{"name":"Status","type":"select","options":{"choices":["Idea","Active","Done"]}},
				{"name":"Budget","type":"number"}
			]
		}]
	}`)
	if created.IsError {
		t.Fatalf("create failed: %s", created.Output)
	}

	var databaseItem struct {
		ID     string `json:"id"`
		Tables []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"tables"`
	}
	if err := json.Unmarshal([]byte(created.Output), &databaseItem); err != nil {
		t.Fatal(err)
	}
	if databaseItem.ID == "" || len(databaseItem.Tables) != 1 {
		t.Fatalf("created database = %s", created.Output)
	}
	tableID := databaseItem.Tables[0].ID
	if databaseItem.Tables[0].Name != "Projects" {
		t.Fatalf("agent did not receive table name: %s", created.Output)
	}

	listWithSchema := callDatabaseTool(t, handlers, "list_databases", `{}`)
	if listWithSchema.IsError || !strings.Contains(listWithSchema.Output, `"name": "Projects"`) ||
		!strings.Contains(listWithSchema.Output, `"id": "`+tableID+`"`) {
		t.Fatalf("list did not expose table names and IDs: %s", listWithSchema.Output)
	}

	renamedTable := callDatabaseTool(t, handlers, "alter_database", `{
		"action":"update_table",
		"table_id":"`+tableID+`",
		"name":"Roadmap"
	}`)
	if renamedTable.IsError || !strings.Contains(renamedTable.Output, `"name": "Roadmap"`) {
		t.Fatalf("table rename failed: %s", renamedTable.Output)
	}

	addedColumn := callDatabaseTool(t, handlers, "alter_database", `{
		"action":"add_column",
		"table_id":"`+tableID+`",
		"name":"Owner",
		"column_type":"text"
	}`)
	if addedColumn.IsError {
		t.Fatalf("column create failed: %s", addedColumn.Output)
	}
	var column struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(addedColumn.Output), &column); err != nil || column.ID == "" {
		t.Fatalf("decode created column: %v (%s)", err, addedColumn.Output)
	}
	updatedColumn := callDatabaseTool(t, handlers, "alter_database", `{
		"action":"update_column",
		"column_id":"`+column.ID+`",
		"name":"Lead"
	}`)
	if updatedColumn.IsError || !strings.Contains(updatedColumn.Output, `"name": "Lead"`) {
		t.Fatalf("column update failed: %s", updatedColumn.Output)
	}
	deletedColumn := callDatabaseTool(t, handlers, "alter_database", `{
		"action":"delete_column",
		"column_id":"`+column.ID+`"
	}`)
	if deletedColumn.IsError {
		t.Fatalf("column delete failed: %s", deletedColumn.Output)
	}

	added := callDatabaseTool(t, handlers, "database_rows", `{
		"action":"create",
		"table_id":"`+tableID+`",
		"values":{"Name":"OpenPaw","Status":"Active","Budget":1250}
	}`)
	if added.IsError {
		t.Fatalf("row create failed: %s", added.Output)
	}
	var addedRow struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(added.Output), &addedRow); err != nil || addedRow.ID == "" {
		t.Fatalf("decode created row: %v (%s)", err, added.Output)
	}

	query := callDatabaseTool(t, handlers, "query_database", `{
		"table_id":"`+tableID+`",
		"search":"openpaw"
	}`)
	if query.IsError || !strings.Contains(query.Output, `"Name": "OpenPaw"`) || !strings.Contains(query.Output, `"Status": "Active"`) {
		t.Fatalf("query result = %s", query.Output)
	}

	updated := callDatabaseTool(t, handlers, "database_rows", `{
		"action":"update",
		"table_id":"`+tableID+`",
		"row_id":"`+addedRow.ID+`",
		"values":{"Status":"Done","Budget":1500}
	}`)
	if updated.IsError {
		t.Fatalf("row update failed: %s", updated.Output)
	}
	query = callDatabaseTool(t, handlers, "query_database", `{"table_id":"`+tableID+`","search":"Done"}`)
	if query.IsError || !strings.Contains(query.Output, `"Budget": 1500`) {
		t.Fatalf("updated query result = %s", query.Output)
	}

	deletedRow := callDatabaseTool(t, handlers, "database_rows", `{
		"action":"delete",
		"row_id":"`+addedRow.ID+`"
	}`)
	if deletedRow.IsError {
		t.Fatalf("row delete failed: %s", deletedRow.Output)
	}
	query = callDatabaseTool(t, handlers, "query_database", `{"table_id":"`+tableID+`"}`)
	if query.IsError || !strings.Contains(query.Output, `"total": 0`) {
		t.Fatalf("query after row delete = %s", query.Output)
	}

	renamed := callDatabaseTool(t, handlers, "alter_database", `{
		"action":"update_database",
		"database_id":"`+databaseItem.ID+`",
		"name":"Delivery Projects"
	}`)
	if renamed.IsError || !strings.Contains(renamed.Output, "Delivery Projects") {
		t.Fatalf("database update failed: %s", renamed.Output)
	}
	deletedDatabase := callDatabaseTool(t, handlers, "alter_database", `{
		"action":"delete_database",
		"database_id":"`+databaseItem.ID+`"
	}`)
	if deletedDatabase.IsError {
		t.Fatalf("database delete failed: %s", deletedDatabase.Output)
	}
	list := callDatabaseTool(t, handlers, "list_databases", `{}`)
	if strings.Contains(list.Output, "Delivery Projects") {
		t.Fatalf("deleted database remained: %s", list.Output)
	}

	if broadcasts < 6 {
		t.Fatalf("broadcasts = %d, want database and row CRUD updates", broadcasts)
	}

	var audits int
	_ = db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE user_id = 'agent:reporter' AND category = 'database'").Scan(&audits)
	if audits < 6 {
		t.Fatalf("database audits = %d, want CRUD audit entries", audits)
	}
}

func TestDatabaseToolsStayInRunWorkspace(t *testing.T) {
	db, workspaceID := newDatabaseToolsTestDB(t)
	otherWorkspace := uuid.New().String()
	_, _ = db.Exec("INSERT INTO workspaces (id, name) VALUES (?, 'Other')", otherWorkspace)

	first := MakeDatabaseToolHandlers(db, workspaceID, "one", nil)
	second := MakeDatabaseToolHandlers(db, otherWorkspace, "two", nil)
	result := callDatabaseTool(t, first, "create_database", `{"name":"Private Projects"}`)
	if result.IsError {
		t.Fatal(result.Output)
	}
	list := callDatabaseTool(t, second, "list_databases", `{}`)
	if strings.Contains(list.Output, "Private Projects") {
		t.Fatalf("cross-workspace database leak: %s", list.Output)
	}
}

func TestDatabaseToolDefinitionsMatchHandlers(t *testing.T) {
	db, workspaceID := newDatabaseToolsTestDB(t)
	handlers := MakeDatabaseToolHandlers(db, workspaceID, "test", nil)
	defs := BuildDatabaseToolDefs()
	if len(defs) != 5 {
		t.Fatalf("got %d definitions, want 5", len(defs))
	}
	for _, def := range defs {
		if handlers[def.Function.Name] == nil {
			t.Errorf("definition %q has no handler", def.Function.Name)
		}
	}
}
