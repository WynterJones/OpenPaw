package agents

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
)

// BuildTodoToolDefs returns tool definitions for todo list management.
func BuildTodoToolDefs() []llm.ToolDef {
	return []llm.ToolDef{
		buildTodoListAllDef(),
		buildTodoListItemsDef(),
		buildTodoAddItemDef(),
		buildTodoUpdateItemDef(),
		buildTodoCheckItemDef(),
		buildTodoUncheckItemDef(),
		buildTodoCreateListDef(),
	}
}

// MakeTodoToolHandlers returns handler closures for todo tools, capturing the agentSlug for attribution.
// The optional broadcast function is called after mutations so the frontend can live-update.
func MakeTodoToolHandlers(db *database.DB, agentSlug string, broadcast func(string, interface{})) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"todo_list_all":     handleTodoListAll(db),
		"todo_list_items":   handleTodoListItems(db),
		"todo_add_item":     handleTodoAddItem(db, agentSlug, broadcast),
		"todo_update_item":  handleTodoUpdateItem(db, agentSlug, broadcast),
		"todo_check_item":   handleTodoCheckItem(db, agentSlug, broadcast),
		"todo_uncheck_item": handleTodoUncheckItem(db, agentSlug, broadcast),
		"todo_create_list":  handleTodoCreateList(db, agentSlug, broadcast),
	}
}

// buildTodoPromptSection builds a prompt section showing todo list summary.
func buildTodoPromptSection(db *database.DB) string {
	rows, err := db.Query(`
		SELECT tl.name,
			(SELECT COUNT(*) FROM todo_items WHERE list_id = tl.id) as total,
			(SELECT COUNT(*) FROM todo_items WHERE list_id = tl.id AND completed = 1) as done
		FROM todo_lists tl ORDER BY tl.sort_order ASC`)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var name string
		var total, done int
		if rows.Scan(&name, &total, &done) != nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s (%d items, %d done)", name, total, done))
	}
	if len(lines) == 0 {
		return ""
	}
	return "## TODO LISTS\nThe user has todo lists. Use todo_* tools to view and manage them.\n" + strings.Join(lines, "\n")
}

// --- Tool Definitions ---

func buildTodoListAllDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "todo_list_all",
			Description: "List all todo lists with item counts. Use this to see what todo lists the user has.",
			Parameters:  params,
		},
	}
}

func buildTodoListItemsDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"list_id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the todo list",
			},
			"include_completed": map[string]interface{}{
				"type":        "boolean",
				"description": "Include completed items (default false)",
				"default":     false,
			},
		},
		"required": []string{"list_id"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "todo_list_items",
			Description: "List items in a todo list. By default only shows incomplete items.",
			Parameters:  params,
		},
	}
}

func buildTodoAddItemDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"list_id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the todo list to add the item to",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Title of the todo item",
			},
			"notes": map[string]interface{}{
				"type":        "string",
				"description": "Optional notes for the item",
			},
			"due_date": map[string]interface{}{
				"type":        "string",
				"description": "Optional due date (ISO 8601 format, e.g. 2024-12-31)",
			},
		},
		"required": []string{"list_id", "title"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "todo_add_item",
			Description: "Add a new item to a todo list.",
			Parameters:  params,
		},
	}
}

func buildTodoUpdateItemDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"item_id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the todo item to update",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "New title",
			},
			"notes": map[string]interface{}{
				"type":        "string",
				"description": "New notes",
			},
			"due_date": map[string]interface{}{
				"type":        "string",
				"description": "New due date (ISO 8601 format, empty string to clear)",
			},
			"note": map[string]interface{}{
				"type":        "string",
				"description": "A note about why this update was made",
			},
		},
		"required": []string{"item_id"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "todo_update_item",
			Description: "Update an existing todo item's title, notes, or due date.",
			Parameters:  params,
		},
	}
}

func buildTodoCheckItemDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"item_id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the todo item to mark as completed",
			},
			"note": map[string]interface{}{
				"type":        "string",
				"description": "Optional note about completion",
			},
		},
		"required": []string{"item_id"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "todo_check_item",
			Description: "Mark a todo item as completed.",
			Parameters:  params,
		},
	}
}

func buildTodoUncheckItemDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"item_id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the todo item to mark as incomplete",
			},
			"note": map[string]interface{}{
				"type":        "string",
				"description": "Optional note about why it was unchecked",
			},
		},
		"required": []string{"item_id"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "todo_uncheck_item",
			Description: "Mark a todo item as incomplete (uncheck it).",
			Parameters:  params,
		},
	}
}

func buildTodoCreateListDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the new todo list",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Optional description of the list",
			},
		},
		"required": []string{"name"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "todo_create_list",
			Description: "Create a new todo list.",
			Parameters:  params,
		},
	}
}

// --- Tool Handlers ---

func handleTodoListAll(db *database.DB) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		rows, err := db.Query(`
			SELECT tl.id, tl.name, tl.description,
				(SELECT COUNT(*) FROM todo_items WHERE list_id = tl.id) as total,
				(SELECT COUNT(*) FROM todo_items WHERE list_id = tl.id AND completed = 1) as done
			FROM todo_lists tl ORDER BY tl.sort_order ASC, tl.created_at ASC`)
		if err != nil {
			return llm.ToolResult{Output: "Failed to list todo lists: " + err.Error(), IsError: true}
		}
		defer rows.Close()

		var lists []map[string]interface{}
		for rows.Next() {
			var id, name, description string
			var total, done int
			if rows.Scan(&id, &name, &description, &total, &done) != nil {
				continue
			}
			lists = append(lists, map[string]interface{}{
				"id":              id,
				"name":            name,
				"description":     description,
				"total_items":     total,
				"completed_items": done,
			})
		}

		if lists == nil {
			lists = []map[string]interface{}{}
		}

		result, _ := json.Marshal(map[string]interface{}{
			"lists": lists,
			"count": len(lists),
		})
		return llm.ToolResult{Output: string(result)}
	}
}

