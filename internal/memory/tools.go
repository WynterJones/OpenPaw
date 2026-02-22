package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	llm "github.com/openpaw/openpaw/internal/llm"
)

func BuildMemoryToolDefs() []llm.ToolDef {
	return []llm.ToolDef{
		buildMemorySaveDef(),
		buildMemorySearchDef(),
		buildMemoryListDef(),
		buildMemoryUpdateDef(),
		buildMemoryForgetDef(),
		buildMemoryStatsDef(),
	}
}

func (m *Manager) MakeMemoryHandlers(slug string) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"memory_save":   m.handleSave(slug),
		"memory_search": m.handleSearch(slug),
		"memory_list":   m.handleList(slug),
		"memory_update": m.handleUpdate(slug),
		"memory_forget": m.handleForget(slug),
		"memory_stats":  m.handleStats(slug),
	}
}

// --- Tool Definitions ---

func buildMemorySaveDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The memory content to save",
			},
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "A one-line summary of the memory (used in boot summaries)",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Category for organization (e.g. general, preference, fact, task, project)",
				"default":     "general",
			},
			"importance": map[string]interface{}{
				"type":        "integer",
				"description": "Importance level 1-10 (10 = critical, always show at boot; 1 = trivial)",
				"default":     5,
				"minimum":     1,
				"maximum":     10,
			},
			"tags": map[string]interface{}{
				"type":        "string",
				"description": "Comma-separated tags for filtering",
			},
		},
		"required": []string{"content"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "memory_save",
			Description: "Save a new memory to your persistent memory database. Use this to remember important information about the user, their preferences, project details, decisions, and anything you should recall in future conversations.",
			Parameters:  params,
		},
	}
}

func buildMemorySearchDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Full-text search query (supports natural language)",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Filter by category",
			},
			"min_importance": map[string]interface{}{
				"type":        "integer",
				"description": "Minimum importance level (1-10)",
			},
			"tags": map[string]interface{}{
				"type":        "string",
				"description": "Filter by tag (comma-separated, matches any)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max results to return (default 20, max 100)",
				"default":     20,
			},
			"include_archived": map[string]interface{}{
				"type":        "boolean",
				"description": "Include archived memories (default false)",
			},
		},
		"required": []string{"query"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "memory_search",
			Description: "Search your memory database using full-text search. Results are ranked by relevance. Search before assuming you don't know something.",
			Parameters:  params,
		},
	}
}

func buildMemoryListDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Filter by category",
			},
			"min_importance": map[string]interface{}{
				"type":        "integer",
				"description": "Minimum importance level",
			},
			"sort": map[string]interface{}{
				"type":        "string",
				"description": "Sort by: created, importance, accessed, updated",
				"default":     "created",
				"enum":        []string{"created", "importance", "accessed", "updated"},
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max results (default 20, max 100)",
				"default":     20,
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "Offset for pagination",
				"default":     0,
			},
			"include_archived": map[string]interface{}{
				"type":        "boolean",
				"description": "Include archived memories",
			},
		},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "memory_list",
			Description: "Browse your memories with sorting and filtering. Use for reviewing memories by category or importance.",
			Parameters:  params,
		},
	}
}

func buildMemoryUpdateDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The memory ID to update",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "New content (replaces existing)",
			},
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "New summary",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "New category",
			},
			"importance": map[string]interface{}{
				"type":        "integer",
				"description": "New importance (1-10)",
			},
			"tags": map[string]interface{}{
				"type":        "string",
				"description": "New tags (comma-separated)",
			},
			"archived": map[string]interface{}{
				"type":        "boolean",
				"description": "Set archived status",
			},
		},
		"required": []string{"id"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "memory_update",
			Description: "Update an existing memory's content, importance, category, tags, or archived status.",
			Parameters:  params,
		},
	}
}

func buildMemoryForgetDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The memory ID to forget",
			},
			"archive_only": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, archive instead of permanently deleting (default false)",
			},
		},
		"required": []string{"id"},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "memory_forget",
			Description: "Delete or archive a memory. Use archive_only=true to keep it searchable but hidden from default views.",
			Parameters:  params,
		},
	}
}

func buildMemoryStatsDef() llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	})
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "memory_stats",
			Description: "Get statistics about your memory database: total count, counts by category, top memories, and collection list.",
			Parameters:  params,
		},
	}
}

// --- Tool Handlers ---

