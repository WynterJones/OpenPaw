package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const elevenLabsAPI = "https://api.elevenlabs.io/v1"

// ProviderElevenLabs is text-to-speech. It is the odd one out among the
// providers: elsewhere a "model" is a generator and the prompt describes what
// to make, but here the prompt IS the script and the "model" is the voice that
// reads it. Mapping voices onto the model picker keeps the editor unchanged —
// picking a voice works exactly like picking a model.
type ProviderElevenLabs struct {
	keyFn func() string
	http  *http.Client

	mu        sync.RWMutex
	voices    []Model
	fetchedAt time.Time
}

func NewElevenLabsProvider(keyFn func() string) *ProviderElevenLabs {
	return &ProviderElevenLabs{
		keyFn: keyFn,
		http:  &http.Client{Timeout: 3 * time.Minute},
	}
}

func (p *ProviderElevenLabs) Name() string { return "elevenlabs" }

func (p *ProviderElevenLabs) Configured() bool { return p.key() != "" }

func (p *ProviderElevenLabs) key() string {
	if p.keyFn == nil {
		return ""
	}
	return strings.TrimSpace(p.keyFn())
}

// Kinds is audio only — ElevenLabs makes no images or video.
func (p *ProviderElevenLabs) Kinds() []Kind { return []Kind{KindAudio} }

// ttsModel is the synthesis model used for every voice. v3 is the current
// flagship; the voice, not this, is what the user chooses.
const elevenLabsTTSModel = "eleven_multilingual_v2"

// voiceCacheTTL keeps the picker responsive without pinning a stale voice list
// after the user adds or clones a voice in the ElevenLabs dashboard.
const voiceCacheTTL = 5 * time.Minute

func (p *ProviderElevenLabs) Models(ctx context.Context, kind Kind) ([]Model, error) {
	if kind != KindAudio {
		return nil, nil
	}
	if !p.Configured() {
		return nil, fmt.Errorf("elevenlabs is not configured")
	}

	p.mu.RLock()
	fresh := time.Since(p.fetchedAt) < voiceCacheTTL && len(p.voices) > 0
	cached := p.voices
	p.mu.RUnlock()
	if fresh {
		return cached, nil
	}

	voices, err := p.fetchVoices(ctx)
	if err != nil {
		// Serve a stale list rather than an empty picker when the API blips.
		if len(cached) > 0 {
			return cached, nil
		}
		return nil, err
	}

	p.mu.Lock()
	p.voices = voices
	p.fetchedAt = time.Now()
	p.mu.Unlock()

	return voices, nil
}

func (p *ProviderElevenLabs) fetchVoices(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, elevenLabsAPI+"/voices", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", p.key())

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("elevenlabs returned %d: %s", resp.StatusCode, describeAPIError(raw))
	}

	var body struct {
		Voices []struct {
			VoiceID     string            `json:"voice_id"`
			Name        string            `json:"name"`
			Category    string            `json:"category"`
			Description string            `json:"description"`
			Labels      map[string]string `json:"labels"`
		} `json:"voices"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("could not parse elevenlabs voices: %w", err)
	}

	out := make([]Model, 0, len(body.Voices))
	for _, v := range body.Voices {
		out = append(out, Model{
			ID:          v.VoiceID,
			Name:        v.Name,
			Provider:    p.Name(),
			Kind:        KindAudio,
			Description: describeVoice(v.Description, v.Category, v.Labels),
		})
	}

	// Cloned and generated voices are the user's own, so they lead; the stock
	// library follows alphabetically.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// describeVoice builds a one-line summary from whichever metadata the voice
// happens to carry — ElevenLabs populates these inconsistently.
func describeVoice(description, category string, labels map[string]string) string {
	if description != "" {
		return truncate(description, 160)
	}

	var parts []string
	for _, key := range []string{"accent", "gender", "age", "use_case", "description"} {
		if v := strings.TrimSpace(labels[key]); v != "" {
			parts = append(parts, v)
		}
	}
	if category != "" && category != "premade" {
		parts = append(parts, category)
	}
	return strings.Join(parts, " · ")
}

func (p *ProviderElevenLabs) Generate(ctx context.Context, req Request) (*Asset, error) {
	if req.Kind != KindAudio {
		return nil, fmt.Errorf("elevenlabs only produces audio")
	}
	if !p.Configured() {
		return nil, fmt.Errorf("elevenlabs is not configured — add an API key in Settings")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf("nothing to speak — the prompt is the script")
	}

	voiceID := req.Model
	if voiceID == "" {
		// Fall back to the first available voice so a bare request still works.
		voices, err := p.Models(ctx, KindAudio)
		if err != nil || len(voices) == 0 {
			return nil, fmt.Errorf("no voice selected and none could be listed")
		}
		voiceID = voices[0].ID
	}

	modelID := elevenLabsTTSModel
	if v, ok := req.Params["model_id"].(string); ok && v != "" {
		modelID = v
	}

	payload := map[string]interface{}{
		"text":     req.Prompt,
		"model_id": modelID,
	}
	if vs, ok := req.Params["voice_settings"]; ok {
		payload["voice_settings"] = vs
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/text-to-speech/%s", elevenLabsAPI, voiceID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("xi-api-key", p.key())
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "audio/mpeg")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs request failed: %w", err)
	}
	defer resp.Body.Close()

	// Unlike Replicate and fal, this returns the audio bytes directly rather
	// than a URL to poll and download.
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read elevenlabs audio: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("elevenlabs returned %d: %s", resp.StatusCode, describeAPIError(data))
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("elevenlabs returned no audio")
	}
	if len(data) > maxAssetBytes {
		return nil, fmt.Errorf("generated audio exceeds the %d MB limit", maxAssetBytes>>20)
	}

	mimeType := resp.Header.Get("Content-Type")
	if idx := strings.Index(mimeType, ";"); idx >= 0 {
		mimeType = mimeType[:idx]
	}
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "audio/mpeg"
	}

	return &Asset{
		Data:     data,
		MimeType: mimeType,
		Ext:      extensionFor(mimeType, KindAudio),
	}, nil
}
