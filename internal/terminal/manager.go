package terminal

import (
	"context"
	"database/sql"
	"errors"
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
	Cwd         string
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

// NewManager creates a new terminal session manager and recreates the shells
// represented by terminal rows saved during the previous shutdown.
func NewManager(db *database.DB, workDir string) *Manager {
	m := &Manager{
		sessions: make(map[string]*Session),
		db:       db,
		workDir:  workDir,
	}

	m.restoreSessions()

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

type sessionSpec struct {
	ID          string
	Title       string
	Shell       string
	Cwd         string
	Cols        uint16
	Rows        uint16
	Color       string
	WorkbenchID string
	CreatedAt   time.Time
}

// CreateSession spawns a new PTY session with the given title, dimensions, color, and workbench.
// Optional cwd overrides the working directory. Optional initialCommand is sent to the PTY after startup.
func (m *Manager) CreateSession(title string, cols, rows uint16, color, workbenchID, cwd, initialCommand string) (*Session, error) {
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	workDir := m.workDir
	if cwd != "" {
		workDir = cwd
	}

	spec := sessionSpec{
		ID:          uuid.New().String(),
		Title:       title,
		Shell:       detectShell(),
		Cwd:         workDir,
		Cols:        cols,
		Rows:        rows,
		Color:       color,
		WorkbenchID: workbenchID,
		CreatedAt:   time.Now().UTC(),
	}
	return m.startSession(spec, initialCommand, true)
}

// startSession starts one shell. New sessions are inserted into the database;
// restored sessions retain their existing row and stable id so the frontend's
// saved split/tab layout still points at the right terminal.
func (m *Manager) startSession(spec sessionSpec, initialCommand string, insert bool) (*Session, error) {
	if spec.Title == "" {
		spec.Title = "Terminal"
	}
	if spec.Shell == "" {
		spec.Shell = detectShell()
	}
	if _, err := os.Stat(spec.Shell); err != nil {
		spec.Shell = detectShell()
	}
	if spec.Cols == 0 {
		spec.Cols = 80
	}
	if spec.Rows == 0 {
		spec.Rows = 24
	}
	if spec.Cwd == "" {
		spec.Cwd = m.workDir
	}
	if info, err := os.Stat(spec.Cwd); err != nil || !info.IsDir() {
		spec.Cwd = m.workDir
	}

	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, spec.Shell)
	cmd.Dir = spec.Cwd
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: spec.Rows, Cols: spec.Cols})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("start pty: %w", err)
	}

	s := &Session{
		ID:          spec.ID,
		Title:       spec.Title,
		Shell:       spec.Shell,
		Cwd:         spec.Cwd,
		Cols:        spec.Cols,
		Rows:        spec.Rows,
		Color:       spec.Color,
		WorkbenchID: spec.WorkbenchID,
		cmd:         cmd,
		Ptmx:        ptmx,
		cancel:      cancel,
		CreatedAt:   spec.CreatedAt,
	}

	if insert {
		_, err = m.db.Exec(
			"INSERT INTO terminal_sessions (id, title, shell, cwd, cols, rows, color, workbench_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			s.ID, s.Title, s.Shell, s.Cwd, s.Cols, s.Rows, s.Color, s.WorkbenchID, s.CreatedAt,
		)
		if err != nil {
			cancel()
			ptmx.Close()
			cmd.Process.Kill()
			return nil, fmt.Errorf("insert session: %w", err)
		}
	} else {
		// Repair fallbacks (for example a directory that was moved while the
		// app was closed) so the next restart uses the working values.
		_, _ = m.db.Exec(
			"UPDATE terminal_sessions SET shell = ?, cwd = ?, cols = ?, rows = ? WHERE id = ?",
			s.Shell, s.Cwd, s.Cols, s.Rows, s.ID,
		)
	}

	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()

	// Watch for process exit to clean up
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if _, exists := m.sessions[s.ID]; exists {
			delete(m.sessions, s.ID)
			m.db.Exec("DELETE FROM terminal_sessions WHERE id = ?", s.ID)
			logger.Info("Terminal session %s exited naturally", s.ID)
		}
		m.mu.Unlock()
	}()

	// Send initial command to the PTY after a brief delay for shell startup
	if initialCommand != "" {
		go func() {
			time.Sleep(300 * time.Millisecond)
			m.mu.Lock()
			sess, exists := m.sessions[s.ID]
			m.mu.Unlock()
			if exists && sess.Ptmx != nil {
				sess.Ptmx.Write([]byte(initialCommand + "\n"))
			}
		}()
	}

	logger.Success("Created terminal session %s (%s) using %s", s.ID, s.Title, s.Shell)
	return s, nil
}

