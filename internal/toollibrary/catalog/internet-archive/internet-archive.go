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

var archiveClient = &http.Client{Timeout: 20 * time.Second}

func registerRoutes(r chi.Router) {
	r.Get("/check", handleCheck)
	r.Get("/snapshots", handleSnapshots)
}

func handleCheck(w http.ResponseWriter, r *http.Request) {
	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetURL == "" {
		writeError(w, http.StatusBadRequest, "url parameter is required")
		return
	}

	apiURL := fmt.Sprintf(
		"https://archive.org/wayback/available?url=%s",
		url.QueryEscape(targetURL),
	)

	resp, err := archiveClient.Get(apiURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Wayback Machine API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read response")
		return
	}

	if resp.StatusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Wayback Machine API error (%d)", resp.StatusCode))
		return
	}

	var parsed struct {
		URL              string `json:"url"`
		ArchivedSnapshots struct {
			Closest struct {
				Status    string `json:"status"`
				Available bool   `json:"available"`
				URL       string `json:"url"`
				Timestamp string `json:"timestamp"`
			} `json:"closest"`
		} `json:"archived_snapshots"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}

	closest := parsed.ArchivedSnapshots.Closest

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"url":          targetURL,
		"available":    closest.Available,
		"snapshot_url": closest.URL,
		"timestamp":    closest.Timestamp,
		"status":       closest.Status,
	})
}

func handleSnapshots(w http.ResponseWriter, r *http.Request) {
	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetURL == "" {
		writeError(w, http.StatusBadRequest, "url parameter is required")
		return
	}

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}

	apiURL := fmt.Sprintf(
		"https://web.archive.org/cdx/search/cdx?url=%s&output=json&limit=%d",
		url.QueryEscape(targetURL),
		limit,
	)

	resp, err := archiveClient.Get(apiURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "CDX API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read response")
		return
	}

	if resp.StatusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("CDX API error (%d)", resp.StatusCode))
		return
	}

	// CDX API returns a JSON array of arrays. First row is headers.
	var rows [][]string
	if err := json.Unmarshal(body, &rows); err != nil {
		// CDX may return empty or non-JSON for no results
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"url":       targetURL,
			"count":     0,
			"snapshots": []interface{}{},
		})
		return
	}

	if len(rows) < 2 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"url":       targetURL,
			"count":     0,
			"snapshots": []interface{}{},
		})
		return
	}

	// First row is headers: urlkey, timestamp, original, mimetype, statuscode, digest, length
	headers := rows[0]
	snapshots := make([]map[string]string, 0, len(rows)-1)

	for _, row := range rows[1:] {
		snapshot := make(map[string]string)
		for i, header := range headers {
			if i < len(row) {
				snapshot[header] = row[i]
			}
		}
		// Add a convenience URL to view the snapshot
		if ts, ok := snapshot["timestamp"]; ok {
			if orig, ok2 := snapshot["original"]; ok2 {
				snapshot["view_url"] = fmt.Sprintf("https://web.archive.org/web/%s/%s", ts, orig)
			}
		}
		snapshots = append(snapshots, snapshot)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"url":       targetURL,
		"count":     len(snapshots),
		"snapshots": snapshots,
	})
}
