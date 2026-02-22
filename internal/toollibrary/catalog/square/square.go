package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	squareToken  string
	squareClient = &http.Client{Timeout: 20 * time.Second}
)

func initSquare() {
	squareToken = os.Getenv("SQUARE_ACCESS_TOKEN")
}

func registerRoutes(r chi.Router) {
	r.Get("/locations", handleSquareLocations)
	r.Get("/payments", handleSquarePayments)
}

func handleSquareLocations(w http.ResponseWriter, r *http.Request) {
	data, err := squareGET("https://connect.squareup.com/v2/locations", nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleSquarePayments(w http.ResponseWriter, r *http.Request) {
	limit := parseSquareInt(r.URL.Query().Get("limit"), 20, 1, 100)
	params := url.Values{"limit": {strconv.Itoa(limit)}}
	if begin := r.URL.Query().Get("begin_time"); begin != "" {
		params.Set("begin_time", begin)
	}
	if end := r.URL.Query().Get("end_time"); end != "" {
		params.Set("end_time", end)
	}
	data, err := squareGET("https://connect.squareup.com/v2/payments", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func squareGET(baseURL string, params url.Values) (map[string]interface{}, error) {
	u := baseURL
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+squareToken)
	req.Header.Set("Square-Version", "2025-01-23")
	req.Header.Set("Accept", "application/json")
	resp, err := squareClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Square request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Square response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Square API error (%d): %s", resp.StatusCode, string(body))
	}
	return data, nil
}

func parseSquareInt(raw string, fallback, min, max int) int {
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
