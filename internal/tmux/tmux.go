// Package tmux inspects running tmux sessions so the UI can show what a
// long-lived CLI coding session (Claude Code, Codex) is doing.
//
// Everything here is read-only and best-effort: tmux may not be installed, the
// session may be a plain shell, and the CLI's status line is presentation, not
// an API. Callers get whatever could be parsed plus the raw tail, and nothing
// fails hard just because a field was missing.
package tmux

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Session is one tmux session plus whatever could be read from its pane.
type Session struct {
	Name     string    `json:"name"`
	Windows  int       `json:"windows"`
	Created  time.Time `json:"created"`
	Attached bool      `json:"attached"`

	// Kind is "claude", "codex" or "shell".
	Kind string `json:"kind"`

	Status *Status `json:"status,omitempty"`

	// Tail is the last few non-empty pane lines, always populated so the UI can
	// show something even when nothing parsed.
	Tail []string `json:"tail,omitempty"`
}

// Status is the structured form of a Claude Code status line:
//
//	╭╼ WynterAI (main) +23 | Rails
//	╰──╼ Opus 5 (1M context) | ━━──────── 22% (smart) | 39m22s | +2255/-1991
//	⏵⏵ auto mode on (shift+tab to cycle) · ← 1 agent
type Status struct {
	Project      string `json:"project,omitempty"`
	Branch       string `json:"branch,omitempty"`
	Uncommitted  int    `json:"uncommitted,omitempty"`
	Framework    string `json:"framework,omitempty"`
	Model        string `json:"model,omitempty"`
	ContextPct   int    `json:"context_pct,omitempty"`
	Elapsed      string `json:"elapsed,omitempty"`
	LinesAdded   int    `json:"lines_added,omitempty"`
	LinesRemoved int    `json:"lines_removed,omitempty"`
	AutoMode     bool   `json:"auto_mode"`
	Agents       int    `json:"agents,omitempty"`
}

const tailLines = 6

// Available reports whether tmux is installed.
func Available() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// List returns every running tmux session with its parsed pane state.
func List(ctx context.Context) ([]Session, error) {
	if !Available() {
		return nil, nil
	}

	out, err := run(ctx, "list-sessions", "-F",
		"#{session_name}\x1f#{session_windows}\x1f#{session_created}\x1f#{session_attached}")
	if err != nil {
		// No server running is the normal "nothing to show" case, not an error.
		return nil, nil
	}

	var sessions []Session
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) < 4 {
			continue
		}
		s := Session{Name: parts[0], Kind: "shell"}
		s.Windows, _ = strconv.Atoi(parts[1])
		if secs, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
			s.Created = time.Unix(secs, 0)
		}
		s.Attached = parts[3] != "0"

		if pane, err := run(ctx, "capture-pane", "-p", "-t", s.Name); err == nil {
			s.Tail = tail(pane, tailLines)
			s.Kind = detectKind(pane)
			s.Status = ParseStatus(pane)
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// Capture returns the current pane text for one session.
func Capture(ctx context.Context, name string) (string, error) {
	return run(ctx, "capture-pane", "-p", "-t", name)
}

// Exists reports whether a session is still running — the signal a watcher uses
// to stop polling.
func Exists(ctx context.Context, name string) bool {
	if !Available() {
		return false
	}
	_, err := run(ctx, "has-session", "-t", name)
	return err == nil
}

func run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", args...).Output()
	return string(out), err
}

func detectKind(pane string) string {
	switch {
	case strings.Contains(pane, "auto mode on"), strings.Contains(pane, "(shift+tab to cycle)"):
		return "claude"
	case strings.Contains(pane, "codex"), strings.Contains(pane, "Codex"):
		return "codex"
	default:
		return "shell"
	}
}

func tail(pane string, n int) []string {
	var lines []string
	for _, l := range strings.Split(pane, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimRight(l, " \t"))
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// ParseStatus pulls the structured fields out of a Claude Code status line.
// Returns nil when the pane holds no recognisable status block.
func ParseStatus(pane string) *Status {
	var st Status
	found := false

	for _, raw := range strings.Split(pane, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.Contains(line, "╭╼"):
			parseProjectLine(after(line, "╭╼"), &st)
			found = true
		case strings.Contains(line, "╰─"):
			parseModelLine(afterArrow(line), &st)
			found = true
		case strings.Contains(line, "⏵⏵"):
			parseModeLine(line, &st)
			found = true
		}
	}
	if !found {
		return nil
	}
	return &st
}

// after returns the text following a marker, trimmed.
func after(line, marker string) string {
	i := strings.Index(line, marker)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(line[i+len(marker):])
}

// afterArrow handles the model line's variable-length "╰──╼" prefix.
func afterArrow(line string) string {
	if i := strings.Index(line, "╼"); i >= 0 {
		return strings.TrimSpace(line[i+len("╼"):])
	}
	return ""
}

// parseProjectLine reads "WynterAI (main) +23 | Rails".
func parseProjectLine(s string, st *Status) {
	segs := strings.Split(s, "|")
	head := strings.TrimSpace(segs[0])
	if len(segs) > 1 {
		st.Framework = strings.TrimSpace(segs[1])
	}

	// Trailing "+23" = uncommitted changes.
	if i := strings.LastIndex(head, " +"); i >= 0 {
		if n, err := strconv.Atoi(strings.TrimSpace(head[i+2:])); err == nil {
			st.Uncommitted = n
			head = strings.TrimSpace(head[:i])
		}
	}
	// "(main)" = branch.
	if open := strings.LastIndex(head, "("); open >= 0 {
		if close := strings.Index(head[open:], ")"); close >= 0 {
			st.Branch = head[open+1 : open+close]
			head = strings.TrimSpace(head[:open])
		}
	}
	st.Project = strings.TrimSpace(head)
}

// parseModelLine reads "Opus 5 (1M context) | ━━──── 22% (smart) | 39m22s | +2255/-1991".
func parseModelLine(s string, st *Status) {
	for i, seg := range strings.Split(s, "|") {
		seg = strings.TrimSpace(seg)
		switch {
		case i == 0:
			st.Model = seg
		case strings.Contains(seg, "%"):
			// Strip the bar glyphs, keep the number before '%'.
			if pct := strings.Index(seg, "%"); pct > 0 {
				digits := trailingDigits(seg[:pct])
				if n, err := strconv.Atoi(digits); err == nil {
					st.ContextPct = n
				}
			}
		case strings.HasPrefix(seg, "+") && strings.Contains(seg, "/-"):
			add, rem, _ := strings.Cut(seg, "/-")
			st.LinesAdded, _ = strconv.Atoi(strings.TrimPrefix(add, "+"))
			st.LinesRemoved, _ = strconv.Atoi(rem)
		case seg != "" && (strings.HasSuffix(seg, "s") || strings.Contains(seg, "m")):
			st.Elapsed = seg
		}
	}
}

// parseModeLine reads "⏵⏵ auto mode on (shift+tab to cycle) · ← 1 agent".
func parseModeLine(line string, st *Status) {
	st.AutoMode = strings.Contains(line, "auto mode on")
	if i := strings.Index(line, " agent"); i > 0 {
		if n, err := strconv.Atoi(trailingDigits(line[:i])); err == nil {
			st.Agents = n
		}
	}
}

// trailingDigits returns the run of digits at the end of s.
func trailingDigits(s string) string {
	end := len(s)
	for end > 0 {
		c := s[end-1]
		if c < '0' || c > '9' {
			break
		}
		end--
	}
	return s[end:]
}
