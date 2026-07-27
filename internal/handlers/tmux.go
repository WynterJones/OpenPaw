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
	"github.com/openpaw/openpaw/internal/middleware"
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

// KillTmuxSession ends a session and everything running in it.
//
// Watches are cancelled first, for every thread and not just the caller's: the
// session is about to vanish, and a watcher left running would report it as
// "finished" on its next tick, which reads as a build completing rather than a
// session the user closed.
func (h *ChatHandler) KillTmuxSession(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session")
	if session == "" {
		writeError(w, http.StatusBadRequest, "session is required")
		return
	}

	stopped := h.stopWatchesForSession(session)

	if err := tmux.Kill(r.Context(), session); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.db.LogAudit(middleware.GetUserID(r.Context()), "tmux_session_killed", "chat", "tmux_session", session, "")
	logger.Info("tmux session %s killed (%d watches stopped)", session, stopped)
	writeJSON(w, http.StatusOK, map[string]interface{}{"killed": session, "watches_stopped": stopped})
}

// stopWatchesForSession cancels every watch on a session, whichever thread
// started it. Returns how many were stopped.
func (h *ChatHandler) stopWatchesForSession(session string) int {
	stopped := 0
	tmuxWatches.Range(func(k, v interface{}) bool {
		wch, ok := v.(*tmuxWatch)
		if !ok || wch.Session != session {
			return true
		}
		wch.cancel()
		tmuxWatches.Delete(k)
		stopped++
		return true
	})
	return stopped
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
	if !tmux.Exists(r.Context(), req.Session) {
		writeError(w, http.StatusNotFound, "no tmux session named "+req.Session)
		return
	}

	wch, err := h.StartWatch(threadID, req.Session, req.IntervalS)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, wch)
}

// clampInterval keeps a poll useful: below 10s it hammers tmux for no benefit,
// and beyond 15 minutes it stops being a live check. Zero means "unspecified",
// which takes the default rather than the floor.
func clampInterval(s int) int {
	const (
		min      = 10
		max      = 900
		fallback = 60
	)
	if s <= 0 {
		return fallback
	}
	if s < min {
		return min
	}
	if s > max {
		return max
	}
	return s
}

// StartWatch begins polling a session on behalf of a thread, or returns the
// watch already running for that pair. Exported so the agent-facing tmux_watch
// tool can start one: an agent's turn ends when it replies, so this is the only
// way a promise to "check back later" becomes something that actually happens.
func (h *ChatHandler) StartWatch(threadID, session string, intervalSeconds int) (*tmuxWatch, error) {
	if threadID == "" || session == "" {
		return nil, fmt.Errorf("thread and session are required")
	}
	intervalSeconds = clampInterval(intervalSeconds)

	key := watchKey(threadID, session)
	if existing, loaded := tmuxWatches.Load(key); loaded {
		if wch, ok := existing.(*tmuxWatch); ok {
			return wch, nil
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now().UTC()
	wch := &tmuxWatch{
		ThreadID:  threadID,
		Session:   session,
		IntervalS: intervalSeconds,
		StartedAt: now,
		NextCheck: now.Add(time.Duration(intervalSeconds) * time.Second),
		cancel:    cancel,
	}
	tmuxWatches.Store(key, wch)

	go h.runTmuxWatch(ctx, key, wch)
	return wch, nil
}

// StopWatch cancels watches for a thread; an empty session stops all of them.
// Returns how many were stopped.
func (h *ChatHandler) StopWatch(threadID, session string) int {
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
	return stopped
}

// StopTmuxWatch cancels a running watch.
func (h *ChatHandler) StopTmuxWatch(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")
	session := r.URL.Query().Get("session")

	writeJSON(w, http.StatusOK, map[string]int{"stopped": h.StopWatch(threadID, session)})
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
	// How many consecutive checks with an unchanged pane before reporting in.
	// What that means is deliberately not decided here — see quietReport.
	const quietAfter = 3

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
				"**Check-in — tmux session `%s`**\n\nThe session is gone, so whatever was running in it has ended. "+
					"I checked %d times and have stopped watching.",
				wch.Session, wch.Checks))
			return
		}

		pane, err := tmux.Capture(ctx, wch.Session)
		if err != nil {
			continue
		}

		// A session started by tmux_run outlives its command so the output stays
		// readable, so "finished" can't be inferred from the session vanishing.
		// Without this the run would be reported as *stalled* three checks later
		// — and with no exit status, which is the one thing worth knowing.
		if dead, status, ok := tmux.Finished(ctx, wch.Session); ok && dead {
			outcome := "The command finished successfully (exit status 0)."
			if status != 0 {
				outcome = fmt.Sprintf("The command failed with exit status %d.", status)
			}
			h.reportTmux(wch, fmt.Sprintf(
				"**Check-in — tmux session `%s`**\n\n%s\n\n%s",
				wch.Session, outcome, tmux.Describe(wch.Session, pane)))
			return
		}

		if pane == lastPane {
			unchanged++
		} else {
			unchanged = 0
			lastPane = pane
		}

		if unchanged >= quietAfter {
			h.reportTmux(wch, quietReport(
				wch.Session, pane, time.Duration(quietAfter)*interval))
			return
		}
	}
}

// quietReport is what the watcher says when a session is still running but the
// pane has not changed for a while.
//
// It used to call that "stalled, most likely waiting on input", which is a
// guess dressed as a finding — and often a wrong one. A pane sits still while
// the CLI thinks, while a long build step produces no output, and (for a
// detached TUI that has not been attached yet) before it has drawn anything at
// all. Reporting those as stalled sent people to attach to a session that was
// working fine, and told the agent something untrue about its own build.
//
// So the report states what was observed and leaves the conclusion open —
// except when BlockedOn recognises the actual prompt on screen, which is a fact
// rather than an inference and is worth saying plainly.
func quietReport(session, pane string, quiet time.Duration) string {
	head := fmt.Sprintf("**Check-in — tmux session `%s`**\n\n", session)

	if blocked := tmux.BlockedOn(pane); blocked != "" {
		return head + fmt.Sprintf(
			"It has been sitting on a prompt for %s. %s\n\n%s",
			quiet, blocked, tmux.Describe(session, pane))
	}

	reason := "That is not in itself a problem: a pane sits still while the CLI thinks, " +
		"while a step prints nothing, and while it waits for someone to type. " +
		"I can't tell which of those it is from out here."
	if strings.TrimSpace(pane) == "" {
		reason = "Nothing has been drawn to it at all, which is normal for a TUI that nobody " +
			"has attached to yet — some only paint once a terminal is attached."
	}

	return head + fmt.Sprintf(
		"Still running, with nothing new on screen for %s. %s\n\n"+
			"Attach with `tmux attach -t %s` to see what it is actually doing. I have stopped watching.\n\n%s",
		quiet, reason, session, tmux.Describe(session, pane))
}

// reportTmux posts the watcher's finding into the thread as an assistant message.
//
// Attributed to the gateway, because that is who is speaking: the watch belongs
// to the thread, not to whichever agent happened to start the session. Without
// a slug the message has no identity at all and renders under the stock avatar
// rather than the user's own gateway.
func (h *ChatHandler) reportTmux(wch *tmuxWatch, message string) {
	h.saveAssistantMessage(wch.ThreadID, gatewayRoleSlug, message, 0, 0, 0)
	h.broadcastStatus(wch.ThreadID, "message_saved", "")
	h.broadcastStatus(wch.ThreadID, "done", "")
	logger.Info("tmux watch on %s reported into thread %s", wch.Session, wch.ThreadID)
}
