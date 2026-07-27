package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/models"
	"github.com/robfig/cron/v3"
)

// PromptSender sends a scheduled prompt to an AI agent.
type PromptSender interface {
	SendScheduledPrompt(ctx context.Context, slug, prompt, threadID, workspaceID, provider string) (response, usedThreadID string, err error)
	// AgentTimeout is how long a single agent run may take. A scheduled run is
	// the same work as a chat turn and gets the same budget.
	AgentTimeout() time.Duration
}

// NotifyFunc creates a notification and broadcasts it.
type NotifyFunc func(models.NotificationInput)

type ScheduleConfig struct {
	ID            string
	CronExpr      string
	AgentRoleSlug string
	PromptContent string
	ThreadID      string
	// WorkspaceID is the schedule's target workspace ("" = global; the run path
	// falls back to the active workspace when creating a new thread).
	WorkspaceID string
	// Provider pins the engine this routine runs on ("" = whatever is active).
	Provider string
}

type Scheduler struct {
	cron          *cron.Cron
	entries       map[string]cron.EntryID
	mu            sync.Mutex
	db            *database.DB
	promptSender  PromptSender
	notifyFn      NotifyFunc
	retentionStop chan struct{}
}

func New(db *database.DB) *Scheduler {
	return &Scheduler{
		cron:    cron.New(cron.WithSeconds()),
		entries: make(map[string]cron.EntryID),
		db:      db,
	}
}

func (s *Scheduler) SetPromptSender(ps PromptSender) {
	s.promptSender = ps
}

func (s *Scheduler) SetNotifyFunc(fn NotifyFunc) {
	s.notifyFn = fn
}

func (s *Scheduler) Start() {
	s.reapOrphanedExecutions()
	s.cron.Start()
	logger.Success("Scheduler started")
}

// reapOrphanedExecutions closes out runs left mid-flight by a crash or restart.
//
// Nothing else ever finishes those rows, so they stay 'running' forever — the
// Executions tab shows a run that will never end, and the live-activity
// indicator reports work that isn't happening. Anything still marked running at
// boot cannot be: the process that owned it is gone.
func (s *Scheduler) reapOrphanedExecutions() {
	result, err := s.db.Exec(
		`UPDATE schedule_executions SET status = 'error', error = ?, finished_at = ?
		 WHERE status = 'running'`,
		"Interrupted — OpenPaw restarted while this run was in progress.", time.Now().UTC(),
	)
	if err != nil {
		logger.Error("Failed to reap orphaned schedule executions: %v", err)
		return
	}
	if n, _ := result.RowsAffected(); n > 0 {
		logger.Info("Reaped %d interrupted schedule execution(s)", n)
	}
}

