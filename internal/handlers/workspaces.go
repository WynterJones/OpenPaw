package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
}

func NewWorkspacesHandler(db *database.DB, dataDir string) *WorkspacesHandler {
	return &WorkspacesHandler{db: db, dataDir: dataDir}
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

	// Reassign scoped rows to the Default workspace to avoid data loss.
	for _, tbl := range []string{"chat_threads", "dashboards", "context_files", "context_folders", "todo_lists"} {
		h.db.Exec("UPDATE "+tbl+" SET workspace_id = ? WHERE workspace_id = ?", DefaultWorkspaceID, id)
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

	tree, err := buildFileTree(root, "")
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

// buildFileTree walks dir recursively, returning nodes with paths relative to
// the workspace files root. Symlinks are reported but not followed.
func buildFileTree(dir, relBase string) ([]fileNode, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	nodes := []fileNode{}
	for _, e := range entries {
		rel := filepath.ToSlash(filepath.Join(relBase, e.Name()))
		node := fileNode{Name: e.Name(), Path: rel, IsDir: e.IsDir()}
		if e.IsDir() {
			children, err := buildFileTree(filepath.Join(dir, e.Name()), rel)
			if err == nil {
				node.Children = children
			}
		} else if info, err := e.Info(); err == nil {
			node.Size = info.Size()
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
