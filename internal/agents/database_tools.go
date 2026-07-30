package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/userdb"
)

// BuildDatabaseToolDefs returns the workspace database tools. The surface is
// deliberately compact: list/query, one creation tool, one schema mutation
// tool, and one row mutation tool cover full CRUD without flooding every turn
// with a separate function for each small operation.
func BuildDatabaseToolDefs() []llm.ToolDef {
	return []llm.ToolDef{
		databaseToolDef("list_databases",
			"List the databases in this workspace with every table name, table ID, column name/type/ID, and row count. Use this before querying or changing stored structured data.",
			map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}),
		databaseToolDef("query_database",
			"Read and search rows in one database table. Returns both a table-shaped columns/rows result and records keyed by column name. Use this to answer questions from saved structured data.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"table_id": map[string]interface{}{"type": "string", "description": "Table ID from list_databases"},
					"search":   map[string]interface{}{"type": "string", "description": "Optional case-insensitive text search across every column"},
					"limit":    map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 500, "default": 100},
					"offset":   map[string]interface{}{"type": "integer", "minimum": 0, "default": 0},
				},
				"required": []string{"table_id"},
			}),
		databaseToolDef("create_database",
			"Create a workspace database, optionally with its tables and typed columns in one call. Column types: text, long_text, number, checkbox, date, url, email, select.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string"},
					"description": map[string]interface{}{"type": "string"},
					"tables": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"name": map[string]interface{}{"type": "string"},
								"columns": map[string]interface{}{
									"type": "array",
									"items": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"name":    map[string]interface{}{"type": "string"},
											"type":    map[string]interface{}{"type": "string", "enum": []string{"text", "long_text", "number", "checkbox", "date", "url", "email", "select"}},
											"options": map[string]interface{}{"type": "object", "description": "For select columns, use {\"choices\":[\"One\",\"Two\"]}"},
										},
										"required": []string{"name"},
									},
								},
							},
							"required": []string{"name"},
						},
					},
				},
				"required": []string{"name"},
			}),
		databaseToolDef("alter_database",
			"Change database structure or metadata. Supports update/delete database, create/update/delete table, and add/update/delete column. Use update_table with table_id and name to rename a table.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type": "string",
						"enum": []string{"update_database", "delete_database", "create_table", "update_table", "delete_table", "add_column", "update_column", "delete_column"},
					},
					"database_id": map[string]interface{}{"type": "string"},
					"table_id":    map[string]interface{}{"type": "string"},
					"column_id":   map[string]interface{}{"type": "string"},
					"name":        map[string]interface{}{"type": "string"},
					"description": map[string]interface{}{"type": "string"},
					"column_type": map[string]interface{}{"type": "string", "enum": []string{"text", "long_text", "number", "checkbox", "date", "url", "email", "select"}},
					"options":     map[string]interface{}{"type": "object"},
				},
				"required": []string{"action"},
			}),
		databaseToolDef("database_rows",
			"Create, update, or delete rows. Values are keyed by human-readable column name, not column ID. For updates include table_id so column names can be resolved.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action":   map[string]interface{}{"type": "string", "enum": []string{"create", "update", "delete"}},
					"table_id": map[string]interface{}{"type": "string"},
					"row_id":   map[string]interface{}{"type": "string"},
					"values":   map[string]interface{}{"type": "object", "description": "Column-name to value mapping, e.g. {\"Name\":\"OpenPaw\",\"Status\":\"Active\"}"},
				},
				"required": []string{"action"},
			}),
	}
}

func databaseToolDef(name, description string, params map[string]interface{}) llm.ToolDef {
	raw, _ := json.Marshal(params)
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        name,
			Description: description,
			Parameters:  raw,
		},
	}
}

