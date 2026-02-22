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
	newsAPIKey    string
	newsAPIBase   = "https://newsapi.org/v2"
	newsAPIClient = &http.Client{Timeout: 15 * time.Second}
)

func initNewsAPI(key string) {
	newsAPIKey = key
}

func registerRoutes(r chi.Router) {
	r.Get("/top-headlines", handleTopHeadlines)
	r.Get("/everything", handleEverything)
}

func handleTopHeadlines(w http.ResponseWriter, r *http.Request) {
	params := url.Values{}

	country := strings.TrimSpace(r.URL.Query().Get("country"))
	if country == "" {
		country = "us"
	}
	params.Set("country", country)

	if category := strings.TrimSpace(r.URL.Query().Get("category")); category != "" {
		params.Set("category", category)
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		params.Set("q", q)
	}

	pageSize := parseIntInRange(r.URL.Query().Get("page_size"), 20, 1, 100)
	params.Set("pageSize", strconv.Itoa(pageSize))

	data, err := newsAPIGet("/top-headlines", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":        data["status"],
		"total_results": data["totalResults"],
		"articles":      compactArticles(data["articles"]),
	})
}

func handleEverything(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("sortBy", strings.TrimSpace(defaultString(r.URL.Query().Get("sort_by"), "publishedAt")))

	if language := strings.TrimSpace(r.URL.Query().Get("language")); language != "" {
		params.Set("language", language)
	}
	if from := strings.TrimSpace(r.URL.Query().Get("from")); from != "" {
		params.Set("from", from)
	}
	if to := strings.TrimSpace(r.URL.Query().Get("to")); to != "" {
		params.Set("to", to)
	}

	pageSize := parseIntInRange(r.URL.Query().Get("page_size"), 20, 1, 100)
	params.Set("pageSize", strconv.Itoa(pageSize))

	data, err := newsAPIGet("/everything", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":        data["status"],
		"query":         q,
		"total_results": data["totalResults"],
		"articles":      compactArticles(data["articles"]),
	})
}

func newsAPIGet(path string, params url.Values) (map[string]interface{}, error) {
	params.Set("apiKey", newsAPIKey)
	u := newsAPIBase + path + "?" + params.Encode()

	resp, err := newsAPIClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("News API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read News API response: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse News API response: %w", err)
	}

	if resp.StatusCode >= 400 {
		msg, _ := data["message"].(string)
		if msg == "" {
			msg = string(body)
		}
		return nil, fmt.Errorf("News API error (%d): %s", resp.StatusCode, msg)
	}

	return data, nil
}

func compactArticles(raw interface{}) []map[string]interface{} {
	items, _ := raw.([]interface{})
	articles := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		article, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		sourceName := ""
		if source, ok := article["source"].(map[string]interface{}); ok {
			sourceName, _ = source["name"].(string)
		}

		articles = append(articles, map[string]interface{}{
			"source":       sourceName,
			"author":       article["author"],
			"title":        article["title"],
			"description":  article["description"],
			"url":          article["url"],
			"image_url":    article["urlToImage"],
			"published_at": article["publishedAt"],
		})
	}
	return articles
}

func parseIntInRange(value string, fallback, min, max int) int {
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
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

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
