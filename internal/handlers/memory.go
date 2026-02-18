package handlers

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/memory"
)

type MemoryHandler struct {
	memoryMgr *memory.Manager
}

func NewMemoryHandler(memoryMgr *memory.Manager) *MemoryHandler {
	return &MemoryHandler{memoryMgr: memoryMgr}
}

type memoryItem struct {
	ID          string `json:"id"`
	Content     string `json:"content"`
	Summary     string `json:"summary"`
	Category    string `json:"category"`
	Importance  int    `json:"importance"`
	Source      string `json:"source"`
	Tags        string `json:"tags"`
	AccessCount int    `json:"access_count"`
	Archived    bool   `json:"archived"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func (h *MemoryHandler) listMemoriesForSlug(w http.ResponseWriter, r *http.Request, slug string) {
	db, err := h.memoryMgr.GetDB(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to open memory DB: "+err.Error())
		return
	}

	category := r.URL.Query().Get("category")
	query := `SELECT id, content, summary, category, importance, source, tags,
	          access_count, archived, created_at, updated_at
	          FROM memories WHERE 1=1`
	var args []interface{}

	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}
	if r.URL.Query().Get("archived") != "true" {
		query += " AND archived = 0"
	}

	query += " ORDER BY created_at DESC LIMIT 100"

	rows, err := db.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query failed: "+err.Error())
		return
	}
	defer rows.Close()

	var memories []memoryItem
	for rows.Next() {
		var m memoryItem
		if err := rows.Scan(&m.ID, &m.Content, &m.Summary, &m.Category, &m.Importance,
			&m.Source, &m.Tags, &m.AccessCount, &m.Archived, &m.CreatedAt, &m.UpdatedAt); err != nil {
			log.Printf("[warn] scan memory row: %v", err)
			continue
		}
		memories = append(memories, m)
	}

	if memories == nil {
		memories = []memoryItem{}
	}

	writeJSON(w, http.StatusOK, memories)
}

func (h *MemoryHandler) statsForSlug(w http.ResponseWriter, r *http.Request, slug string) {
	db, err := h.memoryMgr.GetDB(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to open memory DB: "+err.Error())
		return
	}

	var total, archived int
	db.QueryRow("SELECT COUNT(*) FROM memories WHERE archived = 0").Scan(&total)
	db.QueryRow("SELECT COUNT(*) FROM memories WHERE archived = 1").Scan(&archived)

	catRows, err := db.Query("SELECT category, COUNT(*) FROM memories WHERE archived = 0 GROUP BY category ORDER BY COUNT(*) DESC")
	categories := map[string]int{}
	if err == nil {
		defer catRows.Close()
		for catRows.Next() {
			var cat string
			var cnt int
			catRows.Scan(&cat, &cnt)
			categories[cat] = cnt
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_active":   total,
		"total_archived": archived,
		"categories":     categories,
	})
}

// Agent memory endpoints

func (h *MemoryHandler) ListMemories(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "slug is required")
		return
	}
	h.listMemoriesForSlug(w, r, slug)
}

func (h *MemoryHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "slug is required")
		return
	}
	h.statsForSlug(w, r, slug)
}

func (h *MemoryHandler) DeleteMemory(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	memoryID := chi.URLParam(r, "memoryId")
	if slug == "" || memoryID == "" {
		writeError(w, http.StatusBadRequest, "slug and memoryId are required")
		return
	}

	db, err := h.memoryMgr.GetDB(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to open memory DB: "+err.Error())
		return
	}

	result, err := db.Exec("DELETE FROM memories WHERE id = ?", memoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Delete failed: "+err.Error())
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "Memory not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

// Gateway memory endpoints

func (h *MemoryHandler) ListGatewayMemories(w http.ResponseWriter, r *http.Request) {
	h.listMemoriesForSlug(w, r, "gateway")
}

func (h *MemoryHandler) GetGatewayStats(w http.ResponseWriter, r *http.Request) {
	h.statsForSlug(w, r, "gateway")
}
