package agents

import (
	"context"
	"encoding/json"
	"fmt"
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
		buildReadContextDocumentDef(),
		buildUpdateContextDocumentDef(),
		buildDeleteContextDocumentDef(),
	}
}

// MakeContextToolHandlers returns handler closures for the context-document
// tools, capturing the data dir (for on-disk storage) and the agentSlug (for
// audit attribution). The optional broadcast function notifies the frontend.
func MakeContextToolHandlers(db *database.DB, dataDir, workspaceID, agentSlug string, broadcast func(string, interface{})) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"create_context_document": handleCreateContextDocument(db, dataDir, workspaceID, agentSlug, broadcast),
		"list_context_documents":  handleListContextDocuments(db, workspaceID),
		"read_context_document":   handleReadContextDocument(db, dataDir, workspaceID),
		"update_context_document": handleUpdateContextDocument(db, dataDir, workspaceID, agentSlug, broadcast),
		"delete_context_document": handleDeleteContextDocument(db, dataDir, workspaceID, agentSlug, broadcast),
	}
}

// buildContextPromptSection summarises existing context documents so the agent
// knows what already exists before creating new ones.
func buildContextPromptSection(db *database.DB, workspaceID string) string {
	rows, err := db.Query(`SELECT name FROM context_files WHERE is_about_you = 0 AND workspace_id = ? ORDER BY updated_at DESC LIMIT 25`, workspaceID)
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

	section := "## CONTEXT DOCUMENTS\n" +
		"You have full CRUD access to this workspace's reusable Context documents (Markdown): create, list, read, update, and delete. Use a document for prose the user will read or refine—PRDs, specs, research, plans, checklists, big ideas, meeting notes, and curated lists. Use a database instead when the information is a repeated set of structured records that benefits from fields, filtering, sorting, or dashboards.\n\n" +
		"When durable output would clearly help but the user did not ask to save it, briefly offer to make a Context document or database, whichever fits. Do not interrupt every answer with this offer and do not create one silently. If the user explicitly asks to write, make, save, or keep it, create it directly.\n\n" +
		"ALWAYS `read_context_document` before `update_context_document`. Updating replaces the entire file, and the user edits these documents by hand between conversations—writing from memory silently destroys whatever they changed. Read it, apply your change to what is actually there, then write the whole document back. Only delete a document when the user explicitly asks; never delete one merely to reorganize."
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
			Description: "Create and persist a new Context document (Markdown) in this workspace. Use when asked to create, write, or save a PRD, spec, plan, note, summary, big-idea document, checklist, or curated list for later reference.",
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

func buildReadContextDocumentDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The document's ID from list_context_documents, or its exact name.",
			},
		},
		"required": []string{"id"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "read_context_document",
			Description: "Read a context document's current contents. Always call this before update_context_document — the document may have been edited since you last saw it, and updating replaces the whole file.",
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

func buildDeleteContextDocumentDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The document's ID from list_context_documents.",
			},
		},
		"required": []string{"id"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "delete_context_document",
			Description: "Permanently delete a Context document. Only use when the user explicitly asks to delete it.",
			Parameters:  params,
		},
	}
}

// --- Handlers ---

func contextDir(dataDir string) string {
	return filepath.Join(dataDir, "context")
}

