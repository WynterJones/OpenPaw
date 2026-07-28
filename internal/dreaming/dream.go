package dreaming

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/memory"
	"github.com/openpaw/openpaw/internal/models"
)

const (
	// dreamTimeout bounds a whole run across every agent. A dream is a batch of
	// small calls, not one long one, so this is generous rather than tight.
	dreamTimeout = 45 * time.Minute
	// threadTimeout bounds reading one conversation.
	threadTimeout = 3 * time.Minute
	// consolidateTimeout bounds the review pass, which sends the most context of
	// any call here (a run's facts plus the memories under review).
	consolidateTimeout = 5 * time.Minute

	// transcriptChars caps one conversation's transcript. Long chats are kept
	// head and tail: how a conversation opened and how it resolved carry most of
	// the durable material, and the middle is usually the work being done.
	transcriptChars = 24000

	// candidateWindow is how many of an agent's recent threads are examined for
	// eligibility before the per-run cap is applied. Filtering happens in Go
	// rather than SQL because timestamps are compared as time.Time, not text.
	candidateWindow = 200

	// maxForgets caps deletions in one consolidation, whatever the model asks
	// for. Nothing else in the app deletes memories unprompted, so a single
	// misjudged pass is the one way an agent could quietly lose everything it
	// knows. A dream that wants to drop more than this is doing something the
	// user should be present for.
	maxForgets = 25

	// protectedImportance is the level at or above which the consolidation pass
	// may not delete. These are the memories the agent is shown at the start of
	// every conversation — identity and standing instructions. They can still be
	// rewritten by an update, which is the safe way to correct one.
	protectedImportance = 9
)

// RunStats is the outcome of one agent's dream.
type RunStats struct {
	AgentSlug       string `json:"agent_slug"`
	ThreadsScanned  int    `json:"threads_scanned"`
	FactsFound      int    `json:"facts_found"`
	MemoriesAdded   int    `json:"memories_added"`
	MemoriesUpdated int    `json:"memories_updated"`
	MemoriesPruned  int    `json:"memories_pruned"`
	Summary         string `json:"summary"`
}

// dreamAll runs a dream for every eligible agent, one after another.
//
// Sequential on purpose: these are background calls competing with whatever the
// user is doing in the foreground, and running a fleet of them at once against
// the same provider is how a nightly maintenance pass turns into rate-limit
// errors on the next real message.
func (m *Manager) dreamAll() {
	if !m.dreaming.CompareAndSwap(false, true) {
		logger.Warn("Dreaming is already in progress — skipping this run")
		return
	}
	defer m.dreaming.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), dreamTimeout)
	defer cancel()

	slugs, err := m.dreamingAgents()
	if err != nil {
		logger.Error("Dreaming could not list agents: %v", err)
		return
	}
	if len(slugs) == 0 {
		logger.Info("Dreaming found no eligible agents")
		return
	}

	m.broadcastState(true)
	defer m.broadcastState(false)

	logger.Info("Dreaming started for %d agent(s)", len(slugs))
	for _, slug := range slugs {
		if ctx.Err() != nil {
			logger.Warn("Dreaming ran out of time before reaching %s", slug)
			return
		}
		m.dreamAgent(ctx, slug)
	}
	logger.Success("Dreaming finished")
}

