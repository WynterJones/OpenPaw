package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	railwayToken  string
	railwayClient = &http.Client{Timeout: 15 * time.Second}
	railwayBase   = "https://backboard.railway.com/graphql/v2"
)

func initRailway(token string) {
	railwayToken = token
}

func railwayQuery(query string, variables map[string]interface{}) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"query": query,
	}
	if variables != nil {
		payload["variables"] = variables
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", railwayBase, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+railwayToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := railwayClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Railway API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if errors, ok := result["errors"]; ok {
		return nil, fmt.Errorf("GraphQL errors: %v", errors)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format")
	}

	return data, nil
}

func registerRoutes(r chi.Router) {
	r.Get("/projects", handleListProjects)
	r.Get("/project/{id}", handleGetProject)
	r.Get("/deployments", handleListDeployments)
	r.Get("/services", handleListServices)
	r.Post("/redeploy", handleRedeploy)
}

func handleListProjects(w http.ResponseWriter, r *http.Request) {
	query := `{ me { projects { edges { node { id name description createdAt updatedAt } } } } }`

	data, err := railwayQuery(query, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	me, _ := data["me"].(map[string]interface{})
	if me == nil {
		writeError(w, http.StatusBadGateway, "unexpected response format")
		return
	}
	projects, _ := me["projects"].(map[string]interface{})
	if projects == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"projects": []interface{}{}})
		return
	}
	edges, _ := projects["edges"].([]interface{})

	output := make([]map[string]interface{}, 0, len(edges))
	for _, edge := range edges {
		e, ok := edge.(map[string]interface{})
		if !ok {
			continue
		}
		node, ok := e["node"].(map[string]interface{})
		if !ok {
			continue
		}
		output = append(output, map[string]interface{}{
			"id":          node["id"],
			"name":        node["name"],
			"description": node["description"],
			"created_at":  node["createdAt"],
			"updated_at":  node["updatedAt"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":    len(output),
		"projects": output,
	})
}

func handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	query := `query($id: String!) {
		project(id: $id) {
			id
			name
			description
			environments { edges { node { id name } } }
			services { edges { node { id name } } }
		}
	}`

	data, err := railwayQuery(query, map[string]interface{}{"id": id})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	project, ok := data["project"].(map[string]interface{})
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	environments := extractEdgeNodes(project, "environments")
	services := extractEdgeNodes(project, "services")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":           project["id"],
		"name":         project["name"],
		"description":  project["description"],
		"environments": environments,
		"services":     services,
	})
}

func handleListDeployments(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id parameter is required")
		return
	}

	serviceID := r.URL.Query().Get("service_id")

	query := `query($projectId: String!, $serviceId: String) {
		deployments(
			first: 25
			input: {
				projectId: $projectId
				serviceId: $serviceId
			}
		) {
			edges {
				node {
					id
					status
					createdAt
					staticUrl
				}
			}
		}
	}`

	vars := map[string]interface{}{
		"projectId": projectID,
	}
	if serviceID != "" {
		vars["serviceId"] = serviceID
	}

	data, err := railwayQuery(query, vars)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	deployments, _ := data["deployments"].(map[string]interface{})
	if deployments == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"deployments": []interface{}{}})
		return
	}
	edges, _ := deployments["edges"].([]interface{})

	output := make([]map[string]interface{}, 0, len(edges))
	for _, edge := range edges {
		e, ok := edge.(map[string]interface{})
		if !ok {
			continue
		}
		node, ok := e["node"].(map[string]interface{})
		if !ok {
			continue
		}
		output = append(output, map[string]interface{}{
			"id":         node["id"],
			"status":     node["status"],
			"created_at": node["createdAt"],
			"static_url": node["staticUrl"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":       len(output),
		"deployments": output,
	})
}

func handleListServices(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id parameter is required")
		return
	}

	query := `query($id: String!) {
		project(id: $id) {
			services { edges { node { id name icon } } }
		}
	}`

	data, err := railwayQuery(query, map[string]interface{}{"id": projectID})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	project, ok := data["project"].(map[string]interface{})
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	services := make([]map[string]interface{}, 0)
	if svcConn, ok := project["services"].(map[string]interface{}); ok {
		if edges, ok := svcConn["edges"].([]interface{}); ok {
			for _, edge := range edges {
				e, ok := edge.(map[string]interface{})
				if !ok {
					continue
				}
				node, ok := e["node"].(map[string]interface{})
				if !ok {
					continue
				}
				services = append(services, map[string]interface{}{
					"id":   node["id"],
					"name": node["name"],
					"icon": node["icon"],
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":    len(services),
		"services": services,
	})
}

type redeployRequest struct {
	ServiceID     string `json:"service_id"`
	EnvironmentID string `json:"environment_id"`
}

func handleRedeploy(w http.ResponseWriter, r *http.Request) {
	var req redeployRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ServiceID == "" || req.EnvironmentID == "" {
		writeError(w, http.StatusBadRequest, "service_id and environment_id are required")
		return
	}

	query := `mutation($serviceId: String!, $environmentId: String!) {
		serviceInstanceRedeploy(serviceId: $serviceId, environmentId: $environmentId)
	}`

	_, err := railwayQuery(query, map[string]interface{}{
		"serviceId":     req.ServiceID,
		"environmentId": req.EnvironmentID,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"service_id": req.ServiceID,
	})
}

func extractEdgeNodes(parent map[string]interface{}, field string) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	conn, ok := parent[field].(map[string]interface{})
	if !ok {
		return result
	}
	edges, ok := conn["edges"].([]interface{})
	if !ok {
		return result
	}
	for _, edge := range edges {
		e, ok := edge.(map[string]interface{})
		if !ok {
			continue
		}
		node, ok := e["node"].(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, node)
	}
	return result
}