func handleTodoListItems(db *database.DB) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			ListID           string `json:"list_id"`
			IncludeCompleted bool   `json:"include_completed"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.ListID == "" {
			return llm.ToolResult{Output: "list_id is required", IsError: true}
		}

		query := `SELECT id, title, notes, completed, due_date, last_actor_agent_slug, last_actor_note, created_at, completed_at
			FROM todo_items WHERE list_id = ?`
		args := []interface{}{params.ListID}

		if !params.IncludeCompleted {
			query += " AND completed = 0"
		}
		query += " ORDER BY completed ASC, sort_order ASC, created_at ASC"

		rows, err := db.Query(query, args...)
		if err != nil {
			return llm.ToolResult{Output: "Failed to list items: " + err.Error(), IsError: true}
		}
		defer rows.Close()

		var items []map[string]interface{}
		for rows.Next() {
			var id, title, notes, lastActorNote string
			var completed int
			var dueDate, agentSlug sql.NullString
			var createdAt time.Time
			var completedAt sql.NullTime

			if rows.Scan(&id, &title, &notes, &completed, &dueDate, &agentSlug, &lastActorNote, &createdAt, &completedAt) != nil {
				continue
			}

			item := map[string]interface{}{
				"id":        id,
				"title":     title,
				"notes":     notes,
				"completed": completed == 1,
			}
			if dueDate.Valid {
				item["due_date"] = dueDate.String
			}
			if agentSlug.Valid {
				item["last_actor"] = agentSlug.String
			}
			if lastActorNote != "" {
				item["last_actor_note"] = lastActorNote
			}
			if completedAt.Valid {
				item["completed_at"] = completedAt.Time.Format(time.RFC3339)
			}
			items = append(items, item)
		}

		if items == nil {
			items = []map[string]interface{}{}
		}

		result, _ := json.Marshal(map[string]interface{}{
			"list_id": params.ListID,
			"items":   items,
			"count":   len(items),
		})
		return llm.ToolResult{Output: string(result)}
	}
}

func handleTodoAddItem(db *database.DB, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			ListID  string `json:"list_id"`
			Title   string `json:"title"`
			Notes   string `json:"notes"`
			DueDate string `json:"due_date"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.ListID == "" {
			return llm.ToolResult{Output: "list_id is required", IsError: true}
		}
		if params.Title == "" {
			return llm.ToolResult{Output: "title is required", IsError: true}
		}

		// Verify list exists
		var exists int
		if db.QueryRow("SELECT 1 FROM todo_lists WHERE id = ?", params.ListID).Scan(&exists) != nil {
			return llm.ToolResult{Output: "Todo list not found: " + params.ListID, IsError: true}
		}

		id := uuid.New().String()
		now := time.Now().UTC()

		var maxOrder int
		db.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM todo_items WHERE list_id = ?", params.ListID).Scan(&maxOrder)

		var dueDate sql.NullString
		if params.DueDate != "" {
			dueDate = sql.NullString{String: params.DueDate, Valid: true}
		}

		_, err := db.Exec(
			`INSERT INTO todo_items (id, list_id, title, notes, completed, sort_order, due_date, last_actor_agent_slug, last_actor_note, created_at, updated_at)
			 VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?)`,
			id, params.ListID, params.Title, params.Notes, maxOrder+1, dueDate,
			sql.NullString{String: agentSlug, Valid: agentSlug != ""},
			fmt.Sprintf("Added by agent %s", agentSlug),
			now, now,
		)
		if err != nil {
			return llm.ToolResult{Output: "Failed to add item: " + err.Error(), IsError: true}
		}

		db.LogAudit("system", "todo_item_created", "todo", "todo_list", params.ListID, "agent="+agentSlug+" title="+params.Title)

		if broadcast != nil {
			broadcast("todo_updated", map[string]interface{}{"type": "item_added", "list_id": params.ListID})
		}

		result, _ := json.Marshal(map[string]interface{}{
			"id":      id,
			"title":   params.Title,
			"list_id": params.ListID,
			"added":   true,
		})
		return llm.ToolResult{Output: string(result)}
	}
}

func handleTodoUpdateItem(db *database.DB, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			ItemID  string  `json:"item_id"`
			Title   *string `json:"title"`
			Notes   *string `json:"notes"`
			DueDate *string `json:"due_date"`
			Note    *string `json:"note"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.ItemID == "" {
			return llm.ToolResult{Output: "item_id is required", IsError: true}
		}

		now := time.Now().UTC()
		updated := false

		if params.Title != nil {
			db.Exec("UPDATE todo_items SET title = ?, updated_at = ? WHERE id = ?", *params.Title, now, params.ItemID)
			updated = true
		}
		if params.Notes != nil {
			db.Exec("UPDATE todo_items SET notes = ?, updated_at = ? WHERE id = ?", *params.Notes, now, params.ItemID)
			updated = true
		}
		if params.DueDate != nil {
			if *params.DueDate == "" {
				db.Exec("UPDATE todo_items SET due_date = NULL, updated_at = ? WHERE id = ?", now, params.ItemID)
			} else {
				db.Exec("UPDATE todo_items SET due_date = ?, updated_at = ? WHERE id = ?", *params.DueDate, now, params.ItemID)
			}
			updated = true
		}

		// Always update agent attribution
		actorNote := fmt.Sprintf("Updated by agent %s", agentSlug)
		if params.Note != nil && *params.Note != "" {
			actorNote = *params.Note
		}
		db.Exec("UPDATE todo_items SET last_actor_agent_slug = ?, last_actor_note = ?, updated_at = ? WHERE id = ?",
			agentSlug, actorNote, now, params.ItemID)

		if !updated {
			return llm.ToolResult{Output: "No fields to update", IsError: true}
		}

		db.LogAudit("system", "todo_item_updated", "todo", "todo_item", params.ItemID, "agent="+agentSlug)

		if broadcast != nil {
			broadcast("todo_updated", map[string]interface{}{"type": "item_updated", "item_id": params.ItemID})
		}

		result, _ := json.Marshal(map[string]interface{}{
			"id":      params.ItemID,
			"updated": true,
		})
		return llm.ToolResult{Output: string(result)}
	}
}

func handleTodoCheckItem(db *database.DB, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			ItemID string `json:"item_id"`
			Note   string `json:"note"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.ItemID == "" {
			return llm.ToolResult{Output: "item_id is required", IsError: true}
		}

		now := time.Now().UTC()
		actorNote := fmt.Sprintf("Completed by agent %s", agentSlug)
		if params.Note != "" {
			actorNote = params.Note
		}

		result, err := db.Exec(
			"UPDATE todo_items SET completed = 1, completed_at = ?, last_actor_agent_slug = ?, last_actor_note = ?, updated_at = ? WHERE id = ?",
			now, agentSlug, actorNote, now, params.ItemID,
		)
		if err != nil {
			return llm.ToolResult{Output: "Failed to check item: " + err.Error(), IsError: true}
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return llm.ToolResult{Output: "Todo item not found: " + params.ItemID, IsError: true}
		}

		db.LogAudit("system", "todo_item_completed", "todo", "todo_item", params.ItemID, "agent="+agentSlug)

		if broadcast != nil {
			broadcast("todo_updated", map[string]interface{}{"type": "item_checked", "item_id": params.ItemID})
		}

		resp, _ := json.Marshal(map[string]interface{}{
			"id":        params.ItemID,
			"completed": true,
		})
		return llm.ToolResult{Output: string(resp)}
	}
}

