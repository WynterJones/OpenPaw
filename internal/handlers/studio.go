package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/media"
	"github.com/openpaw/openpaw/internal/middleware"
)

// maxGenerationsPerRequest bounds one Generate call. Each asset is a paid API
// hit, so a slipped zero shouldn't be able to spend real money in a loop.
const maxGenerationsPerRequest = 8

type StudioHandler struct {
	db       *database.DB
	dataDir  string
	registry *media.Registry
}

func NewStudioHandler(db *database.DB, dataDir string, registry *media.Registry) *StudioHandler {
	return &StudioHandler{db: db, dataDir: dataDir, registry: registry}
}

// Providers lists every provider with what it can make and whether it has a
// key. Unconfigured providers are still returned so the UI can show them
// disabled with a reason rather than pretending they don't exist.
func (h *StudioHandler) Providers(w http.ResponseWriter, r *http.Request) {
	out := []map[string]interface{}{}
	for _, p := range h.registry.All() {
		kinds := make([]string, 0, len(p.Kinds()))
		for _, k := range p.Kinds() {
			kinds = append(kinds, string(k))
		}
		out = append(out, map[string]interface{}{
			"name":       p.Name(),
			"configured": p.Configured(),
			"kinds":      kinds,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"providers": out,
		"supports": map[string]bool{
			"image": h.registry.Supports(media.KindImage),
			"video": h.registry.Supports(media.KindVideo),
			"audio": h.registry.Supports(media.KindAudio),
		},
	})
}

// Models returns the model picker's options for one media type, across every
// configured provider (or just one, with ?provider=).
func (h *StudioHandler) Models(w http.ResponseWriter, r *http.Request) {
	kind, err := media.ParseKind(r.URL.Query().Get("type"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()

	if name := r.URL.Query().Get("provider"); name != "" {
		p := h.registry.Get(name)
		if p == nil {
			writeError(w, http.StatusBadRequest, "unknown provider "+name)
			return
		}
		models, err := p.Models(ctx, kind)
		if err != nil {
			writeError(w, http.StatusBadGateway, "failed to list models: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"models": models})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"models": h.registry.ModelsFor(ctx, kind)})
}

type generateRequest struct {
	Provider  string                 `json:"provider"`
	Type      string                 `json:"type"`
	Model     string                 `json:"model"`
	Prompt    string                 `json:"prompt"`
	Count     int                    `json:"count"`
	Size      string                 `json:"size"`
	Duration  int                    `json:"duration"`
	FolderID  string                 `json:"folder_id"`
	ThreadID  string                 `json:"thread_id"`
	RefImages []string               `json:"ref_images"`
	Params    map[string]interface{} `json:"params"`
}

// Generate runs a batch and returns whatever succeeded. Partial success is
// reported rather than discarded: when three of four images come back, losing
// them because the fourth failed would mean paying for them twice.
func (h *StudioHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "a prompt is required")
		return
	}

	kind, err := media.ParseKind(req.Type)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	provider, err := h.registry.Resolve(req.Provider, kind)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	count := req.Count
	if count < 1 {
		count = 1
	}
	if count > maxGenerationsPerRequest {
		count = maxGenerationsPerRequest
	}

	if req.FolderID != "" && !h.folderExists(req.FolderID) {
		writeError(w, http.StatusBadRequest, "folder not found")
		return
	}

	// Generation is slow by nature (video runs for minutes), so the ceiling is
	// the whole batch rather than the default server timeout.
	ctx, cancel := contextWithTimeout(r, 15*time.Minute)
	defer cancel()

	workspaceID := activeWorkspaceID(h.db)

	items := []*media.Record{}
	errs := []string{}

	for i := 0; i < count; i++ {
		asset, genErr := provider.Generate(ctx, media.Request{
			Kind:      kind,
			Prompt:    req.Prompt,
			Model:     req.Model,
			Size:      req.Size,
			Duration:  req.Duration,
			RefImages: req.RefImages,
			Params:    req.Params,
		})
		if genErr != nil {
			errs = append(errs, genErr.Error())
			// A failed first attempt usually means bad credentials or a bad
			// model id, which will fail identically N times — stop instead of
			// hammering the provider.
			if i == 0 {
				break
			}
			continue
		}

		rec, saveErr := media.Save(h.db, h.dataDir, asset, media.SaveMeta{
			Provider:    provider.Name(),
			Model:       req.Model,
			Prompt:      req.Prompt,
			Kind:        kind,
			WorkspaceID: workspaceID,
			FolderID:    req.FolderID,
			ThreadID:    req.ThreadID,
			Source:      "studio",
		})
		if saveErr != nil {
			errs = append(errs, saveErr.Error())
			continue
		}
		items = append(items, rec)
	}

	if len(items) == 0 {
		msg := "generation failed"
		if len(errs) > 0 {
			msg = errs[0]
		}
		writeError(w, http.StatusBadGateway, msg)
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "studio_generated", "studio", "media", "", fmt.Sprintf("%s x%d via %s", kind, len(items), provider.Name()))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"errors": errs,
	})
}

// --- Folders ---

func (h *StudioHandler) ListFolders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"folders": h.folders()})
}

type folderRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Count     int    `json:"count"`
	CreatedAt string `json:"created_at"`
}

func (h *StudioHandler) folders() []folderRow {
	workspaceID := activeWorkspaceID(h.db)
	rows, err := h.db.Query(
		`SELECT f.id, f.name, f.created_at, (SELECT COUNT(*) FROM media m WHERE m.folder_id = f.id)
		 FROM media_folders f WHERE f.workspace_id = ? ORDER BY f.name ASC`,
		workspaceID,
	)
	if err != nil {
		return []folderRow{}
	}
	defer rows.Close()

	out := []folderRow{}
	for rows.Next() {
		var f folderRow
		if err := rows.Scan(&f.ID, &f.Name, &f.CreatedAt, &f.Count); err != nil {
			continue
		}
		out = append(out, f)
	}
	return out
}

func (h *StudioHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "a folder name is required")
		return
	}
	if len(name) > 120 {
		name = name[:120]
	}

	id := uuid.New().String()
	if _, err := h.db.Exec(
		"INSERT INTO media_folders (id, workspace_id, name, created_at) VALUES (?, ?, ?, ?)",
		id, activeWorkspaceID(h.db), name, time.Now().UTC(),
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create folder")
		return
	}

	writeJSON(w, http.StatusOK, folderRow{ID: id, Name: name})
}

func (h *StudioHandler) RenameFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "a folder name is required")
		return
	}

	res, err := h.db.Exec("UPDATE media_folders SET name = ? WHERE id = ?", name, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rename folder")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "folder not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteFolder removes the folder but keeps its media, which becomes unfiled.
// Deleting generated assets as a side effect of tidying folders would be a
// nasty surprise; emptying is an explicit action on the items themselves.
func (h *StudioHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	res, err := h.db.Exec("DELETE FROM media_folders WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete folder")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "folder not found")
		return
	}
	h.db.Exec("UPDATE media SET folder_id = '' WHERE folder_id = ?", id)

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "studio_folder_deleted", "studio", "media_folder", id, "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *StudioHandler) folderExists(id string) bool {
	var found string
	err := h.db.QueryRow("SELECT id FROM media_folders WHERE id = ?", id).Scan(&found)
	return err == nil
}

