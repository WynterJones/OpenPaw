package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ToolResult struct {
	Output  string
	IsError bool
}

type ToolHandler func(ctx context.Context, workDir string, input json.RawMessage) ToolResult

type ToolExecutor struct {
	workDir      string
	handlers     map[string]ToolHandler
	sandboxPaths []string
}

func NewToolExecutor(workDir string) *ToolExecutor {
	te := &ToolExecutor{
		workDir:  workDir,
		handlers: make(map[string]ToolHandler),
	}
	te.handlers["Read"] = handleRead
	te.handlers["Write"] = handleWrite
	te.handlers["Edit"] = handleEdit
	te.handlers["Bash"] = handleBash
	te.handlers["Grep"] = handleGrep
	te.handlers["Glob"] = handleGlob
	te.handlers["WebFetch"] = handleWebFetch
	te.handlers["WebSearch"] = handleWebSearch
	te.handlers["NotebookEdit"] = handleNotebookEdit
	return te
}

// NewSandboxedToolExecutor creates a tool executor that restricts file operations
// (Read, Write, Edit) to the given sandbox paths. Other tools (Bash, etc.) are excluded.
func NewSandboxedToolExecutor(workDir string, sandboxPaths []string, tools []string) *ToolExecutor {
	te := &ToolExecutor{
		workDir:      workDir,
		handlers:     make(map[string]ToolHandler),
		sandboxPaths: sandboxPaths,
	}

	allowed := make(map[string]bool, len(tools))
	for _, t := range tools {
		allowed[t] = true
	}

	if allowed["Read"] {
		te.handlers["Read"] = te.sandboxedRead
	}
	if allowed["Write"] {
		te.handlers["Write"] = te.sandboxedWrite
	}
	if allowed["Edit"] {
		te.handlers["Edit"] = te.sandboxedEdit
	}
	return te
}

var sensitivePathPrefixes = []string{
	".ssh",
	".gnupg",
	".aws",
	".config/gcloud",
	".docker",
	".kube",
}

var sensitiveExactPaths = []string{
	"/etc/shadow",
	"/etc/master.passwd",
}

func isSensitivePath(filePath string) bool {
	resolved, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}

	for _, exact := range sensitiveExactPaths {
		if resolved == exact {
			return true
		}
	}

	home := os.Getenv("HOME")
	if home == "" {
		return false
	}

	for _, prefix := range sensitivePathPrefixes {
		sensitive := filepath.Join(home, prefix)
		if strings.HasPrefix(resolved, sensitive+string(filepath.Separator)) || resolved == sensitive {
			return true
		}
	}

	return false
}

func (te *ToolExecutor) Execute(ctx context.Context, name string, input json.RawMessage) ToolResult {
	handler, ok := te.handlers[name]
	if !ok {
		return ToolResult{
			Output:  fmt.Sprintf("Unknown tool: %s", name),
			IsError: true,
		}
	}

	// Check sensitive paths for file operations (even in non-sandboxed mode)
	if name == "Read" || name == "Write" || name == "Edit" {
		var pathCheck struct {
			FilePath string `json:"file_path"`
		}
		if err := json.Unmarshal(input, &pathCheck); err == nil && pathCheck.FilePath != "" {
			path := resolvePath(te.workDir, pathCheck.FilePath)
			if isSensitivePath(path) {
				return ToolResult{
					Output:  "Access denied: path contains sensitive system files",
					IsError: true,
				}
			}
		}
	}

	return handler(ctx, te.workDir, input)
}

// isPathAllowed checks if a resolved path is within any of the sandbox paths.
func (te *ToolExecutor) isPathAllowed(path string) bool {
	if len(te.sandboxPaths) == 0 {
		return true
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, allowed := range te.sandboxPaths {
		absAllowed, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		if strings.HasPrefix(resolved, absAllowed+string(filepath.Separator)) || resolved == absAllowed {
			return true
		}
	}
	return false
}

func (te *ToolExecutor) sandboxedRead(ctx context.Context, workDir string, input json.RawMessage) ToolResult {
	var params struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
	}
	path := resolvePath(workDir, params.FilePath)
	if !te.isPathAllowed(path) {
		return ToolResult{Output: "Access denied: path outside sandbox", IsError: true}
	}
	return handleRead(ctx, workDir, input)
}

func (te *ToolExecutor) sandboxedWrite(ctx context.Context, workDir string, input json.RawMessage) ToolResult {
	var params struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
	}
	path := resolvePath(workDir, params.FilePath)
	if !te.isPathAllowed(path) {
		return ToolResult{Output: "Access denied: path outside sandbox", IsError: true}
	}
	return handleWrite(ctx, workDir, input)
}

func (te *ToolExecutor) sandboxedEdit(ctx context.Context, workDir string, input json.RawMessage) ToolResult {
	var params struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
	}
	path := resolvePath(workDir, params.FilePath)
	if !te.isPathAllowed(path) {
		return ToolResult{Output: "Access denied: path outside sandbox", IsError: true}
	}
	return handleEdit(ctx, workDir, input)
}
