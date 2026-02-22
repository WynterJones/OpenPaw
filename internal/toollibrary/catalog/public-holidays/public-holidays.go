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

var holidayClient = &http.Client{Timeout: 15 * time.Second}

const nagerBaseURL = "https://date.nager.at/api/v3"

func registerRoutes(r chi.Router) {
	r.Get("/holidays", handleHolidays)
	r.Get("/countries", handleCountries)
	r.Get("/next", handleNextHolidays)
}

func nagerFetch(path string) ([]byte, int, error) {
	resp, err := holidayClient.Get(nagerBaseURL + path)
	if err != nil {
		return nil, 0, fmt.Errorf("Nager.Date API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return body, resp.StatusCode, nil
}

func handleHolidays(w http.ResponseWriter, r *http.Request) {
	country := strings.TrimSpace(r.URL.Query().Get("country"))
	if country == "" {
		writeError(w, http.StatusBadRequest, "country parameter is required")
		return
	}

	yearStr := strings.TrimSpace(r.URL.Query().Get("year"))
	if yearStr == "" {
		writeError(w, http.StatusBadRequest, "year parameter is required")
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 1 || year > 9999 {
		writeError(w, http.StatusBadRequest, "year must be a valid integer")
		return
	}

	body, statusCode, err := nagerFetch(fmt.Sprintf("/PublicHolidays/%d/%s", year, country))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		writeError(w, http.StatusNotFound, "country not found or no data available")
		return
	}
	if statusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Nager.Date API error (%d)", statusCode))
		return
	}

	var holidays []interface{}
	if err := json.Unmarshal(body, &holidays); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse holidays")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"country":  country,
		"year":     year,
		"count":    len(holidays),
		"holidays": holidays,
	})
}

func handleCountries(w http.ResponseWriter, r *http.Request) {
	body, statusCode, err := nagerFetch("/AvailableCountries")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if statusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Nager.Date API error (%d)", statusCode))
		return
	}

	var countries []interface{}
	if err := json.Unmarshal(body, &countries); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse countries")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":     len(countries),
		"countries": countries,
	})
}

func handleNextHolidays(w http.ResponseWriter, r *http.Request) {
	country := strings.TrimSpace(r.URL.Query().Get("country"))
	if country == "" {
		writeError(w, http.StatusBadRequest, "country parameter is required")
		return
	}

	body, statusCode, err := nagerFetch(fmt.Sprintf("/NextPublicHolidays/%s", country))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		writeError(w, http.StatusNotFound, "country not found or no data available")
		return
	}
	if statusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Nager.Date API error (%d)", statusCode))
		return
	}

	var holidays []interface{}
	if err := json.Unmarshal(body, &holidays); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse holidays")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"country":  country,
		"count":    len(holidays),
		"holidays": holidays,
	})
}
