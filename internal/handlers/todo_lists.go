package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
)

type TodoListsHandler struct {
	db *database.DB
}

func NewTodoListsHandler(db *database.DB) *TodoListsHandler {
	return &TodoListsHandler{db: db}
}

// ListLists returns all todo lists with item counts.
func (h *TodoListsHandler) ListLists(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT tl.*,
			(SELECT COUNT(*) FROM todo_items WHERE list_id = tl.id) as total_items,
			(SELECT COUNT(*) FROM todo_items WHERE list_id = tl.id AND completed = 1) as completed_items
		FROM todo_lists tl ORDER BY tl.sort_order ASC, tl.created_at ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list todo lists")
		return
	}
	defer rows.Close()

	type listRow struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Description    string `json:"description"`
		Color          string `json:"color"`
		SortOrder      int    `json:"sort_order"`
		CreatedAt      string `json:"created_at"`
		UpdatedAt      string `json:"updated_at"`
		TotalItems     int    `json:"total_items"`
		CompletedItems int    `json:"completed_items"`
	}

	lists := []listRow{}
	for rows.Next() {
		var l listRow
		var createdAt, updatedAt time.Time
		if rows.Scan(&l.ID, &l.Name, &l.Description, &l.Color, &l.SortOrder, &createdAt, &updatedAt, &l.TotalItems, &l.CompletedItems) != nil {
			continue
		}
		l.CreatedAt = createdAt.Format(time.RFC3339)
		l.UpdatedAt = updatedAt.Format(time.RFC3339)
		lists = append(lists, l)
	}
	writeJSON(w, http.StatusOK, lists)
}

// Summary returns a lightweight list of todo lists with counts.
func (h *TodoListsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT tl.id, tl.name, tl.color,
			(SELECT COUNT(*) FROM todo_items WHERE list_id = tl.id) as total_items,
			(SELECT COUNT(*) FROM todo_items WHERE list_id = tl.id AND completed = 1) as completed_items
		FROM todo_lists tl ORDER BY tl.sort_order ASC, tl.created_at ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get todo list summary")
		return
	}
	defer rows.Close()

	type summaryRow struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Color          string `json:"color"`
		TotalItems     int    `json:"total_items"`
		CompletedItems int    `json:"completed_items"`
	}

	lists := []summaryRow{}
	for rows.Next() {
		var s summaryRow
		if rows.Scan(&s.ID, &s.Name, &s.Color, &s.TotalItems, &s.CompletedItems) != nil {
			continue
		}
		lists = append(lists, s)
	}
	writeJSON(w, http.StatusOK, lists)
}

// CreateList creates a new todo list.
func (h *TodoListsHandler) CreateList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	var maxOrder int
	h.db.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM todo_lists").Scan(&maxOrder)

	_, err := h.db.Exec(
		"INSERT INTO todo_lists (id, name, description, color, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, req.Name, req.Description, req.Color, maxOrder+1, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create todo list")
		return
	}

	h.db.LogAudit("system", "todo_list_created", "todo", "todo_list", id, "name="+req.Name)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":              id,
		"name":            req.Name,
		"description":     req.Description,
		"color":           req.Color,
		"sort_order":      maxOrder + 1,
		"total_items":     0,
		"completed_items": 0,
		"created_at":      now.Format(time.RFC3339),
		"updated_at":      now.Format(time.RFC3339),
	})
}

// GetList returns a single todo list with all its items.
func (h *TodoListsHandler) GetList(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")

	// Fetch the list
	var l struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
		Color       string    `json:"color"`
		SortOrder   int       `json:"sort_order"`
		CreatedAt   time.Time `json:"-"`
		UpdatedAt   time.Time `json:"-"`
	}
	err := h.db.QueryRow(
		"SELECT id, name, description, color, sort_order, created_at, updated_at FROM todo_lists WHERE id = ?",
		listID,
	).Scan(&l.ID, &l.Name, &l.Description, &l.Color, &l.SortOrder, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "todo list not found")
		return
	}

	// Fetch items with agent info
	items := h.fetchItems(listID, nil)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"list": map[string]interface{}{
			"id":          l.ID,
			"name":        l.Name,
			"description": l.Description,
			"color":       l.Color,
			"sort_order":  l.SortOrder,
			"created_at":  l.CreatedAt.Format(time.RFC3339),
			"updated_at":  l.UpdatedAt.Format(time.RFC3339),
		},
		"items": items,
	})
}

