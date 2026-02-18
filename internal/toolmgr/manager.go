package toolmgr

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/toollibrary"
)

type BroadcastFunc func(msgType string, payload interface{})

type RunningTool struct {
	ToolID    string
	Port      int
	PID       int
	Status    string // "starting", "running", "stopped", "error"
	Cmd       *exec.Cmd
	Cancel    context.CancelFunc
	Dir       string
	Error     string
	StartedAt time.Time
	Restarts  int
	logFile   *os.File
}

type Manager struct {
	db          *database.DB
	toolsDir    string
	tools       map[string]*RunningTool
	mu          sync.RWMutex
	nextPort    int
	broadcast   BroadcastFunc
	ctx         context.Context
	cancel      context.CancelFunc
	httpClient  *http.Client
	healthClient *http.Client
}

func New(db *database.DB, toolsDir string, broadcast BroadcastFunc) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		db:           db,
		toolsDir:     toolsDir,
		tools:        make(map[string]*RunningTool),
		nextPort:     9100,
		broadcast:    broadcast,
		ctx:          ctx,
		cancel:       cancel,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		healthClient: &http.Client{Timeout: 2 * time.Second},
	}
}

func (m *Manager) Start() {
	rows, err := m.db.Query(
		"SELECT id FROM tools WHERE enabled = 1 AND deleted_at IS NULL",
	)
	if err != nil {
		logger.Error("Failed to query tools for startup: %v", err)
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			logger.Warn("scan tool id row: %v", err)
			continue
		}
		ids = append(ids, id)
	}

	for _, id := range ids {
		toolDir := filepath.Join(m.toolsDir, id)
		binaryPath := filepath.Join(toolDir, "tool")
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			if err := m.CompileTool(id); err != nil {
				logger.Error("Failed to compile tool %s on startup: %v", id, err)
				continue
			}
		}
		if err := m.StartTool(id); err != nil {
			logger.Error("Failed to start tool %s on startup: %v", id, err)
		}
	}
}

func (m *Manager) Shutdown() {
	m.cancel()

	m.mu.Lock()
	toolsCopy := make(map[string]*RunningTool)
	for k, v := range m.tools {
		toolsCopy[k] = v
	}
	m.mu.Unlock()

	for id, rt := range toolsCopy {
		if rt.Cmd != nil && rt.Cmd.Process != nil {
			rt.Cmd.Process.Signal(os.Interrupt)
			done := make(chan error, 1)
			go func() { done <- rt.Cmd.Wait() }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				rt.Cmd.Process.Kill()
			}
		}
		closeLogFile(rt)
		m.updateToolDB(id, "stopped", 0, 0)
		logger.Info("Stopped tool %s", id)
	}

	m.mu.Lock()
	m.tools = make(map[string]*RunningTool)
	m.mu.Unlock()
}

func (m *Manager) CompileTool(toolID string) error {
	toolDir := filepath.Join(m.toolsDir, toolID)
	if _, err := os.Stat(filepath.Join(toolDir, "main.go")); os.IsNotExist(err) {
		return fmt.Errorf("no main.go found in tool directory")
	}

	m.updateToolStatus(toolID, "compiling")

	// Ensure go.sum is up to date before building
	if _, err := os.Stat(filepath.Join(toolDir, "go.mod")); err == nil {
		tidy := exec.Command("go", "mod", "tidy")
		tidy.Dir = toolDir
		tidy.Env = append(filterEnv(os.Environ()), "CGO_ENABLED=0")
		var tidyErr bytes.Buffer
		tidy.Stderr = &tidyErr
		if err := tidy.Run(); err != nil {
			m.setToolError(toolID, fmt.Sprintf("go mod tidy failed: %s", tidyErr.String()))
			return fmt.Errorf("go mod tidy failed: %s", tidyErr.String())
		}
	}

	cmd := exec.Command("go", "build", "-o", "tool", ".")
	cmd.Dir = toolDir
	cmd.Env = append(filterEnv(os.Environ()), "CGO_ENABLED=0")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		m.setToolError(toolID, fmt.Sprintf("compile failed: %s", errMsg))
		return fmt.Errorf("compile failed: %s", errMsg)
	}

	now := time.Now().UTC()
	m.db.Exec("UPDATE tools SET status = 'active', updated_at = ? WHERE id = ?", now, toolID)
	m.broadcast("tool_status", map[string]interface{}{
		"tool_id": toolID,
		"status":  "active",
	})

	if err := toollibrary.RecordIntegrity(m.db, toolID, toolDir); err != nil {
		logger.Warn("Failed to record integrity for tool %s: %v", toolID, err)
	}

	logger.Success("Compiled tool %s", toolID)
	return nil
}

