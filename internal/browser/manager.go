package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
)

type SessionStatus string

const (
	StatusIdle    SessionStatus = "idle"
	StatusActive  SessionStatus = "active"
	StatusBusy    SessionStatus = "busy"
	StatusHuman   SessionStatus = "human"
	StatusStopped SessionStatus = "stopped"
	StatusError   SessionStatus = "error"
)

type Session struct {
	ID             string
	Name           string
	Status         SessionStatus
	Headless       bool
	UserDataDir    string
	CurrentURL     string
	CurrentTitle   string
	OwnerAgentSlug string
	CreatedAt      time.Time
	UpdatedAt      time.Time

	browser        *rod.Browser
	page           *rod.Page
	lastScreenshot []byte
	screenshotStop chan struct{}
	mu             sync.Mutex
}

type BroadcastFunc func(msgType string, payload interface{})
type TopicBroadcastFunc func(topic, msgType string, payload interface{})

type Manager struct {
	sessions       map[string]*Session
	mu             sync.RWMutex
	db             *database.DB
	broadcast      BroadcastFunc
	topicBroadcast TopicBroadcastFunc
	dataDir        string
	sessionsDir    string
}

func NewManager(db *database.DB, dataDir string, broadcast BroadcastFunc) *Manager {
	sessionsDir := filepath.Join(dataDir, "browser_sessions")
	os.MkdirAll(sessionsDir, 0755)

	m := &Manager{
		sessions:    make(map[string]*Session),
		db:          db,
		broadcast:   broadcast,
		dataDir:     dataDir,
		sessionsDir: sessionsDir,
	}

	m.loadSessionsFromDB()
	return m
}

// SetTopicBroadcast sets the topic-filtered broadcast function for scoped messages like screenshots.
func (m *Manager) SetTopicBroadcast(fn TopicBroadcastFunc) {
	m.topicBroadcast = fn
}

func (m *Manager) loadSessionsFromDB() {
	rows, err := m.db.Query(
		"SELECT id, name, status, headless, user_data_dir, current_url, current_title, owner_agent_slug, created_at, updated_at FROM browser_sessions",
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		s := &Session{}
		var headless int
		if err := rows.Scan(&s.ID, &s.Name, &s.Status, &headless, &s.UserDataDir, &s.CurrentURL, &s.CurrentTitle, &s.OwnerAgentSlug, &s.CreatedAt, &s.UpdatedAt); err != nil {
			logger.Warn("scan browser session row: %v", err)
			continue
		}
		s.Headless = headless == 1
		if s.Status == StatusActive || s.Status == StatusBusy || s.Status == StatusHuman {
			s.Status = StatusStopped
			m.db.Exec("UPDATE browser_sessions SET status = 'stopped', updated_at = ? WHERE id = ?", time.Now().UTC(), s.ID)
		}
		m.sessions[s.ID] = s
	}
}

