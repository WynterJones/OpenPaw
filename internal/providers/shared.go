package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	llm "github.com/openpaw/openpaw/internal/llm"
)

const (
	// maxConcurrentCLI limits simultaneous CLI processes per provider so
	// delegate/spawn fan-out doesn't slam subscription rate limits.
	maxConcurrentCLI = 3
	probeInterval    = 60 * time.Second
	probeTimeout     = 5 * time.Second
	scannerBuffer    = 10 << 20 // 10MB per JSONL line (tool outputs can be large)
)

// probeState caches CLI binary availability checks.
type probeState struct {
	mu        sync.Mutex
	checkedAt time.Time
	available bool
	version   string
	path      string
	loggedIn  bool
}

// probeBinary checks PATH for the binary and runs `<bin> --version`, caching
// the result. extra (optional) runs an additional auth check.
func (ps *probeState) probe(binName string, extra func(path string) bool) (bool, string, string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if time.Since(ps.checkedAt) < probeInterval {
		return ps.available, ps.version, ps.path
	}
	ps.checkedAt = time.Now()
	ps.available = false
	ps.version = ""
	ps.path = ""
	ps.loggedIn = false

	path, err := exec.LookPath(binName)
	if err != nil {
		return false, "", ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return false, "", ""
	}

	ps.available = true
	ps.version = strings.TrimSpace(string(out))
	ps.path = path
	if extra != nil {
		ps.loggedIn = extra(path)
	} else {
		ps.loggedIn = true
	}
	return ps.available, ps.version, ps.path
}

func (ps *probeState) isLoggedIn() bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.loggedIn
}

// acquireSem takes a semaphore slot, respecting context cancellation.
func acquireSem(ctx context.Context, sem chan struct{}) error {
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// runJSONL executes a prepared command, feeding stdin and invoking onLine for
// every line of stdout. Returns the tail of stderr for error reporting.
func runJSONL(cmd *exec.Cmd, stdin string, onLine func(line []byte)) (string, error) {
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, limit: 16 << 10}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	setupProcessGroup(cmd)
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), scannerBuffer)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		onLine(line)
	}
	scanErr := scanner.Err()

	waitErr := cmd.Wait()
	stderrTail := strings.TrimSpace(stderr.String())
	if waitErr != nil {
		return stderrTail, fmt.Errorf("%w: %s", waitErr, tail(stderrTail, 500))
	}
	if scanErr != nil {
		return stderrTail, fmt.Errorf("output read error: %w", scanErr)
	}
	return stderrTail, nil
}

type limitedWriter struct {
	w     *bytes.Buffer
	limit int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.w.Len() < lw.limit {
		lw.w.Write(p[:min(len(p), lw.limit-lw.w.Len())])
	}
	return len(p), nil
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// buildReplayPrompt embeds prior conversation history into a fresh-session
// prompt (used when no native session exists or resume failed).
func buildReplayPrompt(history []llm.HistoryMessage, userMessage string) string {
	if len(history) == 0 {
		return userMessage
	}
	var sb strings.Builder
	sb.WriteString("## CONVERSATION SO FAR (prior context — do not repeat)\n\n")
	for _, h := range history {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", h.Role, h.Content))
	}
	sb.WriteString("## NEW MESSAGE\n\n")
	sb.WriteString(userMessage)
	return sb.String()
}

// flattenToolInput parses tool input JSON into the map StreamEvent expects.
func flattenToolInput(raw []byte) map[string]interface{} {
	var m map[string]interface{}
	if len(raw) > 0 {
		json.Unmarshal(raw, &m)
	}
	return m
}
