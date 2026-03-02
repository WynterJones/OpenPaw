package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/models"
)

func BuildAvatarToolDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"media_url": map[string]interface{}{
				"type":        "string",
				"description": "The local media URL of the generated image (e.g. /api/v1/media/{id}/file)",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "A brief description of the avatar for future reference (e.g. 'cartoon cat with pink and black fur')",
			},
		},
		"required": []string{"media_url", "description"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "update_own_avatar",
			Description: "Update your own avatar image. Use after generating an image with generate_image. Pass the local media URL from the generation result.",
			Parameters:  params,
		},
	}
}

func MakeAvatarToolHandler(db *database.DB, dataDir, agentRoleSlug string, broadcast func(string, interface{})) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"update_own_avatar": handleUpdateOwnAvatar(db, dataDir, agentRoleSlug, broadcast),
	}
}

func handleUpdateOwnAvatar(db *database.DB, dataDir, agentRoleSlug string, broadcast func(string, interface{})) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			MediaURL    string `json:"media_url"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.MediaURL == "" {
			return llm.ToolResult{Output: "media_url is required", IsError: true}
		}
		if params.Description == "" {
			return llm.ToolResult{Output: "description is required", IsError: true}
		}

		// Extract media ID from /api/v1/media/{id}/file
		if !strings.HasPrefix(params.MediaURL, "/api/v1/media/") {
			return llm.ToolResult{Output: "media_url must be a local media URL (e.g. /api/v1/media/{id}/file)", IsError: true}
		}
		parts := strings.Split(strings.TrimPrefix(params.MediaURL, "/api/v1/media/"), "/")
		if len(parts) < 1 || parts[0] == "" {
			return llm.ToolResult{Output: "could not extract media ID from URL", IsError: true}
		}
		mediaID := parts[0]

		// Find the source file in the media directory
		mediaDir := filepath.Join(dataDir, "..", "media")
		matches, _ := filepath.Glob(filepath.Join(mediaDir, mediaID+".*"))
		if len(matches) == 0 {
			return llm.ToolResult{Output: "media file not found for ID " + mediaID, IsError: true}
		}
		srcPath := matches[0]

		// Copy to data/avatars/{new-uuid}.{ext}
		ext := filepath.Ext(srcPath)
		newID := uuid.New().String()
		avatarsDir := filepath.Join(dataDir, "avatars")
		os.MkdirAll(avatarsDir, 0755)
		destFilename := newID + ext
		destPath := filepath.Join(avatarsDir, destFilename)

		src, err := os.Open(srcPath)
		if err != nil {
			return llm.ToolResult{Output: "failed to open source image: " + err.Error(), IsError: true}
		}
		defer src.Close()

		dst, err := os.Create(destPath)
		if err != nil {
			return llm.ToolResult{Output: "failed to create avatar file: " + err.Error(), IsError: true}
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			return llm.ToolResult{Output: "failed to copy image: " + err.Error(), IsError: true}
		}

		// Update the agent_roles row
		avatarPath := "/api/v1/uploads/avatars/" + destFilename
		now := time.Now().UTC()
		if _, err := db.Exec(
			"UPDATE agent_roles SET avatar_path = ?, avatar_description = ?, updated_at = ? WHERE slug = ?",
			avatarPath, params.Description, now, agentRoleSlug,
		); err != nil {
			return llm.ToolResult{Output: "failed to update avatar in database: " + err.Error(), IsError: true}
		}

		db.LogAudit("system", "agent_avatar_updated", "agent_role", "agent_role", agentRoleSlug, params.Description)

		if broadcast != nil {
			broadcast("agent_avatar_updated", models.WSAgentAvatarUpdated{
				AgentRoleSlug:     agentRoleSlug,
				AvatarPath:        avatarPath,
				AvatarDescription: params.Description,
			})
		}

		result, _ := json.Marshal(map[string]interface{}{
			"status":      "success",
			"avatar_path": avatarPath,
			"description": params.Description,
		})
		return llm.ToolResult{Output: string(result)}
	}
}

func buildAvatarPromptSection(db *database.DB, agentName string) string {
	accent := "#E84BA5"
	var designJSON string
	if err := db.QueryRow("SELECT value FROM settings WHERE key = 'design_config'").Scan(&designJSON); err == nil {
		var config map[string]interface{}
		if json.Unmarshal([]byte(designJSON), &config) == nil {
			if a, ok := config["accent"].(string); ok && a != "" {
				accent = a
			}
		}
	}

	return fmt.Sprintf(`## AVATAR SELF-UPDATE

You can update your own avatar using a 2-step process:
1. Use generate_image to create an avatar image
2. Use update_own_avatar with the returned media URL and a description

Default style: a cartoon cat character using %s (accent color) and black as the primary colors. Your name is %s — incorporate your personality into the design.
Keep the image square and simple — it will be displayed as a small avatar.`, accent, agentName)
}
