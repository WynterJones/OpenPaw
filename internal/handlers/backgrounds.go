package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/media"
	"github.com/openpaw/openpaw/internal/middleware"
)

// BackgroundsHandler generates UI background images in the app's own visual
// style and keeps them next to the shipped presets in Settings.
//
// It does not talk to OpenRouter directly: generation goes through the same
// media.Registry Studio uses, so a background is produced by exactly the code
// path (and retry/parse behaviour) that already works for Studio images.
type BackgroundsHandler struct {
	db         *database.DB
	dataDir    string
	registry   *media.Registry
	frontendFS fs.FS
}

func NewBackgroundsHandler(db *database.DB, dataDir string, registry *media.Registry, frontendFS fs.FS) *BackgroundsHandler {
	return &BackgroundsHandler{db: db, dataDir: dataDir, registry: registry, frontendFS: frontendFS}
}

// mascotAsset is the OpenPaw cat, attached to every generation so the produced
// background belongs to the same world as the presets.
const mascotAsset = "/cat-toolbar.webp"

// defaultStyleRef is the preset used as the style reference when the caller
// doesn't pick one. Any of the nine would do; the first is the most
// representative of the set.
const defaultStyleRef = "/preset-bg/bg-1.webp"

// backgroundSize is a 3:2 landscape hint. The image models treat size as a
// suggestion rather than a constraint, so this is about steering them away from
// a square, not about getting exact pixels.
const backgroundSize = "1536x1024"

// maxBackgroundPrompt bounds the free-text field. Long prompts don't fail, they
// just dilute the style instructions.
const maxBackgroundPrompt = 1500

func (h *BackgroundsHandler) dir() string {
	return filepath.Join(h.dataDir, "generated-backgrounds")
}

type backgroundRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Prompt    string `json:"prompt"`
	AgentSlug string `json:"agent_slug"`
	StyleRef  string `json:"style_ref"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	MimeType  string `json:"mime_type"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SizeBytes int    `json:"size_bytes"`
	CreatedAt string `json:"created_at"`
	URL       string `json:"url"`
}

// List returns every generated background, newest first. Backgrounds are app
// chrome rather than workspace content, so they are deliberately not scoped to
// the active workspace.
func (h *BackgroundsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		`SELECT id, name, prompt, agent_slug, style_ref, provider, model, mime_type,
		        width, height, size_bytes, created_at
		 FROM generated_backgrounds ORDER BY created_at DESC LIMIT 200`,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list backgrounds")
		return
	}
	defer rows.Close()

	items := []backgroundRow{}
	for rows.Next() {
		var b backgroundRow
		if err := rows.Scan(&b.ID, &b.Name, &b.Prompt, &b.AgentSlug, &b.StyleRef, &b.Provider,
			&b.Model, &b.MimeType, &b.Width, &b.Height, &b.SizeBytes, &b.CreatedAt); err != nil {
			continue
		}
		b.URL = backgroundURL(b.ID)
		items = append(items, b)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"backgrounds": items})
}

func backgroundURL(id string) string {
	return "/api/v1/backgrounds/" + id + "/file"
}

type generateBackgroundRequest struct {
	Prompt string `json:"prompt"`
	// AgentSlug supplies the third reference image — that agent's avatar, so the
	// character on the background is the one the user actually works with.
	AgentSlug string `json:"agent_slug"`
	// StyleRef is the background whose art style should be matched. A preset
	// URL, an uploaded background, or a previously generated one.
	StyleRef string `json:"style_ref"`
	Name     string `json:"name"`
	// Model overrides the automatic Gemini pick. Empty is the normal case.
	Model string `json:"model"`
}

// Generate composes the mascot, a style reference, an agent avatar and the
// user's prompt into one image request.
func (h *BackgroundsHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req generateBackgroundRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeError(w, http.StatusBadRequest, "a prompt is required")
		return
	}
	if len([]rune(prompt)) > maxBackgroundPrompt {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("prompt must be under %d characters", maxBackgroundPrompt))
		return
	}

	provider, err := h.registry.Resolve("openrouter", media.KindImage)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := contextWithTimeout(r, 10*time.Minute)
	defer cancel()

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model, err = h.pickGeminiModel(ctx, provider)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	}

	styleRef := strings.TrimSpace(req.StyleRef)
	if styleRef == "" {
		styleRef = defaultStyleRef
	}

	refs, notes, err := h.composeReferences(styleRef, req.AgentSlug)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	asset, err := provider.Generate(ctx, media.Request{
		Kind:      media.KindImage,
		Prompt:    buildBackgroundPrompt(prompt, notes),
		Model:     model,
		Size:      backgroundSize,
		RefImages: refs,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = truncateRunes(prompt, 60)
	}

	rec, err := h.save(asset, name, prompt, req.AgentSlug, styleRef, provider.Name(), model)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "background_generated", "settings", "generated_background", rec.ID, model)

	writeJSON(w, http.StatusOK, rec)
}

