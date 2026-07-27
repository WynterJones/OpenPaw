package tmux

import (
	"fmt"
	"strings"
)

// BlockedOn names the prompt a session is sitting on, or "" if it is not on one
// we recognise.
//
// Worth singling out because the watcher otherwise reports a quiet pane
// without claiming to know why — correct, but unhelpful when the pane says
// exactly why right there on screen. The folder-trust dialog in particular is
// not covered by
// --dangerously-skip-permissions — an interactive Claude Code in a directory it
// has not seen before stops here no matter what, so a detached session waits
// forever with no indication of why.
func BlockedOn(pane string) string {
	switch {
	case strings.Contains(pane, "Yes, I trust this folder"),
		strings.Contains(pane, "Claude Code'll be able to"):
		return "It is waiting on Claude Code's \"do you trust this folder?\" prompt, " +
			"which is asked once per directory and is not covered by the skip-permissions flag. " +
			"Attach and answer it, or run Claude Code in that directory once by hand."
	}
	return ""
}

// Describe renders the parsed status, falling back to the raw tail.
func Describe(session, pane string) string {
	var b strings.Builder
	if blocked := BlockedOn(pane); blocked != "" {
		b.WriteString(blocked + "\n\n")
	}
	if st := ParseStatus(pane); st != nil {
		b.WriteString("Current state:\n")
		if st.Project != "" {
			b.WriteString(fmt.Sprintf("- Project: %s", st.Project))
			if st.Branch != "" {
				b.WriteString(fmt.Sprintf(" (%s)", st.Branch))
			}
			b.WriteString("\n")
		}
		if st.Model != "" {
			b.WriteString(fmt.Sprintf("- Model: %s\n", st.Model))
		}
		if st.ContextPct > 0 {
			b.WriteString(fmt.Sprintf("- Context: %d%%\n", st.ContextPct))
		}
		if st.Elapsed != "" {
			b.WriteString(fmt.Sprintf("- Running for: %s\n", st.Elapsed))
		}
		if st.LinesAdded > 0 || st.LinesRemoved > 0 {
			b.WriteString(fmt.Sprintf("- Changes: +%d/-%d\n", st.LinesAdded, st.LinesRemoved))
		}
		b.WriteString("\n")
	}

	// An empty pane is a real state, not a missing one — a detached TUI often
	// draws nothing until a terminal attaches. Rendering it as an empty code
	// block read as "the output is broken"; say what it means instead.
	lines := lastLines(pane, 8)
	if len(lines) == 0 {
		b.WriteString("Last output: nothing has been drawn to the pane yet.")
		return b.String()
	}

	b.WriteString("Last output:\n```\n")
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	b.WriteString("```")
	return b.String()
}

func lastLines(s string, n int) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, strings.TrimRight(l, " \t"))
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}
