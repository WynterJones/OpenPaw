package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	youtubeKey    string
	youtubeClient = &http.Client{Timeout: 15 * time.Second}
	youtubeBase   = "https://www.googleapis.com/youtube/v3"
)

func initYouTube(key string) {
	youtubeKey = key
}

func youtubeGet(path string) (*http.Response, error) {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	url := youtubeBase + path + separator + "key=" + youtubeKey

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return youtubeClient.Do(req)
}

func youtubeReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("YouTube API error (%d): %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, v)
}

func registerRoutes(r chi.Router) {
	r.Get("/search", handleSearch)
	r.Get("/video/{id}", handleGetVideo)
	r.Get("/channel/{id}", handleGetChannel)
	r.Get("/captions/{video_id}", handleListCaptions)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	maxResults := 10
	if m := r.URL.Query().Get("max_results"); m != "" {
		if n, err := strconv.Atoi(m); err == nil && n >= 1 && n <= 50 {
			maxResults = n
		}
	}

	searchType := "video"
	if t := r.URL.Query().Get("type"); t != "" {
		searchType = t
	}

	order := "relevance"
	if o := r.URL.Query().Get("order"); o != "" {
		order = o
	}

	path := fmt.Sprintf("/search?part=snippet&q=%s&maxResults=%d&type=%s&order=%s", q, maxResults, searchType, order)

	resp, err := youtubeGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "YouTube API request failed")
		return
	}

	var searchResult map[string]interface{}
	if err := youtubeReadJSON(resp, &searchResult); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	pageInfo, _ := searchResult["pageInfo"].(map[string]interface{})
	totalResults := 0
	if pageInfo != nil {
		if tr, ok := pageInfo["totalResults"].(float64); ok {
			totalResults = int(tr)
		}
	}

	items, _ := searchResult["items"].([]interface{})
	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		snippet, _ := entry["snippet"].(map[string]interface{})
		if snippet == nil {
			continue
		}

		id, _ := entry["id"].(map[string]interface{})
		videoID := ""
		if id != nil {
			if vid, ok := id["videoId"].(string); ok {
				videoID = vid
			} else if cid, ok := id["channelId"].(string); ok {
				videoID = cid
			} else if pid, ok := id["playlistId"].(string); ok {
				videoID = pid
			}
		}

		thumbnail := ""
		if thumbs, ok := snippet["thumbnails"].(map[string]interface{}); ok {
			if def, ok := thumbs["default"].(map[string]interface{}); ok {
				thumbnail, _ = def["url"].(string)
			}
		}

		results = append(results, map[string]interface{}{
			"video_id":      videoID,
			"title":         snippet["title"],
			"description":   snippet["description"],
			"channel_title": snippet["channelTitle"],
			"published_at":  snippet["publishedAt"],
			"thumbnail":     thumbnail,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_results": totalResults,
		"results":       results,
	})
}

func handleGetVideo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "video id is required")
		return
	}

	path := fmt.Sprintf("/videos?part=snippet,contentDetails,statistics&id=%s", id)

	resp, err := youtubeGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "YouTube API request failed")
		return
	}

	var result map[string]interface{}
	if err := youtubeReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	items, _ := result["items"].([]interface{})
	if len(items) == 0 {
		writeError(w, http.StatusNotFound, "video not found")
		return
	}

	video, ok := items[0].(map[string]interface{})
	if !ok {
		writeError(w, http.StatusBadGateway, "unexpected response format")
		return
	}

	snippet, _ := video["snippet"].(map[string]interface{})
	contentDetails, _ := video["contentDetails"].(map[string]interface{})
	statistics, _ := video["statistics"].(map[string]interface{})

	thumbnail := ""
	if snippet != nil {
		if thumbs, ok := snippet["thumbnails"].(map[string]interface{}); ok {
			if high, ok := thumbs["high"].(map[string]interface{}); ok {
				thumbnail, _ = high["url"].(string)
			} else if def, ok := thumbs["default"].(map[string]interface{}); ok {
				thumbnail, _ = def["url"].(string)
			}
		}
	}

	var tags interface{}
	if snippet != nil {
		tags = snippet["tags"]
	}

	duration := ""
	if contentDetails != nil {
		duration, _ = contentDetails["duration"].(string)
	}

	viewCount := ""
	likeCount := ""
	commentCount := ""
	if statistics != nil {
		viewCount, _ = statistics["viewCount"].(string)
		likeCount, _ = statistics["likeCount"].(string)
		commentCount, _ = statistics["commentCount"].(string)
	}

	channelTitle := ""
	title := ""
	description := ""
	publishedAt := ""
	if snippet != nil {
		channelTitle, _ = snippet["channelTitle"].(string)
		title, _ = snippet["title"].(string)
		description, _ = snippet["description"].(string)
		publishedAt, _ = snippet["publishedAt"].(string)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":            id,
		"title":         title,
		"description":   description,
		"channel_title": channelTitle,
		"published_at":  publishedAt,
		"duration":      duration,
		"view_count":    viewCount,
		"like_count":    likeCount,
		"comment_count": commentCount,
		"tags":          tags,
		"thumbnail":     thumbnail,
	})
}

func handleGetChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "channel id is required")
		return
	}

	path := fmt.Sprintf("/channels?part=snippet,statistics&id=%s", id)

	resp, err := youtubeGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "YouTube API request failed")
		return
	}

	var result map[string]interface{}
	if err := youtubeReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	items, _ := result["items"].([]interface{})
	if len(items) == 0 {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	channel, ok := items[0].(map[string]interface{})
	if !ok {
		writeError(w, http.StatusBadGateway, "unexpected response format")
		return
	}

	snippet, _ := channel["snippet"].(map[string]interface{})
	statistics, _ := channel["statistics"].(map[string]interface{})

	thumbnail := ""
	if snippet != nil {
		if thumbs, ok := snippet["thumbnails"].(map[string]interface{}); ok {
			if def, ok := thumbs["default"].(map[string]interface{}); ok {
				thumbnail, _ = def["url"].(string)
			}
		}
	}

	title := ""
	description := ""
	if snippet != nil {
		title, _ = snippet["title"].(string)
		description, _ = snippet["description"].(string)
	}

	subscriberCount := ""
	videoCount := ""
	viewCount := ""
	if statistics != nil {
		subscriberCount, _ = statistics["subscriberCount"].(string)
		videoCount, _ = statistics["videoCount"].(string)
		viewCount, _ = statistics["viewCount"].(string)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":               id,
		"title":            title,
		"description":      description,
		"subscriber_count": subscriberCount,
		"video_count":      videoCount,
		"view_count":       viewCount,
		"thumbnail":        thumbnail,
	})
}

func handleListCaptions(w http.ResponseWriter, r *http.Request) {
	videoID := chi.URLParam(r, "video_id")
	if videoID == "" {
		writeError(w, http.StatusBadRequest, "video_id is required")
		return
	}

	path := fmt.Sprintf("/captions?part=snippet&videoId=%s", videoID)

	resp, err := youtubeGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "YouTube API request failed")
		return
	}

	var result map[string]interface{}
	if err := youtubeReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	items, _ := result["items"].([]interface{})
	captions := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		snippet, _ := entry["snippet"].(map[string]interface{})
		if snippet == nil {
			continue
		}

		captionID, _ := entry["id"].(string)

		captions = append(captions, map[string]interface{}{
			"id":         captionID,
			"language":   snippet["language"],
			"name":       snippet["name"],
			"track_kind": snippet["trackKind"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"video_id": videoID,
		"count":    len(captions),
		"captions": captions,
	})
}