// cronParser mirrors the parser cron.WithSeconds() installs on the scheduler, so
// the next-run times recorded here agree with when cron will actually fire.
var cronParser = cron.NewParser(
	cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// maxMissedRunScan bounds the walk over fire times that were missed. A per-second
// schedule that was down for a week would otherwise count to half a million; the
// exact number stops being interesting long before that.
const maxMissedRunScan = 10000

// setNextRun records when this schedule fires next.
//
// The column existed from the beginning but nothing ever wrote it, so the
// Scheduler page's "Next run" read a value that was always NULL and every
// schedule, however active, reported "Not scheduled".
func (s *Scheduler) setNextRun(id, expr string) {
	schedule, err := cronParser.Parse(expr)
	if err != nil {
		logger.Error("Schedule %s has an unparseable cron expression %q: %v", id, expr, err)
		s.db.Exec("UPDATE schedules SET next_run_at = NULL WHERE id = ?", id)
		return
	}
	s.db.Exec("UPDATE schedules SET next_run_at = ? WHERE id = ?", schedule.Next(time.Now().UTC()), id)
}

// clearNextRun drops the next-run time for a schedule that is no longer
// registered, so a paused schedule doesn't keep advertising a run that won't
// happen — and so re-enabling it later isn't read as a missed run.
func (s *Scheduler) clearNextRun(id string) {
	s.db.Exec("UPDATE schedules SET next_run_at = NULL WHERE id = ?", id)
}

// syncNextRun registers a schedule's next fire time and, when the time already
// recorded has passed, files the runs that were missed while it was unregistered.
//
// cron has no catch-up: a 9am daily prompt on a laptop that was asleep simply
// never happened, and nothing anywhere said so — no execution row, no error, a
// silent gap in the history. Recording it as a missed run makes the gap visible
// where the user already looks for runs.
func (s *Scheduler) syncNextRun(id, expr string) {
	schedule, err := cronParser.Parse(expr)
	if err != nil {
		s.setNextRun(id, expr) // logs and clears
		return
	}

	var due sql.NullTime
	s.db.QueryRow("SELECT next_run_at FROM schedules WHERE id = ?", id).Scan(&due)
	now := time.Now().UTC()
	if due.Valid && due.Time.Before(now) {
		s.recordMissedRuns(id, schedule, due.Time.UTC(), now)
	}

	s.db.Exec("UPDATE schedules SET next_run_at = ? WHERE id = ?", schedule.Next(now), id)
}

// recordMissedRuns files one execution row covering every fire between the time
// this schedule was due and now. One row rather than one per fire: a minutely
// schedule offline overnight would otherwise bury the history under 500 entries
// that all say the same thing.
func (s *Scheduler) recordMissedRuns(id string, schedule cron.Schedule, due, now time.Time) {
	missed := 0
	last := due
	for t := due; t.Before(now) && missed < maxMissedRunScan; t = schedule.Next(t) {
		missed++
		last = t
	}
	if missed == 0 {
		return
	}

	msg := "OpenPaw wasn't running when this was due, so the run was skipped."
	if missed > 1 {
		msg = fmt.Sprintf(
			"OpenPaw wasn't running, so %d runs were skipped (from %s to %s).",
			missed, due.Format("Jan 2 3:04 PM"), last.Format("Jan 2 3:04 PM"),
		)
	}

	if _, err := s.db.Exec(
		`INSERT INTO schedule_executions (id, schedule_id, status, error, started_at, finished_at)
		 VALUES (?, ?, 'missed', ?, ?, ?)`,
		uuid.New().String(), id, msg, due, now,
	); err != nil {
		logger.Error("Failed to record missed runs for schedule %s: %v", id, err)
		return
	}
	logger.Warn("Schedule %s missed %d run(s) while OpenPaw was not running", id, missed)
}

func (s *Scheduler) Stop() {
	if s.retentionStop != nil {
		close(s.retentionStop)
	}
	s.cron.Stop()
	logger.Success("Scheduler stopped")
}

func (s *Scheduler) AddSchedule(cfg ScheduleConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, exists := s.entries[cfg.ID]; exists {
		s.cron.Remove(entryID)
	}

	schedCfg := cfg // capture for closure
	entryID, err := s.cron.AddFunc(cfg.CronExpr, func() {
		s.executeSchedule(schedCfg)
	})
	if err != nil {
		logger.Error("Failed to add schedule %s: %v", cfg.ID, err)
		return
	}

	s.entries[cfg.ID] = entryID
	s.syncNextRun(cfg.ID, cfg.CronExpr)
	logger.Success("Added schedule %s (prompt) with cron=%s", cfg.ID, cfg.CronExpr)
}

func (s *Scheduler) RemoveSchedule(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, exists := s.entries[id]; exists {
		s.cron.Remove(entryID)
		delete(s.entries, id)
		s.clearNextRun(id)
		logger.Info("Removed schedule %s", id)
	}
}

func (s *Scheduler) executeSchedule(cfg ScheduleConfig) {
	execID := uuid.New().String()
	now := time.Now().UTC()

	s.db.Exec(
		"INSERT INTO schedule_executions (id, schedule_id, status, started_at) VALUES (?, ?, 'running', ?)",
		execID, cfg.ID, now,
	)

	s.db.Exec("UPDATE schedules SET last_run_at = ?, updated_at = ? WHERE id = ?", now, now, cfg.ID)

	// Advance the next-run time up front rather than after the run: a long run
	// would otherwise leave "Next run" showing a time in the past for its whole
	// duration, and a crash mid-run would look like a missed fire at next boot.
	s.setNextRun(cfg.ID, cfg.CronExpr)

	output, threadID, execErr := s.executePrompt(cfg)

	finishedAt := time.Now().UTC()
	status := "success"
	errStr := ""
	if execErr != nil {
		status = "error"
		errStr = execErr.Error()
		logger.Error("Schedule %s execution failed: %v", cfg.ID, execErr)
	} else {
		logger.Info("Schedule %s executed successfully", cfg.ID)
	}

	s.db.Exec(
		"UPDATE schedule_executions SET status = ?, output = ?, error = ?, finished_at = ? WHERE id = ?",
		status, output, errStr, finishedAt, execID,
	)

	s.fileReport(cfg, execID, status, output, errStr, threadID)
}

// fileReport files the run's outcome into the Inbox.
//
// Both outcomes report. A failed run used to notify nothing at all, so a broken
// scheduled prompt failed silently — the only trace was a row in
// schedule_executions nobody thinks to look at.
func (s *Scheduler) fileReport(cfg ScheduleConfig, execID, status, output, errStr, threadID string) {
	if s.notifyFn == nil {
		return
	}

	// Prefer the agent's display name — "Research Assistant reported back" reads
	// like mail from a colleague, which is how the Inbox presents it.
	who := cfg.AgentRoleSlug
	var agentName string
	s.db.QueryRow("SELECT name FROM agent_roles WHERE slug = ?", cfg.AgentRoleSlug).Scan(&agentName)
	if agentName != "" {
		who = agentName
	}

	var scheduleName string
	s.db.QueryRow("SELECT name FROM schedules WHERE id = ?", cfg.ID).Scan(&scheduleName)
	subject := scheduleName
	if subject == "" {
		subject = truncate(cfg.PromptContent, 60)
	}

	in := models.NotificationInput{
		Prompt:          cfg.PromptContent,
		WorkspaceID:     cfg.WorkspaceID,
		SourceAgentSlug: cfg.AgentRoleSlug,
		SourceType:      "schedule",
		SourceID:        execID,
	}

	if status == "success" {
		in.Title = who + ": " + subject
		in.Detail = output
		in.Body = preview(output)
		in.Priority = "normal"
		if in.Detail == "" {
			in.Detail = "_The agent completed the task without producing a written report._"
			in.Body = "Completed with no output"
		}
	} else {
		in.Title = "Failed — " + who + ": " + subject
		in.Detail = "**This scheduled run failed.**\n\n```\n" + errStr + "\n```\n\n**Prompt**\n\n" + cfg.PromptContent
		in.Body = truncate(errStr, 160)
		in.Priority = "high"
	}

	// A schedule pinned to a thread already wrote its turn there, so link it.
	// Threadless runs leave the link empty and the Inbox offers "Open as chat".
	if threadID != "" {
		in.Link = "/chat/" + threadID
	}

	s.notifyFn(in)
}

// preview reduces a markdown report to a one-line summary for the notification
// list and OS push, skipping headings and blank lines so the line shown is
// actual prose rather than "## Summary".
func preview(report string) string {
	for _, line := range strings.Split(report, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "```") {
			continue
		}
		return truncate(strings.TrimLeft(line, "-*> "), 160)
	}
	return truncate(strings.TrimSpace(report), 160)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}