func (m *Manager) StartTool(toolID string) error {
	m.mu.Lock()
	if rt, exists := m.tools[toolID]; exists && rt.Status == "running" {
		m.mu.Unlock()
		return fmt.Errorf("tool already running on port %d", rt.Port)
	}
	m.mu.Unlock()

	toolDir := filepath.Join(m.toolsDir, toolID)
	binaryPath := filepath.Join(toolDir, "tool")
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("tool binary not found, compile first")
	}

	port := m.allocatePort()

	ctx, cancel := context.WithCancel(m.ctx)
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Dir = toolDir
	cmd.Env = append(filterEnv(os.Environ()), fmt.Sprintf("PORT=%d", port))

	logFile, err := os.OpenFile(
		filepath.Join(toolDir, "tool.log"),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644,
	)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	rt := &RunningTool{
		ToolID:    toolID,
		Port:      port,
		Status:    "starting",
		Cmd:       cmd,
		Cancel:    cancel,
		Dir:       toolDir,
		StartedAt: time.Now(),
		logFile:   logFile,
	}

	m.mu.Lock()
	m.tools[toolID] = rt
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		cancel()
		m.mu.Lock()
		delete(m.tools, toolID)
		m.mu.Unlock()
		m.setToolError(toolID, fmt.Sprintf("start failed: %v", err))
		return fmt.Errorf("failed to start tool: %w", err)
	}

	rt.PID = cmd.Process.Pid
	m.updateToolDB(toolID, "running", port, rt.PID)

	m.broadcast("tool_status", map[string]interface{}{
		"tool_id": toolID,
		"status":  "starting",
		"port":    port,
		"pid":     rt.PID,
	})

	// Wait for health check
	go func() {
		if err := m.WaitForHealth(toolID, 10*time.Second); err != nil {
			logger.Warn("Tool %s health check failed: %v", toolID, err)
			rt.Status = "running" // still running, just health check didn't pass yet
		} else {
			rt.Status = "running"
			m.broadcast("tool_status", map[string]interface{}{
				"tool_id": toolID,
				"status":  "running",
				"port":    port,
				"pid":     rt.PID,
			})
			logger.Success("Tool %s running on port %d (pid %d)", toolID, port, rt.PID)
		}
	}()

	// Monitor process
	go m.monitorProcess(toolID)

	return nil
}

func (m *Manager) StopTool(toolID string) error {
	m.mu.Lock()
	rt, exists := m.tools[toolID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("tool not running")
	}
	delete(m.tools, toolID)
	m.mu.Unlock()

	if rt.Cancel != nil {
		rt.Cancel()
	}
	if rt.Cmd != nil && rt.Cmd.Process != nil {
		rt.Cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- rt.Cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			rt.Cmd.Process.Kill()
		}
	}
	closeLogFile(rt)

	m.updateToolDB(toolID, "stopped", 0, 0)
	m.broadcast("tool_status", map[string]interface{}{
		"tool_id": toolID,
		"status":  "stopped",
	})

	logger.Info("Stopped tool %s", toolID)
	return nil
}

func (m *Manager) RestartTool(toolID string) error {
	_ = m.StopTool(toolID)
	time.Sleep(500 * time.Millisecond)
	return m.StartTool(toolID)
}

