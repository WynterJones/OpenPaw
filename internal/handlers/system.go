package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/netutil"
)

var startTime = time.Now()

// AppVersion is set from main at startup via ldflags or the VERSION file.
var AppVersion = "dev"

type SystemHandler struct {
	db      *database.DB
	dataDir string
	client  *llm.Client
	port    int
}

func NewSystemHandler(db *database.DB, dataDir string, client *llm.Client, port int) *SystemHandler {
	return &SystemHandler{db: db, dataDir: dataDir, client: client, port: port}
}

func (h *SystemHandler) Info(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(startTime)
	uptimeStr := formatDuration(uptime)

	dbSize := "unknown"
	dbPath := filepath.Join(h.dataDir, "openpaw.db")
	if info, err := os.Stat(dbPath); err == nil {
		dbSize = formatBytes(info.Size())
	}

	var toolCount, secretCount, scheduleCount int
	h.db.QueryRow("SELECT COUNT(*) FROM tools WHERE deleted_at IS NULL").Scan(&toolCount)
	h.db.QueryRow("SELECT COUNT(*) FROM secrets").Scan(&secretCount)
	h.db.QueryRow("SELECT COUNT(*) FROM schedules").Scan(&scheduleCount)

	apiKeySource := resolveAPIKeySource(h.client)
	apiKeyConfigured := apiKeySource != "none"

	lanIP := netutil.GetLANIP()
	tailscaleIP := netutil.GetTailscaleIP()

	var tailscaleEnabled string
	h.db.QueryRow("SELECT value FROM settings WHERE key = 'tailscale_enabled'").Scan(&tailscaleEnabled)

	var bindAddress string
	h.db.QueryRow("SELECT value FROM settings WHERE key = 'bind_address'").Scan(&bindAddress)
	if bindAddress == "" {
		bindAddress = "127.0.0.1"
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":            AppVersion,
		"go_version":         runtime.Version(),
		"os":                 runtime.GOOS,
		"arch":               runtime.GOARCH,
		"uptime":             uptimeStr,
		"db_size":            dbSize,
		"tool_count":         toolCount,
		"secret_count":       secretCount,
		"schedule_count":     scheduleCount,
		"api_key_configured": apiKeyConfigured,
		"api_key_source":     apiKeySource,
		"lan_ip":             lanIP,
		"tailscale_ip":       tailscaleIP,
		"port":               h.port,
		"tailscale_enabled":  tailscaleEnabled == "true",
		"bind_address":       bindAddress,
	})
}

func (h *SystemHandler) DeleteData(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	// Archive live counters before deletion
	var liveCost, liveInTok, liveOutTok float64
	h.db.QueryRow("SELECT COALESCE(value,0) FROM system_stats WHERE key='live_cost_usd'").Scan(&liveCost)
	h.db.QueryRow("SELECT COALESCE(value,0) FROM system_stats WHERE key='live_input_tokens'").Scan(&liveInTok)
	h.db.QueryRow("SELECT COALESCE(value,0) FROM system_stats WHERE key='live_output_tokens'").Scan(&liveOutTok)
	var activityCount int
	h.db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&activityCount)

	if _, err := h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'archived_cost_usd'", liveCost); err != nil {
		logger.Error("Failed to archive cost: %v", err)
	}
	if _, err := h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'archived_input_tokens'", liveInTok); err != nil {
		logger.Error("Failed to archive input tokens: %v", err)
	}
	if _, err := h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'archived_output_tokens'", liveOutTok); err != nil {
		logger.Error("Failed to archive output tokens: %v", err)
	}
	if _, err := h.db.Exec("UPDATE system_stats SET value = value + ? WHERE key = 'archived_activity_count'", float64(activityCount)); err != nil {
		logger.Error("Failed to archive activity count: %v", err)
	}
	// Reset live counters
	h.db.Exec("UPDATE system_stats SET value = 0 WHERE key IN ('live_cost_usd', 'live_input_tokens', 'live_output_tokens')")

	// Delete all application data. Order: standalone/child tables first,
	// then parent tables. CASCADE FKs auto-delete: tool_integrity,
	// schedule_executions, thread_members, chat_attachments,
	// dashboard_data_points, browser_tasks, browser_action_log.
	tablesToDelete := []string{
		"agent_tool_access",
		"context_files",
		"context_folders",
		"browser_sessions",
		"notifications",
		"heartbeat_executions",
		"chat_messages",
		"chat_threads",
		"work_orders",
		"agents",
		"schedules",
		"secrets",
		"dashboards",
		"agent_roles",
		"audit_logs",
		"settings",
		"tools",
	}
	for _, table := range tablesToDelete {
		if _, err := h.db.Exec("DELETE FROM " + table); err != nil {
			logger.Error("Failed to delete %s: %v", table, err)
		}
	}

	// Clear filesystem data
	for _, dir := range []string{"skills", "agents", "gateway", "context", "browser_sessions"} {
		dirPath := filepath.Join(h.dataDir, dir)
		os.RemoveAll(dirPath)
		os.MkdirAll(dirPath, 0755)
	}

	h.db.LogAudit(userID, "data_deleted", "system", "system", "", "All application data deleted")
	writeJSON(w, http.StatusOK, map[string]string{"message": "all data deleted"})
}

func (h *SystemHandler) Balance(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || !h.client.IsConfigured() {
		writeError(w, http.StatusServiceUnavailable, "API key not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	info, err := h.client.GetKeyInfo(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to fetch balance: "+err.Error())
		return
	}

	credits, _ := h.client.GetCredits(ctx)

	resp := map[string]interface{}{
		"usage":           info.Usage,
		"usage_monthly":   info.UsageMonthly,
		"limit":           info.Limit,
		"limit_remaining": info.LimitRemaining,
		"is_free_tier":    info.IsFreeTier,
		"label":           info.Label,
	}
	if info.RateLimit != nil {
		resp["rate_limit"] = map[string]interface{}{
			"requests": info.RateLimit.Requests,
			"interval": info.RateLimit.Interval,
		}
	}
	if credits != nil {
		resp["total_credits"] = credits.TotalCredits
		resp["total_usage"] = credits.TotalUsage
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *SystemHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *SystemHandler) Prerequisites(w http.ResponseWriter, r *http.Request) {
	apiKeyConfigured := h.client != nil && h.client.IsConfigured()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"api_key_configured": apiKeyConfigured,
	})
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
