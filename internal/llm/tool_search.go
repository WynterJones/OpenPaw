package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func handleGrep(ctx context.Context, workDir string, input json.RawMessage) ToolResult {
	var params struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		Glob       string `json:"glob"`
		OutputMode string `json:"output_mode"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
	}

	searchPath := workDir
	if params.Path != "" {
		searchPath = resolvePath(workDir, params.Path)
	}

	// Try ripgrep first, fall back to grep
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return grepFallback(ctx, params.Pattern, searchPath, params.Glob, params.OutputMode)
	}

	args := []string{"--no-heading", "--line-number"}
	switch params.OutputMode {
	case "files_with_matches", "":
		args = append(args, "--files-with-matches")
	case "count":
		args = append(args, "--count")
	default:
		// "content" — default rg behavior
	}
	if params.Glob != "" {
		args = append(args, "--glob", params.Glob)
	}
	args = append(args, params.Pattern, searchPath)

	cmd := exec.CommandContext(ctx, rgPath, args...)
	out, err := cmd.CombinedOutput()
	output := string(out)

	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes] + "\n... [output truncated]"
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return ToolResult{Output: "No matches found"}
		}
		if strings.TrimSpace(output) != "" {
			return ToolResult{Output: output, IsError: true}
		}
		return ToolResult{Output: "Search error: " + err.Error(), IsError: true}
	}

	if strings.TrimSpace(output) == "" {
		return ToolResult{Output: "No matches found"}
	}
	return ToolResult{Output: output}
}

func grepFallback(ctx context.Context, pattern, path, glob, mode string) ToolResult {
	args := []string{"-r", "-n"}
	switch mode {
	case "files_with_matches", "":
		args = append(args, "-l")
	case "count":
		args = append(args, "-c")
	}
	if glob != "" {
		args = append(args, "--include="+glob)
	}
	args = append(args, pattern, path)

	cmd := exec.CommandContext(ctx, "grep", args...)
	out, _ := cmd.CombinedOutput()
	output := string(out)
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes] + "\n... [output truncated]"
	}
	if strings.TrimSpace(output) == "" {
		return ToolResult{Output: "No matches found"}
	}
	return ToolResult{Output: output}
}

func handleGlob(_ context.Context, workDir string, input json.RawMessage) ToolResult {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
	}

	base := workDir
	if params.Path != "" {
		base = resolvePath(workDir, params.Path)
	}

	fullPattern := filepath.Join(base, params.Pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Glob error: %v", err), IsError: true}
	}

	if len(matches) == 0 {
		return ToolResult{Output: "No files matched"}
	}

	// Limit results
	if len(matches) > 500 {
		matches = matches[:500]
	}
	return ToolResult{Output: strings.Join(matches, "\n")}
}
