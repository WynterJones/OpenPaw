package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	sentryToken  string
	sentryOrg    string
	sentryClient = &http.Client{Timeout: 15 * time.Second}
	sentryBase   = "https://sentry.io"
)

func initSentry(token, org string) {
	sentryToken = token
	sentryOrg = org
}

func sentryRequest(method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, sentryBase+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sentryToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return sentryClient.Do(req)
}

func sentryReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Sentry API error (%d): %s", resp.StatusCode, string(data))
	}
	return json.Unmarshal(data, v)
}

func registerRoutes(r chi.Router) {
	r.Get("/projects", handleListProjects)
	r.Get("/issues", handleListIssues)
	r.Get("/issue/{id}", handleGetIssue)
	r.Put("/issue/{id}", handleUpdateIssue)
	r.Get("/events", handleListEvents)
}

func handleListProjects(w http.ResponseWriter, r *http.Request) {
	resp, err := sentryRequest("GET", fmt.Sprintf("/api/0/organizations/%s/projects/", sentryOrg), nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Sentry API request failed")
		return
	}

	var projects []map[string]interface{}
	if err := sentryReadJSON(resp, &projects); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(projects))
	for _, p := range projects {
		result = append(result, map[string]interface{}{
			"id":          p["id"],
			"slug":        p["slug"],
			"name":        p["name"],
			"platform":    p["platform"],
			"dateCreated": p["dateCreated"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":    len(result),
		"projects": result,
	})
}

func handleListIssues(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		writeError(w, http.StatusBadRequest, "project parameter is required")
		return
	}

	query := r.URL.Query().Get("query")
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "date"
	}

	params := url.Values{}
	if query != "" {
		params.Set("query", query)
	}
	params.Set("sort", sort)

	path := fmt.Sprintf("/api/0/projects/%s/%s/issues/?%s", sentryOrg, project, params.Encode())
	resp, err := sentryRequest("GET", path, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Sentry API request failed")
		return
	}

	var issues []map[string]interface{}
	if err := sentryReadJSON(resp, &issues); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(issues))
	for _, issue := range issues {
		result = append(result, map[string]interface{}{
			"id":        issue["id"],
			"title":     issue["title"],
			"culprit":   issue["culprit"],
			"status":    issue["status"],
			"level":     issue["level"],
			"count":     issue["count"],
			"firstSeen": issue["firstSeen"],
			"lastSeen":  issue["lastSeen"],
			"shortId":   issue["shortId"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"project": project,
		"count":   len(result),
		"issues":  result,
	})
}

func handleGetIssue(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "id")
	if issueID == "" {
		writeError(w, http.StatusBadRequest, "issue id is required")
		return
	}

	resp, err := sentryRequest("GET", fmt.Sprintf("/api/0/issues/%s/", issueID), nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Sentry API request failed")
		return
	}

	var issue map[string]interface{}
	if err := sentryReadJSON(resp, &issue); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":            issue["id"],
		"title":         issue["title"],
		"culprit":       issue["culprit"],
		"status":        issue["status"],
		"level":         issue["level"],
		"count":         issue["count"],
		"firstSeen":     issue["firstSeen"],
		"lastSeen":      issue["lastSeen"],
		"shortId":       issue["shortId"],
		"project":       issue["project"],
		"metadata":      issue["metadata"],
		"type":          issue["type"],
		"platform":      issue["platform"],
		"logger":        issue["logger"],
		"permalink":     issue["permalink"],
		"userCount":     issue["userCount"],
		"isPublic":      issue["isPublic"],
		"isSubscribed":  issue["isSubscribed"],
		"statusDetails": issue["statusDetails"],
	})
}

type updateIssueRequest struct {
	Status string `json:"status"`
}

func handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "id")
	if issueID == "" {
		writeError(w, http.StatusBadRequest, "issue id is required")
		return
	}

	var req updateIssueRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Status == "" {
		writeError(w, http.StatusBadRequest, "status is required")
		return
	}

	validStatuses := map[string]bool{
		"resolved":   true,
		"unresolved": true,
		"ignored":    true,
	}
	if !validStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "status must be 'resolved', 'unresolved', or 'ignored'")
		return
	}

	resp, err := sentryRequest("PUT", fmt.Sprintf("/api/0/issues/%s/", issueID), map[string]string{
		"status": req.Status,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "Sentry API request failed")
		return
	}

	var issue map[string]interface{}
	if err := sentryReadJSON(resp, &issue); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":        issue["id"],
		"title":     issue["title"],
		"status":    issue["status"],
		"shortId":   issue["shortId"],
		"firstSeen": issue["firstSeen"],
		"lastSeen":  issue["lastSeen"],
	})
}

func handleListEvents(w http.ResponseWriter, r *http.Request) {
	issueID := r.URL.Query().Get("issue_id")
	if issueID == "" {
		writeError(w, http.StatusBadRequest, "issue_id parameter is required")
		return
	}

	resp, err := sentryRequest("GET", fmt.Sprintf("/api/0/issues/%s/events/", issueID), nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Sentry API request failed")
		return
	}

	var events []map[string]interface{}
	if err := sentryReadJSON(resp, &events); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		result = append(result, map[string]interface{}{
			"id":          e["id"],
			"eventID":     e["eventID"],
			"dateCreated": e["dateCreated"],
			"message":     e["message"],
			"tags":        e["tags"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"issue_id": issueID,
		"count":    len(result),
		"events":   result,
	})
}
