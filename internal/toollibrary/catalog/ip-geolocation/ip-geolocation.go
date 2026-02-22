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

var geoClient = &http.Client{Timeout: 15 * time.Second}

type ipAPIResponse struct {
	Status     string  `json:"status"`
	Message    string  `json:"message"`
	Query      string  `json:"query"`
	Country    string  `json:"country"`
	RegionName string  `json:"regionName"`
	City       string  `json:"city"`
	Zip        string  `json:"zip"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	Timezone   string  `json:"timezone"`
	ISP        string  `json:"isp"`
	Org        string  `json:"org"`
}

func registerRoutes(r chi.Router) {
	r.Get("/lookup", handleLookup)
	r.Get("/me", handleMe)
}

func fetchIPInfo(ip string) (*ipAPIResponse, error) {
	u := "http://ip-api.com/json/"
	if ip != "" {
		u += url.PathEscape(ip)
	}

	resp, err := geoClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("ip-api.com request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ip-api.com error (%d)", resp.StatusCode)
	}

	var parsed ipAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if parsed.Status == "fail" {
		return nil, fmt.Errorf("lookup failed: %s", parsed.Message)
	}

	return &parsed, nil
}

func formatIPResponse(data *ipAPIResponse) map[string]interface{} {
	return map[string]interface{}{
		"ip":         data.Query,
		"country":    data.Country,
		"regionName": data.RegionName,
		"city":       data.City,
		"zip":        data.Zip,
		"lat":        data.Lat,
		"lon":        data.Lon,
		"timezone":   data.Timezone,
		"isp":        data.ISP,
		"org":        data.Org,
	}
}

func handleLookup(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimSpace(r.URL.Query().Get("ip"))
	if ip == "" {
		writeError(w, http.StatusBadRequest, "ip parameter is required")
		return
	}

	data, err := fetchIPInfo(ip)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, formatIPResponse(data))
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	data, err := fetchIPInfo("")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, formatIPResponse(data))
}
