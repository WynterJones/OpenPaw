// Package tmux inspects running tmux sessions so the UI can show what a
// long-lived CLI coding session (Claude Code, Codex) is doing.
//
// Inspection here is best-effort: tmux may not be installed, the session may be
// a plain shell, and the CLI's status line is presentation, not an API. Callers
// get whatever could be parsed plus the raw tail, and nothing fails hard just
// because a field was missing. Start is the one write — it launches work that
// needs to outlive the request that asked for it.
package tmux

import (
	"context"
	"errors"
	"fmt"
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

	// Fields are space-separated with the name LAST, because tmux's format
	// expansion rewrites non-printable bytes: a \x1f separator comes back as a
	// literal "_", so every line failed to split and the list silently came back
	// empty. The numeric fields can't contain spaces, and putting the
	// user-controlled name at the end means a name with a space in it survives
	// intact rather than corrupting the parse.
	out, err := run(ctx, "list-sessions", "-F",
		"#{session_windows} #{session_created} #{session_attached} #{session_name}")
	if err != nil {
		// No server running is the normal "nothing to show" case, not an error.
		return nil, nil
	}

	var sessions []Session
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 4)
		if len(parts) < 4 {
			continue
		}
		s := Session{Name: parts[3], Kind: "shell"}
		s.Windows, _ = strconv.Atoi(parts[0])
		if secs, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			s.Created = time.Unix(secs, 0)
		}
		s.Attached = parts[2] != "0"

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
//
// Once a command exits, tmux paints "Pane is dead (status N)" over the pane and
// a plain capture returns mostly that overlay — the command's own output has
// scrolled into history. So for a finished pane we read the scrollback too,
// which is precisely when the output matters most: it is the only record of
// what the run actually did.
func Capture(ctx context.Context, name string) (string, error) {
	if dead, _, ok := Finished(ctx, name); ok && dead {
		if out, err := run(ctx, "capture-pane", "-p", "-S", "-", "-t", name); err == nil {
			return out, nil
		}
	}
	return run(ctx, "capture-pane", "-p", "-t", name)
}

// Finished reports whether a session's command has exited and with what status.
// ok is false when tmux could not answer (no session, no server, old tmux), so
// callers can tell "still running" apart from "cannot tell".
func Finished(ctx context.Context, name string) (dead bool, status int, ok bool) {
	if !Available() {
		return false, 0, false
	}
	// Space-separated for the same reason as List: tmux mangles non-printable
	// separators. Both fields are numeric, and pane_dead_status is empty while
	// the pane is still alive.
	out, err := run(ctx, "list-panes", "-t", name, "-F", "#{pane_dead} #{pane_dead_status}")
	if err != nil {
		return false, 0, false
	}
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false, 0, false
	}
	if len(fields) > 1 {
		status, _ = strconv.Atoi(fields[1])
	}
	return fields[0] == "1", status, true
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

// Start launches command in a new detached session named name, in workDir.
//
// The one write in this package, and the reason it exists: an agent's turn ends
// when it replies, so a build started as a plain subprocess dies with the turn.
// Detached in tmux it keeps running, the user can attach to it, and tmux_watch
// can report back when it finishes.
//
// The session stays alive after the command exits (remain-on-exit) so the exit
// status and last output are still readable — a session that vanished on
// completion would be indistinguishable from one that never started.
//
// The command is typed into an idle shell rather than passed to new-session,
// because remain-on-exit can only be set on a session that already exists.
// Launching the command directly races it: a quick one exits before the option
// lands, the session disappears, and the output and exit status are gone. An
// idle shell cannot outrun the setup.
func Start(ctx context.Context, name, workDir, command string) error {
	if !Available() {
		return errors.New("tmux is not installed")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("session name is required")
	}
	if strings.TrimSpace(command) == "" {
		return errors.New("command is required")
	}
	if Exists(ctx, name) {
		return fmt.Errorf("a tmux session named %q is already running", name)
	}

	// 1. An idle shell, which will sit there indefinitely.
	args := []string{"new-session", "-d", "-s", name}
	if workDir != "" {
		args = append(args, "-c", workDir)
	}
	if _, err := run(ctx, args...); err != nil {
		return fmt.Errorf("tmux new-session failed: %w", err)
	}

	// 2. Keep the pane after its command exits. Best-effort: an old tmux
	//    without the option just leaves the default and the session ends on
	//    completion, which the watcher still detects.
	run(ctx, "set-option", "-t", name, "remain-on-exit", "on")

	// 3. Type the command. The trailing "exit $?" hands the command's own
	//    status to the pane, which is what pane_dead_status then reports —
	//    without it every run would look like it succeeded.
	if _, err := run(ctx, "send-keys", "-t", name, command+"; exit $?", "Enter"); err != nil {
		run(ctx, "kill-session", "-t", name)
		return fmt.Errorf("tmux send-keys failed: %w", err)
	}
	return nil
}

// Kill ends a session and everything running inside it.
//
// The counterpart to Start: sessions deliberately outlive their command so the
// output stays readable, which means something has to clear them away. Killing
// the last session also stops the tmux server itself, which is tmux's own
// behaviour and the point — nothing is left running in the background.
func Kill(ctx context.Context, name string) error {
	if !Available() {
		return errors.New("tmux is not installed")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("session name is required")
	}
	if !Exists(ctx, name) {
		return fmt.Errorf("no tmux session named %q", name)
	}
	if _, err := run(ctx, "kill-session", "-t", name); err != nil {
		return fmt.Errorf("tmux kill-session failed: %w", err)
	}
	return nil
}

// SessionName turns an agent's requested label into a tmux-safe session name.
// tmux treats "." and ":" as window/pane separators, so a name containing them
// cannot be targeted afterwards.
func SessionName(label string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(label) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ', r == '.', r == ':', r == '/':
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "op-task"
	}
	if len(name) > 60 {
		name = strings.Trim(name[:60], "-")
	}
	return name
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
