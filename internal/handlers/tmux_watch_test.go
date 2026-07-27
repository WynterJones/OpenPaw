package handlers

import (
	"strings"
	"testing"
	"time"
)

// A quiet pane is not evidence of a stall — the CLI may be thinking, or a build
// step may simply print nothing. The report must describe, not conclude.
func TestQuietReport_DoesNotClaimAStall(t *testing.T) {
	got := quietReport("build", "✻ Sautéed for 2s\n", 3*time.Minute)

	// "waiting on input" is fine as one listed possibility; asserting it is not.
	for _, banned := range []string{"stalled", "most likely", "looks like it"} {
		if strings.Contains(strings.ToLower(got), banned) {
			t.Errorf("report asserts %q, which it cannot know:\n%s", banned, got)
		}
	}
	if !strings.Contains(got, "I can't tell") {
		t.Errorf("report does not admit the uncertainty:\n%s", got)
	}
	if !strings.Contains(got, "Check-in") {
		t.Errorf("report is not framed as a check-in:\n%s", got)
	}
	if !strings.Contains(got, "tmux attach -t build") {
		t.Errorf("report does not say how to look for yourself:\n%s", got)
	}
}

// An empty pane is the normal state of a detached TUI nobody has attached to,
// and reading it as trouble is what made a healthy session look broken.
func TestQuietReport_EmptyPaneIsNotTreatedAsTrouble(t *testing.T) {
	got := quietReport("rb-ads-fix", "   \n\n", 3*time.Minute)

	if !strings.Contains(got, "normal") {
		t.Errorf("an empty pane is not explained as normal:\n%s", got)
	}
	if strings.Contains(got, "```") {
		t.Errorf("an empty pane rendered as an empty code block:\n%s", got)
	}
}

// When the pane names the prompt it is sitting on, that is a fact rather than a
// guess, and hedging it would bury the one actionable thing on screen.
func TestQuietReport_StatesARecognisedPromptPlainly(t *testing.T) {
	pane := " Claude Code'll be able to read, edit, and execute files here.\n ❯ 1. Yes, I trust this folder\n"
	got := quietReport("build", pane, 3*time.Minute)

	if !strings.Contains(got, "sitting on a prompt") {
		t.Errorf("a recognised prompt was not stated plainly:\n%s", got)
	}
	if strings.Contains(got, "I can't tell which") {
		t.Errorf("hedged about a prompt it had already identified:\n%s", got)
	}
}

// The old ceiling was 600s, so an agent asked to "check back in 15 minutes"
// silently got 10. Zero means unspecified and takes the default, not the floor.
func TestClampInterval(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 60},
		{-5, 60},
		{1, 10},
		{9, 10},
		{10, 10},
		{60, 60},
		{900, 900},
		{901, 900},
		{99999, 900},
	}
	for _, c := range cases {
		if got := clampInterval(c.in); got != c.want {
			t.Errorf("clampInterval(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestStartWatch_RequiresThreadAndSession(t *testing.T) {
	h := &ChatHandler{}
	if _, err := h.StartWatch("", "build", 60); err == nil {
		t.Error("expected an error with no thread")
	}
	if _, err := h.StartWatch("thread-1", "", 60); err == nil {
		t.Error("expected an error with no session")
	}
}

// Starting twice for the same pair must reuse the running watch rather than
// spawn a second goroutine polling the same session.
func TestStartWatch_IsIdempotentPerThreadSession(t *testing.T) {
	h := &ChatHandler{}
	t.Cleanup(func() { h.StopWatch("thread-x", "") })

	first, err := h.StartWatch("thread-x", "sess", 60)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	second, err := h.StartWatch("thread-x", "sess", 60)
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if first != second {
		t.Error("a second watch was created for the same thread/session")
	}
	if n := h.StopWatch("thread-x", ""); n != 1 {
		t.Errorf("stopped %d watches, want 1", n)
	}
}

func TestStopWatch_ScopedToThread(t *testing.T) {
	h := &ChatHandler{}
	t.Cleanup(func() { h.StopWatch("t1", ""); h.StopWatch("t2", "") })

	h.StartWatch("t1", "a", 60)
	h.StartWatch("t1", "b", 60)
	h.StartWatch("t2", "c", 60)

	if n := h.StopWatch("t1", ""); n != 2 {
		t.Errorf("stopped %d for t1, want 2", n)
	}
	if n := h.StopWatch("t2", ""); n != 1 {
		t.Errorf("stopped %d for t2, want 1 — t2's watch should be untouched by t1", n)
	}
}
