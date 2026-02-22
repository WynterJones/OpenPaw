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
	openaiTTSKey    string
	openaiTTSClient = &http.Client{Timeout: 60 * time.Second}
	openaiTTSBase   = "https://api.openai.com/v1"
	audioStore      = &audioFileStore{files: make(map[string]string)}
	audioDir        string
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

func initOpenAITTS(key string) {
	openaiTTSKey = key

	// Set up persistent audio directory
	if dir := os.Getenv("TOOL_DATA_DIR"); dir != "" {
		audioDir = filepath.Join(dir, "openai-tts")
	} else {
		audioDir = filepath.Join(os.TempDir(), "openai-tts")
	}
	os.MkdirAll(audioDir, 0755)

	// Scan for existing audio files to restore the in-memory store
	entries, _ := os.ReadDir(audioDir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "openai-tts-") {
			audioStore.Add(e.Name(), filepath.Join(audioDir, e.Name()))
		}
	}
}

func registerRoutes(r chi.Router) {
	r.Post("/tts", handleTTS)
	r.Get("/voices", handleListVoices)
	r.Get("/audio/{filename}", handleServeAudio)
}

type openaiTTSRequest struct {
	Text           string  `json:"text"`
	Voice          string  `json:"voice"`
	Model          string  `json:"model"`
	Speed          float64 `json:"speed"`
	ResponseFormat string  `json:"response_format"`
}

func handleTTS(w http.ResponseWriter, r *http.Request) {
	var req openaiTTSRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	if req.Voice == "" {
		req.Voice = "alloy"
	}
	if req.Model == "" {
		req.Model = "tts-1"
	}
	if req.Speed == 0 {
		req.Speed = 1.0
	}
	if req.ResponseFormat == "" {
		req.ResponseFormat = "mp3"
	}

	validVoices := map[string]bool{
		"alloy": true, "echo": true, "fable": true,
		"onyx": true, "nova": true, "shimmer": true,
	}
	if !validVoices[req.Voice] {
		writeError(w, http.StatusBadRequest, "invalid voice: must be alloy, echo, fable, onyx, nova, or shimmer")
		return
	}

	if req.Model != "tts-1" && req.Model != "tts-1-hd" {
		writeError(w, http.StatusBadRequest, "invalid model: must be tts-1 or tts-1-hd")
		return
	}

	if req.Speed < 0.25 || req.Speed > 4.0 {
		writeError(w, http.StatusBadRequest, "speed must be between 0.25 and 4.0")
		return
	}

	validFormats := map[string]bool{
		"mp3": true, "opus": true, "aac": true, "flac": true,
	}
	if !validFormats[req.ResponseFormat] {
		writeError(w, http.StatusBadRequest, "invalid response_format: must be mp3, opus, aac, or flac")
		return
	}

	apiBody := map[string]interface{}{
		"model":           req.Model,
		"input":           req.Text,
		"voice":           req.Voice,
		"speed":           req.Speed,
		"response_format": req.ResponseFormat,
	}

	bodyBytes, err := json.Marshal(apiBody)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to marshal request body")
		return
	}

	apiReq, err := http.NewRequest("POST", openaiTTSBase+"/audio/speech", bytes.NewReader(bodyBytes))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}
	apiReq.Header.Set("Authorization", "Bearer "+openaiTTSKey)
	apiReq.Header.Set("Content-Type", "application/json")

	resp, err := openaiTTSClient.Do(apiReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("OpenAI TTS request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeError(w, resp.StatusCode, fmt.Sprintf("OpenAI TTS error: %s", string(body)))
		return
	}

	extMap := map[string]string{
		"mp3": ".mp3", "opus": ".opus", "aac": ".aac", "flac": ".flac",
	}
	contentTypeMap := map[string]string{
		"mp3": "audio/mpeg", "opus": "audio/opus", "aac": "audio/aac", "flac": "audio/flac",
	}

	ext := extMap[req.ResponseFormat]
	contentType := contentTypeMap[req.ResponseFormat]

	filename := fmt.Sprintf("openai-tts-%d%s", time.Now().UnixNano(), ext)
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
		"voice":        req.Voice,
		"model":        req.Model,
		"content_type": contentType,
		"text":         displayText,
		"__widget": map[string]string{
			"type":  "audio-player",
			"title": "OpenAI TTS",
		},
	})
}

func handleServeAudio(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")

	filename = filepath.Base(filename)
	if !strings.HasPrefix(filename, "openai-tts-") {
		writeError(w, http.StatusBadRequest, "invalid audio filename")
		return
	}

	fullPath, ok := audioStore.Get(filename)
	if !ok {
		writeError(w, http.StatusNotFound, "audio file not found")
		return
	}

	ext := filepath.Ext(fullPath)
	contentTypes := map[string]string{
		".mp3": "audio/mpeg", ".opus": "audio/opus", ".aac": "audio/aac", ".flac": "audio/flac",
	}
	if ct, ok := contentTypes[ext]; ok {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeFile(w, r, fullPath)
}

func handleListVoices(w http.ResponseWriter, r *http.Request) {
	voices := []map[string]string{
		{"voice_id": "alloy", "description": "Neutral and balanced"},
		{"voice_id": "echo", "description": "Male voice"},
		{"voice_id": "fable", "description": "British accent"},
		{"voice_id": "onyx", "description": "Deep male voice"},
		{"voice_id": "nova", "description": "Female voice"},
		{"voice_id": "shimmer", "description": "Soft female voice"},
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"voices": voices})
}
