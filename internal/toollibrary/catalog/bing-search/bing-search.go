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
	bingKey    string
	bingBase   = "https://api.bing.microsoft.com/v7.0"
	bingClient = &http.Client{Timeout: 20 * time.Second}
)

func initBing(key string) {
	bingKey = key
}

func registerRoutes(r chi.Router) {
	r.Get("/search", handleBingSearch)
	r.Get("/news", handleBingNews)
}

func handleBingSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	count := parseBingInt(r.URL.Query().Get("count"), 10, 1, 50)

	params := url.Values{}
	params.Set("q", q)
	params.Set("count", strconv.Itoa(count))
	params.Set("textDecorations", "false")
	params.Set("textFormat", "Raw")

	data, err := bingGET("/search", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	webPages, _ := data["webPages"].(map[string]interface{})
	results, _ := webPages["value"].([]interface{})
	compact := make([]map[string]interface{}, 0, len(results))
	for _, item := range results {
		result, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		compact = append(compact, map[string]interface{}{
			"name":    result["name"],
			"url":     result["url"],
			"snippet": result["snippet"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   q,
		"count":   len(compact),
		"results": compact,
	})
}

func handleBingNews(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	count := parseBingInt(r.URL.Query().Get("count"), 10, 1, 100)

	params := url.Values{}
	params.Set("q", q)
	params.Set("count", strconv.Itoa(count))
	params.Set("textFormat", "Raw")

	data, err := bingGET("/news/search", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	items, _ := data["value"].([]interface{})
	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		newsItem, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		provider := ""
		if providers, ok := newsItem["provider"].([]interface{}); ok && len(providers) > 0 {
			if first, ok := providers[0].(map[string]interface{}); ok {
				provider, _ = first["name"].(string)
			}
		}
		results = append(results, map[string]interface{}{
			"name":           newsItem["name"],
			"url":            newsItem["url"],
			"description":    newsItem["description"],
			"date_published": newsItem["datePublished"],
			"provider":       provider,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   q,
		"count":   len(results),
		"results": results,
	})
}

func bingGET(path string, params url.Values) (map[string]interface{}, error) {
	req, err := http.NewRequest(http.MethodGet, bingBase+path+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", bingKey)
	req.Header.Set("Accept", "application/json")

	resp, err := bingClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Bing request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Bing response: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Bing response: %w", err)
	}

	if resp.StatusCode >= 400 {
		msg := string(body)
		if errObj, ok := data["error"].(map[string]interface{}); ok {
			if inner, ok := errObj["message"].(string); ok {
				msg = inner
			}
		}
		return nil, fmt.Errorf("Bing API error (%d): %s", resp.StatusCode, msg)
	}
	return data, nil
}

func parseBingInt(raw string, fallback, min, max int) int {
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
