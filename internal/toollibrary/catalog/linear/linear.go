package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	linearAPIKey string
	linearClient = &http.Client{Timeout: 15 * time.Second}
	linearAPI    = "https://api.linear.app/graphql"
)

func initLinear(key string) {
	linearAPIKey = key
}

type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type graphQLResponse struct {
	Data   map[string]interface{} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func linearQuery(query string, variables map[string]interface{}) (*graphQLResponse, error) {
	payload, err := json.Marshal(graphQLRequest{
		Query:     query,
		Variables: variables,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal GraphQL request: %w", err)
	}

	req, err := http.NewRequest("POST", linearAPI, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", linearAPIKey)

	resp, err := linearClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Linear API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Linear response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Linear API error (%d): %s", resp.StatusCode, string(body))
	}

	var result graphQLResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse Linear response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("Linear GraphQL error: %s", result.Errors[0].Message)
	}

	return &result, nil
}

func registerRoutes(r chi.Router) {
	r.Get("/issues", handleListIssues)
	r.Get("/issue/{id}", handleGetIssue)
	r.Post("/issue", handleCreateIssue)
	r.Put("/issue/{id}", handleUpdateIssue)
	r.Get("/teams", handleListTeams)
}

func handleListIssues(w http.ResponseWriter, r *http.Request) {
	team := r.URL.Query().Get("team")
	status := r.URL.Query().Get("status")
	limitStr := r.URL.Query().Get("limit")

	limit := 25
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}

	filterParts := []string{}
	variables := map[string]interface{}{
		"limit": limit,
	}

	if team != "" {
		filterParts = append(filterParts, `team: { key: { eq: $team } }`)
		variables["team"] = team
	}
	if status != "" {
		filterParts = append(filterParts, `state: { name: { eq: $status } }`)
		variables["status"] = status
	}

	filterClause := ""
	if len(filterParts) > 0 {
		filterClause = "filter: { "
		for i, part := range filterParts {
			if i > 0 {
				filterClause += ", "
			}
			filterClause += part
		}
		filterClause += " }, "
	}

	varDefs := "$limit: Int!"
	if team != "" {
		varDefs += ", $team: String!"
	}
	if status != "" {
		varDefs += ", $status: String!"
	}

	query := fmt.Sprintf(`query(%s) {
		issues(%sfirst: $limit, orderBy: createdAt) {
			nodes {
				id
				identifier
				title
				description
				state { name }
				assignee { name }
				priority
				createdAt
				updatedAt
				url
			}
		}
	}`, varDefs, filterClause)

	result, err := linearQuery(query, variables)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	issues, _ := result.Data["issues"].(map[string]interface{})
	nodes, _ := issues["nodes"].([]interface{})

	output := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		issue, ok := n.(map[string]interface{})
		if !ok {
			continue
		}

		stateName := ""
		if state, ok := issue["state"].(map[string]interface{}); ok {
			stateName, _ = state["name"].(string)
		}

		assigneeName := ""
		if assignee, ok := issue["assignee"].(map[string]interface{}); ok {
			assigneeName, _ = assignee["name"].(string)
		}

		output = append(output, map[string]interface{}{
			"id":         issue["id"],
			"identifier": issue["identifier"],
			"title":      issue["title"],
			"state":      stateName,
			"assignee":   assigneeName,
			"priority":   issue["priority"],
			"createdAt":  issue["createdAt"],
			"updatedAt":  issue["updatedAt"],
			"url":        issue["url"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":  len(output),
		"issues": output,
	})
}

func handleGetIssue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "issue ID is required")
		return
	}

	query := `query($id: String!) {
		issue(id: $id) {
			id
			identifier
			title
			description
			state { id name }
			assignee { id name }
			priority
			estimate
			createdAt
			updatedAt
			url
			team { id name key }
			labels { nodes { id name color } }
			comments { nodes { id body user { name } createdAt } }
		}
	}`

	result, err := linearQuery(query, map[string]interface{}{"id": id})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	issue, ok := result.Data["issue"].(map[string]interface{})
	if !ok || issue == nil {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}

	stateName := ""
	stateID := ""
	if state, ok := issue["state"].(map[string]interface{}); ok {
		stateName, _ = state["name"].(string)
		stateID, _ = state["id"].(string)
	}

	assigneeName := ""
	assigneeID := ""
	if assignee, ok := issue["assignee"].(map[string]interface{}); ok {
		assigneeName, _ = assignee["name"].(string)
		assigneeID, _ = assignee["id"].(string)
	}

	teamName := ""
	teamKey := ""
	teamID := ""
	if team, ok := issue["team"].(map[string]interface{}); ok {
		teamName, _ = team["name"].(string)
		teamKey, _ = team["key"].(string)
		teamID, _ = team["id"].(string)
	}

	labels := []map[string]interface{}{}
	if labelsObj, ok := issue["labels"].(map[string]interface{}); ok {
		if nodes, ok := labelsObj["nodes"].([]interface{}); ok {
			for _, n := range nodes {
				if label, ok := n.(map[string]interface{}); ok {
					labels = append(labels, map[string]interface{}{
						"id":    label["id"],
						"name":  label["name"],
						"color": label["color"],
					})
				}
			}
		}
	}

	comments := []map[string]interface{}{}
	if commentsObj, ok := issue["comments"].(map[string]interface{}); ok {
		if nodes, ok := commentsObj["nodes"].([]interface{}); ok {
			for _, n := range nodes {
				if comment, ok := n.(map[string]interface{}); ok {
					userName := ""
					if user, ok := comment["user"].(map[string]interface{}); ok {
						userName, _ = user["name"].(string)
					}
					comments = append(comments, map[string]interface{}{
						"id":        comment["id"],
						"body":      comment["body"],
						"user":      userName,
						"createdAt": comment["createdAt"],
					})
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"issue": map[string]interface{}{
			"id":          issue["id"],
			"identifier":  issue["identifier"],
			"title":       issue["title"],
			"description": issue["description"],
			"state":       stateName,
			"state_id":    stateID,
			"assignee":    assigneeName,
			"assignee_id": assigneeID,
			"priority":    issue["priority"],
			"estimate":    issue["estimate"],
			"team":        teamName,
			"team_key":    teamKey,
			"team_id":     teamID,
			"labels":      labels,
			"comments":    comments,
			"createdAt":   issue["createdAt"],
			"updatedAt":   issue["updatedAt"],
			"url":         issue["url"],
		},
	})
}

type createIssueRequest struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	TeamID      string  `json:"teamId"`
	Priority    *int    `json:"priority"`
}

func handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	var req createIssueRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Title == "" || req.TeamID == "" {
		writeError(w, http.StatusBadRequest, "title and teamId are required")
		return
	}

	input := map[string]interface{}{
		"title":  req.Title,
		"teamId": req.TeamID,
	}
	if req.Description != "" {
		input["description"] = req.Description
	}
	if req.Priority != nil {
		input["priority"] = *req.Priority
	}

	query := `mutation($input: IssueCreateInput!) {
		issueCreate(input: $input) {
			success
			issue {
				id
				identifier
				title
				url
				state { name }
				priority
				createdAt
			}
		}
	}`

	result, err := linearQuery(query, map[string]interface{}{"input": input})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	create, _ := result.Data["issueCreate"].(map[string]interface{})
	success, _ := create["success"].(bool)
	if !success {
		writeError(w, http.StatusBadGateway, "failed to create issue")
		return
	}

	issue, _ := create["issue"].(map[string]interface{})
	stateName := ""
	if state, ok := issue["state"].(map[string]interface{}); ok {
		stateName, _ = state["name"].(string)
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"issue": map[string]interface{}{
			"id":         issue["id"],
			"identifier": issue["identifier"],
			"title":      issue["title"],
			"state":      stateName,
			"priority":   issue["priority"],
			"url":        issue["url"],
			"createdAt":  issue["createdAt"],
		},
	})
}

type updateIssueRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	StateID     *string `json:"stateId"`
	Priority    *int    `json:"priority"`
}

func handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "issue ID is required")
		return
	}

	var req updateIssueRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	input := map[string]interface{}{}
	if req.Title != nil {
		input["title"] = *req.Title
	}
	if req.Description != nil {
		input["description"] = *req.Description
	}
	if req.StateID != nil {
		input["stateId"] = *req.StateID
	}
	if req.Priority != nil {
		input["priority"] = *req.Priority
	}

	if len(input) == 0 {
		writeError(w, http.StatusBadRequest, "at least one field to update is required")
		return
	}

	query := `mutation($id: String!, $input: IssueUpdateInput!) {
		issueUpdate(id: $id, input: $input) {
			success
			issue {
				id
				identifier
				title
				url
				state { name }
				priority
				updatedAt
			}
		}
	}`

	result, err := linearQuery(query, map[string]interface{}{
		"id":    id,
		"input": input,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	update, _ := result.Data["issueUpdate"].(map[string]interface{})
	success, _ := update["success"].(bool)
	if !success {
		writeError(w, http.StatusBadGateway, "failed to update issue")
		return
	}

	issue, _ := update["issue"].(map[string]interface{})
	stateName := ""
	if state, ok := issue["state"].(map[string]interface{}); ok {
		stateName, _ = state["name"].(string)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"issue": map[string]interface{}{
			"id":         issue["id"],
			"identifier": issue["identifier"],
			"title":      issue["title"],
			"state":      stateName,
			"priority":   issue["priority"],
			"url":        issue["url"],
			"updatedAt":  issue["updatedAt"],
		},
	})
}

func handleListTeams(w http.ResponseWriter, r *http.Request) {
	query := `query {
		teams {
			nodes {
				id
				name
				key
				description
			}
		}
	}`

	result, err := linearQuery(query, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	teams, _ := result.Data["teams"].(map[string]interface{})
	nodes, _ := teams["nodes"].([]interface{})

	output := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		team, ok := n.(map[string]interface{})
		if !ok {
			continue
		}
		output = append(output, map[string]interface{}{
			"id":          team["id"],
			"name":        team["name"],
			"key":         team["key"],
			"description": team["description"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(output),
		"teams": output,
	})
}
