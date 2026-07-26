package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/media"
)

// Studio tools let the chat agent work with the same media library the Studio
// page uses: see what folders exist, look at what has already been made, and
// generate new assets into a folder.
//
// The agent is told to propose before it spends: generation costs real money
// per asset, so the default behaviour is to offer the user a set of prompts as
// options and only generate once they pick one.

func BuildStudioToolDefs(registry *media.Registry) []llm.ToolDef {
	if registry == nil {
		return nil
	}
	defs := []llm.ToolDef{buildStudioFoldersDef(), buildStudioListMediaDef()}
	// Only advertise generation when something can actually generate; a tool
	// the model can call but that always errors is worse than no tool.
	if registry.Supports(media.KindImage) || registry.Supports(media.KindVideo) || registry.Supports(media.KindAudio) {
		defs = append(defs, buildStudioGenerateDef(registry))
	}
	return defs
}

func buildStudioFoldersDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "studio_list_folders",
			Description: "List the Studio media folders in the current workspace, with how many items each holds. Use this before generating so you can file new work somewhere sensible, or when the user asks what folders exist.",
			Parameters:  params,
		},
	}
}

func buildStudioListMediaDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"folder_id": map[string]interface{}{"type": "string", "description": "Only list items in this folder. Pass 'unfiled' for items in no folder. Omit for everything."},
			"type":      map[string]interface{}{"type": "string", "enum": []string{"image", "video", "audio"}, "description": "Only list this media type."},
			"limit":     map[string]interface{}{"type": "integer", "description": "How many items to return (default 25, max 100)."},
		},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "studio_list_media",
			Description: "List media already generated in this workspace — images, video and audio — with their prompts, models and URLs. Use it to answer questions about existing work, to reference an earlier piece, or to avoid regenerating something that already exists.",
			Parameters:  params,
		},
	}
}

func buildStudioGenerateDef(registry *media.Registry) llm.ToolDef {
	kinds := []string{}
	for _, k := range []media.Kind{media.KindImage, media.KindVideo, media.KindAudio} {
		if registry.Supports(k) {
			kinds = append(kinds, string(k))
		}
	}

	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"type":      map[string]interface{}{"type": "string", "enum": kinds, "description": "What to generate."},
			"prompt":    map[string]interface{}{"type": "string", "description": "The generation prompt. Be specific and visual; these models reward detail."},
			"provider":  map[string]interface{}{"type": "string", "description": "Optional provider override (openrouter, replicate, fal). Omit to use the first configured provider that can make this type."},
			"model":     map[string]interface{}{"type": "string", "description": "Optional model id. Omit for the provider default."},
			"count":     map[string]interface{}{"type": "integer", "description": "How many to generate (default 1, max 4 from chat). Each one costs money — do not raise this without the user asking."},
			"size":      map[string]interface{}{"type": "string", "description": "Image size hint: 1024x1024, 1536x1024 or 1024x1536."},
			"duration":  map[string]interface{}{"type": "integer", "description": "Video or audio length in seconds, if the model supports it."},
			"folder_id": map[string]interface{}{"type": "string", "description": "File the result into this Studio folder. Get ids from studio_list_folders."},
		},
		"required": []string{"type", "prompt"},
	})

	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name: "studio_generate",
			Description: "Generate media (" + strings.Join(kinds, ", ") + ") and save it to the Studio library, where it appears in chat and on the Studio page. " +
				"IMPORTANT: generation costs real money per asset. Unless the user has clearly asked you to generate right now, offer 2-3 concrete prompt options in your reply and let them choose, rather than calling this immediately. " +
				"Never generate more than the user asked for.",
			Parameters: params,
		},
	}
}

