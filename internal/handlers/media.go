package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
)

type MediaHandler struct {
	db      *database.DB
	dataDir string
}

func NewMediaHandler(db *database.DB, dataDir string) *MediaHandler {
	return &MediaHandler{db: db, dataDir: dataDir}
}

func (h *MediaHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	mediaType := r.URL.Query().Get("type")
	source := r.URL.Query().Get("source")

	where := "1=1"
	args := []interface{}{}
	if mediaType != "" {
		where += " AND media_type = ?"
		args = append(args, mediaType)
	}
	if source != "" {
		where += " AND source = ?"
		args = append(args, source)
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	h.db.QueryRow("SELECT COUNT(*) FROM media WHERE "+where, countArgs...).Scan(&total)

	args = append(args, perPage, offset)
	rows, err := h.db.Query(
		"SELECT id, thread_id, message_id, source, source_model, media_type, url, filename, mime_type, width, height, size_bytes, prompt, metadata, created_at FROM media WHERE "+where+" ORDER BY created_at DESC LIMIT ? OFFSET ?",
		args...,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query media")
		return
	}
	defer rows.Close()

	items := []map[string]interface{}{}
	for rows.Next() {
		var id, threadID, messageID, src, srcModel, mType, url, filename, mimeType, prompt, metadata, createdAt string
		var width, height, sizeBytes int
		if err := rows.Scan(&id, &threadID, &messageID, &src, &srcModel, &mType, &url, &filename, &mimeType, &width, &height, &sizeBytes, &prompt, &metadata, &createdAt); err != nil {
			continue
		}
		items = append(items, map[string]interface{}{
			"id":           id,
			"thread_id":    threadID,
			"message_id":   messageID,
			"source":       src,
			"source_model": srcModel,
			"media_type":   mType,
			"url":          url,
			"filename":     filename,
			"mime_type":    mimeType,
			"width":        width,
			"height":       height,
			"size_bytes":   sizeBytes,
			"prompt":       prompt,
			"metadata":     metadata,
			"created_at":   createdAt,
			"local_url":    fmt.Sprintf("/api/v1/media/%s/file", id),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"total": total,
	})
}

func (h *MediaHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var threadID, messageID, src, srcModel, mType, url, filename, mimeType, prompt, metadata, createdAt string
	var width, height, sizeBytes int
	err := h.db.QueryRow(
		"SELECT id, thread_id, message_id, source, source_model, media_type, url, filename, mime_type, width, height, size_bytes, prompt, metadata, created_at FROM media WHERE id = ?",
		id,
	).Scan(&id, &threadID, &messageID, &src, &srcModel, &mType, &url, &filename, &mimeType, &width, &height, &sizeBytes, &prompt, &metadata, &createdAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":           id,
		"thread_id":    threadID,
		"message_id":   messageID,
		"source":       src,
		"source_model": srcModel,
		"media_type":   mType,
		"url":          url,
		"filename":     filename,
		"mime_type":    mimeType,
		"width":        width,
		"height":       height,
		"size_bytes":   sizeBytes,
		"prompt":       prompt,
		"metadata":     metadata,
		"created_at":   createdAt,
		"local_url":    fmt.Sprintf("/api/v1/media/%s/file", id),
	})
}

func (h *MediaHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var filename string
	err := h.db.QueryRow("SELECT filename FROM media WHERE id = ?", id).Scan(&filename)
	if err != nil {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}

	// Delete file from disk
	mediaDir := filepath.Join(h.dataDir, "..", "media")
	filePath := filepath.Join(mediaDir, filename)
	os.Remove(filePath)

	// Delete record
	h.db.Exec("DELETE FROM media WHERE id = ?", id)

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "media_deleted", "media", "media", id, "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *MediaHandler) ServeFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var filename string
	err := h.db.QueryRow("SELECT filename FROM media WHERE id = ?", id).Scan(&filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Sanitize filename to prevent path traversal
	if filename == "" || strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		http.NotFound(w, r)
		return
	}

	mediaDir := filepath.Join(h.dataDir, "..", "media")
	filePath := filepath.Join(mediaDir, filename)
	http.ServeFile(w, r, filePath)
}

func (h *MediaHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	searchTerm := "%" + escapeLike(q) + "%"

	var total int
	h.db.QueryRow("SELECT COUNT(*) FROM media WHERE prompt LIKE ? ESCAPE '\\'", searchTerm).Scan(&total)

	rows, err := h.db.Query(
		"SELECT id, thread_id, message_id, source, source_model, media_type, url, filename, mime_type, width, height, size_bytes, prompt, metadata, created_at FROM media WHERE prompt LIKE ? ESCAPE '\\' ORDER BY created_at DESC LIMIT ? OFFSET ?",
		searchTerm, perPage, offset,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	items := []map[string]interface{}{}
	for rows.Next() {
		var id, threadID, messageID, src, srcModel, mType, url, filename, mimeType, prompt, metadata, createdAt string
		var width, height, sizeBytes int
		if err := rows.Scan(&id, &threadID, &messageID, &src, &srcModel, &mType, &url, &filename, &mimeType, &width, &height, &sizeBytes, &prompt, &metadata, &createdAt); err != nil {
			continue
		}
		items = append(items, map[string]interface{}{
			"id":           id,
			"thread_id":    threadID,
			"message_id":   messageID,
			"source":       src,
			"source_model": srcModel,
			"media_type":   mType,
			"url":          url,
			"filename":     filename,
			"mime_type":    mimeType,
			"width":        width,
			"height":       height,
			"size_bytes":   sizeBytes,
			"prompt":       prompt,
			"metadata":     metadata,
			"created_at":   createdAt,
			"local_url":    fmt.Sprintf("/api/v1/media/%s/file", id),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"total": total,
	})
}
