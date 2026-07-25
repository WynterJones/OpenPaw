package agents

import (
	"strings"
	"testing"
)

// The directive exists to stop scheduled reports ending in questions nobody can
// answer, so the threadless variant must be explicit that no one will reply.
func TestScheduledReportDirective(t *testing.T) {
	threadless := scheduledReportDirective(true)
	threaded := scheduledReportDirective(false)

	for _, want := range []string{"SCHEDULED RUN", "Do not end with a question", "recurring check"} {
		if !strings.Contains(threadless, want) {
			t.Errorf("threadless directive missing %q", want)
		}
		if !strings.Contains(threaded, want) {
			t.Errorf("threaded directive missing %q", want)
		}
	}

	if !strings.Contains(threadless, "Inbox") || !strings.Contains(threadless, "go unanswered") {
		t.Error("threadless directive should say the report goes to the Inbox and questions go unanswered")
	}
	if strings.Contains(threaded, "Inbox") {
		t.Error("threaded directive should not claim the report goes to the Inbox")
	}
	if !strings.Contains(threaded, "existing chat thread") {
		t.Error("threaded directive should say it posts into the existing thread")
	}
}
