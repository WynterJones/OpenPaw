package tmux

import (
	"strings"
	"testing"
)

// The states a Claude Code session passes through before its footer exists.
// Text detection used to see a plain shell in every one of them, so the card
// above the composer stayed hidden until the TUI settled — which is why a
// freshly started session only appeared after leaving the chat and returning.
func TestDetectKind_ClaudeStartupStates(t *testing.T) {
	panes := map[string]string{
		"trust prompt": "" +
			" Quick safety check: Is this a project you created or one you trust?\n" +
			"\n" +
			" Claude Code'll be able to read, edit, and execute files here.\n" +
			"\n" +
			" ❯ 1. Yes, I trust this folder\n" +
			"   2. No, exit\n",
		"welcome splash": " Welcome to Claude Code\n\n /help for help\n",
		"working":        "✻ Sautéed for 2s\n\n  ⏵⏵ bypass permissions on (shift+tab to cycle) · ← 1 agent\n",
		"accept edits":   "  ⏵ accept edits on (shift+tab to cycle)\n",
		"idle footer":    "❯ \n  ? for shortcuts\n",
	}
	for name, pane := range panes {
		t.Run(name, func(t *testing.T) {
			if got := detectKind(pane); got != "claude" {
				t.Errorf("detected as %q, want claude", got)
			}
		})
	}
}

func TestBlockedOn_TrustPrompt(t *testing.T) {
	pane := " Claude Code'll be able to read, edit, and execute files here.\n ❯ 1. Yes, I trust this folder\n"
	got := BlockedOn(pane)
	if got == "" {
		t.Fatal("the folder-trust prompt was not recognised")
	}
	// The point of the message is that the flag does not cover this case.
	if !strings.Contains(got, "trust this folder") || !strings.Contains(got, "skip-permissions") {
		t.Errorf("unhelpful message: %q", got)
	}

	if other := BlockedOn("❯ \n  ? for shortcuts\n"); other != "" {
		t.Errorf("a healthy pane reported as blocked: %q", other)
	}
	// The expiry banner shows on healthy sessions; it must not read as blocked.
	if other := BlockedOn("⚠ Your login expires in 1 day · run /login to renew\n"); other != "" {
		t.Errorf("the login-expiry banner reported as blocked: %q", other)
	}
}

// kindUnder has to find a CLI launched through a wrapper ("bash -lc 'claude …'"),
// not just a direct child of the pane's shell.
func TestKindUnder_WalksDescendants(t *testing.T) {
	table := procTable{
		children: map[int][]int{
			100: {200},
			200: {300},
			300: {400},
		},
		command: map[int]string{
			100: "zsh",
			200: "bash",
			300: "/opt/homebrew/bin/codex",
			400: "rg",
		},
	}
	if got := table.kindUnder(100); got != "codex" {
		t.Errorf("kindUnder = %q, want codex", got)
	}
	if got := table.kindUnder(400); got != "" {
		t.Errorf("kindUnder(leaf) = %q, want empty", got)
	}
}

// The pane's own process is the shell, and a shell that happens to be named
// "claude" (a session opened in ~/claude, say) is not a Claude Code session.
func TestKindUnder_IgnoresThePaneProcessItself(t *testing.T) {
	table := procTable{
		children: map[int][]int{},
		command:  map[int]string{100: "claude"},
	}
	if got := table.kindUnder(100); got != "" {
		t.Errorf("kindUnder = %q, want empty", got)
	}
}

// A cycle in the table must not hang the poll.
func TestKindUnder_SurvivesCycles(t *testing.T) {
	table := procTable{
		children: map[int][]int{100: {200}, 200: {100}},
		command:  map[int]string{100: "zsh", 200: "zsh"},
	}
	if got := table.kindUnder(100); got != "" {
		t.Errorf("kindUnder = %q, want empty", got)
	}
}
