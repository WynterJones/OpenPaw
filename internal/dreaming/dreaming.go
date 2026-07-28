// Package dreaming turns what happened in an agent's chats into memory.
//
// Agents almost never call memory_save mid-turn — they are busy answering — so
// left to themselves their memory stays close to empty no matter how firmly the
// prompt asks. Something else has to do the remembering. Two things here can,
// and they run at very different rhythms:
//
//   - The dream runs on a cron and is the default path. It reads the chats the
//     agent took part in but has not read yet, extracts the facts in them, and
//     then reviews those facts against the memories already stored: merging
//     duplicates, sharpening wording, dropping what has gone stale. That review
//     is what keeps memory from becoming a landfill, which is what any
//     append-only capture eventually becomes.
//
//   - The reflex fires after every single reply and captures the same material
//     sooner. It is off by default, because it substantially duplicates the
//     dream: both read the same conversation, the dream just gets to it later
//     and sees the whole arc rather than one turn. Paying a model call per
//     message to be told "nothing here" — which is the correct answer for most
//     exchanges, and what its prompt deliberately steers towards — is a poor
//     trade unless same-day recall actually matters to you.
//
// The reflex does cover one thing the dream cannot: the middle of a very long
// conversation, which transcript truncation drops. Turn it on if that bites.
//
// Both run on the gateway model rather than the agent's own. The work is
// summarisation and bookkeeping, the gateway is already the model configured for
// that kind of pass, and it means a fleet of agents on expensive models doesn't
// multiply the cost of routine memory upkeep.
package dreaming

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/memory"
	"github.com/openpaw/openpaw/internal/models"
	"github.com/robfig/cron/v3"
)

// Thinker runs a single completion on the gateway model. Satisfied by
// agents.Manager; kept as an interface here so this package doesn't import
// agents (which would be a cycle by way of the memory manager it holds).
type Thinker interface {
	GatewayOneShot(ctx context.Context, system, prompt string) (string, error)
}

// BroadcastFunc pushes a WebSocket event to connected clients.
type BroadcastFunc func(msgType string, payload interface{})

// NotifyFunc files an Inbox notification.
type NotifyFunc func(models.NotificationInput)

// Config is the user-facing dreaming setup.
type Config struct {
	// Enabled gates the cron. The reflex has its own switch — capturing
	// memories per-reply and consolidating them nightly are independently
	// useful, and the reflex costs a small call per turn while the dream costs
	// a burst once a night.
	Enabled bool `json:"enabled"`
	// CronExpr is a 6-field expression (with seconds), matching the scheduler's
	// parser so the two agree on what a given string means.
	CronExpr string `json:"cron_expr"`
	// MaxThreads caps how many unscanned chats one agent reads per dream, so a
	// first run against a long history doesn't turn into an unbounded bill.
	MaxThreads int `json:"max_threads"`
	// ReviewLimit is how many of the most recent memories the consolidation
	// pass reconsiders each night.
	ReviewLimit int `json:"review_limit"`
	// ReflexEnabled gates the after-every-reply capture.
	ReflexEnabled bool `json:"reflex_enabled"`
}

// DefaultConfig is dreaming on nightly, reflex off.
//
// Only one of the two should normally be running. They read the same
// conversations for the same facts, so having both on means paying twice for
// the same material — and the reflex pays per message, including the majority
// of exchanges that contain nothing worth keeping.
//
// The dream is the one left on because it is strictly cheaper for the same
// coverage and because its consolidation pass is the half that has no
// substitute: without it, capture is append-only and memory degrades into a
// pile nobody can use. Its cost is bounded by MaxThreads per agent per run
// regardless of how much was said that day, which the reflex's cannot be.
//
// That does mean deletions happen unattended on a schedule out of the box, so
// the deletion guards in applyOps are load-bearing rather than belt-and-braces.
func DefaultConfig() Config {
	return Config{
		Enabled:       true,
		CronExpr:      CronDaily,
		MaxThreads:    15,
		ReviewLimit:   100,
		ReflexEnabled: false,
	}
}

// Cron presets offered in the UI. Everything runs at 3am local time, when the
// user is least likely to be mid-conversation with an agent whose memories are
// about to be rewritten underneath it.
const (
	CronHourly  = "0 0 * * * *"
	CronDaily   = "0 0 3 * * *"
	CronWeekly  = "0 0 3 * * 0"
	CronMonthly = "0 0 3 1 * *"
)

