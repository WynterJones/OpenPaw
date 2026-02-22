package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

const randomUserBase = "https://randomuser.me/api/"

type randomUserAPIResponse struct {
	Results []struct {
		Gender string `json:"gender"`
		Name   struct {
			First string `json:"first"`
			Last  string `json:"last"`
		} `json:"name"`
		Location struct {
			City    string `json:"city"`
			State   string `json:"state"`
			Country string `json:"country"`
		} `json:"location"`
		Email   string `json:"email"`
		Login   struct {
			Username string `json:"username"`
		} `json:"login"`
		DOB struct {
			Date string `json:"date"`
			Age  int    `json:"age"`
		} `json:"dob"`
		Phone   string `json:"phone"`
		Picture struct {
			Large     string `json:"large"`
			Medium    string `json:"medium"`
			Thumbnail string `json:"thumbnail"`
		} `json:"picture"`
		Nat string `json:"nat"`
	} `json:"results"`
	Info struct {
		Results int `json:"results"`
	} `json:"info"`
}

func registerRoutes(r chi.Router) {
	r.Get("/generate", handleGenerate)
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	count := 1
	if c := r.URL.Query().Get("count"); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n >= 1 && n <= 50 {
			count = n
		} else if err == nil {
			writeError(w, http.StatusBadRequest, "count must be between 1 and 50")
			return
		}
	}

	u := fmt.Sprintf("%s?results=%d", randomUserBase, count)

	nat := r.URL.Query().Get("nat")
	if nat != "" {
		u += "&nat=" + nat
	}

	resp, err := httpClient.Get(u)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Random User API request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read response")
		return
	}

	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Random User API error (%d): %s", resp.StatusCode, string(body)))
		return
	}

	var apiResp randomUserAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}

	users := make([]map[string]interface{}, 0, len(apiResp.Results))
	for _, u := range apiResp.Results {
		users = append(users, map[string]interface{}{
			"name": map[string]string{
				"first": u.Name.First,
				"last":  u.Name.Last,
			},
			"email":  u.Email,
			"gender": u.Gender,
			"phone":  u.Phone,
			"location": map[string]string{
				"city":    u.Location.City,
				"state":   u.Location.State,
				"country": u.Location.Country,
			},
			"picture": map[string]string{
				"thumbnail": u.Picture.Thumbnail,
				"medium":    u.Picture.Medium,
				"large":     u.Picture.Large,
			},
			"login": map[string]string{
				"username": u.Login.Username,
			},
			"dob": map[string]interface{}{
				"date": u.DOB.Date,
				"age":  u.DOB.Age,
			},
			"nat": u.Nat,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(users),
		"users": users,
	})
}
