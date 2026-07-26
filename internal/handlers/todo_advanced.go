package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/media"
	"github.com/openpaw/openpaw/internal/middleware"
)

// TodoAttachment is one piece of material a task carries.
//
// Path is what an agent is handed, so it has to be openable by an agent — an
// absolute path on disk, not a browser URL.
//
// Ref is write-only: the browser knows a context file's id or a media id, but
// not where either lives on disk, and it has no business guessing. The server
// resolves Ref into Path on the way in; from then on only Path matters.
type TodoAttachment struct {
	Kind string `json:"kind"` // image | file | directory | media
	Path string `json:"path"`
	Name string `json:"name"`
	Ref  string `json:"ref,omitempty"`
}

// maxTodoAttachments bounds a single task. Generous for real use, low enough
// that a runaway client cannot write an unbounded blob into the row.
const maxTodoAttachments = 20

// resolveAttachment turns a client-supplied reference into a real path.
//
// Only "file" (a context file id) and "media" (a media id) need resolving —
// images and directories already arrive as absolute paths, because the upload
// endpoint and the folder picker both return one.
func (h *TodoListsHandler) resolveAttachment(a TodoAttachment) TodoAttachment {
	ref := strings.TrimSpace(a.Ref)
	if ref == "" {
		return a
	}
	switch a.Kind {
	case "file":
		var filename string
		if h.db.QueryRow("SELECT filename FROM context_files WHERE id = ?", ref).Scan(&filename) == nil && filename != "" {
			a.Path = filepath.Join(h.dataDir, "context", filename)
		}
	case "media":
		var filename string
		if h.db.QueryRow("SELECT filename FROM media WHERE id = ?", ref).Scan(&filename) == nil && filename != "" {
			a.Path = filepath.Join(media.Dir(h.dataDir), filename)
		}
	}
	return a
}

func (h *TodoListsHandler) encodeAttachments(list []TodoAttachment) string {
	resolved := make([]TodoAttachment, 0, len(list))
	for _, a := range list {
		resolved = append(resolved, h.resolveAttachment(a))
	}
	return encodeAttachments(resolved)
}

func encodeAttachments(list []TodoAttachment) string {
	cleaned := make([]TodoAttachment, 0, len(list))
	for _, a := range list {
		a.Path = strings.TrimSpace(a.Path)
		a.Ref = "" // resolved already; never stored
		if a.Path == "" {
			continue
		}
		a.Kind = strings.TrimSpace(a.Kind)
		if a.Kind == "" {
			a.Kind = "file"
		}
		if strings.TrimSpace(a.Name) == "" {
			a.Name = a.Path
		}
		cleaned = append(cleaned, a)
		if len(cleaned) >= maxTodoAttachments {
			break
		}
	}
	b, err := json.Marshal(cleaned)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// decodeAttachments never returns nil — the frontend maps over this directly,
// and a null would crash the list rather than render an empty one.
func decodeAttachments(raw string) []TodoAttachment {
	out := []TodoAttachment{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []TodoAttachment{}
	}
	if out == nil {
		return []TodoAttachment{}
	}
	return out
}

// FormatTodoAttachments renders attachments for an agent: a plain list of real
// paths. Used wherever a task is handed to a model, so "the screenshot" in a
// task body resolves to something the agent can actually open.
func FormatTodoAttachments(raw string) string {
	list := decodeAttachments(raw)
	if len(list) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Attached material (open these directly):\n")
	for _, a := range list {
		fmt.Fprintf(&b, "- %s (%s): %s\n", a.Name, a.Kind, a.Path)
	}
	return b.String()
}

// enhanceSystemPrompt turns a terse task into something an agent can act on.
//
// Constrained hard against invention: the failure mode of "make this a better
// prompt" is a model that confidently adds requirements nobody asked for, and
// a task list is exactly where that goes unnoticed until an agent acts on it.
const enhanceSystemPrompt = `You rewrite a user's task into a clear, actionable prompt for an AI agent.

Rules:
- Preserve the user's intent exactly. Do NOT invent requirements, constraints, deadlines, or acceptance criteria they did not state.
- If something is genuinely ambiguous, say so in one short line rather than guessing.
- Keep it tight: a one-line objective, then only the detail that is actually present in the original.
- Reference any attached files, directories or images by the paths given, and do not invent paths.
- Write in plain prose or short bullets. No preamble, no "Sure!", no headings like "Enhanced Task".
- Output ONLY the rewritten task.`

// EnhanceItem rewrites a task with the connected model.
//
// Stateless — it takes the draft and returns text without touching the
// database, so it works from the composer before the item exists.
func (h *TodoListsHandler) EnhanceItem(w http.ResponseWriter, r *http.Request) {
	if h.agentMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "no AI engine is configured")
		return
	}

	var req struct {
		Title       string           `json:"title"`
		Notes       string           `json:"notes"`
		Attachments []TodoAttachment `json:"attachments"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Title) == "" && strings.TrimSpace(req.Notes) == "" {
		writeError(w, http.StatusBadRequest, "write something to enhance first")
		return
	}

	var prompt strings.Builder
	prompt.WriteString("Task: " + strings.TrimSpace(req.Title) + "\n")
	if n := strings.TrimSpace(req.Notes); n != "" {
		prompt.WriteString("\nDetail the user already wrote:\n" + n + "\n")
	}
	if s := FormatTodoAttachments(h.encodeAttachments(req.Attachments)); s != "" {
		prompt.WriteString("\n" + s)
	}

	provider := h.agentMgr.Provider()
	if provider == nil || !provider.IsConfigured() {
		writeError(w, http.StatusServiceUnavailable, "no AI engine is configured")
		return
	}

	// Short timeout: this runs while the user waits with the composer open, so
	// failing quickly and letting them save the draft beats a long stall.
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	text, _, err := provider.RunOneShot(ctx,
		provider.ResolveModel(h.agentMgr.BuilderModel, llm.ModelSonnet),
		enhanceSystemPrompt, prompt.String())
	if err != nil {
		writeError(w, http.StatusBadGateway, "enhance failed: "+err.Error())
		return
	}

	enhanced := strings.TrimSpace(text)
	if enhanced == "" {
		writeError(w, http.StatusBadGateway, "the model returned nothing")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "todo_item_enhanced", "todo", "todo_item", "", truncateStr(req.Title, 80, true))

	writeJSON(w, http.StatusOK, map[string]string{"notes": enhanced})
}
