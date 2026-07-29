package handlers

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/openpaw/openpaw/internal/llm"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
	"github.com/openpaw/openpaw/internal/platform"
)

// DefaultWorkspaceID is the fixed, well-known uuid of the seeded "Default"
// workspace (see migration 049). Scoped rows fall back to this workspace and it
// can never be deleted.
const DefaultWorkspaceID = database.DefaultWorkspaceID

// activeWorkspaceID returns the currently active workspace id from settings,
// falling back to the Default workspace when unset.
func activeWorkspaceID(db *database.DB) string {
	return db.ActiveWorkspaceID()
}

type WorkspacesHandler struct {
	db      *database.DB
	dataDir string
	llm     *llm.Client
}

func NewWorkspacesHandler(db *database.DB, dataDir string, llmClient *llm.Client) *WorkspacesHandler {
	return &WorkspacesHandler{db: db, dataDir: dataDir, llm: llmClient}
}

// workspacesRoot is the on-disk parent for every workspace's real files dir.
func (h *WorkspacesHandler) workspacesRoot() string {
	return filepath.Join(h.dataDir, "workspaces")
}

// filesDir returns data/workspaces/<id>/files for a validated workspace id.
func (h *WorkspacesHandler) filesDir(id string) string {
	return filepath.Join(h.workspacesRoot(), id, "files")
}

// EnsureDefaultWorkspaceDir creates the Default workspace's files directory on
// startup so the on-disk layout always exists.
func EnsureDefaultWorkspaceDir(dataDir string) {
	dir := filepath.Join(dataDir, "workspaces", DefaultWorkspaceID, "files")
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Warn("failed to create default workspace files dir: %v", err)
	}
}

func (h *WorkspacesHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		"SELECT id, name, image_url, sort_order, is_default, created_at, updated_at FROM workspaces ORDER BY sort_order ASC, created_at ASC",
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspaces")
		return
	}
	defer rows.Close()

	workspaces := []models.Workspace{}
	for rows.Next() {
		var ws models.Workspace
		if err := rows.Scan(&ws.ID, &ws.Name, &ws.ImageURL, &ws.SortOrder, &ws.IsDefault, &ws.CreatedAt, &ws.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan workspace")
			return
		}
		workspaces = append(workspaces, ws)
	}
	writeJSON(w, http.StatusOK, workspaces)
}

