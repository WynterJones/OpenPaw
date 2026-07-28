package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openpaw/openpaw/internal/models"
)

// ListRuns is hand-written SQL against a table added in the same change, so a
// mismatched column name or scan order would only show up as an empty history
// panel at runtime.
func TestDreamingListRuns(t *testing.T) {
	db := newTestDB(t)
	h := NewDreamingHandler(db, nil)

	if _, err := db.Exec(
		"INSERT INTO agent_roles (id, slug, name, description, system_prompt, enabled) VALUES ('a1', 'scout', 'Scout', '', '', 1)",
	); err != nil {
		t.Fatalf("insert agent role: %v", err)
	}

	started := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	if _, err := db.Exec(
		`INSERT INTO dream_runs (id, agent_slug, status, threads_scanned, facts_found,
		 memories_added, memories_updated, memories_pruned, summary, error, started_at, finished_at)
		 VALUES ('r1', 'scout', 'success', 4, 9, 3, 2, 1, 'Merged duplicates.', '', ?, ?)`,
		started, started.Add(time.Minute),
	); err != nil {
		t.Fatalf("insert dream run: %v", err)
	}

	// A run for an agent that has since been deleted must still list — the whole
	// point of the history is that it outlives what it describes.
	if _, err := db.Exec(
		`INSERT INTO dream_runs (id, agent_slug, status, summary, started_at)
		 VALUES ('r2', 'ghost', 'error', '', ?)`,
		started.Add(-time.Hour),
	); err != nil {
		t.Fatalf("insert orphan dream run: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ListRuns(rec, httptest.NewRequest(http.MethodGet, "/dreaming/runs", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var runs []models.DreamRun
	if err := json.Unmarshal(rec.Body.Bytes(), &runs); err != nil {
		t.Fatalf("decode response: %v — body %s", err, rec.Body.String())
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}

	// Newest first.
	got := runs[0]
	if got.ID != "r1" {
		t.Fatalf("expected the newest run first, got %q", got.ID)
	}
	if got.AgentName != "Scout" {
		t.Errorf("agent_name = %q, want the display name %q", got.AgentName, "Scout")
	}
	if got.ThreadsScanned != 4 || got.FactsFound != 9 {
		t.Errorf("counts = %d chats / %d facts, want 4 / 9", got.ThreadsScanned, got.FactsFound)
	}
	if got.MemoriesAdded != 3 || got.MemoriesUpdated != 2 || got.MemoriesPruned != 1 {
		t.Errorf("memory counts = +%d ~%d -%d, want +3 ~2 -1",
			got.MemoriesAdded, got.MemoriesUpdated, got.MemoriesPruned)
	}
	if got.Summary != "Merged duplicates." {
		t.Errorf("summary = %q", got.Summary)
	}
	if got.FinishedAt == nil {
		t.Error("finished_at was dropped")
	}

	// Falls back to the slug when the agent row is gone.
	if runs[1].AgentName != "ghost" {
		t.Errorf("orphan run agent_name = %q, want the slug %q", runs[1].AgentName, "ghost")
	}

	// ?agent= filters.
	rec = httptest.NewRecorder()
	h.ListRuns(rec, httptest.NewRequest(http.MethodGet, "/dreaming/runs?agent=scout", nil))
	runs = nil
	json.Unmarshal(rec.Body.Bytes(), &runs)
	if len(runs) != 1 || runs[0].ID != "r1" {
		t.Errorf("agent filter returned %+v", runs)
	}
}
