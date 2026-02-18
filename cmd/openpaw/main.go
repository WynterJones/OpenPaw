package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/browser"
	"github.com/openpaw/openpaw/internal/handlers"
	"github.com/openpaw/openpaw/internal/heartbeat"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/auth"
	"github.com/openpaw/openpaw/internal/config"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/memory"
	"github.com/openpaw/openpaw/internal/platform"
	"github.com/openpaw/openpaw/internal/scheduler"
	"github.com/openpaw/openpaw/internal/secrets"
	"github.com/openpaw/openpaw/internal/server"
	"github.com/openpaw/openpaw/internal/toolmgr"
	"github.com/openpaw/openpaw/internal/updater"
	ws "github.com/openpaw/openpaw/internal/websocket"
	"github.com/openpaw/openpaw/web"
)

var version = "dev"

func main() {
	// Handle --version / -v flag
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("openpaw " + version)
		os.Exit(0)
	}

	// Handle update command (early exit before server startup)
	if len(os.Args) == 2 && os.Args[1] == "update" {
		cfg := config.Load()
		updater.RunUpdateCommand(version, cfg.DataDir)
	}

	logger.Banner()

	// Set version for API responses
	handlers.AppVersion = version

	cfg := config.Load()

	// Non-blocking startup version check
	go updater.StartupCheck(version, cfg.DataDir)

	// Initialize database
	db, err := database.New(cfg.DataDir)
	if err != nil {
		logger.Fatal("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Resolve JWT secret: env var > database > generate and persist
	jwtSecret := cfg.JWTSecret
	if jwtSecret == "" {
		// Try loading from database
		var stored string
		err := db.QueryRow("SELECT value FROM settings WHERE key = 'jwt_secret'").Scan(&stored)
		if err == nil && stored != "" {
			jwtSecret = stored
		} else {
			jwtSecret, err = secrets.GenerateKey()
			if err != nil {
				logger.Fatal("Failed to generate JWT secret: %v", err)
			}
			// Persist to database so tokens survive restarts
			if _, err := db.Exec("INSERT INTO settings (id, key, value) VALUES (?, 'jwt_secret', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
				"jwt-secret-key", jwtSecret); err != nil {
				logger.Error("Failed to persist JWT secret: %v", err)
			}
			logger.Success("Generated and persisted JWT secret")
		}
	}

	// Initialize services
	authService := auth.NewService(jwtSecret)

	// Resolve encryption key: env var > database > generate and persist (separate from JWT)
	encKey := cfg.EncryptionKey
	if encKey == "" {
		var storedEncKey string
		err := db.QueryRow("SELECT value FROM settings WHERE key = 'encryption_key'").Scan(&storedEncKey)
		if err == nil && storedEncKey != "" {
			encKey = storedEncKey
		} else {
			// For backward compatibility: if no separate key exists but JWT secret was
			// previously used as encryption key, preserve it as the explicit encryption key
			// to avoid breaking existing encrypted secrets.
			var existingSecrets int
			db.QueryRow("SELECT COUNT(*) FROM secrets").Scan(&existingSecrets)
			if existingSecrets > 0 {
				// Existing secrets were encrypted with JWT secret, so use it
				encKey = jwtSecret
				logger.Info("Migrating encryption key (preserving JWT-derived key for existing secrets)")
			} else {
				// Fresh installation or no secrets yet — generate a truly separate key
				encKey, err = secrets.GenerateKey()
				if err != nil {
					logger.Fatal("Failed to generate encryption key: %v", err)
				}
				logger.Success("Generated separate encryption key")
			}
			// Persist so it's independent going forward
			if _, err := db.Exec("INSERT INTO settings (id, key, value) VALUES (?, 'encryption_key', ?)",
				"encryption-key", encKey); err != nil {
				logger.Fatal("Failed to persist encryption key: %v", err)
			}
		}
	}
	secretsMgr := secrets.NewManager(encKey)

	sched := scheduler.New(db)
	sched.Start()
	defer sched.Stop()

	// Extract embedded frontend FS
	frontendFS, err := fs.Sub(web.FrontendFS, "frontend/dist")
	if err != nil {
		logger.Fatal("Failed to load frontend assets: %v", err)
	}

	// Migrate existing agents to identity file system
	if err := agents.MigrateExistingAgents(db, cfg.DataDir); err != nil {
		logger.Error("Agent identity migration failed: %v", err)
	}

	// Migrate IDENTITY.md → SOUL.md and remove deprecated TOOLS.md
	if err := agents.MigrateIdentityToSoul(cfg.DataDir); err != nil {
		logger.Error("Identity-to-soul migration failed: %v", err)
	}

	// Initialize gateway identity directory
	if err := agents.InitGatewayDir(cfg.DataDir); err != nil {
		logger.Error("Gateway dir initialization failed: %v", err)
	}

	// Tools directory
	toolsDir := filepath.Join(cfg.DataDir, "..", "tools")
	os.MkdirAll(toolsDir, 0755)

	// Dashboards directory (for custom HTML/JS dashboards)
	dashboardsDir := filepath.Join(cfg.DataDir, "..", "dashboards")
	os.MkdirAll(dashboardsDir, 0755)

	// Resolve API key: OPENROUTER_API_KEY > ANTHROPIC_API_KEY (legacy) > encrypted DB value
	envAPIKey := os.Getenv("OPENROUTER_API_KEY")
	if envAPIKey == "" {
		envAPIKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	var dbAPIKey string
	var encryptedKey string
	err = db.QueryRow("SELECT value FROM settings WHERE key = 'openrouter_api_key'").Scan(&encryptedKey)
	if err == nil && encryptedKey != "" {
		decrypted, decErr := secretsMgr.Decrypt(encryptedKey)
		if decErr == nil {
			dbAPIKey = decrypted
		}
	}
	apiKey, _ := llm.ResolveAPIKey(envAPIKey, dbAPIKey)
	llmClient := llm.NewClient(apiKey)
	agents.CheckAPIKey(llmClient)

	// Create agent manager (broadcast will be wired after server creation)
	var wsHub *ws.Hub
	broadcastFn := func(msgType string, payload interface{}) {
		if wsHub == nil {
			return
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		wsHub.Broadcast(ws.Message{
			Type:    msgType,
			Payload: data,
		})
	}
	db.OnAudit = func(action, category string) {
		broadcastFn("audit_log_created", map[string]string{
			"action": action, "category": category,
		})
	}

	agentMgr := agents.NewManager(db, toolsDir, broadcastFn, llmClient)
	agentMgr.DataDir = cfg.DataDir

	// Wire notification function (creates notification + broadcasts)
	notifyFn := func(title, body, priority, sourceAgentSlug, sourceType, link string) {
		n, err := handlers.CreateNotification(db, title, body, priority, sourceAgentSlug, sourceType, link)
		if err != nil {
			logger.Error("Failed to create notification: %v", err)
			return
		}
		broadcastFn("notification_created", n)
	}
	agentMgr.NotifyFn = notifyFn

	// Create tool process manager
	toolMgr := toolmgr.New(db, toolsDir, broadcastFn)
	agentMgr.ToolMgr = toolMgr
	agentMgr.DashboardsDir = dashboardsDir

	// Create browser manager
	browserMgr := browser.NewManager(db, cfg.DataDir, broadcastFn)
	browserMgr.SetTopicBroadcast(func(topic, msgType string, payload interface{}) {
		if wsHub == nil {
			return
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		wsHub.BroadcastToTopic(topic, ws.Message{
			Type:    msgType,
			Payload: data,
		})
	})
	agentMgr.BrowserMgr = browserMgr

	// Create memory manager
	memoryMgr := memory.NewManager(cfg.DataDir)
	agentMgr.MemoryMgr = memoryMgr

	// Create heartbeat manager (broadcast will be wired after wsHub is available)
	heartbeatMgr := heartbeat.New(db, agentMgr, broadcastFn, cfg.DataDir)
	heartbeatMgr.LoadConfig()

	// Wire scheduler dependencies and load schedules
	sched.SetToolCaller(toolMgr)
	sched.SetPromptSender(agentMgr)
	sched.SetBrowserExecutor(browserMgr)
	sched.SetNotifyFunc(notifyFn)
	sched.LoadSchedules()
	sched.StartDataRetention()

	// Load model settings from database
	var gatewayModel, builderModel, maxTurnsStr, agentTimeoutStr string
	db.QueryRow("SELECT value FROM settings WHERE key = 'gateway_model'").Scan(&gatewayModel)
	db.QueryRow("SELECT value FROM settings WHERE key = 'builder_model'").Scan(&builderModel)
	db.QueryRow("SELECT value FROM settings WHERE key = 'max_turns'").Scan(&maxTurnsStr)
	db.QueryRow("SELECT value FROM settings WHERE key = 'agent_timeout_min'").Scan(&agentTimeoutStr)
	if gatewayModel != "" {
		agentMgr.GatewayModel = agents.ParseModel(gatewayModel, llm.ModelHaiku)
	}
	if builderModel != "" {
		agentMgr.BuilderModel = agents.ParseModel(builderModel, llm.ModelSonnet)
	}
	if maxTurnsStr != "" {
		if v, err := strconv.Atoi(maxTurnsStr); err == nil && v > 0 {
			agentMgr.MaxTurns = v
		}
	}
	if agentTimeoutStr != "" {
		if v, err := strconv.Atoi(agentTimeoutStr); err == nil && v > 0 {
			agentMgr.AgentTimeoutMin = v
		}
	}

	// Create server
	srv := server.New(server.Config{
		DB:           db,
		Auth:         authService,
		Secrets:      secretsMgr,
		Scheduler:    sched,
		AgentMgr:     agentMgr,
		ToolMgr:      toolMgr,
		BrowserMgr:   browserMgr,
		HeartbeatMgr: heartbeatMgr,
		MemoryMgr:    memoryMgr,
		LLMClient:    llmClient,
		FrontendFS:   frontendFS,
		ToolsDir:     toolsDir,
		DataDir:      cfg.DataDir,
		Port:         cfg.Port,
	})
	wsHub = srv.WSHub

	// Start WebSocket hub
	go srv.WSHub.Run()

	// Start tool process manager (auto-starts enabled tools)
	go toolMgr.Start()

	// Start heartbeat manager
	heartbeatMgr.Start()

	// Check if setup is needed
	hasAdmin, err := db.HasAdminUser()
	if err != nil {
		logger.Fatal("Failed to check admin user: %v", err)
	}
	if !hasAdmin {
		logger.Warn("No admin user found. Visit the app to complete setup.")
	}

	addr := fmt.Sprintf("%s:%d", cfg.BindAddress, cfg.Port)
	if cfg.BindAddress != "127.0.0.1" && cfg.BindAddress != "localhost" {
		logger.Warn("Binding to %s — accessible from the network. Use OPENPAW_BIND=127.0.0.1 for localhost-only.", cfg.BindAddress)
	}
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // intentionally zero for SSE/WebSocket support
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		url := fmt.Sprintf("http://localhost:%d", cfg.Port)
		logger.Listen(addr, url, cfg.Port)
		if os.Getenv("OPENPAW_NO_OPEN") != "1" {
			platform.OpenBrowser(url)
		}
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server error: %v", err)
		}
	}()

	<-done
	logger.Shutdown("Shutting down server...")

	// Shut down heartbeat manager
	heartbeatMgr.Stop()

	// Shut down browser sessions
	browserMgr.Shutdown()

	// Shut down tool processes
	toolMgr.Shutdown()

	// Shut down agents
	agentMgr.Shutdown()

	// Close memory databases
	memoryMgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Fatal("Server shutdown failed: %v", err)
	}

	logger.Bye()
}


