package handlers

import (
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/updater"
)

type BroadcastFunc func(msgType string, payload interface{})

type UpdateHandler struct {
	db        *database.DB
	dataDir   string
	broadcast BroadcastFunc
	updating  atomic.Bool
}

func NewUpdateHandler(db *database.DB, dataDir string, broadcast BroadcastFunc) *UpdateHandler {
	return &UpdateHandler{db: db, dataDir: dataDir, broadcast: broadcast}
}

func (h *UpdateHandler) CheckUpdate(w http.ResponseWriter, r *http.Request) {
	currentVersion := AppVersion
	installMethod := updater.DetectInstallMethod()

	if currentVersion == "dev" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"current_version": currentVersion,
			"latest_version":  currentVersion,
			"update_available": false,
			"install_method":  installMethod.String(),
			"can_self_update": false,
		})
		return
	}

	current, err := updater.ParseSemVer(currentVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Cannot parse current version")
		return
	}

	// Check cache first
	var latestStr string
	if cached := updater.LoadCache(h.dataDir); cached != nil {
		latestStr = cached.LatestVersion
	} else {
		release, err := updater.FetchLatestRelease()
		if err != nil {
			writeError(w, http.StatusBadGateway, "Failed to check for updates: "+err.Error())
			return
		}
		latest, err := updater.ParseSemVer(release.TagName)
		if err != nil {
			writeError(w, http.StatusBadGateway, "Cannot parse latest version")
			return
		}
		latestStr = latest.String()
		updater.SaveCache(h.dataDir, &updater.CachedCheck{
			LatestVersion: latestStr,
			CheckedAt:     time.Now(),
		})
	}

	latest, _ := updater.ParseSemVer(latestStr)
	canSelfUpdate := installMethod == updater.InstallDirect

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current_version":  current.String(),
		"latest_version":   latest.String(),
		"update_available": latest.IsNewer(current),
		"install_method":   installMethod.String(),
		"can_self_update":  canSelfUpdate,
	})
}

func (h *UpdateHandler) ApplyUpdate(w http.ResponseWriter, r *http.Request) {
	if AppVersion == "dev" {
		writeError(w, http.StatusBadRequest, "Cannot self-update a development build")
		return
	}

	installMethod := updater.DetectInstallMethod()
	if installMethod != updater.InstallDirect {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Self-update not supported for %s installs", installMethod))
		return
	}

	if !h.updating.CompareAndSwap(false, true) {
		writeError(w, http.StatusConflict, "Update already in progress")
		return
	}

	userID := middleware.GetUserID(r.Context())
	go h.runUpdate(userID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

type updateProgress struct {
	Step    string `json:"step"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Percent int    `json:"percent"`
}

func (h *UpdateHandler) sendProgress(step, status, message string, percent int) {
	h.broadcast("update_progress", updateProgress{
		Step:    step,
		Status:  status,
		Message: message,
		Percent: percent,
	})
}

func (h *UpdateHandler) runUpdate(userID string) {
	defer h.updating.Store(false)

	// Step 1: Fetch latest release
	h.sendProgress("check", "running", "Checking for latest release...", 0)
	release, err := updater.FetchLatestRelease()
	if err != nil {
		h.sendProgress("check", "error", "Failed to fetch release: "+err.Error(), 0)
		return
	}
	h.sendProgress("check", "done", "Found "+release.TagName, 10)

	// Step 2: Download checksums
	h.sendProgress("checksum", "running", "Downloading checksums...", 15)
	binName := updater.BinaryName()
	checksums, err := updater.FetchChecksums(release.TagName)
	if err != nil {
		h.sendProgress("checksum", "error", "Failed to download checksums: "+err.Error(), 15)
		return
	}
	expectedHash, ok := checksums[binName]
	if !ok {
		h.sendProgress("checksum", "error", fmt.Sprintf("No checksum found for %s", binName), 15)
		return
	}
	h.sendProgress("checksum", "done", "Checksums verified", 20)

	// Step 3: Download binary with progress
	h.sendProgress("download", "running", "Downloading "+binName+"...", 25)
	var lastPercent int
	tmpPath, err := updater.DownloadBinaryWithProgress(release.TagName, binName, func(downloaded, total int64) {
		if total <= 0 {
			return
		}
		pct := int(float64(downloaded) / float64(total) * 100)
		// Throttle: only broadcast every 2%
		if pct-lastPercent >= 2 || pct == 100 {
			lastPercent = pct
			// Map download progress to 25-70% of overall progress
			overall := 25 + (pct * 45 / 100)
			h.sendProgress("download", "running", fmt.Sprintf("Downloading... %d%%", pct), overall)
		}
	})
	if err != nil {
		h.sendProgress("download", "error", "Download failed: "+err.Error(), 25)
		return
	}
	defer os.Remove(tmpPath)
	h.sendProgress("download", "done", "Download complete", 70)

	// Step 4: Verify checksum
	h.sendProgress("verify", "running", "Verifying SHA256 checksum...", 75)
	if err := updater.VerifyChecksum(tmpPath, expectedHash); err != nil {
		h.sendProgress("verify", "error", "Checksum verification failed: "+err.Error(), 75)
		return
	}
	h.sendProgress("verify", "done", "Checksum verified", 80)

	// Step 5: Replace binary
	h.sendProgress("install", "running", "Installing update...", 85)
	if err := updater.ReplaceBinary(tmpPath); err != nil {
		h.sendProgress("install", "error", "Installation failed: "+err.Error(), 85)
		return
	}

	latest, _ := updater.ParseSemVer(release.TagName)
	updater.SaveCache(h.dataDir, &updater.CachedCheck{
		LatestVersion: latest.String(),
		CheckedAt:     time.Now(),
	})

	h.db.LogAudit(userID, "system_updated", "system", "system", "", fmt.Sprintf("Updated to %s", release.TagName))
	logger.Success("Binary updated to %s", release.TagName)
	h.sendProgress("install", "done", "Update installed", 90)

	// Step 6: Restart
	h.sendProgress("restart", "running", "Restarting server...", 95)
	time.Sleep(500 * time.Millisecond) // Allow WS message delivery
	updater.RequestRestart()
}