// referenceNote pairs an attached image with the sentence that tells the model
// what that image is for. Kept together so the numbering in the prompt can
// never drift from the order the images are actually sent in.
type referenceNote struct {
	ref  string
	note string
}

// composeReferences assembles the reference images in a fixed order: mascot,
// style reference, agent avatar. Only the style reference is required — a
// missing mascot or avatar drops that reference and its prompt line rather than
// failing the whole generation, so a partial install still produces something.
func (h *BackgroundsHandler) composeReferences(styleRef, agentSlug string) ([]string, []string, error) {
	pairs := []referenceNote{}

	if mascot, err := llm.ResolveImageToBase64(h.dataDir, mascotAsset, h.frontendFS); err == nil {
		pairs = append(pairs, referenceNote{
			ref:  mascot,
			note: "the OpenPaw mascot — a cat character. Put this cat in the scene, faithful to its design, colours and proportions",
		})
	} else {
		log.Printf("background generate: mascot reference unavailable: %v", err)
	}

	style, err := h.resolveBackgroundRef(styleRef)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read the style reference: %w", err)
	}
	pairs = append(pairs, referenceNote{
		ref:  style,
		note: "an existing OpenPaw background. Match its art style, colour palette, lighting, texture and level of detail as closely as you can — this is the look to copy",
	})

	if slug := strings.TrimSpace(agentSlug); slug != "" {
		avatarPath, agentName := h.agentAvatar(slug)
		if avatarPath == "" {
			return nil, nil, fmt.Errorf("agent %q has no avatar to use", slug)
		}
		avatar, err := llm.ResolveImageToBase64(h.dataDir, avatarPath, h.frontendFS)
		if err != nil {
			return nil, nil, fmt.Errorf("could not read the avatar for agent %q: %w", slug, err)
		}
		pairs = append(pairs, referenceNote{
			ref:  avatar,
			note: fmt.Sprintf("the avatar of the agent %q. Include this character in the scene alongside the mascot, keeping it recognisable", agentName),
		})
	}

	refs := make([]string, 0, len(pairs))
	notes := make([]string, 0, len(pairs))
	for i, p := range pairs {
		refs = append(refs, p.ref)
		notes = append(notes, fmt.Sprintf("Reference image %d is %s.", i+1, p.note))
	}
	return refs, notes, nil
}

func (h *BackgroundsHandler) agentAvatar(slug string) (avatarPath, name string) {
	h.db.QueryRow("SELECT COALESCE(avatar_path,''), COALESCE(name,'') FROM agent_roles WHERE slug = ?", slug).
		Scan(&avatarPath, &name)
	if name == "" {
		name = slug
	}
	return avatarPath, name
}

// buildBackgroundPrompt wraps the user's idea in the constraints that make an
// image usable as app chrome: landscape, uncluttered in the middle, no text.
func buildBackgroundPrompt(userPrompt string, notes []string) string {
	var b strings.Builder
	b.WriteString("Create a single wide desktop wallpaper for an app's user interface.\n\n")
	for _, n := range notes {
		b.WriteString(n)
		b.WriteString("\n")
	}
	b.WriteString("\nScene the user asked for: ")
	b.WriteString(userPrompt)
	b.WriteString("\n\nRules: 3:2 landscape orientation. Illustrated, not photographic. " +
		"Keep the centre of the frame calm and low-contrast so UI panels sitting on top stay readable, " +
		"and push the detail toward the edges and lower third. " +
		"No text, lettering, logos, watermarks, signatures, borders, UI elements or window frames. " +
		"Return only the image.")
	return b.String()
}

