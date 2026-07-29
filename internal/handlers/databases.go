package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/userdb"
)

type DatabasesHandler struct {
	db        *database.DB
	store     *userdb.Store
	broadcast func(string, interface{})
}

func NewDatabasesHandler(db *database.DB, broadcast func(string, interface{})) *DatabasesHandler {
	return &DatabasesHandler{db: db, store: userdb.NewStore(db), broadcast: broadcast}
}

func (h *DatabasesHandler) changed(databaseID, tableID string) {
	if h.broadcast != nil {
		h.broadcast("database_updated", map[string]string{
			"database_id": databaseID,
			"table_id":    tableID,
		})
	}
}

func writeDatabaseError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "database item not found")
		return
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "required"),
		strings.Contains(message, "cannot be empty"),
		strings.Contains(message, "already exists"),
		strings.Contains(message, "unsupported column"),
		strings.Contains(message, "unknown column"):
		writeError(w, http.StatusBadRequest, message)
	default:
		writeError(w, http.StatusInternalServerError, "database operation failed")
	}
}

func (h *DatabasesHandler) List(w http.ResponseWriter, _ *http.Request) {
	items, err := h.store.ListDatabases(activeWorkspaceID(h.db))
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *DatabasesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	item, err := h.store.CreateDatabase(activeWorkspaceID(h.db), req.Name, req.Description)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	h.db.LogAudit(middleware.GetUserID(r.Context()), "database_created", "database", "database", item.ID, item.Name)
	h.changed(item.ID, "")
	writeJSON(w, http.StatusCreated, item)
}

func (h *DatabasesHandler) Get(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetDatabase(activeWorkspaceID(h.db), chi.URLParam(r, "id"))
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *DatabasesHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id := chi.URLParam(r, "id")
	item, err := h.store.UpdateDatabase(activeWorkspaceID(h.db), id, req.Name, req.Description)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	h.db.LogAudit(middleware.GetUserID(r.Context()), "database_updated", "database", "database", id, item.Name)
	h.changed(id, "")
	writeJSON(w, http.StatusOK, item)
}

func (h *DatabasesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteDatabase(activeWorkspaceID(h.db), id); err != nil {
		writeDatabaseError(w, err)
		return
	}
	h.db.LogAudit(middleware.GetUserID(r.Context()), "database_deleted", "database", "database", id, "")
	h.changed(id, "")
	w.WriteHeader(http.StatusNoContent)
}

func (h *DatabasesHandler) CreateTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	databaseID := chi.URLParam(r, "id")
	table, err := h.store.CreateTable(activeWorkspaceID(h.db), databaseID, req.Name)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	h.db.LogAudit(middleware.GetUserID(r.Context()), "database_table_created", "database", "database_table", table.ID, table.Name)
	h.changed(databaseID, table.ID)
	writeJSON(w, http.StatusCreated, table)
}

func (h *DatabasesHandler) UpdateTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name *string `json:"name"`
	}
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	table, err := h.store.UpdateTable(activeWorkspaceID(h.db), chi.URLParam(r, "tableId"), req.Name)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	h.db.LogAudit(middleware.GetUserID(r.Context()), "database_table_updated", "database", "database_table", table.ID, table.Name)
	h.changed(table.DatabaseID, table.ID)
	writeJSON(w, http.StatusOK, table)
}

func (h *DatabasesHandler) DeleteTable(w http.ResponseWriter, r *http.Request) {
	tableID := chi.URLParam(r, "tableId")
	table, err := h.store.GetTable(activeWorkspaceID(h.db), tableID)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	if err := h.store.DeleteTable(activeWorkspaceID(h.db), tableID); err != nil {
		writeDatabaseError(w, err)
		return
	}
	h.db.LogAudit(middleware.GetUserID(r.Context()), "database_table_deleted", "database", "database_table", tableID, table.Name)
	h.changed(table.DatabaseID, tableID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *DatabasesHandler) CreateColumn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string                 `json:"name"`
		Type    string                 `json:"type"`
		Options map[string]interface{} `json:"options"`
	}
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Type == "" {
		req.Type = "text"
	}
	tableID := chi.URLParam(r, "tableId")
	column, err := h.store.CreateColumn(activeWorkspaceID(h.db), tableID, req.Name, req.Type, req.Options)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	table, _ := h.store.GetTable(activeWorkspaceID(h.db), tableID)
	h.db.LogAudit(middleware.GetUserID(r.Context()), "database_column_created", "database", "database_column", column.ID, column.Name)
	h.changed(table.DatabaseID, tableID)
	writeJSON(w, http.StatusCreated, column)
}

