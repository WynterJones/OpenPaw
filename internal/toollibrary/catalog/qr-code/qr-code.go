package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var qrClient = &http.Client{Timeout: 15 * time.Second}

func registerRoutes(r chi.Router) {
	r.Get("/generate", handleGenerate)
	r.Get("/read", handleRead)
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	data := strings.TrimSpace(r.URL.Query().Get("data"))
	if data == "" {
		writeError(w, http.StatusBadRequest, "data parameter is required")
		return
	}

	size := 300
	if sizeStr := r.URL.Query().Get("size"); sizeStr != "" {
		if n, err := strconv.Atoi(sizeStr); err == nil && n >= 10 && n <= 1000 {
			size = n
		}
	}

	qrURL := fmt.Sprintf(
		"https://api.qrserver.com/v1/create-qr-code/?size=%dx%d&data=%s",
		size, size,
		url.QueryEscape(data),
	)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"url":  qrURL,
		"data": data,
		"size": size,
	})
}

func handleRead(w http.ResponseWriter, r *http.Request) {
	imageURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if imageURL == "" {
		writeError(w, http.StatusBadRequest, "url parameter is required")
		return
	}

	apiURL := fmt.Sprintf(
		"https://api.qrserver.com/v1/read-qr-code/?fileurl=%s",
		url.QueryEscape(imageURL),
	)

	resp, err := qrClient.Get(apiURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "QR Server API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read response")
		return
	}

	if resp.StatusCode >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("QR Server API error (%d)", resp.StatusCode))
		return
	}

	// The API returns an array of objects with symbol arrays
	var parsed []struct {
		Type   string `json:"type"`
		Symbol []struct {
			Data  string `json:"data"`
			Error string `json:"error"`
		} `json:"symbol"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}

	if len(parsed) == 0 || len(parsed[0].Symbol) == 0 {
		writeError(w, http.StatusNotFound, "no QR code found in image")
		return
	}

	symbol := parsed[0].Symbol[0]
	if symbol.Error != "" {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("QR decode error: %s", symbol.Error))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": symbol.Data,
		"type": parsed[0].Type,
	})
}
