package scheduler

import (
	"fmt"
	"strings"
	"time"
)

// NormalizeCron validates a cron expression and returns it in the 6-field form
// this scheduler actually runs.
//
// The scheduler is built with cron.WithSeconds(), so every expression here needs
// six fields. Practically nobody writes six — crontab, every tutorial, and every
// model trained on them produce the five-field form, and handing "0 9 * * *" to
// a six-field parser fails with "expected exactly 6 fields" rather than doing
// the obvious thing. Five fields are therefore accepted and read as
// minute-first, with a leading "0 " for seconds: "0 9 * * *" means 9am daily,
// which is what whoever typed it meant.
//
// Descriptors (@daily, @hourly, @every 30m) pass through untouched — the parser
// already understands them.
func NormalizeCron(expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", fmt.Errorf("cron expression is empty")
	}

	if strings.HasPrefix(expr, "@") {
		if _, err := cronParser.Parse(expr); err != nil {
			return "", fmt.Errorf("%q is not a valid cron descriptor: %w", expr, err)
		}
		return expr, nil
	}

	fields := strings.Fields(expr)
	switch len(fields) {
	case 5:
		expr = "0 " + strings.Join(fields, " ")
	case 6:
		expr = strings.Join(fields, " ")
	default:
		return "", fmt.Errorf(
			"a cron expression needs 5 fields (minute hour day month weekday) or 6 (with a leading seconds field), got %d in %q",
			len(fields), expr)
	}

	if _, err := cronParser.Parse(expr); err != nil {
		return "", fmt.Errorf("%q is not a valid cron expression: %w", expr, err)
	}
	return expr, nil
}

// NextRun reports when a normalized expression fires next. Used to confirm back
// to the user what they just scheduled — "every 6 hours" is easy to agree to and
// hard to picture, and a wrong expression is otherwise only discovered by the
// run not happening.
func NextRun(expr string) (time.Time, error) {
	schedule, err := cronParser.Parse(expr)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(time.Now()), nil
}
