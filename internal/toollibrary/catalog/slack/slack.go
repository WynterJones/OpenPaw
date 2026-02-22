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
	slackToken  string
	slackClient = &http.Client{Timeout: 15 * time.Second}
	slackBase   = "https://slack.com/api"
)

func initSlack(token string) {
	slackToken = token
}

func slackGet(method string) (*http.Response, error) {
	req, err := http.NewRequest("GET", slackBase+"/"+method, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+slackToken)
	return slackClient.Do(req)
}

func slackPost(method string, payload interface{}) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", slackBase+"/"+method, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+slackToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	return slackClient.Do(req)
}

func slackReadJSON(resp *http.Response) (map[string]interface{}, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if ok, _ := result["ok"].(bool); !ok {
		errMsg, _ := result["error"].(string)
		return nil, fmt.Errorf("Slack API error: %s", errMsg)
	}
	return result, nil
}

func registerRoutes(r chi.Router) {
	r.Get("/channels", handleListChannels)
	r.Post("/message", handleSendMessage)
	r.Get("/messages", handleGetMessages)
	r.Get("/users", handleListUsers)
}

func handleListChannels(w http.ResponseWriter, r *http.Request) {
	resp, err := slackGet("conversations.list?types=public_channel,private_channel&limit=200")
	if err != nil {
		writeError(w, http.StatusBadGateway, "Slack API request failed")
		return
	}

	result, err := slackReadJSON(resp)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	channels, _ := result["channels"].([]interface{})
	output := make([]map[string]interface{}, 0, len(channels))
	for _, ch := range channels {
		c, ok := ch.(map[string]interface{})
		if !ok {
			continue
		}
		output = append(output, map[string]interface{}{
			"id":          c["id"],
			"name":        c["name"],
			"is_private":  c["is_private"],
			"is_archived": c["is_archived"],
			"num_members": c["num_members"],
			"topic":       extractText(c, "topic"),
			"purpose":     extractText(c, "purpose"),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":    len(output),
		"channels": output,
	})
}

func extractText(c map[string]interface{}, field string) string {
	if obj, ok := c[field].(map[string]interface{}); ok {
		if v, ok := obj["value"].(string); ok {
			return v
		}
	}
	return ""
}

type sendMessageRequest struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

func handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var req sendMessageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Channel == "" || req.Text == "" {
		writeError(w, http.StatusBadRequest, "channel and text are required")
		return
	}

	resp, err := slackPost("chat.postMessage", map[string]string{
		"channel": req.Channel,
		"text":    req.Text,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "Slack API request failed")
		return
	}

	result, err := slackReadJSON(resp)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"channel": result["channel"],
		"ts":      result["ts"],
	})
}

func handleGetMessages(w http.ResponseWriter, r *http.Request) {
	channel := r.URL.Query().Get("channel")
	if channel == "" {
		writeError(w, http.StatusBadRequest, "channel parameter is required")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}

	resp, err := slackGet(fmt.Sprintf("conversations.history?channel=%s&limit=%d", channel, limit))
	if err != nil {
		writeError(w, http.StatusBadGateway, "Slack API request failed")
		return
	}

	result, err := slackReadJSON(resp)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	messages, _ := result["messages"].([]interface{})
	output := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		output = append(output, map[string]interface{}{
			"user":    msg["user"],
			"text":    msg["text"],
			"ts":      msg["ts"],
			"type":    msg["type"],
			"subtype": msg["subtype"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"channel":  channel,
		"count":    len(output),
		"messages": output,
	})
}

func handleListUsers(w http.ResponseWriter, r *http.Request) {
	resp, err := slackGet("users.list?limit=200")
	if err != nil {
		writeError(w, http.StatusBadGateway, "Slack API request failed")
		return
	}

	result, err := slackReadJSON(resp)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	members, _ := result["members"].([]interface{})
	output := make([]map[string]interface{}, 0, len(members))
	for _, m := range members {
		user, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		if deleted, _ := user["deleted"].(bool); deleted {
			continue
		}
		if isBot, _ := user["is_bot"].(bool); isBot {
			continue
		}

		profile, _ := user["profile"].(map[string]interface{})
		displayName := ""
		if profile != nil {
			displayName, _ = profile["display_name"].(string)
		}

		output = append(output, map[string]interface{}{
			"id":           user["id"],
			"name":         user["name"],
			"real_name":    user["real_name"],
			"display_name": displayName,
			"is_admin":     user["is_admin"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(output),
		"users": output,
	})
}
