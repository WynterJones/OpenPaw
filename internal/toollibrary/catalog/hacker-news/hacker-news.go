package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

var hnClient = &http.Client{Timeout: 15 * time.Second}

const hnBaseURL = "https://hacker-news.firebaseio.com/v0"

func registerRoutes(r chi.Router) {
	r.Get("/top", handleTopStories)
	r.Get("/item", handleItem)
	r.Get("/user", handleUser)
}

func hnFetch(path string) ([]byte, error) {
	resp, err := hnClient.Get(hnBaseURL + path)
	if err != nil {
		return nil, fmt.Errorf("HN API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HN API error (%d)", resp.StatusCode)
	}

	return body, nil
}

func handleTopStories(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n >= 1 && n <= 30 {
			limit = n
		}
	}

	body, err := hnFetch("/topstories.json")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	var ids []int
	if err := json.Unmarshal(body, &ids); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse story IDs")
		return
	}

	if len(ids) > limit {
		ids = ids[:limit]
	}

	type storyResult struct {
		index int
		data  map[string]interface{}
		err   error
	}

	results := make([]storyResult, len(ids))
	var wg sync.WaitGroup

	for i, id := range ids {
		wg.Add(1)
		go func(idx, storyID int) {
			defer wg.Done()
			itemBody, err := hnFetch(fmt.Sprintf("/item/%d.json", storyID))
			if err != nil {
				results[idx] = storyResult{index: idx, err: err}
				return
			}

			var item map[string]interface{}
			if err := json.Unmarshal(itemBody, &item); err != nil {
				results[idx] = storyResult{index: idx, err: err}
				return
			}

			story := map[string]interface{}{
				"id":          item["id"],
				"title":       item["title"],
				"url":         item["url"],
				"score":       item["score"],
				"by":          item["by"],
				"time":        item["time"],
				"descendants": item["descendants"],
			}
			results[idx] = storyResult{index: idx, data: story}
		}(i, id)
	}

	wg.Wait()

	stories := make([]map[string]interface{}, 0, len(ids))
	for _, res := range results {
		if res.err == nil && res.data != nil {
			stories = append(stories, res.data)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":   len(stories),
		"stories": stories,
	})
}

func handleItem(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSpace(r.URL.Query().Get("id"))
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "id parameter is required")
		return
	}

	if _, err := strconv.Atoi(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "id must be a valid integer")
		return
	}

	body, err := hnFetch(fmt.Sprintf("/item/%s.json", idStr))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	var item map[string]interface{}
	if err := json.Unmarshal(body, &item); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse item")
		return
	}

	if item == nil {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}

	writeJSON(w, http.StatusOK, item)
}

func handleUser(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "id parameter is required")
		return
	}

	body, err := hnFetch(fmt.Sprintf("/user/%s.json", id))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	var user map[string]interface{}
	if err := json.Unmarshal(body, &user); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse user")
		return
	}

	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	result := map[string]interface{}{
		"id":      user["id"],
		"karma":   user["karma"],
		"about":   user["about"],
		"created": user["created"],
	}

	writeJSON(w, http.StatusOK, result)
}