func handleTodoUncheckItem(db *database.DB, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			ItemID string `json:"item_id"`
			Note   string `json:"note"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.ItemID == "" {
			return llm.ToolResult{Output: "item_id is required", IsError: true}
		}

		now := time.Now().UTC()
		actorNote := fmt.Sprintf("Unchecked by agent %s", agentSlug)
		if params.Note != "" {
			actorNote = params.Note
		}

		result, err := db.Exec(
			"UPDATE todo_items SET completed = 0, completed_at = NULL, last_actor_agent_slug = ?, last_actor_note = ?, updated_at = ? WHERE id = ?",
			agentSlug, actorNote, now, params.ItemID,
		)
		if err != nil {
			return llm.ToolResult{Output: "Failed to uncheck item: " + err.Error(), IsError: true}
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return llm.ToolResult{Output: "Todo item not found: " + params.ItemID, IsError: true}
		}

		db.LogAudit("system", "todo_item_uncompleted", "todo", "todo_item", params.ItemID, "agent="+agentSlug)

		if broadcast != nil {
			broadcast("todo_updated", map[string]interface{}{"type": "item_unchecked", "item_id": params.ItemID})
		}

		resp, _ := json.Marshal(map[string]interface{}{
			"id":        params.ItemID,
			"completed": false,
		})
		return llm.ToolResult{Output: string(resp)}
	}
}

func handleTodoCreateList(db *database.DB, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.Name == "" {
			return llm.ToolResult{Output: "name is required", IsError: true}
		}

		id := uuid.New().String()
		now := time.Now().UTC()

		var maxOrder int
		db.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM todo_lists").Scan(&maxOrder)

		_, err := db.Exec(
			"INSERT INTO todo_lists (id, name, description, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			id, params.Name, params.Description, maxOrder+1, now, now,
		)
		if err != nil {
			return llm.ToolResult{Output: "Failed to create list: " + err.Error(), IsError: true}
		}

		db.LogAudit("system", "todo_list_created", "todo", "todo_list", id, "agent="+agentSlug+" name="+params.Name)

		if broadcast != nil {
			broadcast("todo_updated", map[string]interface{}{"type": "list_created", "list_id": id})
		}

		result, _ := json.Marshal(map[string]interface{}{
			"id":      id,
			"name":    params.Name,
			"created": true,
		})
		return llm.ToolResult{Output: string(result)}
	}
}