// UpdateList partially updates a todo list.
func (h *TodoListsHandler) UpdateList(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Color       *string `json:"color"`
		SortOrder   *int    `json:"sort_order"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()

	if req.Name != nil {
		h.db.Exec("UPDATE todo_lists SET name = ?, updated_at = ? WHERE id = ?", *req.Name, now, listID)
	}
	if req.Description != nil {
		h.db.Exec("UPDATE todo_lists SET description = ?, updated_at = ? WHERE id = ?", *req.Description, now, listID)
	}
	if req.Color != nil {
		h.db.Exec("UPDATE todo_lists SET color = ?, updated_at = ? WHERE id = ?", *req.Color, now, listID)
	}
	if req.SortOrder != nil {
		h.db.Exec("UPDATE todo_lists SET sort_order = ?, updated_at = ? WHERE id = ?", *req.SortOrder, now, listID)
	}

	// Re-read
	var l struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
		Color       string    `json:"color"`
		SortOrder   int       `json:"sort_order"`
		CreatedAt   time.Time `json:"-"`
		UpdatedAt   time.Time `json:"-"`
	}
	err := h.db.QueryRow(
		"SELECT id, name, description, color, sort_order, created_at, updated_at FROM todo_lists WHERE id = ?",
		listID,
	).Scan(&l.ID, &l.Name, &l.Description, &l.Color, &l.SortOrder, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "todo list not found")
		return
	}

	h.db.LogAudit("system", "todo_list_updated", "todo", "todo_list", listID, "name="+l.Name)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":          l.ID,
		"name":        l.Name,
		"description": l.Description,
		"color":       l.Color,
		"sort_order":  l.SortOrder,
		"created_at":  l.CreatedAt.Format(time.RFC3339),
		"updated_at":  l.UpdatedAt.Format(time.RFC3339),
	})
}

// DeleteList deletes a todo list and cascades to its items.
func (h *TodoListsHandler) DeleteList(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")

	result, err := h.db.Exec("DELETE FROM todo_lists WHERE id = ?", listID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete todo list")
		return
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "todo list not found")
		return
	}

	h.db.LogAudit("system", "todo_list_deleted", "todo", "todo_list", listID, "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListItems returns items in a todo list, optionally filtered by completed status.
func (h *TodoListsHandler) ListItems(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")

	var completedFilter *bool
	if v := r.URL.Query().Get("completed"); v != "" {
		b := v == "true"
		completedFilter = &b
	}

	items := h.fetchItems(listID, completedFilter)
	writeJSON(w, http.StatusOK, items)
}

// CreateItem creates a new item in a todo list.
func (h *TodoListsHandler) CreateItem(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")

	var req struct {
		Title     string `json:"title"`
		Notes     string `json:"notes"`
		DueDate   string `json:"due_date"`
		AgentSlug string `json:"agent_slug"`
		AgentNote string `json:"agent_note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	// Verify list exists
	var exists int
	if h.db.QueryRow("SELECT 1 FROM todo_lists WHERE id = ?", listID).Scan(&exists) != nil {
		writeError(w, http.StatusNotFound, "todo list not found")
		return
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	var maxOrder int
	h.db.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM todo_items WHERE list_id = ?", listID).Scan(&maxOrder)

	var dueDate sql.NullString
	if req.DueDate != "" {
		dueDate = sql.NullString{String: req.DueDate, Valid: true}
	}

	var agentSlug sql.NullString
	if req.AgentSlug != "" {
		agentSlug = sql.NullString{String: req.AgentSlug, Valid: true}
	}

	_, err := h.db.Exec(
		`INSERT INTO todo_items (id, list_id, title, notes, completed, sort_order, due_date, last_actor_agent_slug, last_actor_note, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?)`,
		id, listID, req.Title, req.Notes, maxOrder+1, dueDate, agentSlug, req.AgentNote, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create todo item")
		return
	}

	h.db.LogAudit("system", "todo_item_created", "todo", "todo_list", listID, "title="+req.Title)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                    id,
		"list_id":               listID,
		"title":                 req.Title,
		"notes":                 req.Notes,
		"completed":             false,
		"sort_order":            maxOrder + 1,
		"due_date":              req.DueDate,
		"last_actor_agent_slug": req.AgentSlug,
		"last_actor_note":       req.AgentNote,
		"created_at":            now.Format(time.RFC3339),
		"updated_at":            now.Format(time.RFC3339),
		"completed_at":          nil,
	})
}

