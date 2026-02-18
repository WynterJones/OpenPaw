package memory

import (
	"fmt"
	"strings"
	"time"
)

// BuildMemoryPromptSection generates the Tier 1 boot summary for an agent's system prompt.
func (m *Manager) BuildMemoryPromptSection(slug string) string {
	db, err := m.GetDB(slug)
	if err != nil {
		return ""
	}

	var total, archived int
	db.QueryRow("SELECT COUNT(*) FROM memories WHERE archived = 0").Scan(&total)
	db.QueryRow("SELECT COUNT(*) FROM memories WHERE archived = 1").Scan(&archived)

	if total == 0 && archived == 0 {
		return buildEmptyMemorySection()
	}

	var sb strings.Builder
	sb.WriteString("## MEMORY SYSTEM\n\n")
	sb.WriteString(fmt.Sprintf("You have **%d** memories", total))
	if archived > 0 {
		sb.WriteString(fmt.Sprintf(" (%d archived)", archived))
	}

	// Category breakdown
	rows, err := db.Query("SELECT category, COUNT(*) FROM memories WHERE archived = 0 GROUP BY category ORDER BY COUNT(*) DESC")
	if err == nil {
		defer rows.Close()
		var cats []string
		for rows.Next() {
			var cat string
			var cnt int
			rows.Scan(&cat, &cnt)
			cats = append(cats, fmt.Sprintf("%s: %d", cat, cnt))
		}
		if len(cats) > 0 {
			sb.WriteString(fmt.Sprintf(" across %d categories (%s)", len(cats), strings.Join(cats, ", ")))
		}
	}
	sb.WriteString(".\n\n")

	// Key memories (importance >= 8)
	keyRows, err := db.Query(
		`SELECT summary, content, category FROM memories
		 WHERE archived = 0 AND importance >= 8
		 ORDER BY importance DESC, created_at DESC LIMIT 10`,
	)
	if err == nil {
		defer keyRows.Close()
		var keyMemories []string
		for keyRows.Next() {
			var summary, content, category string
			keyRows.Scan(&summary, &content, &category)
			display := summary
			if display == "" {
				// Use first line of content, truncated
				display = firstLine(content, 100)
			}
			keyMemories = append(keyMemories, fmt.Sprintf("- [%s] %s", category, display))
		}
		if len(keyMemories) > 0 {
			sb.WriteString("### Key Memories\n")
			sb.WriteString(strings.Join(keyMemories, "\n"))
			sb.WriteString("\n\n")
		}
	}

	// Today's memories
	today := time.Now().Format("2006-01-02")
	todayRows, err := db.Query(
		`SELECT summary, content FROM memories
		 WHERE archived = 0 AND DATE(created_at) = ?
		 ORDER BY created_at DESC LIMIT 5`,
		today,
	)
	if err == nil {
		defer todayRows.Close()
		var todayMems []string
		for todayRows.Next() {
			var summary, content string
			todayRows.Scan(&summary, &content)
			display := summary
			if display == "" {
				display = firstLine(content, 80)
			}
			todayMems = append(todayMems, "- "+display)
		}
		if len(todayMems) > 0 {
			sb.WriteString("### Today\n")
			sb.WriteString(strings.Join(todayMems, "\n"))
			sb.WriteString("\n\n")
		}
	}

	// Recent memories (last 3 days, excluding today, importance >= 5)
	recentRows, err := db.Query(
		`SELECT summary, content FROM memories
		 WHERE archived = 0 AND DATE(created_at) < ? AND DATE(created_at) >= DATE(?, '-3 days')
		   AND importance >= 5
		 ORDER BY created_at DESC LIMIT 5`,
		today, today,
	)
	if err == nil {
		defer recentRows.Close()
		var recentMems []string
		for recentRows.Next() {
			var summary, content string
			recentRows.Scan(&summary, &content)
			display := summary
			if display == "" {
				display = firstLine(content, 80)
			}
			recentMems = append(recentMems, "- "+display)
		}
		if len(recentMems) > 0 {
			sb.WriteString("### Recent\n")
			sb.WriteString(strings.Join(recentMems, "\n"))
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString("Use `memory_save`, `memory_search`, `memory_list`, `memory_update`, `memory_forget`, `memory_stats` to interact with your full memory database.\n")
	sb.WriteString("**Search before assuming you don't know something.**\n")

	return sb.String()
}

func buildEmptyMemorySection() string {
	var sb strings.Builder
	sb.WriteString("## MEMORY SYSTEM\n\n")
	sb.WriteString("Your memory database is empty. Save important information using `memory_save` to remember it across conversations.\n\n")
	sb.WriteString("Available tools: `memory_save`, `memory_search`, `memory_list`, `memory_update`, `memory_forget`, `memory_stats`\n")
	return sb.String()
}

func firstLine(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