// MoveMedia files an asset into a folder, or unfiles it when folder_id is "".
func (h *StudioHandler) MoveMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		FolderID string `json:"folder_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.FolderID != "" && !h.folderExists(req.FolderID) {
		writeError(w, http.StatusBadRequest, "folder not found")
		return
	}

	res, err := h.db.Exec("UPDATE media SET folder_id = ? WHERE id = ?", req.FolderID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to move media")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListMedia is Studio's gallery query: workspace-scoped, filterable by folder
// and media type.
func (h *StudioHandler) ListMedia(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 60
	}

	where := []string{"(workspace_id = ? OR workspace_id = '')"}
	args := []interface{}{activeWorkspaceID(h.db)}

	// "unfiled" is a real filter, distinct from "no folder filter at all".
	if folder := q.Get("folder_id"); folder != "" {
		if folder == "unfiled" {
			where = append(where, "(folder_id = '' OR folder_id IS NULL)")
		} else {
			where = append(where, "folder_id = ?")
			args = append(args, folder)
		}
	}
	if t := q.Get("type"); t != "" {
		kind, err := media.ParseKind(t)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		where = append(where, "media_type = ?")
		args = append(args, string(kind))
	}

	args = append(args, limit)
	rows, err := h.db.Query(
		`SELECT id, media_type, COALESCE(provider,''), source_model, prompt, filename, mime_type,
		        width, height, COALESCE(duration_ms,0), size_bytes, COALESCE(folder_id,''),
		        COALESCE(workspace_id,''), thread_id, created_at
		 FROM media WHERE `+strings.Join(where, " AND ")+`
		 ORDER BY created_at DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list media")
		return
	}
	defer rows.Close()

	items := []media.Record{}
	for rows.Next() {
		var m media.Record
		if err := rows.Scan(&m.ID, &m.MediaType, &m.Provider, &m.SourceModel, &m.Prompt, &m.Filename,
			&m.MimeType, &m.Width, &m.Height, &m.DurationMS, &m.SizeBytes, &m.FolderID,
			&m.WorkspaceID, &m.ThreadID, &m.CreatedAt); err != nil {
			continue
		}
		m.LocalURL = fmt.Sprintf("/api/v1/media/%s/file", m.ID)
		items = append(items, m)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

// --- Saved presets ---

type presetRow struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Provider    string          `json:"provider"`
	MediaType   string          `json:"media_type"`
	Model       string          `json:"model"`
	Prompt      string          `json:"prompt"`
	Count       int             `json:"count"`
	Size        string          `json:"size"`
	FolderID    string          `json:"folder_id"`
	Params      json.RawMessage `json:"params"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

func (h *StudioHandler) ListPresets(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		`SELECT id, name, provider, media_type, model, prompt, count, size, folder_id, params, created_at, updated_at
		 FROM studio_presets WHERE workspace_id = ? ORDER BY updated_at DESC LIMIT 200`,
		activeWorkspaceID(h.db),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list presets")
		return
	}
	defer rows.Close()

	items := []presetRow{}
	for rows.Next() {
		var p presetRow
		var params string
		if err := rows.Scan(&p.ID, &p.Name, &p.Provider, &p.MediaType, &p.Model, &p.Prompt,
			&p.Count, &p.Size, &p.FolderID, &params, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		if params == "" {
			params = "{}"
		}
		p.Params = json.RawMessage(params)
		items = append(items, p)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"presets": items})
}

func (h *StudioHandler) SavePreset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Provider  string          `json:"provider"`
		MediaType string          `json:"media_type"`
		Model     string          `json:"model"`
		Prompt    string          `json:"prompt"`
		Count     int             `json:"count"`
		Size      string          `json:"size"`
		FolderID  string          `json:"folder_id"`
		Params    json.RawMessage `json:"params"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		// Falling back to the prompt means the save button never blocks on a
		// naming dialog for the common case.
		name = truncateRunes(strings.TrimSpace(req.Prompt), 60)
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "a name or prompt is required")
		return
	}

	params := "{}"
	if len(req.Params) > 0 {
		params = string(req.Params)
	}
	now := time.Now().UTC()

	if req.ID != "" {
		res, err := h.db.Exec(
			`UPDATE studio_presets SET name = ?, provider = ?, media_type = ?, model = ?, prompt = ?,
			        count = ?, size = ?, folder_id = ?, params = ?, updated_at = ?
			 WHERE id = ?`,
			name, req.Provider, req.MediaType, req.Model, req.Prompt, req.Count, req.Size,
			req.FolderID, params, now, req.ID,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update preset")
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			writeError(w, http.StatusNotFound, "preset not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": req.ID, "name": name})
		return
	}

	id := uuid.New().String()
	if _, err := h.db.Exec(
		`INSERT INTO studio_presets (id, workspace_id, name, provider, media_type, model, prompt,
		                             count, size, folder_id, params, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, activeWorkspaceID(h.db), name, req.Provider, req.MediaType, req.Model, req.Prompt,
		req.Count, req.Size, req.FolderID, params, now, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save preset")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": id, "name": name})
}

func (h *StudioHandler) DeletePreset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := h.db.Exec("DELETE FROM studio_presets WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete preset")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "preset not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// contextWithTimeout bounds a slow provider call while still inheriting the
// request context, so a user navigating away also cancels the generation.
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
