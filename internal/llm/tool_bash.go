package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultBashTimeout = 120 * time.Second
	maxBashTimeout     = 600 * time.Second
	maxOutputBytes     = 30 * 1024
)

var dangerousPatterns = []string{
	"rm -rf /",
	"rm -rf /*",
	"mkfs.",
	"dd if=",
	":(){ :|:& };:",
	"> /dev/sd",
	"> /dev/nvme",
	"shutdown",
	"reboot",
	"halt",
	"poweroff",
	"init 0",
	"init 6",
	"chmod -R 777 /",
	"chown -R",
}

func isDangerousCommand(cmd string) (bool, string) {
	normalized := strings.TrimSpace(strings.ToLower(cmd))
	for _, pattern := range dangerousPatterns {
		if strings.Contains(normalized, strings.ToLower(pattern)) {
			return true, pattern
		}
	}
	return false, ""
}

func handleBash(ctx context.Context, workDir string, input json.RawMessage) ToolResult {
	var params struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
	}

	// Check for dangerous commands before execution
	if dangerous, pattern := isDangerousCommand(params.Command); dangerous {
		return ToolResult{
			Output:  fmt.Sprintf("Command blocked: matches dangerous pattern '%s'. This command could cause system damage.", pattern),
			IsError: true,
		}
	}

	timeout := defaultBashTimeout
	if params.Timeout > 0 {
		timeout = time.Duration(params.Timeout) * time.Second
		if timeout > maxBashTimeout {
			timeout = maxBashTimeout
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "bash", "-c", params.Command)
	cmd.Dir = workDir

	out, err := cmd.CombinedOutput()
	output := string(out)
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes] + "\n... [output truncated]"
	}

	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return ToolResult{
				Output:  fmt.Sprintf("Command timed out after %s\n%s", timeout, output),
				IsError: true,
			}
		}
		exitMsg := ""
		if len(strings.TrimSpace(output)) > 0 {
			exitMsg = output + "\n"
		}
		return ToolResult{
			Output:  exitMsg + "Exit code: " + err.Error(),
			IsError: true,
		}
	}

	if len(strings.TrimSpace(output)) == 0 {
		output = "(no output)"
	}
	return ToolResult{Output: output}
}
