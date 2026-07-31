package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/models"
)

// The canvas tool: an agent putting something on the chat's preview pane.
//
// An agent that starts a dev server can only say "it's at localhost:5173" and
// hope the user goes and looks. canvas_show opens it beside the conversation
// instead, which is the whole point of working on something local — change,
// look, say what's next, without leaving the chat.
//
// Local files go through the canvas file route rather than file://, which an
// iframe on an http page cannot load. Both cases end up as one URL, which is
// all the frontend is handed.

// canvasFSPrefix mirrors handlers.CanvasFSPrefix. Duplicated rather than
// imported because handlers already imports this package.
const canvasFSPrefix = "/api/v1/canvas/fs/"

// BuildCanvasToolDefs returns the canvas tool definitions.
func BuildCanvasToolDefs() []llm.ToolDef {
	previewParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type": "string",
				"description": "A URL to show — usually a local dev server you just started, " +
					"e.g. \"http://localhost:5173\". Any http(s) URL works.",
			},
			"path": map[string]interface{}{
				"type": "string",
				"description": "Absolute path to a local file or folder to show instead of a URL, " +
					"e.g. \"/Users/me/site/index.html\". A folder shows its index.html. " +
					"Relative links inside the page keep working.",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Short label for what is being shown, e.g. \"Landing page\".",
			},
		},
	})
	documentParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The Context document ID from list_context_documents, or its exact name.",
			},
		},
		"required": []string{"id"},
	})
	return []llm.ToolDef{
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name: "canvas_show",
				Description: "Show a page on the canvas next to this chat — a local dev server, a built " +
					"site, an HTML file. Call it as soon as you have something to look at: after starting a " +
					"dev server (give the server a moment first), after building a page, or when the user " +
					"asks to see what you changed. Opening the canvas rearranges their screen, so do it when " +
					"there is genuinely something to see, not for every edit.",
				Parameters: previewParams,
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "canvas_show_document",
				Description: "Open a text document from this workspace's Context library in the chat canvas so the user can read and edit it beside the conversation. Use after creating or updating a PRD, spec, plan, note, or other document when the user wants to work on or see it.",
				Parameters:  documentParams,
			},
		},
	}
}

// MakeCanvasToolHandlers builds the canvas handlers for one run, with the
// thread baked in so the canvas opens where the conversation is.
func (m *Manager) MakeCanvasToolHandlers(threadID, workspaceID, agentRoleSlug string) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"canvas_show":          m.handleCanvasShow(threadID, agentRoleSlug),
		"canvas_show_document": m.handleCanvasShowDocument(threadID, workspaceID, agentRoleSlug),
	}
}

