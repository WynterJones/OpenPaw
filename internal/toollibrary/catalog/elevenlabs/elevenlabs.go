package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	elevenLabsKey    string
	elevenLabsClient = &http.Client{Timeout: 60 * time.Second}
	elevenLabsBase   = "https://api.elevenlabs.io/v1"
	audioStore       = &audioFileStore{files: make(map[string]string)}
	audioDir         string
)

type audioFileStore struct {
	mu    sync.RWMutex
	files map[string]string
}

func (s *audioFileStore) Add(filename, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[filename] = path
}

func (s *audioFileStore) Get(filename string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.files[filename]
	return p, ok
}

func initElevenLabs(key string) {
	elevenLabsKey = key

	// Set up persistent audio directory
	if dir := os.Getenv("TOOL_DATA_DIR"); dir != "" {
		audioDir = filepath.Join(dir, "elevenlabs")
	} else {
		audioDir = filepath.Join(os.TempDir(), "elevenlabs")
	}
	os.MkdirAll(audioDir, 0755)

	// Scan for existing audio files to restore the in-memory store
	entries, _ := os.ReadDir(audioDir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "elevenlabs-") && strings.HasSuffix(e.Name(), ".mp3") {
			audioStore.Add(e.Name(), filepath.Join(audioDir, e.Name()))
		}
	}
}

func registerRoutes(r chi.Router) {
	r.Get("/voices", handleListVoices)
	r.Post("/tts", handleTTS)
	r.Get("/models", handleListModels)
	r.Get("/audio/{filename}", handleServeAudio)
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

	filename := fmt.Sprintf("elevenlabs-%d.mp3", time.Now().UnixNano())
	filePath := filepath.Join(audioDir, filename)
	outFile, err := os.Create(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create audio file")
		return
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		os.Remove(filePath)
		writeError(w, http.StatusInternalServerError, "failed to save audio data")
		return
	}

	audioStore.Add(filename, filePath)

	displayText := req.Text
	if len(displayText) > 80 {
		displayText = displayText[:77] + "..."
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"audio_file":   filename,
		"voice_id":     req.VoiceID,
		"characters":   len(req.Text),
		"content_type": "audio/mpeg",
		"text":         displayText,
		"__widget": map[string]string{
			"type":  "audio-player",
			"title": "ElevenLabs TTS",
		},
	})
}

func handleServeAudio(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")

	filename = filepath.Base(filename)
	if !strings.HasPrefix(filename, "elevenlabs-") || !strings.HasSuffix(filename, ".mp3") {
		writeError(w, http.StatusBadRequest, "invalid audio filename")
		return
	}

	fullPath, ok := audioStore.Get(filename)
	if !ok {
		writeError(w, http.StatusNotFound, "audio file not found")
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	http.ServeFile(w, r, fullPath)
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
