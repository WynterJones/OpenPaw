package main

import (
	"encoding/json"
	"fmt"
	"io"
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

type weatherResponse struct {
	Current struct {
		Temperature   float64 `json:"temperature_2m"`
		Humidity      int     `json:"relative_humidity_2m"`
		WindSpeed     float64 `json:"wind_speed_10m"`
		WeatherCode   int     `json:"weather_code"`
		ApparentTemp  float64 `json:"apparent_temperature"`
		Precipitation float64 `json:"precipitation"`
	} `json:"current"`
	CurrentUnits struct {
		Temperature string `json:"temperature_2m"`
		WindSpeed   string `json:"wind_speed_10m"`
	} `json:"current_units"`
}

type forecastResponse struct {
	Daily struct {
		Time         []string  `json:"time"`
		TempMax      []float64 `json:"temperature_2m_max"`
		TempMin      []float64 `json:"temperature_2m_min"`
		WeatherCode  []int     `json:"weather_code"`
		PrecipSum    []float64 `json:"precipitation_sum"`
		WindSpeedMax []float64 `json:"wind_speed_10m_max"`
	} `json:"daily"`
}

func registerRoutes(r chi.Router) {
	r.Get("/current", handleCurrentWeather)
	r.Get("/forecast", handleForecast)
}

func weatherCodeToDescription(code int) string {
	descriptions := map[int]string{
		0:  "Clear sky",
		1:  "Mainly clear",
		2:  "Partly cloudy",
		3:  "Overcast",
		45: "Foggy",
		48: "Depositing rime fog",
		51: "Light drizzle",
		53: "Moderate drizzle",
		55: "Dense drizzle",
		61: "Slight rain",
		63: "Moderate rain",
		65: "Heavy rain",
		71: "Slight snowfall",
		73: "Moderate snowfall",
		75: "Heavy snowfall",
		80: "Slight rain showers",
		81: "Moderate rain showers",
		82: "Violent rain showers",
		85: "Slight snow showers",
		86: "Heavy snow showers",
		95: "Thunderstorm",
		96: "Thunderstorm with slight hail",
		99: "Thunderstorm with heavy hail",
	}
	if desc, ok := descriptions[code]; ok {
		return desc
	}
	return "Unknown"
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

func handleCurrentWeather(w http.ResponseWriter, r *http.Request) {
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
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,relative_humidity_2m,apparent_temperature,precipitation,weather_code,wind_speed_10m",
		lat, lon,
	)

	resp, err := httpClient.Get(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, "weather API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read weather response")
		return
	}

	var weather weatherResponse
	if err := json.Unmarshal(body, &weather); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse weather response")
		return
	}

	c := weather.Current
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"location":             location,
		"latitude":             lat,
		"longitude":            lon,
		"temperature":          c.Temperature,
		"temperature_unit":     weather.CurrentUnits.Temperature,
		"apparent_temperature": c.ApparentTemp,
		"humidity":             c.Humidity,
		"wind_speed":           c.WindSpeed,
		"wind_speed_unit":      weather.CurrentUnits.WindSpeed,
		"precipitation":        c.Precipitation,
		"conditions":           weatherCodeToDescription(c.WeatherCode),
		"weather_code":         c.WeatherCode,
	})
}

func handleForecast(w http.ResponseWriter, r *http.Request) {
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

	days := 7
	if daysStr != "" {
		d, err := strconv.Atoi(daysStr)
		if err == nil && d >= 1 && d <= 16 {
			days = d
		}
	}

	u := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&daily=weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum,wind_speed_10m_max&forecast_days=%d",
		lat, lon, days,
	)

	resp, err := httpClient.Get(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, "weather API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read forecast response")
		return
	}

	var forecast forecastResponse
	if err := json.Unmarshal(body, &forecast); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse forecast response")
		return
	}

	d := forecast.Daily
	dailyForecasts := make([]map[string]interface{}, 0, len(d.Time))
	for i := range d.Time {
		dailyForecasts = append(dailyForecasts, map[string]interface{}{
			"date":           d.Time[i],
			"temp_max":       d.TempMax[i],
			"temp_min":       d.TempMin[i],
			"conditions":     weatherCodeToDescription(d.WeatherCode[i]),
			"precipitation":  d.PrecipSum[i],
			"wind_speed_max": d.WindSpeedMax[i],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"latitude":  lat,
		"longitude": lon,
		"days":      days,
		"forecast":  dailyForecasts,
	})
}
