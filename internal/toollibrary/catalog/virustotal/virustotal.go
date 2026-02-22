package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	vtKey    string
	vtClient = &http.Client{Timeout: 30 * time.Second}
	vtBase   = "https://www.virustotal.com/api/v3"
)

func initVirusTotal(key string) {
	vtKey = key
}

func vtRequest(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest(method, vtBase+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-apikey", vtKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return vtClient.Do(req)
}

func vtReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("VirusTotal API error (%d): %s", resp.StatusCode, string(data))
	}
	return json.Unmarshal(data, v)
}

func registerRoutes(r chi.Router) {
	r.Get("/url-scan", handleURLScan)
	r.Get("/domain", handleDomainReport)
	r.Get("/ip", handleIPReport)
}

func handleURLScan(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		writeError(w, http.StatusBadRequest, "url parameter is required")
		return
	}

	// Submit URL for scanning
	form := url.Values{}
	form.Set("url", rawURL)

	resp, err := vtRequest("POST", "/urls", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		writeError(w, http.StatusBadGateway, "VirusTotal API request failed")
		return
	}

	var submitResult map[string]interface{}
	if err := vtReadJSON(resp, &submitResult); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Extract analysis ID from the submission response
	data, _ := submitResult["data"].(map[string]interface{})
	analysisID, _ := data["id"].(string)

	if analysisID == "" {
		// Fallback: use base64 URL ID to get the report directly
		urlID := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString([]byte(rawURL))
		resp2, err := vtRequest("GET", "/urls/"+urlID, nil, "")
		if err != nil {
			writeError(w, http.StatusBadGateway, "VirusTotal API request failed")
			return
		}

		var urlReport map[string]interface{}
		if err := vtReadJSON(resp2, &urlReport); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}

		reportData, _ := urlReport["data"].(map[string]interface{})
		attrs, _ := reportData["attributes"].(map[string]interface{})
		stats, _ := attrs["last_analysis_stats"].(map[string]interface{})

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"url":    rawURL,
			"status": "completed",
			"stats":  stats,
		})
		return
	}

	// Poll analysis result
	resp2, err := vtRequest("GET", "/analyses/"+analysisID, nil, "")
	if err != nil {
		writeError(w, http.StatusBadGateway, "VirusTotal API request failed")
		return
	}

	var analysisResult map[string]interface{}
	if err := vtReadJSON(resp2, &analysisResult); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	analysisData, _ := analysisResult["data"].(map[string]interface{})
	attrs, _ := analysisData["attributes"].(map[string]interface{})
	status, _ := attrs["status"].(string)
	stats, _ := attrs["stats"].(map[string]interface{})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"url":    rawURL,
		"status": status,
		"stats":  stats,
	})
}

func handleDomainReport(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain parameter is required")
		return
	}

	resp, err := vtRequest("GET", "/domains/"+domain, nil, "")
	if err != nil {
		writeError(w, http.StatusBadGateway, "VirusTotal API request failed")
		return
	}

	var result map[string]interface{}
	if err := vtReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	data, _ := result["data"].(map[string]interface{})
	attrs, _ := data["attributes"].(map[string]interface{})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"domain":              domain,
		"reputation":          attrs["reputation"],
		"last_analysis_stats": attrs["last_analysis_stats"],
		"categories":          attrs["categories"],
	})
}

func handleIPReport(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		writeError(w, http.StatusBadRequest, "ip parameter is required")
		return
	}

	resp, err := vtRequest("GET", "/ip_addresses/"+ip, nil, "")
	if err != nil {
		writeError(w, http.StatusBadGateway, "VirusTotal API request failed")
		return
	}

	var result map[string]interface{}
	if err := vtReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	data, _ := result["data"].(map[string]interface{})
	attrs, _ := data["attributes"].(map[string]interface{})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ip":                  ip,
		"reputation":          attrs["reputation"],
		"last_analysis_stats": attrs["last_analysis_stats"],
		"country":             attrs["country"],
		"as_owner":            attrs["as_owner"],
	})
}
