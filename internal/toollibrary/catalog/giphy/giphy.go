package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	giphyKey    string
	giphyClient = &http.Client{Timeout: 15 * time.Second}
	giphyBase   = "https://api.giphy.com/v1/gifs"
)

func initGiphy(key string) {
	giphyKey = key
}

func giphyGet(path string) ([]byte, int, error) {
	u := giphyBase + path

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := giphyClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("Giphy API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}

	return body, resp.StatusCode, nil
}

func extractGIF(entry map[string]interface{}) map[string]interface{} {
	gif := map[string]interface{}{
		"id":    entry["id"],
		"title": entry["title"],
		"url":   entry["url"],
	}

	images, _ := entry["images"].(map[string]interface{})
	if images != nil {
		gifImages := map[string]interface{}{}

		if original, ok := images["original"].(map[string]interface{}); ok {
			gifImages["original"] = map[string]interface{}{
				"url":    original["url"],
				"width":  original["width"],
				"height": original["height"],
			}
		}

		if fixedHeight, ok := images["fixed_height"].(map[string]interface{}); ok {
			gifImages["fixed_height"] = map[string]interface{}{
				"url":    fixedHeight["url"],
				"width":  fixedHeight["width"],
				"height": fixedHeight["height"],
			}
		}

		gif["images"] = gifImages
	}

	return gif
}

func registerRoutes(r chi.Router) {
	r.Get("/search", handleSearch)
	r.Get("/trending", handleTrending)
	r.Get("/random", handleRandom)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter is required")
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 25 {
			limit = n
		}
	}

	path := fmt.Sprintf("/search?api_key=%s&q=%s&limit=%d",
		giphyKey, url.QueryEscape(query), limit)

	body, statusCode, err := giphyGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if statusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Giphy API error (%d): %s", statusCode, string(body)))
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}

	data, _ := result["data"].([]interface{})
	gifs := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		gifs = append(gifs, extractGIF(entry))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(gifs),
		"gifs":  gifs,
	})
}

func handleTrending(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 25 {
			limit = n
		}
	}

	path := fmt.Sprintf("/trending?api_key=%s&limit=%d", giphyKey, limit)

	body, statusCode, err := giphyGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if statusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Giphy API error (%d): %s", statusCode, string(body)))
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}

	data, _ := result["data"].([]interface{})
	gifs := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		gifs = append(gifs, extractGIF(entry))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(gifs),
		"gifs":  gifs,
	})
}

func handleRandom(w http.ResponseWriter, r *http.Request) {
	path := fmt.Sprintf("/random?api_key=%s", giphyKey)

	tag := r.URL.Query().Get("tag")
	if tag != "" {
		path += "&tag=" + url.QueryEscape(tag)
	}

	body, statusCode, err := giphyGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if statusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Giphy API error (%d): %s", statusCode, string(body)))
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		writeError(w, http.StatusBadGateway, "unexpected response format")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"gif": extractGIF(data),
	})
}
