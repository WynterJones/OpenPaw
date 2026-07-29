package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/models"
)

// BuildInboxToolDefs gives every role-chat agent workspace-scoped CRUD over the
// reports/posts visible in Inbox. Archive is separate from delete so the common
// "summarize these, then archive them" workflow remains recoverable.
func BuildInboxToolDefs() []llm.ToolDef {
	return []llm.ToolDef{
		databaseToolDef("list_inbox_posts",
			"List or search Inbox reports/posts in this workspace, newest first. Read detail for the full report. Use status=archived for the archive or status=all for both.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"search":      map[string]interface{}{"type": "string", "description": "Case-insensitive search across title, preview, full detail, and prompt"},
					"status":      map[string]interface{}{"type": "string", "enum": []string{"active", "archived", "all"}, "default": "active"},
					"unread_only": map[string]interface{}{"type": "boolean", "default": false},
					"source_type": map[string]interface{}{"type": "string", "description": "Optional source filter such as schedule or heartbeat"},
					"limit":       map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 500, "default": 50},
				},
			}),
		databaseToolDef("manage_inbox_post",
			"Create, update, archive, restore, permanently delete, mark read, or mark unread an Inbox post in this workspace.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type": "string",
						"enum": []string{"create", "update", "archive", "restore", "delete", "mark_read", "mark_unread"},
					},
					"id":          map[string]interface{}{"type": "string", "description": "Required except for create"},
					"title":       map[string]interface{}{"type": "string"},
					"body":        map[string]interface{}{"type": "string", "description": "Short preview"},
					"detail":      map[string]interface{}{"type": "string", "description": "Full markdown post/report"},
					"prompt":      map[string]interface{}{"type": "string"},
					"priority":    map[string]interface{}{"type": "string", "enum": []string{"low", "normal", "high"}},
					"source_type": map[string]interface{}{"type": "string"},
					"source_id":   map[string]interface{}{"type": "string"},
					"link":        map[string]interface{}{"type": "string"},
				},
				"required": []string{"action"},
			}),
	}
}

func inboxWorkspaceClause(workspaceID string) (string, []interface{}) {
	// Legacy notifications predate workspace_id and are still shown in the
	// Inbox UI. Keep those visible from the active workspace's agent tools;
	// explicitly assigned posts remain isolated to their own workspace.
	return "(workspace_id = ? OR workspace_id = '')", []interface{}{workspaceID}
}

