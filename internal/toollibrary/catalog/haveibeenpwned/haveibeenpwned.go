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
	hibpKey    string
	hibpClient = &http.Client{Timeout: 15 * time.Second}
	hibpBase   = "https://haveibeenpwned.com/api/v3"
)

func initHIBP(key string) {
	hibpKey = key
}

func hibpRequest(path string, authenticated bool) (*http.Response, error) {
	req, err := http.NewRequest("GET", hibpBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-agent", "OpenPaw")
	if authenticated {
		req.Header.Set("hibp-api-key", hibpKey)
	}
	return hibpClient.Do(req)
}

func hibpReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()

	// 404 means no results found - not an error
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HIBP API error (%d): %s", resp.StatusCode, string(data))
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

func registerRoutes(r chi.Router) {
	r.Get("/breaches", handleBreaches)
	r.Get("/breach", handleBreach)
	r.Get("/pastes", handlePastes)
}

func handleBreaches(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		writeError(w, http.StatusBadRequest, "email parameter is required")
		return
	}

	path := "/breachedaccount/" + url.PathEscape(email) + "?truncateResponse=false"
	resp, err := hibpRequest(path, true)
	if err != nil {
		writeError(w, http.StatusBadGateway, "HIBP API request failed")
		return
	}

	var breaches []map[string]interface{}
	if err := hibpReadJSON(resp, &breaches); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Return empty array if no breaches found (404 case)
	if breaches == nil {
		breaches = []map[string]interface{}{}
	}

	results := make([]map[string]interface{}, 0, len(breaches))
	for _, breach := range breaches {
		results = append(results, map[string]interface{}{
			"name":         breach["Name"],
			"title":        breach["Title"],
			"domain":       breach["Domain"],
			"breach_date":  breach["BreachDate"],
			"data_classes": breach["DataClasses"],
			"pwn_count":    breach["PwnCount"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"email":    email,
		"count":    len(results),
		"breaches": results,
	})
}

func handleBreach(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name parameter is required")
		return
	}

	// Single breach lookup does not require authentication
	resp, err := hibpRequest("/breach/"+url.PathEscape(name), false)
	if err != nil {
		writeError(w, http.StatusBadGateway, "HIBP API request failed")
		return
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		writeError(w, http.StatusNotFound, fmt.Sprintf("breach not found: %s", name))
		return
	}

	var breach map[string]interface{}
	if err := hibpReadJSON(resp, &breach); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":         breach["Name"],
		"title":        breach["Title"],
		"domain":       breach["Domain"],
		"breach_date":  breach["BreachDate"],
		"pwn_count":    breach["PwnCount"],
		"data_classes": breach["DataClasses"],
		"description":  breach["Description"],
		"is_verified":  breach["IsVerified"],
	})
}

func handlePastes(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		writeError(w, http.StatusBadRequest, "email parameter is required")
		return
	}

	path := "/pasteaccount/" + url.PathEscape(email)
	resp, err := hibpRequest(path, true)
	if err != nil {
		writeError(w, http.StatusBadGateway, "HIBP API request failed")
		return
	}

	var pastes []map[string]interface{}
	if err := hibpReadJSON(resp, &pastes); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Return empty array if no pastes found (404 case)
	if pastes == nil {
		pastes = []map[string]interface{}{}
	}

	results := make([]map[string]interface{}, 0, len(pastes))
	for _, paste := range pastes {
		results = append(results, map[string]interface{}{
			"source":      paste["Source"],
			"id":          paste["Id"],
			"title":       paste["Title"],
			"date":        paste["Date"],
			"email_count": paste["EmailCount"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"email":  email,
		"count":  len(results),
		"pastes": results,
	})
}
