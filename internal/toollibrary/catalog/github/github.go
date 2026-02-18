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
	ghToken     string
	ghClient    = &http.Client{Timeout: 15 * time.Second}
	ghAPIBase   = "https://api.github.com"
)

func initGitHub(token string) {
	ghToken = token
}

func ghRequest(method, path string, body io.Reader) (*http.Response, error) {
	url := ghAPIBase + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+ghToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return ghClient.Do(req)
}

func ghReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, v)
}

func parseRepo(repo string) (string, string, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("repo must be in owner/repo format")
	}
	return parts[0], parts[1], nil
}

func registerRoutes(r chi.Router) {
	r.Get("/repos", handleListRepos)
	r.Get("/issues", handleListIssues)
	r.Post("/issues", handleCreateIssue)
	r.Get("/pulls", handleListPulls)
}

func handleListRepos(w http.ResponseWriter, r *http.Request) {
	org := r.URL.Query().Get("org")
	if org == "" {
		writeError(w, http.StatusBadRequest, "org parameter is required")
		return
	}

	resp, err := ghRequest("GET", fmt.Sprintf("/orgs/%s/repos?per_page=100&sort=updated", org), nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "GitHub API request failed")
		return
	}

	if resp.StatusCode == 404 {
		resp.Body.Close()
		resp, err = ghRequest("GET", fmt.Sprintf("/users/%s/repos?per_page=100&sort=updated", org), nil)
		if err != nil {
			writeError(w, http.StatusBadGateway, "GitHub API request failed")
			return
		}
	}

	var repos []map[string]interface{}
	if err := ghReadJSON(resp, &repos); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(repos))
	for _, repo := range repos {
		result = append(result, map[string]interface{}{
			"name":        repo["name"],
			"full_name":   repo["full_name"],
			"description": repo["description"],
			"private":     repo["private"],
			"html_url":    repo["html_url"],
			"language":    repo["language"],
			"updated_at":  repo["updated_at"],
			"stars":       repo["stargazers_count"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"org":   org,
		"count": len(result),
		"repos": result,
	})
}

func handleListIssues(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	if repo == "" {
		writeError(w, http.StatusBadRequest, "repo parameter is required (owner/repo)")
		return
	}

	owner, name, err := parseRepo(repo)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}

	resp, err := ghRequest("GET", fmt.Sprintf("/repos/%s/%s/issues?state=%s&per_page=30", owner, name, state), nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "GitHub API request failed")
		return
	}

	var issues []map[string]interface{}
	if err := ghReadJSON(resp, &issues); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(issues))
	for _, issue := range issues {
		if issue["pull_request"] != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"number":     issue["number"],
			"title":      issue["title"],
			"state":      issue["state"],
			"user":       issue["user"].(map[string]interface{})["login"],
			"created_at": issue["created_at"],
			"updated_at": issue["updated_at"],
			"html_url":   issue["html_url"],
			"labels":     issue["labels"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"repo":   repo,
		"state":  state,
		"count":  len(result),
		"issues": result,
	})
}

type createIssueRequest struct {
	Repo  string `json:"repo"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	var req createIssueRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Repo == "" || req.Title == "" {
		writeError(w, http.StatusBadRequest, "repo and title are required")
		return
	}

	owner, name, err := parseRepo(req.Repo)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"title": req.Title,
		"body":  req.Body,
	})

	resp, err := ghRequest("POST", fmt.Sprintf("/repos/%s/%s/issues", owner, name), strings.NewReader(string(payload)))
	if err != nil {
		writeError(w, http.StatusBadGateway, "GitHub API request failed")
		return
	}

	var issue map[string]interface{}
	if err := ghReadJSON(resp, &issue); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"number":   issue["number"],
		"title":    issue["title"],
		"html_url": issue["html_url"],
		"state":    issue["state"],
	})
}

func handleListPulls(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	if repo == "" {
		writeError(w, http.StatusBadRequest, "repo parameter is required (owner/repo)")
		return
	}

	owner, name, err := parseRepo(repo)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := ghRequest("GET", fmt.Sprintf("/repos/%s/%s/pulls?per_page=30&state=open", owner, name), nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "GitHub API request failed")
		return
	}

	var pulls []map[string]interface{}
	if err := ghReadJSON(resp, &pulls); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(pulls))
	for _, pr := range pulls {
		result = append(result, map[string]interface{}{
			"number":     pr["number"],
			"title":      pr["title"],
			"state":      pr["state"],
			"user":       pr["user"].(map[string]interface{})["login"],
			"created_at": pr["created_at"],
			"updated_at": pr["updated_at"],
			"html_url":   pr["html_url"],
			"draft":      pr["draft"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"repo":          repo,
		"count":         len(result),
		"pull_requests": result,
	})
}
