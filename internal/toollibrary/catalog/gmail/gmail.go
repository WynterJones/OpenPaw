package main

import (
	"encoding/base64"
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
	gmailAPIKey string
	gmailClient = &http.Client{Timeout: 15 * time.Second}
	gmailBase   = "https://gmail.googleapis.com/gmail/v1/users/me"
)

func initGmail(apiKey string) {
	gmailAPIKey = apiKey
}

func gmailRequest(method, path string, body io.Reader) (*http.Response, error) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	fullURL := gmailBase + path + sep + "key=" + url.QueryEscape(gmailAPIKey)

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return gmailClient.Do(req)
}

func gmailReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Gmail API error (%d): %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, v)
}

func registerRoutes(r chi.Router) {
	r.Get("/messages", handleListMessages)
	r.Get("/messages/{id}", handleGetMessage)
	r.Post("/send", handleSendMessage)
}

func handleListMessages(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	maxStr := r.URL.Query().Get("max")

	maxResults := 10
	if maxStr != "" {
		if m, err := strconv.Atoi(maxStr); err == nil && m >= 1 && m <= 100 {
			maxResults = m
		}
	}

	path := fmt.Sprintf("/messages?maxResults=%d", maxResults)
	if query != "" {
		path += "&q=" + url.QueryEscape(query)
	}

	resp, err := gmailRequest("GET", path, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Gmail API request failed")
		return
	}

	var listResult struct {
		Messages []struct {
			ID       string `json:"id"`
			ThreadID string `json:"threadId"`
		} `json:"messages"`
		ResultSizeEstimate int `json:"resultSizeEstimate"`
	}
	if err := gmailReadJSON(resp, &listResult); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	messages := make([]map[string]interface{}, 0, len(listResult.Messages))
	for _, msg := range listResult.Messages {
		detail, err := fetchMessageSummary(msg.ID)
		if err != nil {
			continue
		}
		messages = append(messages, detail)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":    len(messages),
		"messages": messages,
	})
}

func fetchMessageSummary(id string) (map[string]interface{}, error) {
	resp, err := gmailRequest("GET", "/messages/"+id+"?format=metadata&metadataHeaders=From&metadataHeaders=To&metadataHeaders=Subject&metadataHeaders=Date", nil)
	if err != nil {
		return nil, err
	}

	var msg struct {
		ID      string `json:"id"`
		Snippet string `json:"snippet"`
		Payload struct {
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
		} `json:"payload"`
		InternalDate string `json:"internalDate"`
	}
	if err := gmailReadJSON(resp, &msg); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"id":      msg.ID,
		"snippet": msg.Snippet,
	}
	for _, h := range msg.Payload.Headers {
		switch h.Name {
		case "From":
			result["from"] = h.Value
		case "To":
			result["to"] = h.Value
		case "Subject":
			result["subject"] = h.Value
		case "Date":
			result["date"] = h.Value
		}
	}

	return result, nil
}

func handleGetMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "message id is required")
		return
	}

	resp, err := gmailRequest("GET", "/messages/"+id+"?format=full", nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Gmail API request failed")
		return
	}

	var msg map[string]interface{}
	if err := gmailReadJSON(resp, &msg); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, msg)
}

type sendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.To == "" || req.Subject == "" {
		writeError(w, http.StatusBadRequest, "to and subject are required")
		return
	}

	rawMsg := fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		req.To, req.Subject, req.Body)

	encoded := base64.URLEncoding.EncodeToString([]byte(rawMsg))

	payload, _ := json.Marshal(map[string]string{
		"raw": encoded,
	})

	resp, err := gmailRequest("POST", "/messages/send", strings.NewReader(string(payload)))
	if err != nil {
		writeError(w, http.StatusBadGateway, "Gmail API request failed")
		return
	}

	var result map[string]interface{}
	if err := gmailReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"message_id": result["id"],
		"thread_id":  result["threadId"],
	})
}