func MakeDatabaseToolHandlers(db *database.DB, workspaceID, agentSlug string, broadcast func(string, interface{})) map[string]llm.ToolHandler {
	store := userdb.NewStore(db)
	actor := "system"
	if agentSlug != "" {
		actor = "agent:" + agentSlug
	}
	changed := func(databaseID, tableID string) {
		if broadcast != nil {
			broadcast("database_updated", map[string]string{"database_id": databaseID, "table_id": tableID})
		}
	}
	return map[string]llm.ToolHandler{
		"list_databases":  handleListDatabases(store, workspaceID),
		"query_database":  handleQueryDatabase(store, workspaceID),
		"create_database": handleCreateDatabase(db, store, workspaceID, actor, changed),
		"alter_database":  handleAlterDatabase(db, store, workspaceID, actor, changed),
		"database_rows":   handleDatabaseRows(db, store, workspaceID, actor, changed),
	}
}

func buildDatabasesPromptSection(db *database.DB, workspaceID string) string {
	store := userdb.NewStore(db)
	items, err := store.ListDatabases(workspaceID)
	if err != nil || len(items) == 0 {
		return "## DATABASES\nThis workspace has no databases yet. You can create one with `create_database` when structured, durable data would help.\n"
	}
	var lines []string
	for _, item := range items {
		full, err := store.GetDatabase(workspaceID, item.ID)
		if err != nil {
			continue
		}
		tables := make([]string, 0, len(full.Tables))
		for _, table := range full.Tables {
			columns := make([]string, 0, len(table.Columns))
			for _, column := range table.Columns {
				columns = append(columns, column.Name+" ["+column.Type+"]")
			}
			tables = append(tables, fmt.Sprintf("%s (table ID: `%s`; %d rows; columns: %s)",
				table.Name, table.ID, table.RowCount, strings.Join(columns, ", ")))
		}
		lines = append(lines, fmt.Sprintf("- %s (ID: `%s`) — %s", item.Name, item.ID, strings.Join(tables, ", ")))
	}
	return "## DATABASES\nWorkspace databases hold durable structured information. Use `list_databases` for schema IDs, `query_database` to read/search, and database mutation tools when asked to save or maintain records. Dashboard builders can connect widgets directly to these tables.\n" +
		strings.Join(lines, "\n") + "\n"
}

func handleListDatabases(store *userdb.Store, workspaceID string) llm.ToolHandler {
	return func(context.Context, string, json.RawMessage) llm.ToolResult {
		items, err := store.ListDatabases(workspaceID)
		if err != nil {
			return databaseToolError(err)
		}
		for i := range items {
			full, err := store.GetDatabase(workspaceID, items[i].ID)
			if err == nil {
				items[i].Tables = full.Tables
			}
		}
		return databaseToolJSON(items)
	}
}

func handleQueryDatabase(store *userdb.Store, workspaceID string) llm.ToolHandler {
	return func(_ context.Context, _ string, input json.RawMessage) llm.ToolResult {
		var params struct {
			TableID string `json:"table_id"`
			Search  string `json:"search"`
			Limit   int    `json:"limit"`
			Offset  int    `json:"offset"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return databaseToolError(err)
		}
		if params.TableID == "" {
			return databaseToolError(fmt.Errorf("table_id is required"))
		}
		rows, err := store.NamedRows(workspaceID, params.TableID, params.Search, params.Limit, params.Offset)
		if err != nil {
			return databaseToolError(err)
		}
		return databaseToolJSON(rows)
	}
}

type databaseSchemaInput struct {
	Name    string                 `json:"name"`
	Type    string                 `json:"type"`
	Options map[string]interface{} `json:"options"`
}

type databaseTableInput struct {
	Name    string                `json:"name"`
	Columns []databaseSchemaInput `json:"columns"`
}

func handleCreateDatabase(db *database.DB, store *userdb.Store, workspaceID, actor string, changed func(string, string)) llm.ToolHandler {
	return func(_ context.Context, _ string, input json.RawMessage) llm.ToolResult {
		var params struct {
			Name        string               `json:"name"`
			Description string               `json:"description"`
			Tables      []databaseTableInput `json:"tables"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return databaseToolError(err)
		}
		item, err := store.CreateDatabase(workspaceID, params.Name, params.Description)
		if err != nil {
			return databaseToolError(err)
		}
		if len(params.Tables) > 0 {
			if err := configureStarterTable(store, workspaceID, item.Tables[0], params.Tables[0]); err != nil {
				_ = store.DeleteDatabase(workspaceID, item.ID)
				return databaseToolError(err)
			}
			for _, tableInput := range params.Tables[1:] {
				table, err := store.CreateTable(workspaceID, item.ID, tableInput.Name)
				if err != nil {
					_ = store.DeleteDatabase(workspaceID, item.ID)
					return databaseToolError(err)
				}
				if err := configureStarterTable(store, workspaceID, table, tableInput); err != nil {
					_ = store.DeleteDatabase(workspaceID, item.ID)
					return databaseToolError(err)
				}
			}
		}
		item, _ = store.GetDatabase(workspaceID, item.ID)
		db.LogAudit(actor, "database_created", "database", "database", item.ID, item.Name)
		changed(item.ID, "")
		return databaseToolJSON(item)
	}
}

