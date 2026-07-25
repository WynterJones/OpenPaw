package scheduler

import "testing"

// The inbox row and the OS push notification both show `preview`. Reports
// almost always open with a markdown heading, so taking the first line verbatim
// would make every row read "## Summary".
func TestPreview_SkipsHeadingsAndFences(t *testing.T) {
	cases := []struct {
		name   string
		report string
		want   string
	}{
		{
			name:   "skips a heading",
			report: "## Summary\n\nThe deploy finished cleanly.",
			want:   "The deploy finished cleanly.",
		},
		{
			name:   "skips a code fence",
			report: "```\nlog output\n```\nNothing needed your attention.",
			want:   "log output",
		},
		{
			name:   "strips list markers",
			report: "# Report\n\n- Checked 4 feeds\n- All healthy",
			want:   "Checked 4 feeds",
		},
		{
			name:   "plain prose passes through",
			report: "All three services responded.",
			want:   "All three services responded.",
		},
		{
			name:   "heading-only report falls back to the heading",
			report: "## Nothing to report",
			want:   "## Nothing to report",
		},
		{
			name:   "empty report yields empty preview",
			report: "",
			want:   "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := preview(c.report); got != c.want {
				t.Errorf("preview(%q) = %q, want %q", c.report, got, c.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 20); got != "short" {
		t.Errorf("truncate under limit = %q, want unchanged", got)
	}
	if got := truncate("abcdefghij", 5); got != "abcde…" {
		t.Errorf("truncate over limit = %q, want %q", got, "abcde…")
	}
	if got := truncate("   padded   ", 20); got != "padded" {
		t.Errorf("truncate should trim, got %q", got)
	}
}
