package agents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
)

// BuildContextToolDefs returns tool definitions that let agents create and
// manage context documents (knowledge stored in the Context tab).
func BuildContextToolDefs() []llm.ToolDef {
	return []llm.ToolDef{
		buildCreateContextDocumentDef(),
		buildListContextDocumentsDef(),
		buildUpdateContextDocumentDef(),
	}
}

// MakeContextToolHandlers returns handler closures for the context-document
// tools, capturing the data dir (for on-disk storage) and the agentSlug (for
// audit attribution). The optional broadcast function notifies the frontend.
func MakeContextToolHandlers(db *database.DB, dataDir, agentSlug string, broadcast func(string, interface{})) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"create_context_document": handleCreateContextDocument(db, dataDir, agentSlug, broadcast),
		"list_context_documents":  handleListContextDocuments(db),
		"update_context_document": handleUpdateContextDocument(db, dataDir, agentSlug, broadcast),
	}
}

// buildContextPromptSection summarises existing context documents so the agent
// knows what already exists before creating new ones.
func buildContextPromptSection(db *database.DB) string {
	rows, err := db.Query(`SELECT name FROM context_files WHERE is_about_you = 0 ORDER BY updated_at DESC LIMIT 25`)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) != nil {
			continue
		}
		names = append(names, name)
	}

	section := "## CONTEXT DOCUMENTS\nYou can save knowledge as reusable context documents (markdown) with the `create_context_document` tool, list them with `list_context_documents`, and revise them with `update_context_document`. Create one whenever the user asks you to write, save, or remember a document, note, spec, or summary."
	if len(names) > 0 {
		section += "\n\nExisting documents:\n- " + strings.Join(names, "\n- ")
	}
	return section
}

// --- Tool Definitions ---

func buildCreateContextDocumentDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "A short, descriptive title for the document (e.g. \"Onboarding Checklist\").",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The full document body in Markdown.",
			},
			"folder_id": map[string]interface{}{
				"type":        "string",
				"description": "Optional ID of a context folder to file the document under. Omit to place it at the top level.",
			},
		},
		"required": []string{"name", "content"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "create_context_document",
			Description: "Create and persist a new context document (Markdown) in the user's Context library. Use this when asked to create, write, or save a document, note, spec, or summary for later reference.",
			Parameters:  params,
		},
	}
}

func buildListContextDocumentsDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "list_context_documents",
			Description: "List existing context documents (id, name, folder) so you can reference or update them.",
			Parameters:  params,
		},
	}
}

func buildUpdateContextDocumentDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the context document to update (from list_context_documents).",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Optional new title for the document.",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Optional new Markdown body. Replaces the existing content entirely.",
			},
		},
		"required": []string{"id"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "update_context_document",
			Description: "Update the title and/or content of an existing context document.",
			Parameters:  params,
		},
	}
}

// --- Handlers ---

func contextDir(dataDir string) string {
	return filepath.Join(dataDir, "context")
}