// buildStudioPromptSection tells the agent what Studio is and, more
// importantly, how to behave around it. Left to its own devices a model will
// happily fire off four video generations because the user said "some clips" —
// each of which is a real charge. The default is therefore propose-then-generate.
func buildStudioPromptSection(registry *media.Registry) string {
	if registry == nil {
		return ""
	}

	var kinds []string
	for _, k := range []media.Kind{media.KindImage, media.KindVideo, media.KindAudio} {
		if registry.Supports(k) {
			kinds = append(kinds, string(k))
		}
	}

	var b strings.Builder
	b.WriteString("## STUDIO (media generation)\n\n")
	b.WriteString("OpenPaw has a Studio for generating media. You can use it too.\n\n")

	if len(kinds) == 0 {
		b.WriteString("No media provider is configured right now, so you cannot generate anything. ")
		b.WriteString("You can still browse existing media with `studio_list_media` and `studio_list_folders`. ")
		b.WriteString("If the user asks for generation, tell them to add an OpenRouter key (images) or a Replicate/fal key (video, music) in Settings.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "You can currently generate: **%s**.\n\n", strings.Join(kinds, ", "))
	b.WriteString("- `studio_list_folders` — see the folders media can be filed into.\n")
	b.WriteString("- `studio_list_media` — see what has already been made, with prompts and URLs.\n")
	b.WriteString("- `studio_generate` — generate and save new media.\n\n")

	b.WriteString("### How to handle generation requests\n\n")
	b.WriteString("Every generation costs the user real money, and video costs substantially more than images. So:\n\n")
	b.WriteString("1. **Offer before you spend.** When the user describes something they want, reply with 2-3 specific, well-crafted prompt options and ask which to run — do not generate immediately. Number them so they can just say \"2\".\n")
	b.WriteString("2. **Generate when asked.** If the user says \"make it\", \"go ahead\", picks an option, or asked for generation outright, call `studio_generate` without further questions.\n")
	b.WriteString("3. **Respect the count.** Generate exactly as many as asked. If no number was given, generate one.\n")
	b.WriteString("4. **File it sensibly.** Check `studio_list_folders` and pass a `folder_id` when an obvious folder exists. Suggest a new folder if the work is a new project — the user creates folders on the Studio page.\n")
	b.WriteString("5. **Write good prompts.** These models reward concrete detail: subject, style, lighting, composition, mood. Expand a terse request into a proper prompt rather than passing it through verbatim.\n\n")
	b.WriteString("Generated media is saved automatically and appears both in this chat and on the Studio page.\n")

	return b.String()
}

// buildStudioRoutingNote tells the gateway that media generation exists and is
// something an agent does, not something it can answer in prose. The gateway
// never carries the studio tools itself — it only decides who handles a message.
func buildStudioRoutingNote(registry *media.Registry) string {
	if registry == nil {
		return ""
	}

	var kinds []string
	for _, k := range []media.Kind{media.KindImage, media.KindVideo, media.KindAudio} {
		if registry.Supports(k) {
			kinds = append(kinds, string(k))
		}
	}
	if len(kinds) == 0 {
		return "\n\n## MEDIA GENERATION\nNo media provider is configured, so images, video and music cannot be generated. If the user asks for one, say so and point them at Settings to add an OpenRouter key (images) or a Replicate/fal key (video, music).\n"
	}

	return fmt.Sprintf(`

## MEDIA GENERATION (Studio)

OpenPaw can generate %s. Specialist agents hold the studio tools; you do not.

When the user asks to create, generate, make or design an image, video, song or
piece of audio, ASSIGN it to an agent — do not answer conversationally and do
not claim you have made something. Assigning is what causes it to actually be
generated and saved.

The same applies to questions about existing generated media ("what images do we
have", "show me the logos"), which agents answer with the studio tools.
`, strings.Join(kinds, ", "))
}

// MakeStudioToolHandlers builds the handlers, bound to the calling thread so
// generated assets are linked to the conversation that produced them.
func (m *Manager) MakeStudioToolHandlers(threadID string) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"studio_list_folders": m.handleStudioListFolders(),
		"studio_list_media":   m.handleStudioListMedia(),
		"studio_generate":     m.handleStudioGenerate(threadID),
	}
}

func (m *Manager) handleStudioListFolders() llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		workspaceID := m.db.ActiveWorkspaceID()
		rows, err := m.db.Query(
			`SELECT f.id, f.name, (SELECT COUNT(*) FROM media x WHERE x.folder_id = f.id)
			 FROM media_folders f WHERE f.workspace_id = ? ORDER BY f.name ASC`,
			workspaceID,
		)
		if err != nil {
			return llm.ToolResult{Output: "ERROR: could not list folders: " + err.Error(), IsError: true}
		}
		defer rows.Close()

		var b strings.Builder
		count := 0
		for rows.Next() {
			var id, name string
			var n int
			if err := rows.Scan(&id, &name, &n); err != nil {
				continue
			}
			fmt.Fprintf(&b, "- %s (id: %s) — %d item(s)\n", name, id, n)
			count++
		}

		var unfiled int
		m.db.QueryRow(
			"SELECT COUNT(*) FROM media WHERE (folder_id = '' OR folder_id IS NULL) AND (workspace_id = ? OR workspace_id = '')",
			workspaceID,
		).Scan(&unfiled)
		fmt.Fprintf(&b, "- Unfiled (id: unfiled) — %d item(s)\n", unfiled)

		if count == 0 {
			return llm.ToolResult{Output: "No folders yet in this workspace.\n" + b.String() + "\nThe user can create folders on the Studio page."}
		}
		return llm.ToolResult{Output: "Studio folders:\n" + b.String()}
	}
}

