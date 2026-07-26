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
		buildTodoNextItemDef(),
		buildTodoStartItemDef(),
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
		"todo_next_item":    handleTodoNextItem(db, agentSlug, broadcast),
		"todo_start_item":   handleTodoStartItem(db, agentSlug, broadcast),
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
			(SELECT COUNT(*) FROM todo_items WHERE list_id = tl.id AND completed = 1) as done,
			(SELECT COUNT(*) FROM todo_items WHERE list_id = tl.id AND completed = 0 AND in_progress = 1) as doing
		FROM todo_lists tl ORDER BY tl.sort_order ASC`)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var name string
		var total, done, doing int
		if rows.Scan(&name, &total, &done, &doing) != nil {
			continue
		}
		line := fmt.Sprintf("- %s (%d items, %d done", name, total, done)
		if doing > 0 {
			line += fmt.Sprintf(", %d in progress", doing)
		}
		lines = append(lines, line+")")
	}
	if len(lines) == 0 {
		return ""
	}
	return "## TODO LISTS\nThe user has todo lists. Use todo_* tools to view and manage them.\n" +
		strings.Join(lines, "\n") + "\n\n" + todoWorkflowGuidance()
}

// todoWorkflowGuidance is the three-state convention, spelled out because the
// failure it prevents is invisible: an item someone is already working on looks
// exactly like an untouched one until it is ticked off, so "do the next task"
// picks it up a second time.
func todoWorkflowGuidance() string {
	return `### Picking up work

An item is one of three things: **not started**, **in progress**, or **done**.

