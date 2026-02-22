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
	twilioSID    string
	twilioToken  string
	twilioClient = &http.Client{Timeout: 15 * time.Second}
	twilioBase   string
)

func initTwilio(sid, token string) {
	twilioSID = sid
	twilioToken = token
	twilioBase = fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s", sid)
}

func twilioGet(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", twilioBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(twilioSID, twilioToken)
	return twilioClient.Do(req)
}

func twilioPostForm(path string, values url.Values) (*http.Response, error) {
	req, err := http.NewRequest("POST", twilioBase+path, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(twilioSID, twilioToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return twilioClient.Do(req)
}

func twilioReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Twilio API error (%d): %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, v)
}

func registerRoutes(r chi.Router) {
	r.Post("/sms", handleSendSMS)
	r.Get("/messages", handleListMessages)
	r.Get("/account", handleGetAccount)
}

type sendSMSRequest struct {
	To   string `json:"to"`
	From string `json:"from"`
	Body string `json:"body"`
}

func handleSendSMS(w http.ResponseWriter, r *http.Request) {
	var req sendSMSRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.To == "" || req.From == "" || req.Body == "" {
		writeError(w, http.StatusBadRequest, "to, from, and body are required")
		return
	}

	values := url.Values{}
	values.Set("To", req.To)
	values.Set("From", req.From)
	values.Set("Body", req.Body)

	resp, err := twilioPostForm("/Messages.json", values)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Twilio API request failed")
		return
	}

	var msg map[string]interface{}
	if err := twilioReadJSON(resp, &msg); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sid":          msg["sid"],
		"to":           msg["to"],
		"from":         msg["from"],
		"status":       msg["status"],
		"date_created": msg["date_created"],
	})
}

func handleListMessages(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}

	path := fmt.Sprintf("/Messages.json?PageSize=%d", limit)

	resp, err := twilioGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Twilio API request failed")
		return
	}

	var result map[string]interface{}
	if err := twilioReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	rawMessages, _ := result["messages"].([]interface{})
	messages := make([]map[string]interface{}, 0, len(rawMessages))
	for _, item := range rawMessages {
		msg, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		messages = append(messages, map[string]interface{}{
			"sid":       msg["sid"],
			"to":        msg["to"],
			"from":      msg["from"],
			"body":      msg["body"],
			"status":    msg["status"],
			"direction": msg["direction"],
			"date_sent": msg["date_sent"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":    len(messages),
		"messages": messages,
	})
}

func handleGetAccount(w http.ResponseWriter, r *http.Request) {
	reqURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s.json", twilioSID)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}
	req.SetBasicAuth(twilioSID, twilioToken)

	resp, err := twilioClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Twilio API request failed")
		return
	}

	var account map[string]interface{}
	if err := twilioReadJSON(resp, &account); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sid":           account["sid"],
		"friendly_name": account["friendly_name"],
		"status":        account["status"],
		"type":          account["type"],
		"date_created":  account["date_created"],
	})
}
