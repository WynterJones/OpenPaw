package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
)

const maxUploadSize = 10 << 20 // 10MB

type ContextHandler struct {
	db      *database.DB
	dataDir string
}

func NewContextHandler(db *database.DB, dataDir string) *ContextHandler {
	return &ContextHandler{db: db, dataDir: dataDir}
}

func (h *ContextHandler) contextDir() string {
	return filepath.Join(h.dataDir, "context")
}

func (h *ContextHandler) attachmentsDir() string {
	return filepath.Join(h.dataDir, "chat-attachments")
}

// --- Tree ---

type TreeNode struct {
	models.ContextFolder
	Children []TreeNode           `json:"children"`
	Files    []models.ContextFile `json:"files"`
}

func (h *ContextHandler) GetTree(w http.ResponseWriter, r *http.Request) {
	folders := []models.ContextFolder{}
	rows, err := h.db.Query("SELECT id, parent_id, name, sort_order, created_at, updated_at FROM context_folders ORDER BY sort_order, name")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list folders")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var f models.ContextFolder
		if err := rows.Scan(&f.ID, &f.ParentID, &f.Name, &f.SortOrder, &f.CreatedAt, &f.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan folder")
			return
		}
		folders = append(folders, f)
	}

	files := []models.ContextFile{}
	frows, err := h.db.Query("SELECT id, folder_id, name, filename, mime_type, size_bytes, is_about_you, created_at, updated_at FROM context_files ORDER BY name")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list files")
		return
	}
	defer frows.Close()
	for frows.Next() {
		var f models.ContextFile
		if err := frows.Scan(&f.ID, &f.FolderID, &f.Name, &f.Filename, &f.MimeType, &f.SizeBytes, &f.IsAboutYou, &f.CreatedAt, &f.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan file")
			return
		}
		files = append(files, f)
	}

	tree := buildTree(folders, files, nil)

	// Root-level files (no folder)
	rootFiles := []models.ContextFile{}
	for _, f := range files {
		if f.FolderID == nil {
			rootFiles = append(rootFiles, f)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"folders": tree,
		"files":   rootFiles,
	})
}

