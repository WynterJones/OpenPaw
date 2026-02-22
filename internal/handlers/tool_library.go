package handlers

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
	"github.com/openpaw/openpaw/internal/secrets"
	"github.com/openpaw/openpaw/internal/toollibrary"
	"github.com/openpaw/openpaw/internal/toolmgr"
)

type ToolLibraryHandler struct {
	db         *database.DB
	toolMgr    *toolmgr.Manager
	toolsDir   string
	secretsMgr *secrets.Manager
}

func NewToolLibraryHandler(db *database.DB, toolMgr *toolmgr.Manager, toolsDir string, secretsMgr *secrets.Manager) *ToolLibraryHandler {
	return &ToolLibraryHandler{db: db, toolMgr: toolMgr, toolsDir: toolsDir, secretsMgr: secretsMgr}
}

func (h *ToolLibraryHandler) ListCatalog(w http.ResponseWriter, r *http.Request) {
	tools, err := toollibrary.LoadRegistry()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load catalog")
		return
	}
	tools = toollibrary.MarkInstalled(tools, h.db)

	category := r.URL.Query().Get("category")
	search := strings.ToLower(r.URL.Query().Get("q"))

	var filtered []toollibrary.CatalogTool
	for _, t := range tools {
		if category != "" && !strings.EqualFold(t.Category, category) {
			continue
		}
		if search != "" {
			match := strings.Contains(strings.ToLower(t.Name), search) ||
				strings.Contains(strings.ToLower(t.Description), search)
			if !match {
				for _, tag := range t.Tags {
					if strings.Contains(strings.ToLower(tag), search) {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}
		filtered = append(filtered, t)
	}

	if filtered == nil {
		filtered = []toollibrary.CatalogTool{}
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (h *ToolLibraryHandler) GetCatalogTool(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	tool, err := toollibrary.GetCatalogTool(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "catalog tool not found")
		return
	}

	installed, installedID := toollibrary.IsInstalled(h.db, slug)
	tool.Installed = installed

	resp := struct {
		toollibrary.CatalogTool
		InstalledID string `json:"installed_id,omitempty"`
	}{
		CatalogTool: *tool,
		InstalledID: installedID,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *ToolLibraryHandler) InstallCatalogTool(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	if installed, _ := toollibrary.IsInstalled(h.db, slug); installed {
		writeError(w, http.StatusConflict, "tool already installed")
		return
	}

	toolID, err := toollibrary.InstallTool(h.db, slug, h.toolsDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("install failed: %v", err))
		return
	}

	if h.toolMgr != nil {
		if err := h.toolMgr.CompileTool(toolID); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("compile failed: %v", err))
			return
		}
		if err := h.toolMgr.StartTool(toolID); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("start failed: %v", err))
			return
		}
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "library_tool_installed", "tool", "tool", toolID, slug)

	// Create placeholder secrets for required env vars that don't exist yet
	catalogTool, _ := toollibrary.GetCatalogTool(slug)
	if catalogTool != nil && len(catalogTool.Env) > 0 && h.secretsMgr != nil {
		for _, envName := range catalogTool.Env {
			var exists int
			h.db.QueryRow("SELECT COUNT(*) FROM secrets WHERE name = ?", envName).Scan(&exists)
			if exists == 0 {
				placeholder := "REPLACE_ME"
				encrypted, encErr := h.secretsMgr.Encrypt(placeholder)
				if encErr == nil {
					secretID := uuid.New().String()
					now := time.Now().UTC()
					h.db.Exec(
						"INSERT INTO secrets (id, name, encrypted_value, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
						secretID, envName, encrypted, fmt.Sprintf("Required by %s — replace with your real key", catalogTool.Name), now, now,
					)
				}
			}
		}
	}

	var t models.Tool
	h.db.QueryRow(
		"SELECT id, name, description, type, config, enabled, status, port, pid, capabilities, owner_agent_slug, library_slug, library_version, source_hash, binary_hash, created_at, updated_at FROM tools WHERE id = ?",
		toolID,
	).Scan(&t.ID, &t.Name, &t.Description, &t.Type, &t.Config, &t.Enabled, &t.Status, &t.Port, &t.PID, &t.Capabilities, &t.OwnerAgentSlug, &t.LibrarySlug, &t.LibraryVersion, &t.SourceHash, &t.BinaryHash, &t.CreatedAt, &t.UpdatedAt)

	writeJSON(w, http.StatusCreated, t)
}

func (h *ToolLibraryHandler) ExportTool(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var t models.Tool
	err := h.db.QueryRow(
		"SELECT id, name, description, type, source_hash FROM tools WHERE id = ? AND deleted_at IS NULL",
		id,
	).Scan(&t.ID, &t.Name, &t.Description, &t.Type, &t.SourceHash)
	if err != nil {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}

	toolDir := filepath.Join(h.toolsDir, id)
	if _, err := os.Stat(toolDir); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "tool directory not found")
		return
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	fileHashes := make(map[string]string)
	fileCount := 0

	err = filepath.Walk(toolDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if fileCount >= 100 {
			return fmt.Errorf("too many files (max 100)")
		}

		rel, _ := filepath.Rel(toolDir, path)

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		h := sha256.Sum256(data)
		fileHashes[rel] = hex.EncodeToString(h[:])

		fw, err := zw.Create(rel)
		if err != nil {
			return err
		}
		_, err = fw.Write(data)
		fileCount++
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("export failed: %v", err))
		return
	}

	exportMeta := map[string]interface{}{
		"tool_name":        t.Name,
		"tool_description": t.Description,
		"tool_type":        t.Type,
		"source_hash":      t.SourceHash,
		"file_hashes":      fileHashes,
		"exported_at":      time.Now().UTC().Format(time.RFC3339),
		"version":          "1.0",
	}
	metaJSON, _ := json.MarshalIndent(exportMeta, "", "  ")
	fw, _ := zw.Create("tool-export.json")
	fw.Write(metaJSON)

	zw.Close()

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "tool_exported", "tool", "tool", id, t.Name)

	safeName := strings.ReplaceAll(strings.ToLower(t.Name), " ", "-")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, safeName))
	w.Write(buf.Bytes())
}

