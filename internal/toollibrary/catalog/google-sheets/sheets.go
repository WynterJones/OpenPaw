package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

var sheetsAPIKey string

func initSheets(apiKey string) {
	sheetsAPIKey = apiKey
}

func registerRoutes(r chi.Router) {
	r.Get("/read", handleRead)
	r.Post("/write", handleWrite)
	r.Get("/info", handleInfo)
}

func handleRead(w http.ResponseWriter, r *http.Request) {
	spreadsheetID := r.URL.Query().Get("spreadsheet_id")
	cellRange := r.URL.Query().Get("range")
	if spreadsheetID == "" || cellRange == "" {
		writeError(w, http.StatusBadRequest, "spreadsheet_id and range are required")
		return
	}

	apiURL := fmt.Sprintf(
		"https://sheets.googleapis.com/v4/spreadsheets/%s/values/%s?key=%s",
		url.PathEscape(spreadsheetID),
		url.PathEscape(cellRange),
		url.QueryEscape(sheetsAPIKey),
	)

	resp, err := http.Get(apiURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("sheets API error: %v", err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		writeError(w, resp.StatusCode, fmt.Sprintf("sheets API returned %d: %s", resp.StatusCode, string(body)))
		return
	}

	var result struct {
		Range  string          `json:"range"`
		Values [][]interface{} `json:"values"`
	}
	json.Unmarshal(body, &result)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"range":  result.Range,
		"values": result.Values,
	})
}

func handleWrite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SpreadsheetID string          `json:"spreadsheet_id"`
		Range         string          `json:"range"`
		Values        [][]interface{} `json:"values"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SpreadsheetID == "" || req.Range == "" {
		writeError(w, http.StatusBadRequest, "spreadsheet_id and range are required")
		return
	}

	apiURL := fmt.Sprintf(
		"https://sheets.googleapis.com/v4/spreadsheets/%s/values/%s?valueInputOption=USER_ENTERED&key=%s",
		url.PathEscape(req.SpreadsheetID),
		url.PathEscape(req.Range),
		url.QueryEscape(sheetsAPIKey),
	)

	payload := map[string]interface{}{
		"range":  req.Range,
		"values": req.Values,
	}
	payloadJSON, _ := json.Marshal(payload)

	httpReq, _ := http.NewRequest("PUT", apiURL, bytes.NewReader(payloadJSON))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("sheets API error: %v", err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		writeError(w, resp.StatusCode, fmt.Sprintf("sheets API returned %d: %s", resp.StatusCode, string(body)))
		return
	}

	var result struct {
		UpdatedCells int `json:"updatedCells"`
	}
	json.Unmarshal(body, &result)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"updated_cells": result.UpdatedCells,
		"status":        "success",
	})
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	spreadsheetID := r.URL.Query().Get("spreadsheet_id")
	if spreadsheetID == "" {
		writeError(w, http.StatusBadRequest, "spreadsheet_id is required")
		return
	}

	apiURL := fmt.Sprintf(
		"https://sheets.googleapis.com/v4/spreadsheets/%s?key=%s&fields=properties.title,sheets.properties.title",
		url.PathEscape(spreadsheetID),
		url.QueryEscape(sheetsAPIKey),
	)

	resp, err := http.Get(apiURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("sheets API error: %v", err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		writeError(w, resp.StatusCode, fmt.Sprintf("sheets API returned %d: %s", resp.StatusCode, string(body)))
		return
	}

	var result struct {
		Properties struct {
			Title string `json:"title"`
		} `json:"properties"`
		Sheets []struct {
			Properties struct {
				Title string `json:"title"`
			} `json:"properties"`
		} `json:"sheets"`
	}
	json.Unmarshal(body, &result)

	var sheets []string
	for _, s := range result.Sheets {
		sheets = append(sheets, s.Properties.Title)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"title":  result.Properties.Title,
		"sheets": sheets,
	})
}