func (m *Manager) WaitForHealth(toolID string, timeout time.Duration) error {
	m.mu.RLock()
	rt, exists := m.tools[toolID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("tool not running")
	}

	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/health", rt.Port)

	for time.Now().Before(deadline) {
		resp, err := m.healthClient.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("health check timed out after %v", timeout)
}

func (m *Manager) CallTool(toolID, endpoint string, payload []byte) ([]byte, error) {
	m.mu.RLock()
	rt, exists := m.tools[toolID]
	m.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("tool %s not running", toolID)
	}
	if rt.Status != "running" {
		return nil, fmt.Errorf("tool %s status is %s, not running", toolID, rt.Status)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d%s", rt.Port, endpoint)

	var body io.Reader
	method := "GET"
	if payload != nil && len(payload) > 0 {
		body = bytes.NewReader(payload)
		method = "POST"
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call tool: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return respBody, fmt.Errorf("tool returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (m *Manager) GetStatus(toolID string) map[string]interface{} {
	m.mu.RLock()
	rt, exists := m.tools[toolID]
	m.mu.RUnlock()

	if !exists {
		return map[string]interface{}{
			"tool_id": toolID,
			"status":  "stopped",
		}
	}

	return map[string]interface{}{
		"tool_id":    toolID,
		"status":     rt.Status,
		"port":       rt.Port,
		"pid":        rt.PID,
		"started_at": rt.StartedAt,
		"restarts":   rt.Restarts,
		"error":      rt.Error,
	}
}

func (m *Manager) allocatePort() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	port := m.nextPort
	m.nextPort++
	return port
}

func (m *Manager) monitorProcess(toolID string) {
	m.mu.RLock()
	rt, exists := m.tools[toolID]
	m.mu.RUnlock()
	if !exists || rt.Cmd == nil {
		return
	}

	err := rt.Cmd.Wait()

	// Check if we're shutting down
	select {
	case <-m.ctx.Done():
		return
	default:
	}

	m.mu.Lock()
	current, stillExists := m.tools[toolID]
	if !stillExists || current != rt {
		m.mu.Unlock()
		return
	}
	delete(m.tools, toolID)
	m.mu.Unlock()

	closeLogFile(rt)

	if err != nil {
		logger.Warn("Tool %s exited unexpectedly: %v", toolID, err)
	} else {
		logger.Warn("Tool %s exited with code 0", toolID)
	}

	m.broadcast("tool_status", map[string]interface{}{
		"tool_id": toolID,
		"status":  "crashed",
	})

	// Restart with backoff (max 5 retries)
	maxRetries := 5
	backoffs := []time.Duration{5 * time.Second, 15 * time.Second, 45 * time.Second, 2 * time.Minute, 5 * time.Minute}

	restartCount := rt.Restarts
	if restartCount >= maxRetries {
		m.setToolError(toolID, fmt.Sprintf("exceeded max restarts (%d)", maxRetries))
		logger.Error("Tool %s exceeded max restarts, giving up", toolID)
		return
	}

	backoff := backoffs[restartCount]
	logger.Info("Restarting tool %s in %v (attempt %d/%d)", toolID, backoff, restartCount+1, maxRetries)

	select {
	case <-time.After(backoff):
	case <-m.ctx.Done():
		return
	}

	if err := m.StartTool(toolID); err != nil {
		logger.Error("Failed to restart tool %s: %v", toolID, err)
		m.setToolError(toolID, fmt.Sprintf("restart failed: %v", err))
		return
	}

	m.mu.Lock()
	if newRt, exists := m.tools[toolID]; exists {
		newRt.Restarts = restartCount + 1
	}
	m.mu.Unlock()
}


func closeLogFile(rt *RunningTool) {
	if rt.logFile != nil {
		rt.logFile.Close()
		rt.logFile = nil
	}
}

func (m *Manager) updateToolDB(toolID, status string, port, pid int) {
	now := time.Now().UTC()
	m.db.Exec("UPDATE tools SET status = ?, port = ?, pid = ?, updated_at = ? WHERE id = ?",
		status, port, pid, now, toolID)
}

func (m *Manager) updateToolStatus(toolID, status string) {
	now := time.Now().UTC()
	m.db.Exec("UPDATE tools SET status = ?, updated_at = ? WHERE id = ?", status, now, toolID)
	m.broadcast("tool_status", map[string]interface{}{
		"tool_id": toolID,
		"status":  status,
	})
}

var sensitiveEnvPrefixes = []string{
	"AWS_SECRET",
	"AWS_SESSION",
	"GOOGLE_APPLICATION_CREDENTIALS",
	"GCLOUD_",
	"AZURE_",
	"OPENAI_API",
	"ANTHROPIC_API",
	"OPENROUTER_API",
	"OPENPAW_JWT",
	"OPENPAW_ENCRYPTION",
	"SSH_",
	"GPG_",
}

var sensitiveEnvExact = []string{
	"AWS_ACCESS_KEY_ID",
	"DATABASE_URL",
	"DB_PASSWORD",
	"GITHUB_TOKEN",
	"GH_TOKEN",
	"GITLAB_TOKEN",
	"NPM_TOKEN",
	"DOCKER_PASSWORD",
}

func filterEnv(env []string) []string {
	var filtered []string
	for _, e := range env {
		key := e
		if idx := strings.Index(e, "="); idx >= 0 {
			key = e[:idx]
		}
		upper := strings.ToUpper(key)
		skip := false
		for _, prefix := range sensitiveEnvPrefixes {
			if strings.HasPrefix(upper, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			for _, exact := range sensitiveEnvExact {
				if upper == exact {
					skip = true
					break
				}
			}
		}
		if !skip {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func (m *Manager) setToolError(toolID, errMsg string) {
	now := time.Now().UTC()
	m.db.Exec("UPDATE tools SET status = 'error', updated_at = ? WHERE id = ?", now, toolID)
	logger.Error("Tool %s: %s", toolID, errMsg)

	m.broadcast("tool_status", map[string]interface{}{
		"tool_id": toolID,
		"status":  "error",
		"error":   errMsg,
	})
}
