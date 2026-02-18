package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	unsplashKey    string
	unsplashClient = &http.Client{Timeout: 15 * time.Second}
	unsplashBase   = "https://api.unsplash.com"
)

func initUnsplash(key string) {
	unsplashKey = key
}

func unsplashGet(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", unsplashBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Client-ID "+unsplashKey)
	req.Header.Set("Accept-Version", "v1")
	return unsplashClient.Do(req)
}

func unsplashReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Unsplash API error (%d): %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, v)
}

func registerRoutes(r chi.Router) {
	r.Get("/search", handleSearchPhotos)
	r.Get("/photo/{id}", handleGetPhoto)
	r.Get("/random", handleRandomPhotos)
	r.Get("/download/{id}", handleDownloadPhoto)
}

func handleSearchPhotos(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter is required")
		return
	}

	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n >= 1 {
			page = n
		}
	}

	perPage := 10
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if n, err := strconv.Atoi(pp); err == nil && n >= 1 && n <= 30 {
			perPage = n
		}
	}

	path := fmt.Sprintf("/search/photos?query=%s&page=%d&per_page=%d", query, page, perPage)
	if orientation := r.URL.Query().Get("orientation"); orientation != "" {
		path += "&orientation=" + orientation
	}

	resp, err := unsplashGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Unsplash API request failed")
		return
	}

	var searchResult map[string]interface{}
	if err := unsplashReadJSON(resp, &searchResult); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	total, _ := searchResult["total"].(float64)
	totalPages, _ := searchResult["total_pages"].(float64)
	rawResults, _ := searchResult["results"].([]interface{})

	photos := make([]map[string]interface{}, 0, len(rawResults))
	for _, item := range rawResults {
		photo, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		photos = append(photos, extractPhoto(photo))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total":       int(total),
		"total_pages": int(totalPages),
		"results":     photos,
	})
}

func handleGetPhoto(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "photo id is required")
		return
	}

	resp, err := unsplashGet("/photos/" + id)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Unsplash API request failed")
		return
	}

	var photo map[string]interface{}
	if err := unsplashReadJSON(resp, &photo); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	result := extractPhoto(photo)
	result["downloads"] = photo["downloads"]
	result["exif"] = photo["exif"]
	result["location"] = photo["location"]

	writeJSON(w, http.StatusOK, result)
}

func handleRandomPhotos(w http.ResponseWriter, r *http.Request) {
	count := 1
	if c := r.URL.Query().Get("count"); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n >= 1 && n <= 30 {
			count = n
		}
	}

	path := fmt.Sprintf("/photos/random?count=%d", count)
	if query := r.URL.Query().Get("query"); query != "" {
		path += "&query=" + query
	}
	if orientation := r.URL.Query().Get("orientation"); orientation != "" {
		path += "&orientation=" + orientation
	}

	resp, err := unsplashGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Unsplash API request failed")
		return
	}

	var rawPhotos []map[string]interface{}
	if err := unsplashReadJSON(resp, &rawPhotos); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	photos := make([]map[string]interface{}, 0, len(rawPhotos))
	for _, photo := range rawPhotos {
		photos = append(photos, extractPhoto(photo))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":  len(photos),
		"photos": photos,
	})
}

func handleDownloadPhoto(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "photo id is required")
		return
	}

	resp, err := unsplashGet("/photos/" + id + "/download")
	if err != nil {
		writeError(w, http.StatusBadGateway, "Unsplash API request failed")
		return
	}

	var result map[string]interface{}
	if err := unsplashReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"url": result["url"],
	})
}

func extractPhoto(photo map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{
		"id":              photo["id"],
		"description":     photo["description"],
		"alt_description": photo["alt_description"],
		"width":           photo["width"],
		"height":          photo["height"],
		"likes":           photo["likes"],
		"urls":            photo["urls"],
	}

	if user, ok := photo["user"].(map[string]interface{}); ok {
		result["user"] = map[string]interface{}{
			"name":     user["name"],
			"username": user["username"],
		}
	}

	return result
}
