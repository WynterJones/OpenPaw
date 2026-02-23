package terminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
)

// Session represents a running PTY terminal session.
type Session struct {
	ID          string
	Title       string
	Shell       string
	Cols        uint16
	Rows        uint16
	Color       string
	WorkbenchID string
	cmd         *exec.Cmd
	Ptmx        *os.File
	cancel      context.CancelFunc
	CreatedAt   time.Time
}

// Workbench represents a named grouping of terminal sessions.
type Workbench struct {
	ID        string
	Name      string
	Color     string
	SortOrder int
	CreatedAt time.Time
}

// Manager manages PTY terminal sessions.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	db       *database.DB
	workDir  string
}

// NewManager creates a new terminal session manager. It cleans up any stale
// sessions left in the database from a previous run.
func NewManager(db *database.DB, workDir string) *Manager {
	m := &Manager{
		sessions: make(map[string]*Session),
		db:       db,
		workDir:  workDir,
	}

	// Clean stale DB rows from previous runs
	_, err := m.db.Exec("DELETE FROM terminal_sessions")
	if err != nil {
		logger.Warn("Failed to clean stale terminal sessions: %v", err)
	}

	return m
}

// detectShell returns the user's preferred shell, falling back to /bin/bash
// then /bin/sh.
func detectShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	if _, err := os.Stat("/bin/bash"); err == nil {
		return "/bin/bash"
	}
	return "/bin/sh"
}

// CreateSession spawns a new PTY session with the given title, dimensions, color, and workbench.
func (m *Manager) CreateSession(title string, cols, rows uint16, color, workbenchID string) (*Session, error) {
	id := uuid.New().String()
	shell := detectShell()
	now := time.Now().UTC()

	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, shell)
	cmd.Dir = m.workDir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("start pty: %w", err)
	}

	s := &Session{
		ID:          id,
		Title:       title,
		Shell:       shell,
		Cols:        cols,
		Rows:        rows,
		Color:       color,
		WorkbenchID: workbenchID,
		cmd:         cmd,
		Ptmx:        ptmx,
		cancel:      cancel,
		CreatedAt:   now,
	}

	// Save to database
	_, err = m.db.Exec(
		"INSERT INTO terminal_sessions (id, title, shell, cols, rows, color, workbench_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		s.ID, s.Title, s.Shell, s.Cols, s.Rows, s.Color, s.WorkbenchID, s.CreatedAt,
	)
	if err != nil {
		// Clean up on DB failure
		cancel()
		ptmx.Close()
		cmd.Process.Kill()
		return nil, fmt.Errorf("insert session: %w", err)
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()

	// Watch for process exit to clean up
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if _, exists := m.sessions[id]; exists {
			delete(m.sessions, id)
			m.db.Exec("DELETE FROM terminal_sessions WHERE id = ?", id)
			logger.Info("Terminal session %s exited naturally", id)
		}
		m.mu.Unlock()
	}()

	logger.Success("Created terminal session %s (%s) using %s", id, title, shell)
	return s, nil
}

// GetSession returns the session with the given ID, or nil if not found.
func (m *Manager) GetSession(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// ListSessions returns active sessions, optionally filtered by workbench.
func (m *Manager) ListSessions(workbenchID string) []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		if workbenchID != "" && s.WorkbenchID != workbenchID {
			continue
		}
		result = append(result, s)
	}
	return result
}

// UpdateSession updates the title and color of a session.
func (m *Manager) UpdateSession(id string, title, color string) error {
	m.mu.Lock()
	s, exists := m.sessions[id]
	m.mu.Unlock()
	if !exists {
		return fmt.Errorf("session not found: %s", id)
	}
	if title != "" {
		s.Title = title
	}
	s.Color = color
	m.db.Exec("UPDATE terminal_sessions SET title = ?, color = ? WHERE id = ?", s.Title, s.Color, id)
	return nil
}

