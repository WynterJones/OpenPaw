package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/robfig/cron/v3"
)

// PromptSender sends a scheduled prompt to an AI agent.
type PromptSender interface {
	SendScheduledPrompt(ctx context.Context, slug, prompt, threadID string) (string, error)
}

// NotifyFunc creates a notification and broadcasts it.
type NotifyFunc func(title, body, priority, sourceAgentSlug, sourceType, link string)

type ScheduleConfig struct {
	ID            string
	CronExpr      string
	AgentRoleSlug string
	PromptContent string
	ThreadID      string
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
	s.cron.Start()
	logger.Success("Scheduler started")
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
	logger.Success("Added schedule %s (prompt) with cron=%s", cfg.ID, cfg.CronExpr)
}

func (s *Scheduler) RemoveSchedule(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, exists := s.entries[id]; exists {
		s.cron.Remove(entryID)
		delete(s.entries, id)
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

	output, execErr := s.executePrompt(cfg)

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
}

func (s *Scheduler) executePrompt(cfg ScheduleConfig) (string, error) {
	if s.promptSender == nil {
		logger.Warn("Schedule %s: prompt sender not configured, skipping", cfg.ID)
		return "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return s.promptSender.SendScheduledPrompt(ctx, cfg.AgentRoleSlug, cfg.PromptContent, cfg.ThreadID)
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
		`SELECT id, cron_expr, agent_role_slug, prompt_content, thread_id
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
		if err := rows.Scan(&cfg.ID, &cfg.CronExpr, &cfg.AgentRoleSlug, &cfg.PromptContent, &cfg.ThreadID); err != nil {
			logger.Error("Failed to scan schedule: %v", err)
			continue
		}
		s.AddSchedule(cfg)
		count++
	}
	if count > 0 {
		logger.Info("Loaded %d schedules from database", count)
	}
}
