package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var arxivClient = &http.Client{Timeout: 20 * time.Second}

type arxivFeed struct {
	Entries []arxivEntry `xml:"entry"`
}

type arxivEntry struct {
	ID        string        `xml:"id"`
	Title     string        `xml:"title"`
	Summary   string        `xml:"summary"`
	Published string        `xml:"published"`
	Updated   string        `xml:"updated"`
	Authors   []arxivAuthor `xml:"author"`
	Links     []arxivLink   `xml:"link"`
}

type arxivAuthor struct {
	Name string `xml:"name"`
}

type arxivLink struct {
	Href  string `xml:"href,attr"`
	Rel   string `xml:"rel,attr"`
	Type  string `xml:"type,attr"`
	Title string `xml:"title,attr"`
}

func registerRoutes(r chi.Router) {
	r.Get("/search", handleArxivSearch)
}

func handleArxivSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	start := parseIntWithBounds(r.URL.Query().Get("start"), 0, 0, 10000)
	maxResults := parseIntWithBounds(r.URL.Query().Get("max_results"), 10, 1, 50)

	params := url.Values{}
	params.Set("search_query", "all:"+q)
	params.Set("start", strconv.Itoa(start))
	params.Set("max_results", strconv.Itoa(maxResults))
	params.Set("sortBy", "relevance")
	params.Set("sortOrder", "descending")

	u := "https://export.arxiv.org/api/query?" + params.Encode()
	resp, err := arxivClient.Get(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, "arXiv API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read response")
		return
	}

	if resp.StatusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("arXiv API error (%d)", resp.StatusCode))
		return
	}

	var feed arxivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse arXiv response")
		return
	}

	results := make([]map[string]interface{}, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		authors := make([]string, 0, len(entry.Authors))
		for _, author := range entry.Authors {
			authors = append(authors, author.Name)
		}

		pdfURL := ""
		for _, link := range entry.Links {
			if strings.EqualFold(link.Title, "pdf") || link.Type == "application/pdf" {
				pdfURL = link.Href
				break
			}
		}

		results = append(results, map[string]interface{}{
			"id":           strings.TrimSpace(entry.ID),
			"title":        cleanWhitespace(entry.Title),
			"summary":      cleanWhitespace(entry.Summary),
			"published_at": entry.Published,
			"updated_at":   entry.Updated,
			"authors":      authors,
			"url":          strings.TrimSpace(entry.ID),
			"pdf_url":      pdfURL,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":       q,
		"start":       start,
		"max_results": maxResults,
		"count":       len(results),
		"results":     results,
	})
}

func parseIntWithBounds(raw string, fallback, min, max int) int {
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

func cleanWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