func (m *Manager) handleCanvasShow(threadID, agentRoleSlug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var req struct {
			URL   string `json:"url"`
			Path  string `json:"path"`
			Title string `json:"title"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}

		target, err := resolveCanvasTarget(strings.TrimSpace(req.URL), strings.TrimSpace(req.Path), workDir)
		if err != nil {
			return llm.ToolResult{Output: err.Error(), IsError: true}
		}
		if threadID == "" {
			return llm.ToolResult{Output: "There is no chat to show this in.", IsError: true}
		}

		if m.broadcast != nil {
			m.broadcast("canvas_open", models.WSCanvasOpen{
				ThreadID:      threadID,
				URL:           target,
				Title:         strings.TrimSpace(req.Title),
				AgentRoleSlug: agentRoleSlug,
			})
		}
		logger.Info("canvas_show: thread=%s url=%s", threadID, target)

		out, _ := json.Marshal(map[string]interface{}{
			"shown": target,
			"note":  "The canvas is open beside the chat. Tell the user what to look at — do not paste the URL as if they cannot see it.",
		})
		return llm.ToolResult{Output: string(out)}
	}
}

func (m *Manager) handleCanvasShowDocument(threadID, workspaceID, agentRoleSlug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		ref := strings.TrimSpace(req.ID)
		if ref == "" {
			return llm.ToolResult{Output: "id is required", IsError: true}
		}
		if threadID == "" {
			return llm.ToolResult{Output: "There is no chat to show this in.", IsError: true}
		}

		var id, name, mimeType string
		if err := m.db.QueryRow(
			`SELECT id, name, mime_type FROM context_files
			 WHERE workspace_id = ? AND is_about_you = 0 AND (id = ? OR LOWER(name) = LOWER(?))
			 ORDER BY id = ? DESC LIMIT 1`,
			workspaceID, ref, ref, ref,
		).Scan(&id, &name, &mimeType); err != nil {
			return llm.ToolResult{Output: fmt.Sprintf("No Context document %q in this workspace.", ref), IsError: true}
		}
		if !isTextDocument(mimeType) {
			return llm.ToolResult{Output: fmt.Sprintf("%q is not an editable text document.", name), IsError: true}
		}

		if m.broadcast != nil {
			m.broadcast("canvas_open", models.WSCanvasOpen{
				ThreadID:      threadID,
				DocumentID:    id,
				Title:         name,
				AgentRoleSlug: agentRoleSlug,
			})
		}
		logger.Info("canvas_show_document: thread=%s document=%s", threadID, id)

		out, _ := json.Marshal(map[string]interface{}{
			"shown_document_id": id,
			"name":              name,
			"note":              "The document is open in the canvas and remains editable by the user. Continue collaborating on it through the Context document tools.",
		})
		return llm.ToolResult{Output: string(out)}
	}
}

// resolveCanvasTarget turns whichever of url/path the agent gave into the one
// URL the canvas loads.
func resolveCanvasTarget(rawURL, path, workDir string) (string, error) {
	if rawURL == "" && path == "" {
		return "", fmt.Errorf("give either a url or a path")
	}

	if rawURL != "" {
		// A bare "localhost:5173" is the most likely way for this to arrive, and
		// it parses as a scheme-less URL that no iframe will load.
		if !strings.Contains(rawURL, "://") {
			rawURL = "http://" + rawURL
		}
		u, err := url.Parse(rawURL)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("%q is not a URL I can show", rawURL)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", fmt.Errorf("the canvas shows http and https URLs; use the path argument for local files")
		}
		return u.String(), nil
	}

	// A relative path is measured from the agent's working directory, which is
	// where it has just been building things.
	abs := path
	if !filepath.IsAbs(abs) {
		if workDir == "" {
			return "", fmt.Errorf("give an absolute path")
		}
		abs = filepath.Join(workDir, abs)
	}
	abs = filepath.Clean(abs)

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("there is nothing at %s", abs)
	}
	if info.IsDir() {
		index := filepath.Join(abs, "index.html")
		if _, err := os.Stat(index); err != nil {
			return "", fmt.Errorf("%s has no index.html — point at a file instead", abs)
		}
		abs = index
	}

	return canvasURLForPath(abs), nil
}

// canvasURLForPath maps an absolute path onto the canvas file route, escaping
// each segment so "/" stays a separator and spaces survive.
func canvasURLForPath(abs string) string {
	parts := strings.Split(strings.TrimPrefix(filepath.ToSlash(abs), "/"), "/")
	for i, p := range parts {
		parts[i] = (&url.URL{Path: p}).EscapedPath()
	}
	return canvasFSPrefix + strings.Join(parts, "/")
}

// buildCanvasPromptSection tells agents the canvas exists and when to use it.
func buildCanvasPromptSection() string {
	return `## THE CANVAS — SHOWING YOUR WORK

There is a canvas beside this chat. ` + "`canvas_show`" + ` puts a page in it: a local dev server you started, a built site, an HTML file. ` + "`canvas_show_document`" + ` opens a text document from Context in an editor beside the conversation.

- Started a dev server? Give it a few seconds, then ` + "`canvas_show`" + ` with its URL (` + "`{\"url\": \"http://localhost:5173\"}`" + `).
- Built or edited a page on disk? ` + "`canvas_show`" + ` with its path (` + "`{\"path\": \"/Users/me/site/index.html\"}`" + `).
- Created or updated a PRD, spec, plan, note, or other Context document that the user wants to see or work on? ` + "`canvas_show_document`" + ` with its ID or exact name.

Opening the canvas rearranges the user's screen and closes any reply thread, so show something when there is genuinely something to look at—not after every edit. Once it is open, describe what changed rather than repeating the URL or document contents.`
}
