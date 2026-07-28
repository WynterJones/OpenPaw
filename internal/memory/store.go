package memory

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Record is a single memory row, in the shape callers outside this package
// (the dreaming pass, the reflex writer) need to read and write one.
//
// The tool handlers in tools.go build their SQL inline from JSON tool input;
// these helpers exist so a Go caller doesn't have to hand-roll the same
// statements — and so the invariants (importance clamping, summary fallback,
// timestamp format) live in one place rather than at every call site.
type Record struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	Summary    string `json:"summary"`
	Category   string `json:"category"`
	Importance int    `json:"importance"`
	Source     string `json:"source"`
	Tags       string `json:"tags"`
	CreatedAt  string `json:"created_at"`
}

// sqlTime is the format the rest of this package writes timestamps in. SQLite
// compares DATETIME columns as text, so a differently-formatted write would sort
// and range-filter wrongly against every existing row.
const sqlTime = "2006-01-02 15:04:05"

// normalize fills in the derived fields a caller shouldn't have to think about.
func (r *Record) normalize() {
	r.Content = strings.TrimSpace(r.Content)
	r.Summary = strings.TrimSpace(r.Summary)
	r.Category = strings.TrimSpace(r.Category)
	r.Tags = strings.TrimSpace(r.Tags)

	if r.Category == "" {
		r.Category = "general"
	}
	if r.Source == "" {
		r.Source = "agent"
	}
	if r.Summary == "" {
		r.Summary = firstLine(r.Content, 120)
	}
	if r.Importance <= 0 {
		r.Importance = 5
	}
	if r.Importance > 10 {
		r.Importance = 10
	}
}

// Recent returns the newest active memories, most recent first. This is what a
// dreaming pass reviews — the window is deliberately bounded, because handing a
// model every memory an agent has ever stored gets expensive and stops fitting
// long before the memory database stops growing.
func (m *Manager) Recent(slug string, limit int) ([]Record, error) {
	if limit <= 0 {
		limit = 100
	}
	db, err := m.GetDB(slug)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(
		`SELECT id, content, summary, category, importance, source, tags, created_at
		 FROM memories WHERE archived = 0
		 ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.Content, &r.Summary, &r.Category, &r.Importance,
			&r.Source, &r.Tags, &r.CreatedAt); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Add stores a new memory and returns its id.
func (m *Manager) Add(slug string, r Record) (string, error) {
	r.normalize()
	if r.Content == "" {
		return "", fmt.Errorf("memory content is empty")
	}

	db, err := m.GetDB(slug)
	if err != nil {
		return "", err
	}

	id := uuid.New().String()
	now := time.Now().UTC().Format(sqlTime)
	_, err = db.Exec(
		`INSERT INTO memories (id, content, summary, category, importance, source, tags, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, r.Content, r.Summary, r.Category, r.Importance, r.Source, r.Tags, now, now,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// Update rewrites an existing memory in place. Only non-empty fields are
// applied, so a consolidation that only sharpens the wording doesn't have to
// restate the category and tags to avoid blanking them.
func (m *Manager) Update(slug string, r Record) error {
	if r.ID == "" {
		return fmt.Errorf("memory id is required")
	}

	db, err := m.GetDB(slug)
	if err != nil {
		return err
	}

	sets := []string{"updated_at = ?"}
	args := []interface{}{time.Now().UTC().Format(sqlTime)}

	if c := strings.TrimSpace(r.Content); c != "" {
		sets = append(sets, "content = ?")
		args = append(args, c)
	}
	if s := strings.TrimSpace(r.Summary); s != "" {
		sets = append(sets, "summary = ?")
		args = append(args, s)
	}
	if c := strings.TrimSpace(r.Category); c != "" {
		sets = append(sets, "category = ?")
		args = append(args, c)
	}
	if t := strings.TrimSpace(r.Tags); t != "" {
		sets = append(sets, "tags = ?")
		args = append(args, t)
	}
	if r.Importance > 0 {
		imp := r.Importance
		if imp > 10 {
			imp = 10
		}
		sets = append(sets, "importance = ?")
		args = append(args, imp)
	}

	args = append(args, r.ID)
	_, err = db.Exec("UPDATE memories SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

// Forget deletes a memory outright. Reported separately from Update by callers
// so a run's history says how many memories it removed.
func (m *Manager) Forget(slug, id string) error {
	if id == "" {
		return fmt.Errorf("memory id is required")
	}
	db, err := m.GetDB(slug)
	if err != nil {
		return err
	}
	_, err = db.Exec("DELETE FROM memories WHERE id = ?", id)
	return err
}

// HasSimilar reports whether an active memory already says essentially this.
//
// A cheap guard for the reflex writer, which runs after every single reply: the
// same standing fact ("prefers TypeScript") comes up in conversation constantly,
// and without this the database fills with near-identical rows that the nightly
// consolidation then has to pay to merge. Comparison is on the normalized text,
// so punctuation and casing differences don't defeat it — anything subtler is
// left to the dreaming pass, which has a model to judge with.
func (m *Manager) HasSimilar(slug, content, summary string) bool {
	db, err := m.GetDB(slug)
	if err != nil {
		return false
	}

	want := normalizeText(content)
	wantSummary := normalizeText(summary)
	if want == "" && wantSummary == "" {
		return false
	}

	rows, err := db.Query(
		`SELECT content, summary FROM memories WHERE archived = 0
		 ORDER BY created_at DESC LIMIT 300`,
	)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var c, s string
		if rows.Scan(&c, &s) != nil {
			continue
		}
		if want != "" && normalizeText(c) == want {
			return true
		}
		if wantSummary != "" && normalizeText(s) == wantSummary {
			return true
		}
	}
	return false
}

// normalizeText reduces a string to lowercase words separated by single spaces,
// so two phrasings that differ only in punctuation or whitespace compare equal.
func normalizeText(s string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}