func buildTree(folders []models.ContextFolder, files []models.ContextFile, parentID *string) []TreeNode {
	nodes := []TreeNode{}
	for _, f := range folders {
		match := false
		if parentID == nil && f.ParentID == nil {
			match = true
		} else if parentID != nil && f.ParentID != nil && *parentID == *f.ParentID {
			match = true
		}
		if !match {
			continue
		}
		node := TreeNode{
			ContextFolder: f,
			Children:      buildTree(folders, files, &f.ID),
			Files:         folderFiles(files, f.ID),
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func folderFiles(files []models.ContextFile, folderID string) []models.ContextFile {
	result := []models.ContextFile{}
	for _, f := range files {
		if f.FolderID != nil && *f.FolderID == folderID {
			result = append(result, f)
		}
	}
	return result
}

// --- Folders ---

func (h *ContextHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id,omitempty"`
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

	var maxSort int
	h.db.QueryRow("SELECT COALESCE(MAX(sort_order), 0) FROM context_folders WHERE parent_id IS ?", req.ParentID).Scan(&maxSort)

	_, err := h.db.Exec(
		"INSERT INTO context_folders (id, parent_id, name, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, req.ParentID, req.Name, maxSort+1, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create folder")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "context_folder_created", "context", "context_folder", id, req.Name)

	writeJSON(w, http.StatusCreated, models.ContextFolder{
		ID:        id,
		ParentID:  req.ParentID,
		Name:      req.Name,
		SortOrder: maxSort + 1,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (h *ContextHandler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name     *string `json:"name,omitempty"`
		ParentID *string `json:"parent_id,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()
	if req.Name != nil {
		h.db.Exec("UPDATE context_folders SET name = ?, updated_at = ? WHERE id = ?", *req.Name, now, id)
	}
	if req.ParentID != nil {
		if *req.ParentID == id {
			writeError(w, http.StatusBadRequest, "folder cannot be its own parent")
			return
		}
		h.db.Exec("UPDATE context_folders SET parent_id = ?, updated_at = ? WHERE id = ?", *req.ParentID, now, id)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *ContextHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Get all files in this folder (and subfolders) to delete from disk
	h.deleteFilesInFolder(id)

	// Delete child folders recursively (SQLite CASCADE should handle DB, but we need disk cleanup)
	childRows, _ := h.db.Query("SELECT id FROM context_folders WHERE parent_id = ?", id)
	if childRows != nil {
		defer childRows.Close()
		for childRows.Next() {
			var childID string
			childRows.Scan(&childID)
			h.deleteFilesInFolder(childID)
		}
	}

	// Orphan files in this folder (move to root)
	h.db.Exec("UPDATE context_files SET folder_id = NULL WHERE folder_id = ?", id)

	result, err := h.db.Exec("DELETE FROM context_folders WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete folder")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "folder not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "context_folder_deleted", "context", "context_folder", id, "")

	w.WriteHeader(http.StatusNoContent)
}

func (h *ContextHandler) deleteFilesInFolder(folderID string) {
	rows, err := h.db.Query("SELECT filename FROM context_files WHERE folder_id = ?", folderID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var filename string
		rows.Scan(&filename)
		os.Remove(filepath.Join(h.contextDir(), filepath.Base(filename)))
	}
	h.db.Exec("DELETE FROM context_files WHERE folder_id = ?", folderID)
}

// --- Files ---

func (h *ContextHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query("SELECT id, folder_id, name, filename, mime_type, size_bytes, is_about_you, created_at, updated_at FROM context_files ORDER BY name")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list files")
		return
	}
	defer rows.Close()

	files := []models.ContextFile{}
	for rows.Next() {
		var f models.ContextFile
		if err := rows.Scan(&f.ID, &f.FolderID, &f.Name, &f.Filename, &f.MimeType, &f.SizeBytes, &f.IsAboutYou, &f.CreatedAt, &f.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan file")
			return
		}
		files = append(files, f)
	}
	writeJSON(w, http.StatusOK, files)
}

func (h *ContextHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var f models.ContextFile
	err := h.db.QueryRow(
		"SELECT id, folder_id, name, filename, mime_type, size_bytes, is_about_you, created_at, updated_at FROM context_files WHERE id = ?",
		id,
	).Scan(&f.ID, &f.FolderID, &f.Name, &f.Filename, &f.MimeType, &f.SizeBytes, &f.IsAboutYou, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	result := map[string]interface{}{
		"file": f,
	}

	// For text files, include content
	if isTextMime(f.MimeType) {
		diskPath := filepath.Join(h.contextDir(), filepath.Base(f.Filename))
		data, err := os.ReadFile(diskPath)
		if err == nil {
			result["content"] = string(data)
		}
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *ContextHandler) ServeRaw(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var filename, mimeType string
	err := h.db.QueryRow("SELECT filename, mime_type FROM context_files WHERE id = ?", id).Scan(&filename, &mimeType)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	diskPath := filepath.Join(h.contextDir(), filepath.Base(filename))
	w.Header().Set("Content-Type", mimeType)
	http.ServeFile(w, r, diskPath)
}

func (h *ContextHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large (max 10MB)")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	folderID := r.FormValue("folder_id")
	var folderPtr *string
	if folderID != "" {
		folderPtr = &folderID
	}

	// Ensure context dir exists
	if err := os.MkdirAll(h.contextDir(), 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create storage directory")
		return
	}

	ext := filepath.Ext(header.Filename)
	mimeType := detectMimeType(header.Filename, header.Header.Get("Content-Type"))
	diskFilename := uuid.New().String() + ext

	diskPath := filepath.Join(h.contextDir(), diskFilename)
	dst, err := os.Create(diskPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create file")
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		os.Remove(diskPath)
		writeError(w, http.StatusInternalServerError, "failed to write file")
		return
	}
	dst.Close()

	displayName := strings.TrimSuffix(header.Filename, ext)
	if displayName == "" {
		displayName = header.Filename
	}

	// PDF conversion
	if mimeType == "application/pdf" {
		converted, convName, convErr := h.convertPDF(diskPath, displayName)
		if convErr == nil {
			// Replace the file with the converted text
			os.Remove(diskPath)
			diskFilename = convName
			diskPath = filepath.Join(h.contextDir(), diskFilename)
			mimeType = "text/markdown"
			ext = ".md"
			displayName = displayName + " (converted)"
			info, _ := os.Stat(diskPath)
			if info != nil {
				written = info.Size()
			}
			_ = converted
		}
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	_, err = h.db.Exec(
		"INSERT INTO context_files (id, folder_id, name, filename, mime_type, size_bytes, is_about_you, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)",
		id, folderPtr, displayName, diskFilename, mimeType, written, now, now,
	)
	if err != nil {
		os.Remove(diskPath)
		writeError(w, http.StatusInternalServerError, "failed to save file record")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "context_file_uploaded", "context", "context_file", id, displayName)

	writeJSON(w, http.StatusCreated, models.ContextFile{
		ID:        id,
		FolderID:  folderPtr,
		Name:      displayName,
		Filename:  diskFilename,
		MimeType:  mimeType,
		SizeBytes: written,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (h *ContextHandler) convertPDF(pdfPath, baseName string) (string, string, error) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return "", "", fmt.Errorf("pdftotext not available")
	}

	mdFilename := uuid.New().String() + ".md"
	txtPath := filepath.Join(h.contextDir(), mdFilename)

	cmd := exec.Command("pdftotext", "-layout", pdfPath, txtPath)
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("pdftotext failed: %w", err)
	}

	return txtPath, mdFilename, nil
}

func (h *ContextHandler) UpdateFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Name    *string `json:"name,omitempty"`
		Content *string `json:"content,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var filename, mimeType string
	err := h.db.QueryRow("SELECT filename, mime_type FROM context_files WHERE id = ?", id).Scan(&filename, &mimeType)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	now := time.Now().UTC()

	if req.Name != nil {
		h.db.Exec("UPDATE context_files SET name = ?, updated_at = ? WHERE id = ?", *req.Name, now, id)
	}

	if req.Content != nil && isTextMime(mimeType) {
		diskPath := filepath.Join(h.contextDir(), filepath.Base(filename))
		if err := os.WriteFile(diskPath, []byte(*req.Content), 0644); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update file content")
			return
		}
		info, _ := os.Stat(diskPath)
		if info != nil {
			h.db.Exec("UPDATE context_files SET size_bytes = ?, updated_at = ? WHERE id = ?", info.Size(), now, id)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *ContextHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var filename string
	err := h.db.QueryRow("SELECT filename FROM context_files WHERE id = ?", id).Scan(&filename)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	// Delete from disk
	os.Remove(filepath.Join(h.contextDir(), filepath.Base(filename)))

	h.db.Exec("DELETE FROM context_files WHERE id = ?", id)

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "context_file_deleted", "context", "context_file", id, "")

	w.WriteHeader(http.StatusNoContent)
}

func (h *ContextHandler) MoveFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		FolderID *string `json:"folder_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()
	result, err := h.db.Exec("UPDATE context_files SET folder_id = ?, updated_at = ? WHERE id = ?", req.FolderID, now, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to move file")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "moved"})
}

// --- About You ---

func (h *ContextHandler) GetAboutYou(w http.ResponseWriter, r *http.Request) {
	var content string
	err := h.db.QueryRow("SELECT value FROM settings WHERE key = 'about_you'").Scan(&content)
	if err != nil {
		content = ""
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content})
}

func (h *ContextHandler) UpdateAboutYou(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var exists string
	err := h.db.QueryRow("SELECT id FROM settings WHERE key = 'about_you'").Scan(&exists)
	if err == sql.ErrNoRows {
		id := uuid.New().String()
		_, err = h.db.Exec("INSERT INTO settings (id, key, value) VALUES (?, ?, ?)", id, "about_you", req.Content)
	} else {
		_, err = h.db.Exec("UPDATE settings SET value = ? WHERE key = 'about_you'", req.Content)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save about you")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "about_you_updated", "context", "settings", "about_you", "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// --- Chat Attachments ---

func (h *ContextHandler) UploadChatAttachment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large (max 10MB)")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	messageID := r.FormValue("message_id")
	if messageID == "" {
		writeError(w, http.StatusBadRequest, "message_id is required")
		return
	}

	if err := os.MkdirAll(h.attachmentsDir(), 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create attachments directory")
		return
	}

	ext := filepath.Ext(header.Filename)
	diskFilename := uuid.New().String() + ext
	diskPath := filepath.Join(h.attachmentsDir(), diskFilename)

	dst, err := os.Create(diskPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create file")
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		os.Remove(diskPath)
		writeError(w, http.StatusInternalServerError, "failed to write file")
		return
	}

	mimeType := detectMimeType(header.Filename, header.Header.Get("Content-Type"))

	id := uuid.New().String()
	now := time.Now().UTC()

	_, err = h.db.Exec(
		"INSERT INTO chat_attachments (id, message_id, filename, original_name, mime_type, size_bytes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, messageID, diskFilename, header.Filename, mimeType, written, now,
	)
	if err != nil {
		os.Remove(diskPath)
		writeError(w, http.StatusInternalServerError, "failed to save attachment record")
		return
	}

	writeJSON(w, http.StatusCreated, models.ChatAttachment{
		ID:           id,
		MessageID:    messageID,
		Filename:     diskFilename,
		OriginalName: header.Filename,
		MimeType:     mimeType,
		SizeBytes:    written,
		CreatedAt:    now,
	})
}

func (h *ContextHandler) ServeChatAttachment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var filename, mimeType string
	err := h.db.QueryRow("SELECT filename, mime_type FROM chat_attachments WHERE id = ?", id).Scan(&filename, &mimeType)
	if err != nil {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}

	diskPath := filepath.Join(h.attachmentsDir(), filepath.Base(filename))
	w.Header().Set("Content-Type", mimeType)
	http.ServeFile(w, r, diskPath)
}

// --- About You for Chat Injection ---

func GetAboutYouContent(db *database.DB, dataDir string) string {
	var content string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'about_you'").Scan(&content)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(content)
}

// --- Helpers ---

func isTextMime(mime string) bool {
	return strings.HasPrefix(mime, "text/") || mime == "application/json" || mime == "application/xml" || mime == "application/javascript"
}

func detectMimeType(filename, contentType string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".md", ".markdown":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	case ".html", ".htm":
		return "text/html"
	case ".xml":
		return "application/xml"
	case ".js":
		return "application/javascript"
	case ".ts":
		return "text/typescript"
	case ".go":
		return "text/x-go"
	case ".py":
		return "text/x-python"
	case ".rs":
		return "text/x-rust"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".toml":
		return "text/toml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	}
	if contentType != "" && contentType != "application/octet-stream" {
		return contentType
	}
	return "text/plain"
}

