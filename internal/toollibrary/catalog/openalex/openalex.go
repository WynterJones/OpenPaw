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

var openAlexClient = &http.Client{Timeout: 20 * time.Second}

func registerRoutes(r chi.Router) {
	r.Get("/works", handleOpenAlexWorks)
	r.Get("/authors", handleOpenAlexAuthors)
	r.Get("/institutions", handleOpenAlexInstitutions)
}

func handleOpenAlexWorks(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	perPage := parseOpenAlexInt(r.URL.Query().Get("per_page"), 10, 1, 100)

	params := url.Values{}
	params.Set("search", q)
	params.Set("per-page", strconv.Itoa(perPage))

	data, err := openAlexGET("https://api.openalex.org/works", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   q,
		"meta":    data["meta"],
		"results": compactOpenAlexResults(data["results"]),
	})
}

func handleOpenAlexAuthors(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	perPage := parseOpenAlexInt(r.URL.Query().Get("per_page"), 10, 1, 100)

	params := url.Values{}
	params.Set("search", q)
	params.Set("per-page", strconv.Itoa(perPage))

	data, err := openAlexGET("https://api.openalex.org/authors", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   q,
		"meta":    data["meta"],
		"results": data["results"],
	})
}

func handleOpenAlexInstitutions(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	perPage := parseOpenAlexInt(r.URL.Query().Get("per_page"), 10, 1, 100)

	params := url.Values{}
	params.Set("search", q)
	params.Set("per-page", strconv.Itoa(perPage))

	data, err := openAlexGET("https://api.openalex.org/institutions", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   q,
		"meta":    data["meta"],
		"results": data["results"],
	})
}

func openAlexGET(baseURL string, params url.Values) (map[string]interface{}, error) {
	u := baseURL + "?" + params.Encode()
	resp, err := openAlexClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("OpenAlex request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read OpenAlex response: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse OpenAlex response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("OpenAlex API error (%d): %s", resp.StatusCode, string(body))
	}
	return data, nil
}

func compactOpenAlexResults(raw interface{}) []map[string]interface{} {
	items, _ := raw.([]interface{})
	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		work, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, map[string]interface{}{
			"id":               work["id"],
			"doi":              work["doi"],
			"display_name":     work["display_name"],
			"publication_year": work["publication_year"],
			"cited_by_count":   work["cited_by_count"],
			"type":             work["type"],
			"primary_location": work["primary_location"],
		})
	}
	return results
}

func parseOpenAlexInt(raw string, fallback, min, max int) int {
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