func (m *Manager) handleSave(slug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			Content    string `json:"content"`
			Summary    string `json:"summary"`
			Category   string `json:"category"`
			Importance int    `json:"importance"`
			Tags       string `json:"tags"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.Content == "" {
			return llm.ToolResult{Output: "content is required", IsError: true}
		}
		if params.Category == "" {
			params.Category = "general"
		}
		if params.Importance <= 0 {
			params.Importance = 5
		}
		if params.Importance > 10 {
			params.Importance = 10
		}

		db, err := m.GetDB(slug)
		if err != nil {
			return llm.ToolResult{Output: "Memory DB error: " + err.Error(), IsError: true}
		}

		id := uuid.New().String()
		now := time.Now().UTC().Format("2006-01-02 15:04:05")
		_, err = db.Exec(
			`INSERT INTO memories (id, content, summary, category, importance, source, tags, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, 'agent', ?, ?, ?)`,
			id, params.Content, params.Summary, params.Category, params.Importance, params.Tags, now, now,
		)
		if err != nil {
			return llm.ToolResult{Output: "Failed to save memory: " + err.Error(), IsError: true}
		}

		result, _ := json.Marshal(map[string]interface{}{
			"id":         id,
			"saved":      true,
			"category":   params.Category,
			"importance": params.Importance,
		})
		return llm.ToolResult{Output: string(result)}
	}
}

func (m *Manager) handleSearch(slug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			Query           string `json:"query"`
			Category        string `json:"category"`
			MinImportance   int    `json:"min_importance"`
			Tags            string `json:"tags"`
			Limit           int    `json:"limit"`
			IncludeArchived bool   `json:"include_archived"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.Query == "" {
			return llm.ToolResult{Output: "query is required", IsError: true}
		}
		if params.Limit <= 0 {
			params.Limit = 20
		}
		if params.Limit > 100 {
			params.Limit = 100
		}

		db, err := m.GetDB(slug)
		if err != nil {
			return llm.ToolResult{Output: "Memory DB error: " + err.Error(), IsError: true}
		}

		var query string
		var args []interface{}

		if HasFTS() {
			// FTS5 full-text search
			query = `
				SELECT m.id, m.content, m.summary, m.category, m.importance, m.tags,
				       m.access_count, m.created_at, m.updated_at, m.archived,
				       bm25(memories_fts) as rank
				FROM memories_fts fts
				JOIN memories m ON m.rowid = fts.rowid
				WHERE memories_fts MATCH ?`
			args = []interface{}{params.Query}

			if !params.IncludeArchived {
				query += " AND m.archived = 0"
			}
			if params.Category != "" {
				query += " AND m.category = ?"
				args = append(args, params.Category)
			}
			if params.MinImportance > 0 {
				query += " AND m.importance >= ?"
				args = append(args, params.MinImportance)
			}
			if params.Tags != "" {
				for _, tag := range strings.Split(params.Tags, ",") {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						query += " AND m.tags LIKE ?"
						args = append(args, "%"+tag+"%")
					}
				}
			}
			query += " ORDER BY rank LIMIT ?"
			args = append(args, params.Limit)
		} else {
			// Fallback: LIKE-based search when FTS5 is not available
			likePattern := "%" + params.Query + "%"
			query = `
				SELECT id, content, summary, category, importance, tags,
				       access_count, created_at, updated_at, archived,
				       0 as rank
				FROM memories
				WHERE (content LIKE ? OR summary LIKE ? OR tags LIKE ?)`
			args = []interface{}{likePattern, likePattern, likePattern}

			if !params.IncludeArchived {
				query += " AND archived = 0"
			}
			if params.Category != "" {
				query += " AND category = ?"
				args = append(args, params.Category)
			}
			if params.MinImportance > 0 {
				query += " AND importance >= ?"
				args = append(args, params.MinImportance)
			}
			if params.Tags != "" {
				for _, tag := range strings.Split(params.Tags, ",") {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						query += " AND tags LIKE ?"
						args = append(args, "%"+tag+"%")
					}
				}
			}
			query += " ORDER BY importance DESC, created_at DESC LIMIT ?"
			args = append(args, params.Limit)
		}

		rows, err := db.Query(query, args...)
		if err != nil {
			return llm.ToolResult{Output: "Search failed: " + err.Error(), IsError: true}
		}
		defer rows.Close()

		var memories []map[string]interface{}
		var ids []string
		for rows.Next() {
			var id, content, summary, category, tags, createdAt, updatedAt string
			var importance, accessCount int
			var archived bool
			var rank float64
			if err := rows.Scan(&id, &content, &summary, &category, &importance, &tags,
				&accessCount, &createdAt, &updatedAt, &archived, &rank); err != nil {
				log.Printf("[warn] scan memory search row: %v", err)
				continue
			}
			memories = append(memories, map[string]interface{}{
				"id":           id,
				"content":      content,
				"summary":      summary,
				"category":     category,
				"importance":   importance,
				"tags":         tags,
				"access_count": accessCount,
				"created_at":   createdAt,
				"archived":     archived,
			})
			ids = append(ids, id)
		}

		// Update access counts
		for _, id := range ids {
			db.Exec("UPDATE memories SET access_count = access_count + 1, last_accessed_at = CURRENT_TIMESTAMP WHERE id = ?", id)
		}

		result, _ := json.Marshal(map[string]interface{}{
			"query":   params.Query,
			"count":   len(memories),
			"results": memories,
		})
		return llm.ToolResult{Output: string(result)}
	}
}