- Asked for "the next task"? Call ` + "`todo_next_item`" + `. It returns the first item that is neither done nor already in progress, and claims it for you. Never pick an in-progress item because it looks like the next one — someone else is on it.
- Already know which item you want? ` + "`todo_start_item`" + ` claims it. It refuses if the item is already claimed, and tells you by whom.
- Finished? ` + "`todo_check_item`" + `, which also clears the claim.
- Stopped without finishing? ` + "`todo_uncheck_item`" + ` releases it so someone else can take it. Do this rather than leaving a claim behind — a stale claim makes work invisible to every later run.`
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

func buildTodoNextItemDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"list_id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the todo list. Omit to take the next item from any list.",
			},
			"claim": map[string]interface{}{
				"type": "boolean",
				"description": "Mark the item as in progress so nobody else picks it up (default true). " +
					"Set false only to look ahead without taking the work.",
				"default": true,
			},
		},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name: "todo_next_item",
			Description: "Get the next todo item to work on, and claim it. Skips anything already done or " +
				"already in progress, so two runs never pick up the same task. USE THIS whenever the user " +
				"says \"work on the next task\", \"pick up the next item\" or similar, rather than reading " +
				"the list and choosing yourself.",
			Parameters: params,
		},
	}
}

func buildTodoStartItemDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"item_id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the todo item you are starting work on",
			},
			"note": map[string]interface{}{
				"type":        "string",
				"description": "Optional note about what you are doing",
			},
		},
		"required": []string{"item_id"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name: "todo_start_item",
			Description: "Mark a todo item as in progress — you are working on it now. Do this before you " +
				"start, not after: an unclaimed item looks available to every other run. Refuses if someone " +
				"else already claimed it.",
			Parameters: params,
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

		query := `SELECT id, title, notes, attachments, completed, in_progress, started_by_agent_slug, started_at, due_date, last_actor_agent_slug, last_actor_note, created_at, completed_at
			FROM todo_items WHERE list_id = ?`
		args := []interface{}{params.ListID}

		if !params.IncludeCompleted {
			query += " AND completed = 0"
		}
		// In-progress first: they are the ones not to touch, so they should be
		// the ones read first rather than buried.
		query += " ORDER BY completed ASC, in_progress DESC, sort_order ASC, created_at ASC"

		rows, err := db.Query(query, args...)
		if err != nil {
			return llm.ToolResult{Output: "Failed to list items: " + err.Error(), IsError: true}
		}
		defer rows.Close()

		var items []map[string]interface{}
		for rows.Next() {
			var id, title, notes, attachments, lastActorNote string
			var completed, inProgress int
			var dueDate, agentSlug, startedBy sql.NullString
			var createdAt time.Time
			var completedAt, startedAt sql.NullTime

			if rows.Scan(&id, &title, &notes, &attachments, &completed, &inProgress, &startedBy, &startedAt, &dueDate, &agentSlug, &lastActorNote, &createdAt, &completedAt) != nil {
				continue
			}

			// status is spelled out alongside the booleans because "completed:
			// false" on its own reads as "available", which is the mistake this
			// whole thing exists to stop.
			status := "not_started"
			if completed == 1 {
				status = "done"
			} else if inProgress == 1 {
				status = "in_progress"
			}

			item := map[string]interface{}{
				"id":          id,
				"title":       title,
				"notes":       notes,
				"completed":   completed == 1,
				"in_progress": inProgress == 1,
				"status":      status,
			}
			if inProgress == 1 {
				if startedBy.Valid && startedBy.String != "" {
					item["started_by"] = startedBy.String
				}
				if startedAt.Valid {
					item["started_at"] = startedAt.Time.Format(time.RFC3339)
				}
			}
			// Real on-disk paths, so "the screenshot" in a task body resolves
			// to something the agent can actually open.
			if paths := todoAttachmentPaths(attachments); len(paths) > 0 {
				item["attachments"] = paths
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

// handleTodoNextItem hands out the next unclaimed item and claims it in one
// step.
//
// One step on purpose: "read the list, then start the first one" is two calls
// with a gap between them, and the gap is exactly where a second run picks the
// same task. It is also the thing agents skip — they read the list, choose, and
// never claim anything.
func handleTodoNextItem(db *database.DB, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			ListID string `json:"list_id"`
			Claim  *bool  `json:"claim"`
		}
		json.Unmarshal(input, &params)
		claim := params.Claim == nil || *params.Claim

		query := `SELECT ti.id, ti.list_id, tl.name, ti.title, ti.notes, ti.attachments, ti.due_date
			FROM todo_items ti
			JOIN todo_lists tl ON tl.id = ti.list_id
			WHERE ti.completed = 0 AND ti.in_progress = 0`
		args := []interface{}{}
		if params.ListID != "" {
			query += " AND ti.list_id = ?"
			args = append(args, params.ListID)
		}
		query += " ORDER BY tl.sort_order ASC, ti.sort_order ASC, ti.created_at ASC LIMIT 1"

		var id, listID, listName, title, notes, attachments string
		var dueDate sql.NullString
		if err := db.QueryRow(query, args...).Scan(&id, &listID, &listName, &title, &notes, &attachments, &dueDate); err != nil {
			// Nothing available is a real answer, not a failure — but say which
			// kind, because "all done" and "all claimed" call for opposite
			// responses.
			var doing int
			countQuery := "SELECT COUNT(*) FROM todo_items WHERE completed = 0 AND in_progress = 1"
			countArgs := []interface{}{}
			if params.ListID != "" {
				countQuery += " AND list_id = ?"
				countArgs = append(countArgs, params.ListID)
			}
			db.QueryRow(countQuery, countArgs...).Scan(&doing)

			msg := "Nothing left to pick up — everything is done."
			if doing > 0 {
				msg = fmt.Sprintf("Nothing available: the %d remaining item(s) are already in progress. Do not start them.", doing)
			}
			resp, _ := json.Marshal(map[string]interface{}{"item": nil, "reason": msg})
			return llm.ToolResult{Output: string(resp)}
		}

		if claim {
			now := time.Now().UTC()
			// Guarded on in_progress = 0 so two runs racing here cannot both
			// walk away believing they own the item.
			res, err := db.Exec(
				"UPDATE todo_items SET in_progress = 1, started_at = ?, started_by_agent_slug = ?, last_actor_agent_slug = ?, last_actor_note = ?, updated_at = ? WHERE id = ? AND in_progress = 0 AND completed = 0",
				now, agentSlug, agentSlug, fmt.Sprintf("Started by agent %s", agentSlug), now, id,
			)
			if err != nil {
				return llm.ToolResult{Output: "Failed to claim item: " + err.Error(), IsError: true}
			}
			if n, _ := res.RowsAffected(); n == 0 {
				resp, _ := json.Marshal(map[string]interface{}{
					"item":   nil,
					"reason": "Someone claimed that item first. Call todo_next_item again for the one after it.",
				})
				return llm.ToolResult{Output: string(resp)}
			}
			db.LogAudit("system", "todo_item_started", "todo", "todo_item", id, "agent="+agentSlug)
			if broadcast != nil {
				broadcast("todo_updated", map[string]interface{}{"type": "item_started", "item_id": id, "list_id": listID})
			}
		}

		item := map[string]interface{}{
			"id":          id,
			"list_id":     listID,
			"list_name":   listName,
			"title":       title,
			"notes":       notes,
			"in_progress": claim,
		}
		if paths := todoAttachmentPaths(attachments); len(paths) > 0 {
			item["attachments"] = paths
		}
		if dueDate.Valid {
			item["due_date"] = dueDate.String
		}

		note := "Claimed — it is now in progress and no other run will pick it up. Call todo_check_item when it is done, or todo_uncheck_item to hand it back."
		if !claim {
			note = "Not claimed — call todo_start_item before you begin working on it."
		}
		resp, _ := json.Marshal(map[string]interface{}{"item": item, "note": note})
		return llm.ToolResult{Output: string(resp)}
	}
}

func handleTodoStartItem(db *database.DB, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
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

		var title string
		var completed, inProgress int
		var startedBy sql.NullString
		var startedAt sql.NullTime
		err := db.QueryRow(
			"SELECT title, completed, in_progress, started_by_agent_slug, started_at FROM todo_items WHERE id = ?",
			params.ItemID,
		).Scan(&title, &completed, &inProgress, &startedBy, &startedAt)
		if err != nil {
			return llm.ToolResult{Output: "Todo item not found: " + params.ItemID, IsError: true}
		}
		if completed == 1 {
			return llm.ToolResult{Output: fmt.Sprintf("%q is already done — nothing to start.", title), IsError: true}
		}
		if inProgress == 1 {
			// Not an error when it is already yours: re-running the same
			// scheduled job should be a no-op, not a failure.
			if startedBy.Valid && startedBy.String == agentSlug {
				resp, _ := json.Marshal(map[string]interface{}{"id": params.ItemID, "in_progress": true, "note": "Already yours — carry on."})
				return llm.ToolResult{Output: string(resp)}
			}
			who := "another agent"
			if startedBy.Valid && startedBy.String != "" {
				who = startedBy.String
			}
			when := ""
			if startedAt.Valid {
				when = " (since " + startedAt.Time.Format(time.RFC3339) + ")"
			}
			return llm.ToolResult{
				Output:  fmt.Sprintf("%q is already in progress, claimed by %s%s. Pick something else — call todo_next_item.", title, who, when),
				IsError: true,
			}
		}

		now := time.Now().UTC()
		actorNote := fmt.Sprintf("Started by agent %s", agentSlug)
		if params.Note != "" {
			actorNote = params.Note
		}
		if _, err := db.Exec(
			"UPDATE todo_items SET in_progress = 1, started_at = ?, started_by_agent_slug = ?, last_actor_agent_slug = ?, last_actor_note = ?, updated_at = ? WHERE id = ?",
			now, agentSlug, agentSlug, actorNote, now, params.ItemID,
		); err != nil {
			return llm.ToolResult{Output: "Failed to start item: " + err.Error(), IsError: true}
		}

		db.LogAudit("system", "todo_item_started", "todo", "todo_item", params.ItemID, "agent="+agentSlug)
		if broadcast != nil {
			broadcast("todo_updated", map[string]interface{}{"type": "item_started", "item_id": params.ItemID})
		}

		resp, _ := json.Marshal(map[string]interface{}{
			"id":          params.ItemID,
			"title":       title,
			"in_progress": true,
		})
		return llm.ToolResult{Output: string(resp)}
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

		// Completing releases the claim: an item both done and in progress would
		// be a contradiction, and the claim is what other runs read to skip it.
		result, err := db.Exec(
			"UPDATE todo_items SET completed = 1, completed_at = ?, in_progress = 0, last_actor_agent_slug = ?, last_actor_note = ?, updated_at = ? WHERE id = ?",
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

		// Unchecking is also how work is handed back, so it clears the claim too
		// — otherwise an abandoned item stays invisible to todo_next_item.
		result, err := db.Exec(
			"UPDATE todo_items SET completed = 0, completed_at = NULL, in_progress = 0, started_at = NULL, started_by_agent_slug = NULL, last_actor_agent_slug = ?, last_actor_note = ?, updated_at = ? WHERE id = ?",
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

// todoAttachmentPaths flattens a task's stored attachments into the shape an
// agent needs: what it is, and where to open it. Unparseable JSON yields
// nothing rather than an error — a malformed attachment list should not stop
// an agent reading the task itself.
func todoAttachmentPaths(raw string) []map[string]string {
	var stored []struct {
		Kind string `json:"kind"`
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if json.Unmarshal([]byte(raw), &stored) != nil {
		return nil
	}
	out := make([]map[string]string, 0, len(stored))
	for _, a := range stored {
		if a.Path == "" {
			continue
		}
		out = append(out, map[string]string{"kind": a.Kind, "name": a.Name, "path": a.Path})
	}
	return out
}