// restoreSessions recreates fresh shell processes for terminal tabs that were
// still open when OpenPaw last shut down. Commands are intentionally not
// replayed: restoration returns the user to a safe prompt in the same starting
// directory instead of repeating a potentially destructive action.
func (m *Manager) restoreSessions() {
	rows, err := m.db.Query(
		"SELECT id, title, shell, cwd, cols, rows, color, workbench_id, created_at FROM terminal_sessions ORDER BY created_at",
	)
	if err != nil {
		logger.Warn("Failed to load saved terminal sessions: %v", err)
		return
	}

	var saved []sessionSpec
	for rows.Next() {
		var spec sessionSpec
		if err := rows.Scan(
			&spec.ID,
			&spec.Title,
			&spec.Shell,
			&spec.Cwd,
			&spec.Cols,
			&spec.Rows,
			&spec.Color,
			&spec.WorkbenchID,
			&spec.CreatedAt,
		); err != nil {
			logger.Warn("Failed to read saved terminal session: %v", err)
			continue
		}
		saved = append(saved, spec)
	}
	if err := rows.Close(); err != nil {
		logger.Warn("Failed to close saved terminal query: %v", err)
	}
	if err := rows.Err(); err != nil {
		logger.Warn("Failed while loading saved terminal sessions: %v", err)
	}

	restored := 0
	for _, spec := range saved {
		if _, err := m.startSession(spec, "", false); err != nil {
			logger.Warn("Failed to restore terminal session %s: %v", spec.ID, err)
			_, _ = m.db.Exec("DELETE FROM terminal_sessions WHERE id = ?", spec.ID)
			continue
		}
		restored++
	}
	if restored > 0 {
		logger.Success("Restored %d terminal session(s)", restored)
	}
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

// Shutdown stops active PTY processes while retaining their database rows.
// Those rows represent open tabs and are recreated by NewManager next launch.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	// Remove them from the live map before killing the processes. Their watcher
	// goroutines will then know this is app shutdown, not a natural shell exit,
	// and will leave the saved rows alone.
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()

	for _, session := range sessions {
		session.cancel()
		if session.cmd.Process != nil {
			_ = session.cmd.Process.Kill()
		}
		_ = session.Ptmx.Close()
	}

	logger.Info("Terminal manager shut down with %d session(s) saved", len(sessions))
}

// ListWorkbenches returns all workbenches ordered by sort_order then created_at.
// ListWorkbenches returns the workbenches belonging to the active workspace.
//
// Workspace scoping is the point of the workspace: a client project's terminals
// have no business appearing while you are working on something else. The
// column has been on the table since 055 but nothing filtered on it, so every
// workspace showed the same terminals.
func (m *Manager) ListWorkbenches() ([]Workbench, error) {
	rows, err := m.db.Query(
		"SELECT id, name, color, sort_order, created_at FROM workbenches WHERE workspace_id = ? ORDER BY sort_order, created_at",
		m.db.ActiveWorkspaceID(),
	)
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

// CreateWorkbench creates a new workbench in the active workspace.
//
// The insert used to omit workspace_id and lean on the column default, which
// put every workbench in the Default workspace no matter where it was created.
func (m *Manager) CreateWorkbench(name string) (*Workbench, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := m.db.Exec(
		"INSERT INTO workbenches (id, name, color, workspace_id, created_at) VALUES (?, ?, '', ?, ?)",
		id, name, m.db.ActiveWorkspaceID(), now,
	)
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

// ReorderWorkbenches updates the sort_order of workbenches based on the given ID order.
func (m *Manager) ReorderWorkbenches(ids []string) error {
	for i, id := range ids {
		_, err := m.db.Exec("UPDATE workbenches SET sort_order = ? WHERE id = ?", i, id)
		if err != nil {
			return err
		}
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

// EnsureDefaultWorkbench returns the active workspace's first workbench, or
// creates one named "Default" for it.
func (m *Manager) EnsureDefaultWorkbench() (*Workbench, error) {
	var w Workbench
	err := m.db.QueryRow(
		"SELECT id, name, color, sort_order, created_at FROM workbenches WHERE workspace_id = ? ORDER BY sort_order, created_at LIMIT 1",
		m.db.ActiveWorkspaceID(),
	).Scan(&w.ID, &w.Name, &w.Color, &w.SortOrder, &w.CreatedAt)
	if err == nil {
		return &w, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return m.CreateWorkbench("Default")
}