func (h *DatabasesHandler) UpdateColumn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    *string                `json:"name"`
		Type    *string                `json:"type"`
		Options map[string]interface{} `json:"options"`
	}
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	column, err := h.store.UpdateColumn(activeWorkspaceID(h.db), chi.URLParam(r, "columnId"), req.Name, req.Type, req.Options)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	table, _ := h.store.GetTable(activeWorkspaceID(h.db), column.TableID)
	h.db.LogAudit(middleware.GetUserID(r.Context()), "database_column_updated", "database", "database_column", column.ID, column.Name)
	h.changed(table.DatabaseID, table.ID)
	writeJSON(w, http.StatusOK, column)
}

func (h *DatabasesHandler) DeleteColumn(w http.ResponseWriter, r *http.Request) {
	columnID := chi.URLParam(r, "columnId")
	if err := h.store.DeleteColumn(activeWorkspaceID(h.db), columnID); err != nil {
		writeDatabaseError(w, err)
		return
	}
	h.db.LogAudit(middleware.GetUserID(r.Context()), "database_column_deleted", "database", "database_column", columnID, "")
	h.changed("", "")
	w.WriteHeader(http.StatusNoContent)
}

func (h *DatabasesHandler) ListRows(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	page, err := h.store.ListRows(
		activeWorkspaceID(h.db),
		chi.URLParam(r, "tableId"),
		r.URL.Query().Get("search"),
		limit,
		offset,
	)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *DatabasesHandler) QueryRows(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	result, err := h.store.NamedRows(
		activeWorkspaceID(h.db),
		chi.URLParam(r, "tableId"),
		r.URL.Query().Get("search"),
		limit,
		offset,
	)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *DatabasesHandler) CreateRow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Values map[string]interface{} `json:"values"`
	}
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tableID := chi.URLParam(r, "tableId")
	row, err := h.store.CreateRow(activeWorkspaceID(h.db), tableID, req.Values)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	table, _ := h.store.GetTable(activeWorkspaceID(h.db), tableID)
	h.db.LogAudit(middleware.GetUserID(r.Context()), "database_row_created", "database", "database_row", row.ID, table.Name)
	h.changed(table.DatabaseID, tableID)
	writeJSON(w, http.StatusCreated, row)
}

func (h *DatabasesHandler) UpdateRow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Values map[string]interface{} `json:"values"`
	}
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	row, err := h.store.UpdateRow(activeWorkspaceID(h.db), chi.URLParam(r, "rowId"), req.Values)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	table, _ := h.store.GetTable(activeWorkspaceID(h.db), row.TableID)
	h.db.LogAudit(middleware.GetUserID(r.Context()), "database_row_updated", "database", "database_row", row.ID, table.Name)
	h.changed(table.DatabaseID, row.TableID)
	writeJSON(w, http.StatusOK, row)
}

func (h *DatabasesHandler) DeleteRow(w http.ResponseWriter, r *http.Request) {
	rowID := chi.URLParam(r, "rowId")
	if err := h.store.DeleteRow(activeWorkspaceID(h.db), rowID); err != nil {
		writeDatabaseError(w, err)
		return
	}
	h.db.LogAudit(middleware.GetUserID(r.Context()), "database_row_deleted", "database", "database_row", rowID, "")
	h.changed("", "")
	w.WriteHeader(http.StatusNoContent)
}
