package main

import (
	"bytes"
	"encoding/hex"
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
	minimaxKey     string
	minimaxGroupID string
	minimaxClient  = &http.Client{Timeout: 60 * time.Second}
	audioStore     = &audioFileStore{files: make(map[string]string)}
	audioDir       string
)

type audioFileStore struct {
	mu    sync.RWMutex
	files map[string]string // filename -> full path
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

func initMiniMax(key, groupID string) {
	minimaxKey = key
	minimaxGroupID = groupID

	// Set up persistent audio directory
	if dir := os.Getenv("TOOL_DATA_DIR"); dir != "" {
		audioDir = filepath.Join(dir, "minimax-tts")
	} else {
		audioDir = filepath.Join(os.TempDir(), "minimax-tts")
	}
	os.MkdirAll(audioDir, 0755)

	// Scan for existing audio files to restore the in-memory store
	entries, _ := os.ReadDir(audioDir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".mp3") {
			audioStore.Add(e.Name(), filepath.Join(audioDir, e.Name()))
		}
	}
}

func registerRoutes(r chi.Router) {
	r.Post("/tts", handleTTS)
	r.Get("/voices", handleListVoices)
	r.Get("/audio/{filename}", handleServeAudio)
}

type minimaxTTSRequest struct {
	Text            string  `json:"text"`
	VoiceID         string  `json:"voice_id"`
	Speed           float64 `json:"speed"`
	Pitch           int     `json:"pitch"`
	Vol             float64 `json:"vol"`
	AudioSampleRate int     `json:"audio_sample_rate"`
}

func handleTTS(w http.ResponseWriter, r *http.Request) {
	var req minimaxTTSRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	if req.VoiceID == "" {
		req.VoiceID = "male-qn-qingse"
	}
	if req.Speed == 0 {
		req.Speed = 1.0
	}
	if req.Vol == 0 {
		req.Vol = 1.0
	}
	if req.AudioSampleRate == 0 {
		req.AudioSampleRate = 32000
	}

	apiBody := map[string]interface{}{
		"model": "speech-01-turbo",
		"text":  req.Text,
		"voice_setting": map[string]interface{}{
			"voice_id": req.VoiceID,
			"speed":    req.Speed,
			"pitch":    req.Pitch,
			"vol":      req.Vol,
		},
		"audio_setting": map[string]interface{}{
			"sample_rate": req.AudioSampleRate,
			"format":      "mp3",
		},
	}

	bodyBytes, err := json.Marshal(apiBody)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to marshal request body")
		return
	}

	apiURL := fmt.Sprintf("https://api.minimaxi.chat/v1/t2a_v2?GroupId=%s", minimaxGroupID)
	apiReq, err := http.NewRequest("POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}
	apiReq.Header.Set("Authorization", "Bearer "+minimaxKey)
	apiReq.Header.Set("Content-Type", "application/json")

	resp, err := minimaxClient.Do(apiReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("MiniMax API request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeError(w, resp.StatusCode, fmt.Sprintf("MiniMax API error: %s", string(body)))
		return
	}

	var apiResp struct {
		Data struct {
			Audio string `json:"audio"`
		} `json:"data"`
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse MiniMax response")
		return
	}

	if apiResp.BaseResp.StatusCode != 0 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("MiniMax error: %s", apiResp.BaseResp.StatusMsg))
		return
	}

	if apiResp.Data.Audio == "" {
		writeError(w, http.StatusInternalServerError, "MiniMax returned empty audio data")
		return
	}

	audioData, err := hex.DecodeString(apiResp.Data.Audio)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decode hex audio data")
		return
	}

	filename := fmt.Sprintf("minimax-%d.mp3", time.Now().UnixNano())
	filePath := filepath.Join(audioDir, filename)
	if err := os.WriteFile(filePath, audioData, 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save audio data")
		return
	}
	audioStore.Add(filename, filePath)

	// Truncate long text for display
	displayText := req.Text
	if len(displayText) > 80 {
		displayText = displayText[:77] + "..."
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"audio_file":   filename,
		"voice_id":     req.VoiceID,
		"content_type": "audio/mpeg",
		"text":         displayText,
		"__widget": map[string]string{
			"type":  "audio-player",
			"title": "MiniMax TTS",
		},
	})
}

func handleServeAudio(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")

	// Sanitize filename to prevent path traversal
	filename = filepath.Base(filename)
	if !strings.HasPrefix(filename, "minimax-") || !strings.HasSuffix(filename, ".mp3") {
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

func handleListVoices(w http.ResponseWriter, r *http.Request) {
	voices := []map[string]string{
		{"voice_id": "male-qn-qingse", "description": "Male - Youthful and clear"},
		{"voice_id": "male-qn-jingying", "description": "Male - Elite and polished"},
		{"voice_id": "male-qn-badao", "description": "Male - Bold and dominant"},
		{"voice_id": "male-qn-daxuesheng", "description": "Male - College student"},
		{"voice_id": "female-shaonv", "description": "Female - Young girl"},
		{"voice_id": "female-yujie", "description": "Female - Mature and elegant"},
		{"voice_id": "female-chengshu", "description": "Female - Composed and steady"},
		{"voice_id": "female-tianmei", "description": "Female - Sweet and gentle"},
		{"voice_id": "presenter_male", "description": "Male - Professional presenter"},
		{"voice_id": "presenter_female", "description": "Female - Professional presenter"},
		{"voice_id": "audiobook_male_1", "description": "Male - Audiobook narrator 1"},
		{"voice_id": "audiobook_male_2", "description": "Male - Audiobook narrator 2"},
		{"voice_id": "audiobook_female_1", "description": "Female - Audiobook narrator 1"},
		{"voice_id": "audiobook_female_2", "description": "Female - Audiobook narrator 2"},
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"voices": voices})
}
