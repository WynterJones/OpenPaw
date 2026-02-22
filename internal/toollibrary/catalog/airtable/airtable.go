package main

import (
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
	airtableKey    string
	airtableBaseID string
	airtableClient = &http.Client{Timeout: 20 * time.Second}
)

func initAirtable() {
	airtableKey = os.Getenv("AIRTABLE_API_KEY")
	airtableBaseID = os.Getenv("AIRTABLE_BASE_ID")
}

func registerRoutes(r chi.Router) {
	r.Get("/tables", handleAirtableTables)
	r.Get("/records/{table}", handleAirtableRecords)
}

func handleAirtableTables(w http.ResponseWriter, r *http.Request) {
	u := "https://api.airtable.com/v0/meta/bases/" + url.PathEscape(airtableBaseID) + "/tables"
	data, err := airtableGET(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleAirtableRecords(w http.ResponseWriter, r *http.Request) {
	table := strings.TrimSpace(chi.URLParam(r, "table"))
	if table == "" {
		writeError(w, http.StatusBadRequest, "table is required")
		return
	}
	params := url.Values{}
	params.Set("maxRecords", strconv.Itoa(parseAirtableInt(r.URL.Query().Get("max_records"), 50, 1, 100)))
	if view := strings.TrimSpace(r.URL.Query().Get("view")); view != "" {
		params.Set("view", view)
	}
	u := "https://api.airtable.com/v0/" + url.PathEscape(airtableBaseID) + "/" + url.PathEscape(table) + "?" + params.Encode()
	data, err := airtableGET(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func airtableGET(u string) (map[string]interface{}, error) {
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+airtableKey)
	req.Header.Set("Accept", "application/json")
	resp, err := airtableClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Airtable request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Airtable response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Airtable API error (%d): %s", resp.StatusCode, string(body))
	}
	return data, nil
}

func parseAirtableInt(raw string, fallback, min, max int) int {
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