func (s *Scheduler) executePrompt(cfg ScheduleConfig) (output, threadID string, err error) {
	if s.promptSender == nil {
		logger.Warn("Schedule %s: prompt sender not configured, skipping", cfg.ID)
		return "", "", nil
	}

	// Use the configured agent timeout rather than a fixed five minutes. A
	// scheduled prompt asks an agent to actually do work — with a CLI engine
	// like Claude Code that routinely runs longer, and the old cap killed the
	// subprocess mid-task, surfacing as "signal: killed" with nothing done.
	ctx, cancel := context.WithTimeout(context.Background(), s.promptSender.AgentTimeout())
	defer cancel()

	return s.promptSender.SendScheduledPrompt(ctx, cfg.AgentRoleSlug, cfg.PromptContent, cfg.ThreadID, cfg.WorkspaceID, cfg.Provider)
}

// RunNow executes a schedule immediately (called from API).
func (s *Scheduler) RunNow(cfg ScheduleConfig) {
	go s.executeSchedule(cfg)
}

// StartDataRetention starts a background goroutine that cleans up old dashboard data points daily.
func (s *Scheduler) StartDataRetention() {
	s.retentionStop = make(chan struct{})
	go func() {
		s.cleanupOldDataPoints()

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-s.retentionStop:
				return
			case <-ticker.C:
				s.cleanupOldDataPoints()
			}
		}
	}()
}

func (s *Scheduler) cleanupOldDataPoints() {
	result, err := s.db.Exec("DELETE FROM dashboard_data_points WHERE collected_at < datetime('now', '-30 days')")
	if err != nil {
		logger.Error("Data retention cleanup failed: %v", err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		logger.Info("Data retention: cleaned up %d old data points", rows)
	}
}

// LoadSchedules loads all enabled schedules from the DB and registers them with cron.
func (s *Scheduler) LoadSchedules() {
	rows, err := s.db.Query(
		`SELECT id, cron_expr, agent_role_slug, prompt_content, thread_id, workspace_id, provider
		 FROM schedules WHERE enabled = 1 AND type = 'prompt'`,
	)
	if err != nil {
		logger.Error("Failed to load schedules: %v", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var cfg ScheduleConfig
		var workspaceID sql.NullString
		if err := rows.Scan(&cfg.ID, &cfg.CronExpr, &cfg.AgentRoleSlug, &cfg.PromptContent, &cfg.ThreadID, &workspaceID, &cfg.Provider); err != nil {
			logger.Error("Failed to scan schedule: %v", err)
			continue
		}
		cfg.WorkspaceID = workspaceID.String
		s.AddSchedule(cfg)
		count++
	}
	if count > 0 {
		logger.Info("Loaded %d schedules from database", count)
	}
}
