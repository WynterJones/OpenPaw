package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	sgKey    string
	sgClient = &http.Client{Timeout: 15 * time.Second}
	sgBase   = "https://api.sendgrid.com/v3"
)

func initSendGrid(key string) {
	sgKey = key
}

func sgRequest(method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, sgBase+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sgKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return sgClient.Do(req)
}

func sgReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("SendGrid API error (%d): %s", resp.StatusCode, string(data))
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

func registerRoutes(r chi.Router) {
	r.Post("/send", handleSendEmail)
	r.Get("/stats", handleGetStats)
}

type sendEmailRequest struct {
	To      string `json:"to"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

func handleSendEmail(w http.ResponseWriter, r *http.Request) {
	var req sendEmailRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.To == "" {
		writeError(w, http.StatusBadRequest, "to is required")
		return
	}
	if req.From == "" {
		writeError(w, http.StatusBadRequest, "from is required")
		return
	}
	if req.Subject == "" {
		writeError(w, http.StatusBadRequest, "subject is required")
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	// Build SendGrid personalizations format
	content := []map[string]string{
		{
			"type":  "text/plain",
			"value": req.Text,
		},
	}
	if req.HTML != "" {
		content = append(content, map[string]string{
			"type":  "text/html",
			"value": req.HTML,
		})
	}

	sgBody := map[string]interface{}{
		"personalizations": []map[string]interface{}{
			{
				"to": []map[string]string{
					{"email": req.To},
				},
			},
		},
		"from": map[string]string{
			"email": req.From,
		},
		"subject": req.Subject,
		"content": content,
	}

	resp, err := sgRequest("POST", "/mail/send", sgBody)
	if err != nil {
		writeError(w, http.StatusBadGateway, "SendGrid API request failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		writeError(w, http.StatusBadGateway, fmt.Sprintf("SendGrid API error (%d): %s", resp.StatusCode, string(body)))
		return
	}

	// SendGrid returns 202 Accepted with the message ID in the header
	messageID := resp.Header.Get("X-Message-Id")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"message_id": messageID,
	})
}

func handleGetStats(w http.ResponseWriter, r *http.Request) {
	startDate := r.URL.Query().Get("start_date")
	if startDate == "" {
		writeError(w, http.StatusBadRequest, "start_date parameter is required (YYYY-MM-DD)")
		return
	}

	path := "/stats?start_date=" + url.QueryEscape(startDate)
	if endDate := r.URL.Query().Get("end_date"); endDate != "" {
		path += "&end_date=" + url.QueryEscape(endDate)
	}

	resp, err := sgRequest("GET", path, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "SendGrid API request failed")
		return
	}

	var rawStats []map[string]interface{}
	if err := sgReadJSON(resp, &rawStats); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	stats := make([]map[string]interface{}, 0, len(rawStats))
	for _, dayStat := range rawStats {
		date, _ := dayStat["date"].(string)
		metricsArr, _ := dayStat["stats"].([]interface{})

		// Aggregate metrics from all categories for this date
		aggregated := map[string]float64{
			"requests":     0,
			"delivered":    0,
			"opens":        0,
			"clicks":       0,
			"bounces":      0,
			"spam_reports": 0,
		}

		for _, m := range metricsArr {
			metricGroup, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			metrics, ok := metricGroup["metrics"].(map[string]interface{})
			if !ok {
				continue
			}
			for key := range aggregated {
				if val, ok := metrics[key].(float64); ok {
					aggregated[key] += val
				}
			}
		}

		stats = append(stats, map[string]interface{}{
			"date":         date,
			"requests":     aggregated["requests"],
			"delivered":    aggregated["delivered"],
			"opens":        aggregated["opens"],
			"clicks":       aggregated["clicks"],
			"bounces":      aggregated["bounces"],
			"spam_reports": aggregated["spam_reports"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(stats),
		"stats": stats,
	})
}