func MakeInboxToolHandlers(db *database.DB, workspaceID, agentSlug string, broadcast func(string, interface{})) map[string]llm.ToolHandler {
	actor := "system"
	if agentSlug != "" {
		actor = "agent:" + agentSlug
	}
	changed := func(kind, id string) {
		if broadcast != nil {
			broadcast(kind, map[string]string{"id": id, "status": "changed"})
		}
	}
	return map[string]llm.ToolHandler{
		"list_inbox_posts": func(_ context.Context, _ string, input json.RawMessage) llm.ToolResult {
			var params struct {
				Search     string `json:"search"`
				Status     string `json:"status"`
				UnreadOnly bool   `json:"unread_only"`
				SourceType string `json:"source_type"`
				Limit      int    `json:"limit"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return databaseToolError(err)
			}
			if params.Limit <= 0 {
				params.Limit = 50
			}
			if params.Limit > 500 {
				params.Limit = 500
			}
			scope, args := inboxWorkspaceClause(workspaceID)
			where := []string{scope}
			switch params.Status {
			case "", "active":
				where = append(where, "dismissed = 0")
			case "archived":
				where = append(where, "dismissed = 1")
			case "all":
			default:
				return databaseToolError(fmt.Errorf("status must be active, archived, or all"))
			}
			if params.UnreadOnly {
				where = append(where, "read = 0")
			}
			if params.SourceType != "" {
				where = append(where, "source_type = ?")
				args = append(args, params.SourceType)
			}
			if search := strings.TrimSpace(params.Search); search != "" {
				where = append(where, "(title LIKE ? OR body LIKE ? OR detail LIKE ? OR prompt LIKE ?)")
				like := "%" + search + "%"
				args = append(args, like, like, like, like)
			}
			args = append(args, params.Limit)
			rows, err := db.Query(`SELECT id, title, body, detail, prompt, workspace_id, priority,
				source_agent_slug, source_type, source_id, link, read, dismissed, created_at
				FROM notifications WHERE `+strings.Join(where, " AND ")+` ORDER BY created_at DESC LIMIT ?`, args...)
			if err != nil {
				return databaseToolError(err)
			}
			defer rows.Close()
			items := []models.Notification{}
			for rows.Next() {
				var item models.Notification
				if err := rows.Scan(&item.ID, &item.Title, &item.Body, &item.Detail, &item.Prompt,
					&item.WorkspaceID, &item.Priority, &item.SourceAgentSlug, &item.SourceType,
					&item.SourceID, &item.Link, &item.Read, &item.Dismissed, &item.CreatedAt); err != nil {
					return databaseToolError(err)
				}
				items = append(items, item)
			}
			return databaseToolJSON(items)
		},
		"manage_inbox_post": func(_ context.Context, _ string, input json.RawMessage) llm.ToolResult {
			var params struct {
				Action     string  `json:"action"`
				ID         string  `json:"id"`
				Title      *string `json:"title"`
				Body       *string `json:"body"`
				Detail     *string `json:"detail"`
				Prompt     *string `json:"prompt"`
				Priority   *string `json:"priority"`
				SourceType *string `json:"source_type"`
				SourceID   *string `json:"source_id"`
				Link       *string `json:"link"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return databaseToolError(err)
			}
			action := strings.ToLower(strings.TrimSpace(params.Action))
			scope, scopeArgs := inboxWorkspaceClause(workspaceID)

			if action == "create" {
				if params.Title == nil || strings.TrimSpace(*params.Title) == "" {
					return databaseToolError(fmt.Errorf("title is required"))
				}
				id := uuid.NewString()
				body, detail, prompt, priority, sourceType, sourceID, link := "", "", "", "normal", "agent", "", ""
				if params.Body != nil {
					body = *params.Body
				}
				if params.Detail != nil {
					detail = *params.Detail
				}
				if params.Prompt != nil {
					prompt = *params.Prompt
				}
				if params.Priority != nil {
					priority = *params.Priority
				}
				if params.SourceType != nil {
					sourceType = *params.SourceType
				}
				if params.SourceID != nil {
					sourceID = *params.SourceID
				}
				if params.Link != nil {
					link = *params.Link
				}
				_, err := db.Exec(`INSERT INTO notifications
					(id, title, body, detail, prompt, workspace_id, priority, source_agent_slug, source_type, source_id, link, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					id, strings.TrimSpace(*params.Title), body, detail, prompt, workspaceID, priority,
					agentSlug, sourceType, sourceID, link, time.Now().UTC())
				if err != nil {
					return databaseToolError(err)
				}
				db.LogAudit(actor, "inbox_post_created", "agent", "notification", id, strings.TrimSpace(*params.Title))
				changed("notification_created", id)
				return databaseToolJSON(map[string]string{"id": id, "status": "created"})
			}

			if params.ID == "" {
				return databaseToolError(fmt.Errorf("id is required for %s", action))
			}
			var exists int
			checkArgs := append([]interface{}{params.ID}, scopeArgs...)
			if err := db.QueryRow("SELECT COUNT(*) FROM notifications WHERE id = ? AND "+scope, checkArgs...).Scan(&exists); err != nil || exists == 0 {
				return databaseToolError(fmt.Errorf("inbox post not found in this workspace"))
			}

			var resultStatus string
			switch action {
			case "update":
				sets := []string{}
				args := []interface{}{}
				add := func(column string, value *string) {
					if value != nil {
						sets = append(sets, column+" = ?")
						args = append(args, *value)
					}
				}
				add("title", params.Title)
				add("body", params.Body)
				add("detail", params.Detail)
				add("prompt", params.Prompt)
				add("priority", params.Priority)
				add("source_type", params.SourceType)
				add("source_id", params.SourceID)
				add("link", params.Link)
				if len(sets) == 0 {
					return databaseToolError(fmt.Errorf("provide at least one field to update"))
				}
				args = append(args, params.ID)
				if _, err := db.Exec("UPDATE notifications SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil {
					return databaseToolError(err)
				}
				resultStatus = "updated"
			case "archive":
				_, _ = db.Exec("UPDATE notifications SET dismissed = 1 WHERE id = ?", params.ID)
				resultStatus = "archived"
			case "restore":
				_, _ = db.Exec("UPDATE notifications SET dismissed = 0 WHERE id = ?", params.ID)
				resultStatus = "restored"
			case "mark_read":
				_, _ = db.Exec("UPDATE notifications SET read = 1 WHERE id = ?", params.ID)
				resultStatus = "read"
			case "mark_unread":
				_, _ = db.Exec("UPDATE notifications SET read = 0 WHERE id = ?", params.ID)
				resultStatus = "unread"
			case "delete":
				if _, err := db.Exec("DELETE FROM notifications WHERE id = ?", params.ID); err != nil {
					return databaseToolError(err)
				}
				resultStatus = "deleted"
			default:
				return databaseToolError(fmt.Errorf("unsupported action %q", action))
			}
			db.LogAudit(actor, "inbox_post_"+action, "agent", "notification", params.ID, resultStatus)
			event := "notifications_cleared"
			if action == "mark_read" || action == "mark_unread" {
				event = "notification_read"
			}
			changed(event, params.ID)
			return databaseToolJSON(map[string]string{"id": params.ID, "status": resultStatus})
		},
	}
}

func buildInboxPromptSection() string {
	return "## INBOX POSTS\n" +
		"You can read and search this workspace's Inbox with `list_inbox_posts`, including full report detail, and use `manage_inbox_post` for full CRUD. " +
		"Archive posts after processing when asked; archive is recoverable, while delete is permanent. Never access posts from another workspace.\n"
}
