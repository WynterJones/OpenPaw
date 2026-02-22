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

var wikipediaClient = &http.Client{Timeout: 15 * time.Second}

type wikipediaSearchResponse struct {
	Query struct {
		Search []struct {
			Title     string `json:"title"`
			Snippet   string `json:"snippet"`
			PageID    int    `json:"pageid"`
			Size      int    `json:"size"`
			WordCount int    `json:"wordcount"`
			Timestamp string `json:"timestamp"`
		} `json:"search"`
	} `json:"query"`
}

type wikipediaSummaryResponse struct {
	Title       string `json:"title"`
	DisplayText string `json:"displaytitle"`
	Description string `json:"description"`
	Extract     string `json:"extract"`
	ContentURLs struct {
		Desktop struct {
			Page string `json:"page"`
		} `json:"desktop"`
	} `json:"content_urls"`
	Thumbnail struct {
		Source string `json:"source"`
	} `json:"thumbnail"`
}

func registerRoutes(r chi.Router) {
	r.Get("/search", handleWikipediaSearch)
	r.Get("/summary/{title}", handleWikipediaSummary)
}

func handleWikipediaSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n >= 1 && n <= 50 {
			limit = n
		}
	}

	u := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=query&list=search&format=json&utf8=1&srsearch=%s&srlimit=%d",
		url.QueryEscape(q),
		limit,
	)

	resp, err := wikipediaClient.Get(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Wikipedia API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read response")
		return
	}

	if resp.StatusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Wikipedia API error (%d)", resp.StatusCode))
		return
	}

	var parsed wikipediaSearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}

	results := make([]map[string]interface{}, 0, len(parsed.Query.Search))
	for _, item := range parsed.Query.Search {
		results = append(results, map[string]interface{}{
			"title":      item.Title,
			"snippet":    item.Snippet,
			"page_id":    item.PageID,
			"size":       item.Size,
			"word_count": item.WordCount,
			"timestamp":  item.Timestamp,
			"url":        "https://en.wikipedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(item.Title, " ", "_")),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   q,
		"count":   len(results),
		"results": results,
	})
}

func handleWikipediaSummary(w http.ResponseWriter, r *http.Request) {
	title := strings.TrimSpace(chi.URLParam(r, "title"))
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	u := "https://en.wikipedia.org/api/rest_v1/page/summary/" + url.PathEscape(title)
	resp, err := wikipediaClient.Get(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Wikipedia API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read response")
		return
	}

	if resp.StatusCode == http.StatusNotFound {
		writeError(w, http.StatusNotFound, "page not found")
		return
	}
	if resp.StatusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Wikipedia API error (%d)", resp.StatusCode))
		return
	}

	var parsed wikipediaSummaryResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"title":         parsed.Title,
		"display_title": parsed.DisplayText,
		"description":   parsed.Description,
		"extract":       parsed.Extract,
		"url":           parsed.ContentURLs.Desktop.Page,
		"thumbnail":     parsed.Thumbnail.Source,
	})
}
