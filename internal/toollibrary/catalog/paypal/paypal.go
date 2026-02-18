package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	paypalClientID string
	paypalSecret   string
	paypalBaseURL  string
	paypalClient   = &http.Client{Timeout: 25 * time.Second}
)

func initPayPal() {
	paypalClientID = os.Getenv("PAYPAL_CLIENT_ID")
	paypalSecret = os.Getenv("PAYPAL_CLIENT_SECRET")
	env := strings.ToLower(os.Getenv("PAYPAL_ENV"))
	if env == "" {
		env = "sandbox"
	}
	if env == "live" || env == "production" {
		paypalBaseURL = "https://api-m.paypal.com"
	} else {
		paypalBaseURL = "https://api-m.sandbox.paypal.com"
	}
}

func registerRoutes(r chi.Router) {
	r.Get("/invoices", handlePayPalInvoices)
	r.Get("/transactions", handlePayPalTransactions)
}

func handlePayPalInvoices(w http.ResponseWriter, r *http.Request) {
	pageSize := parsePayPalInt(r.URL.Query().Get("page_size"), 20, 1, 100)
	params := url.Values{"page_size": {strconv.Itoa(pageSize)}, "total_required": {"true"}}
	data, err := paypalGET("/v2/invoicing/invoices", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handlePayPalTransactions(w http.ResponseWriter, r *http.Request) {
	start := strings.TrimSpace(r.URL.Query().Get("start_date"))
	end := strings.TrimSpace(r.URL.Query().Get("end_date"))
	if start == "" || end == "" {
		writeError(w, http.StatusBadRequest, "start_date and end_date are required (RFC3339)")
		return
	}
	params := url.Values{"start_date": {start}, "end_date": {end}, "page_size": {"100"}}
	data, err := paypalGET("/v1/reporting/transactions", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func paypalGET(path string, params url.Values) (map[string]interface{}, error) {
	token, err := paypalAccessToken()
	if err != nil {
		return nil, err
	}
	u := paypalBaseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := paypalClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PayPal request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse PayPal response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("PayPal API error (%d): %s", resp.StatusCode, string(body))
	}
	return data, nil
}

func paypalAccessToken() (string, error) {
	vals := url.Values{"grant_type": {"client_credentials"}}
	req, _ := http.NewRequest(http.MethodPost, paypalBaseURL+"/v1/oauth2/token", bytes.NewBufferString(vals.Encode()))
	creds := base64.StdEncoding.EncodeToString([]byte(paypalClientID + ":" + paypalSecret))
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := paypalClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("PayPal auth failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("parse PayPal auth response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("PayPal auth error (%d): %s", resp.StatusCode, string(body))
	}
	token, _ := data["access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("PayPal auth response missing access_token")
	}
	return token, nil
}

func parsePayPalInt(raw string, fallback, min, max int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
