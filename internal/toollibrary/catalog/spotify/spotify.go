package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	spotifyClientID     string
	spotifyClientSecret string
	spotifyClient       = &http.Client{Timeout: 15 * time.Second}
	spotifyBase         = "https://api.spotify.com/v1"
	spotifyTokenURL     = "https://accounts.spotify.com/api/token"

	spotifyMu          sync.Mutex
	spotifyAccessToken string
	spotifyTokenExpiry time.Time
)

func initSpotify(clientID, clientSecret string) {
	spotifyClientID = clientID
	spotifyClientSecret = clientSecret
}

// spotifyGetToken returns a valid access token, refreshing if expired.
func spotifyGetToken() (string, error) {
	spotifyMu.Lock()
	defer spotifyMu.Unlock()

	if spotifyAccessToken != "" && time.Now().Before(spotifyTokenExpiry) {
		return spotifyAccessToken, nil
	}

	values := url.Values{}
	values.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", spotifyTokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.SetBasicAuth(spotifyClientID, spotifyClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := spotifyClient.Do(req)
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

	spotifyAccessToken = tokenResp.AccessToken
	// Refresh 60 seconds before actual expiry to avoid edge cases
	spotifyTokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)

	return spotifyAccessToken, nil
}

func spotifyGet(path string) (*http.Response, error) {
	token, err := spotifyGetToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", spotifyBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return spotifyClient.Do(req)
}

func spotifyReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Spotify API error (%d): %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, v)
}

func registerRoutes(r chi.Router) {
	r.Get("/search", handleSearch)
	r.Get("/artist", handleGetArtist)
	r.Get("/album", handleGetAlbum)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter is required")
		return
	}

	searchType := r.URL.Query().Get("type")
	if searchType == "" {
		searchType = "track"
	}

	// Validate type
	validTypes := map[string]bool{"artist": true, "track": true, "album": true}
	if !validTypes[searchType] {
		writeError(w, http.StatusBadRequest, "type must be artist, track, or album")
		return
	}

	path := fmt.Sprintf("/search?q=%s&type=%s&limit=10", url.QueryEscape(query), url.QueryEscape(searchType))

	resp, err := spotifyGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Spotify API request failed")
		return
	}

	var result map[string]interface{}
	if err := spotifyReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Extract items from the appropriate key (artists, tracks, or albums)
	var items []interface{}
	key := searchType + "s" // "artists", "tracks", or "albums"
	if container, ok := result[key].(map[string]interface{}); ok {
		items, _ = container["items"].([]interface{})
	}

	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		out := map[string]interface{}{
			"id":   entry["id"],
			"name": entry["name"],
			"type": entry["type"],
		}

		switch searchType {
		case "artist":
			out["genres"] = entry["genres"]
			out["popularity"] = entry["popularity"]
			if followers, ok := entry["followers"].(map[string]interface{}); ok {
				out["followers"] = followers["total"]
			}
		case "track":
			out["duration_ms"] = entry["duration_ms"]
			out["popularity"] = entry["popularity"]
			out["artists"] = extractArtistNames(entry)
			if album, ok := entry["album"].(map[string]interface{}); ok {
				out["album"] = album["name"]
			}
		case "album":
			out["release_date"] = entry["release_date"]
			out["total_tracks"] = entry["total_tracks"]
			out["artists"] = extractArtistNames(entry)
		}

		results = append(results, out)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   query,
		"type":    searchType,
		"count":   len(results),
		"results": results,
	})
}

func handleGetArtist(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id parameter is required")
		return
	}

	resp, err := spotifyGet(fmt.Sprintf("/artists/%s", url.PathEscape(id)))
	if err != nil {
		writeError(w, http.StatusBadGateway, "Spotify API request failed")
		return
	}

	var artist map[string]interface{}
	if err := spotifyReadJSON(resp, &artist); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	var followers interface{}
	if f, ok := artist["followers"].(map[string]interface{}); ok {
		followers = f["total"]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":         artist["id"],
		"name":       artist["name"],
		"genres":     artist["genres"],
		"popularity": artist["popularity"],
		"followers":  followers,
	})
}

func handleGetAlbum(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id parameter is required")
		return
	}

	resp, err := spotifyGet(fmt.Sprintf("/albums/%s", url.PathEscape(id)))
	if err != nil {
		writeError(w, http.StatusBadGateway, "Spotify API request failed")
		return
	}

	var album map[string]interface{}
	if err := spotifyReadJSON(resp, &album); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Extract track names
	var trackNames []string
	if tracks, ok := album["tracks"].(map[string]interface{}); ok {
		if items, ok := tracks["items"].([]interface{}); ok {
			for _, item := range items {
				if t, ok := item.(map[string]interface{}); ok {
					if name, ok := t["name"].(string); ok {
						trackNames = append(trackNames, name)
					}
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":           album["id"],
		"name":         album["name"],
		"artists":      extractArtistNames(album),
		"release_date": album["release_date"],
		"total_tracks": album["total_tracks"],
		"tracks":       trackNames,
	})
}

func extractArtistNames(entry map[string]interface{}) []string {
	artists, _ := entry["artists"].([]interface{})
	names := make([]string, 0, len(artists))
	for _, a := range artists {
		if artist, ok := a.(map[string]interface{}); ok {
			if name, ok := artist["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return names
}
