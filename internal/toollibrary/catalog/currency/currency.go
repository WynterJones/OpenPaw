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

var httpClient = &http.Client{Timeout: 15 * time.Second}

type exchangeRateResponse struct {
	Result         string             `json:"result"`
	BaseCode       string             `json:"base_code"`
	TimeLastUpdate string             `json:"time_last_update_utc"`
	Rates          map[string]float64 `json:"rates"`
}

func registerRoutes(r chi.Router) {
	r.Get("/convert", handleConvert)
	r.Get("/rates", handleRates)
}

func fetchRates(base string) (*exchangeRateResponse, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	if base == "" {
		base = "USD"
	}

	url := fmt.Sprintf("https://open.er-api.com/v6/latest/%s", base)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("exchange rate API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result exchangeRateResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if result.Result != "success" {
		return nil, fmt.Errorf("API returned non-success result for base currency: %s", base)
	}

	return &result, nil
}

func handleConvert(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	amountStr := r.URL.Query().Get("amount")

	if from == "" || to == "" {
		writeError(w, http.StatusBadRequest, "from and to parameters are required")
		return
	}

	amount := 1.0
	if amountStr != "" {
		a, err := strconv.ParseFloat(amountStr, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid amount value")
			return
		}
		amount = a
	}

	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))

	rates, err := fetchRates(from)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	rate, ok := rates.Rates[to]
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown target currency: %s", to))
		return
	}

	converted := amount * rate

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"from":        from,
		"to":          to,
		"amount":      amount,
		"rate":        rate,
		"result":      converted,
		"last_update": rates.TimeLastUpdate,
	})
}

func handleRates(w http.ResponseWriter, r *http.Request) {
	base := r.URL.Query().Get("base")
	if base == "" {
		base = "USD"
	}

	rates, err := fetchRates(base)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"base":        rates.BaseCode,
		"last_update": rates.TimeLastUpdate,
		"count":       len(rates.Rates),
		"rates":       rates.Rates,
	})
}
