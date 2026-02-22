package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var coinbaseClient = &http.Client{Timeout: 20 * time.Second}

func registerRoutes(r chi.Router) {
	r.Get("/spot", handleCoinbaseSpot)
	r.Get("/exchange-rates", handleCoinbaseExchangeRates)
}

func handleCoinbaseSpot(w http.ResponseWriter, r *http.Request) {
	pair := strings.TrimSpace(r.URL.Query().Get("pair"))
	if pair == "" {
		pair = "BTC-USD"
	}
	u := "https://api.coinbase.com/v2/prices/" + url.PathEscape(pair) + "/spot"
	data, err := coinbaseGET(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleCoinbaseExchangeRates(w http.ResponseWriter, r *http.Request) {
	currency := strings.TrimSpace(r.URL.Query().Get("currency"))
	if currency == "" {
		currency = "USD"
	}
	u := "https://api.coinbase.com/v2/exchange-rates?currency=" + url.QueryEscape(currency)
	data, err := coinbaseGET(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func coinbaseGET(u string) (map[string]interface{}, error) {
	resp, err := coinbaseClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("Coinbase request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Coinbase response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Coinbase API error (%d): %s", resp.StatusCode, string(body))
	}
	return data, nil
}
