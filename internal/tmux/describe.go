package tmux

import (
	"fmt"
	"strings"
)

// Describe renders the parsed status, falling back to the raw tail.
func Describe(session, pane string) string {
	var b strings.Builder
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

	b.WriteString("Last output:\n```\n")
	for _, l := range lastLines(pane, 8) {
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