func configureStarterTable(store *userdb.Store, workspaceID string, table userdb.Table, input databaseTableInput) error {
	if input.Name != "" && input.Name != table.Name {
		name := input.Name
		var err error
		table, err = store.UpdateTable(workspaceID, table.ID, &name)
		if err != nil {
			return err
		}
	}
	if len(input.Columns) == 0 {
		return nil
	}
	first := input.Columns[0]
	columnType := first.Type
	if columnType == "" {
		columnType = "text"
	}
	name := first.Name
	if _, err := store.UpdateColumn(workspaceID, table.Columns[0].ID, &name, &columnType, first.Options); err != nil {
		return err
	}
	for _, column := range input.Columns[1:] {
		columnType := column.Type
		if columnType == "" {
			columnType = "text"
		}
		if _, err := store.CreateColumn(workspaceID, table.ID, column.Name, columnType, column.Options); err != nil {
			return err
		}
	}
	return nil
}

func handleAlterDatabase(db *database.DB, store *userdb.Store, workspaceID, actor string, changed func(string, string)) llm.ToolHandler {
	return func(_ context.Context, _ string, input json.RawMessage) llm.ToolResult {
		var params struct {
			Action      string                 `json:"action"`
			DatabaseID  string                 `json:"database_id"`
			TableID     string                 `json:"table_id"`
			ColumnID    string                 `json:"column_id"`
			Name        *string                `json:"name"`
			Description *string                `json:"description"`
			ColumnType  *string                `json:"column_type"`
			Options     map[string]interface{} `json:"options"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return databaseToolError(err)
		}

		var result interface{}
		var err error
		databaseID, tableID := params.DatabaseID, params.TableID
		switch params.Action {
		case "update_database":
			if params.DatabaseID == "" {
				err = fmt.Errorf("database_id is required")
				break
			}
			result, err = store.UpdateDatabase(workspaceID, params.DatabaseID, params.Name, params.Description)
		case "delete_database":
			if params.DatabaseID == "" {
				err = fmt.Errorf("database_id is required")
				break
			}
			err = store.DeleteDatabase(workspaceID, params.DatabaseID)
			result = map[string]string{"deleted_database_id": params.DatabaseID}
		case "create_table":
			if params.DatabaseID == "" || params.Name == nil {
				err = fmt.Errorf("database_id and name are required")
				break
			}
			result, err = store.CreateTable(workspaceID, params.DatabaseID, *params.Name)
		case "update_table":
			if params.TableID == "" {
				err = fmt.Errorf("table_id is required")
				break
			}
			var table userdb.Table
			table, err = store.UpdateTable(workspaceID, params.TableID, params.Name)
			result = table
			databaseID = table.DatabaseID
		case "delete_table":
			if params.TableID == "" {
				err = fmt.Errorf("table_id is required")
				break
			}
			table, getErr := store.GetTable(workspaceID, params.TableID)
			if getErr != nil {
				err = getErr
				break
			}
			databaseID = table.DatabaseID
			err = store.DeleteTable(workspaceID, params.TableID)
			result = map[string]string{"deleted_table_id": params.TableID}
		case "add_column":
			if params.TableID == "" || params.Name == nil {
				err = fmt.Errorf("table_id and name are required")
				break
			}
			columnType := "text"
			if params.ColumnType != nil {
				columnType = *params.ColumnType
			}
			result, err = store.CreateColumn(workspaceID, params.TableID, *params.Name, columnType, params.Options)
		case "update_column":
			if params.ColumnID == "" {
				err = fmt.Errorf("column_id is required")
				break
			}
			result, err = store.UpdateColumn(workspaceID, params.ColumnID, params.Name, params.ColumnType, params.Options)
		case "delete_column":
			if params.ColumnID == "" {
				err = fmt.Errorf("column_id is required")
				break
			}
			err = store.DeleteColumn(workspaceID, params.ColumnID)
			result = map[string]string{"deleted_column_id": params.ColumnID}
		default:
			err = fmt.Errorf("unsupported action %q", params.Action)
		}
		if err != nil {
			return databaseToolError(err)
		}
		db.LogAudit(actor, "database_schema_changed", "database", "database", databaseID, params.Action)
		changed(databaseID, tableID)
		return databaseToolJSON(result)
	}
}

func handleDatabaseRows(db *database.DB, store *userdb.Store, workspaceID, actor string, changed func(string, string)) llm.ToolHandler {
	return func(_ context.Context, _ string, input json.RawMessage) llm.ToolResult {
		var params struct {
			Action  string                 `json:"action"`
			TableID string                 `json:"table_id"`
			RowID   string                 `json:"row_id"`
			Values  map[string]interface{} `json:"values"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return databaseToolError(err)
		}

		var result interface{}
		var err error
		switch params.Action {
		case "create":
			if params.TableID == "" {
				err = fmt.Errorf("table_id is required")
				break
			}
			var values map[string]interface{}
			values, err = store.ColumnIDsForNames(workspaceID, params.TableID, params.Values)
			if err == nil {
				result, err = store.CreateRow(workspaceID, params.TableID, values)
			}
		case "update":
			if params.TableID == "" || params.RowID == "" {
				err = fmt.Errorf("table_id and row_id are required")
				break
			}
			var values map[string]interface{}
			values, err = store.ColumnIDsForNames(workspaceID, params.TableID, params.Values)
			if err == nil {
				result, err = store.UpdateRow(workspaceID, params.RowID, values)
			}
		case "delete":
			if params.RowID == "" {
				err = fmt.Errorf("row_id is required")
				break
			}
			err = store.DeleteRow(workspaceID, params.RowID)
			result = map[string]string{"deleted_row_id": params.RowID}
		default:
			err = fmt.Errorf("unsupported action %q", params.Action)
		}
		if err != nil {
			return databaseToolError(err)
		}

		databaseID := ""
		if params.TableID != "" {
			if table, getErr := store.GetTable(workspaceID, params.TableID); getErr == nil {
				databaseID = table.DatabaseID
			}
		}
		db.LogAudit(actor, "database_rows_changed", "database", "database_row", params.RowID, params.Action)
		changed(databaseID, params.TableID)
		return databaseToolJSON(result)
	}
}

func databaseToolJSON(value interface{}) llm.ToolResult {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return databaseToolError(err)
	}
	return llm.ToolResult{Output: string(raw)}
}

func databaseToolError(err error) llm.ToolResult {
	return llm.ToolResult{Output: "Database operation failed: " + err.Error(), IsError: true}
}
