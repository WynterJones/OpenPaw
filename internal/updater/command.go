package updater

import (
	"fmt"
	"os"
	"time"

	"github.com/openpaw/openpaw/internal/logger"
)

// RunUpdateCommand handles the `openpaw update` CLI command.
func RunUpdateCommand(currentVersion, dataDir string) {
	// Check install method
	method := DetectInstallMethod()
	if method == InstallNPM {
		logger.Info("OpenPaw was installed via npm. Update with:")
		fmt.Println("  npm update -g openpaw")
		os.Exit(0)
	}
	if method == InstallBrew {
		logger.Info("OpenPaw was installed via Homebrew. Update with:")
		fmt.Println("  brew upgrade openpaw")
		os.Exit(0)
	}

	// Reject dev builds
	if currentVersion == "dev" {
		logger.Error("Cannot self-update a development build.")
		os.Exit(1)
	}

	current, err := ParseSemVer(currentVersion)
	if err != nil {
		logger.Error("Cannot parse current version %q: %v", currentVersion, err)
		os.Exit(1)
	}

	// Fetch latest release
	logger.Info("Checking for updates...")
	release, err := FetchLatestRelease()
	if err != nil {
		logger.Error("Failed to check for updates: %v", err)
		os.Exit(1)
	}

	latest, err := ParseSemVer(release.TagName)
	if err != nil {
		logger.Error("Cannot parse latest version %q: %v", release.TagName, err)
		os.Exit(1)
	}

	if !latest.IsNewer(current) {
		logger.Success("Already up to date (v%s).", current)
		saveQuietly(dataDir, latest.String())
		os.Exit(0)
	}

	logger.Info("Update available: v%s -> v%s", current, latest)

	// Download checksums and verify expected binary exists
	binName := BinaryName()
	logger.Info("Downloading checksums...")
	checksums, err := FetchChecksums(release.TagName)
	if err != nil {
		logger.Error("Failed to download checksums: %v", err)
		os.Exit(1)
	}

	expectedHash, ok := checksums[binName]
	if !ok {
		logger.Error("No checksum found for %s in release %s", binName, release.TagName)
		os.Exit(1)
	}

	// Download binary
	logger.Info("Downloading %s...", binName)
	tmpPath, err := DownloadBinary(release.TagName, binName)
	if err != nil {
		logger.Error("Failed to download binary: %v", err)
		os.Exit(1)
	}
	defer os.Remove(tmpPath)

	// Verify checksum
	logger.Info("Verifying checksum...")
	if err := VerifyChecksum(tmpPath, expectedHash); err != nil {
		logger.Error("Checksum verification failed: %v", err)
		os.Exit(1)
	}
	logger.Success("Checksum verified.")

	// Replace binary
	logger.Info("Installing update...")
	if err := ReplaceBinary(tmpPath); err != nil {
		logger.Error("Failed to install update: %v", err)
		os.Exit(1)
	}

	saveQuietly(dataDir, latest.String())
	logger.Success("Updated to v%s. Restart OpenPaw to use the new version.", latest)
	os.Exit(0)
}

func saveQuietly(dataDir, version string) {
	SaveCache(dataDir, &CachedCheck{
		LatestVersion: version,
		CheckedAt:     time.Now(),
	})
}
