package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

type geoResult struct {
	Results []struct {
		Name      string  `json:"name"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Country   string  `json:"country"`
		Admin1    string  `json:"admin1"`
	} `json:"results"`
}

type airQualityCurrentResponse struct {
	Current struct {
		USAQI           float64 `json:"us_aqi"`
		PM10            float64 `json:"pm10"`
		PM25            float64 `json:"pm2_5"`
		CarbonMonoxide  float64 `json:"carbon_monoxide"`
		NitrogenDioxide float64 `json:"nitrogen_dioxide"`
		SulphurDioxide  float64 `json:"sulphur_dioxide"`
		Ozone           float64 `json:"ozone"`
	} `json:"current"`
}

type airQualityForecastResponse struct {
	Hourly struct {
		Time  []string  `json:"time"`
		USAQI []float64 `json:"us_aqi"`
		PM25  []float64 `json:"pm2_5"`
		PM10  []float64 `json:"pm10"`
	} `json:"hourly"`
}

func geocodeCity(city string) (float64, float64, string, error) {
	u := fmt.Sprintf("https://geocoding-api.open-meteo.com/v1/search?name=%s&count=1&language=en", url.QueryEscape(city))
	resp, err := httpClient.Get(u)
	if err != nil {
		return 0, 0, "", fmt.Errorf("geocoding request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, "", fmt.Errorf("read geocoding response: %w", err)
	}

	var geo geoResult
	if err := json.Unmarshal(body, &geo); err != nil {
		return 0, 0, "", fmt.Errorf("parse geocoding response: %w", err)
	}

	if len(geo.Results) == 0 {
		return 0, 0, "", fmt.Errorf("city not found: %s", city)
	}

	r := geo.Results[0]
	location := r.Name
	if r.Admin1 != "" {
		location += ", " + r.Admin1
	}
	if r.Country != "" {
		location += ", " + r.Country
	}

	return r.Latitude, r.Longitude, location, nil
}

func aqiCategory(aqi float64) string {
	switch {
	case aqi <= 50:
		return "Good"
	case aqi <= 100:
		return "Moderate"
	case aqi <= 150:
		return "Unhealthy for Sensitive Groups"
	case aqi <= 200:
		return "Unhealthy"
	case aqi <= 300:
		return "Very Unhealthy"
	default:
		return "Hazardous"
	}
}

func registerRoutes(r chi.Router) {
	r.Get("/current", handleCurrentAQ)
	r.Get("/forecast", handleForecastAQ)
}

func handleCurrentAQ(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var lat, lon float64
	var location string
	var err error

	if q.Get("lat") != "" && q.Get("lon") != "" {
		lat, err = strconv.ParseFloat(q.Get("lat"), 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid lat value")
			return
		}
		lon, err = strconv.ParseFloat(q.Get("lon"), 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid lon value")
			return
		}
		location = fmt.Sprintf("%.2f, %.2f", lat, lon)
	} else if q.Get("city") != "" {
		lat, lon, location, err = geocodeCity(q.Get("city"))
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
	} else {
		writeError(w, http.StatusBadRequest, "city or lat/lon parameters are required")
		return
	}

	u := fmt.Sprintf(
		"https://air-quality-api.open-meteo.com/v1/air-quality?latitude=%.4f&longitude=%.4f&current=us_aqi,pm10,pm2_5,carbon_monoxide,nitrogen_dioxide,sulphur_dioxide,ozone",
		lat, lon,
	)

	resp, err := httpClient.Get(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, "air quality API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read air quality response")
		return
	}

	var aq airQualityCurrentResponse
	if err := json.Unmarshal(body, &aq); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse air quality response")
		return
	}

	c := aq.Current
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"location":         location,
		"latitude":         lat,
		"longitude":        lon,
		"aqi":              c.USAQI,
		"aqi_category":     aqiCategory(c.USAQI),
		"pm2_5":            c.PM25,
		"pm10":             c.PM10,
		"ozone":            c.Ozone,
		"nitrogen_dioxide": c.NitrogenDioxide,
		"sulphur_dioxide":  c.SulphurDioxide,
		"carbon_monoxide":  c.CarbonMonoxide,
	})
}

func handleForecastAQ(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	daysStr := r.URL.Query().Get("days")

	if latStr == "" || lonStr == "" {
		writeError(w, http.StatusBadRequest, "lat and lon parameters are required")
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid lat value")
		return
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid lon value")
		return
	}

	days := 5
	if daysStr != "" {
		d, err := strconv.Atoi(daysStr)
		if err == nil && d >= 1 && d <= 5 {
			days = d
		}
	}

	u := fmt.Sprintf(
		"https://air-quality-api.open-meteo.com/v1/air-quality?latitude=%.4f&longitude=%.4f&hourly=us_aqi,pm2_5,pm10&forecast_days=%d",
		lat, lon, days,
	)

	resp, err := httpClient.Get(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, "air quality API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read forecast response")
		return
	}

	var forecast airQualityForecastResponse
	if err := json.Unmarshal(body, &forecast); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse forecast response")
		return
	}

	h := forecast.Hourly
	if len(h.Time) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"latitude":  lat,
			"longitude": lon,
			"days":      days,
			"forecast":  []interface{}{},
		})
		return
	}

	// Group hourly data by date (first 10 chars of timestamp = YYYY-MM-DD)
	type daySummary struct {
		Date    string
		MaxAQI  float64
		SumPM25 float64
		Count   int
	}

	dailyMap := map[string]*daySummary{}
	dayOrder := []string{}

	for i, t := range h.Time {
		if len(t) < 10 {
			continue
		}
		date := t[:10]

		if _, exists := dailyMap[date]; !exists {
			dailyMap[date] = &daySummary{Date: date, MaxAQI: -1}
			dayOrder = append(dayOrder, date)
		}

		ds := dailyMap[date]

		if i < len(h.USAQI) {
			if h.USAQI[i] > ds.MaxAQI {
				ds.MaxAQI = h.USAQI[i]
			}
		}

		if i < len(h.PM25) {
			ds.SumPM25 += h.PM25[i]
			ds.Count++
		}
	}

	dailyForecasts := make([]map[string]interface{}, 0, len(dayOrder))
	for _, date := range dayOrder {
		ds := dailyMap[date]
		avgPM25 := 0.0
		if ds.Count > 0 {
			avgPM25 = math.Round(ds.SumPM25/float64(ds.Count)*100) / 100
		}
		maxAQI := ds.MaxAQI
		if maxAQI < 0 {
			maxAQI = 0
		}

		dailyForecasts = append(dailyForecasts, map[string]interface{}{
			"date":         date,
			"max_aqi":      maxAQI,
			"aqi_category": aqiCategory(maxAQI),
			"avg_pm2_5":    avgPM25,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"latitude":  lat,
		"longitude": lon,
		"days":      days,
		"forecast":  dailyForecasts,
	})
}