func (h *WorkspacesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	var maxOrder int
	h.db.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM workspaces").Scan(&maxOrder)

	_, err := h.db.Exec(
		"INSERT INTO workspaces (id, name, sort_order, is_default, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)",
		id, req.Name, maxOrder+1, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workspace")
		return
	}

	// Create the real on-disk files directory for this workspace.
	if err := os.MkdirAll(h.filesDir(id), 0755); err != nil {
		logger.Warn("failed to create workspace files dir for %s: %v", id, err)
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "workspace_created", "workspace", "workspace", id, req.Name)

	writeJSON(w, http.StatusCreated, models.Workspace{
		ID:        id,
		Name:      req.Name,
		SortOrder: maxOrder + 1,
		IsDefault: false,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (h *WorkspacesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Name      *string `json:"name,omitempty"`
		ImageURL  *string `json:"image_url,omitempty"`
		SortOrder *int    `json:"sort_order,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()
	if req.Name != nil {
		if *req.Name == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		h.db.Exec("UPDATE workspaces SET name = ?, updated_at = ? WHERE id = ?", *req.Name, now, id)
	}
	if req.ImageURL != nil {
		h.db.Exec("UPDATE workspaces SET image_url = ?, updated_at = ? WHERE id = ?", *req.ImageURL, now, id)
	}
	if req.SortOrder != nil {
		h.db.Exec("UPDATE workspaces SET sort_order = ?, updated_at = ? WHERE id = ?", *req.SortOrder, now, id)
	}

	var ws models.Workspace
	err := h.db.QueryRow(
		"SELECT id, name, image_url, sort_order, is_default, created_at, updated_at FROM workspaces WHERE id = ?", id,
	).Scan(&ws.ID, &ws.Name, &ws.ImageURL, &ws.SortOrder, &ws.IsDefault, &ws.CreatedAt, &ws.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "workspace_updated", "workspace", "workspace", id, ws.Name)

	writeJSON(w, http.StatusOK, ws)
}

func (h *WorkspacesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if id == DefaultWorkspaceID {
		writeError(w, http.StatusBadRequest, "the default workspace cannot be deleted")
		return
	}

	var isDefault int
	err := h.db.QueryRow("SELECT is_default FROM workspaces WHERE id = ?", id).Scan(&isDefault)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if isDefault == 1 {
		writeError(w, http.StatusBadRequest, "the default workspace cannot be deleted")
		return
	}

	// Database names are unique inside a workspace. Preserve databases when a
	// workspace is removed, but disambiguate any names that would collide after
	// moving them into Default.
	collisions, err := h.db.Query(`
		SELECT source.id, source.name
		  FROM user_databases source
		  JOIN user_databases target
		    ON target.workspace_id = ? AND target.name = source.name COLLATE NOCASE
		 WHERE source.workspace_id = ?`, DefaultWorkspaceID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prepare workspace databases")
		return
	}
	type databaseRename struct{ id, name string }
	var renames []databaseRename
	for collisions.Next() {
		var rename databaseRename
		if err := collisions.Scan(&rename.id, &rename.name); err != nil {
			collisions.Close()
			writeError(w, http.StatusInternalServerError, "failed to prepare workspace databases")
			return
		}
		renames = append(renames, rename)
	}
	collisions.Close()
	for _, rename := range renames {
		candidate := fmt.Sprintf("%s (%s)", rename.name, rename.id)
		for copyNumber := 2; ; copyNumber++ {
			var count int
			if err := h.db.QueryRow(
				`SELECT COUNT(*) FROM user_databases
				  WHERE workspace_id IN (?, ?) AND id <> ? AND name = ? COLLATE NOCASE`,
				DefaultWorkspaceID, id, rename.id, candidate,
			).Scan(&count); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to prepare workspace databases")
				return
			}
			if count == 0 {
				break
			}
			candidate = fmt.Sprintf("%s (%s, copy %d)", rename.name, rename.id, copyNumber)
		}
		if _, err := h.db.Exec("UPDATE user_databases SET name = ? WHERE id = ? AND workspace_id = ?", candidate, rename.id, id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to prepare workspace databases")
			return
		}
	}

	// Reassign scoped rows to the Default workspace to avoid data loss.
	for _, tbl := range []string{"chat_threads", "dashboards", "context_files", "context_folders", "todo_lists"} {
		h.db.Exec("UPDATE "+tbl+" SET workspace_id = ? WHERE workspace_id = ?", DefaultWorkspaceID, id)
	}
	if _, err := h.db.Exec("UPDATE user_databases SET workspace_id = ? WHERE workspace_id = ?", DefaultWorkspaceID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to preserve workspace databases")
		return
	}
	// Schedules / heartbeat use nullable workspace_id (= global) — null them out.
	h.db.Exec("UPDATE schedules SET workspace_id = NULL WHERE workspace_id = ?", id)

	if _, err := h.db.Exec("DELETE FROM workspaces WHERE id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workspace")
		return
	}

	// If this was the active workspace, fall back to Default.
	if activeWorkspaceID(h.db) == id {
		h.setActiveWorkspace(DefaultWorkspaceID)
	}

	// Remove its real files directory.
	if err := os.RemoveAll(filepath.Join(h.workspacesRoot(), id)); err != nil {
		logger.Warn("failed to remove workspace files dir for %s: %v", id, err)
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "workspace_deleted", "workspace", "workspace", id, "")

	w.WriteHeader(http.StatusNoContent)
}

// UploadImage accepts a multipart image and stores it alongside avatars,
// returning a served URL the caller can persist as a workspace's image_url.
func (h *WorkspacesHandler) UploadImage(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(5 << 20) // 5MB max

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "image file required")
		return
	}
	defer file.Close()

	ct := header.Header.Get("Content-Type")
	var ext string
	switch {
	case strings.HasPrefix(ct, "image/png"):
		ext = ".png"
	case strings.HasPrefix(ct, "image/jpeg"):
		ext = ".jpg"
	case strings.HasPrefix(ct, "image/webp"):
		ext = ".webp"
	default:
		writeError(w, http.StatusBadRequest, "image must be PNG, JPEG, or WebP")
		return
	}

	if !validateImageMagicBytes(file, ext) {
		writeError(w, http.StatusBadRequest, "file content does not match declared type")
		return
	}

	// Reuse the avatars uploads dir so the existing /uploads/avatars serve route
	// handles it too — no new static route needed.
	uploadsDir := filepath.Join(h.dataDir, "avatars")
	os.MkdirAll(uploadsDir, 0755)

	filename := fmt.Sprintf("ws-%s%s", uuid.New().String(), ext)
	destPath := filepath.Join(uploadsDir, filename)

	dest, err := os.Create(destPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save image")
		return
	}
	defer dest.Close()

	if _, err := io.Copy(dest, file); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write image")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"image_url": "/api/v1/uploads/avatars/" + filename})
}

// GenerateImage creates a workspace image from a text prompt via OpenRouter
// (when configured), saves it alongside avatars, sets it as the workspace's
// image_url, and returns the URL. Requires an OpenRouter API key — CLI-only
// setups get a clear error.
func (h *WorkspacesHandler) GenerateImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid workspace id")
		return
	}
	var exists int
	if err := h.db.QueryRow("SELECT 1 FROM workspaces WHERE id = ?", id).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if h.llm == nil || !h.llm.IsConfigured() {
		writeError(w, http.StatusBadRequest, "image generation requires an OpenRouter API key — add one in Settings → Models")
		return
	}

	// Try the fallback image models in order until one returns image data.
	var b64 string
	var lastErr string
	for _, model := range llm.ImageGenModels {
		res, err := h.llm.GenerateImage(r.Context(), model, req.Prompt, "1024x1024", nil)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		if res.Base64 != "" {
			b64 = res.Base64
			break
		}
	}
	if b64 == "" {
		if lastErr == "" {
			lastErr = "no image data returned"
		}
		writeError(w, http.StatusBadGateway, "image generation failed: "+lastErr)
		return
	}

	imgData, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decode generated image")
		return
	}

	uploadsDir := filepath.Join(h.dataDir, "avatars")
	os.MkdirAll(uploadsDir, 0755)
	filename := "ws-" + uuid.New().String() + ".png"
	if err := os.WriteFile(filepath.Join(uploadsDir, filename), imgData, 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save generated image")
		return
	}

	imageURL := "/api/v1/uploads/avatars/" + filename
	h.db.Exec("UPDATE workspaces SET image_url = ?, updated_at = ? WHERE id = ?", imageURL, time.Now().UTC(), id)

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "workspace_image_generated", "workspace", "workspace", id, req.Prompt)

	writeJSON(w, http.StatusOK, map[string]string{"image_url": imageURL})
}

func (h *WorkspacesHandler) GetActive(w http.ResponseWriter, r *http.Request) {
	id := activeWorkspaceID(h.db)

	var ws models.Workspace
	err := h.db.QueryRow(
		"SELECT id, name, image_url, sort_order, is_default, created_at, updated_at FROM workspaces WHERE id = ?", id,
	).Scan(&ws.ID, &ws.Name, &ws.ImageURL, &ws.SortOrder, &ws.IsDefault, &ws.CreatedAt, &ws.UpdatedAt)
	if err != nil {
		// Active id points at a missing workspace — repair to Default.
		h.setActiveWorkspace(DefaultWorkspaceID)
		err = h.db.QueryRow(
			"SELECT id, name, image_url, sort_order, is_default, created_at, updated_at FROM workspaces WHERE id = ?", DefaultWorkspaceID,
		).Scan(&ws.ID, &ws.Name, &ws.ImageURL, &ws.SortOrder, &ws.IsDefault, &ws.CreatedAt, &ws.UpdatedAt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to resolve active workspace")
			return
		}
	}
	writeJSON(w, http.StatusOK, ws)
}

func (h *WorkspacesHandler) SetActive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WorkspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id is required")
		return
	}

	var exists int
	if err := h.db.QueryRow("SELECT 1 FROM workspaces WHERE id = ?", req.WorkspaceID).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	h.setActiveWorkspace(req.WorkspaceID)

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "workspace_activated", "workspace", "workspace", req.WorkspaceID, "")

	h.GetActive(w, r)
}

// setActiveWorkspace upserts the active workspace pointer in settings.
func (h *WorkspacesHandler) setActiveWorkspace(id string) {
	if _, err := h.db.Exec(
		"INSERT INTO settings (id, key, value) VALUES (?, 'active_workspace_id', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		uuid.New().String(), id,
	); err != nil {
		logger.Error("failed to set active workspace: %v", err)
	}
}

// fileNode is one entry in a workspace's real on-disk files tree.
type fileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"` // relative to the workspace files dir
	IsDir    bool       `json:"is_dir"`
	Size     int64      `json:"size"`
	Children []fileNode `json:"children,omitempty"`
}

// ListFiles returns the real contents of data/workspaces/<id>/files as a tree.
// This is the actual on-disk directory (repos pulled, files an agent created),
// NOT the curated context_files. The directory is created if missing.
func (h *WorkspacesHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Validate the id is a real uuid to prevent path traversal via the segment.
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid workspace id")
		return
	}
	var exists int
	if err := h.db.QueryRow("SELECT 1 FROM workspaces WHERE id = ?", id).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	root := h.filesDir(id)
	if err := os.MkdirAll(root, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workspace files dir")
		return
	}

	tree, err := listDirLevel(root, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read workspace files")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"workspace_id": id,
		"files":        tree,
	})
}

// RevealFiles opens the workspace's on-disk files directory in the OS-native
// file manager (Finder/Explorer). Only meaningful when the server runs on the
// same machine as the client — which is always the case for OpenPaw.
func (h *WorkspacesHandler) RevealFiles(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid workspace id")
		return
	}
	var exists int
	if err := h.db.QueryRow("SELECT 1 FROM workspaces WHERE id = ?", id).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	root := h.filesDir(id)
	if err := os.MkdirAll(root, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workspace files dir")
		return
	}
	if err := platform.OpenPath(root); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to open files directory")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": root})
}

