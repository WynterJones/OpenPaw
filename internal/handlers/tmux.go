package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
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

// SendTmuxInput types into a session from the UI — the same mechanism the
// tmux_send tool uses, so a user watching a session stop on a prompt can answer
// it from the card in chat instead of finding a terminal to attach from.
func (h *ChatHandler) SendTmuxInput(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Session string `json:"session"`
		Text    string `json:"text"`
		Submit  *bool  `json:"submit"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Session == "" || req.Text == "" {
		writeError(w, http.StatusBadRequest, "session and text are required")
		return
	}

	submit := req.Submit == nil || *req.Submit
	if err := tmux.Send(r.Context(), req.Session, req.Text, submit); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// The text itself is not logged: it is whatever the user typed into a
	// running session, which can be a credential answering a prompt.
	h.db.LogAudit(middleware.GetUserID(r.Context()), "tmux_input_sent", "chat", "tmux_session", req.Session, "")
	writeJSON(w, http.StatusOK, map[string]interface{}{"sent": true, "session": req.Session})
}

// GetTmuxLogs returns a session's scrollback.
func (h *ChatHandler) GetTmuxLogs(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session")
	if session == "" {
		writeError(w, http.StatusBadRequest, "session is required")
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))

	logs, err := tmux.Logs(r.Context(), session, lines)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"session": session, "logs": logs})
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

// How many consecutive checks with an unchanged pane before reporting in.
// What that means is deliberately not decided here — see quietReport.
const quietAfter = 3

// maxWatchDuration is how long a watch runs before giving up. Long enough for
// an overnight migration, short enough that a session forgotten in a closed tab
// does not poll tmux for a week.
const maxWatchDuration = 12 * time.Hour

// runTmuxWatch polls until the command exits, the session disappears, the watch
// is cancelled, or it runs out of time — reporting into the thread as it goes.
//
// A quiet pane used to end the watch, and that is why completion notifications
// never seemed to fire. The common case for a dispatched coding agent is
// several minutes of no visible output while it thinks; three of those and the
// watcher filed its "nothing new on screen" note and stopped, so the finish it
// was armed for — the exit status, the closing report, the whole point — was
// never reported by anyone. From out here a quiet pane is an observation, not
// an ending: say it once, keep watching, and let the command's exit be what
// ends the watch.
func (h *ChatHandler) runTmuxWatch(ctx context.Context, key string, wch *tmuxWatch) {
	defer tmuxWatches.Delete(key)

	interval := time.Duration(wch.IntervalS) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	deadline := time.After(maxWatchDuration)

	var lastPane string
	unchanged := 0
	// Each quiet note costs the user a message, so the threshold doubles after
	// every one: a session that is quiet by nature reports at 3 checks, then 6,
	// then 12, instead of filing the same non-event every three minutes.
	quietThreshold := quietAfter

	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			h.reportTmux(wch, fmt.Sprintf(
				"**Check-in — tmux session `%s`**\n\nI have been watching this for %s and it is still running, "+
					"so I have stopped watching to avoid polling it indefinitely. "+
					"It is still going — check it with `tmux_status`, or watch it again with `tmux_watch`.",
				wch.Session, maxWatchDuration))
			return
		case <-ticker.C:
		}

		wch.Checks++
		wch.NextCheck = time.Now().UTC().Add(interval)

		if !tmux.Exists(ctx, wch.Session) {
			h.reportTmux(wch, fmt.Sprintf(
				"**Finished — tmux session `%s`**\n\nThe session is gone, so whatever was running in it has ended. "+
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
			h.reportTmux(wch, finishedReport(ctx, wch.Session, status))
			return
		}

		if pane == lastPane {
			unchanged++
		} else {
			unchanged = 0
			lastPane = pane
		}

		if unchanged >= quietThreshold {
			h.reportTmux(wch, quietReport(
				wch.Session, pane, time.Duration(unchanged)*interval))
			unchanged = 0
			quietThreshold *= 2
		}
	}
}

// finishedReport is what the watcher says when the command has exited.
//
// It carries the tail of the scrollback rather than the visible pane, because
// every dispatched agent is asked to end with what it changed, what it assumed
// and what it left open — a report longer than the ten lines a pane shows, and
// one that has already scrolled past by the time anyone looks. Reconstructing
// that from git log afterwards loses exactly the parts that were not code.
func finishedReport(ctx context.Context, session string, status int) string {
	outcome := "The command finished successfully (exit status 0)."
	if status != 0 {
		outcome = fmt.Sprintf("The command failed with exit status %d.", status)
	}

	report := fmt.Sprintf("**Finished — tmux session `%s`**\n\n%s\n\n", session, outcome)

	// Enough to hold a closing report, little enough not to swamp the thread
	// with a whole build log.
	if final := tmux.FinalOutput(ctx, session, 120); strings.TrimSpace(final) != "" {
		report += "Final output:\n```\n" + final + "\n```\n\n"
	}
	report += fmt.Sprintf(
		"Read further back with `tmux_logs` on `%s` — the scrollback is kept until the session is killed.", session)
	return report
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
			"It has been sitting on a prompt for %s. %s\n\n"+
				"Answer it with `tmux_send` on `%s` rather than killing the session — it keeps everything "+
				"it has worked out so far. I am still watching.\n\n%s",
			quiet, blocked, session, tmux.Describe(session, pane))
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
			"Attach with `tmux attach -t %s` to see what it is actually doing, or send it something with "+
			"`tmux_send`. I am still watching and will report when it finishes.\n\n%s",
		quiet, reason, session, tmux.Describe(session, pane))
}

// reportTmux posts the watcher's finding into the thread as an assistant message.
//
// Attributed to the gateway, because that is who is speaking: the watch belongs
// to the thread, not to whichever agent happened to start the session. Without
// a slug the message has no identity at all and renders under the stock avatar
// rather than the user's own gateway.
//
// A finish also files a notification. The chat message alone only lands if
// somebody is looking at that thread — dispatched work is precisely the kind
// nobody sits and watches, which is why "it's done" kept arriving as an answer
// to "any progress?" rather than as news.
func (h *ChatHandler) reportTmux(wch *tmuxWatch, message string) {
	h.saveAssistantMessage(wch.ThreadID, gatewayRoleSlug, message, 0, 0, 0)
	h.broadcastStatus(wch.ThreadID, "message_saved", "")
	h.broadcastStatus(wch.ThreadID, "done", "")
	logger.Info("tmux watch on %s reported into thread %s", wch.Session, wch.ThreadID)

	if strings.HasPrefix(message, "**Finished") {
		h.notifyTmuxFinished(wch, message)
	}
}

// notifyTmuxFinished files the finish in the Inbox, linked back to the thread
// the work was started from.
func (h *ChatHandler) notifyTmuxFinished(wch *tmuxWatch, message string) {
	if h.db == nil {
		return
	}
	body := "The command has ended after " +
		time.Since(wch.StartedAt).Round(time.Second).String() + "."
	if strings.Contains(message, "failed with exit status") {
		body = "The command failed. " + body
	}

	notif, err := CreateNotification(h.db, models.NotificationInput{
		Title:           "tmux session " + wch.Session + " finished",
		Body:            body,
		Detail:          message,
		WorkspaceID:     h.threadWorkspaceID(wch.ThreadID),
		Priority:        "normal",
		SourceAgentSlug: gatewayRoleSlug,
		SourceType:      "tmux",
		SourceID:        wch.Session,
		Link:            "/chat?thread=" + wch.ThreadID,
	})
	if err != nil {
		logger.Warn("could not file a notification for tmux session %s: %v", wch.Session, err)
		return
	}
	// notification_created is what the bell, the sound and the browser
	// notification all hang off — sending the row itself rather than a "go and
	// refresh" ping is what makes the finish audible.
	if h.agentManager != nil {
		h.agentManager.Broadcast("notification_created", notif)
	}
}

// threadWorkspaceID keeps the notification in the workspace the work was
// started from, so it does not surface in an unrelated one.
func (h *ChatHandler) threadWorkspaceID(threadID string) string {
	var wsID string
	if err := h.db.QueryRow(
		"SELECT COALESCE(workspace_id, '') FROM chat_threads WHERE id = ?", threadID,
	).Scan(&wsID); err != nil {
		return ""
	}
	return wsID
}