// pickGeminiModel finds a Gemini image model in OpenRouter's live catalog
// rather than hardcoding an id that may have been retired. Models() already
// ranks known-good generators first, so the first Gemini match is the best one;
// models that accept image input are preferred because this request always
// carries references.
func (h *BackgroundsHandler) pickGeminiModel(ctx context.Context, provider media.Provider) (string, error) {
	models, err := provider.Models(ctx, media.KindImage)
	if err != nil {
		return "", fmt.Errorf("could not read OpenRouter's model catalog: %w", err)
	}

	fallback := ""
	for _, m := range models {
		if !strings.Contains(strings.ToLower(m.ID), "gemini") {
			continue
		}
		if m.MaxRefImages > 0 {
			return m.ID, nil
		}
		if fallback == "" {
			fallback = m.ID
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("no Gemini image model is available on your OpenRouter account right now — pick a model explicitly in Studio, or try again later")
}

func (h *BackgroundsHandler) save(asset *media.Asset, name, prompt, agentSlug, styleRef, provider, model string) (*backgroundRow, error) {
	if asset == nil || len(asset.Data) == 0 {
		return nil, fmt.Errorf("the model returned an empty image")
	}
	if err := os.MkdirAll(h.dir(), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create the backgrounds directory")
	}

	id := uuid.New().String()
	ext := asset.Ext
	if ext == "" {
		ext = ".png"
	}
	filename := id + ext

	if err := os.WriteFile(filepath.Join(h.dir(), filename), asset.Data, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write the background file")
	}

	mimeType := asset.MimeType
	if mimeType == "" {
		mimeType = llm.MimeForPath(filename)
	}
	now := time.Now().UTC()

	if _, err := h.db.Exec(
		`INSERT INTO generated_backgrounds (id, name, prompt, agent_slug, style_ref, provider, model,
		                                    filename, mime_type, width, height, size_bytes, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, prompt, agentSlug, styleRef, provider, model, filename, mimeType,
		asset.Width, asset.Height, len(asset.Data), now,
	); err != nil {
		// The row is what makes the file reachable — don't leak an orphan.
		os.Remove(filepath.Join(h.dir(), filename))
		return nil, fmt.Errorf("failed to record the background")
	}

	return &backgroundRow{
		ID:        id,
		Name:      name,
		Prompt:    prompt,
		AgentSlug: agentSlug,
		StyleRef:  styleRef,
		Provider:  provider,
		Model:     model,
		MimeType:  mimeType,
		Width:     asset.Width,
		Height:    asset.Height,
		SizeBytes: len(asset.Data),
		CreatedAt: now.Format(time.RFC3339),
		URL:       backgroundURL(id),
	}, nil
}

// Delete removes a generated background and its file. If it was the background
// currently in use, the design config is cleared too — otherwise the whole UI
// would be left pointing at a 404.
func (h *BackgroundsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var filename string
	if err := h.db.QueryRow("SELECT filename FROM generated_backgrounds WHERE id = ?", id).Scan(&filename); err != nil {
		writeError(w, http.StatusNotFound, "background not found")
		return
	}

	if _, err := h.db.Exec("DELETE FROM generated_backgrounds WHERE id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete background")
		return
	}
	if safe := safeFilename(filename); safe != "" {
		os.Remove(filepath.Join(h.dir(), safe))
	}
	h.clearActiveBackground(backgroundURL(id))

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "background_deleted", "settings", "generated_background", id, "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// clearActiveBackground blanks design_config.bg_image when it points at url.
func (h *BackgroundsHandler) clearActiveBackground(url string) {
	var value string
	if err := h.db.QueryRow("SELECT value FROM settings WHERE key = 'design_config'").Scan(&value); err != nil || value == "" {
		return
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(value), &cfg); err != nil {
		return
	}
	if current, _ := cfg["bg_image"].(string); current != url {
		return
	}
	cfg["bg_image"] = ""
	updated, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	// The row exists — it was just read above — so an update is enough.
	h.db.Exec("UPDATE settings SET value = ? WHERE key = 'design_config'", string(updated))
}

// ServeFile is public for the same reason avatars and media are: the URL ends
// up in a CSS background-image, which carries no auth header.
func (h *BackgroundsHandler) ServeFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var filename, mimeType string
	if err := h.db.QueryRow("SELECT filename, mime_type FROM generated_backgrounds WHERE id = ?", id).
		Scan(&filename, &mimeType); err != nil {
		http.NotFound(w, r)
		return
	}
	safe := safeFilename(filename)
	if safe == "" {
		http.NotFound(w, r)
		return
	}

	if mimeType == "" {
		mimeType = llm.MimeForPath(safe)
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Files are immutable once written — the id changes when the image does.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, filepath.Join(h.dir(), safe))
}

func safeFilename(name string) string {
	base := filepath.Base(name)
	if base == "." || base == string(filepath.Separator) || strings.Contains(name, "..") {
		return ""
	}
	return base
}

// resolveBackgroundRef inlines whatever the user picked as a style reference.
// Generated and uploaded backgrounds live outside the media library, so they
// are read here; everything else (presets, avatars, media, data URIs) goes
// through the shared resolver.
func (h *BackgroundsHandler) resolveBackgroundRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	switch {
	case strings.HasPrefix(ref, "/api/v1/backgrounds/"):
		id := strings.TrimPrefix(ref, "/api/v1/backgrounds/")
		id = strings.TrimSuffix(id, "/file")
		var filename string
		if err := h.db.QueryRow("SELECT filename FROM generated_backgrounds WHERE id = ?", id).Scan(&filename); err != nil {
			return "", fmt.Errorf("generated background %s not found", id)
		}
		return fileToDataURI(filepath.Join(h.dir(), safeFilename(filename)))

	case strings.HasPrefix(ref, "/api/v1/uploads/backgrounds/"):
		filename := safeFilename(strings.TrimPrefix(ref, "/api/v1/uploads/backgrounds/"))
		if filename == "" {
			return "", fmt.Errorf("invalid background reference")
		}
		return fileToDataURI(filepath.Join(h.dataDir, "backgrounds", filename))
	}

	return llm.ResolveImageToBase64(h.dataDir, ref, h.frontendFS)
}

func fileToDataURI(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read image file: %w", err)
	}
	return fmt.Sprintf("data:%s;base64,%s", llm.MimeForPath(path), base64.StdEncoding.EncodeToString(data)), nil
}
