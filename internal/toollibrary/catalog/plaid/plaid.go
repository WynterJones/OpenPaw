package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	plaidClientID string
	plaidSecret   string
	plaidBaseURL  string
	plaidClient   = &http.Client{Timeout: 25 * time.Second}
)

func initPlaid() {
	plaidClientID = os.Getenv("PLAID_CLIENT_ID")
	plaidSecret = os.Getenv("PLAID_SECRET")
	env := strings.ToLower(os.Getenv("PLAID_ENV"))
	if env == "" {
		env = "sandbox"
	}
	switch env {
	case "production":
		plaidBaseURL = "https://production.plaid.com"
	case "development":
		plaidBaseURL = "https://development.plaid.com"
	default:
		plaidBaseURL = "https://sandbox.plaid.com"
	}
}

func registerRoutes(r chi.Router) {
	r.Post("/accounts/balance", handlePlaidAccountsBalance)
	r.Post("/transactions/get", handlePlaidTransactionsGet)
}

func handlePlaidAccountsBalance(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(fmt.Sprint(req["access_token"])) == "" {
		writeError(w, http.StatusBadRequest, "access_token is required")
		return
	}
	data, err := plaidPost("/accounts/balance/get", req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handlePlaidTransactionsGet(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(fmt.Sprint(req["access_token"])) == "" || strings.TrimSpace(fmt.Sprint(req["start_date"])) == "" || strings.TrimSpace(fmt.Sprint(req["end_date"])) == "" {
		writeError(w, http.StatusBadRequest, "access_token, start_date, and end_date are required")
		return
	}
	if _, ok := req["options"]; !ok {
		req["options"] = map[string]interface{}{"count": 100, "offset": 0}
	}
	data, err := plaidPost("/transactions/get", req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func plaidPost(path string, payload map[string]interface{}) (map[string]interface{}, error) {
	payload["client_id"] = plaidClientID
	payload["secret"] = plaidSecret
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, plaidBaseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PLAID-VERSION", "2020-09-14")

	resp, err := plaidClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Plaid request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Plaid response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Plaid API error (%d): %s", resp.StatusCode, string(body))
	}
	return data, nil
}