// listDirLevel returns ONLY the immediate children of dir (one level, no
// recursion) with paths relative to the tree root. Recursing eagerly is fatal
// for real repos (node_modules/.git); subfolders are loaded on demand via
// Browse when the user expands them. Symlinks are reported but not followed.
func listDirLevel(dir, relBase string) ([]fileNode, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	nodes := []fileNode{}
	for _, e := range entries {
		rel := filepath.ToSlash(filepath.Join(relBase, e.Name()))
		node := fileNode{Name: e.Name(), Path: rel, IsDir: e.IsDir()}
		if !e.IsDir() {
			if info, err := e.Info(); err == nil {
				node.Size = info.Size()
			}
		}
		nodes = append(nodes, node)
	}

	// Directories first, then files, each alphabetical — stable, predictable UI.
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return nodes[i].Name < nodes[j].Name
	})
	return nodes, nil
}

// safeJoin joins a caller-supplied relative path under base, defeating any
// "../" traversal. Returns the absolute target and false if it escapes base.
func safeJoin(base, rel string) (string, bool) {
	if rel == "" {
		return base, true
	}
	// A leading slash makes Clean collapse any leading ".." to root, so the
	// subsequent Rel check reliably catches escapes.
	clean := filepath.Clean("/" + filepath.FromSlash(rel))
	target := filepath.Join(base, clean)
	r, err := filepath.Rel(base, target)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return target, true
}

