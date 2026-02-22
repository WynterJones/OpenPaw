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

var dockerHubClient = &http.Client{Timeout: 20 * time.Second}

func registerRoutes(r chi.Router) {
	r.Get("/search/repositories", handleDockerHubSearch)
	r.Get("/repositories/{namespace}/{repo}/tags", handleDockerHubTags)
}

func handleDockerHubSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	pageSize := parseDockerHubInt(r.URL.Query().Get("page_size"), 20, 1, 100)
	page := parseDockerHubInt(r.URL.Query().Get("page"), 1, 1, 1000)

	params := url.Values{}
	params.Set("query", q)
	params.Set("page", strconv.Itoa(page))
	params.Set("page_size", strconv.Itoa(pageSize))

	data, err := dockerHubGET("https://hub.docker.com/v2/search/repositories", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleDockerHubTags(w http.ResponseWriter, r *http.Request) {
	namespace := strings.TrimSpace(chi.URLParam(r, "namespace"))
	repo := strings.TrimSpace(chi.URLParam(r, "repo"))
	if namespace == "" || repo == "" {
		writeError(w, http.StatusBadRequest, "namespace and repo are required")
		return
	}

	pageSize := parseDockerHubInt(r.URL.Query().Get("page_size"), 20, 1, 100)
	page := parseDockerHubInt(r.URL.Query().Get("page"), 1, 1, 1000)
	params := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(pageSize)}}

	base := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/%s/tags", url.PathEscape(namespace), url.PathEscape(repo))
	data, err := dockerHubGET(base, params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func dockerHubGET(baseURL string, params url.Values) (map[string]interface{}, error) {
	u := baseURL
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	resp, err := dockerHubClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("Docker Hub request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Docker Hub response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Docker Hub API error (%d): %s", resp.StatusCode, string(body))
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Docker Hub response: %w", err)
	}
	return data, nil
}

func parseDockerHubInt(raw string, fallback, min, max int) int {
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
