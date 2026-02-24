package server

import (
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/backup"
	"github.com/openpaw/openpaw/internal/browser"
	"github.com/openpaw/openpaw/internal/heartbeat"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/auth"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/handlers"
	"github.com/openpaw/openpaw/internal/memory"
	mw "github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/scheduler"
	"github.com/openpaw/openpaw/internal/secrets"
	"github.com/openpaw/openpaw/internal/terminal"
	"github.com/openpaw/openpaw/internal/toolmgr"
	ws "github.com/openpaw/openpaw/internal/websocket"
)

type Server struct {
	Router       *chi.Mux
	DB           *database.DB
	Auth         *auth.Service
	Secrets      *secrets.Manager
	Scheduler    *scheduler.Scheduler
	WSHub        *ws.Hub
	AgentManager *agents.Manager
	ToolMgr      *toolmgr.Manager
	BrowserMgr   *browser.Manager
	HeartbeatMgr *heartbeat.Manager
	BackupMgr    *backup.Manager
	MemoryMgr    *memory.Manager
	TerminalMgr  *terminal.Manager
}

type Config struct {
	DB           *database.DB
	Auth         *auth.Service
	Secrets      *secrets.Manager
	Scheduler    *scheduler.Scheduler
	AgentMgr     *agents.Manager
	ToolMgr      *toolmgr.Manager
	BrowserMgr   *browser.Manager
	HeartbeatMgr *heartbeat.Manager
	BackupMgr    *backup.Manager
	MemoryMgr    *memory.Manager
	TerminalMgr  *terminal.Manager
	LLMClient    *llm.Client
	FrontendFS   fs.FS
	ToolsDir     string
	DataDir      string
	Port         int
}

func New(cfg Config) *Server {
	s := &Server{
		Router:       chi.NewRouter(),
		DB:           cfg.DB,
		Auth:         cfg.Auth,
		Secrets:      cfg.Secrets,
		Scheduler:    cfg.Scheduler,
		WSHub:        ws.NewHub(cfg.Auth, cfg.Port),
		AgentManager: cfg.AgentMgr,
		ToolMgr:      cfg.ToolMgr,
		BrowserMgr:   cfg.BrowserMgr,
		HeartbeatMgr: cfg.HeartbeatMgr,
		BackupMgr:    cfg.BackupMgr,
		MemoryMgr:    cfg.MemoryMgr,
		TerminalMgr:  cfg.TerminalMgr,
	}

	s.setupMiddleware()
	s.setupRoutes(cfg.ToolMgr, cfg.ToolsDir, cfg.DataDir, cfg.Secrets, cfg.LLMClient, cfg.Port)
	s.setupFrontend(cfg.FrontendFS)

	return s
}

func (s *Server) setupMiddleware() {
	s.Router.Use(chiMiddleware.RealIP)
	s.Router.Use(mw.RequestID)
	s.Router.Use(mw.SecurityHeaders)
	s.Router.Use(mw.Logger)
	s.Router.Use(mw.CORS)
	s.Router.Use(chiMiddleware.Recoverer)
}

