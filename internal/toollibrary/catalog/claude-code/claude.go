package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	cliTimeout        = 10 * time.Minute
	sensitivePathPrefixes = []string{
		".ssh", ".aws", ".gnupg", ".config/gcloud",
	}
	sensitiveAbsPrefixes = []string{
		"/etc", "/var",
	}
)

type claudeRequest struct {
	Directory string `json:"directory"`
	Prompt    string `json:"prompt"`
	Model     string `json:"model"`
}

func registerRoutes(r chi.Router) {
	r.Post("/plan", handlePlan)
	r.Post("/implement", handleImplement)
	r.Post("/ask", handleAsk)
	r.Get("/status", handleStatus)
}

func getAllowedDirectories() []string {
	envVal := os.Getenv("ALLOWED_DIRECTORIES")
	if envVal == "" {
		return nil
	}
	var dirs []string
	if err := json.Unmarshal([]byte(envVal), &dirs); err != nil {
		return nil
	}
	return dirs
}

func validateDirectory(dir string) error {
	if dir == "" {
		return fmt.Errorf("directory is required")
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("invalid directory path: %w", err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		return fmt.Errorf("directory does not exist: %s", absDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absDir)
	}

	for _, prefix := range sensitiveAbsPrefixes {
		if strings.HasPrefix(absDir, prefix) {
			return fmt.Errorf("access to %s is not allowed", prefix)
		}
	}

	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		for _, sensitive := range sensitivePathPrefixes {
			blocked := filepath.Join(homeDir, sensitive)
			if strings.HasPrefix(absDir, blocked) {
				return fmt.Errorf("access to ~/%s is not allowed", sensitive)
			}
		}
	}

	allowed := getAllowedDirectories()
	if len(allowed) > 0 {
		found := false
		for _, a := range allowed {
			allowedAbs, err := filepath.Abs(a)
			if err != nil {
				continue
			}
			if strings.HasPrefix(absDir, allowedAbs) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("directory not in allowed list: %s", absDir)
		}
	}

	return nil
}

func runClaude(req claudeRequest, extraArgs ...string) (map[string]interface{}, error) {
	if err := validateDirectory(req.Directory); err != nil {
		return nil, err
	}

	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found in PATH")
	}

	absDir, _ := filepath.Abs(req.Directory)

	args := []string{"-p", req.Prompt, "--output-format", "stream-json"}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	args = append(args, extraArgs...)

	ctx, cancel := context.WithTimeout(context.Background(), cliTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, claudePath, args...)
	cmd.Dir = absDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("command timed out after %s", cliTimeout)
		}
		return nil, fmt.Errorf("claude command failed: %s (stderr: %s)", err, stderr.String())
	}

	output := stdout.String()

	var lastResult map[string]interface{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err == nil {
			if msg["type"] == "result" {
				lastResult = msg
			}
		}
	}

	if lastResult != nil {
		return map[string]interface{}{
			"success":   true,
			"result":    lastResult,
			"directory": absDir,
		}, nil
	}

	return map[string]interface{}{
		"success":    true,
		"raw_output": output,
		"directory":  absDir,
	}, nil
}

func handlePlan(w http.ResponseWriter, r *http.Request) {
	var req claudeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := runClaude(req, "--allowedTools", "Read,Grep,Glob", "--max-turns", "5")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result["mode"] = "plan"
	writeJSON(w, http.StatusOK, result)
}

func handleImplement(w http.ResponseWriter, r *http.Request) {
	var req claudeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := runClaude(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result["mode"] = "implement"
	writeJSON(w, http.StatusOK, result)
}

func handleAsk(w http.ResponseWriter, r *http.Request) {
	var req claudeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := runClaude(req, "--max-turns", "1")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result["mode"] = "ask"
	writeJSON(w, http.StatusOK, result)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"available": false,
			"error":     "claude CLI not found in PATH",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"available":           true,
		"path":                claudePath,
		"allowed_directories": getAllowedDirectories(),
	})
}
