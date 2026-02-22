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
	vercelToken  string
	vercelBase   = "https://api.vercel.com"
	vercelClient = &http.Client{Timeout: 20 * time.Second}
)

func initVercel(token string) {
	vercelToken = token
}

func registerRoutes(r chi.Router) {
	r.Get("/projects", handleVercelProjects)
	r.Get("/deployments", handleVercelDeployments)
}

func handleVercelProjects(w http.ResponseWriter, r *http.Request) {
	limit := parseVercelInt(r.URL.Query().Get("limit"), 20, 1, 100)
	params := url.Values{"limit": {strconv.Itoa(limit)}}
	if search := strings.TrimSpace(r.URL.Query().Get("search")); search != "" {
		params.Set("search", search)
	}
	data, err := vercelGET("/v9/projects", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleVercelDeployments(w http.ResponseWriter, r *http.Request) {
	limit := parseVercelInt(r.URL.Query().Get("limit"), 20, 1, 100)
	params := url.Values{"limit": {strconv.Itoa(limit)}}
	if projectID := strings.TrimSpace(r.URL.Query().Get("project_id")); projectID != "" {
		params.Set("projectId", projectID)
	}
	if target := strings.TrimSpace(r.URL.Query().Get("target")); target != "" {
		params.Set("target", target)
	}
	data, err := vercelGET("/v6/deployments", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func vercelGET(path string, params url.Values) (map[string]interface{}, error) {
	u := vercelBase + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+vercelToken)
	req.Header.Set("Accept", "application/json")

	resp, err := vercelClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Vercel request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Vercel response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Vercel API error (%d): %s", resp.StatusCode, string(body))
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Vercel response: %w", err)
	}
	return data, nil
}

func parseVercelInt(raw string, fallback, min, max int) int {
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
