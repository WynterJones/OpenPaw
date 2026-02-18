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
	googleDriveToken  string
	googleDriveClient = &http.Client{Timeout: 20 * time.Second}
)

func initGoogleDrive() {
	googleDriveToken = os.Getenv("GOOGLE_DRIVE_ACCESS_TOKEN")
}

func registerRoutes(r chi.Router) {
	r.Get("/files", handleGoogleDriveFiles)
	r.Get("/files/{id}", handleGoogleDriveFile)
}

func handleGoogleDriveFiles(w http.ResponseWriter, r *http.Request) {
	pageSize := parseGDInt(r.URL.Query().Get("page_size"), 20, 1, 1000)
	params := url.Values{
		"pageSize": {strconv.Itoa(pageSize)},
		"fields":   {"files(id,name,mimeType,size,modifiedTime,webViewLink),nextPageToken"},
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		params.Set("q", q)
	}
	if token := strings.TrimSpace(r.URL.Query().Get("page_token")); token != "" {
		params.Set("pageToken", token)
	}
	data, err := googleDriveGET("https://www.googleapis.com/drive/v3/files", params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleGoogleDriveFile(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	data, err := googleDriveGET("https://www.googleapis.com/drive/v3/files/"+url.PathEscape(id), url.Values{"fields": {"id,name,mimeType,size,modifiedTime,webViewLink,owners,permissions"}})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func googleDriveGET(baseURL string, params url.Values) (map[string]interface{}, error) {
	u := baseURL
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+googleDriveToken)
	req.Header.Set("Accept", "application/json")
	resp, err := googleDriveClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google Drive request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Google Drive response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Google Drive API error (%d): %s", resp.StatusCode, string(body))
	}
	return data, nil
}

func parseGDInt(raw string, fallback, min, max int) int {
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
