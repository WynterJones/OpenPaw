package handlers

import "testing"

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
