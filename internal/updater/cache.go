package updater

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const cacheTTL = 24 * time.Hour

type CachedCheck struct {
	LatestVersion string    `json:"latest_version"`
	CheckedAt     time.Time `json:"checked_at"`
}

func cachePath(dataDir string) string {
	return filepath.Join(dataDir, "update-check.json")
}

// LoadCache returns the cached version check, or nil if missing/expired/invalid.
func LoadCache(dataDir string) *CachedCheck {
	data, err := os.ReadFile(cachePath(dataDir))
	if err != nil {
		return nil
	}
	var cached CachedCheck
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil
	}
	if time.Since(cached.CheckedAt) > cacheTTL {
		return nil
	}
	return &cached
}

// SaveCache writes a version check result to disk.
func SaveCache(dataDir string, check *CachedCheck) error {
	data, err := json.Marshal(check)
	if err != nil {
		return err
	}
	return os.WriteFile(cachePath(dataDir), data, 0644)
}
