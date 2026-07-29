package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/userdb"
)

func TestDeleteWorkspacePreservesDatabasesWithConflictingNames(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec("INSERT INTO workspaces (id, name, sort_order) VALUES ('old-workspace', 'Old', 1)"); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	store := userdb.NewStore(db)
	if _, err := store.CreateDatabase(database.DefaultWorkspaceID, "Projects", "Default records"); err != nil {
		t.Fatalf("create default database: %v", err)
	}
	old, err := store.CreateDatabase("old-workspace", "Projects", "Moved records")
	if err != nil {
		t.Fatalf("create workspace database: %v", err)
	}

	h := NewWorkspacesHandler(db, t.TempDir(), nil)
	router := chi.NewRouter()
	router.Delete("/workspaces/{id}", h.Delete)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/workspaces/old-workspace", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	items, err := store.ListDatabases(database.DefaultWorkspaceID)
	if err != nil || len(items) != 2 {
		t.Fatalf("default databases = %+v, %v", items, err)
	}
	moved, err := store.GetDatabase(database.DefaultWorkspaceID, old.ID)
	if err != nil {
		t.Fatalf("moved database missing: %v", err)
	}
	if moved.Name == "Projects" || moved.Description != "Moved records" {
		t.Fatalf("moved database = %+v", moved)
	}
}
