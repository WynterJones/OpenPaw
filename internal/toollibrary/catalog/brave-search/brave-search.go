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
	braveKey    string
	braveBase   = "https://api.search.brave.com/res/v1"
	braveClient = &http.Client{Timeout: 20 * time.Second}
)

func initBraveSearch(key string) {
	braveKey = key
}

func registerRoutes(r chi.Router) {
	r.Get("/search", handleBraveWebSearch)
	r.Get("/news", handleBraveNewsSearch)
	r.Get("/images", handleBraveImageSearch)
}

func handleBraveWebSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	count := parseBoundedInt(r.URL.Query().Get("count"), 10, 1, 20)

	data, err := braveGET("/web/search", url.Values{"q": {q}, "count": {strconv.Itoa(count)}})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":          q,
		"web":            data["web"],
		"discussions":    data["discussions"],
		"videos":         data["videos"],
		"infobox":        data["infobox"],
		"mixed_response": data,
	})
}

func handleBraveNewsSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	count := parseBoundedInt(r.URL.Query().Get("count"), 10, 1, 20)

	data, err := braveGET("/news/search", url.Values{"q": {q}, "count": {strconv.Itoa(count)}})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":    q,
		"news":     data["results"],
		"raw":      data,
		"provider": "brave",
	})
}

func handleBraveImageSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	count := parseBoundedInt(r.URL.Query().Get("count"), 10, 1, 20)

	data, err := braveGET("/images/search", url.Values{"q": {q}, "count": {strconv.Itoa(count)}})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":  q,
		"images": data["results"],
		"raw":    data,
	})
}

func braveGET(path string, params url.Values) (map[string]interface{}, error) {
	req, err := http.NewRequest(http.MethodGet, braveBase+path+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", braveKey)

	resp, err := braveClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Brave Search request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Brave response: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Brave response: %w", err)
	}

	if resp.StatusCode >= 400 {
		msg, _ := data["error"].(string)
		if msg == "" {
			msg = string(body)
		}
		return nil, fmt.Errorf("Brave API error (%d): %s", resp.StatusCode, msg)
	}
	return data, nil
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
