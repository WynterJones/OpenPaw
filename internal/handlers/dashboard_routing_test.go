package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
)

func newTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertTestDashboard(t *testing.T, db *database.DB, id, name, dashType string) {
	t.Helper()
	if _, err := db.Exec(
		"INSERT INTO dashboards (id, name, description, layout, widgets, dashboard_type) VALUES (?, ?, '', '{}', '[]', ?)",
		id, name, dashType,
	); err != nil {
		t.Fatalf("insert dashboard: %v", err)
	}
}

// The gateway keeps picking "update_tool" for "fix my X Dashboard" requests. It
// then dead-ends on "could not find an existing service named X" and the user's
// request is silently dropped, so routing has to correct the target itself.
func TestRetargetDashboardWorkOrder(t *testing.T) {
	db := newTestDB(t)
	h := &ChatHandler{db: db}

	insertTestDashboard(t, db, "dash-1", "Product & Subscription Manager Dashboard", "custom")
	insertTestDashboard(t, db, "dash-legacy", "Legacy Metrics", "config")
	if _, err := db.Exec(
		"INSERT INTO tools (id, name, description, type, config, enabled, status) VALUES ('tool-1', 'Weather Service', '', 'custom', '{}', 1, 'ready')",
	); err != nil {
		t.Fatalf("insert tool: %v", err)
	}

	cases := []struct {
		name          string
		action        string
		title         string
		dashboardID   string
		toolID        string
		wantAction    string
		wantDashboard string
	}{
		{
			name:   "update_tool naming a dashboard becomes a dashboard update",
			action: "update_tool", title: "Product & Subscription Manager Dashboard",
			wantAction: "build_custom_dashboard", wantDashboard: "dash-1",
		},
		{
			name:   "partial title still resolves the dashboard",
			action: "update_tool", title: "Product & Subscription Manager",
			wantAction: "build_custom_dashboard", wantDashboard: "dash-1",
		},
		{
			name:   "explicit dashboard_id wins over the service action",
			action: "update_tool", title: "Something Else", dashboardID: "dash-1",
			wantAction: "build_custom_dashboard", wantDashboard: "dash-1",
		},
		{
			name:   "config dashboards route to the config builder",
			action: "update_tool", title: "Legacy Metrics",
			wantAction: "build_dashboard", wantDashboard: "dash-legacy",
		},
		{
			name:   "a real service is left alone",
			action: "update_tool", title: "Weather Service", toolID: "tool-1",
			wantAction: "update_tool",
		},
		{
			name:   "a service that only matches by name is left alone",
			action: "update_tool", title: "Weather Service",
			wantAction: "update_tool",
		},
		{
			name:   "build_tool naming an existing dashboard is corrected too",
			action: "build_tool", title: "Product & Subscription Manager Dashboard",
			wantAction: "build_custom_dashboard", wantDashboard: "dash-1",
		},
		{
			name:   "an unknown name is left alone",
			action: "build_tool", title: "Invoice Emailer",
			wantAction: "build_tool",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &agents.GatewayResponse{
				Action: tc.action,
				WorkOrder: &agents.GatewayWorkOrder{
					Title:       tc.title,
					DashboardID: tc.dashboardID,
					ToolID:      tc.toolID,
				},
			}
			h.retargetDashboardWorkOrder(resp, DefaultWorkspaceID)

			if resp.Action != tc.wantAction {
				t.Errorf("action = %q, want %q", resp.Action, tc.wantAction)
			}
			if resp.WorkOrder.DashboardID != tc.wantDashboard {
				t.Errorf("dashboard_id = %q, want %q", resp.WorkOrder.DashboardID, tc.wantDashboard)
			}
		})
	}
}

