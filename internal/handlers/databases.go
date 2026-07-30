package handlers

import (
	"bytes"
	"database/sql"
	"errors"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/userdb"
)

const maxDatabaseCSVUpload = 25 << 20

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
		strings.Contains(message, "invalid CSV"),
		strings.Contains(message, "CSV must"),
		strings.Contains(message, "CSV cannot"),
		strings.Contains(message, "unsupported column"),
		strings.Contains(message, "unknown column"),
		strings.Contains(message, "invalid sort"):
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

func (h *DatabasesHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxDatabaseCSVUpload+(1<<20))
	if err := r.ParseMultipartForm(maxDatabaseCSVUpload); err != nil {
		writeError(w, http.StatusBadRequest, "CSV file is required and must be 25 MB or smaller")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "CSV file is required")
		return
	}
	defer file.Close()
	if header.Size > maxDatabaseCSVUpload {
		writeError(w, http.StatusBadRequest, "CSV file must be 25 MB or smaller")
		return
	}
	if ext := strings.ToLower(filepath.Ext(header.Filename)); ext != ".csv" {
		writeError(w, http.StatusBadRequest, "file must be a CSV")
		return
	}

	result, err := h.store.ImportCSV(activeWorkspaceID(h.db), header.Filename, file)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	h.db.LogAudit(
		middleware.GetUserID(r.Context()),
		"database_imported",
		"database",
		"database",
		result.Database.ID,
		result.Database.Name,
	)
	tableID := ""
	if len(result.Database.Tables) > 0 {
		tableID = result.Database.Tables[0].ID
	}
	h.changed(result.Database.ID, tableID)
	writeJSON(w, http.StatusCreated, result)
}

func (h *DatabasesHandler) ExportTableCSV(w http.ResponseWriter, r *http.Request) {
	workspaceID := activeWorkspaceID(h.db)
	tableID := chi.URLParam(r, "tableId")
	table, err := h.store.GetTable(workspaceID, tableID)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}

	var output bytes.Buffer
	if _, err := h.store.ExportTableCSV(workspaceID, tableID, &output); err != nil {
		writeDatabaseError(w, err)
		return
	}
	filename := strings.TrimSpace(table.Name)
	if filename == "" {
		filename = "table"
	}
	filename = strings.NewReplacer("/", "-", "\\", "-", "\r", "", "\n", "").Replace(filename) + ".csv"
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output.Bytes())
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
	page, err := h.store.ListRowsSorted(
		activeWorkspaceID(h.db),
		chi.URLParam(r, "tableId"),
		r.URL.Query().Get("search"),
		limit,
		offset,
		r.URL.Query().Get("sort_column"),
		r.URL.Query().Get("sort_direction"),
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
