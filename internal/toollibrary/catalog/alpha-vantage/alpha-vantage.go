package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	alphaKey    string
	alphaClient = &http.Client{Timeout: 20 * time.Second}
)

func initAlphaVantage() {
	alphaKey = os.Getenv("ALPHA_VANTAGE_API_KEY")
}

func registerRoutes(r chi.Router) {
	r.Get("/quote", handleAlphaQuote)
	r.Get("/fx", handleAlphaFX)
}

func handleAlphaQuote(w http.ResponseWriter, r *http.Request) {
	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	if symbol == "" {
		writeError(w, http.StatusBadRequest, "symbol parameter is required")
		return
	}
	data, err := alphaGET(url.Values{"function": {"GLOBAL_QUOTE"}, "symbol": {symbol}, "apikey": {alphaKey}})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleAlphaFX(w http.ResponseWriter, r *http.Request) {
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" || to == "" {
		writeError(w, http.StatusBadRequest, "from and to parameters are required")
		return
	}
	data, err := alphaGET(url.Values{"function": {"CURRENCY_EXCHANGE_RATE"}, "from_currency": {from}, "to_currency": {to}, "apikey": {alphaKey}})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func alphaGET(params url.Values) (map[string]interface{}, error) {
	u := "https://www.alphavantage.co/query?" + params.Encode()
	resp, err := alphaClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("Alpha Vantage request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Alpha Vantage response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Alpha Vantage API error (%d): %s", resp.StatusCode, string(body))
	}
	return data, nil
}