// cronParser mirrors the scheduler's, so a cron expression means the same thing
// in both places.
var cronParser = cron.NewParser(
	cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

type Manager struct {
	db        *database.DB
	mem       *memory.Manager
	thinker   Thinker
	broadcast BroadcastFunc
	notify    NotifyFunc

	mu      sync.RWMutex
	cfg     Config
	cron    *cron.Cron
	entryID cron.EntryID
	started bool

	// dreaming is the in-flight guard. A dream can outlast its own interval on
	// a large history, and two passes consolidating the same memory database
	// concurrently would each act on a view the other is invalidating.
	dreaming atomic.Bool
}

func New(db *database.DB, mem *memory.Manager, thinker Thinker, broadcast BroadcastFunc) *Manager {
	return &Manager{
		db:        db,
		mem:       mem,
		thinker:   thinker,
		broadcast: broadcast,
		cfg:       DefaultConfig(),
	}
}

func (m *Manager) SetNotifyFunc(fn NotifyFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notify = fn
}

// LoadConfig reads dreaming settings from the database.
func (m *Manager) LoadConfig() {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := DefaultConfig()

	rows, err := m.db.Query(
		`SELECT key, value FROM settings WHERE key IN
		 ('dreaming_enabled', 'dreaming_cron', 'dreaming_max_threads',
		  'dreaming_review_limit', 'dreaming_reflex_enabled')`,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var key, val string
			if rows.Scan(&key, &val) != nil || val == "" {
				continue
			}
			switch key {
			case "dreaming_enabled":
				cfg.Enabled = val == "true" || val == "1"
			case "dreaming_reflex_enabled":
				cfg.ReflexEnabled = val == "true" || val == "1"
			case "dreaming_cron":
				if _, err := cronParser.Parse(val); err == nil {
					cfg.CronExpr = val
				} else {
					logger.Warn("Ignoring unparseable dreaming cron %q: %v", val, err)
				}
			case "dreaming_max_threads":
				if v := parseInt(val); v > 0 {
					cfg.MaxThreads = clamp(v, 1, 100)
				}
			case "dreaming_review_limit":
				if v := parseInt(val); v > 0 {
					cfg.ReviewLimit = clamp(v, 10, 500)
				}
			}
		}
	}

	m.cfg = cfg
}

// GetConfig returns the current config as the string map the settings API uses.
func (m *Manager) GetConfig() map[string]string {
	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	return map[string]string{
		"dreaming_enabled":        boolStr(cfg.Enabled),
		"dreaming_cron":           cfg.CronExpr,
		"dreaming_max_threads":    fmt.Sprintf("%d", cfg.MaxThreads),
		"dreaming_review_limit":   fmt.Sprintf("%d", cfg.ReviewLimit),
		"dreaming_reflex_enabled": boolStr(cfg.ReflexEnabled),
		"dreaming_next_run":       m.nextRunISO(),
		"dreaming_running":        boolStr(m.dreaming.Load()),
	}
}

// UpdateConfig persists new settings and re-registers the cron.
func (m *Manager) UpdateConfig(in map[string]string) error {
	// Reject a bad expression before writing it, rather than silently falling
	// back at load time — a user who typed a cron and was told nothing would
	// reasonably believe it took effect.
	if expr, ok := in["dreaming_cron"]; ok && strings.TrimSpace(expr) != "" {
		if _, err := cronParser.Parse(strings.TrimSpace(expr)); err != nil {
			return fmt.Errorf("invalid cron expression %q: %w", expr, err)
		}
	}

	for key, val := range in {
		switch key {
		case "dreaming_enabled", "dreaming_cron", "dreaming_max_threads",
			"dreaming_review_limit", "dreaming_reflex_enabled":
		default:
			continue // ignore anything not ours
		}
		if _, err := m.db.Exec(
			"INSERT INTO settings (id, key, value) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
			"dream-"+key, key, strings.TrimSpace(val),
		); err != nil {
			return fmt.Errorf("save %s: %w", key, err)
		}
	}

	m.LoadConfig()
	m.reschedule()
	return nil
}

// Start registers the dream cron if dreaming is enabled.
func (m *Manager) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.cron = cron.New(cron.WithSeconds())
	m.started = true
	m.cron.Start()
	m.mu.Unlock()

	m.reschedule()
}

// Stop tears down the cron. In-flight dreams are left to finish — they hold a
// context of their own and abandoning one mid-consolidation could leave the
// memory database half-rewritten.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started || m.cron == nil {
		return
	}
	m.cron.Stop()
	m.cron = nil
	m.started = false
	logger.Success("Dreaming stopped")
}

// reschedule points the cron at the current config, adding or removing the
// entry as the enabled flag dictates.
func (m *Manager) reschedule() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started || m.cron == nil {
		return
	}

	if m.entryID != 0 {
		m.cron.Remove(m.entryID)
		m.entryID = 0
	}

	if !m.cfg.Enabled {
		logger.Info("Dreaming is disabled")
		return
	}

	id, err := m.cron.AddFunc(m.cfg.CronExpr, func() { m.dreamAll() })
	if err != nil {
		logger.Error("Failed to schedule dreaming with cron %q: %v", m.cfg.CronExpr, err)
		return
	}
	m.entryID = id
	logger.Success("Dreaming scheduled (cron=%s)", m.cfg.CronExpr)
}

// nextRunISO reports when the next dream is due, or "" when none is scheduled.
// Called with m.mu already held for reading by GetConfig.
func (m *Manager) nextRunISO() string {
	if !m.cfg.Enabled {
		return ""
	}
	schedule, err := cronParser.Parse(m.cfg.CronExpr)
	if err != nil {
		return ""
	}
	return schedule.Next(time.Now()).UTC().Format(time.RFC3339)
}

// ReflexEnabled reports whether per-reply memory capture is on.
func (m *Manager) ReflexEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.ReflexEnabled
}

// IsDreaming reports whether a dream is in flight.
func (m *Manager) IsDreaming() bool { return m.dreaming.Load() }

// RunNow starts a dream immediately, regardless of the enabled flag — the user
// pressed the button, which is a clearer signal of intent than the schedule.
func (m *Manager) RunNow() { go m.dreamAll() }

func parseInt(s string) int {
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