// Browse returns one level of a directory on demand: the workspace's own files
// dir (dir="") or an attached directory (dir=<directory id>), under an optional
// relative path. This backs lazy expansion in the Directory tab.
func (h *WorkspacesHandler) Browse(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.validWorkspace(w, id) {
		return
	}

	dirID := r.URL.Query().Get("dir")
	rel := r.URL.Query().Get("path")

	target, ok := h.resolveBrowsePath(w, id, dirID, rel)
	if !ok {
		return
	}
	nodes, err := listDirLevel(target, rel)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read directory")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"path": rel, "files": nodes})
}

type workspaceSearchResult struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	AbsolutePath string `json:"absolute_path"`
	DirID        string `json:"dir_id"`
	Source       string `json:"source"`
	IsDir        bool   `json:"is_dir"`
	Size         int64  `json:"size"`
	score        int
}

// SearchFiles searches only the selected workspace's own files and directories
// explicitly attached to it. It deliberately skips dependency/build metadata
// trees: those produce noisy results and can contain hundreds of thousands of
// entries, which would make opening the command palette feel like a disk scan.
func (h *WorkspacesHandler) SearchFiles(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.validWorkspace(w, id) {
		return
	}

	limit := 100
	if parsed, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && parsed > 0 {
		limit = min(parsed, 200)
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	var workspaceName string
	_ = h.db.QueryRow("SELECT name FROM workspaces WHERE id = ?", id).Scan(&workspaceName)
	type root struct{ dirID, path, label string }
	roots := []root{{path: h.filesDir(id), label: workspaceName + " files"}}
	rows, err := h.db.Query(
		"SELECT id, path, label FROM workspace_directories WHERE workspace_id = ? ORDER BY created_at ASC", id,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item root
			if rows.Scan(&item.dirID, &item.path, &item.label) == nil {
				roots = append(roots, item)
			}
		}
	}

	ignored := map[string]bool{
		".git": true, ".svn": true, ".hg": true, "node_modules": true,
		".next": true, ".cache": true, "__pycache__": true, "dist": true,
		"build": true, "coverage": true, "vendor": true,
	}
	results := make([]workspaceSearchResult, 0, limit)
	// Leave some headroom before sorting so a later, better filename match can
	// beat an earlier weak path match.
	scanCap := limit * 8

	for _, root := range roots {
		if len(results) >= scanCap {
			break
		}
		if info, statErr := os.Stat(root.path); statErr != nil || !info.IsDir() {
			continue
		}
		scanned := 0
		_ = filepath.WalkDir(root.path, func(path string, entry os.DirEntry, walkErr error) error {
			if r.Context().Err() != nil {
				return filepath.SkipAll
			}
			scanned++
			if scanned > 25000 {
				return filepath.SkipAll
			}
			if walkErr != nil {
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if path == root.path {
				return nil
			}
			if entry.IsDir() && ignored[entry.Name()] {
				return filepath.SkipDir
			}
			rel, relErr := filepath.Rel(root.path, path)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			// An empty search provides a useful, fast directory overview instead
			// of recursively returning arbitrary files.
			if query == "" && strings.Contains(rel, "/") {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			nameLower := strings.ToLower(entry.Name())
			pathLower := strings.ToLower(rel)
			if query != "" && !strings.Contains(nameLower, query) && !strings.Contains(pathLower, query) {
				return nil
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				return nil
			}
			score := 3
			if nameLower == query {
				score = 0
			} else if strings.HasPrefix(nameLower, query) {
				score = 1
			} else if strings.Contains(nameLower, query) {
				score = 2
			}
			results = append(results, workspaceSearchResult{
				Name: entry.Name(), Path: rel, AbsolutePath: path, DirID: root.dirID,
				Source: root.label, IsDir: entry.IsDir(), Size: info.Size(), score: score,
			})
			if len(results) >= scanCap {
				return filepath.SkipAll
			}
			return nil
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score < results[j].score
		}
		if results[i].IsDir != results[j].IsDir {
			return results[i].IsDir
		}
		return strings.ToLower(results[i].Path) < strings.ToLower(results[j].Path)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	writeJSON(w, http.StatusOK, results)
}

// resolveBrowsePath maps a (workspace, dir, relative path) triple onto an
// absolute path, refusing anything that escapes its base. Shared by Browse,
// ReadFile and WriteFile so all three enforce the same boundary — a read/write
// endpoint with its own path handling is exactly how traversal bugs appear.
// Writes an error response and returns ok=false on failure.
func (h *WorkspacesHandler) resolveBrowsePath(w http.ResponseWriter, workspaceID, dirID, rel string) (string, bool) {
	var base string
	if dirID == "" {
		base = h.filesDir(workspaceID)
	} else {
		if err := h.db.QueryRow(
			"SELECT path FROM workspace_directories WHERE id = ? AND workspace_id = ?", dirID, workspaceID,
		).Scan(&base); err != nil {
			writeError(w, http.StatusNotFound, "directory not found")
			return "", false
		}
	}

	target, ok := safeJoin(base, rel)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid path")
		return "", false
	}
	return target, true
}

// maxEditableFileSize caps what the in-app editor will open. The editor is for
// quick edits to source and config; loading a multi-megabyte file into a
// textarea would hang the renderer, and a binary would be corrupted on save.
const maxEditableFileSize = 2 << 20 // 2 MiB

// ReadFile returns the contents of a single file for the in-app editor.
//
// Refuses directories, oversized files, and anything that isn't valid UTF-8 —
// the last check is what stops a binary being opened, mangled by a round trip
// through a textarea, and written back over the original.
func (h *WorkspacesHandler) ReadFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.validWorkspace(w, id) {
		return
	}

	target, ok := h.resolveBrowsePath(w, id, r.URL.Query().Get("dir"), r.URL.Query().Get("path"))
	if !ok {
		return
	}

	info, err := os.Stat(target)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "that path is a directory")
		return
	}
	if info.Size() > maxEditableFileSize {
		writeError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("file is %s — too large to edit in the app", formatBytes(info.Size())))
		return
	}

	data, err := os.ReadFile(target)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read file")
		return
	}
	if !utf8.Valid(data) {
		writeError(w, http.StatusUnsupportedMediaType, "this looks like a binary file, so it can't be edited here")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":        r.URL.Query().Get("path"),
		"name":        filepath.Base(target),
		"content":     string(data),
		"size":        info.Size(),
		"modified_at": info.ModTime().UTC(),
	})
}

