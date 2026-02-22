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
	serpAPIKey    string
	searchClient = &http.Client{Timeout: 15 * time.Second}
	serpAPIBase   = "https://serpapi.com/search.json"
)

func initSearch(key string) {
	serpAPIKey = key
}

func serpRequest(params url.Values) (map[string]interface{}, error) {
	params.Set("api_key", serpAPIKey)
	params.Set("engine", "google")

	u := serpAPIBase + "?" + params.Encode()
	resp, err := searchClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("SerpAPI request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read SerpAPI response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("SerpAPI error (%d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse SerpAPI response: %w", err)
	}

	return result, nil
}

func registerRoutes(r chi.Router) {
	r.Get("/search", handleWebSearch)
	r.Get("/news", handleNewsSearch)
	r.Get("/images", handleImageSearch)
}

func handleWebSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	num := 10
	if numStr := r.URL.Query().Get("num"); numStr != "" {
		if n, err := strconv.Atoi(numStr); err == nil && n >= 1 && n <= 100 {
			num = n
		}
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("num", strconv.Itoa(num))

	data, err := serpRequest(params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	organic, _ := data["organic_results"].([]interface{})
	results := make([]map[string]interface{}, 0, len(organic))
	for _, item := range organic {
		r, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, map[string]interface{}{
			"title":    r["title"],
			"link":     r["link"],
			"snippet":  r["snippet"],
			"position": r["position"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   q,
		"count":   len(results),
		"results": results,
	})
}

func handleNewsSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	num := 10
	if numStr := r.URL.Query().Get("num"); numStr != "" {
		if n, err := strconv.Atoi(numStr); err == nil && n >= 1 && n <= 100 {
			num = n
		}
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("num", strconv.Itoa(num))
	params.Set("tbm", "nws")

	data, err := serpRequest(params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	news, _ := data["news_results"].([]interface{})
	results := make([]map[string]interface{}, 0, len(news))
	for _, item := range news {
		r, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, map[string]interface{}{
			"title":   r["title"],
			"link":    r["link"],
			"snippet": r["snippet"],
			"source":  r["source"],
			"date":    r["date"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   q,
		"count":   len(results),
		"results": results,
	})
}

func handleImageSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	num := 10
	if numStr := r.URL.Query().Get("num"); numStr != "" {
		if n, err := strconv.Atoi(numStr); err == nil && n >= 1 && n <= 100 {
			num = n
		}
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("num", strconv.Itoa(num))
	params.Set("tbm", "isch")

	data, err := serpRequest(params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	images, _ := data["images_results"].([]interface{})
	results := make([]map[string]interface{}, 0, len(images))
	for _, item := range images {
		r, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, map[string]interface{}{
			"title":     r["title"],
			"original":  r["original"],
			"thumbnail": r["thumbnail"],
			"source":    r["source"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   q,
		"count":   len(results),
		"results": results,
	})
}
