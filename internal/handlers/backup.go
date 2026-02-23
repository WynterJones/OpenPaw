package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
)

type BackupManager interface {
	GetConfig() map[string]interface{}
	UpdateConfig(cfg map[string]string) error
	RunNow()
	IsRunning() bool
}

type BackupHandler struct {
	db  *database.DB
	mgr BackupManager
}

func NewBackupHandler(db *database.DB, mgr BackupManager) *BackupHandler {
	return &BackupHandler{db: db, mgr: mgr}
}

func (h *BackupHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.mgr.GetConfig()
	writeJSON(w, http.StatusOK, cfg)
}

func (h *BackupHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.mgr.UpdateConfig(req); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update backup config: "+err.Error())
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "backup_config_updated", "backup", "settings", "backup", "")

	writeJSON(w, http.StatusOK, h.mgr.GetConfig())
}

func (h *BackupHandler) RunNow(w http.ResponseWriter, r *http.Request) {
	if h.mgr.IsRunning() {
		writeError(w, http.StatusConflict, "backup already running")
		return
	}

	go h.mgr.RunNow()

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "backup_run_now", "backup", "backup", "manual", "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "triggered"})
}

func (h *BackupHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepoURL    string `json:"repo_url"`
		AuthToken  string `json:"auth_token"`
		AuthMethod string `json:"auth_method"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "repo_url is required")
		return
	}

	// Import backup package function via a simple test
	// We'll test by doing a shallow clone
	if err := testBackupConnection(req.RepoURL, req.AuthToken, req.AuthMethod); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *BackupHandler) ListHistory(w http.ResponseWriter, r *http.Request) {
	limit := 20
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	var total int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM backup_executions").Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count executions")
		return
	}

	rows, err := h.db.Query(
		`SELECT id, status, files_count, commit_sha, error, started_at, finished_at
		 FROM backup_executions ORDER BY started_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list backup history")
		return
	}
	defer rows.Close()

	type execution struct {
		ID         string     `json:"id"`
		Status     string     `json:"status"`
		FilesCount int        `json:"files_count"`
		CommitSHA  string     `json:"commit_sha"`
		Error      string     `json:"error"`
		StartedAt  time.Time  `json:"started_at"`
		FinishedAt *time.Time `json:"finished_at,omitempty"`
	}

	items := []execution{}
	for rows.Next() {
		var e execution
		if err := rows.Scan(&e.ID, &e.Status, &e.FilesCount, &e.CommitSHA, &e.Error, &e.StartedAt, &e.FinishedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan execution")
			return
		}
		items = append(items, e)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"total": total,
	})
}

func (h *BackupHandler) DetectGit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, detectGitMethodFromHandler())
}
