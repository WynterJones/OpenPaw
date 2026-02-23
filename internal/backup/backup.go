package backup

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/secrets"
)

type BroadcastFunc func(msgType string, payload interface{})

type Config struct {
	Enabled    bool   `json:"enabled"`
	RepoURL    string `json:"repo_url"`
	AuthToken  string `json:"auth_token"`
	AuthMethod string `json:"auth_method"`
	Interval   string `json:"interval"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:    false,
		RepoURL:    "",
		AuthToken:  "",
		AuthMethod: "token",
		Interval:   "manual",
	}
}

type Manager struct {
	db         *database.DB
	secretsMgr *secrets.Manager
	dataDir    string
	broadcast  BroadcastFunc

	config  Config
	mu      sync.RWMutex
	running atomic.Bool

	stopCh  chan struct{}
	stopped chan struct{}
	started bool
}

func New(db *database.DB, secretsMgr *secrets.Manager, dataDir string, broadcast BroadcastFunc) *Manager {
	return &Manager{
		db:         db,
		secretsMgr: secretsMgr,
		dataDir:    dataDir,
		broadcast:  broadcast,
		config:     DefaultConfig(),
		stopCh:     make(chan struct{}),
		stopped:    make(chan struct{}),
	}
}

func (m *Manager) LoadConfig() {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := DefaultConfig()

	rows, err := m.db.Query(
		"SELECT key, value FROM settings WHERE key IN ('backup_enabled', 'backup_repo_url', 'backup_auth_token', 'backup_auth_method', 'backup_interval')",
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var key, val string
			if rows.Scan(&key, &val) != nil || val == "" {
				continue
			}
			switch key {
			case "backup_enabled":
				cfg.Enabled = val == "true" || val == "1"
			case "backup_repo_url":
				cfg.RepoURL = val
			case "backup_auth_token":
				if decrypted, err := m.secretsMgr.Decrypt(val); err == nil {
					cfg.AuthToken = decrypted
				}
			case "backup_auth_method":
				cfg.AuthMethod = val
			case "backup_interval":
				cfg.Interval = val
			}
		}
	}

	m.config = cfg
}

func (m *Manager) GetConfig() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := map[string]interface{}{
		"enabled":          m.config.Enabled,
		"repo_url":         m.config.RepoURL,
		"auth_method":      m.config.AuthMethod,
		"interval":         m.config.Interval,
		"token_configured": m.config.AuthToken != "",
		"running":          m.running.Load(),
	}

	// Load last backup status from settings
	var lastStatus, lastAt, lastError, lastSHA string
	m.db.QueryRow("SELECT value FROM settings WHERE key = 'backup_last_status'").Scan(&lastStatus)
	m.db.QueryRow("SELECT value FROM settings WHERE key = 'backup_last_at'").Scan(&lastAt)
	m.db.QueryRow("SELECT value FROM settings WHERE key = 'backup_last_error'").Scan(&lastError)
	m.db.QueryRow("SELECT value FROM settings WHERE key = 'backup_last_sha'").Scan(&lastSHA)

	result["last_status"] = lastStatus
	result["last_at"] = lastAt
	result["last_error"] = lastError
	result["last_sha"] = lastSHA

	return result
}

func (m *Manager) UpdateConfig(cfg map[string]string) error {
	for key, val := range cfg {
		dbKey := key
		dbVal := val

		// Encrypt token before storing
		if key == "backup_auth_token" && val != "" {
			encrypted, err := m.secretsMgr.Encrypt(val)
			if err != nil {
				return fmt.Errorf("encrypt token: %w", err)
			}
			dbVal = encrypted
		}

		m.db.Exec(
			"INSERT INTO settings (id, key, value) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
			"bk-"+dbKey, dbKey, dbVal, dbVal,
		)
	}

	m.LoadConfig()

	m.mu.RLock()
	enabled := m.config.Enabled
	interval := m.config.Interval
	m.mu.RUnlock()

	if m.started {
		m.Stop()
		if enabled && interval != "manual" {
			m.Start()
		}
	} else if enabled && interval != "manual" {
		m.Start()
	}

	return nil
}

func (m *Manager) Start() {
	m.mu.RLock()
	enabled := m.config.Enabled
	interval := m.config.Interval
	m.mu.RUnlock()

	if !enabled || interval == "manual" {
		logger.Info("Backup: not starting (enabled=%v, interval=%s)", enabled, interval)
		return
	}

	m.stopCh = make(chan struct{})
	m.stopped = make(chan struct{})
	m.started = true

	go m.tickLoop()
	logger.Success("Backup manager started (interval: %s)", interval)
}

func (m *Manager) Stop() {
	if !m.started {
		return
	}
	close(m.stopCh)
	<-m.stopped
	m.started = false
	logger.Info("Backup manager stopped")
}

func (m *Manager) IsRunning() bool {
	return m.running.Load()
}

func (m *Manager) RunNow() {
	m.runBackup()
}

func (m *Manager) tickLoop() {
	defer close(m.stopped)

	for {
		m.mu.RLock()
		interval := intervalDuration(m.config.Interval)
		m.mu.RUnlock()

		if interval == 0 {
			return
		}

		// Check if we should skip based on last run
		if m.shouldSkip(interval) {
			remaining := m.timeUntilNext(interval)
			if remaining > 0 {
				select {
				case <-m.stopCh:
					return
				case <-time.After(remaining):
					m.runBackup()
				}
				continue
			}
		}

		select {
		case <-m.stopCh:
			return
		case <-time.After(interval):
			m.runBackup()
		}
	}
}

func (m *Manager) shouldSkip(interval time.Duration) bool {
	var lastAt string
	m.db.QueryRow("SELECT value FROM settings WHERE key = 'backup_last_at'").Scan(&lastAt)
	if lastAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, lastAt)
	if err != nil {
		return false
	}
	return time.Since(t) < interval
}

func (m *Manager) timeUntilNext(interval time.Duration) time.Duration {
	var lastAt string
	m.db.QueryRow("SELECT value FROM settings WHERE key = 'backup_last_at'").Scan(&lastAt)
	if lastAt == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, lastAt)
	if err != nil {
		return 0
	}
	next := t.Add(interval)
	remaining := time.Until(next)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (m *Manager) runBackup() {
	if !m.running.CompareAndSwap(false, true) {
		logger.Warn("Backup skipped — previous backup still running")
		return
	}
	defer func() {
		m.running.Store(false)
		m.broadcast("backup_done", map[string]interface{}{})
	}()

	m.mu.RLock()
	cfg := m.config
	m.mu.RUnlock()

	execID := uuid.New().String()
	now := currentTime()

	m.db.Exec(
		"INSERT INTO backup_executions (id, status, started_at) VALUES (?, 'running', ?)",
		execID, now,
	)

	m.broadcast("backup_started", map[string]interface{}{})

	// Create temp working directory
	workDir, err := os.MkdirTemp("", "openpaw-backup-*")
	if err != nil {
		m.finishBackup(execID, "failed", 0, "", "create temp dir: "+err.Error())
		return
	}
	defer os.RemoveAll(workDir)

	// Clone or init repo
	if err := cloneOrInit(cfg.RepoURL, cfg.AuthToken, cfg.AuthMethod, workDir); err != nil {
		m.finishBackup(execID, "failed", 0, "", "clone repo: "+err.Error())
		return
	}

	// Remove old backup files (keep .git)
	if err := removeOldFiles(workDir); err != nil {
		m.finishBackup(execID, "failed", 0, "", "clean repo: "+err.Error())
		return
	}

	// Export data
	filesCount, err := exportData(m.db, m.dataDir, workDir)
	if err != nil {
		m.finishBackup(execID, "failed", 0, "", "export data: "+err.Error())
		return
	}

	// Commit and push
	sha, err := commitAndPush(workDir)
	if err != nil {
		m.finishBackup(execID, "failed", filesCount, sha, "push: "+err.Error())
		return
	}

	m.finishBackup(execID, "success", filesCount, sha, "")
	logger.Success("Backup completed: %d files, SHA %s", filesCount, sha)
}

func (m *Manager) finishBackup(execID, status string, filesCount int, sha, errMsg string) {
	now := currentTime()

	m.db.Exec(
		"UPDATE backup_executions SET status = ?, files_count = ?, commit_sha = ?, error = ?, finished_at = ? WHERE id = ?",
		status, filesCount, sha, errMsg, now, execID,
	)

	// Update last-run settings
	m.setSetting("backup_last_status", status)
	m.setSetting("backup_last_at", now.Format(time.RFC3339))
	if sha != "" {
		m.setSetting("backup_last_sha", sha)
	}
	if errMsg != "" {
		m.setSetting("backup_last_error", errMsg)
	} else {
		m.setSetting("backup_last_error", "")
	}

	m.db.LogAudit("system", "backup_executed", "backup", "backup", execID,
		fmt.Sprintf("status=%s files=%d sha=%s", status, filesCount, sha))
}

func (m *Manager) setSetting(key, value string) {
	m.db.Exec(
		"INSERT INTO settings (id, key, value) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
		"bk-"+key, key, value, value,
	)
}

func intervalDuration(interval string) time.Duration {
	switch interval {
	case "daily":
		return 24 * time.Hour
	case "weekly":
		return 7 * 24 * time.Hour
	default:
		return 0
	}
}

// currentTime returns current UTC time — extracted for testability.
var currentTime = func() time.Time {
	return time.Now().UTC()
}
