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

var openLibraryClient = &http.Client{Timeout: 15 * time.Second}

func registerRoutes(r chi.Router) {
	r.Get("/search", handleOpenLibrarySearch)
	r.Get("/work/{id}", handleOpenLibraryWork)
	r.Get("/editions", handleOpenLibraryEditions)
}

func handleOpenLibrarySearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	limit := parseBoundedInt(r.URL.Query().Get("limit"), 10, 1, 50)

	params := url.Values{}
	params.Set("q", q)
	params.Set("limit", strconv.Itoa(limit))

	data, err := openLibraryGet("/search.json", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	docs, _ := data["docs"].([]interface{})
	books := make([]map[string]interface{}, 0, len(docs))
	for _, item := range docs {
		doc, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		books = append(books, map[string]interface{}{
			"key":                doc["key"],
			"title":              doc["title"],
			"author_name":        doc["author_name"],
			"first_publish_year": doc["first_publish_year"],
			"edition_count":      doc["edition_count"],
			"isbn":               doc["isbn"],
			"cover_id":           doc["cover_i"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":         q,
		"num_found":     data["numFound"],
		"results_count": len(books),
		"results":       books,
	})
}

func handleOpenLibraryWork(w http.ResponseWriter, r *http.Request) {
	workID := normalizeWorkID(chi.URLParam(r, "id"))
	if workID == "" {
		writeError(w, http.StatusBadRequest, "work id is required")
		return
	}

	data, err := openLibraryGet("/works/"+workID+".json", nil)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			writeError(w, http.StatusNotFound, "work not found")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"key":              data["key"],
		"title":            data["title"],
		"description":      extractDescription(data["description"]),
		"subjects":         data["subjects"],
		"first_publish_at": data["first_publish_date"],
	})
}

func handleOpenLibraryEditions(w http.ResponseWriter, r *http.Request) {
	workID := normalizeWorkID(r.URL.Query().Get("work_id"))
	if workID == "" {
		writeError(w, http.StatusBadRequest, "work_id parameter is required")
		return
	}

	limit := parseBoundedInt(r.URL.Query().Get("limit"), 10, 1, 50)
	params := url.Values{}
	params.Set("limit", strconv.Itoa(limit))

	data, err := openLibraryGet("/works/"+workID+"/editions.json", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	entries, _ := data["entries"].([]interface{})
	editions := make([]map[string]interface{}, 0, len(entries))
	for _, item := range entries {
		edition, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		editions = append(editions, map[string]interface{}{
			"key":             edition["key"],
			"title":           edition["title"],
			"publish_date":    edition["publish_date"],
			"number_of_pages": edition["number_of_pages"],
			"publishers":      edition["publishers"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"work_id":       workID,
		"results_count": len(editions),
		"results":       editions,
	})
}

func openLibraryGet(path string, params url.Values) (map[string]interface{}, error) {
	u := "https://openlibrary.org" + path
	if params != nil && len(params) > 0 {
		u += "?" + params.Encode()
	}

	resp, err := openLibraryClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("Open Library request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Open Library response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Open Library error (%d): %s", resp.StatusCode, string(body))
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Open Library response: %w", err)
	}

	return data, nil
}

func normalizeWorkID(raw string) string {
	id := strings.TrimSpace(raw)
	id = strings.TrimPrefix(id, "/works/")
	id = strings.TrimSuffix(id, ".json")
	return id
}

func parseBoundedInt(raw string, fallback, min, max int) int {
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

func extractDescription(raw interface{}) interface{} {
	switch v := raw.(type) {
	case string:
		return v
	case map[string]interface{}:
		if text, ok := v["value"]; ok {
			return text
		}
	}
	return raw
}
