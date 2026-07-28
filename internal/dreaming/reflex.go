package dreaming

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/memory"
)

// reflexTimeout bounds the per-reply capture. It runs detached from the chat
// turn, so a slow one costs the user nothing directly — but an unbounded one on
// a wedged provider would leak a goroutine per message sent.
const reflexTimeout = 90 * time.Second

// reflexMaxMemories caps what one exchange may contribute. The prompt asks for
// at most three; this is the enforcement, because a prompt is not a limit.
const reflexMaxMemories = 3

// reflexContextChars bounds how much of the exchange is sent. Agent replies can
// run to tens of thousands of characters of generated code, none of which is
// what we're mining for — the durable material is nearly always near the top.
const reflexContextChars = 6000

// Reflect looks over one completed exchange and saves whatever in it is worth
// remembering. Safe to call in a goroutine and safe to call with a cancelled
// parent — it takes a background context of its own precisely because the chat
// turn it belongs to is over by the time this matters.
//
// Errors are logged, never surfaced: a failed memory capture must not turn into
// a visible failure of a reply that already succeeded.
func (m *Manager) Reflect(agentSlug, userMessage, agentResponse string) {
	if !m.ReflexEnabled() {
		return
	}
	agentSlug = strings.TrimSpace(agentSlug)
	if agentSlug == "" || strings.TrimSpace(userMessage) == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), reflexTimeout)
	defer cancel()

	added, err := m.reflect(ctx, agentSlug, userMessage, agentResponse)
	if err != nil {
		logger.Warn("Memory reflex for %s: %v", agentSlug, err)
		return
	}
	if added > 0 {
		logger.Info("Memory reflex saved %d memory(s) for %s", added, agentSlug)
		if m.broadcast != nil {
			m.broadcast("memories_updated", map[string]interface{}{
				"agent_slug": agentSlug,
				"added":      added,
				"source":     "reflex",
			})
		}
	}
}

func (m *Manager) reflect(ctx context.Context, agentSlug, userMessage, agentResponse string) (int, error) {
	m.mem.EnsureMigrated(agentSlug)

	var prompt strings.Builder
	prompt.WriteString("## EXISTING MEMORIES\n\n")
	prompt.WriteString(m.recentSummaries(agentSlug, 40))
	prompt.WriteString("\n\n## THE EXCHANGE\n\n### User said\n\n")
	prompt.WriteString(truncate(userMessage, reflexContextChars))
	prompt.WriteString("\n\n### Agent replied\n\n")
	prompt.WriteString(truncate(agentResponse, reflexContextChars))
	prompt.WriteString("\n\nWhat, if anything, from this exchange should be remembered permanently?")

	out, err := m.thinker.GatewayOneShot(ctx, reflexSystem, prompt.String())
	if err != nil {
		return 0, fmt.Errorf("reflex model call: %w", err)
	}

	var parsed struct {
		Memories []memory.Record `json:"memories"`
	}
	if err := json.Unmarshal([]byte(extractJSON(out)), &parsed); err != nil {
		return 0, fmt.Errorf("reflex returned unparseable JSON: %w", err)
	}

	added := 0
	for _, rec := range parsed.Memories {
		if added >= reflexMaxMemories {
			break
		}
		if strings.TrimSpace(rec.Content) == "" {
			continue
		}
		if m.mem.HasSimilar(agentSlug, rec.Content, rec.Summary) {
			continue
		}
		rec.Source = "reflex"
		if _, err := m.mem.Add(agentSlug, rec); err != nil {
			logger.Warn("Reflex could not save a memory for %s: %v", agentSlug, err)
			continue
		}
		added++
	}
	return added, nil
}

// recentSummaries renders the agent's latest memories as a one-line-each list,
// so the model can see what is already known and decline to write it down
// again. Summaries only — full contents would dominate the prompt and the
// dedupe judgement doesn't need them.
func (m *Manager) recentSummaries(agentSlug string, limit int) string {
	records, err := m.mem.Recent(agentSlug, limit)
	if err != nil || len(records) == 0 {
		return "_(none yet — this agent has no memories)_"
	}

	var b strings.Builder
	for _, r := range records {
		line := r.Summary
		if line == "" {
			line = truncate(r.Content, 120)
		}
		fmt.Fprintf(&b, "- [%s] %s\n", r.Category, singleLine(line))
	}
	return b.String()
}

// extractJSON pulls the JSON object out of a model reply that was asked for
// bare JSON and, often enough to matter, wrapped it in a fence or prefaced it
// with a sentence anyway.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if fence := strings.Index(s, "```"); fence >= 0 {
		rest := s[fence+3:]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[nl+1:]
		}
		if end := strings.Index(rest, "```"); end >= 0 {
			rest = rest[:end]
		}
		s = strings.TrimSpace(rest)
	}
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n… [truncated]"
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