func (m *Manager) handleStudioListMedia() llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var req struct {
			FolderID string `json:"folder_id"`
			Type     string `json:"type"`
			Limit    int    `json:"limit"`
		}
		json.Unmarshal(input, &req)

		limit := req.Limit
		if limit < 1 || limit > 100 {
			limit = 25
		}

		where := []string{"(workspace_id = ? OR workspace_id = '')"}
		args := []interface{}{m.db.ActiveWorkspaceID()}

		if req.FolderID == "unfiled" {
			where = append(where, "(folder_id = '' OR folder_id IS NULL)")
		} else if req.FolderID != "" {
			where = append(where, "folder_id = ?")
			args = append(args, req.FolderID)
		}
		if req.Type != "" {
			kind, err := media.ParseKind(req.Type)
			if err != nil {
				return llm.ToolResult{Output: "ERROR: " + err.Error(), IsError: true}
			}
			where = append(where, "media_type = ?")
			args = append(args, string(kind))
		}
		args = append(args, limit)

		rows, err := m.db.Query(
			`SELECT id, media_type, source_model, prompt, created_at
			 FROM media WHERE `+strings.Join(where, " AND ")+`
			 ORDER BY created_at DESC LIMIT ?`,
			args...,
		)
		if err != nil {
			return llm.ToolResult{Output: "ERROR: could not list media: " + err.Error(), IsError: true}
		}
		defer rows.Close()

		var b strings.Builder
		n := 0
		for rows.Next() {
			var id, mediaType, model, prompt, createdAt string
			if err := rows.Scan(&id, &mediaType, &model, &prompt, &createdAt); err != nil {
				continue
			}
			fmt.Fprintf(&b, "- [%s] %s\n  url: /api/v1/media/%s/file\n  model: %s, created: %s\n",
				mediaType, truncateLine(prompt, 140), id, model, createdAt)
			n++
		}
		if n == 0 {
			return llm.ToolResult{Output: "No media found for that filter."}
		}
		return llm.ToolResult{Output: fmt.Sprintf("%d item(s):\n%s", n, b.String())}
	}
}

// maxAgentGenerations is deliberately lower than the Studio page's limit. The
// user clicking Generate has seen the count; an agent choosing it has not.
const maxAgentGenerations = 4

func (m *Manager) handleStudioGenerate(threadID string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		if m.MediaRegistry == nil {
			return llm.ToolResult{Output: "ERROR: media generation is not available in this build.", IsError: true}
		}

		var req struct {
			Type     string `json:"type"`
			Prompt   string `json:"prompt"`
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Count    int    `json:"count"`
			Size     string `json:"size"`
			Duration int    `json:"duration"`
			FolderID string `json:"folder_id"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return llm.ToolResult{Output: "ERROR: invalid input: " + err.Error(), IsError: true}
		}
		if strings.TrimSpace(req.Prompt) == "" {
			return llm.ToolResult{Output: "ERROR: prompt is required", IsError: true}
		}

		kind, err := media.ParseKind(req.Type)
		if err != nil {
			return llm.ToolResult{Output: "ERROR: " + err.Error(), IsError: true}
		}

		provider, err := m.MediaRegistry.Resolve(req.Provider, kind)
		if err != nil {
			return llm.ToolResult{Output: "ERROR: " + err.Error(), IsError: true}
		}

		count := req.Count
		if count < 1 {
			count = 1
		}
		if count > maxAgentGenerations {
			count = maxAgentGenerations
		}

		if req.FolderID == "unfiled" {
			req.FolderID = ""
		}
		if req.FolderID != "" {
			var found string
			if err := m.db.QueryRow("SELECT id FROM media_folders WHERE id = ?", req.FolderID).Scan(&found); err != nil {
				return llm.ToolResult{Output: "ERROR: folder not found. Call studio_list_folders for valid ids.", IsError: true}
			}
		}

		var out strings.Builder
		var firstURL string
		made := 0

		for i := 0; i < count; i++ {
			asset, genErr := provider.Generate(ctx, media.Request{
				Kind:     kind,
				Prompt:   req.Prompt,
				Model:    req.Model,
				Size:     req.Size,
				Duration: req.Duration,
			})
			if genErr != nil {
				fmt.Fprintf(&out, "Generation %d failed: %s\n", i+1, genErr.Error())
				if i == 0 {
					return llm.ToolResult{Output: "ERROR: " + genErr.Error(), IsError: true}
				}
				continue
			}

			rec, saveErr := media.Save(m.db, m.DataDir, asset, media.SaveMeta{
				Provider:    provider.Name(),
				Model:       req.Model,
				Prompt:      req.Prompt,
				Kind:        kind,
				WorkspaceID: m.db.ActiveWorkspaceID(),
				FolderID:    req.FolderID,
				ThreadID:    threadID,
				Source:      "studio",
			})
			if saveErr != nil {
				fmt.Fprintf(&out, "Generation %d could not be saved: %s\n", i+1, saveErr.Error())
				continue
			}

			made++
			if firstURL == "" {
				firstURL = rec.LocalURL
			}
			// Images use markdown image syntax. Video and audio are links with
			// a ?kind= hint, which the chat renderer turns into a real player —
			// the media URL carries no file extension for it to sniff.
			if kind == media.KindImage {
				fmt.Fprintf(&out, "![%s](%s)\n", truncateLine(req.Prompt, 80), rec.LocalURL)
			} else {
				fmt.Fprintf(&out, "[%s %d](%s?kind=%s)\n", kind, i+1, rec.LocalURL, kind)
			}
		}

		if made == 0 {
			return llm.ToolResult{Output: "ERROR: nothing was generated.\n" + out.String(), IsError: true}
		}

		result := fmt.Sprintf("Generated %d %s(s) with %s and saved them to the Studio library.\n\n%s",
			made, kind, provider.Name(), out.String())
		if kind == media.KindImage {
			return llm.ToolResult{Output: result, ImageURL: firstURL}
		}
		return llm.ToolResult{Output: result}
	}
}

func truncateLine(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