func (m *Manager) handleList(slug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			Category        string `json:"category"`
			MinImportance   int    `json:"min_importance"`
			Sort            string `json:"sort"`
			Limit           int    `json:"limit"`
			Offset          int    `json:"offset"`
			IncludeArchived bool   `json:"include_archived"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.Limit <= 0 {
			params.Limit = 20
		}
		if params.Limit > 100 {
			params.Limit = 100
		}

		db, err := m.GetDB(slug)
		if err != nil {
			return llm.ToolResult{Output: "Memory DB error: " + err.Error(), IsError: true}
		}

		query := `SELECT id, content, summary, category, importance, tags,
		          access_count, created_at, updated_at, archived FROM memories WHERE 1=1`
		var args []interface{}

		if !params.IncludeArchived {
			query += " AND archived = 0"
		}
		if params.Category != "" {
			query += " AND category = ?"
			args = append(args, params.Category)
		}
		if params.MinImportance > 0 {
			query += " AND importance >= ?"
			args = append(args, params.MinImportance)
		}

		switch params.Sort {
		case "importance":
			query += " ORDER BY importance DESC, created_at DESC"
		case "accessed":
			query += " ORDER BY last_accessed_at DESC NULLS LAST"
		case "updated":
			query += " ORDER BY updated_at DESC"
		default:
			query += " ORDER BY created_at DESC"
		}

		query += " LIMIT ? OFFSET ?"
		args = append(args, params.Limit, params.Offset)

		rows, err := db.Query(query, args...)
		if err != nil {
			return llm.ToolResult{Output: "List failed: " + err.Error(), IsError: true}
		}
		defer rows.Close()

		var memories []map[string]interface{}
		for rows.Next() {
			var id, content, summary, category, tags, createdAt, updatedAt string
			var importance, accessCount int
			var archived bool
			if err := rows.Scan(&id, &content, &summary, &category, &importance, &tags,
				&accessCount, &createdAt, &updatedAt, &archived); err != nil {
				log.Printf("[warn] scan memory list row: %v", err)
				continue
			}
			memories = append(memories, map[string]interface{}{
				"id":           id,
				"content":      content,
				"summary":      summary,
				"category":     category,
				"importance":   importance,
				"tags":         tags,
				"access_count": accessCount,
				"created_at":   createdAt,
				"archived":     archived,
			})
		}

		// Get total count
		var total int
		countQuery := "SELECT COUNT(*) FROM memories WHERE 1=1"
		var countArgs []interface{}
		if !params.IncludeArchived {
			countQuery += " AND archived = 0"
		}
		if params.Category != "" {
			countQuery += " AND category = ?"
			countArgs = append(countArgs, params.Category)
		}
		if params.MinImportance > 0 {
			countQuery += " AND importance >= ?"
			countArgs = append(countArgs, params.MinImportance)
		}
		db.QueryRow(countQuery, countArgs...).Scan(&total)

		result, _ := json.Marshal(map[string]interface{}{
			"total":   total,
			"offset":  params.Offset,
			"limit":   params.Limit,
			"count":   len(memories),
			"results": memories,
		})
		return llm.ToolResult{Output: string(result)}
	}
}

func (m *Manager) handleUpdate(slug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			ID         string  `json:"id"`
			Content    *string `json:"content"`
			Summary    *string `json:"summary"`
			Category   *string `json:"category"`
			Importance *int    `json:"importance"`
			Tags       *string `json:"tags"`
			Archived   *bool   `json:"archived"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.ID == "" {
			return llm.ToolResult{Output: "id is required", IsError: true}
		}

		db, err := m.GetDB(slug)
		if err != nil {
			return llm.ToolResult{Output: "Memory DB error: " + err.Error(), IsError: true}
		}

		var sets []string
		var args []interface{}

		if params.Content != nil {
			sets = append(sets, "content = ?")
			args = append(args, *params.Content)
		}
		if params.Summary != nil {
			sets = append(sets, "summary = ?")
			args = append(args, *params.Summary)
		}
		if params.Category != nil {
			sets = append(sets, "category = ?")
			args = append(args, *params.Category)
		}
		if params.Importance != nil {
			imp := *params.Importance
			if imp < 1 {
				imp = 1
			}
			if imp > 10 {
				imp = 10
			}
			sets = append(sets, "importance = ?")
			args = append(args, imp)
		}
		if params.Tags != nil {
			sets = append(sets, "tags = ?")
			args = append(args, *params.Tags)
		}
		if params.Archived != nil {
			archived := 0
			if *params.Archived {
				archived = 1
			}
			sets = append(sets, "archived = ?")
			args = append(args, archived)
		}

		if len(sets) == 0 {
			return llm.ToolResult{Output: "No fields to update", IsError: true}
		}

		sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
		args = append(args, params.ID)

		query := fmt.Sprintf("UPDATE memories SET %s WHERE id = ?", strings.Join(sets, ", "))
		result, err := db.Exec(query, args...)
		if err != nil {
			return llm.ToolResult{Output: "Update failed: " + err.Error(), IsError: true}
		}

		affected, _ := result.RowsAffected()
		if affected == 0 {
			return llm.ToolResult{Output: "Memory not found: " + params.ID, IsError: true}
		}

		resp, _ := json.Marshal(map[string]interface{}{
			"id":      params.ID,
			"updated": true,
		})
		return llm.ToolResult{Output: string(resp)}
	}
}