func handleCreateContextDocument(db *database.DB, dataDir, workspaceID, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
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
			var exists int
			if err := db.QueryRow("SELECT COUNT(*) FROM context_folders WHERE id = ? AND workspace_id = ?", params.FolderID, workspaceID).Scan(&exists); err != nil || exists == 0 {
				os.Remove(filepath.Join(dir, diskFilename))
				return llm.ToolResult{Output: "folder not found in this workspace", IsError: true}
			}
			folderID = params.FolderID
		}
		now := time.Now().UTC()
		_, err := db.Exec(
			"INSERT INTO context_files (id, folder_id, name, filename, mime_type, size_bytes, is_about_you, workspace_id, created_at, updated_at) VALUES (?, ?, ?, ?, 'text/markdown', ?, 0, ?, ?, ?)",
			id, folderID, params.Name, diskFilename, int64(len(params.Content)), workspaceID, now, now,
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

func handleListContextDocuments(db *database.DB, workspaceID string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		rows, err := db.Query(`SELECT id, name, COALESCE(folder_id, ''), size_bytes, updated_at FROM context_files WHERE is_about_you = 0 AND workspace_id = ? ORDER BY updated_at DESC LIMIT 200`, workspaceID)
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

// maxContextReadBytes caps what a single document can add to an agent's
// context. Documents are usually notes and specs; a large upload should not be
// able to consume the whole window in one tool call.
const maxContextReadBytes = 200_000

// handleReadContextDocument returns a document's current contents.
//
// Read straight from disk rather than from anything cached, because the point
// is to see edits: the Context tab writes the same file (handlers/context.go
// UpdateFile), so a document the user changed by hand comes back changed. Until
// this existed an agent could list documents and overwrite them but never see
// one — so "add a section to that doc" in a new conversation meant rewriting it
// blind, silently dropping whatever the user had put there.
func handleReadContextDocument(db *database.DB, dataDir, workspaceID string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		ref := strings.TrimSpace(params.ID)
		if ref == "" {
			return llm.ToolResult{Output: "id is required", IsError: true}
		}

		// Agents are shown document names, not IDs, so accept either.
		var id, name, filename, mimeType string
		var updated time.Time
		err := db.QueryRow(
			`SELECT id, name, filename, mime_type, updated_at FROM context_files
			 WHERE is_about_you = 0 AND workspace_id = ? AND (id = ? OR LOWER(name) = LOWER(?))
			 ORDER BY id = ? DESC LIMIT 1`,
			workspaceID, ref, ref, ref,
		).Scan(&id, &name, &filename, &mimeType, &updated)
		if err != nil {
			return llm.ToolResult{
				Output:  fmt.Sprintf("No document %q. Call list_context_documents for the available names and IDs.", ref),
				IsError: true,
			}
		}

		if !isTextDocument(mimeType) {
			return llm.ToolResult{
				Output:  fmt.Sprintf("%q is a %s file, which cannot be read as text.", name, mimeType),
				IsError: true,
			}
		}

		data, err := os.ReadFile(filepath.Join(contextDir(dataDir), filepath.Base(filename)))
		if err != nil {
			return llm.ToolResult{Output: "Could not read " + name + ": " + err.Error(), IsError: true}
		}

		content := string(data)
		truncated := false
		if len(content) > maxContextReadBytes {
			content = content[:maxContextReadBytes]
			truncated = true
		}

		out, _ := json.Marshal(map[string]interface{}{
			"id":         id,
			"name":       name,
			"updated_at": updated.Format(time.RFC3339),
			"content":    content,
			"truncated":  truncated,
		})
		return llm.ToolResult{Output: string(out)}
	}
}

// isTextDocument reports whether a context file can be handed to a model as
// text. Mirrors isTextMime in handlers/context.go.
func isTextDocument(mime string) bool {
	return strings.HasPrefix(mime, "text/") ||
		mime == "application/json" ||
		mime == "application/xml" ||
		mime == "application/javascript"
}

func handleUpdateContextDocument(db *database.DB, dataDir, workspaceID, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
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
		var nextName string
		if params.Name != nil {
			nextName = strings.TrimSpace(*params.Name)
			if nextName == "" {
				return llm.ToolResult{Output: "name cannot be empty", IsError: true}
			}
		}

		var filename string
		if err := db.QueryRow("SELECT filename FROM context_files WHERE id = ? AND workspace_id = ? AND is_about_you = 0", params.ID, workspaceID).Scan(&filename); err != nil {
			return llm.ToolResult{Output: "document not found", IsError: true}
		}

		now := time.Now().UTC()
		if params.Content != nil {
			diskPath := filepath.Join(contextDir(dataDir), filepath.Base(filename))
			if err := os.WriteFile(diskPath, []byte(*params.Content), 0644); err != nil {
				return llm.ToolResult{Output: "Failed to write document: " + err.Error(), IsError: true}
			}
			if _, err := db.Exec("UPDATE context_files SET size_bytes = ?, updated_at = ? WHERE id = ? AND workspace_id = ?", int64(len(*params.Content)), now, params.ID, workspaceID); err != nil {
				return llm.ToolResult{Output: "Failed to update document: " + err.Error(), IsError: true}
			}
		}
		if params.Name != nil {
			if _, err := db.Exec("UPDATE context_files SET name = ?, updated_at = ? WHERE id = ? AND workspace_id = ?", nextName, now, params.ID, workspaceID); err != nil {
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

func handleDeleteContextDocument(db *database.DB, dataDir, workspaceID, agentSlug string, broadcast func(string, interface{})) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		params.ID = strings.TrimSpace(params.ID)
		if params.ID == "" {
			return llm.ToolResult{Output: "id is required", IsError: true}
		}

		var name, filename string
		if err := db.QueryRow(
			"SELECT name, filename FROM context_files WHERE id = ? AND workspace_id = ? AND is_about_you = 0",
			params.ID, workspaceID,
		).Scan(&name, &filename); err != nil {
			return llm.ToolResult{Output: "document not found", IsError: true}
		}

		if err := os.Remove(filepath.Join(contextDir(dataDir), filepath.Base(filename))); err != nil && !os.IsNotExist(err) {
			return llm.ToolResult{Output: "Failed to remove stored document: " + err.Error(), IsError: true}
		}
		if _, err := db.Exec("DELETE FROM context_files WHERE id = ? AND workspace_id = ?", params.ID, workspaceID); err != nil {
			return llm.ToolResult{Output: "Failed to delete document record: " + err.Error(), IsError: true}
		}

		db.LogAudit("system", "context_document_deleted", "context", "context_file", params.ID, "agent="+agentSlug+" name="+name)
		if broadcast != nil {
			broadcast("context_updated", map[string]interface{}{"type": "document_deleted", "file_id": params.ID})
		}
		out, _ := json.Marshal(map[string]interface{}{"id": params.ID, "name": name, "deleted": true})
		return llm.ToolResult{Output: string(out)}
	}
}
