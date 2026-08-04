package tmux

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sending input into a running session.
//
// Without this a detached session is write-once: an agent could start work and
// read it, but the moment the thing it started asked a question — an
// interactive menu, a confirmation, "which branch?" — the only move left was to
// kill it and dispatch a fresh one, which re-reads the repo and re-derives
// everything it already knew. The same gap made a follow-up instruction
// ("also fix X") cost a full restart.

// namedKeys maps what a caller would naturally write to the key name tmux
// expects. Anything not in here is typed literally, which is why single
// characters are absent: "y" is an answer to a prompt, not a key request.
var namedKeys = map[string]string{
	"enter":     "Enter",
	"return":    "Enter",
	"escape":    "Escape",
	"esc":       "Escape",
	"tab":       "Tab",
	"space":     "Space",
	"backspace": "BSpace",
	"delete":    "DC",
	"up":        "Up",
	"down":      "Down",
	"left":      "Left",
	"right":     "Right",
	"pageup":    "PageUp",
	"pagedown":  "PageDown",
	"home":      "Home",
	"end":       "End",
	"ctrl-c":    "C-c",
	"ctrl+c":    "C-c",
	"ctrl-d":    "C-d",
	"ctrl+d":    "C-d",
	"ctrl-z":    "C-z",
	"ctrl+z":    "C-z",
	"ctrl-r":    "C-r",
	"ctrl+r":    "C-r",
	"shift-tab": "BTab",
	"shift+tab": "BTab",
}

// KeyFor returns the tmux key name for a control key, or "" when the text
// should be typed as literal characters.
func KeyFor(text string) string {
	return namedKeys[strings.ToLower(strings.TrimSpace(text))]
}

// Send types text into a session's pane, optionally submitting it.
//
// Literal text and Enter go in two separate send-keys calls rather than one
// combined one. A TUI reads its input asynchronously, and a submit arriving in
// the same write as the text it submits is regularly processed before the text
// has been placed — the prompt fires on an empty buffer and the typed line is
// left sitting there. The gap between two exec calls plus a short pause is
// enough for the reader to catch up.
//
// A dead pane is reported as such instead of being written to: send-keys to a
// pane whose command has exited succeeds silently, so without this an agent
// answering a prompt in a session that finished ten minutes ago would be told
// it worked.
func Send(ctx context.Context, name, text string, submit bool) error {
	if !Available() {
		return errors.New("tmux is not installed")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("session name is required")
	}
	if text == "" && !submit {
		return errors.New("text is required")
	}
	if !Exists(ctx, name) {
		return fmt.Errorf("no tmux session named %q", name)
	}
	if dead, status, ok := Finished(ctx, name); ok && dead {
		return fmt.Errorf("the command in %q has already exited (status %d), so there is nothing listening for input", name, status)
	}

	// A named control key is sent as a key, never as characters — "escape"
	// typed literally is six letters into whatever is on screen.
	if key := KeyFor(text); key != "" {
		_, err := run(ctx, "send-keys", "-t", name, key)
		return err
	}

	if text != "" {
		if _, err := run(ctx, "send-keys", "-t", name, "-l", text); err != nil {
			return fmt.Errorf("tmux send-keys failed: %w", err)
		}
	}
	if submit {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
		if _, err := run(ctx, "send-keys", "-t", name, "Enter"); err != nil {
			return fmt.Errorf("tmux send-keys Enter failed: %w", err)
		}
	}
	return nil
}

const (
	defaultLogLines = 400
	maxLogLines     = 5000
	// Roughly the point past which scrollback stops being readable context and
	// starts being ballast in the window.
	maxLogChars = 60000
)

// Logs returns a session's scrollback — everything the pane has printed, not
// just what currently fits on screen.
//
// tmux_status shows the visible pane, which is about ten lines, and every
// dispatched agent is asked to end with a report of what changed, what it
// assumed and what is still open. That report is longer than ten lines, so it
// scrolled out of reach the moment it was written and the caller was left
// reconstructing intent from git log.
func Logs(ctx context.Context, name string, lines int) (string, error) {
	if !Available() {
		return "", errors.New("tmux is not installed")
	}
	if strings.TrimSpace(name) == "" {
		return "", errors.New("session name is required")
	}
	if !Exists(ctx, name) {
		return "", fmt.Errorf("no tmux session named %q", name)
	}

	switch {
	case lines <= 0:
		lines = defaultLogLines
	case lines > maxLogLines:
		lines = maxLogLines
	}

	// -J rejoins lines the pane wrapped, so a long report comes back as the
	// sentences it was written as rather than as terminal-width fragments.
	out, err := run(ctx, "capture-pane", "-p", "-J", "-S", fmt.Sprintf("-%d", lines), "-t", name)
	if err != nil {
		// Some tmux builds reject a start line further back than the history
		// they hold rather than clamping to it.
		if out, err = run(ctx, "capture-pane", "-p", "-J", "-S", "-", "-t", name); err != nil {
			return "", fmt.Errorf("tmux capture-pane failed: %w", err)
		}
	}
	return trimScrollback(out, lines), nil
}

// trimScrollback drops the blank padding tmux returns for unused rows and caps
// the result from the end, keeping the most recent output — which for a
// finished run is the summary.
func trimScrollback(out string, lines int) string {
	all := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")

	start, end := 0, len(all)
	for start < end && strings.TrimSpace(all[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(all[end-1]) == "" {
		end--
	}
	kept := all[start:end]
	if len(kept) > lines {
		kept = kept[len(kept)-lines:]
	}

	text := strings.Join(kept, "\n")
	if len(text) > maxLogChars {
		text = "… (earlier output trimmed)\n" + text[len(text)-maxLogChars:]
	}
	return text
}

// FinalOutput returns the tail of a finished session's scrollback: the last
// thing the command said before it exited, which is where a dispatched agent
// puts its report.
func FinalOutput(ctx context.Context, name string, lines int) string {
	out, err := Logs(ctx, name, lines)
	if err != nil {
		return ""
	}
	return out
}
