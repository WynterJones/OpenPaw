package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

const worldTimeBase = "https://worldtimeapi.org/api"

type worldTimeResponse struct {
	Datetime    string `json:"datetime"`
	Timezone    string `json:"timezone"`
	UTCOffset   string `json:"utc_offset"`
	DayOfWeek   int    `json:"day_of_week"`
	DayOfYear   int    `json:"day_of_year"`
	WeekNumber  int    `json:"week_number"`
	Abbreviation string `json:"abbreviation"`
	UnixTime    int64  `json:"unixtime"`
}

func registerRoutes(r chi.Router) {
	r.Get("/time", handleGetTime)
	r.Get("/timezones", handleListTimezones)
}

func handleGetTime(w http.ResponseWriter, r *http.Request) {
	timezone := r.URL.Query().Get("timezone")
	if timezone == "" {
		writeError(w, http.StatusBadRequest, "timezone parameter is required")
		return
	}

	u := fmt.Sprintf("%s/timezone/%s", worldTimeBase, timezone)

	resp, err := httpClient.Get(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, "World Time API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read response")
		return
	}

	if resp.StatusCode == http.StatusNotFound {
		writeError(w, http.StatusNotFound, "timezone not found")
		return
	}

	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("World Time API error (%d): %s", resp.StatusCode, string(body)))
		return
	}

	var timeData worldTimeResponse
	if err := json.Unmarshal(body, &timeData); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}

	dayNames := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	dayName := ""
	if timeData.DayOfWeek >= 0 && timeData.DayOfWeek < len(dayNames) {
		dayName = dayNames[timeData.DayOfWeek]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"datetime":     timeData.Datetime,
		"timezone":     timeData.Timezone,
		"abbreviation": timeData.Abbreviation,
		"utc_offset":   timeData.UTCOffset,
		"day_of_week":  dayName,
		"day_of_year":  timeData.DayOfYear,
		"week_number":  timeData.WeekNumber,
		"unix_time":    timeData.UnixTime,
	})
}

func handleListTimezones(w http.ResponseWriter, r *http.Request) {
	u := fmt.Sprintf("%s/timezone", worldTimeBase)

	resp, err := httpClient.Get(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, "World Time API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read response")
		return
	}

	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("World Time API error (%d)", resp.StatusCode))
		return
	}

	var timezones []string
	if err := json.Unmarshal(body, &timezones); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse timezone list")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":     len(timezones),
		"timezones": timezones,
	})
}
