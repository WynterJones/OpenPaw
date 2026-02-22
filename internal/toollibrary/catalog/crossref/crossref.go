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

var crossrefClient = &http.Client{Timeout: 20 * time.Second}

func registerRoutes(r chi.Router) {
	r.Get("/works", handleCrossrefWorks)
	r.Get("/works/{doi}", handleCrossrefDOI)
}

func handleCrossrefWorks(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	rows := parseCrossrefInt(r.URL.Query().Get("rows"), 10, 1, 50)

	params := url.Values{}
	params.Set("query", q)
	params.Set("rows", strconv.Itoa(rows))
	params.Set("sort", "relevance")

	data, err := crossrefGET("https://api.crossref.org/works", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	message, _ := data["message"].(map[string]interface{})
	items, _ := message["items"].([]interface{})

	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		work, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		titles, _ := work["title"].([]interface{})
		title := ""
		if len(titles) > 0 {
			title, _ = titles[0].(string)
		}
		results = append(results, map[string]interface{}{
			"doi":              work["DOI"],
			"title":            title,
			"publisher":        work["publisher"],
			"published_print":  work["published-print"],
			"published_online": work["published-online"],
			"type":             work["type"],
			"url":              work["URL"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   q,
		"count":   len(results),
		"results": results,
	})
}

func handleCrossrefDOI(w http.ResponseWriter, r *http.Request) {
	doi := strings.TrimSpace(chi.URLParam(r, "doi"))
	if doi == "" {
		writeError(w, http.StatusBadRequest, "doi is required")
		return
	}

	escapedDOI := url.PathEscape(doi)
	data, err := crossrefGET("https://api.crossref.org/works/"+escapedDOI, nil)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			writeError(w, http.StatusNotFound, "DOI not found")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	message, _ := data["message"].(map[string]interface{})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"doi":              message["DOI"],
		"title":            message["title"],
		"publisher":        message["publisher"],
		"container_title":  message["container-title"],
		"published_print":  message["published-print"],
		"published_online": message["published-online"],
		"reference_count":  message["reference-count"],
		"is_referenced_by": message["is-referenced-by-count"],
		"author":           message["author"],
		"url":              message["URL"],
	})
}

func crossrefGET(baseURL string, params url.Values) (map[string]interface{}, error) {
	u := baseURL
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "OpenPaw/1.0 (mailto:support@openpaw.local)")

	resp, err := crossrefClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Crossref request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Crossref response: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Crossref response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Crossref API error (%d): %s", resp.StatusCode, string(body))
	}
	return data, nil
}

func parseCrossrefInt(raw string, fallback, min, max int) int {
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
