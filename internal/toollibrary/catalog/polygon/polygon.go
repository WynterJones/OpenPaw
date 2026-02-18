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
	polygonKey    string
	polygonClient = &http.Client{Timeout: 20 * time.Second}
)

func initPolygon() {
	polygonKey = os.Getenv("POLYGON_API_KEY")
}

func registerRoutes(r chi.Router) {
	r.Get("/snapshot", handlePolygonSnapshot)
	r.Get("/aggs", handlePolygonAggs)
}

func handlePolygonSnapshot(w http.ResponseWriter, r *http.Request) {
	ticker := strings.TrimSpace(r.URL.Query().Get("ticker"))
	if ticker == "" {
		writeError(w, http.StatusBadRequest, "ticker parameter is required")
		return
	}
	u := fmt.Sprintf("https://api.polygon.io/v2/snapshot/locale/us/markets/stocks/tickers/%s?apiKey=%s", url.PathEscape(ticker), url.QueryEscape(polygonKey))
	data, err := polygonGetURL(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handlePolygonAggs(w http.ResponseWriter, r *http.Request) {
	ticker := strings.TrimSpace(r.URL.Query().Get("ticker"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if ticker == "" || from == "" || to == "" {
		writeError(w, http.StatusBadRequest, "ticker, from, and to parameters are required (YYYY-MM-DD)")
		return
	}
	u := fmt.Sprintf("https://api.polygon.io/v2/aggs/ticker/%s/range/1/day/%s/%s?adjusted=true&sort=asc&limit=500&apiKey=%s", url.PathEscape(ticker), url.PathEscape(from), url.PathEscape(to), url.QueryEscape(polygonKey))
	data, err := polygonGetURL(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func polygonGetURL(u string) (map[string]interface{}, error) {
	resp, err := polygonClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("Polygon request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Polygon response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Polygon API error (%d): %s", resp.StatusCode, string(body))
	}
	return data, nil
}
