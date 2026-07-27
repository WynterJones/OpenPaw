package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/models"
)

// A disabled gateway used to disappear from ?enabled=true, which is the list
// the chat resolves avatars and names from. Every message it posted then fell
// back to the stock avatar instead of the one the user picked for it.
func TestListRolesKeepsGatewayWhenDisabled(t *testing.T) {
	db, err := database.New(t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := agents.SeedPresetRoles(db, nil); err != nil {
		t.Fatalf("seed preset roles: %v", err)
	}
	if _, err := db.Exec("UPDATE agent_roles SET enabled = 0 WHERE slug = ?", gatewayRoleSlug); err != nil {
		t.Fatalf("disable gateway: %v", err)
	}

	h := &AgentRolesHandler{db: db}
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/agent-roles?enabled=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	var roles []models.AgentRole
	if err := json.Unmarshal(rec.Body.Bytes(), &roles); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var found *models.AgentRole
	for i := range roles {
		if roles[i].Slug == gatewayRoleSlug {
			found = &roles[i]
		}
		if roles[i].Slug != gatewayRoleSlug && !roles[i].Enabled {
			t.Errorf("role %q is disabled but was listed — only the gateway is exempt", roles[i].Slug)
		}
	}
	if found == nil {
		t.Fatal("the gateway role is missing from ?enabled=true")
	}
	if found.AvatarPath == "" {
		t.Error("the gateway came back without an avatar, which is the whole point of listing it")
	}
}
