package backup

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/openpaw/openpaw/internal/logger"
)

type GitMethod struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// DetectGitMethod checks for gh CLI or git availability.
func DetectGitMethod() GitMethod {
	return detectGitMethod()
}

func detectGitMethod() GitMethod {
	// Try gh first
	if out, err := exec.Command("gh", "--version").Output(); err == nil {
		ver := strings.TrimSpace(strings.Split(string(out), "\n")[0])
		// Check if authenticated
		if err := exec.Command("gh", "auth", "status").Run(); err == nil {
			return GitMethod{Name: "gh", Version: ver}
		}
	}

	// Fall back to git
	if out, err := exec.Command("git", "--version").Output(); err == nil {
		ver := strings.TrimSpace(string(out))
		return GitMethod{Name: "git", Version: ver}
	}

	return GitMethod{Name: "none", Version: ""}
}

// TestConnection validates that the repo URL + credentials work.
func TestConnection(repoURL, authToken, authMethod string) error {
	return testConnection(repoURL, authToken, authMethod)
}

func testConnection(repoURL, authToken, authMethod string) error {
	tmpDir, err := os.MkdirTemp("", "openpaw-backup-test-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneURL, err := injectAuth(repoURL, authToken, authMethod)
	if err != nil {
		return err
	}

	cmd := exec.Command("git", "clone", "--depth", "1", cloneURL, tmpDir)
	cmd.Env = sanitizeEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's just an empty repo (expected for first use)
		if strings.Contains(string(out), "empty repository") || strings.Contains(string(out), "warning: You appear to have cloned an empty repository") {
			return nil
		}
		return fmt.Errorf("clone failed: %s", strings.TrimSpace(string(out)))
	}

	return nil
}

func cloneOrInit(repoURL, authToken, authMethod, workDir string) error {
	cloneURL, err := injectAuth(repoURL, authToken, authMethod)
	if err != nil {
		return err
	}

	cmd := exec.Command("git", "clone", cloneURL, workDir)
	cmd.Env = sanitizeEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := string(out)
		if strings.Contains(outStr, "empty repository") || strings.Contains(outStr, "warning: You appear to have cloned an empty repository") {
			// Init a fresh repo instead
			if err := initFreshRepo(cloneURL, workDir); err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("clone failed: %s", strings.TrimSpace(outStr))
	}

	return nil
}

func initFreshRepo(remoteURL, workDir string) error {
	os.MkdirAll(workDir, 0755)

	cmds := [][]string{
		{"git", "init"},
		{"git", "remote", "add", "origin", remoteURL},
		{"git", "checkout", "-b", "main"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = workDir
		cmd.Env = sanitizeEnv()
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s failed: %s", args[0], strings.TrimSpace(string(out)))
		}
	}

	return nil
}

func commitAndPush(workDir string) (string, error) {
	// Configure git user for this repo
	for _, kv := range [][2]string{
		{"user.name", "OpenPaw \xf0\x9f\x90\xbe"},
		{"user.email", "backup@openpaw.local"},
	} {
		cmd := exec.Command("git", "config", kv[0], kv[1])
		cmd.Dir = workDir
		cmd.Env = sanitizeEnv()
		cmd.Run()
	}

	// Stage all files
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = workDir
	cmd.Env = sanitizeEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add failed: %s", strings.TrimSpace(string(out)))
	}

	// Check if there are changes
	cmd = exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = workDir
	cmd.Env = sanitizeEnv()
	if err := cmd.Run(); err == nil {
		logger.Info("Backup: no changes to commit")
		return "", nil
	}

	// Commit
	msg := fmt.Sprintf("OpenPaw backup %s", strings.Replace(
		strings.Replace(currentTime().Format("2006-01-02T15:04:05Z"), "T", " ", 1),
		"Z", " UTC", 1))
	cmd = exec.Command("git", "commit", "-m", msg)
	cmd.Dir = workDir
	cmd.Env = sanitizeEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit failed: %s", strings.TrimSpace(string(out)))
	}

	// Get commit SHA
	cmd = exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = workDir
	cmd.Env = sanitizeEnv()
	shaOut, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get SHA failed: %w", err)
	}
	sha := strings.TrimSpace(string(shaOut))

	// Push
	cmd = exec.Command("git", "push", "-u", "origin", "main")
	cmd.Dir = workDir
	cmd.Env = sanitizeEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return sha, fmt.Errorf("git push failed: %s", strings.TrimSpace(string(out)))
	}

	return sha, nil
}

func injectAuth(repoURL, authToken, authMethod string) (string, error) {
	if authMethod == "gh_cli" || authToken == "" {
		return repoURL, nil
	}

	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("parse repo URL: %w", err)
	}

	u.User = url.UserPassword("x-access-token", authToken)
	return u.String(), nil
}

func sanitizeEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		env = append(env, e)
	}
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	return env
}

func removeOldFiles(workDir string) error {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		os.RemoveAll(filepath.Join(workDir, entry.Name()))
	}
	return nil
}
