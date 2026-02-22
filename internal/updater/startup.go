package updater

import (
	"os"
	"time"

	"github.com/openpaw/openpaw/internal/logger"
)

// StartupCheck runs a non-blocking version check at startup.
// It silently returns on any error to avoid disrupting normal operation.
func StartupCheck(currentVersion, dataDir string) {
	if currentVersion == "dev" {
		return
	}
	if os.Getenv("OPENPAW_NO_UPDATE_CHECK") == "1" {
		return
	}

	current, err := ParseSemVer(currentVersion)
	if err != nil {
		return
	}

	// Check cache first to avoid unnecessary API calls
	if cached := LoadCache(dataDir); cached != nil {
		latest, err := ParseSemVer(cached.LatestVersion)
		if err == nil && latest.IsNewer(current) {
			logger.Info("Update available: v%s -> v%s (run 'openpaw update')", current, latest)
		}
		return
	}

	// Cache is stale or missing — fetch from GitHub
	release, err := FetchLatestRelease()
	if err != nil {
		return
	}

	latest, err := ParseSemVer(release.TagName)
	if err != nil {
		return
	}

	// Save to cache regardless of result
	SaveCache(dataDir, &CachedCheck{
		LatestVersion: latest.String(),
		CheckedAt:     time.Now(),
	})

	if latest.IsNewer(current) {
		logger.Info("Update available: v%s -> v%s (run 'openpaw update')", current, latest)
	}
}