func (m *Manager) handleForget(slug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			ID          string `json:"id"`
			ArchiveOnly bool   `json:"archive_only"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if params.ID == "" {
			return llm.ToolResult{Output: "id is required", IsError: true}
		}

		db, err := m.GetDB(slug)
		if err != nil {
			return llm.ToolResult{Output: "Memory DB error: " + err.Error(), IsError: true}
		}

		var result interface{}
		if params.ArchiveOnly {
			res, err := db.Exec("UPDATE memories SET archived = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?", params.ID)
			if err != nil {
				return llm.ToolResult{Output: "Archive failed: " + err.Error(), IsError: true}
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				return llm.ToolResult{Output: "Memory not found: " + params.ID, IsError: true}
			}
			result = map[string]interface{}{"id": params.ID, "archived": true}
		} else {
			res, err := db.Exec("DELETE FROM memories WHERE id = ?", params.ID)
			if err != nil {
				return llm.ToolResult{Output: "Delete failed: " + err.Error(), IsError: true}
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				return llm.ToolResult{Output: "Memory not found: " + params.ID, IsError: true}
			}
			result = map[string]interface{}{"id": params.ID, "deleted": true}
		}

		resp, _ := json.Marshal(result)
		return llm.ToolResult{Output: string(resp)}
	}
}

func (m *Manager) handleStats(slug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		db, err := m.GetDB(slug)
		if err != nil {
			return llm.ToolResult{Output: "Memory DB error: " + err.Error(), IsError: true}
		}

		stats := map[string]interface{}{}

		// Total counts
		var total, archived int
		db.QueryRow("SELECT COUNT(*) FROM memories WHERE archived = 0").Scan(&total)
		db.QueryRow("SELECT COUNT(*) FROM memories WHERE archived = 1").Scan(&archived)
		stats["total_active"] = total
		stats["total_archived"] = archived

		// By category
		rows, err := db.Query("SELECT category, COUNT(*) FROM memories WHERE archived = 0 GROUP BY category ORDER BY COUNT(*) DESC")
		if err == nil {
			defer rows.Close()
			categories := map[string]int{}
			for rows.Next() {
				var cat string
				var cnt int
				rows.Scan(&cat, &cnt)
				categories[cat] = cnt
			}
			stats["categories"] = categories
		}

		// Top memories by importance
		topRows, err := db.Query(
			"SELECT id, summary, category, importance FROM memories WHERE archived = 0 ORDER BY importance DESC LIMIT 5",
		)
		if err == nil {
			defer topRows.Close()
			var top []map[string]interface{}
			for topRows.Next() {
				var id, summary, cat string
				var imp int
				topRows.Scan(&id, &summary, &cat, &imp)
				display := summary
				if display == "" {
					display = "(no summary)"
				}
				top = append(top, map[string]interface{}{
					"id":         id,
					"summary":    display,
					"category":   cat,
					"importance": imp,
				})
			}
			stats["top_memories"] = top
		}

		// Collections
		collRows, err := db.Query("SELECT c.id, c.name, COUNT(mc.memory_id) FROM collections c LEFT JOIN memory_collections mc ON mc.collection_id = c.id GROUP BY c.id")
		if err == nil {
			defer collRows.Close()
			var collections []map[string]interface{}
			for collRows.Next() {
				var id, name string
				var cnt int
				collRows.Scan(&id, &name, &cnt)
				collections = append(collections, map[string]interface{}{
					"id":    id,
					"name":  name,
					"count": cnt,
				})
			}
			stats["collections"] = collections
		}

		result, _ := json.Marshal(stats)
		return llm.ToolResult{Output: string(result)}
	}
}
