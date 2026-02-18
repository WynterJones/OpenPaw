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
	netlifyToken  string
	netlifyBase   = "https://api.netlify.com/api/v1"
	netlifyClient = &http.Client{Timeout: 20 * time.Second}
)

func initNetlify(token string) {
	netlifyToken = token
}

func registerRoutes(r chi.Router) {
	r.Get("/sites", handleNetlifySites)
	r.Get("/sites/{site_id}/deploys", handleNetlifySiteDeploys)
	r.Get("/deploys/{deploy_id}", handleNetlifyDeploy)
}

func handleNetlifySites(w http.ResponseWriter, r *http.Request) {
	page := parseNetlifyInt(r.URL.Query().Get("page"), 1, 1, 1000)
	perPage := parseNetlifyInt(r.URL.Query().Get("per_page"), 20, 1, 100)
	params := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(perPage)}}
	data, err := netlifyGET("/sites", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleNetlifySiteDeploys(w http.ResponseWriter, r *http.Request) {
	siteID := strings.TrimSpace(chi.URLParam(r, "site_id"))
	if siteID == "" {
		writeError(w, http.StatusBadRequest, "site_id is required")
		return
	}
	page := parseNetlifyInt(r.URL.Query().Get("page"), 1, 1, 1000)
	perPage := parseNetlifyInt(r.URL.Query().Get("per_page"), 20, 1, 100)
	params := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(perPage)}}
	data, err := netlifyGET("/sites/"+url.PathEscape(siteID)+"/deploys", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleNetlifyDeploy(w http.ResponseWriter, r *http.Request) {
	deployID := strings.TrimSpace(chi.URLParam(r, "deploy_id"))
	if deployID == "" {
		writeError(w, http.StatusBadRequest, "deploy_id is required")
		return
	}
	data, err := netlifyGET("/deploys/"+url.PathEscape(deployID), nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func netlifyGET(path string, params url.Values) (interface{}, error) {
	u := netlifyBase + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+netlifyToken)
	req.Header.Set("Accept", "application/json")

	resp, err := netlifyClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Netlify request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Netlify response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Netlify API error (%d): %s", resp.StatusCode, string(body))
	}
	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Netlify response: %w", err)
	}
	return data, nil
}

func parseNetlifyInt(raw string, fallback, min, max int) int {
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
