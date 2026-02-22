package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	gitlabToken  string
	gitlabBase   string
	gitlabClient = &http.Client{Timeout: 20 * time.Second}
)

func initGitLab(token, baseURL string) {
	gitlabToken = token
	gitlabBase = strings.TrimSuffix(baseURL, "/")
}

func registerRoutes(r chi.Router) {
	r.Get("/projects", handleGitLabProjects)
	r.Get("/projects/{id}/issues", handleGitLabIssues)
	r.Get("/projects/{id}/merge-requests", handleGitLabMergeRequests)
	r.Get("/projects/{id}/pipelines", handleGitLabPipelines)
}

func handleGitLabProjects(w http.ResponseWriter, r *http.Request) {
	perPage := parseInt(r.URL.Query().Get("per_page"), 20, 1, 100)
	params := url.Values{"membership": {"true"}, "simple": {"true"}, "per_page": {strconv.Itoa(perPage)}}
	if search := strings.TrimSpace(r.URL.Query().Get("search")); search != "" {
		params.Set("search", search)
	}
	data, err := gitlabGET("/projects", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleGitLabIssues(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	state := defaultStr(r.URL.Query().Get("state"), "opened")
	perPage := parseInt(r.URL.Query().Get("per_page"), 20, 1, 100)
	params := url.Values{"state": {state}, "per_page": {strconv.Itoa(perPage)}}
	if labels := strings.TrimSpace(r.URL.Query().Get("labels")); labels != "" {
		params.Set("labels", labels)
	}
	data, err := gitlabGET("/projects/"+url.PathEscape(projectID)+"/issues", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleGitLabMergeRequests(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	state := defaultStr(r.URL.Query().Get("state"), "opened")
	perPage := parseInt(r.URL.Query().Get("per_page"), 20, 1, 100)
	params := url.Values{"state": {state}, "per_page": {strconv.Itoa(perPage)}}
	data, err := gitlabGET("/projects/"+url.PathEscape(projectID)+"/merge_requests", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleGitLabPipelines(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	perPage := parseInt(r.URL.Query().Get("per_page"), 20, 1, 100)
	params := url.Values{"per_page": {strconv.Itoa(perPage)}}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		params.Set("status", status)
	}
	data, err := gitlabGET("/projects/"+url.PathEscape(projectID)+"/pipelines", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func gitlabGET(path string, params url.Values) (interface{}, error) {
	u := gitlabBase + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", gitlabToken)
	req.Header.Set("Accept", "application/json")

	resp, err := gitlabClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitLab request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read GitLab response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GitLab API error (%d): %s", resp.StatusCode, string(body))
	}
	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse GitLab response: %w", err)
	}
	return data, nil
}

func parseInt(raw string, fallback, min, max int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func defaultStr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