func (s *Server) setupRoutes(toolMgr *toolmgr.Manager, toolsDir string, dataDir string, secretsMgr *secrets.Manager, llmClient *llm.Client, port int) {
	authHandler := handlers.NewAuthHandler(s.DB, s.Auth, dataDir)
	setupHandler := handlers.NewSetupHandler(s.DB, s.Auth, secretsMgr, llmClient)
	toolsHandler := handlers.NewToolsHandler(s.DB, s.AgentManager, toolMgr, toolsDir)
	secretsHandler := handlers.NewSecretsHandler(s.DB, s.Secrets, toolMgr)
	schedulesHandler := handlers.NewSchedulesHandler(s.DB, s.Scheduler)
	dashboardsDir := filepath.Join(dataDir, "..", "dashboards")
	dashboardsHandler := handlers.NewDashboardsHandler(s.DB, toolMgr, dashboardsDir)
	agentRolesHandler := handlers.NewAgentRolesHandler(s.DB, dataDir)
	chatHandler := handlers.NewChatHandler(s.DB, s.AgentManager, toolsDir, dataDir)
	contextHandler := handlers.NewContextHandler(s.DB, dataDir)
	skillsHandler := handlers.NewSkillsHandler(dataDir)
	logsHandler := handlers.NewLogsHandler(s.DB)
	settingsHandler := handlers.NewSettingsHandler(s.DB, s.AgentManager, secretsMgr, llmClient, dataDir, port)
	agentsHandler := handlers.NewAgentsHandler(s.DB, s.AgentManager)
	systemHandler := handlers.NewSystemHandler(s.DB, dataDir, llmClient, port)
	browserHandler := handlers.NewBrowserHandler(s.DB, s.BrowserMgr)
	notificationsHandler := handlers.NewNotificationsHandler(s.DB)
	heartbeatHandler := handlers.NewHeartbeatHandler(s.DB, s.HeartbeatMgr)
	backupHandler := handlers.NewBackupHandler(s.DB, s.BackupMgr)
	memoryHandler := handlers.NewMemoryHandler(s.MemoryMgr)
	toolLibraryHandler := handlers.NewToolLibraryHandler(s.DB, toolMgr, toolsDir, secretsMgr)
	agentLibraryHandler := handlers.NewAgentLibraryHandler(s.DB, dataDir)
	skillLibraryHandler := handlers.NewSkillLibraryHandler(s.DB, dataDir)
	skillsShHandler := handlers.NewSkillsShHandler(s.DB, dataDir)
	terminalHandler := handlers.NewTerminalHandler(s.DB, s.TerminalMgr, s.Auth, port, dataDir)
	projectsHandler := handlers.NewProjectsHandler(s.DB)
	agentTasksHandler := handlers.NewAgentTasksHandler(s.DB)

	s.Router.Route("/api/v1", func(r chi.Router) {
		// Public routes (no auth required)
		r.Route("/auth", func(r chi.Router) {
			r.With(mw.RateLimit(10, time.Minute)).Post("/login", authHandler.Login)
		})

		r.Route("/setup", func(r chi.Router) {
			r.With(mw.RateLimit(5, time.Minute)).Get("/status", setupHandler.Status)
			r.With(mw.RateLimit(5, time.Minute)).Post("/init", setupHandler.Init)
			r.With(mw.RateLimit(5, time.Minute)).Post("/agent-roles", func(w http.ResponseWriter, req *http.Request) {
				hasAdmin, err := s.DB.HasAdminUser()
				if err != nil || hasAdmin {
					http.Error(w, `{"error":"setup already complete"}`, http.StatusForbidden)
					return
				}
				agentRolesHandler.SeedPresets(w, req)
			})
		})

		// Public design config (needed before login for UI theming)
		r.Get("/settings/design", settingsHandler.GetDesign)

		// Public prerequisites check (needed during setup)
		r.Get("/system/prerequisites", systemHandler.Prerequisites)

		// Public health check (used by Tauri sidecar polling)
		r.Get("/system/health", systemHandler.Health)

		// Public uploaded avatar serving
		r.Get("/uploads/avatars/{filename}", agentRolesHandler.ServeAvatar)

		// Public uploaded background serving
		r.Get("/uploads/backgrounds/{filename}", settingsHandler.ServeBackground)

		// Public dashboard assets (served in sandboxed iframes that can't send auth cookies)
		r.Get("/dashboards/{id}/assets/*", dashboardsHandler.ServeAssets)

		// WebSocket (auth handled internally)
		r.Get("/ws", s.WSHub.HandleWS)

		// Terminal WebSocket (auth handled internally)
		r.Get("/terminal/ws/{sessionId}", terminalHandler.HandleWS)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(mw.Auth(s.Auth))
			r.Use(mw.CSRFProtection)

			// Auth
			r.Post("/auth/logout", authHandler.Logout)
			r.Post("/auth/change-password", authHandler.ChangePassword)
			r.Delete("/auth/account", authHandler.DeleteAccount)
			r.Get("/auth/me", authHandler.Me)
			r.Put("/auth/profile", authHandler.UpdateProfile)
			r.Post("/auth/avatar", authHandler.UploadAvatar)

			// Tools
			r.Route("/tools", func(r chi.Router) {
				r.Get("/", toolsHandler.List)
				r.Post("/", toolsHandler.Create)
				r.Get("/{id}", toolsHandler.Get)
				r.Put("/{id}", toolsHandler.Update)
				r.Delete("/{id}", toolsHandler.Delete)
				r.Post("/{id}/call", toolsHandler.Call)
				r.Post("/{id}/enable", toolsHandler.Enable)
				r.Post("/{id}/disable", toolsHandler.Disable)
				r.Post("/{id}/compile", toolsHandler.Compile)
				r.Post("/{id}/start", toolsHandler.Start)
				r.Post("/{id}/stop", toolsHandler.Stop)
				r.Post("/{id}/restart", toolsHandler.Restart)
				r.Get("/{id}/status", toolsHandler.Status)
				r.Get("/{id}/widget.js", toolsHandler.WidgetJS)
				r.Get("/{id}/proxy/*", toolsHandler.Proxy)
			})

			// Tool Library
			r.Route("/tool-library", func(r chi.Router) {
				r.Get("/", toolLibraryHandler.ListCatalog)
				r.Get("/{slug}", toolLibraryHandler.GetCatalogTool)
				r.Post("/{slug}/install", toolLibraryHandler.InstallCatalogTool)
			})

			// Agent Library
			r.Route("/agent-library", func(r chi.Router) {
				r.Get("/", agentLibraryHandler.ListCatalog)
				r.Get("/{slug}", agentLibraryHandler.GetCatalogAgent)
				r.Post("/{slug}/install", agentLibraryHandler.InstallCatalogAgent)
			})

			// Skill Library
			r.Route("/skill-library", func(r chi.Router) {
				r.Get("/", skillLibraryHandler.ListCatalog)
				r.Get("/{slug}", skillLibraryHandler.GetCatalogSkill)
				r.Post("/{slug}/install", skillLibraryHandler.InstallCatalogSkill)
			})

			// Skills.sh (external skills directory)
			r.Route("/skills-sh", func(r chi.Router) {
				r.Get("/", skillsShHandler.Search)
				r.Get("/detail", skillsShHandler.GetSkill)
				r.Post("/install", skillsShHandler.Install)
			})

			// Tool Import/Export/Integrity
			r.Get("/tools/{id}/export", toolLibraryHandler.ExportTool)
			r.Post("/tools/import", toolLibraryHandler.ImportTool)
			r.Get("/tools/{id}/integrity", toolLibraryHandler.GetIntegrity)

			// Secrets
			r.Route("/secrets", func(r chi.Router) {
				r.Get("/", secretsHandler.List)
				r.Post("/", secretsHandler.Create)
				r.Post("/check", secretsHandler.CheckNames)
				r.Post("/ensure", secretsHandler.EnsurePlaceholders)
				r.Delete("/{id}", secretsHandler.Delete)
				r.Post("/{id}/rotate", secretsHandler.Rotate)
				r.Post("/{id}/test", secretsHandler.Test)
			})

			// Schedules
			r.Route("/schedules", func(r chi.Router) {
				r.Get("/", schedulesHandler.List)
				r.Post("/", schedulesHandler.Create)
				r.Put("/{id}", schedulesHandler.Update)
				r.Delete("/{id}", schedulesHandler.Delete)
				r.Post("/{id}/run-now", schedulesHandler.RunNow)
				r.Post("/{id}/toggle", schedulesHandler.Toggle)
				r.Get("/{id}/executions", schedulesHandler.Executions)
			})

			// Dashboards
			r.Route("/dashboards", func(r chi.Router) {
				r.Get("/", dashboardsHandler.List)
				r.Post("/", dashboardsHandler.Create)
				r.Get("/{id}", dashboardsHandler.Get)
				r.Put("/{id}", dashboardsHandler.Update)
				r.Delete("/{id}", dashboardsHandler.Delete)
				r.Post("/{id}/refresh", dashboardsHandler.RefreshData)
				r.Get("/{id}/data/{widgetId}", dashboardsHandler.GetWidgetData)
				r.Post("/{id}/collect", dashboardsHandler.CollectData)
			})

			// Agent Roles
			r.Route("/agent-roles", func(r chi.Router) {
				r.Get("/", agentRolesHandler.List)
				r.Post("/", agentRolesHandler.Create)
				r.Post("/upload-avatar", agentRolesHandler.UploadAvatar)
				// Task counts across all agents (before {slug} to avoid conflict)
				r.Get("/task-counts", agentTasksHandler.AllCounts)
				// Gateway file endpoints (before {slug} to avoid conflict)
				r.Get("/gateway/files", agentRolesHandler.GetGatewayFiles)
				r.Get("/gateway/files/*", agentRolesHandler.GetGatewayFile)
				r.Put("/gateway/files/*", agentRolesHandler.UpdateGatewayFile)
				r.Get("/gateway/memory", agentRolesHandler.GetGatewayMemory)
				r.Get("/{slug}", agentRolesHandler.Get)
				r.Put("/{slug}", agentRolesHandler.Update)
				r.Put("/{slug}/toggle", agentRolesHandler.Toggle)
				r.Delete("/{slug}", agentRolesHandler.Delete)
				// Identity files
				r.Get("/{slug}/files", agentRolesHandler.GetFiles)
				r.Get("/{slug}/files/*", agentRolesHandler.GetFile)
				r.Put("/{slug}/files/*", agentRolesHandler.UpdateFile)
				r.Post("/{slug}/files/init", agentRolesHandler.InitFiles)
				r.Get("/{slug}/memory", agentRolesHandler.ListMemory)
				// Agent tools
				r.Get("/{slug}/tools", agentRolesHandler.ListTools)
				r.Post("/{slug}/tools/{toolId}/grant", agentRolesHandler.GrantToolAccess)
				r.Delete("/{slug}/tools/{toolId}/revoke", agentRolesHandler.RevokeToolAccess)
				// Agent skills
				r.Get("/{slug}/skills", skillsHandler.ListAgentSkills)
				r.Post("/{slug}/skills/add", skillsHandler.AddSkillToAgent)
				r.Put("/{slug}/skills/{name}", skillsHandler.UpdateAgentSkill)
				r.Delete("/{slug}/skills/{name}", skillsHandler.RemoveSkillFromAgent)
				r.Post("/{slug}/skills/{name}/publish", skillsHandler.PublishAgentSkill)
				// Agent tasks (Kanban board)
				r.Get("/{slug}/tasks/counts", agentTasksHandler.Counts)
				r.Put("/{slug}/tasks/reorder", agentTasksHandler.Reorder)
				r.Delete("/{slug}/tasks/clear-done", agentTasksHandler.ClearDone)
				r.Get("/{slug}/tasks", agentTasksHandler.List)
				r.Post("/{slug}/tasks", agentTasksHandler.Create)
				r.Put("/{slug}/tasks/{taskId}", agentTasksHandler.Update)
				r.Delete("/{slug}/tasks/{taskId}", agentTasksHandler.Delete)
			})

			// Agent Memories
			r.Get("/agent-roles/{slug}/memories", memoryHandler.ListMemories)
			r.Get("/agent-roles/{slug}/memories/stats", memoryHandler.GetStats)
			r.Delete("/agent-roles/{slug}/memories/{memoryId}", memoryHandler.DeleteMemory)

			// Gateway Memories
			r.Get("/gateway/memories", memoryHandler.ListGatewayMemories)
			r.Get("/gateway/memories/stats", memoryHandler.GetGatewayStats)

			// Tool ownership
			r.Put("/tools/{id}/owner", agentRolesHandler.UpdateToolOwner)

			// Global Skills
			r.Route("/skills", func(r chi.Router) {
				r.Get("/", skillsHandler.List)
				r.Post("/", skillsHandler.Create)
				r.Get("/{name}", skillsHandler.Get)
				r.Put("/{name}", skillsHandler.Update)
				r.Delete("/{name}", skillsHandler.Delete)
			})

			// Chat
			r.Route("/chat", func(r chi.Router) {
				r.Get("/threads", chatHandler.ListThreads)
				r.Get("/threads/active", chatHandler.ActiveThreadIds)
				r.Post("/threads", chatHandler.CreateThread)
				r.Put("/threads/{id}", chatHandler.UpdateThread)
				r.Delete("/threads/{id}", chatHandler.DeleteThread)
				r.Get("/threads/{id}/status", chatHandler.ThreadStatus)
				r.Get("/threads/{id}/stats", chatHandler.ThreadStats)
				r.Get("/threads/{id}/messages", chatHandler.GetMessages)
				r.Post("/threads/{id}/messages", chatHandler.SendMessage)
				r.Post("/threads/{id}/compact", chatHandler.CompactThread)
				r.Post("/threads/{id}/confirm", chatHandler.ConfirmWork)
				r.Post("/threads/{id}/reject", chatHandler.RejectWork)
				r.Post("/threads/{id}/stop", chatHandler.StopThread)
				r.Get("/threads/{id}/members", chatHandler.ListThreadMembers)
				r.Delete("/threads/{id}/members/{slug}", chatHandler.RemoveThreadMember)
			})

			// Context
			r.Route("/context", func(r chi.Router) {
				r.Get("/tree", contextHandler.GetTree)
				r.Post("/folders", contextHandler.CreateFolder)
				r.Put("/folders/{id}", contextHandler.UpdateFolder)
				r.Delete("/folders/{id}", contextHandler.DeleteFolder)
				r.Get("/files", contextHandler.ListFiles)
				r.Get("/files/{id}", contextHandler.GetFile)
				r.Get("/files/{id}/raw", contextHandler.ServeRaw)
				r.Post("/files", contextHandler.UploadFile)
				r.Put("/files/{id}", contextHandler.UpdateFile)
				r.Delete("/files/{id}", contextHandler.DeleteFile)
				r.Put("/files/{id}/move", contextHandler.MoveFile)
				r.Get("/about-you", contextHandler.GetAboutYou)
				r.Put("/about-you", contextHandler.UpdateAboutYou)
			})

			// Chat attachments
			r.Post("/chat/attachments", contextHandler.UploadChatAttachment)
			r.Get("/chat/attachments/{id}", contextHandler.ServeChatAttachment)

			// Agents
			r.Route("/agents", func(r chi.Router) {
				r.Get("/", agentsHandler.List)
				r.Get("/{id}", agentsHandler.Get)
				r.Post("/{id}/stop", agentsHandler.Stop)
			})

			// Browser
			r.Route("/browser", func(r chi.Router) {
				r.Get("/sessions", browserHandler.ListSessions)
				r.Post("/sessions", browserHandler.CreateSession)
				r.Get("/sessions/{id}", browserHandler.GetSession)
				r.Put("/sessions/{id}", browserHandler.UpdateSession)
				r.Delete("/sessions/{id}", browserHandler.DeleteSession)
				r.Post("/sessions/{id}/start", browserHandler.StartSession)
				r.Post("/sessions/{id}/stop", browserHandler.StopSession)
				r.Post("/sessions/{id}/action", browserHandler.ExecuteAction)
				r.Get("/sessions/{id}/screenshot", browserHandler.GetScreenshot)
				r.Post("/sessions/{id}/control", browserHandler.TakeControl)
				r.Post("/sessions/{id}/release", browserHandler.ReleaseControl)
				r.Get("/sessions/{id}/tasks", browserHandler.ListTasks)
				r.Get("/tasks", browserHandler.ListAllTasks)
				r.Get("/tasks/{taskId}", browserHandler.GetTask)
				r.Get("/tasks/{taskId}/actions", browserHandler.GetTaskActions)
			})

			// Notifications
			r.Route("/notifications", func(r chi.Router) {
				r.Get("/", notificationsHandler.List)
				r.Get("/count", notificationsHandler.UnreadCount)
				r.Put("/{id}/read", notificationsHandler.MarkRead)
				r.Put("/read-all", notificationsHandler.MarkAllRead)
				r.Delete("/{id}", notificationsHandler.Dismiss)
				r.Delete("/", notificationsHandler.DismissAll)
			})

			// Heartbeat
			r.Route("/heartbeat", func(r chi.Router) {
				r.Get("/config", heartbeatHandler.GetConfig)
				r.Put("/config", heartbeatHandler.UpdateConfig)
				r.Get("/history", heartbeatHandler.ListExecutions)
				r.Post("/run-now", heartbeatHandler.RunNow)
			})

			// Backup
			r.Route("/settings/backup", func(r chi.Router) {
				r.Get("/", backupHandler.GetConfig)
				r.Put("/", backupHandler.UpdateConfig)
				r.Post("/run", backupHandler.RunNow)
				r.Post("/test", backupHandler.TestConnection)
				r.Get("/history", backupHandler.ListHistory)
				r.Get("/detect-git", backupHandler.DetectGit)
			})

			// Terminal
			r.Route("/terminal", func(r chi.Router) {
				r.Get("/sessions", terminalHandler.ListSessions)
				r.Post("/sessions", terminalHandler.CreateSession)
				r.Get("/sessions/{id}", terminalHandler.GetSession)
				r.Put("/sessions/{id}", terminalHandler.UpdateSession)
				r.Delete("/sessions/{id}", terminalHandler.DeleteSession)
				r.Post("/upload", terminalHandler.UploadFile)
				r.Post("/resolve-path", terminalHandler.ResolvePath)
				r.Get("/workbenches", terminalHandler.ListWorkbenches)
				r.Post("/workbenches", terminalHandler.CreateWorkbench)
				r.Put("/workbenches/{id}", terminalHandler.UpdateWorkbench)
				r.Delete("/workbenches/{id}", terminalHandler.DeleteWorkbench)
				r.Put("/workbenches-reorder", terminalHandler.ReorderWorkbenches)
			})

			// Projects
			r.Route("/projects", func(r chi.Router) {
				r.Get("/", projectsHandler.List)
				r.Post("/", projectsHandler.Create)
				r.Put("/{id}", projectsHandler.Update)
				r.Delete("/{id}", projectsHandler.Delete)
			})

			// Logs
			r.Get("/logs", logsHandler.List)
			r.Get("/logs/stats", logsHandler.Stats)
			r.Get("/logs/tools/{id}", logsHandler.ToolLogs)

			// System
			r.Get("/system/info", systemHandler.Info)
			r.Get("/system/balance", systemHandler.Balance)
			r.Delete("/system/data", systemHandler.DeleteData)
			r.Post("/system/pick-folder", systemHandler.PickFolder)

			// Settings
			r.Get("/settings", settingsHandler.Get)
			r.Put("/settings", settingsHandler.Update)
			r.Get("/settings/general", settingsHandler.GetGeneral)
			r.Put("/settings/general", settingsHandler.UpdateGeneral)
			r.Put("/settings/design", settingsHandler.UpdateDesign)
			r.Post("/settings/design/background", settingsHandler.UploadBackground)
			r.Delete("/settings/design/background", settingsHandler.DeleteBackground)
			r.Get("/settings/models", settingsHandler.GetModels)
			r.Put("/settings/models", settingsHandler.UpdateModels)
			r.Get("/settings/api-key", settingsHandler.GetAPIKey)
			r.Put("/settings/api-key", settingsHandler.UpdateAPIKey)
			r.Get("/settings/available-models", settingsHandler.AvailableModels)
		})
	})
}

func (s *Server) setupFrontend(frontendFS fs.FS) {
	fileServer := http.FileServer(http.FS(frontendFS))

	s.Router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		// If the request is for an API route, return 404
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}

		// Try to serve the file directly
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if the file exists
		f, err := frontendFS.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for all other routes
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
