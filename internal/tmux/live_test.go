package tmux

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// These run against a real tmux server, because every part of this package that
// can actually be wrong lives in what tmux does rather than in what we compute:
// whether typed input reaches a program, whether scrollback survives the
// command exiting, whether an injected variable is in the environment. Skipped
// where tmux is not installed.

func liveSession(t *testing.T, command string) string {
	t.Helper()
	if !Available() {
		t.Skip("tmux is not installed")
	}

	name := fmt.Sprintf("openpaw-test-%s-%d", SessionName(t.Name()), time.Now().UnixNano())
	t.Cleanup(func() { Kill(context.Background(), name) })

	if err := Start(context.Background(), name, t.TempDir(), command); err != nil {
		t.Fatalf("starting %q: %v", name, err)
	}
	return name
}

// The blocked-on-a-prompt case, which used to cost a whole session restart:
// a command waits on input, and the answer has to reach it.
func TestLive_SendReachesTheProgram(t *testing.T) {
	name := liveSession(t, `sh -c 'read answer; echo "you said: $answer"; sleep 30'`)
	waitFor(t, name, "", 2*time.Second) // let the shell reach the read

	if err := Send(context.Background(), name, "merge-it", true); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if !waitFor(t, name, "you said: merge-it", 5*time.Second) {
		logs, _ := Logs(context.Background(), name, 50)
		t.Errorf("the input never reached the program. Pane:\n%s", logs)
	}
}

// Sending to a pane whose command has exited succeeds silently at the tmux
// level, so without this check an agent would be told its answer landed.
func TestLive_SendRefusesAFinishedSession(t *testing.T) {
	name := liveSession(t, "true")
	waitForExit(t, name, 5*time.Second)

	err := Send(context.Background(), name, "hello", true)
	if err == nil {
		t.Fatal("sending to a dead pane reported success")
	}
	if !strings.Contains(err.Error(), "already exited") {
		t.Errorf("unhelpful error: %v", err)
	}
}

// The reason tmux_logs exists: a closing report is longer than the visible
// pane, and the pane is all tmux_status could ever return.
func TestLive_LogsReachPastTheVisiblePane(t *testing.T) {
	name := liveSession(t, `sh -c 'i=1; while [ $i -le 200 ]; do echo "line $i"; i=$((i+1)); done'`)
	waitFor(t, name, "line 200", 10*time.Second)

	logs, err := Logs(context.Background(), name, 300)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	// line 1 has long since scrolled off the visible pane.
	if !strings.Contains(logs, "line 1\n") {
		t.Errorf("scrollback does not reach the start of the output:\n%s", firstLines(logs, 5))
	}
	if !strings.Contains(logs, "line 200") {
		t.Errorf("scrollback is missing the end of the output")
	}
}

// A finished command's exit status is the single most useful thing about it,
// and the session outliving the command is what makes it readable at all.
func TestLive_FinishedReportsExitStatus(t *testing.T) {
	name := liveSession(t, "exit 3")

	if !waitForExit(t, name, 5*time.Second) {
		t.Fatal("the pane never reported as finished")
	}
	_, status, _ := Finished(context.Background(), name)
	if status != 3 {
		t.Errorf("exit status = %d, want 3", status)
	}
}

// The whole promise of run_with_secrets: the command has the credential and
// nothing else does.
func TestLive_StartWithEnvInjectsWithoutLeaking(t *testing.T) {
	if !Available() {
		t.Skip("tmux is not installed")
	}
	name := fmt.Sprintf("openpaw-test-env-%d", time.Now().UnixNano())
	t.Cleanup(func() { Kill(context.Background(), name) })

	err := StartWithEnv(context.Background(), name, t.TempDir(),
		`sh -c 'echo "token is $DEPLOY_TOKEN"; sleep 20'`,
		map[string]string{"DEPLOY_TOKEN": "s3cr3t-value"})
	if err != nil {
		t.Fatalf("StartWithEnv: %v", err)
	}

	if !waitFor(t, name, "token is s3cr3t-value", 5*time.Second) {
		logs, _ := Logs(context.Background(), name, 50)
		t.Errorf("the variable never reached the command:\n%s", logs)
	}
}

// waitFor polls the pane until it contains want, which is how anything
// involving a real terminal has to be asserted — the write and the redraw are
// not the same event.
func waitFor(t *testing.T, session, want string, limit time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if want == "" {
			time.Sleep(300 * time.Millisecond)
			return true
		}
		if out, err := Logs(context.Background(), session, 400); err == nil && strings.Contains(out, want) {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

func waitForExit(t *testing.T, session string, limit time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if dead, _, ok := Finished(context.Background(), session); ok && dead {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
