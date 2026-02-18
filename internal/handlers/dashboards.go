package handlers

import (
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
)

type DashboardsHandler struct {
	db            *database.DB
	toolMgr       agents.ToolManager
	dashboardsDir string
}

func NewDashboardsHandler(db *database.DB, toolMgr agents.ToolManager, dashboardsDir string) *DashboardsHandler {
	return &DashboardsHandler{db: db, toolMgr: toolMgr, dashboardsDir: dashboardsDir}
}

type dashboardResponse struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Layout         json.RawMessage `json:"layout"`
	Widgets        json.RawMessage `json:"widgets"`
	DashboardType  string          `json:"dashboard_type"`
	OwnerAgentSlug string          `json:"owner_agent_slug"`
	BgImage        string          `json:"bg_image"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func toDashboardResponse(d models.Dashboard) dashboardResponse {
	layout := json.RawMessage(d.Layout)
	if !json.Valid(layout) {
		layout = json.RawMessage("{}")
	}
	widgets := json.RawMessage(d.Widgets)
	if !json.Valid(widgets) {
		widgets = json.RawMessage("[]")
	}
	dashType := d.DashboardType
	if dashType == "" {
		dashType = "config"
	}
	return dashboardResponse{
		ID:             d.ID,
		Name:           d.Name,
		Description:    d.Description,
		Layout:         layout,
		Widgets:        widgets,
		DashboardType:  dashType,
		OwnerAgentSlug: d.OwnerAgentSlug,
		BgImage:        d.BgImage,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
}

func (h *DashboardsHandler) scanDashboard(scanner interface{ Scan(...interface{}) error }) (models.Dashboard, error) {
	var d models.Dashboard
	err := scanner.Scan(&d.ID, &d.Name, &d.Description, &d.Layout, &d.Widgets, &d.DashboardType, &d.OwnerAgentSlug, &d.BgImage, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

const dashboardCols = "id, name, description, layout, widgets, dashboard_type, owner_agent_slug, bg_image, created_at, updated_at"

func (h *DashboardsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		"SELECT "+dashboardCols+" FROM dashboards ORDER BY created_at DESC",
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list dashboards")
		return
	}
	defer rows.Close()

	dashboards := []dashboardResponse{}
	for rows.Next() {
		d, err := h.scanDashboard(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan dashboard")
			return
		}
		dashboards = append(dashboards, toDashboardResponse(d))
	}
	writeJSON(w, http.StatusOK, dashboards)
}

func (h *DashboardsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Layout      string `json:"layout"`
		Widgets     string `json:"widgets"`
		BgImage     string `json:"bg_image"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Layout == "" {
		req.Layout = "{}"
	}
	if req.Widgets == "" {
		req.Widgets = "[]"
	}

	id := generateID()
	now := time.Now().UTC()

	_, err := h.db.Exec(
		"INSERT INTO dashboards (id, name, description, layout, widgets, owner_agent_slug, bg_image, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id, req.Name, req.Description, req.Layout, req.Widgets, "", req.BgImage, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create dashboard")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "dashboard_created", "dashboard", "dashboard", id, req.Name)

	writeJSON(w, http.StatusCreated, toDashboardResponse(models.Dashboard{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Layout:      req.Layout,
		Widgets:     req.Widgets,
		BgImage:     req.BgImage,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
}

func (h *DashboardsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, err := h.scanDashboard(h.db.QueryRow(
		"SELECT "+dashboardCols+" FROM dashboards WHERE id = ?", id,
	))
	if err != nil {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}
	writeJSON(w, http.StatusOK, toDashboardResponse(d))
}

func (h *DashboardsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var exists string
	err := h.db.QueryRow("SELECT id FROM dashboards WHERE id = ?", id).Scan(&exists)
	if err != nil {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Layout      *string `json:"layout"`
		Widgets     *string `json:"widgets"`
		BgImage     *string `json:"bg_image"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC()
	setClauses := []string{"updated_at = ?"}
	args := []interface{}{now}

	if req.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		setClauses = append(setClauses, "description = ?")
		args = append(args, *req.Description)
	}
	if req.Layout != nil {
		setClauses = append(setClauses, "layout = ?")
		args = append(args, *req.Layout)
	}
	if req.Widgets != nil {
		setClauses = append(setClauses, "widgets = ?")
		args = append(args, *req.Widgets)
	}
	if req.BgImage != nil {
		setClauses = append(setClauses, "bg_image = ?")
		args = append(args, *req.BgImage)
	}

	args = append(args, id)
	if _, err := h.db.Exec("UPDATE dashboards SET "+strings.Join(setClauses, ", ")+" WHERE id = ?", args...); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update dashboard")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "dashboard_updated", "dashboard", "dashboard", id, "")

	d, _ := h.scanDashboard(h.db.QueryRow(
		"SELECT "+dashboardCols+" FROM dashboards WHERE id = ?", id,
	))
	writeJSON(w, http.StatusOK, toDashboardResponse(d))
}

func (h *DashboardsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Check dashboard type before deletion for cleanup
	var dashType string
	h.db.QueryRow("SELECT dashboard_type FROM dashboards WHERE id = ?", id).Scan(&dashType)

	result, err := h.db.Exec("DELETE FROM dashboards WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete dashboard")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}

	// Cleanup related data (CASCADE handles data_points, but clean up schedules too)
	h.db.Exec("DELETE FROM schedules WHERE dashboard_id = ?", id)

	// Remove custom dashboard files from disk
	if dashType == "custom" && h.dashboardsDir != "" {
		dashDir := filepath.Join(h.dashboardsDir, id)
		os.RemoveAll(dashDir)
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "dashboard_deleted", "dashboard", "dashboard", id, "")

	writeJSON(w, http.StatusOK, map[string]string{"message": "dashboard deleted"})
}

func (h *DashboardsHandler) ServeAssets(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Verify dashboard exists and is custom type
	var dashType string
	err := h.db.QueryRow("SELECT dashboard_type FROM dashboards WHERE id = ?", id).Scan(&dashType)
	if err != nil || dashType != "custom" {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}

	if h.dashboardsDir == "" {
		writeError(w, http.StatusInternalServerError, "dashboards directory not configured")
		return
	}

	// Extract the file path from the wildcard
	assetPath := chi.URLParam(r, "*")
	if assetPath == "" {
		assetPath = "index.html"
	}

	// Construct the full path and prevent traversal
	dashDir := filepath.Join(h.dashboardsDir, id)
	fullPath := filepath.Join(dashDir, assetPath)
	resolved, err := filepath.Abs(fullPath)
	if err != nil || !strings.HasPrefix(resolved, dashDir) {
		writeError(w, http.StatusForbidden, "invalid path")
		return
	}

	// Check file exists
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	// Set content type based on extension
	ext := filepath.Ext(resolved)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")

	http.ServeFile(w, r, resolved)
}

// RefreshData calls tool endpoints for each widget and returns fresh data.
func (h *DashboardsHandler) RefreshData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	d, err := h.scanDashboard(h.db.QueryRow(
		"SELECT "+dashboardCols+" FROM dashboards WHERE id = ?", id,
	))
	if err != nil {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}

	var widgets []struct {
		ID         string `json:"id"`
		DataSource *struct {
			Type     string `json:"type"`
			ToolID   string `json:"toolId"`
			Endpoint string `json:"endpoint"`
			Method   string `json:"method"`
			DataPath string `json:"dataPath"`
		} `json:"dataSource"`
	}
	if err := json.Unmarshal([]byte(d.Widgets), &widgets); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse widgets")
		return
	}

	results := make(map[string]interface{})
	for _, w := range widgets {
		if w.DataSource == nil || w.DataSource.Type != "tool" || w.DataSource.ToolID == "" {
			continue
		}
		if h.toolMgr == nil {
			continue
		}

		endpoint := w.DataSource.Endpoint
		if endpoint == "" {
			endpoint = "/"
		}

		data, err := h.toolMgr.CallTool(w.DataSource.ToolID, endpoint, nil)
		if err != nil {
			results[w.ID] = map[string]string{"error": err.Error()}
			continue
		}

		var parsed interface{}
		if json.Unmarshal(data, &parsed) == nil {
			if w.DataSource.DataPath != "" {
				parsed = extractDataPath(parsed, w.DataSource.DataPath)
			}
			results[w.ID] = parsed
		} else {
			results[w.ID] = string(data)
		}
	}

	writeJSON(w, http.StatusOK, results)
}

// GetWidgetData returns historical time-series data for a widget.
func (h *DashboardsHandler) GetWidgetData(w http.ResponseWriter, r *http.Request) {
	dashboardID := chi.URLParam(r, "id")
	widgetID := chi.URLParam(r, "widgetId")
	timeRange := r.URL.Query().Get("range")

	if timeRange == "" {
		timeRange = "24h"
	}

	var interval string
	switch timeRange {
	case "1h":
		interval = "-1 hour"
	case "6h":
		interval = "-6 hours"
	case "24h":
		interval = "-1 day"
	case "7d":
		interval = "-7 days"
	case "30d":
		interval = "-30 days"
	default:
		interval = "-1 day"
	}

	rows, err := h.db.Query(
		`SELECT id, dashboard_id, widget_id, tool_id, endpoint, data, collected_at
		 FROM dashboard_data_points
		 WHERE dashboard_id = ? AND widget_id = ? AND collected_at >= datetime('now', ?)
		 ORDER BY collected_at ASC`,
		dashboardID, widgetID, interval,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query data points")
		return
	}
	defer rows.Close()

	points := []map[string]interface{}{}
	for rows.Next() {
		var dp models.DashboardDataPoint
		if err := rows.Scan(&dp.ID, &dp.DashboardID, &dp.WidgetID, &dp.ToolID, &dp.Endpoint, &dp.Data, &dp.CollectedAt); err != nil {
			logger.Error("Failed to scan dashboard data point: %v", err)
			continue
		}
		var parsed interface{}
		json.Unmarshal([]byte(dp.Data), &parsed)
		points = append(points, map[string]interface{}{
			"collected_at": dp.CollectedAt,
			"data":         parsed,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"widget_id": widgetID,
		"range":     timeRange,
		"points":    points,
	})
}

// CollectData calls tool endpoints and stores results in dashboard_data_points.
func (h *DashboardsHandler) CollectData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	d, err := h.scanDashboard(h.db.QueryRow(
		"SELECT "+dashboardCols+" FROM dashboards WHERE id = ?", id,
	))
	if err != nil {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}

	collected := h.collectDashboardData(d)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"collected": collected,
	})
}

// collectDashboardData fetches data from tools and stores it. Returns count of collected points.
func (h *DashboardsHandler) collectDashboardData(d models.Dashboard) int {
	if h.toolMgr == nil {
		return 0
	}

	var widgets []struct {
		ID         string `json:"id"`
		DataSource *struct {
			Type     string `json:"type"`
			ToolID   string `json:"toolId"`
			Endpoint string `json:"endpoint"`
			DataPath string `json:"dataPath"`
		} `json:"dataSource"`
	}
	if err := json.Unmarshal([]byte(d.Widgets), &widgets); err != nil {
		return 0
	}

	collected := 0
	now := time.Now().UTC()
	for _, w := range widgets {
		if w.DataSource == nil || w.DataSource.Type != "tool" || w.DataSource.ToolID == "" {
			continue
		}

		endpoint := w.DataSource.Endpoint
		if endpoint == "" {
			endpoint = "/"
		}

		data, err := h.toolMgr.CallTool(w.DataSource.ToolID, endpoint, nil)
		if err != nil {
			continue
		}

		var parsed interface{}
		if json.Unmarshal(data, &parsed) == nil && w.DataSource.DataPath != "" {
			parsed = extractDataPath(parsed, w.DataSource.DataPath)
		}

		dataJSON, _ := json.Marshal(parsed)
		pointID := uuid.New().String()
		h.db.Exec(
			"INSERT INTO dashboard_data_points (id, dashboard_id, widget_id, tool_id, endpoint, data, collected_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			pointID, d.ID, w.ID, w.DataSource.ToolID, endpoint, string(dataJSON), now,
		)
		collected++
	}
	return collected
}

// extractDataPath traverses a JSON value using dot-notation (e.g. "current.temperature").
func extractDataPath(data interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	current := data
	for _, part := range parts {
		if part == "" {
			continue
		}
		m, ok := current.(map[string]interface{})
		if !ok {
			return current
		}
		val, exists := m[part]
		if !exists {
			return nil
		}
		current = val
	}
	return current
}
