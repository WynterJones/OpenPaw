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
	twitchClientID     string
	twitchClientSecret string
	twitchClient       = &http.Client{Timeout: 15 * time.Second}
	twitchBase         = "https://api.twitch.tv/helix"
	twitchTokenURL     = "https://id.twitch.tv/oauth2/token"

	twitchMu          sync.Mutex
	twitchAccessToken string
	twitchTokenExpiry time.Time
)

func initTwitch(clientID, clientSecret string) {
	twitchClientID = clientID
	twitchClientSecret = clientSecret
}

// twitchGetToken returns a valid access token, refreshing if expired.
func twitchGetToken() (string, error) {
	twitchMu.Lock()
	defer twitchMu.Unlock()

	if twitchAccessToken != "" && time.Now().Before(twitchTokenExpiry) {
		return twitchAccessToken, nil
	}

	values := url.Values{}
	values.Set("client_id", twitchClientID)
	values.Set("client_secret", twitchClientSecret)
	values.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", twitchTokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := twitchClient.Do(req)
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

	twitchAccessToken = tokenResp.AccessToken
	// Refresh 60 seconds before actual expiry
	twitchTokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)

	return twitchAccessToken, nil
}

func twitchGet(path string) (*http.Response, error) {
	token, err := twitchGetToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", twitchBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", twitchClientID)
	return twitchClient.Do(req)
}

func twitchReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Twitch API error (%d): %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, v)
}

// twitchExtractData extracts the "data" array from Twitch's {data: [...]} response format.
func twitchExtractData(result map[string]interface{}) []interface{} {
	data, _ := result["data"].([]interface{})
	return data
}

func registerRoutes(r chi.Router) {
	r.Get("/streams", handleGetStreams)
	r.Get("/search", handleSearchChannels)
	r.Get("/games", handleSearchGames)
}

func handleGetStreams(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}

	path := fmt.Sprintf("/streams?first=%d", limit)

	if game := r.URL.Query().Get("game"); game != "" {
		path += "&game_name=" + url.QueryEscape(game)
	}

	resp, err := twitchGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Twitch API request failed")
		return
	}

	var result map[string]interface{}
	if err := twitchReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	items := twitchExtractData(result)
	streams := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		s, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		streams = append(streams, map[string]interface{}{
			"id":           s["id"],
			"user_name":    s["user_name"],
			"game_name":    s["game_name"],
			"title":        s["title"],
			"viewer_count": s["viewer_count"],
			"started_at":   s["started_at"],
			"language":     s["language"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":   len(streams),
		"streams": streams,
	})
}

func handleSearchChannels(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter is required")
		return
	}

	path := fmt.Sprintf("/search/channels?query=%s&first=10", url.QueryEscape(query))

	resp, err := twitchGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Twitch API request failed")
		return
	}

	var result map[string]interface{}
	if err := twitchReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	items := twitchExtractData(result)
	channels := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		ch, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		channels = append(channels, map[string]interface{}{
			"id":           ch["id"],
			"display_name": ch["display_name"],
			"game_name":    ch["game_name"],
			"is_live":      ch["is_live"],
			"title":        ch["title"],
			"started_at":   ch["started_at"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":    query,
		"count":    len(channels),
		"channels": channels,
	})
}

func handleSearchGames(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter is required")
		return
	}

	path := fmt.Sprintf("/search/categories?query=%s&first=10", url.QueryEscape(query))

	resp, err := twitchGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Twitch API request failed")
		return
	}

	var result map[string]interface{}
	if err := twitchReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	items := twitchExtractData(result)
	games := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		g, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		games = append(games, map[string]interface{}{
			"id":          g["id"],
			"name":        g["name"],
			"box_art_url": g["box_art_url"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query": query,
		"count": len(games),
		"games": games,
	})
}