// Dashboards render in a frame with an opaque origin, where localStorage and
// cookies silently fail — everything they save has to round-trip through here.
func TestDashboardStorage_RoundTrip(t *testing.T) {
	db := newTestDB(t)
	h := &DashboardsHandler{db: db}
	insertTestDashboard(t, db, "dash-1", "Stack Manager", "custom")

	r := chi.NewRouter()
	r.Get("/d/{id}/storage", h.ListStorage)
	r.Delete("/d/{id}/storage", h.ClearStorage)
	r.Get("/d/{id}/storage/{key}", h.GetStorageItem)
	r.Put("/d/{id}/storage/{key}", h.SetStorageItem)
	r.Delete("/d/{id}/storage/{key}", h.DeleteStorageItem)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		var rdr *strings.Reader
		if body == "" {
			rdr = strings.NewReader("")
		} else {
			rdr = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	// An unset key reads back as null rather than 404, so first-run dashboard
	// code needs no special casing.
	rec := do(http.MethodGet, "/d/dash-1/storage/products", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get unset key: status %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Value json.RawMessage `json:"value"`
		Found bool            `json:"found"`
	}
	decodeTestJSON(t, rec, &got)
	if got.Found {
		t.Errorf("unset key reported found")
	}

	// Save a list, then read it back — the write is what the old localStorage
	// path only pretended to do.
	list := `[{"name":"Widget","price":9.5}]`
	if rec := do(http.MethodPut, "/d/dash-1/storage/products", `{"value":`+list+`}`); rec.Code != http.StatusOK {
		t.Fatalf("set: status %d (%s)", rec.Code, rec.Body.String())
	}
	rec = do(http.MethodGet, "/d/dash-1/storage/products", "")
	decodeTestJSON(t, rec, &got)
	if !got.Found || string(got.Value) != list {
		t.Errorf("value = %s (found=%v), want %s", got.Value, got.Found, list)
	}

	// Overwriting replaces rather than appending.
	if rec := do(http.MethodPut, "/d/dash-1/storage/products", `{"value":[]}`); rec.Code != http.StatusOK {
		t.Fatalf("overwrite: status %d", rec.Code)
	}
	rec = do(http.MethodGet, "/d/dash-1/storage/products", "")
	decodeTestJSON(t, rec, &got)
	if string(got.Value) != "[]" {
		t.Errorf("after overwrite value = %s, want []", got.Value)
	}

	// List returns everything at once. Decoded into a fresh map each time —
	// unmarshalling into a reused map merges rather than replaces.
	listAll := func() map[string]json.RawMessage {
		t.Helper()
		var listed struct {
			Items map[string]json.RawMessage `json:"items"`
		}
		decodeTestJSON(t, do(http.MethodGet, "/d/dash-1/storage", ""), &listed)
		return listed.Items
	}

	do(http.MethodPut, "/d/dash-1/storage/settings", `{"value":{"currency":"USD"}}`)
	items := listAll()
	if len(items) != 2 || string(items["settings"]) != `{"currency":"USD"}` {
		t.Errorf("items = %v, want products + settings", items)
	}

	// Keys are user-facing strings the client percent-encodes — they must be
	// stored and listed decoded, or all() would disagree with set()/get().
	do(http.MethodPut, "/d/dash-1/storage/monthly%20totals", `{"value":42}`)
	rec = do(http.MethodGet, "/d/dash-1/storage/monthly%20totals", "")
	decodeTestJSON(t, rec, &got)
	if !got.Found || string(got.Value) != "42" {
		t.Errorf("encoded key value = %s (found=%v), want 42", got.Value, got.Found)
	}
	if _, ok := listAll()["monthly totals"]; !ok {
		t.Errorf("encoded key not listed under its decoded name: %v", listAll())
	}
	do(http.MethodDelete, "/d/dash-1/storage/monthly%20totals", "")

	// Delete one, clear the rest.
	if rec := do(http.MethodDelete, "/d/dash-1/storage/products", ""); rec.Code != http.StatusOK {
		t.Fatalf("delete: status %d", rec.Code)
	}
	if _, still := listAll()["products"]; still {
		t.Errorf("deleted key still listed")
	}
	do(http.MethodDelete, "/d/dash-1/storage", "")
	if items := listAll(); len(items) != 0 {
		t.Errorf("items after clear = %v, want empty", items)
	}
}

// Dashboards load openpaw-sdk.js from their own directory, so every dashboard
// built before an SDK change would be stuck without the new API (storage, most
// importantly) until someone rebuilt it. Serve the embedded copy instead.
func TestServeAssets_SDKComesFromEmbed(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	h := &DashboardsHandler{db: db, dashboardsDir: dir}
	insertTestDashboard(t, db, "dash-1", "Stack Manager", "custom")

	dashDir := filepath.Join(dir, "dash-1")
	if err := os.MkdirAll(dashDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stale := "window.OpenPaw = { old: true };"
	if err := os.WriteFile(filepath.Join(dashDir, "openpaw-sdk.js"), []byte(stale), 0644); err != nil {
		t.Fatalf("write stale sdk: %v", err)
	}

	r := chi.NewRouter()
	r.Get("/d/{id}/assets/*", h.ServeAssets)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/d/dash-1/assets/openpaw-sdk.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if body == stale {
		t.Fatal("served the stale on-disk SDK")
	}
	if !strings.Contains(body, "storageSet") {
		t.Errorf("served SDK has no storage API:\n%s", body)
	}
	if !strings.Contains(body, "queryDatabase") {
		t.Errorf("served SDK has no database query API:\n%s", body)
	}
}

// The traversal guard compared a bare prefix, so a dashboard could read out of
// any sibling directory whose name it prefixes — on a public route.
func TestServeAssets_RejectsSiblingDirectory(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	h := &DashboardsHandler{db: db, dashboardsDir: dir}
	insertTestDashboard(t, db, "dash", "Mine", "custom")

	if err := os.MkdirAll(filepath.Join(dir, "dash-other"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dash-other", "secret.txt"), []byte("private"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	r := chi.NewRouter()
	r.Get("/d/{id}/assets/*", h.ServeAssets)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/d/dash/assets/../dash-other/secret.txt", nil))

	if rec.Code == http.StatusOK {
		t.Errorf("read a sibling dashboard's file: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "private") {
		t.Errorf("leaked sibling file contents")
	}
}

// Storage rows are addressed by dashboard ID, so an unknown ID must be rejected
// rather than quietly accumulating orphan rows.
func TestDashboardStorage_UnknownDashboard(t *testing.T) {
	db := newTestDB(t)
	h := &DashboardsHandler{db: db}

	r := chi.NewRouter()
	r.Put("/d/{id}/storage/{key}", h.SetStorageItem)

	req := httptest.NewRequest(http.MethodPut, "/d/nope/storage/k", strings.NewReader(`{"value":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM dashboard_storage").Scan(&count)
	if count != 0 {
		t.Errorf("wrote %d orphan rows", count)
	}
}
