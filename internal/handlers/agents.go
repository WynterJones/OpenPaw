package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
)

type AgentsHandler struct {
	db      *database.DB
	manager *agents.Manager
}

func NewAgentsHandler(db *database.DB, manager *agents.Manager) *AgentsHandler {
	return &AgentsHandler{db: db, manager: manager}
}

func (h *AgentsHandler) List(w http.ResponseWriter, r *http.Request) {
	agentList, err := h.manager.ListAllAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	if agentList == nil {
		agentList = []models.Agent{}
	}
	writeJSON(w, http.StatusOK, agentList)
}

func (h *AgentsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agent, err := h.manager.GetAgent(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (h *AgentsHandler) Stop(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	userID := middleware.GetUserID(r.Context())

	if err := h.manager.StopAgent(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.db.LogAudit(userID, "agent_stopped", "agent", "agent", id, "")

	writeJSON(w, http.StatusOK, map[string]string{"message": "agent stopped"})
}
