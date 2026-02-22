package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	tmdbKey    string
	tmdbClient = &http.Client{Timeout: 15 * time.Second}
	tmdbBase   = "https://api.themoviedb.org/3"
)

func initTMDB(key string) {
	tmdbKey = key
}

func tmdbGet(path string) (*http.Response, error) {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	u := tmdbBase + path + separator + "api_key=" + tmdbKey

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	return tmdbClient.Do(req)
}

func tmdbReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("TMDB API error (%d): %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, v)
}

func registerRoutes(r chi.Router) {
	r.Get("/search", handleSearch)
	r.Get("/movie", handleGetMovie)
	r.Get("/trending", handleTrending)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter is required")
		return
	}

	searchType := "movie"
	if t := r.URL.Query().Get("type"); t == "tv" {
		searchType = "tv"
	}

	path := fmt.Sprintf("/search/%s?query=%s", searchType, url.QueryEscape(query))

	resp, err := tmdbGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "TMDB API request failed")
		return
	}

	var result map[string]interface{}
	if err := tmdbReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	totalResults := 0
	if tr, ok := result["total_results"].(float64); ok {
		totalResults = int(tr)
	}

	items, _ := result["results"].([]interface{})
	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		cleaned := map[string]interface{}{
			"id":           entry["id"],
			"overview":     entry["overview"],
			"vote_average": entry["vote_average"],
			"poster_path":  entry["poster_path"],
		}

		// Movie uses "title" and "release_date", TV uses "name" and "first_air_date"
		if searchType == "movie" {
			cleaned["title"] = entry["title"]
			cleaned["release_date"] = entry["release_date"]
		} else {
			cleaned["title"] = entry["name"]
			cleaned["release_date"] = entry["first_air_date"]
		}

		results = append(results, cleaned)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"type":          searchType,
		"total_results": totalResults,
		"results":       results,
	})
}

func handleGetMovie(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id parameter is required")
		return
	}

	path := fmt.Sprintf("/movie/%s", id)

	resp, err := tmdbGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "TMDB API request failed")
		return
	}

	var result map[string]interface{}
	if err := tmdbReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Extract genres as a list of names
	genreNames := make([]string, 0)
	if genres, ok := result["genres"].([]interface{}); ok {
		for _, g := range genres {
			if genre, ok := g.(map[string]interface{}); ok {
				if name, ok := genre["name"].(string); ok {
					genreNames = append(genreNames, name)
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":            result["id"],
		"title":         result["title"],
		"overview":      result["overview"],
		"release_date":  result["release_date"],
		"runtime":       result["runtime"],
		"genres":        genreNames,
		"vote_average":  result["vote_average"],
		"vote_count":    result["vote_count"],
		"budget":        result["budget"],
		"revenue":       result["revenue"],
		"poster_path":   result["poster_path"],
		"backdrop_path": result["backdrop_path"],
		"tagline":       result["tagline"],
		"status":        result["status"],
	})
}

func handleTrending(w http.ResponseWriter, r *http.Request) {
	mediaType := "movie"
	if t := r.URL.Query().Get("type"); t == "tv" {
		mediaType = "tv"
	}

	window := "week"
	if win := r.URL.Query().Get("window"); win == "day" {
		window = "day"
	}

	path := fmt.Sprintf("/trending/%s/%s", mediaType, window)

	resp, err := tmdbGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "TMDB API request failed")
		return
	}

	var result map[string]interface{}
	if err := tmdbReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	items, _ := result["results"].([]interface{})
	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		cleaned := map[string]interface{}{
			"id":           entry["id"],
			"overview":     entry["overview"],
			"vote_average": entry["vote_average"],
			"poster_path":  entry["poster_path"],
		}

		if mediaType == "movie" {
			cleaned["title"] = entry["title"]
			cleaned["release_date"] = entry["release_date"]
		} else {
			cleaned["title"] = entry["name"]
			cleaned["release_date"] = entry["first_air_date"]
		}

		results = append(results, cleaned)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"type":    mediaType,
		"window":  window,
		"results": results,
	})
}
