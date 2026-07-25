package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/tmux"
)

// tmuxWatch is an active poll of one tmux session on behalf of one chat thread.
type tmuxWatch struct {
	ThreadID  string    `json:"thread_id"`
	Session   string    `json:"session"`
	IntervalS int       `json:"interval_seconds"`
	NextCheck time.Time `json:"next_check"`
	StartedAt time.Time `json:"started_at"`
	Checks    int       `json:"checks"`

	cancel context.CancelFunc
}

// tmuxWatches holds the running watchers, keyed threadID+"\x00"+session.
var tmuxWatches sync.Map

func watchKey(threadID, session string) string { return threadID + "\x00" + session }

// ListTmuxSessions returns every running tmux session with its parsed state.
func (h *ChatHandler) ListTmuxSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := tmux.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tmux sessions")
		return
	}
	if sessions == nil {
		sessions = []tmux.Session{}
	}

	// Attach any watch running for this thread so the UI can show its countdown.
	threadID := r.URL.Query().Get("thread_id")
	watches := map[string]*tmuxWatch{}
	if threadID != "" {
		tmuxWatches.Range(func(_, v interface{}) bool {
			if wch, ok := v.(*tmuxWatch); ok && wch.ThreadID == threadID {
				watches[wch.Session] = wch
			}
			return true
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"available": tmux.Available(),
		"sessions":  sessions,
		"watches":   watches,
	})
}

// StartTmuxWatch polls a tmux session on a fixed interval and reports back into
// the thread when it finishes or stalls.
//
// This exists because an agent turn is a single request/response: an agent that
// says "I'll keep checking" cannot actually do so once its turn ends. The poll
// runs server-side and stops on its own when the session exits, so nothing is
// left running after the thing it was watching is gone.
func (h *ChatHandler) StartTmuxWatch(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")

	var req struct {
		Session   string `json:"session"`
		IntervalS int    `json:"interval_seconds"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Session == "" {
		writeError(w, http.StatusBadRequest, "session is required")
		return
	}
	// Clamped: below 10s this hammers tmux for no benefit, above 10 minutes it
	// stops being a live check.
	if req.IntervalS < 10 {
		req.IntervalS = 30
	}
	if req.IntervalS > 600 {
		req.IntervalS = 600
	}
	if !tmux.Exists(r.Context(), req.Session) {
		writeError(w, http.StatusNotFound, "no tmux session named "+req.Session)
		return
	}

	key := watchKey(threadID, req.Session)
	if existing, loaded := tmuxWatches.Load(key); loaded {
		writeJSON(w, http.StatusOK, existing)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	wch := &tmuxWatch{
		ThreadID:  threadID,
		Session:   req.Session,
		IntervalS: req.IntervalS,
		StartedAt: time.Now().UTC(),
		NextCheck: time.Now().UTC().Add(time.Duration(req.IntervalS) * time.Second),
		cancel:    cancel,
	}
	tmuxWatches.Store(key, wch)

	go h.runTmuxWatch(ctx, key, wch)

	writeJSON(w, http.StatusOK, wch)
}

// StopTmuxWatch cancels a running watch.
func (h *ChatHandler) StopTmuxWatch(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")
	session := r.URL.Query().Get("session")

	stopped := 0
	tmuxWatches.Range(func(k, v interface{}) bool {
		wch, ok := v.(*tmuxWatch)
		if !ok || wch.ThreadID != threadID {
			return true
		}
		if session != "" && wch.Session != session {
			return true
		}
		wch.cancel()
		tmuxWatches.Delete(k)
		stopped++
		return true
	})

	writeJSON(w, http.StatusOK, map[string]int{"stopped": stopped})
}

// runTmuxWatch polls until the session ends, the pane goes quiet, or it is
// cancelled — then posts one message into the thread and removes itself.
func (h *ChatHandler) runTmuxWatch(ctx context.Context, key string, wch *tmuxWatch) {
	defer tmuxWatches.Delete(key)

	interval := time.Duration(wch.IntervalS) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastPane string
	unchanged := 0
	// A session that hasn't moved for this many consecutive checks is treated as
	// stalled — usually a prompt waiting on a human, which is exactly the case
	// worth surfacing.
	const stalledAfter = 3

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		wch.Checks++
		wch.NextCheck = time.Now().UTC().Add(interval)

		if !tmux.Exists(ctx, wch.Session) {
			h.reportTmux(wch, fmt.Sprintf(
				"The tmux session `%s` has finished — it is no longer running. I stopped watching it after %d checks.",
				wch.Session, wch.Checks))
			return
		}

		pane, err := tmux.Capture(ctx, wch.Session)
		if err != nil {
			continue
		}

		if pane == lastPane {
			unchanged++
		} else {
			unchanged = 0
			lastPane = pane
		}

		if unchanged >= stalledAfter {
			summary := describeTmux(wch.Session, pane)
			h.reportTmux(wch, fmt.Sprintf(
				"The tmux session `%s` hasn't changed in %s — it looks stalled, most likely waiting on input.\n\n%s",
				wch.Session, (time.Duration(stalledAfter)*interval).String(), summary))
			return
		}
	}
}

// reportTmux posts the watcher's finding into the thread as an assistant message.
func (h *ChatHandler) reportTmux(wch *tmuxWatch, message string) {
	h.saveAssistantMessage(wch.ThreadID, "", message, 0, 0, 0)
	h.broadcastStatus(wch.ThreadID, "message_saved", "")
	h.broadcastStatus(wch.ThreadID, "done", "")
	logger.Info("tmux watch on %s reported into thread %s", wch.Session, wch.ThreadID)
}

// describeTmux renders the parsed status, falling back to the raw tail.
func describeTmux(session, pane string) string {
	var b strings.Builder
	if st := tmux.ParseStatus(pane); st != nil {
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
