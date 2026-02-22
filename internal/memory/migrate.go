package memory

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/logger"
)

// MigrateFileMemoryToDB reads existing memory/memory.md and daily logs,
// then inserts them as memories in the database.
func MigrateFileMemoryToDB(mgr *Manager, slug string) error {
	db, err := mgr.GetDB(slug)
	if err != nil {
		return err
	}

	var memDir string
	if slug == "gateway" {
		memDir = filepath.Join(mgr.dataDir, "gateway")
	} else {
		memDir = filepath.Join(mgr.dataDir, "agents", slug)
	}

	migrated := 0

	// Migrate memory/memory.md
	memPath := filepath.Join(memDir, "memory", "memory.md")
	if content, err := os.ReadFile(memPath); err == nil {
		lines := parseMemoryLines(string(content))
		for _, line := range lines {
			if line == "" {
				continue
			}
			id := uuid.New().String()
			now := time.Now().UTC().Format("2006-01-02 15:04:05")
			summary := firstLine(line, 100)
			_, err := db.Exec(
				`INSERT INTO memories (id, content, summary, category, importance, source, tags, created_at, updated_at)
				 VALUES (?, ?, ?, 'general', 5, 'migration', '', ?, ?)`,
				id, line, summary, now, now,
			)
			if err == nil {
				migrated++
			}
		}
	}

	// Migrate daily logs (memory/YYYY-MM-DD.md)
	dailyDir := filepath.Join(memDir, "memory")
	entries, err := os.ReadDir(dailyDir)
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".md") || name == "memory.md" {
				continue
			}
			// Parse date from filename
			datePart := strings.TrimSuffix(name, ".md")
			logDate, err := time.Parse("2006-01-02", datePart)
			if err != nil {
				continue
			}

			content, err := os.ReadFile(filepath.Join(dailyDir, name))
			if err != nil || strings.TrimSpace(string(content)) == "" {
				continue
			}

			lines := parseMemoryLines(string(content))
			for _, line := range lines {
				if line == "" {
					continue
				}
				id := uuid.New().String()
				createdAt := logDate.Format("2006-01-02 15:04:05")
				summary := firstLine(line, 100)
				_, err := db.Exec(
					`INSERT INTO memories (id, content, summary, category, importance, source, tags, created_at, updated_at)
					 VALUES (?, ?, ?, 'daily_log', 3, 'migration', ?, ?, ?)`,
					id, line, summary, "date:"+datePart, createdAt, createdAt,
				)
				if err == nil {
					migrated++
				}
			}
		}
	}

	if migrated > 0 {
		logger.Success("Migrated %d memories from files to DB for %s", migrated, slug)
	}

	return nil
}

// parseMemoryLines extracts individual memory items from markdown content.
// Handles bullet points, numbered lists, and bare paragraphs.
func parseMemoryLines(content string) []string {
	var results []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip headings
		if strings.HasPrefix(line, "#") {
			continue
		}
		// Strip bullet/number prefix
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "+ ")
		// Handle numbered lists
		for i := 0; i <= 9; i++ {
			line = strings.TrimPrefix(line, string(rune('0'+i))+". ")
		}
		// Strip timestamp prefixes like [2025-01-15 14:30]
		if strings.HasPrefix(line, "[") {
			if idx := strings.Index(line, "]"); idx > 0 && idx < 25 {
				line = strings.TrimSpace(line[idx+1:])
			}
		}

		line = strings.TrimSpace(line)
		if line != "" {
			results = append(results, line)
		}
	}

	return results
}
