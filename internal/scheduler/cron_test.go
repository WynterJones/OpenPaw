package scheduler

import "testing"

// The scheduler runs 6-field expressions, but crontab, every tutorial and every
// model writes 5. Rejecting those would make "run this at 9am" fail for the most
// natural way to say it.
func TestNormalizeCronAcceptsFiveFields(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0 9 * * *", "0 0 9 * * *"},         // 9am daily
		{"30 8 * * 1-5", "0 30 8 * * 1-5"},   // 8:30am weekdays
		{"  0   9  *  *  * ", "0 0 9 * * *"}, // ragged whitespace
		{"0 0 9 * * *", "0 0 9 * * *"},       // already 6 fields
		{"*/15 * * * *", "0 */15 * * * *"},   // step values
	}
	for _, tc := range cases {
		got, err := NormalizeCron(tc.in)
		if err != nil {
			t.Errorf("NormalizeCron(%q) errored: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("NormalizeCron(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeCronAcceptsDescriptors(t *testing.T) {
	for _, expr := range []string{"@daily", "@hourly", "@weekly", "@every 30m"} {
		got, err := NormalizeCron(expr)
		if err != nil {
			t.Errorf("NormalizeCron(%q) errored: %v", expr, err)
		}
		if got != expr {
			t.Errorf("NormalizeCron(%q) = %q, want it passed through unchanged", expr, got)
		}
	}
}

// A schedule saved with a broken expression never fires and says nothing about
// why, which reads as the feature being broken rather than the input.
func TestNormalizeCronRejectsGarbage(t *testing.T) {
	for _, expr := range []string{
		"",
		"   ",
		"every morning",
		"0 9 * *",       // 4 fields
		"0 0 9 * * * *", // 7 fields
		"99 * * * *",    // minute out of range
		"@nonsense",
	} {
		if got, err := NormalizeCron(expr); err == nil {
			t.Errorf("NormalizeCron(%q) = %q, expected an error", expr, got)
		}
	}
}

// The 5-field form has to keep meaning what a crontab would mean; getting the
// shift wrong would silently move every schedule by an hour or run it 60x too
// often, neither of which looks like a bug at the call site.
func TestNormalizeCronPreservesMeaning(t *testing.T) {
	fiveField, err := NormalizeCron("30 8 * * 1-5")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	next, err := NextRun(fiveField)
	if err != nil {
		t.Fatalf("NextRun: %v", err)
	}
	if next.Hour() != 8 || next.Minute() != 30 || next.Second() != 0 {
		t.Errorf("next run = %s, want the next weekday at 08:30:00", next.Format("Mon 15:04:05"))
	}
	if wd := next.Weekday(); wd == 0 || wd == 6 {
		t.Errorf("next run landed on %s, but the expression says weekdays only", wd)
	}
}