// UpdateItem partially updates a todo item.
func (h *TodoListsHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "itemId")

	var req struct {
		Title     *string `json:"title"`
		Notes     *string `json:"notes"`
		DueDate   *string `json:"due_date"`
		AgentSlug *string `json:"agent_slug"`
		AgentNote *string `json:"agent_note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()

	if req.Title != nil {
		h.db.Exec("UPDATE todo_items SET title = ?, updated_at = ? WHERE id = ? AND list_id = ?", *req.Title, now, itemID, listID)
	}
	if req.Notes != nil {
		h.db.Exec("UPDATE todo_items SET notes = ?, updated_at = ? WHERE id = ? AND list_id = ?", *req.Notes, now, itemID, listID)
	}
	if req.DueDate != nil {
		if *req.DueDate == "" {
			h.db.Exec("UPDATE todo_items SET due_date = NULL, updated_at = ? WHERE id = ? AND list_id = ?", now, itemID, listID)
		} else {
			h.db.Exec("UPDATE todo_items SET due_date = ?, updated_at = ? WHERE id = ? AND list_id = ?", *req.DueDate, now, itemID, listID)
		}
	}
	if req.AgentSlug != nil {
		if *req.AgentSlug == "" {
			h.db.Exec("UPDATE todo_items SET last_actor_agent_slug = NULL, updated_at = ? WHERE id = ? AND list_id = ?", now, itemID, listID)
		} else {
			h.db.Exec("UPDATE todo_items SET last_actor_agent_slug = ?, updated_at = ? WHERE id = ? AND list_id = ?", *req.AgentSlug, now, itemID, listID)
		}
	}
	if req.AgentNote != nil {
		h.db.Exec("UPDATE todo_items SET last_actor_note = ?, updated_at = ? WHERE id = ? AND list_id = ?", *req.AgentNote, now, itemID, listID)
	}

	h.db.LogAudit("system", "todo_item_updated", "todo", "todo_list", listID, "item="+itemID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ToggleItem toggles the completed status of a todo item.
func (h *TodoListsHandler) ToggleItem(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "itemId")

	var req struct {
		AgentSlug string `json:"agent_slug"`
		AgentNote string `json:"agent_note"`
	}
	// Body is optional for toggle
	decodeJSON(r, &req)

	// Read current completed state
	var completed int
	err := h.db.QueryRow("SELECT completed FROM todo_items WHERE id = ? AND list_id = ?", itemID, listID).Scan(&completed)
	if err != nil {
		writeError(w, http.StatusNotFound, "todo item not found")
		return
	}

	now := time.Now().UTC()
	newCompleted := 0
	var completedAt sql.NullTime
	if completed == 0 {
		newCompleted = 1
		completedAt = sql.NullTime{Time: now, Valid: true}
	}

	agentSlug := sql.NullString{}
	if req.AgentSlug != "" {
		agentSlug = sql.NullString{String: req.AgentSlug, Valid: true}
	}

	_, err = h.db.Exec(
		"UPDATE todo_items SET completed = ?, completed_at = ?, last_actor_agent_slug = ?, last_actor_note = ?, updated_at = ? WHERE id = ? AND list_id = ?",
		newCompleted, completedAt, agentSlug, req.AgentNote, now, itemID, listID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to toggle todo item")
		return
	}

	action := "todo_item_uncompleted"
	if newCompleted == 1 {
		action = "todo_item_completed"
	}
	h.db.LogAudit("system", action, "todo", "todo_list", listID, "item="+itemID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":        itemID,
		"completed": newCompleted == 1,
	})
}

// DeleteItem deletes a todo item.
func (h *TodoListsHandler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "itemId")

	result, err := h.db.Exec("DELETE FROM todo_items WHERE id = ? AND list_id = ?", itemID, listID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete todo item")
		return
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "todo item not found")
		return
	}

	h.db.LogAudit("system", "todo_item_deleted", "todo", "todo_list", listID, "item="+itemID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ReorderItems reorders items in a todo list.
func (h *TodoListsHandler) ReorderItems(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")

	var req struct {
		Items []struct {
			ID        string `json:"id"`
			SortOrder int    `json:"sort_order"`
		} `json:"items"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()
	for _, item := range req.Items {
		h.db.Exec(
			"UPDATE todo_items SET sort_order = ?, updated_at = ? WHERE id = ? AND list_id = ?",
			item.SortOrder, now, item.ID, listID,
		)
	}

	h.db.LogAudit("system", "todo_items_reordered", "todo", "todo_list", listID, "")

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// fetchItems is a helper that fetches items for a list with optional completed filter.
func (h *TodoListsHandler) fetchItems(listID string, completedFilter *bool) []map[string]interface{} {
	query := `
		SELECT ti.id, ti.list_id, ti.title, ti.notes, ti.completed, ti.sort_order,
		       ti.due_date, ti.last_actor_agent_slug, ti.last_actor_note,
		       ti.created_at, ti.updated_at, ti.completed_at,
		       ar.name as agent_name, ar.avatar_path as agent_avatar
		FROM todo_items ti
		LEFT JOIN agent_roles ar ON ar.slug = ti.last_actor_agent_slug
		WHERE ti.list_id = ?`

	args := []interface{}{listID}

	if completedFilter != nil {
		if *completedFilter {
			query += " AND ti.completed = 1"
		} else {
			query += " AND ti.completed = 0"
		}
	}

	query += " ORDER BY ti.completed ASC, ti.sort_order ASC, ti.created_at ASC"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return []map[string]interface{}{}
	}
	defer rows.Close()

	items := []map[string]interface{}{}
	for rows.Next() {
		var id, listIDVal, title, notes, lastActorNote string
		var completed, sortOrder int
		var dueDate, agentSlug, agentName, agentAvatar sql.NullString
		var createdAt, updatedAt time.Time
		var completedAt sql.NullTime

		if rows.Scan(
			&id, &listIDVal, &title, &notes, &completed, &sortOrder,
			&dueDate, &agentSlug, &lastActorNote,
			&createdAt, &updatedAt, &completedAt,
			&agentName, &agentAvatar,
		) != nil {
			continue
		}

		item := map[string]interface{}{
			"id":                    id,
			"list_id":               listIDVal,
			"title":                 title,
			"notes":                 notes,
			"completed":             completed == 1,
			"sort_order":            sortOrder,
			"due_date":              nullStr(dueDate),
			"last_actor_agent_slug": nullStr(agentSlug),
			"last_actor_note":       lastActorNote,
			"created_at":            createdAt.Format(time.RFC3339),
			"updated_at":            updatedAt.Format(time.RFC3339),
			"completed_at":          nullTime(completedAt),
			"agent_name":            nullStr(agentName),
			"agent_avatar":          nullStr(agentAvatar),
		}

		items = append(items, item)
	}
	return items
}

// nullStr converts a sql.NullString to a value suitable for JSON (string or nil).
func nullStr(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}

// nullTime converts a sql.NullTime to a value suitable for JSON (RFC3339 string or nil).
func nullTime(nt sql.NullTime) interface{} {
	if nt.Valid {
		return nt.Time.Format(time.RFC3339)
	}
	return nil
}