func handleCreateContextDocument(db *database.DB, dataDir, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			Name     string `json:"name"`
			Content  string `json:"content"`
			FolderID string `json:"folder_id"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		params.Name = strings.TrimSpace(params.Name)
		if params.Name == "" {
			return llm.ToolResult{Output: "name is required", IsError: true}
		}
		if strings.TrimSpace(params.Content) == "" {
			return llm.ToolResult{Output: "content is required", IsError: true}
		}

		dir := contextDir(dataDir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return llm.ToolResult{Output: "Failed to create storage: " + err.Error(), IsError: true}
		}

		id := uuid.New().String()
		diskFilename := id + ".md"
		if err := os.WriteFile(filepath.Join(dir, diskFilename), []byte(params.Content), 0644); err != nil {
			return llm.ToolResult{Output: "Failed to write document: " + err.Error(), IsError: true}
		}

		var folderID interface{}
		if strings.TrimSpace(params.FolderID) != "" {
			folderID = params.FolderID
		}
		now := time.Now().UTC()
		_, err := db.Exec(
			"INSERT INTO context_files (id, folder_id, name, filename, mime_type, size_bytes, is_about_you, workspace_id, created_at, updated_at) VALUES (?, ?, ?, ?, 'text/markdown', ?, 0, ?, ?, ?)",
			id, folderID, params.Name, diskFilename, int64(len(params.Content)), db.ActiveWorkspaceID(), now, now,
		)
		if err != nil {
			os.Remove(filepath.Join(dir, diskFilename))
			return llm.ToolResult{Output: "Failed to save document: " + err.Error(), IsError: true}
		}

		db.LogAudit("system", "context_document_created", "context", "context_file", id, "agent="+agentSlug+" name="+params.Name)
		if broadcast != nil {
			broadcast("context_updated", map[string]interface{}{"type": "document_created", "file_id": id})
		}

		result, _ := json.Marshal(map[string]interface{}{
			"id":      id,
			"name":    params.Name,
			"created": true,
		})
		return llm.ToolResult{Output: string(result)}
	}
}

func handleListContextDocuments(db *database.DB) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		rows, err := db.Query(`SELECT id, name, COALESCE(folder_id, ''), size_bytes, updated_at FROM context_files WHERE is_about_you = 0 ORDER BY updated_at DESC LIMIT 200`)
		if err != nil {
			return llm.ToolResult{Output: "Failed to list documents: " + err.Error(), IsError: true}
		}
		defer rows.Close()

		type doc struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			FolderID  string `json:"folder_id,omitempty"`
			SizeBytes int64  `json:"size_bytes"`
			UpdatedAt string `json:"updated_at"`
		}
		var docs []doc
		for rows.Next() {
			var d doc
			var updated time.Time
			if rows.Scan(&d.ID, &d.Name, &d.FolderID, &d.SizeBytes, &updated) != nil {
				continue
			}
			d.UpdatedAt = updated.Format(time.RFC3339)
			docs = append(docs, d)
		}
		out, _ := json.Marshal(map[string]interface{}{"documents": docs, "count": len(docs)})
		return llm.ToolResult{Output: string(out)}
	}
}

func handleUpdateContextDocument(db *database.DB, dataDir, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			ID      string  `json:"id"`
			Name    *string `json:"name"`
			Content *string `json:"content"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if strings.TrimSpace(params.ID) == "" {
			return llm.ToolResult{Output: "id is required", IsError: true}
		}
		if params.Name == nil && params.Content == nil {
			return llm.ToolResult{Output: "provide name and/or content to update", IsError: true}
		}

		var filename string
		if err := db.QueryRow("SELECT filename FROM context_files WHERE id = ?", params.ID).Scan(&filename); err != nil {
			return llm.ToolResult{Output: "document not found", IsError: true}
		}

		now := time.Now().UTC()
		if params.Content != nil {
			diskPath := filepath.Join(contextDir(dataDir), filepath.Base(filename))
			if err := os.WriteFile(diskPath, []byte(*params.Content), 0644); err != nil {
				return llm.ToolResult{Output: "Failed to write document: " + err.Error(), IsError: true}
			}
			if _, err := db.Exec("UPDATE context_files SET size_bytes = ?, updated_at = ? WHERE id = ?", int64(len(*params.Content)), now, params.ID); err != nil {
				return llm.ToolResult{Output: "Failed to update document: " + err.Error(), IsError: true}
			}
		}
		if params.Name != nil {
			name := strings.TrimSpace(*params.Name)
			if name == "" {
				return llm.ToolResult{Output: "name cannot be empty", IsError: true}
			}
			if _, err := db.Exec("UPDATE context_files SET name = ?, updated_at = ? WHERE id = ?", name, now, params.ID); err != nil {
				return llm.ToolResult{Output: "Failed to update document: " + err.Error(), IsError: true}
			}
		}

		db.LogAudit("system", "context_document_updated", "context", "context_file", params.ID, "agent="+agentSlug)
		if broadcast != nil {
			broadcast("context_updated", map[string]interface{}{"type": "document_updated", "file_id": params.ID})
		}

		out, _ := json.Marshal(map[string]interface{}{"id": params.ID, "updated": true})
		return llm.ToolResult{Output: string(out)}
	}
}
