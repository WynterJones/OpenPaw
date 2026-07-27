package tmux

import (
	"context"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

// Working out what a session is actually running.
//
// This used to be pure text-sniffing of the pane, which meant a session only
// counted as Claude Code once the CLI had drawn its footer — the one line
// carrying "(shift+tab to cycle)". Everything before that reads as a plain
// shell: the startup splash, the "do you trust this folder?" prompt, a login
// screen. The card above the composer hides plain shells, so a session the user
// had just started was invisible for as long as the TUI took to settle, and
// indefinitely if it settled on a prompt. Leaving the chat and coming back
// "fixed" it only because time had passed.
//
// So ask the operating system instead. The process is there from the moment it
// launches, whatever is on screen, and it cannot be spoofed by a pane that
// merely mentions the word "codex". Pane text stays as the fallback, for the
// case the process tree cannot answer: a CLI running on the far side of an ssh
// session is somebody else's process, but it still paints its footer here.

// kindForPanes returns session name -> "claude" / "codex" for every session
// running one, found by walking each pane's process descendants.
//
// Sessions with nothing recognisable are absent from the map rather than
// present as "shell", so callers can tell "definitely not" from "did not ask".
func kindForPanes(ctx context.Context) map[string]string {
	// Name last: tmux's format expansion rewrites non-printable bytes, so the
	// fields are space-separated and the one that may contain spaces goes at
	// the end. Same reasoning as List.
	out, err := run(ctx, "list-panes", "-a", "-F", "#{pane_pid} #{session_name}")
	if err != nil {
		return nil
	}

	procs := processTable(ctx)
	if len(procs.command) == 0 {
		return nil
	}

	kinds := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		pidStr, session, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || session == "" {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		// A session with several panes is whatever the first recognisable one
		// is running; the card shows one row per session either way.
		if _, seen := kinds[session]; seen {
			continue
		}
		if kind := procs.kindUnder(pid); kind != "" {
			kinds[session] = kind
		}
	}
	return kinds
}

// procTable is the running processes, as parent/child links plus command names.
type procTable struct {
	children map[int][]int
	command  map[int]string
}

// kindUnder walks pid's descendants for a CLI we recognise. The pane's own
// process is the shell; what we are after is what the shell launched, which may
// be a grandchild ("bash -lc 'claude …'").
func (t procTable) kindUnder(pid int) string {
	// Depth is bounded by the walk itself, but a corrupt table could in
	// principle cycle; the visited set makes that impossible.
	visited := map[int]bool{}
	stack := []int{pid}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[cur] {
			continue
		}
		visited[cur] = true

		if cur != pid {
			switch path.Base(t.command[cur]) {
			case "claude":
				return "claude"
			case "codex":
				return "codex"
			}
		}
		stack = append(stack, t.children[cur]...)
	}
	return ""
}

// processTable snapshots every process once, rather than shelling out per pane.
func processTable(ctx context.Context) procTable {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,comm=").Output()
	if err != nil {
		return procTable{}
	}

	t := procTable{children: map[int][]int{}, command: map[int]string{}}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		// comm may contain spaces on macOS (an .app bundle path), so it is
		// everything left over rather than one field.
		t.command[pid] = strings.Join(fields[2:], " ")
		t.children[ppid] = append(t.children[ppid], pid)
	}
	return t
}

// Pane text that only Claude Code puts on screen. The footer markers cover a
// settled session; the rest cover the startup states it passes through first,
// which is exactly when the user is looking for the card.
var claudePaneMarkers = []string{
	"auto mode on",
	"(shift+tab to cycle)",
	"bypass permissions on",
	"accept edits on",
	"plan mode on",
	"esc to interrupt",
	"? for shortcuts",
	"Welcome to Claude Code",
	"Claude Code'll be able to",
	"claude --resume",
	"/release-notes",
}

var codexPaneMarkers = []string{
	"OpenAI Codex",
	"codex --resume",
	"Ctrl+J newline",
	"/status for session info",
}

// detectKind is the fallback when the process tree came up empty: read the
// pane. Markers are matched against the whole pane because the status line's
// position moves with the terminal height.
func detectKind(pane string) string {
	for _, m := range claudePaneMarkers {
		if strings.Contains(pane, m) {
			return "claude"
		}
	}
	for _, m := range codexPaneMarkers {
		if strings.Contains(pane, m) {
			return "codex"
		}
	}
	// Loose last resort. A pane that merely mentions the word is a weak signal
	// — a shell that ran `cd ~/codex` matches — but a missed session is worse
	// than an extra card the user can dismiss.
	if strings.Contains(pane, "codex") || strings.Contains(pane, "Codex") {
		return "codex"
	}
	return "shell"
}
