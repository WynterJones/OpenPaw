package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/models"
)

// BuildReactionToolDefs returns tool definitions for the react_to_message tool.
func BuildReactionToolDefs() []llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message_id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the message to react to (from the [msg_id:xxx] prefix in conversation history)",
			},
			"emojis": map[string]interface{}{
				"type":        "array",
				"description": "One or more emoji reactions to add",
				"items":       map[string]interface{}{"type": "string"},
			},
		},
		"required": []string{"message_id", "emojis"},
	})
	return []llm.ToolDef{
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "react_to_message",
				Description: "React to a message with emoji(s). Use this to acknowledge, appreciate, or respond to a message without a full reply. Keep it natural and occasional.",
				Parameters:  params,
			},
		},
	}
}

// MakeReactionToolHandlers returns handler closures for the react_to_message tool.
func MakeReactionToolHandlers(db *database.DB, agentSlug string, broadcast func(string, interface{})) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"react_to_message": handleReactToMessage(db, agentSlug, broadcast),
	}
}

func handleReactToMessage(db *database.DB, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			MessageID string   `json:"message_id"`
			Emojis    []string `json:"emojis"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.MessageID == "" {
			return llm.ToolResult{Output: "message_id is required", IsError: true}
		}
		if len(params.Emojis) == 0 {
			return llm.ToolResult{Output: "at least one emoji is required", IsError: true}
		}

		// Verify message exists and get thread ID
		var threadID string
		if err := db.QueryRow("SELECT thread_id FROM chat_messages WHERE id = ?", params.MessageID).Scan(&threadID); err != nil {
			return llm.ToolResult{Output: "Message not found: " + params.MessageID, IsError: true}
		}

		added := 0
		now := time.Now().UTC()
		for _, emoji := range params.Emojis {
			if emoji == "" {
				continue
			}
			// Check if already exists
			var existingID string
			err := db.QueryRow(
				"SELECT id FROM chat_message_reactions WHERE message_id = ? AND emoji = ? AND source = ?",
				params.MessageID, emoji, agentSlug,
			).Scan(&existingID)
			if err == nil {
				continue // already reacted with this emoji
			}
			id := uuid.New().String()
			if _, err := db.Exec(
				"INSERT INTO chat_message_reactions (id, message_id, emoji, source, created_at) VALUES (?, ?, ?, ?, ?)",
				id, params.MessageID, emoji, agentSlug, now,
			); err == nil {
				added++
			}
		}

		// Load updated reactions and broadcast
		reactions := loadReactions(db, params.MessageID)
		if broadcast != nil {
			broadcast("message_reacted", models.WSMessageReacted{
				ThreadID:  threadID,
				MessageID: params.MessageID,
				Reactions: reactions,
			})
		}

		result, _ := json.Marshal(map[string]interface{}{
			"message_id":     params.MessageID,
			"emojis_added":   added,
			"total_reactions": len(reactions),
		})
		return llm.ToolResult{Output: string(result)}
	}
}

func loadReactions(db *database.DB, messageID string) []models.Reaction {
	rows, err := db.Query(
		"SELECT emoji, source, COUNT(*) as count FROM chat_message_reactions WHERE message_id = ? GROUP BY emoji, source ORDER BY MIN(created_at) ASC",
		messageID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var reactions []models.Reaction
	for rows.Next() {
		var r models.Reaction
		if rows.Scan(&r.Emoji, &r.Source, &r.Count) == nil {
			reactions = append(reactions, r)
		}
	}
	return reactions
}

// buildReactionPromptSection returns a prompt section telling agents when to use reactions.
func buildReactionPromptSection() string {
	return fmt.Sprintf(`## MESSAGE REACTIONS

You can react to messages with emoji using the react_to_message tool.
Use reactions to:
- Acknowledge a message without a full response (e.g. thumbs up)
- Show appreciation or agreement quickly
- Signal that you've seen something important

Keep reactions natural and occasional — don't react to every message.
Message IDs appear as [msg_id:xxx] prefixes in the conversation history.`)
}
