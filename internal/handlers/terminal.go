package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/openpaw/openpaw/internal/auth"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/terminal"
)

type sessionResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Shell       string `json:"shell"`
	Cols        int    `json:"cols"`
	Rows        int    `json:"rows"`
	Color       string `json:"color"`
	WorkbenchID string `json:"workbench_id"`
	CreatedAt   string `json:"created_at"`
}

type workbenchResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
	CreatedAt string `json:"created_at"`
}

// TerminalHandler handles terminal/PTY HTTP endpoints.
type TerminalHandler struct {
	db          *database.DB
	terminalMgr *terminal.Manager
	auth        *auth.Service
	port        int
	dataDir     string
}

// NewTerminalHandler creates a new TerminalHandler.
func NewTerminalHandler(db *database.DB, terminalMgr *terminal.Manager, authService *auth.Service, port int, dataDir string) *TerminalHandler {
	return &TerminalHandler{db: db, terminalMgr: terminalMgr, auth: authService, port: port, dataDir: dataDir}
}

func toSessionResponse(s *terminal.Session) sessionResponse {
	return sessionResponse{
		ID:          s.ID,
		Title:       s.Title,
		Shell:       s.Shell,
		Cols:        int(s.Cols),
		Rows:        int(s.Rows),
		Color:       s.Color,
		WorkbenchID: s.WorkbenchID,
		CreatedAt:   s.CreatedAt.Format(time.RFC3339),
	}
}

func toWorkbenchResponse(wb *terminal.Workbench) workbenchResponse {
	return workbenchResponse{
		ID:        wb.ID,
		Name:      wb.Name,
		Color:     wb.Color,
		SortOrder: wb.SortOrder,
		CreatedAt: wb.CreatedAt.Format(time.RFC3339),
	}
}

// ListSessions returns active terminal sessions, optionally filtered by workbench_id.
func (h *TerminalHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	workbenchID := r.URL.Query().Get("workbench_id")
	sessions := h.terminalMgr.ListSessions(workbenchID)

	result := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, toSessionResponse(s))
	}

	writeJSON(w, http.StatusOK, result)
}

// CreateSession creates a new terminal/PTY session.
func (h *TerminalHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title          string `json:"title"`
		Cols           *int   `json:"cols"`
		Rows           *int   `json:"rows"`
		Color          string `json:"color"`
		WorkbenchID    string `json:"workbench_id"`
		Cwd            string `json:"cwd"`
		InitialCommand string `json:"initial_command"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Title == "" {
		req.Title = "Terminal"
	}

	cols := uint16(80)
	rows := uint16(24)
	if req.Cols != nil && *req.Cols > 0 {
		cols = uint16(*req.Cols)
	}
	if req.Rows != nil && *req.Rows > 0 {
		rows = uint16(*req.Rows)
	}

	session, err := h.terminalMgr.CreateSession(req.Title, cols, rows, req.Color, req.WorkbenchID, req.Cwd, req.InitialCommand)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, toSessionResponse(session))
}

// GetSession returns a single terminal session by ID.
func (h *TerminalHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	session := h.terminalMgr.GetSession(id)
	if session == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	writeJSON(w, http.StatusOK, toSessionResponse(session))
}

// UpdateSession updates the title and color of a terminal session.
func (h *TerminalHandler) UpdateSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	session := h.terminalMgr.GetSession(id)
	if session == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var req struct {
		Title string `json:"title"`
		Color string `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.terminalMgr.UpdateSession(id, req.Title, req.Color); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	session = h.terminalMgr.GetSession(id)
	writeJSON(w, http.StatusOK, toSessionResponse(session))
}

// DeleteSession destroys a terminal session and kills its PTY process.
func (h *TerminalHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.terminalMgr.DestroySession(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListWorkbenches returns all workbenches.
func (h *TerminalHandler) ListWorkbenches(w http.ResponseWriter, r *http.Request) {
	workbenches, err := h.terminalMgr.ListWorkbenches()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]workbenchResponse, 0, len(workbenches))
	for _, wb := range workbenches {
		result = append(result, toWorkbenchResponse(&wb))
	}
	writeJSON(w, http.StatusOK, result)
}

// CreateWorkbench creates a new workbench.
func (h *TerminalHandler) CreateWorkbench(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		req.Name = "Workbench"
	}
	wb, err := h.terminalMgr.CreateWorkbench(req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toWorkbenchResponse(wb))
}

