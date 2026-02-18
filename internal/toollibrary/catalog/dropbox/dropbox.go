package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	dropboxToken  string
	dropboxClient = &http.Client{Timeout: 20 * time.Second}
)

func initDropbox() {
	dropboxToken = os.Getenv("DROPBOX_ACCESS_TOKEN")
}

func registerRoutes(r chi.Router) {
	r.Post("/files/search", handleDropboxSearch)
	r.Post("/files/list-folder", handleDropboxListFolder)
}

func handleDropboxSearch(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(fmt.Sprint(req["query"])) == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	if _, ok := req["options"]; !ok {
		req["options"] = map[string]interface{}{"max_results": 20}
	}
	data, err := dropboxPost("https://api.dropboxapi.com/2/files/search_v2", req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleDropboxListFolder(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if _, ok := req["path"]; !ok {
		req["path"] = ""
	}
	if _, ok := req["limit"]; !ok {
		req["limit"] = 100
	}
	data, err := dropboxPost("https://api.dropboxapi.com/2/files/list_folder", req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func dropboxPost(endpoint string, payload map[string]interface{}) (map[string]interface{}, error) {
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+dropboxToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := dropboxClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Dropbox request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse Dropbox response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Dropbox API error (%d): %s", resp.StatusCode, string(body))
	}
	return data, nil
}
