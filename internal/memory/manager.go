package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/logger"
)

type Manager struct {
	dataDir string
	dbs     sync.Map // map[slug]*sql.DB
}

func NewManager(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

func (m *Manager) dbPath(slug string) string {
	if slug == "gateway" {
		return filepath.Join(m.dataDir, "gateway", "memory.db")
	}
	return filepath.Join(m.dataDir, "agents", slug, "memory.db")
}

func (m *Manager) GetDB(slug string) (*sql.DB, error) {
	if v, ok := m.dbs.Load(slug); ok {
		return v.(*sql.DB), nil
	}

	dbPath := m.dbPath(slug)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=3000&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("open memory db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := applySchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply memory schema for %s: %w", slug, err)
	}

	actual, _ := m.dbs.LoadOrStore(slug, db)
	if actual.(*sql.DB) != db {
		db.Close()
	}
	return actual.(*sql.DB), nil
}

func (m *Manager) Close() {
	m.dbs.Range(func(key, value interface{}) bool {
		if db, ok := value.(*sql.DB); ok {
			db.Close()
		}
		m.dbs.Delete(key)
		return true
	})
}

// RunAutoArchival archives old, low-importance, rarely-accessed memories.
func (m *Manager) RunAutoArchival(slug string) (int64, error) {
	db, err := m.GetDB(slug)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().AddDate(0, 0, -90).UTC().Format("2006-01-02 15:04:05")
	result, err := db.Exec(
		`UPDATE memories SET archived = 1, updated_at = CURRENT_TIMESTAMP
		 WHERE archived = 0 AND importance <= 3 AND access_count < 3
		   AND created_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}

	// Clean up expired memories
	db.Exec("DELETE FROM memories WHERE expires_at IS NOT NULL AND expires_at < CURRENT_TIMESTAMP")

	return result.RowsAffected()
}

// SaveNote saves a simple text note as a memory (used by gateway and other callers).
func (m *Manager) SaveNote(slug, note, source string) error {
	if strings.TrimSpace(note) == "" {
		return nil
	}
	db, err := m.GetDB(slug)
	if err != nil {
		return err
	}

	id := uuid.New().String()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	summary := note
	if len(summary) > 100 {
		summary = summary[:100] + "..."
	}
	_, err = db.Exec(
		`INSERT INTO memories (id, content, summary, category, importance, source, tags, created_at, updated_at)
		 VALUES (?, ?, ?, 'general', 5, ?, '', ?, ?)`,
		id, note, summary, source, now, now,
	)
	return err
}

// EnsureMigrated checks if migration from file-based memory is needed and runs it.
func (m *Manager) EnsureMigrated(slug string) {
	db, err := m.GetDB(slug)
	if err != nil {
		logger.Error("Memory DB init for %s: %v", slug, err)
		return
	}

	// Check if we already have memories (migration done)
	var count int
	db.QueryRow("SELECT COUNT(*) FROM memories").Scan(&count)
	if count > 0 {
		return
	}

	// Try migrating from file-based memory
	if err := MigrateFileMemoryToDB(m, slug); err != nil {
		logger.Warn("Memory migration for %s: %v", slug, err)
	}
}