// UpdateWorkbench updates the name and color of a workbench.
func (h *TerminalHandler) UpdateWorkbench(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := h.terminalMgr.UpdateWorkbench(id, req.Name, req.Color); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ReorderWorkbenches updates the sort_order of workbenches.
func (h *TerminalHandler) ReorderWorkbenches(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids is required")
		return
	}
	if err := h.terminalMgr.ReorderWorkbenches(req.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reordered"})
}

// DeleteWorkbench destroys all sessions in the workbench and removes it.
func (h *TerminalHandler) DeleteWorkbench(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.terminalMgr.DeleteWorkbench(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// WebSocket constants
const (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = 54 * time.Second
)

// HandleWS upgrades the connection to a WebSocket and bridges it to a PTY session.
func (h *TerminalHandler) HandleWS(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")

	// Authenticate via cookie or Authorization header
	tokenStr := ""
	if cookie, err := r.Cookie("openpaw_token"); err == nil {
		tokenStr = cookie.Value
	}
	if tokenStr == "" {
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 {
				tokenStr = parts[1]
			}
		}
	}
	if tokenStr == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if _, err := h.auth.ValidateToken(tokenStr); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Look up the session
	session := h.terminalMgr.GetSession(sessionID)
	if session == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Create upgrader with origin checking
	upgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 16384,
		CheckOrigin: func(req *http.Request) bool {
			origin := req.Header.Get("Origin")
			if origin == "" {
				return true // Allow non-browser clients
			}
			allowed := []string{
				fmt.Sprintf("http://localhost:%d", h.port),
				fmt.Sprintf("http://127.0.0.1:%d", h.port),
				fmt.Sprintf("https://localhost:%d", h.port),
				fmt.Sprintf("https://127.0.0.1:%d", h.port),
				"http://localhost:5173",
			}
			for _, a := range allowed {
				if origin == a {
					return true
				}
			}
			return false
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Terminal WebSocket upgrade failed: %v", err)
		return
	}

	logger.WS("connected", fmt.Sprintf("terminal:%s", sessionID))

	done := make(chan struct{})

	// Read pump: WebSocket -> PTY
	go func() {
		defer func() {
			close(done)
			conn.Close()
		}()

		conn.SetReadDeadline(time.Now().Add(wsPongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(wsPongWait))
			return nil
		})

		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					logger.Error("Terminal WS read error: %v", err)
				}
				return
			}

			switch msgType {
			case websocket.BinaryMessage:
				// Raw keyboard input -> PTY
				if _, err := session.Ptmx.Write(data); err != nil {
					logger.Error("Terminal PTY write error: %v", err)
					return
				}

			case websocket.TextMessage:
				// JSON control messages
				var msg struct {
					Type string `json:"type"`
					Cols int    `json:"cols"`
					Rows int    `json:"rows"`
				}
				if err := json.Unmarshal(data, &msg); err != nil {
					continue
				}
				switch msg.Type {
				case "resize":
					if msg.Cols > 0 && msg.Rows > 0 {
						if err := h.terminalMgr.ResizeSession(sessionID, uint16(msg.Cols), uint16(msg.Rows)); err != nil {
							logger.Error("Terminal resize error: %v", err)
						}
					}
				}
			}
		}
	}()

	// Write pump: PTY -> WebSocket
	go func() {
		ticker := time.NewTicker(wsPingPeriod)
		defer func() {
			ticker.Stop()
			conn.Close()
			logger.WS("disconnected", fmt.Sprintf("terminal:%s", sessionID))
		}()

		buf := make([]byte, 4096)
		readCh := make(chan []byte)
		errCh := make(chan error)

		// PTY reader goroutine
		go func() {
			for {
				n, err := session.Ptmx.Read(buf)
				if err != nil {
					errCh <- err
					return
				}
				// Copy data to avoid race with next read
				data := make([]byte, n)
				copy(data, buf[:n])
				readCh <- data
			}
		}()

		for {
			select {
			case <-done:
				return

			case data := <-readCh:
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
					return
				}

			case err := <-errCh:
				if err == io.EOF {
					// Process exited — send exit message
					exitMsg, _ := json.Marshal(map[string]interface{}{
						"type": "exit",
						"code": 0,
					})
					conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
					conn.WriteMessage(websocket.TextMessage, exitMsg)
				} else {
					errMsg, _ := json.Marshal(map[string]interface{}{
						"type":    "error",
						"message": err.Error(),
					})
					conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
					conn.WriteMessage(websocket.TextMessage, errMsg)
				}
				return

			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()
}

// UploadFile accepts a file upload from terminal drag-and-drop or paste,
// saves it to the terminal-uploads directory, and returns the absolute path.
func (h *TerminalHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20) // 50MB limit

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or invalid form data")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	uploadsDir := filepath.Join(h.dataDir, "terminal-uploads")
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create uploads directory")
		return
	}

	ext := filepath.Ext(header.Filename)
	baseName := strings.TrimSuffix(header.Filename, ext)
	if baseName == "" {
		baseName = "file"
	}
	uniqueName := fmt.Sprintf("%s_%s%s", baseName, uuid.New().String()[:8], ext)
	destPath := filepath.Join(uploadsDir, uniqueName)

	dst, err := os.Create(destPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create file")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write file")
		return
	}

	absPath, _ := filepath.Abs(destPath)
	writeJSON(w, http.StatusOK, map[string]string{
		"path":     absPath,
		"filename": header.Filename,
	})
}

// ResolvePath finds the absolute path of a file or directory by name.
// Uses Spotlight (mdfind) on macOS, falling back to checking common directories.
func (h *TerminalHandler) ResolvePath(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		IsDir bool   `json:"is_dir"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	path := resolvePathByName(req.Name, req.IsDir)
	if path == "" {
		writeError(w, http.StatusNotFound, "could not locate path")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

func resolvePathByName(name string, isDir bool) string {
	home, _ := os.UserHomeDir()

	// Try Spotlight on macOS first
	if runtime.GOOS == "darwin" {
		if p := resolveWithSpotlight(name, isDir, home); p != "" {
			return p
		}
	}

	// Fall back to checking common directories
	commonDirs := []string{
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "Downloads"),
		filepath.Join(home, "Documents"),
		home,
	}

	for _, dir := range commonDirs {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if isDir && info.IsDir() {
			return candidate
		}
		if !isDir && !info.IsDir() {
			return candidate
		}
	}

	return ""
}

func resolveWithSpotlight(name string, isDir bool, home string) string {
	typeFilter := "public.content"
	if isDir {
		typeFilter = "public.folder"
	}

	query := fmt.Sprintf("kMDItemFSName == '%s' && kMDItemContentTypeTree == '%s'",
		strings.ReplaceAll(name, "'", "'\\''"), typeFilter)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "mdfind", "-onlyin", home, query)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return ""
	}

	// Prefer paths closer to home directory (shorter path = less nested)
	best := lines[0]
	for _, line := range lines[1:] {
		if len(line) < len(best) {
			best = line
		}
	}
	return best
}
