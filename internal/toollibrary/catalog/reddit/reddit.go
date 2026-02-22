package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	redditClientID     string
	redditClientSecret string
	redditClient       = &http.Client{Timeout: 15 * time.Second}
	redditBase         = "https://oauth.reddit.com"
	redditTokenURL     = "https://www.reddit.com/api/v1/access_token"

	redditMu          sync.Mutex
	redditAccessToken string
	redditTokenExpiry time.Time
)

func initReddit(clientID, clientSecret string) {
	redditClientID = clientID
	redditClientSecret = clientSecret
}

// redditGetToken returns a valid access token, refreshing if expired.
func redditGetToken() (string, error) {
	redditMu.Lock()
	defer redditMu.Unlock()

	if redditAccessToken != "" && time.Now().Before(redditTokenExpiry) {
		return redditAccessToken, nil
	}

	values := url.Values{}
	values.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", redditTokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.SetBasicAuth(redditClientID, redditClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "OpenPaw/1.0")

	resp, err := redditClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request error (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}

	redditAccessToken = tokenResp.AccessToken
	// Refresh 60 seconds before actual expiry
	redditTokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)

	return redditAccessToken, nil
}

func redditGet(path string) (*http.Response, error) {
	token, err := redditGetToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", redditBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "OpenPaw/1.0")
	return redditClient.Do(req)
}

func redditReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Reddit API error (%d): %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, v)
}

// flattenRedditPosts extracts post data from Reddit's nested {data: {children: [{data: {...}}]}} format.
func flattenRedditPosts(result map[string]interface{}) []map[string]interface{} {
	posts := make([]map[string]interface{}, 0)

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return posts
	}

	children, ok := data["children"].([]interface{})
	if !ok {
		return posts
	}

	for _, child := range children {
		childMap, ok := child.(map[string]interface{})
		if !ok {
			continue
		}
		postData, ok := childMap["data"].(map[string]interface{})
		if !ok {
			continue
		}
		posts = append(posts, map[string]interface{}{
			"id":           postData["id"],
			"title":        postData["title"],
			"subreddit":    postData["subreddit"],
			"author":       postData["author"],
			"score":        postData["score"],
			"num_comments": postData["num_comments"],
			"url":          postData["url"],
			"created_utc":  postData["created_utc"],
		})
	}

	return posts
}

func registerRoutes(r chi.Router) {
	r.Get("/search", handleSearchPosts)
	r.Get("/hot", handleHotPosts)
	r.Get("/subreddit", handleSubredditInfo)
}

func handleSearchPosts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter is required")
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}

	subreddit := r.URL.Query().Get("subreddit")

	var path string
	if subreddit != "" {
		path = fmt.Sprintf("/r/%s/search?q=%s&restrict_sr=on&limit=%d",
			url.PathEscape(subreddit), url.QueryEscape(query), limit)
	} else {
		path = fmt.Sprintf("/search?q=%s&limit=%d",
			url.QueryEscape(query), limit)
	}

	resp, err := redditGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Reddit API request failed")
		return
	}

	var result map[string]interface{}
	if err := redditReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	posts := flattenRedditPosts(result)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query": query,
		"count": len(posts),
		"posts": posts,
	})
}

func handleHotPosts(w http.ResponseWriter, r *http.Request) {
	subreddit := r.URL.Query().Get("subreddit")
	if subreddit == "" {
		writeError(w, http.StatusBadRequest, "subreddit parameter is required")
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}

	path := fmt.Sprintf("/r/%s/hot?limit=%d", url.PathEscape(subreddit), limit)

	resp, err := redditGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Reddit API request failed")
		return
	}

	var result map[string]interface{}
	if err := redditReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	posts := flattenRedditPosts(result)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subreddit": subreddit,
		"count":     len(posts),
		"posts":     posts,
	})
}

func handleSubredditInfo(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name parameter is required")
		return
	}

	path := fmt.Sprintf("/r/%s/about", url.PathEscape(name))

	resp, err := redditGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Reddit API request failed")
		return
	}

	var result map[string]interface{}
	if err := redditReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Reddit wraps subreddit about in {data: {...}}
	data, _ := result["data"].(map[string]interface{})
	if data == nil {
		data = result
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":            data["display_name"],
		"title":           data["title"],
		"description":     data["public_description"],
		"subscribers":     data["subscribers"],
		"active_accounts": data["accounts_active"],
		"created_utc":     data["created_utc"],
	})
}
