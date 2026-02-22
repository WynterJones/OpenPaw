package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	cfToken  string
	cfClient = &http.Client{Timeout: 15 * time.Second}
	cfBase   = "https://api.cloudflare.com/client/v4"
)

func initCloudflare(token string) {
	cfToken = token
}

func cfGet(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", cfBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfToken)
	req.Header.Set("Content-Type", "application/json")
	return cfClient.Do(req)
}

func cfPost(path string, payload interface{}) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", cfBase+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfToken)
	req.Header.Set("Content-Type", "application/json")
	return cfClient.Do(req)
}

// cfReadResult reads a Cloudflare API response and extracts the "result" field.
// Cloudflare wraps all responses in {"result": ..., "success": true, "errors": [], "messages": []}.
func cfReadResult(resp *http.Response) (interface{}, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var envelope struct {
		Result   interface{}        `json:"result"`
		Success  bool               `json:"success"`
		Errors   []cfError          `json:"errors"`
		Messages []cfMessage        `json:"messages"`
	}

	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if !envelope.Success {
		if len(envelope.Errors) > 0 {
			return nil, fmt.Errorf("Cloudflare API error: %s (code %d)", envelope.Errors[0].Message, envelope.Errors[0].Code)
		}
		return nil, fmt.Errorf("Cloudflare API error (%d): %s", resp.StatusCode, string(body))
	}

	return envelope.Result, nil
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func registerRoutes(r chi.Router) {
	r.Get("/zones", handleListZones)
	r.Get("/dns", handleListDNSRecords)
	r.Post("/dns", handleCreateDNSRecord)
	r.Get("/analytics", handleGetAnalytics)
}

func handleListZones(w http.ResponseWriter, r *http.Request) {
	resp, err := cfGet("/zones")
	if err != nil {
		writeError(w, http.StatusBadGateway, "Cloudflare API request failed")
		return
	}

	result, err := cfReadResult(resp)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	items, _ := result.([]interface{})
	zones := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		zone, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		zones = append(zones, map[string]interface{}{
			"id":           zone["id"],
			"name":         zone["name"],
			"status":       zone["status"],
			"paused":       zone["paused"],
			"type":         zone["type"],
			"name_servers": zone["name_servers"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(zones),
		"zones": zones,
	})
}

func handleListDNSRecords(w http.ResponseWriter, r *http.Request) {
	zoneID := r.URL.Query().Get("zone_id")
	if zoneID == "" {
		writeError(w, http.StatusBadRequest, "zone_id parameter is required")
		return
	}

	resp, err := cfGet(fmt.Sprintf("/zones/%s/dns_records", zoneID))
	if err != nil {
		writeError(w, http.StatusBadGateway, "Cloudflare API request failed")
		return
	}

	result, err := cfReadResult(resp)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	items, _ := result.([]interface{})
	records := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		rec, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		records = append(records, map[string]interface{}{
			"id":      rec["id"],
			"type":    rec["type"],
			"name":    rec["name"],
			"content": rec["content"],
			"ttl":     rec["ttl"],
			"proxied": rec["proxied"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"zone_id": zoneID,
		"count":   len(records),
		"records": records,
	})
}

type createDNSRequest struct {
	ZoneID  string `json:"zone_id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

func handleCreateDNSRecord(w http.ResponseWriter, r *http.Request) {
	var req createDNSRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ZoneID == "" || req.Type == "" || req.Name == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, "zone_id, type, name, and content are required")
		return
	}

	if req.TTL == 0 {
		req.TTL = 1 // 1 = automatic in Cloudflare
	}

	payload := map[string]interface{}{
		"type":    req.Type,
		"name":    req.Name,
		"content": req.Content,
		"ttl":     req.TTL,
	}

	resp, err := cfPost(fmt.Sprintf("/zones/%s/dns_records", req.ZoneID), payload)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Cloudflare API request failed")
		return
	}

	result, err := cfReadResult(resp)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	rec, _ := result.(map[string]interface{})
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":      rec["id"],
		"type":    rec["type"],
		"name":    rec["name"],
		"content": rec["content"],
		"ttl":     rec["ttl"],
	})
}

func handleGetAnalytics(w http.ResponseWriter, r *http.Request) {
	zoneID := r.URL.Query().Get("zone_id")
	if zoneID == "" {
		writeError(w, http.StatusBadRequest, "zone_id parameter is required")
		return
	}

	resp, err := cfGet(fmt.Sprintf("/zones/%s/analytics/dashboard?since=-1440", zoneID))
	if err != nil {
		writeError(w, http.StatusBadGateway, "Cloudflare API request failed")
		return
	}

	result, err := cfReadResult(resp)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	data, _ := result.(map[string]interface{})
	totals, _ := data["totals"].(map[string]interface{})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"zone_id": zoneID,
		"totals":  totals,
	})
}
