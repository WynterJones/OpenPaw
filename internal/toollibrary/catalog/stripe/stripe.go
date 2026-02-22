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

var (
	stripeKey    string
	stripeClient = &http.Client{Timeout: 15 * time.Second}
	stripeBase   = "https://api.stripe.com/v1"
)

func initStripe(key string) {
	stripeKey = key
}

func stripeGet(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", stripeBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(stripeKey, "")
	return stripeClient.Do(req)
}

func stripePost(path string, values url.Values) (*http.Response, error) {
	req, err := http.NewRequest("POST", stripeBase+path, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(stripeKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return stripeClient.Do(req)
}

func stripeReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Stripe API error (%d): %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, v)
}

func registerRoutes(r chi.Router) {
	r.Get("/customers", handleListCustomers)
	r.Post("/customer", handleCreateCustomer)
	r.Get("/charges", handleListCharges)
	r.Get("/balance", handleGetBalance)
	r.Get("/invoices", handleListInvoices)
}

func handleListCustomers(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}

	path := fmt.Sprintf("/customers?limit=%d", limit)
	if email := r.URL.Query().Get("email"); email != "" {
		path += "&email=" + url.QueryEscape(email)
	}

	resp, err := stripeGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Stripe API request failed")
		return
	}

	var result map[string]interface{}
	if err := stripeReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	data, _ := result["data"].([]interface{})
	customers := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		cust, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		customers = append(customers, map[string]interface{}{
			"id":       cust["id"],
			"email":    cust["email"],
			"name":     cust["name"],
			"created":  cust["created"],
			"currency": cust["currency"],
			"balance":  cust["balance"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":     len(customers),
		"customers": customers,
	})
}

type createCustomerRequest struct {
	Email       string `json:"email"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func handleCreateCustomer(w http.ResponseWriter, r *http.Request) {
	var req createCustomerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	values := url.Values{}
	values.Set("email", req.Email)
	if req.Name != "" {
		values.Set("name", req.Name)
	}
	if req.Description != "" {
		values.Set("description", req.Description)
	}

	resp, err := stripePost("/customers", values)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Stripe API request failed")
		return
	}

	var customer map[string]interface{}
	if err := stripeReadJSON(resp, &customer); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":    customer["id"],
		"email": customer["email"],
		"name":  customer["name"],
	})
}

func handleListCharges(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}

	path := fmt.Sprintf("/charges?limit=%d", limit)
	if customer := r.URL.Query().Get("customer"); customer != "" {
		path += "&customer=" + url.QueryEscape(customer)
	}

	resp, err := stripeGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Stripe API request failed")
		return
	}

	var result map[string]interface{}
	if err := stripeReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	data, _ := result["data"].([]interface{})
	charges := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		charge, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Convert amount from cents to dollars
		amountCents, _ := charge["amount"].(float64)
		amountDollars := amountCents / 100.0

		charges = append(charges, map[string]interface{}{
			"id":          charge["id"],
			"amount":      amountDollars,
			"currency":    charge["currency"],
			"status":      charge["status"],
			"description": charge["description"],
			"customer":    charge["customer"],
			"created":     charge["created"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":   len(charges),
		"charges": charges,
	})
}

func handleGetBalance(w http.ResponseWriter, r *http.Request) {
	resp, err := stripeGet("/balance")
	if err != nil {
		writeError(w, http.StatusBadGateway, "Stripe API request failed")
		return
	}

	var result map[string]interface{}
	if err := stripeReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	available := extractBalances(result, "available")
	pending := extractBalances(result, "pending")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"available": available,
		"pending":   pending,
	})
}

func handleListInvoices(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}

	path := fmt.Sprintf("/invoices?limit=%d", limit)
	if customer := r.URL.Query().Get("customer"); customer != "" {
		path += "&customer=" + url.QueryEscape(customer)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		path += "&status=" + url.QueryEscape(status)
	}

	resp, err := stripeGet(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Stripe API request failed")
		return
	}

	var result map[string]interface{}
	if err := stripeReadJSON(resp, &result); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	data, _ := result["data"].([]interface{})
	invoices := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		inv, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		invoices = append(invoices, map[string]interface{}{
			"id":          inv["id"],
			"customer":    inv["customer"],
			"amount_due":  inv["amount_due"],
			"amount_paid": inv["amount_paid"],
			"currency":    inv["currency"],
			"status":      inv["status"],
			"created":     inv["created"],
			"due_date":    inv["due_date"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":    len(invoices),
		"invoices": invoices,
	})
}

func extractBalances(result map[string]interface{}, key string) []map[string]interface{} {
	balances := make([]map[string]interface{}, 0)
	items, ok := result[key].([]interface{})
	if !ok {
		return balances
	}
	for _, item := range items {
		b, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		balances = append(balances, map[string]interface{}{
			"amount":   b["amount"],
			"currency": b["currency"],
		})
	}
	return balances
}
