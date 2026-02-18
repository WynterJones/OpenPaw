package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	elevenLabsKey    string
	elevenLabsClient = &http.Client{Timeout: 60 * time.Second}
	elevenLabsBase   = "https://api.elevenlabs.io/v1"
)

func initElevenLabs(key string) {
	elevenLabsKey = key
}

func registerRoutes(r chi.Router) {
	r.Get("/voices", handleListVoices)
	r.Post("/tts", handleTTS)
	r.Get("/models", handleListModels)
}

func elevenLabsRequest(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, elevenLabsBase+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", elevenLabsKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return elevenLabsClient.Do(req)
}

func handleListVoices(w http.ResponseWriter, r *http.Request) {
	resp, err := elevenLabsRequest("GET", "/voices", nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("ElevenLabs API request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeError(w, resp.StatusCode, fmt.Sprintf("ElevenLabs API error: %s", string(body)))
		return
	}

	var result struct {
		Voices []struct {
			VoiceID  string            `json:"voice_id"`
			Name     string            `json:"name"`
			Category string            `json:"category"`
			Labels   map[string]string `json:"labels"`
		} `json:"voices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse voices response")
		return
	}

	voices := make([]map[string]interface{}, 0, len(result.Voices))
	for _, v := range result.Voices {
		voices = append(voices, map[string]interface{}{
			"voice_id": v.VoiceID,
			"name":     v.Name,
			"category": v.Category,
			"labels":   v.Labels,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"voices": voices})
}

type ttsRequest struct {
	Text            string  `json:"text"`
	VoiceID         string  `json:"voice_id"`
	ModelID         string  `json:"model_id"`
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
}

func handleTTS(w http.ResponseWriter, r *http.Request) {
	var req ttsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	if req.VoiceID == "" {
		req.VoiceID = "21m00Tcm4TlvDq8ikWAM"
	}
	if req.ModelID == "" {
		req.ModelID = "eleven_monolingual_v1"
	}
	if req.Stability == 0 {
		req.Stability = 0.5
	}
	if req.SimilarityBoost == 0 {
		req.SimilarityBoost = 0.75
	}

	apiBody := map[string]interface{}{
		"text":     req.Text,
		"model_id": req.ModelID,
		"voice_settings": map[string]interface{}{
			"stability":        req.Stability,
			"similarity_boost": req.SimilarityBoost,
		},
	}

	bodyBytes, err := json.Marshal(apiBody)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to marshal request body")
		return
	}

	apiReq, err := http.NewRequest("POST", fmt.Sprintf("%s/text-to-speech/%s", elevenLabsBase, req.VoiceID), bytes.NewReader(bodyBytes))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}
	apiReq.Header.Set("xi-api-key", elevenLabsKey)
	apiReq.Header.Set("Content-Type", "application/json")
	apiReq.Header.Set("Accept", "audio/mpeg")

	resp, err := elevenLabsClient.Do(apiReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("ElevenLabs TTS request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeError(w, resp.StatusCode, fmt.Sprintf("ElevenLabs TTS error: %s", string(body)))
		return
	}

	tmpFile, err := os.CreateTemp("", "elevenlabs-*.mp3")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create temp file")
		return
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		writeError(w, http.StatusInternalServerError, "failed to save audio data")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"file_path":    tmpFile.Name(),
		"voice_id":     req.VoiceID,
		"characters":   len(req.Text),
		"content_type": "audio/mpeg",
	})
}

func handleListModels(w http.ResponseWriter, r *http.Request) {
	resp, err := elevenLabsRequest("GET", "/models", nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("ElevenLabs API request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeError(w, resp.StatusCode, fmt.Sprintf("ElevenLabs API error: %s", string(body)))
		return
	}

	var models []struct {
		ModelID     string `json:"model_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Languages   []struct {
			LanguageID string `json:"language_id"`
			Name       string `json:"name"`
		} `json:"languages"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse models response")
		return
	}

	result := make([]map[string]interface{}, 0, len(models))
	for _, m := range models {
		langs := make([]map[string]string, 0, len(m.Languages))
		for _, l := range m.Languages {
			langs = append(langs, map[string]string{
				"language_id": l.LanguageID,
				"name":        l.Name,
			})
		}
		result = append(result, map[string]interface{}{
			"model_id":    m.ModelID,
			"name":        m.Name,
			"description": m.Description,
			"languages":   langs,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"models": result})
}
