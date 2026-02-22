package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	mapboxToken  string
	mapboxClient = &http.Client{Timeout: 15 * time.Second}
	mapboxBase   = "https://api.mapbox.com"
)

func initMapbox(token string) {
	mapboxToken = token
}

func mapboxGet(path string) (*http.Response, error) {
	// Append access_token to the URL
	separator := "?"
	if len(path) > 0 {
		for _, c := range path {
			if c == '?' {
				separator = "&"
				break
			}
		}
	}
	fullURL := mapboxBase + path + separator + "access_token=" + url.QueryEscape(mapboxToken)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}
	return mapboxClient.Do(req)
}

func mapboxReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Mapbox API error (%d): %s", resp.StatusCode, string(data))
	}
	return json.Unmarshal(data, v)
}

func registerRoutes(r chi.Router) {
	r.Get("/geocode", handleGeocode)
	r.Get("/reverse", handleReverse)
	r.Get("/directions", handleDirections)
}

func handleGeocode(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter is required")
		return
	}

	path := "/geocoding/v5/mapbox.places/" + url.PathEscape(query) + ".json"
	resp, err := mapboxGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Mapbox API request failed")
		return
	}

	var result map[string]interface{}
	if err := mapboxReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	features, _ := result["features"].([]interface{})
	results := make([]map[string]interface{}, 0, len(features))
	for _, f := range features {
		feature, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, map[string]interface{}{
			"place_name": feature["place_name"],
			"center":     feature["center"],
			"relevance":  feature["relevance"],
			"place_type": feature["place_type"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   query,
		"count":   len(results),
		"results": results,
	})
}

func handleReverse(w http.ResponseWriter, r *http.Request) {
	lat := r.URL.Query().Get("lat")
	lon := r.URL.Query().Get("lon")
	if lat == "" || lon == "" {
		writeError(w, http.StatusBadRequest, "lat and lon parameters are required")
		return
	}

	// Mapbox expects lon,lat order
	path := "/geocoding/v5/mapbox.places/" + url.PathEscape(lon+","+lat) + ".json"
	resp, err := mapboxGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Mapbox API request failed")
		return
	}

	var result map[string]interface{}
	if err := mapboxReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	features, _ := result["features"].([]interface{})
	results := make([]map[string]interface{}, 0, len(features))
	for _, f := range features {
		feature, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, map[string]interface{}{
			"place_name": feature["place_name"],
			"place_type": feature["place_type"],
			"center":     feature["center"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"lat":     lat,
		"lon":     lon,
		"count":   len(results),
		"results": results,
	})
}

func handleDirections(w http.ResponseWriter, r *http.Request) {
	origin := r.URL.Query().Get("origin")
	destination := r.URL.Query().Get("destination")
	if origin == "" || destination == "" {
		writeError(w, http.StatusBadRequest, "origin and destination parameters are required")
		return
	}

	profile := r.URL.Query().Get("profile")
	if profile == "" {
		profile = "driving"
	}

	// Validate profile
	validProfiles := map[string]bool{
		"driving":         true,
		"walking":         true,
		"cycling":         true,
		"driving-traffic": true,
	}
	if !validProfiles[profile] {
		writeError(w, http.StatusBadRequest, "invalid profile: must be driving, walking, cycling, or driving-traffic")
		return
	}

	coordinates := url.PathEscape(origin) + ";" + url.PathEscape(destination)
	path := "/directions/v5/mapbox/" + profile + "/" + coordinates + "?geometries=geojson&steps=true"
	resp, err := mapboxGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Mapbox API request failed")
		return
	}

	var result map[string]interface{}
	if err := mapboxReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	routes, _ := result["routes"].([]interface{})
	if len(routes) == 0 {
		writeError(w, http.StatusNotFound, "no route found")
		return
	}

	route, _ := routes[0].(map[string]interface{})
	distance, _ := route["distance"].(float64)
	duration, _ := route["duration"].(float64)

	// Extract steps from the first leg
	legs, _ := route["legs"].([]interface{})
	steps := make([]map[string]interface{}, 0)
	if len(legs) > 0 {
		leg, _ := legs[0].(map[string]interface{})
		legSteps, _ := leg["steps"].([]interface{})
		for _, s := range legSteps {
			step, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			maneuver, _ := step["maneuver"].(map[string]interface{})
			steps = append(steps, map[string]interface{}{
				"instruction": maneuver["instruction"],
				"distance":    step["distance"],
				"duration":    step["duration"],
				"name":        step["name"],
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"origin":      origin,
		"destination": destination,
		"profile":     profile,
		"distance":    distance,
		"duration":    duration,
		"steps":       steps,
	})
}