func (h *ToolLibraryHandler) ImportTool(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20) // 50MB max

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing or invalid file upload")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read upload")
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid zip file")
		return
	}

	if len(zr.File) > 100 {
		writeError(w, http.StatusBadRequest, "too many files in archive (max 100)")
		return
	}

	var exportMeta struct {
		ToolName        string            `json:"tool_name"`
		ToolDescription string            `json:"tool_description"`
		ToolType        string            `json:"tool_type"`
		SourceHash      string            `json:"source_hash"`
		FileHashes      map[string]string `json:"file_hashes"`
	}

	extractedFiles := make(map[string][]byte)
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		if zf.UncompressedSize64 > 10<<20 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("file %s too large", zf.Name))
			return
		}
		rc, err := zf.Open()
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to open %s", zf.Name))
			return
		}
		fileData, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to read %s", zf.Name))
			return
		}
		extractedFiles[zf.Name] = fileData
	}

	metaData, ok := extractedFiles["tool-export.json"]
	if !ok {
		writeError(w, http.StatusBadRequest, "missing tool-export.json in archive")
		return
	}
	if err := json.Unmarshal(metaData, &exportMeta); err != nil {
		writeError(w, http.StatusBadRequest, "invalid tool-export.json")
		return
	}

	for filename, expectedHash := range exportMeta.FileHashes {
		fileData, exists := extractedFiles[filename]
		if !exists {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("missing declared file: %s", filename))
			return
		}
		actualHash := sha256.Sum256(fileData)
		if hex.EncodeToString(actualHash[:]) != expectedHash {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("hash mismatch for %s: file has been tampered with", filename))
			return
		}
	}

	toolID := uuid.New().String()
	toolDir := filepath.Join(h.toolsDir, toolID)
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create tool directory")
		return
	}

	for filename, fileData := range extractedFiles {
		if filename == "tool-export.json" {
			continue
		}
		if filepath.Base(filename) == "tool" {
			continue
		}

		if strings.Contains(filename, "..") {
			os.RemoveAll(toolDir)
			writeError(w, http.StatusBadRequest, "invalid file path in archive")
			return
		}

		outPath := filepath.Join(toolDir, filename)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			os.RemoveAll(toolDir)
			writeError(w, http.StatusInternalServerError, "failed to create directory")
			return
		}
		if err := os.WriteFile(outPath, fileData, 0644); err != nil {
			os.RemoveAll(toolDir)
			writeError(w, http.StatusInternalServerError, "failed to write file")
			return
		}
	}

	now := time.Now().UTC()
	toolName := exportMeta.ToolName
	if toolName == "" {
		toolName = "Imported Tool"
	}
	toolType := exportMeta.ToolType
	if toolType == "" {
		toolType = "generic"
	}

	_, err = h.db.Exec(
		`INSERT INTO tools (id, name, description, type, config, enabled, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, '{}', 1, 'active', ?, ?)`,
		toolID, toolName, exportMeta.ToolDescription, toolType, now, now,
	)
	if err != nil {
		os.RemoveAll(toolDir)
		writeError(w, http.StatusInternalServerError, "failed to create tool record")
		return
	}

	if h.toolMgr != nil {
		if err := h.toolMgr.CompileTool(toolID); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("compile failed: %v", err))
			return
		}
		if err := h.toolMgr.StartTool(toolID); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("start failed: %v", err))
			return
		}
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "tool_imported", "tool", "tool", toolID, toolName)

	var t models.Tool
	h.db.QueryRow(
		"SELECT id, name, description, type, config, enabled, status, port, pid, capabilities, owner_agent_slug, library_slug, library_version, source_hash, binary_hash, created_at, updated_at FROM tools WHERE id = ?",
		toolID,
	).Scan(&t.ID, &t.Name, &t.Description, &t.Type, &t.Config, &t.Enabled, &t.Status, &t.Port, &t.PID, &t.Capabilities, &t.OwnerAgentSlug, &t.LibrarySlug, &t.LibraryVersion, &t.SourceHash, &t.BinaryHash, &t.CreatedAt, &t.UpdatedAt)

	writeJSON(w, http.StatusCreated, t)
}

func (h *ToolLibraryHandler) GetIntegrity(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var t models.Tool
	err := h.db.QueryRow(
		"SELECT id, name, source_hash, binary_hash FROM tools WHERE id = ? AND deleted_at IS NULL",
		id,
	).Scan(&t.ID, &t.Name, &t.SourceHash, &t.BinaryHash)
	if err != nil {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}

	files, err := toollibrary.GetIntegrity(h.db, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get integrity data")
		return
	}
	if files == nil {
		files = []toollibrary.FileHash{}
	}

	toolDir := filepath.Join(h.toolsDir, id)
	tampered := toollibrary.IsTampered(toolDir, t.SourceHash)

	resp := struct {
		SourceHash string                `json:"source_hash"`
		BinaryHash string                `json:"binary_hash"`
		Verified   bool                  `json:"verified"`
		Files      []toollibrary.FileHash `json:"files"`
	}{
		SourceHash: t.SourceHash,
		BinaryHash: t.BinaryHash,
		Verified:   !tampered,
		Files:      files,
	}

	writeJSON(w, http.StatusOK, resp)
}
