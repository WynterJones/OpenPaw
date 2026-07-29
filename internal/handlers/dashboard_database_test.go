package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/userdb"
)

func TestRefreshData_DatabaseSourceUsesNamedWorkspaceRows(t *testing.T) {
	db := newTestDB(t)
	store := userdb.NewStore(db)

	records, err := store.CreateDatabase(database.DefaultWorkspaceID, "Projects", "Current projects")
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	table := records.Tables[0]
	status, err := store.CreateColumn(database.DefaultWorkspaceID, table.ID, "Status", "select", map[string]interface{}{
		"choices": []interface{}{"Planned", "Active"},
	})
	if err != nil {
		t.Fatalf("create column: %v", err)
	}
	if _, err := store.CreateRow(database.DefaultWorkspaceID, table.ID, map[string]interface{}{
		table.Columns[0].ID: "OpenPaw",
		status.ID:           "Active",
	}); err != nil {
		t.Fatalf("create row: %v", err)
	}

	widgets, _ := json.Marshal([]map[string]interface{}{{
		"id": "projects",
		"dataSource": map[string]interface{}{
			"type": "database", "databaseId": records.ID, "tableId": table.ID, "limit": 50,
		},
	}})
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO dashboards
		 (id, name, description, layout, widgets, workspace_id, created_at, updated_at)
		 VALUES ('dash-db', 'Projects', '', '{}', ?, ?, ?, ?)`,
		string(widgets), database.DefaultWorkspaceID, now, now,
	); err != nil {
		t.Fatalf("insert dashboard: %v", err)
	}

	h := NewDashboardsHandler(db, nil, "")
	router := chi.NewRouter()
	router.Post("/dashboards/{id}/refresh", h.RefreshData)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/dashboards/dash-db/refresh", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response map[string]struct {
		Columns []string                 `json:"columns"`
		Records []map[string]interface{} `json:"records"`
		Total   int                      `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	got := response["projects"]
	if got.Total != 1 || len(got.Records) != 1 {
		t.Fatalf("database result = %+v", got)
	}
	if got.Records[0]["Name"] != "OpenPaw" || got.Records[0]["Status"] != "Active" {
		t.Errorf("named record = %#v", got.Records[0])
	}
}

func TestRefreshData_DatabaseSourceCannotCrossWorkspaces(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(
		"INSERT INTO workspaces (id, name, sort_order) VALUES ('private-ws', 'Private', 1)",
	); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	store := userdb.NewStore(db)
	privateDB, err := store.CreateDatabase("private-ws", "Private records", "")
	if err != nil {
		t.Fatalf("create private database: %v", err)
	}

	widgets, _ := json.Marshal([]map[string]interface{}{{
		"id": "private",
		"dataSource": map[string]interface{}{
			"type": "database", "databaseId": privateDB.ID, "tableId": privateDB.Tables[0].ID,
		},
	}})
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO dashboards
		 (id, name, description, layout, widgets, workspace_id, created_at, updated_at)
		 VALUES ('dash-default', 'Default', '', '{}', ?, ?, ?, ?)`,
		string(widgets), database.DefaultWorkspaceID, now, now,
	); err != nil {
		t.Fatalf("insert dashboard: %v", err)
	}

	h := NewDashboardsHandler(db, nil, "")
	router := chi.NewRouter()
	router.Post("/dashboards/{id}/refresh", h.RefreshData)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/dashboards/dash-default/refresh", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response map[string]map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["private"]["error"] == nil {
		t.Fatalf("cross-workspace table was returned: %s", rec.Body.String())
	}
}
