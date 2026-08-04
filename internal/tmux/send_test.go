package tmux

import (
	"strings"
	"testing"
)

// A control key typed as characters is six letters into whatever is on screen,
// which is worse than doing nothing — so the mapping has to be unambiguous.
func TestKeyFor(t *testing.T) {
	cases := []struct{ in, want string }{
		{"enter", "Enter"},
		{"Enter", "Enter"},
		{"  RETURN ", "Enter"},
		{"escape", "Escape"},
		{"esc", "Escape"},
		{"ctrl-c", "C-c"},
		{"ctrl+c", "C-c"},
		{"down", "Down"},
		{"shift-tab", "BTab"},
		// Answers to prompts, not key requests.
		{"y", ""},
		{"2", ""},
		{"yes", ""},
		{"also fix the header", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := KeyFor(c.in); got != c.want {
			t.Errorf("KeyFor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// tmux pads a capture out to the full pane height, so a short run of output
// comes back followed by dozens of blank rows. Returning those as-is filled the
// context window with nothing.
func TestTrimScrollback_DropsPadding(t *testing.T) {
	raw := "\n\n  \nbuilding…\ndone in 4.1s\n\n\n\n   \n"

	got := trimScrollback(raw, 100)

	if got != "building…\ndone in 4.1s" {
		t.Errorf("got %q, want the two real lines with nothing around them", got)
	}
}

// The tail is the part worth keeping: for a finished run it holds the summary
// and the error, while the head is setup noise.
func TestTrimScrollback_KeepsTheEnd(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("line\n")
	}
	b.WriteString("FAILED: 3 tests\n")

	got := trimScrollback(b.String(), 5)

	lines := strings.Split(got, "\n")
	if len(lines) != 5 {
		t.Fatalf("got %d lines, want 5", len(lines))
	}
	if lines[len(lines)-1] != "FAILED: 3 tests" {
		t.Errorf("the last line was dropped: %q", lines[len(lines)-1])
	}
}

// Blank input is a no-op that would otherwise read as a successful answer to a
// prompt that is still sitting there.
func TestTrimScrollback_AllBlankIsEmpty(t *testing.T) {
	if got := trimScrollback("\n\n   \n\t\n", 10); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
