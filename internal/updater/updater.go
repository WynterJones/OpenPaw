package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

type InstallMethod int

const (
	InstallDirect  InstallMethod = iota
	InstallNPM
	InstallBrew
)

func (m InstallMethod) String() string {
	switch m {
	case InstallNPM:
		return "npm"
	case InstallBrew:
		return "homebrew"
	default:
		return "direct"
	}
}

type ReleaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

const githubReleasesURL = "https://api.github.com/repos/WynterJones/OpenPaw/releases/latest"

// DetectInstallMethod checks the binary path to determine how OpenPaw was installed.
func DetectInstallMethod() InstallMethod {
	exe, err := os.Executable()
	if err != nil {
		return InstallDirect
	}
	lower := strings.ToLower(exe)
	if strings.Contains(lower, "node_modules") || strings.Contains(lower, "npm") {
		return InstallNPM
	}
	if strings.Contains(lower, "/cellar/") || strings.Contains(lower, "/homebrew/") {
		return InstallBrew
	}
	return InstallDirect
}

// FetchLatestRelease queries GitHub for the latest release tag.
func FetchLatestRelease() (*ReleaseInfo, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", githubReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "openpaw-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var info ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}
	return &info, nil
}

// BinaryName returns the expected release artifact name for the current platform.
func BinaryName() string {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	var osStr string
	switch osName {
	case "darwin":
		osStr = "darwin"
	case "linux":
		osStr = "linux"
	case "windows":
		osStr = "win32"
	default:
		osStr = osName
	}

	var archStr string
	switch arch {
	case "amd64":
		archStr = "x64"
	case "arm64":
		archStr = "arm64"
	default:
		archStr = arch
	}

	name := fmt.Sprintf("openpaw-%s-%s", osStr, archStr)
	if osName == "windows" {
		name += ".exe"
	}
	return name
}

// FetchChecksums downloads the checksums.txt file from a release and returns a map of filename→sha256.
func FetchChecksums(tag string) (map[string]string, error) {
	url := fmt.Sprintf("https://github.com/WynterJones/OpenPaw/releases/download/%s/checksums.txt", tag)
	client := &http.Client{Timeout: 15 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checksums download returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read checksums: %w", err)
	}

	checksums := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 {
			checksums[parts[1]] = parts[0]
		}
	}
	return checksums, nil
}

// VerifyChecksum computes the SHA256 of the file and compares it to expectedHex.
func VerifyChecksum(filePath, expectedHex string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expectedHex) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHex, actual)
	}
	return nil
}

// DownloadBinary downloads a release binary to a temporary file and returns the path.
func DownloadBinary(tag, binaryName string) (string, error) {
	url := fmt.Sprintf("https://github.com/WynterJones/OpenPaw/releases/download/%s/%s", tag, binaryName)
	client := &http.Client{Timeout: 5 * time.Minute}

	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("binary download returned %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "openpaw-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write binary: %w", err)
	}
	tmpFile.Close()

	return tmpFile.Name(), nil
}

// ReplaceBinary atomically replaces the current binary with the new one.
func ReplaceBinary(newBinaryPath string) error {
	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate current binary: %w", err)
	}

	// Get current binary's permissions
	info, err := os.Stat(currentPath)
	if err != nil {
		return fmt.Errorf("failed to stat current binary: %w", err)
	}

	// Set executable permissions on the new binary
	if err := os.Chmod(newBinaryPath, info.Mode()); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Test write permission by opening current path for writing
	testFile, err := os.OpenFile(currentPath, os.O_WRONLY, 0)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied — try running with sudo: sudo %s update", currentPath)
		}
		return fmt.Errorf("cannot write to binary location: %w", err)
	}
	testFile.Close()

	newPath := currentPath + ".new"
	oldPath := currentPath + ".old"

	// Copy new binary to .new
	src, err := os.Open(newBinaryPath)
	if err != nil {
		return fmt.Errorf("failed to open new binary: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(newPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("failed to create .new file: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(newPath)
		return fmt.Errorf("failed to write .new file: %w", err)
	}
	dst.Close()

	// Rename current → .old
	os.Remove(oldPath) // remove stale .old if present
	if err := os.Rename(currentPath, oldPath); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Rename .new → current
	if err := os.Rename(newPath, currentPath); err != nil {
		// Attempt rollback
		os.Rename(oldPath, currentPath)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Clean up .old
	os.Remove(oldPath)

	return nil
}