// ResizeSession changes the PTY dimensions for a session.
func (m *Manager) ResizeSession(id string, cols, rows uint16) error {
	m.mu.Lock()
	s, exists := m.sessions[id]
	m.mu.Unlock()
	if !exists {
		return fmt.Errorf("session not found: %s", id)
	}

	if err := pty.Setsize(s.Ptmx, &pty.Winsize{Rows: rows, Cols: cols}); err != nil {
		return fmt.Errorf("resize pty: %w", err)
	}

	s.Cols = cols
	s.Rows = rows

	m.db.Exec("UPDATE terminal_sessions SET cols = ?, rows = ? WHERE id = ?", cols, rows, id)
	return nil
}

// DestroySession kills the process, closes the PTY, and removes the session.
func (m *Manager) DestroySession(id string) error {
	m.mu.Lock()
	s, exists := m.sessions[id]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("session not found: %s", id)
	}
	delete(m.sessions, id)
	m.mu.Unlock()

	// Cancel context to signal the process
	s.cancel()

	// Kill the process if still running
	if s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}

	// Close the PTY
	s.Ptmx.Close()

	// Remove from database
	m.db.Exec("DELETE FROM terminal_sessions WHERE id = ?", id)

	logger.Info("Destroyed terminal session %s", id)
	return nil
}

// Shutdown destroys all active sessions.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		m.DestroySession(id)
	}

	logger.Info("Terminal manager shut down")
}

// ListWorkbenches returns all workbenches ordered by sort_order then created_at.
func (m *Manager) ListWorkbenches() ([]Workbench, error) {
	rows, err := m.db.Query("SELECT id, name, color, sort_order, created_at FROM workbenches ORDER BY sort_order, created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Workbench
	for rows.Next() {
		var w Workbench
		if err := rows.Scan(&w.ID, &w.Name, &w.Color, &w.SortOrder, &w.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

// CreateWorkbench creates a new workbench with the given name.
func (m *Manager) CreateWorkbench(name string) (*Workbench, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := m.db.Exec("INSERT INTO workbenches (id, name, color, created_at) VALUES (?, ?, '', ?)", id, name, now)
	if err != nil {
		return nil, err
	}
	return &Workbench{ID: id, Name: name, Color: "", SortOrder: 0, CreatedAt: now}, nil
}

// UpdateWorkbench updates the name and color of a workbench.
func (m *Manager) UpdateWorkbench(id, name, color string) error {
	res, err := m.db.Exec("UPDATE workbenches SET name = ?, color = ? WHERE id = ?", name, color, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("workbench not found: %s", id)
	}
	return nil
}

// DeleteWorkbench destroys all sessions in the workbench and removes it.
func (m *Manager) DeleteWorkbench(id string) error {
	// Collect session IDs in this workbench
	m.mu.Lock()
	var toDestroy []string
	for sid, s := range m.sessions {
		if s.WorkbenchID == id {
			toDestroy = append(toDestroy, sid)
		}
	}
	m.mu.Unlock()

	for _, sid := range toDestroy {
		m.DestroySession(sid)
	}

	// Delete from DB
	m.db.Exec("DELETE FROM terminal_sessions WHERE workbench_id = ?", id)
	_, err := m.db.Exec("DELETE FROM workbenches WHERE id = ?", id)
	return err
}

// EnsureDefaultWorkbench returns the first workbench or creates one named "Default".
func (m *Manager) EnsureDefaultWorkbench() (*Workbench, error) {
	var count int
	err := m.db.QueryRow("SELECT COUNT(*) FROM workbenches").Scan(&count)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		var w Workbench
		err := m.db.QueryRow("SELECT id, name, color, sort_order, created_at FROM workbenches ORDER BY sort_order, created_at LIMIT 1").Scan(&w.ID, &w.Name, &w.Color, &w.SortOrder, &w.CreatedAt)
		if err != nil {
			return nil, err
		}
		return &w, nil
	}
	return m.CreateWorkbench("Default")
}