func (m *Manager) CreateSession(name string, headless bool, ownerAgentSlug string) (*Session, error) {
	id := uuid.New().String()
	userDataDir := filepath.Join(m.sessionsDir, id)
	if err := os.MkdirAll(userDataDir, 0755); err != nil {
		return nil, fmt.Errorf("create user data dir: %w", err)
	}

	now := time.Now().UTC()
	s := &Session{
		ID:             id,
		Name:           name,
		Status:         StatusStopped,
		Headless:       headless,
		UserDataDir:    userDataDir,
		OwnerAgentSlug: ownerAgentSlug,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	headlessInt := 0
	if headless {
		headlessInt = 1
	}

	_, err := m.db.Exec(
		"INSERT INTO browser_sessions (id, name, status, headless, user_data_dir, owner_agent_slug, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		s.ID, s.Name, string(s.Status), headlessInt, s.UserDataDir, s.OwnerAgentSlug, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}

	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()

	logger.Success("Created browser session %s (%s)", s.ID, s.Name)
	return s, nil
}

func (m *Manager) StartSession(id string) error {
	m.mu.RLock()
	s, exists := m.sessions[id]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("session not found: %s", id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.browser != nil {
		return fmt.Errorf("session already running")
	}

	l := launcher.New().
		UserDataDir(s.UserDataDir).
		Headless(s.Headless).
		Set("disable-gpu").
		Set("no-sandbox").
		Set("window-size", "1280,900")

	controlURL, err := l.Launch()
	if err != nil {
		s.Status = StatusError
		m.updateSessionStatus(s)
		return fmt.Errorf("launch browser: %w", err)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		s.Status = StatusError
		m.updateSessionStatus(s)
		return fmt.Errorf("connect browser: %w", err)
	}

	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		browser.Close()
		s.Status = StatusError
		m.updateSessionStatus(s)
		return fmt.Errorf("create page: %w", err)
	}

	// Set a fixed viewport to prevent resize jitter from screenshots.
	page.MustSetViewport(1280, 900, 1, false)

	s.browser = browser
	s.page = page
	s.Status = StatusActive
	m.updateSessionStatus(s)

	s.screenshotStop = make(chan struct{})
	go m.screenshotLoop(s)

	logger.Success("Started browser session %s", id)
	return nil
}

func (m *Manager) StopSession(id string) error {
	m.mu.RLock()
	s, exists := m.sessions[id]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("session not found: %s", id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	m.stopSessionLocked(s)
	return nil
}

func (m *Manager) stopSessionLocked(s *Session) {
	if s.screenshotStop != nil {
		close(s.screenshotStop)
		s.screenshotStop = nil
	}

	if s.browser != nil {
		s.browser.Close()
		s.browser = nil
		s.page = nil
	}

	s.Status = StatusStopped
	s.lastScreenshot = nil
	m.updateSessionStatus(s)
	logger.Info("Stopped browser session %s", s.ID)
}

func (m *Manager) DeleteSession(id string) error {
	m.mu.Lock()
	s, exists := m.sessions[id]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("session not found: %s", id)
	}
	delete(m.sessions, id)
	m.mu.Unlock()

	s.mu.Lock()
	m.stopSessionLocked(s)
	s.mu.Unlock()

	os.RemoveAll(s.UserDataDir)
	m.db.Exec("DELETE FROM browser_sessions WHERE id = ?", id)

	logger.Info("Deleted browser session %s", id)
	return nil
}

func (m *Manager) GetSession(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, exists := m.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return s, nil
}

func (m *Manager) ListSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

func (m *Manager) TakeHumanControl(id string) error {
	m.mu.RLock()
	s, exists := m.sessions[id]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("session not found: %s", id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.browser == nil {
		return fmt.Errorf("session not running")
	}

	s.Status = StatusHuman
	m.updateSessionStatus(s)
	return nil
}

func (m *Manager) ReleaseHumanControl(id string) error {
	m.mu.RLock()
	s, exists := m.sessions[id]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("session not found: %s", id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status != StatusHuman {
		return fmt.Errorf("session not under human control")
	}

	s.Status = StatusActive
	m.updateSessionStatus(s)
	return nil
}

func (m *Manager) GetLastScreenshot(id string) ([]byte, error) {
	m.mu.RLock()
	s, exists := m.sessions[id]
	m.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("session not found: %s", id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastScreenshot, nil
}

func (m *Manager) updateSessionStatus(s *Session) {
	now := time.Now().UTC()
	s.UpdatedAt = now
	m.db.Exec(
		"UPDATE browser_sessions SET status = ?, current_url = ?, current_title = ?, updated_at = ? WHERE id = ?",
		string(s.Status), s.CurrentURL, s.CurrentTitle, now, s.ID,
	)
	m.broadcast("browser_status", map[string]interface{}{
		"session_id":    s.ID,
		"status":        string(s.Status),
		"current_url":   s.CurrentURL,
		"current_title": s.CurrentTitle,
	})
}

// ExecuteScheduledTask starts a session if needed and sends instructions to an agent.
// This satisfies the scheduler.BrowserExecutor interface.
func (m *Manager) ExecuteScheduledTask(ctx context.Context, sessionID, instructions, agentSlug string) (string, error) {
	_, err := m.GetSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("browser session not found: %w", err)
	}

	// Start session if not running
	m.mu.RLock()
	s := m.sessions[sessionID]
	m.mu.RUnlock()

	s.mu.Lock()
	isRunning := s.browser != nil
	s.mu.Unlock()

	if !isRunning {
		if err := m.StartSession(sessionID); err != nil {
			return "", fmt.Errorf("failed to start session: %w", err)
		}
	}

	return fmt.Sprintf("Browser task queued for session %s with instructions: %s (agent: %s)", sessionID, instructions, agentSlug), nil
}

func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, s := range m.sessions {
		s.mu.Lock()
		m.stopSessionLocked(s)
		s.mu.Unlock()
	}
	logger.Info("Browser manager shut down")
}
