package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

var validRecordTypes = map[string]bool{
	"A":     true,
	"AAAA":  true,
	"MX":    true,
	"CNAME": true,
	"TXT":   true,
	"NS":    true,
	"SOA":   true,
	"PTR":   true,
}

type dnsResponse struct {
	Status   int  `json:"Status"`
	TC       bool `json:"TC"`
	RD       bool `json:"RD"`
	RA       bool `json:"RA"`
	AD       bool `json:"AD"`
	CD       bool `json:"CD"`
	Question []struct {
		Name string `json:"name"`
		Type int    `json:"type"`
	} `json:"Question"`
	Answer []struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		TTL  int    `json:"TTL"`
		Data string `json:"data"`
	} `json:"Answer"`
}

func registerRoutes(r chi.Router) {
	r.Get("/resolve", handleResolve)
	r.Get("/reverse", handleReverse)
}

func dnsStatusToString(status int) string {
	codes := map[int]string{
		0: "NOERROR",
		1: "FORMERR",
		2: "SERVFAIL",
		3: "NXDOMAIN",
		4: "NOTIMP",
		5: "REFUSED",
	}
	if s, ok := codes[status]; ok {
		return s
	}
	return fmt.Sprintf("UNKNOWN(%d)", status)
}

func dnsTypeToString(t int) string {
	types := map[int]string{
		1:   "A",
		2:   "NS",
		5:   "CNAME",
		6:   "SOA",
		12:  "PTR",
		15:  "MX",
		16:  "TXT",
		28:  "AAAA",
		257: "CAA",
	}
	if s, ok := types[t]; ok {
		return s
	}
	return fmt.Sprintf("TYPE%d", t)
}

func queryDNS(name, recordType string) (*dnsResponse, error) {
	u := fmt.Sprintf("https://dns.google/resolve?name=%s&type=%s",
		url.QueryEscape(name), url.QueryEscape(recordType))

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/dns-json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DNS request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DNS API error (%d): %s", resp.StatusCode, string(body))
	}

	var dnsResp dnsResponse
	if err := json.Unmarshal(body, &dnsResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &dnsResp, nil
}

func handleResolve(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name parameter is required")
		return
	}

	recordType := strings.ToUpper(r.URL.Query().Get("type"))
	if recordType == "" {
		recordType = "A"
	}

	if !validRecordTypes[recordType] {
		writeError(w, http.StatusBadRequest, "invalid record type; supported: A, AAAA, MX, CNAME, TXT, NS, SOA")
		return
	}

	dnsResp, err := queryDNS(name, recordType)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	questions := make([]map[string]interface{}, 0, len(dnsResp.Question))
	for _, q := range dnsResp.Question {
		questions = append(questions, map[string]interface{}{
			"name": q.Name,
			"type": dnsTypeToString(q.Type),
		})
	}

	answers := make([]map[string]interface{}, 0, len(dnsResp.Answer))
	for _, a := range dnsResp.Answer {
		answers = append(answers, map[string]interface{}{
			"name": a.Name,
			"type": dnsTypeToString(a.Type),
			"ttl":  a.TTL,
			"data": a.Data,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   dnsStatusToString(dnsResp.Status),
		"truncated": dnsResp.TC,
		"recursive_desired":   dnsResp.RD,
		"recursive_available": dnsResp.RA,
		"authenticated":       dnsResp.AD,
		"checking_disabled":   dnsResp.CD,
		"question": questions,
		"answer":   answers,
	})
}

func handleReverse(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		writeError(w, http.StatusBadRequest, "ip parameter is required")
		return
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		writeError(w, http.StatusBadRequest, "invalid IP address")
		return
	}

	// Convert IP to PTR format
	var ptrName string
	if parsed.To4() != nil {
		// IPv4: reverse octets and append .in-addr.arpa
		parts := strings.Split(parsed.To4().String(), ".")
		for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
			parts[i], parts[j] = parts[j], parts[i]
		}
		ptrName = strings.Join(parts, ".") + ".in-addr.arpa"
	} else {
		// IPv6: expand, reverse nibbles, append .ip6.arpa
		expanded := parsed.To16()
		hex := fmt.Sprintf("%032x", []byte(expanded))
		nibbles := make([]string, len(hex))
		for i, c := range hex {
			nibbles[len(hex)-1-i] = string(c)
		}
		ptrName = strings.Join(nibbles, ".") + ".ip6.arpa"
	}

	dnsResp, err := queryDNS(ptrName, "PTR")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	names := make([]string, 0, len(dnsResp.Answer))
	for _, a := range dnsResp.Answer {
		if dnsTypeToString(a.Type) == "PTR" {
			names = append(names, a.Data)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ip":     ip,
		"ptr":    ptrName,
		"names":  names,
		"status": dnsStatusToString(dnsResp.Status),
	})
}
