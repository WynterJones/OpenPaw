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

var semanticScholarClient = &http.Client{Timeout: 20 * time.Second}

func registerRoutes(r chi.Router) {
	r.Get("/papers/search", handleSemanticScholarSearch)
	r.Get("/paper/{paper_id}", handleSemanticScholarPaper)
}

func handleSemanticScholarSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	limit := parseSemanticInt(r.URL.Query().Get("limit"), 10, 1, 100)
	offset := parseSemanticInt(r.URL.Query().Get("offset"), 0, 0, 1000)

	params := url.Values{}
	params.Set("query", q)
	params.Set("limit", strconv.Itoa(limit))
	params.Set("offset", strconv.Itoa(offset))
	params.Set("fields", "title,abstract,year,authors,citationCount,externalIds,url,venue")

	data, err := semanticScholarGET("https://api.semanticscholar.org/graph/v1/paper/search", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	items, _ := data["data"].([]interface{})
	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		paper, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, map[string]interface{}{
			"paper_id":       paper["paperId"],
			"title":          paper["title"],
			"abstract":       paper["abstract"],
			"year":           paper["year"],
			"venue":          paper["venue"],
			"citation_count": paper["citationCount"],
			"authors":        compactSemanticAuthors(paper["authors"]),
			"external_ids":   paper["externalIds"],
			"url":            paper["url"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   q,
		"total":   data["total"],
		"offset":  offset,
		"limit":   limit,
		"results": results,
	})
}

func handleSemanticScholarPaper(w http.ResponseWriter, r *http.Request) {
	paperID := strings.TrimSpace(chi.URLParam(r, "paper_id"))
	if paperID == "" {
		writeError(w, http.StatusBadRequest, "paper_id is required")
		return
	}

	params := url.Values{}
	params.Set("fields", "title,abstract,year,authors,citationCount,referenceCount,externalIds,url,venue,influentialCitationCount")

	u := "https://api.semanticscholar.org/graph/v1/paper/" + url.PathEscape(paperID)
	data, err := semanticScholarGET(u, params)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			writeError(w, http.StatusNotFound, "paper not found")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"paper_id":                   data["paperId"],
		"title":                      data["title"],
		"abstract":                   data["abstract"],
		"year":                       data["year"],
		"venue":                      data["venue"],
		"citation_count":             data["citationCount"],
		"influential_citation_count": data["influentialCitationCount"],
		"reference_count":            data["referenceCount"],
		"authors":                    compactSemanticAuthors(data["authors"]),
		"external_ids":               data["externalIds"],
		"url":                        data["url"],
	})
}

func semanticScholarGET(baseURL string, params url.Values) (map[string]interface{}, error) {
	u := baseURL
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := semanticScholarClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Semantic Scholar request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Semantic Scholar response: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Semantic Scholar response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Semantic Scholar API error (%d): %s", resp.StatusCode, string(body))
	}
	return data, nil
}

func compactSemanticAuthors(raw interface{}) []map[string]interface{} {
	items, _ := raw.([]interface{})
	authors := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		author, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		authors = append(authors, map[string]interface{}{
			"author_id": author["authorId"],
			"name":      author["name"],
		})
	}
	return authors
}

func parseSemanticInt(raw string, fallback, min, max int) int {
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