// WriteFile saves edited contents back over an existing file.
//
// Deliberately refuses to create new files: this endpoint backs an editor
// opened from a tree row, so a path that no longer exists means the file moved
// or was deleted, and silently recreating it would resurrect deleted content.
func (h *WorkspacesHandler) WriteFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.validWorkspace(w, id) {
		return
	}

	var req struct {
		Dir     string `json:"dir"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	target, ok := h.resolveBrowsePath(w, id, req.Dir, req.Path)
	if !ok {
		return
	}

	info, err := os.Stat(target)
	if err != nil {
		writeError(w, http.StatusNotFound, "file no longer exists")
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "that path is a directory")
		return
	}
	if int64(len(req.Content)) > maxEditableFileSize {
		writeError(w, http.StatusRequestEntityTooLarge, "content is too large to save")
		return
	}

	// Preserve the file's existing permissions rather than imposing 0644 — this
	// edits real files in the user's repositories, including scripts that must
	// stay executable.
	if err := os.WriteFile(target, []byte(req.Content), info.Mode().Perm()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save file")
		return
	}

	updated, err := os.Stat(target)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "saved, but failed to stat the file")
		return
	}

	h.db.LogAudit("user", "workspace_file_edited", "user", "file", req.Path,
		fmt.Sprintf("edited %s (%s)", filepath.Base(target), formatBytes(updated.Size())))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "saved",
		"size":        updated.Size(),
		"modified_at": updated.ModTime().UTC(),
	})
}

// workspaceDirectory is one external directory attached to a workspace, beyond
// its own on-disk files dir.
type workspaceDirectory struct {
	ID      string     `json:"id"`
	Path    string     `json:"path"`
	Label   string     `json:"label"`
	Missing bool       `json:"missing,omitempty"`
	Files   []fileNode `json:"files"`
}

// validWorkspace validates id is a uuid and that the workspace exists,
// writing an error response and returning false if not.
func (h *WorkspacesHandler) validWorkspace(w http.ResponseWriter, id string) bool {
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid workspace id")
		return false
	}
	var exists int
	if err := h.db.QueryRow("SELECT 1 FROM workspaces WHERE id = ?", id).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return false
	}
	return true
}

// AddDirectory attaches an external, on-disk directory to a workspace so it
// shows up in the Directory tab and agents can access it alongside the
// workspace's own files dir. Duplicate paths for the same workspace are
// ignored (the existing row is returned).
func (h *WorkspacesHandler) AddDirectory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.validWorkspace(w, id) {
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "path does not exist or is not a directory")
		return
	}

	// Prevent duplicates for the same workspace.
	var existingID string
	err = h.db.QueryRow(
		"SELECT id FROM workspace_directories WHERE workspace_id = ? AND path = ?", id, path,
	).Scan(&existingID)
	if err == nil && existingID != "" {
		var dir workspaceDirectory
		var label string
		h.db.QueryRow("SELECT path, label FROM workspace_directories WHERE id = ?", existingID).Scan(&path, &label)
		dir = workspaceDirectory{ID: existingID, Path: path, Label: label, Files: []fileNode{}}
		if tree, err := listDirLevel(path, ""); err == nil {
			dir.Files = tree
		} else {
			dir.Missing = true
		}
		writeJSON(w, http.StatusOK, dir)
		return
	}

	dirID := uuid.New().String()
	label := filepath.Base(path)
	now := time.Now().UTC()
	if _, err := h.db.Exec(
		"INSERT INTO workspace_directories (id, workspace_id, path, label, created_at) VALUES (?, ?, ?, ?, ?)",
		dirID, id, path, label, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to attach directory")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "workspace_directory_added", "workspace", "workspace_directory", dirID, path)

	tree, err := listDirLevel(path, "")
	if err != nil {
		tree = []fileNode{}
	}
	writeJSON(w, http.StatusCreated, workspaceDirectory{
		ID:    dirID,
		Path:  path,
		Label: label,
		Files: tree,
	})
}

// ListDirectories returns every external directory attached to a workspace,
// each with its own file tree. Directories that no longer exist on disk are
// still returned (so the user can remove the stale attachment) but marked
// missing with an empty tree.
func (h *WorkspacesHandler) ListDirectories(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.validWorkspace(w, id) {
		return
	}

	rows, err := h.db.Query(
		"SELECT id, path, label FROM workspace_directories WHERE workspace_id = ? ORDER BY created_at ASC", id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list directories")
		return
	}
	defer rows.Close()

	dirs := []workspaceDirectory{}
	for rows.Next() {
		var d workspaceDirectory
		if err := rows.Scan(&d.ID, &d.Path, &d.Label); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan directory")
			return
		}
		if info, err := os.Stat(d.Path); err != nil || !info.IsDir() {
			d.Missing = true
			d.Files = []fileNode{}
		} else if tree, err := listDirLevel(d.Path, ""); err == nil {
			d.Files = tree
		} else {
			d.Missing = true
			d.Files = []fileNode{}
		}
		dirs = append(dirs, d)
	}

	writeJSON(w, http.StatusOK, dirs)
}

// RemoveDirectory detaches an external directory from a workspace. It only
// removes the attachment row — the real directory on disk is never touched.
func (h *WorkspacesHandler) RemoveDirectory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.validWorkspace(w, id) {
		return
	}
	dirID := chi.URLParam(r, "dirId")

	var path string
	if err := h.db.QueryRow(
		"SELECT path FROM workspace_directories WHERE id = ? AND workspace_id = ?", dirID, id,
	).Scan(&path); err != nil {
		writeError(w, http.StatusNotFound, "directory not found")
		return
	}

	if _, err := h.db.Exec("DELETE FROM workspace_directories WHERE id = ? AND workspace_id = ?", dirID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove directory")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "workspace_directory_removed", "workspace", "workspace_directory", dirID, path)

	w.WriteHeader(http.StatusNoContent)
}

// WorkspaceExtraDirs returns the absolute paths of every external directory
// attached to a workspace that still exists on disk. Used by the agent
// gateway to grant CLI providers access beyond the workspace's own files dir.
func WorkspaceExtraDirs(db *database.DB, workspaceID string) []string {
	rows, err := db.Query("SELECT path FROM workspace_directories WHERE workspace_id = ? ORDER BY created_at ASC", workspaceID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var dirs []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			continue
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			dirs = append(dirs, path)
		}
	}
	return dirs
}
