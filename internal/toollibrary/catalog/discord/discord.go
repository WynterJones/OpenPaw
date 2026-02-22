package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	discordToken  string
	discordClient = &http.Client{Timeout: 15 * time.Second}
	discordBase   = "https://discord.com/api/v10"
)

func initDiscord(token string) {
	discordToken = token
}

func discordGet(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", discordBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bot "+discordToken)
	return discordClient.Do(req)
}

func discordPost(path string, payload interface{}) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", discordBase+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bot "+discordToken)
	req.Header.Set("Content-Type", "application/json")
	return discordClient.Do(req)
}

func discordReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Discord API error (%d): %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, v)
}

// channelTypeName maps Discord channel type integers to human-readable strings.
func channelTypeName(t float64) string {
	switch int(t) {
	case 0:
		return "text"
	case 1:
		return "dm"
	case 2:
		return "voice"
	case 3:
		return "group_dm"
	case 4:
		return "category"
	case 5:
		return "announcement"
	case 10:
		return "announcement_thread"
	case 11:
		return "public_thread"
	case 12:
		return "private_thread"
	case 13:
		return "stage_voice"
	case 14:
		return "directory"
	case 15:
		return "forum"
	case 16:
		return "media"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

func registerRoutes(r chi.Router) {
	r.Get("/guilds", handleListGuilds)
	r.Get("/channels", handleListChannels)
	r.Post("/message", handleSendMessage)
	r.Get("/messages", handleGetMessages)
}

func handleListGuilds(w http.ResponseWriter, r *http.Request) {
	resp, err := discordGet("/users/@me/guilds")
	if err != nil {
		writeError(w, http.StatusBadGateway, "Discord API request failed")
		return
	}

	var guilds []map[string]interface{}
	if err := discordReadJSON(resp, &guilds); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(guilds))
	for _, g := range guilds {
		result = append(result, map[string]interface{}{
			"id":    g["id"],
			"name":  g["name"],
			"icon":  g["icon"],
			"owner": g["owner"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":  len(result),
		"guilds": result,
	})
}

func handleListChannels(w http.ResponseWriter, r *http.Request) {
	guildID := r.URL.Query().Get("guild_id")
	if guildID == "" {
		writeError(w, http.StatusBadRequest, "guild_id parameter is required")
		return
	}

	resp, err := discordGet(fmt.Sprintf("/guilds/%s/channels", guildID))
	if err != nil {
		writeError(w, http.StatusBadGateway, "Discord API request failed")
		return
	}

	var channels []map[string]interface{}
	if err := discordReadJSON(resp, &channels); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(channels))
	for _, ch := range channels {
		typeNum, _ := ch["type"].(float64)
		result = append(result, map[string]interface{}{
			"id":       ch["id"],
			"name":     ch["name"],
			"type":     channelTypeName(typeNum),
			"topic":    ch["topic"],
			"position": ch["position"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"guild_id": guildID,
		"count":    len(result),
		"channels": result,
	})
}

type sendMessageRequest struct {
	ChannelID string `json:"channel_id"`
	Content   string `json:"content"`
}

func handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var req sendMessageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ChannelID == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, "channel_id and content are required")
		return
	}

	resp, err := discordPost(fmt.Sprintf("/channels/%s/messages", req.ChannelID), map[string]string{
		"content": req.Content,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "Discord API request failed")
		return
	}

	var msg map[string]interface{}
	if err := discordReadJSON(resp, &msg); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":         msg["id"],
		"channel_id": msg["channel_id"],
		"content":    msg["content"],
		"timestamp":  msg["timestamp"],
	})
}

func handleGetMessages(w http.ResponseWriter, r *http.Request) {
	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id parameter is required")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}

	resp, err := discordGet(fmt.Sprintf("/channels/%s/messages?limit=%d", channelID, limit))
	if err != nil {
		writeError(w, http.StatusBadGateway, "Discord API request failed")
		return
	}

	var messages []map[string]interface{}
	if err := discordReadJSON(resp, &messages); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		author := map[string]interface{}{}
		if a, ok := m["author"].(map[string]interface{}); ok {
			author["username"] = a["username"]
		}
		result = append(result, map[string]interface{}{
			"id":        m["id"],
			"author":    author,
			"content":   m["content"],
			"timestamp": m["timestamp"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"channel_id": channelID,
		"count":      len(result),
		"messages":   result,
	})
}
