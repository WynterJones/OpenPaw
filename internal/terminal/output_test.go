package terminal

import (
	"bytes"
	"testing"
	"time"
)

func TestSubscribeOutputReplaysHistoryAndStreamsNewOutput(t *testing.T) {
	db := newWorkbenchTestDB(t)
	m := NewManager(db, t.TempDir())
	t.Cleanup(m.Shutdown)

	session, err := m.CreateSession("Replay", 80, 24, "", "", "", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	marker := []byte("openpaw-history-marker")
	if _, err := session.Ptmx.Write(append([]byte("printf "), append(marker, '\n')...)); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		session.outputMu.Lock()
		hasMarker := bytes.Contains(session.output, marker)
		session.outputMu.Unlock()
		if hasMarker {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("terminal output was not captured")
		}
		time.Sleep(10 * time.Millisecond)
	}

	history, stream, _, unsubscribe, err := m.SubscribeOutput(session.ID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsubscribe()
	if !bytes.Contains(history, marker) {
		t.Fatalf("history %q does not contain %q", history, marker)
	}

	liveMarker := []byte("openpaw-live-marker")
	if _, err := session.Ptmx.Write(append([]byte("printf "), append(liveMarker, '\n')...)); err != nil {
		t.Fatalf("write live marker: %v", err)
	}

	select {
	case output := <-stream:
		if !bytes.Contains(output, liveMarker) {
			t.Fatalf("live output %q does not contain %q", output, liveMarker)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for live terminal output")
	}
}