// dreamingAgents lists the agents whose memory this pass owns: enabled, local
// ones. Remote (OpenClaw) agents run on someone else's assistant and keep their
// memory there — reading their chats here and writing to a local memory
// database nothing consults would just burn tokens.
func (m *Manager) dreamingAgents() ([]string, error) {
	rows, err := m.db.Query(
		`SELECT slug FROM agent_roles
		 WHERE enabled = 1 AND COALESCE(remote_provider, '') = ''
		 ORDER BY sort_order ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slugs []string
	for rows.Next() {
		var slug string
		if rows.Scan(&slug) == nil && slug != "" {
			slugs = append(slugs, slug)
		}
	}
	return slugs, rows.Err()
}

// dreamAgent is one agent's whole dream: read the chats it hasn't read, then
// reconcile what it found against what it already knows.
func (m *Manager) dreamAgent(ctx context.Context, slug string) {
	m.mu.RLock()
	cfg := m.cfg
	notify := m.notify
	m.mu.RUnlock()

	runID := uuid.New().String()
	startedAt := time.Now().UTC()
	m.db.Exec(
		"INSERT INTO dream_runs (id, agent_slug, status, started_at) VALUES (?, ?, 'running', ?)",
		runID, slug, startedAt,
	)

	stats, err := m.runDream(ctx, slug, cfg)
	finishedAt := time.Now().UTC()

	status, errStr := "success", ""
	if err != nil {
		status, errStr = "error", err.Error()
		logger.Error("Dreaming failed for %s: %v", slug, err)
	}

	m.db.Exec(
		`UPDATE dream_runs SET status = ?, threads_scanned = ?, facts_found = ?,
		 memories_added = ?, memories_updated = ?, memories_pruned = ?,
		 summary = ?, error = ?, finished_at = ? WHERE id = ?`,
		status, stats.ThreadsScanned, stats.FactsFound, stats.MemoriesAdded,
		stats.MemoriesUpdated, stats.MemoriesPruned, stats.Summary, errStr, finishedAt, runID,
	)

	if m.broadcast != nil {
		m.broadcast("dream_run_finished", map[string]interface{}{
			"id": runID, "status": status, "stats": stats,
		})
	}

	m.fileReport(notify, slug, runID, status, errStr, stats)
}

func (m *Manager) runDream(ctx context.Context, slug string, cfg Config) (RunStats, error) {
	stats := RunStats{AgentSlug: slug}

	m.mem.EnsureMigrated(slug)

	threads, err := m.unscannedThreads(slug, cfg.MaxThreads)
	if err != nil {
		return stats, fmt.Errorf("find unscanned chats: %w", err)
	}

	var facts []memory.Record
	for _, t := range threads {
		if ctx.Err() != nil {
			break
		}
		found, err := m.scanThread(ctx, slug, t)
		if err != nil {
			// One unreadable conversation must not sink the run. Recording the
			// scan anyway would hide the gap, so it is left unscanned and
			// retried on the next dream.
			logger.Warn("Dreaming could not read chat %s for %s: %v", t.ID, slug, err)
			continue
		}
		facts = append(facts, found...)
		stats.ThreadsScanned++
		m.recordScan(slug, t, len(found))
	}
	stats.FactsFound = len(facts)

	existing, err := m.mem.Recent(slug, cfg.ReviewLimit)
	if err != nil {
		return stats, fmt.Errorf("read existing memories: %w", err)
	}

	// Nothing new and nothing stored is not a failure — it is an agent that
	// hasn't been used since the last dream.
	if len(facts) == 0 && len(existing) == 0 {
		stats.Summary = "Nothing new to review."
		return stats, nil
	}

	ops, err := m.consolidate(ctx, slug, facts, existing)
	if err != nil {
		return stats, err
	}

	added, updated, pruned := m.applyOps(slug, ops, existing)
	stats.MemoriesAdded, stats.MemoriesUpdated, stats.MemoriesPruned = added, updated, pruned
	stats.Summary = strings.TrimSpace(ops.Summary)
	if stats.Summary == "" {
		stats.Summary = fmt.Sprintf("Reviewed %d chat(s) and %d memory(s).", stats.ThreadsScanned, len(existing))
	}

	if m.broadcast != nil && (added+updated+pruned) > 0 {
		m.broadcast("memories_updated", map[string]interface{}{
			"agent_slug": slug,
			"added":      added,
			"updated":    updated,
			"pruned":     pruned,
			"source":     "dream",
		})
	}

	return stats, nil
}

// threadRef is a conversation eligible for scanning.
type threadRef struct {
	ID        string
	Title     string
	UpdatedAt time.Time
}

// unscannedThreads returns the agent's conversations that this dream should
// read: ones it has never scanned, plus ones that have been added to since the
// last scan. A chat that hasn't changed is skipped — that is the whole point of
// the scan ledger, and re-reading a finished conversation every night would be
// the single largest cost in the system.
//
// Freshness is judged by chat_threads.updated_at rather than by aggregating the
// message timestamps. Two reasons: the driver only converts a column to
// time.Time when the column's declared type says so, and MAX(created_at) has no
// declared type — it comes back as a string that will not scan into a time. And
// updated_at is bumped on every message anyway. It is also bumped by a rename or
// a pin, which at worst costs one redundant re-read; the failure that matters is
// missing a conversation, and this cannot.
func (m *Manager) unscannedThreads(slug string, limit int) ([]threadRef, error) {
	rows, err := m.db.Query(
		`SELECT t.id, t.title, t.updated_at, ds.last_message_at
		 FROM chat_threads t
		 LEFT JOIN dream_scans ds ON ds.thread_id = t.id AND ds.agent_slug = ?
		 WHERE EXISTS (
		     SELECT 1 FROM chat_messages m2
		     WHERE m2.thread_id = t.id AND m2.agent_role_slug = ?
		 )
		 ORDER BY t.updated_at DESC
		 LIMIT ?`,
		slug, slug, candidateWindow,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []threadRef
	for rows.Next() {
		var t threadRef
		var title sql.NullString
		var updatedAt, scannedThrough sql.NullTime
		if err := rows.Scan(&t.ID, &title, &updatedAt, &scannedThrough); err != nil {
			logger.Warn("Dreaming skipped an unreadable chat row: %v", err)
			continue
		}
		// Compared in Go rather than in SQL: these columns hold driver-formatted
		// timestamps that SQLite's own date functions do not reliably parse, so
		// a julianday() or text comparison would decide wrongly and silently.
		if scannedThrough.Valid && updatedAt.Valid && !updatedAt.Time.After(scannedThrough.Time) {
			continue
		}
		t.Title = title.String
		t.UpdatedAt = updatedAt.Time
		out = append(out, t)
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

// scanThread reads one conversation and returns the durable facts in it.
func (m *Manager) scanThread(ctx context.Context, slug string, t threadRef) ([]memory.Record, error) {
	transcript, err := m.transcript(t.ID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(transcript) == "" {
		return nil, nil
	}

	callCtx, cancel := context.WithTimeout(ctx, threadTimeout)
	defer cancel()

	title := t.Title
	if title == "" {
		title = "Untitled conversation"
	}
	prompt := fmt.Sprintf("## CONVERSATION: %s\n\n%s\n\nWhat durable facts are in this conversation?", title, transcript)

	out, err := m.thinker.GatewayOneShot(callCtx, extractSystem, prompt)
	if err != nil {
		return nil, fmt.Errorf("extract model call: %w", err)
	}

	var parsed struct {
		Facts []memory.Record `json:"facts"`
	}
	if err := json.Unmarshal([]byte(extractJSON(out)), &parsed); err != nil {
		return nil, fmt.Errorf("extract returned unparseable JSON: %w", err)
	}

	facts := make([]memory.Record, 0, len(parsed.Facts))
	for _, f := range parsed.Facts {
		if strings.TrimSpace(f.Content) == "" {
			continue
		}
		f.ID = "" // ids from an extract pass are meaningless; only the review assigns them
		f.Source = "dream"
		facts = append(facts, f)
	}
	return facts, nil
}

// transcript renders a conversation for the extract pass, keeping the head and
// tail when it is too long to send whole.
func (m *Manager) transcript(threadID string) (string, error) {
	rows, err := m.db.Query(
		`SELECT role, COALESCE(agent_role_slug, ''), content
		 FROM chat_messages WHERE thread_id = ? ORDER BY created_at ASC`,
		threadID,
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var b strings.Builder
	for rows.Next() {
		var role, agentSlug, content string
		if rows.Scan(&role, &agentSlug, &content) != nil {
			continue
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		label := role
		if role == "assistant" && agentSlug != "" {
			label = "assistant/" + agentSlug
		}
		fmt.Fprintf(&b, "[%s]: %s\n\n", label, content)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	s := b.String()
	if len(s) <= transcriptChars {
		return s, nil
	}
	half := transcriptChars / 2
	return s[:half] + "\n\n… [middle of conversation omitted] …\n\n" + s[len(s)-half:], nil
}

// recordScan marks a conversation as read up to its newest message, so the next
// dream skips it unless it has been added to since.
func (m *Manager) recordScan(slug string, t threadRef, factsFound int) {
	if _, err := m.db.Exec(
		`INSERT INTO dream_scans (id, agent_slug, thread_id, facts_found, last_message_at, scanned_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agent_slug, thread_id) DO UPDATE SET
		     facts_found = excluded.facts_found,
		     last_message_at = excluded.last_message_at,
		     scanned_at = excluded.scanned_at`,
		uuid.New().String(), slug, t.ID, factsFound, t.UpdatedAt, time.Now().UTC(),
	); err != nil {
		logger.Warn("Dreaming could not record the scan of chat %s: %v", t.ID, err)
		return
	}
	if m.broadcast != nil {
		m.broadcast("dream_scanned", map[string]interface{}{
			"thread_id":   t.ID,
			"agent_slug":  slug,
			"facts_found": factsFound,
		})
	}
}

// consolidateOps is the review pass's verdict.
type consolidateOps struct {
	Add    []memory.Record `json:"add"`
	Update []memory.Record `json:"update"`
	Forget []struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	} `json:"forget"`
	Summary string `json:"summary"`
}

// consolidate reconciles the newly harvested facts against stored memory.
func (m *Manager) consolidate(ctx context.Context, slug string, facts, existing []memory.Record) (consolidateOps, error) {
	var ops consolidateOps

	var prompt strings.Builder
	prompt.WriteString("## NEW FACTS (from chats not reviewed before)\n\n")
	if len(facts) == 0 {
		prompt.WriteString("_(none — no new conversations since the last review)_\n")
	}
	for _, f := range facts {
		fmt.Fprintf(&prompt, "- [%s|importance %d] %s\n", f.Category, f.Importance, singleLine(f.Content))
	}

	prompt.WriteString("\n## EXISTING MEMORIES\n\n")
	if len(existing) == 0 {
		prompt.WriteString("_(none stored yet)_\n")
	}
	for _, e := range existing {
		fmt.Fprintf(&prompt, "- id=%s [%s|importance %d] %s\n", e.ID, e.Category, e.Importance, singleLine(e.Content))
	}
	prompt.WriteString("\nConsolidate. Use only ids listed above.")

	callCtx, cancel := context.WithTimeout(ctx, consolidateTimeout)
	defer cancel()

	out, err := m.thinker.GatewayOneShot(callCtx, consolidateSystem, prompt.String())
	if err != nil {
		return ops, fmt.Errorf("consolidate model call: %w", err)
	}
	if err := json.Unmarshal([]byte(extractJSON(out)), &ops); err != nil {
		return ops, fmt.Errorf("consolidate returned unparseable JSON: %w", err)
	}
	return ops, nil
}

// applyOps writes the review's verdict to the memory database, enforcing the
// limits the prompt only asks for.
//
// Every id is checked against the set that was actually under review. A model
// inventing an id is harmless on its own — the delete would match no row — but
// an id it half-remembered from a different agent would not be, and this is the
// one code path in the app that deletes memories without a person watching.
func (m *Manager) applyOps(slug string, ops consolidateOps, existing []memory.Record) (added, updated, pruned int) {
	known := make(map[string]memory.Record, len(existing))
	for _, e := range existing {
		known[e.ID] = e
	}

	for _, rec := range ops.Add {
		if strings.TrimSpace(rec.Content) == "" {
			continue
		}
		if m.mem.HasSimilar(slug, rec.Content, rec.Summary) {
			continue
		}
		rec.ID = ""
		rec.Source = "dream"
		if _, err := m.mem.Add(slug, rec); err != nil {
			logger.Warn("Dreaming could not add a memory for %s: %v", slug, err)
			continue
		}
		added++
	}

	for _, rec := range ops.Update {
		if _, ok := known[rec.ID]; !ok {
			logger.Warn("Dreaming tried to update unknown memory %q for %s — skipped", rec.ID, slug)
			continue
		}
		if err := m.mem.Update(slug, rec); err != nil {
			logger.Warn("Dreaming could not update memory %s for %s: %v", rec.ID, slug, err)
			continue
		}
		updated++
	}

	for _, f := range ops.Forget {
		if pruned >= maxForgets {
			logger.Warn("Dreaming stopped at %d deletions for %s — the rest were left in place", maxForgets, slug)
			break
		}
		prev, ok := known[f.ID]
		if !ok {
			logger.Warn("Dreaming tried to forget unknown memory %q for %s — skipped", f.ID, slug)
			continue
		}
		if prev.Importance >= protectedImportance {
			logger.Warn("Dreaming kept memory %s for %s: importance %d is protected from deletion", f.ID, slug, prev.Importance)
			continue
		}
		if err := m.mem.Forget(slug, f.ID); err != nil {
			logger.Warn("Dreaming could not forget memory %s for %s: %v", f.ID, slug, err)
			continue
		}
		pruned++
	}

	return added, updated, pruned
}

// fileReport puts the run's outcome in the Inbox.
//
// A dream that quietly did nothing and a dream that quietly rewrote forty
// memories look identical from the outside, and both happened overnight. Only
// runs that actually changed something (or failed) are reported — a nightly
// "reviewed nothing" notification is how a user learns to ignore the Inbox.
func (m *Manager) fileReport(notify NotifyFunc, slug, runID, status, errStr string, stats RunStats) {
	if notify == nil {
		return
	}
	changed := stats.MemoriesAdded + stats.MemoriesUpdated + stats.MemoriesPruned
	if status == "success" && changed == 0 {
		return
	}

	who := slug
	var agentName string
	m.db.QueryRow("SELECT name FROM agent_roles WHERE slug = ?", slug).Scan(&agentName)
	if agentName != "" {
		who = agentName
	}

	in := models.NotificationInput{
		WorkspaceID:     m.db.ActiveWorkspaceID(),
		SourceAgentSlug: slug,
		SourceType:      "dream",
		SourceID:        runID,
	}

	if status == "success" {
		in.Title = who + ": dreamed"
		in.Body = fmt.Sprintf("%d added · %d updated · %d forgotten", stats.MemoriesAdded, stats.MemoriesUpdated, stats.MemoriesPruned)
		in.Priority = "low"
		in.Detail = fmt.Sprintf(
			"**%s reviewed its memory while you were away.**\n\n%s\n\n"+
				"| | |\n|---|---|\n| Chats read | %d |\n| Facts found | %d |\n| Memories added | %d |\n"+
				"| Memories updated | %d |\n| Memories forgotten | %d |\n",
			who, stats.Summary, stats.ThreadsScanned, stats.FactsFound,
			stats.MemoriesAdded, stats.MemoriesUpdated, stats.MemoriesPruned,
		)
	} else {
		in.Title = "Failed — " + who + ": dreaming"
		in.Body = truncate(errStr, 160)
		in.Priority = "high"
		in.Detail = "**This dream failed.**\n\n```\n" + errStr + "\n```"
	}

	notify(in)
}

// broadcastState tells the UI a dream started or ended, so the Dreaming
// settings can show it running without polling.
func (m *Manager) broadcastState(running bool) {
	if m.broadcast == nil {
		return
	}
	m.broadcast("dreaming_state", map[string]interface{}{"running": running})
}
