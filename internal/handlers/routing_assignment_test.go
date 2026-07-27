package handlers

import (
	"testing"

	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
)

func insertAgentRole(t *testing.T, db *database.DB, slug, name string, enabled bool) {
	t.Helper()
	on := 0
	if enabled {
		on = 1
	}
	if _, err := db.Exec(
		"INSERT INTO agent_roles (slug, name, description, system_prompt, model, enabled) VALUES (?, ?, '', '', 'sonnet', ?)",
		slug, name, on,
	); err != nil {
		t.Fatalf("insert agent role: %v", err)
	}
}

// The gateway writes agent slugs from memory and gets them wrong. Every miss
// used to end the turn on "I couldn't find that agent role or it's disabled" —
// the user's message dropped on the floor, fixed only by saying it again.
func TestResolveGatewayAssignment(t *testing.T) {
	db := newTestDB(t)
	h := &ChatHandler{db: db}

	insertAgentRole(t, db, "research-assistant", "Research Assistant", true)
	insertAgentRole(t, db, "retired", "Retired Helper", false)

	cases := []struct {
		name     string
		assigned string
		want     string
	}{
		{"exact slug", "research-assistant", "research-assistant"},
		{"wrong case", "Research-Assistant", "research-assistant"},
		{"display name", "Research Assistant", "research-assistant"},
		{"name with spaces", "research assistant", "research-assistant"},
		{"invented agent is dropped", "feedback-specialist", ""},
		{"disabled agent is dropped", "retired", ""},
		{"empty stays empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &agents.GatewayResponse{Action: "route", AssignedAgent: tc.assigned}
			h.resolveGatewayAssignment(resp)
			if resp.AssignedAgent != tc.want {
				t.Errorf("assigned_agent = %q, want %q", resp.AssignedAgent, tc.want)
			}
		})
	}
}

// A multi-agent assignment has to survive one bad name in the list rather than
// taking the whole turn down with it.
func TestResolveGatewayAssignment_MultiAgent(t *testing.T) {
	db := newTestDB(t)
	h := &ChatHandler{db: db}

	insertAgentRole(t, db, "atlas", "Atlas", true)
	insertAgentRole(t, db, "whiskers", "Whiskers", true)

	t.Run("drops the invented one and keeps the rest", func(t *testing.T) {
		resp := &agents.GatewayResponse{AssignedAgents: []string{"atlas", "ghost", "Whiskers"}}
		h.resolveGatewayAssignment(resp)
		if len(resp.AssignedAgents) != 2 || resp.AssignedAgents[0] != "atlas" || resp.AssignedAgents[1] != "whiskers" {
			t.Errorf("got %v, want [atlas whiskers]", resp.AssignedAgents)
		}
	})

	t.Run("a list that collapses to one becomes a single assignment", func(t *testing.T) {
		resp := &agents.GatewayResponse{AssignedAgents: []string{"ghost", "atlas"}}
		h.resolveGatewayAssignment(resp)
		if resp.AssignedAgent != "atlas" {
			t.Errorf("assigned_agent = %q, want atlas", resp.AssignedAgent)
		}
	})

	t.Run("duplicates after resolution collapse", func(t *testing.T) {
		resp := &agents.GatewayResponse{AssignedAgents: []string{"atlas", "Atlas", "ATLAS"}}
		h.resolveGatewayAssignment(resp)
		if len(resp.AssignedAgents) != 1 {
			t.Errorf("got %v, want one entry", resp.AssignedAgents)
		}
	})

	t.Run("all invented leaves nothing, so routing falls through", func(t *testing.T) {
		resp := &agents.GatewayResponse{AssignedAgents: []string{"ghost", "phantom"}}
		h.resolveGatewayAssignment(resp)
		if len(resp.AssignedAgents) != 0 || resp.AssignedAgent != "" {
			t.Errorf("got %v / %q, want nothing assigned", resp.AssignedAgents, resp.AssignedAgent)
		}
	})
}
