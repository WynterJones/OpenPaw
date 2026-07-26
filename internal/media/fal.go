package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const falQueueAPI = "https://queue.fal.run"

// ProviderFal runs models on fal.ai. It overlaps heavily with Replicate; fal
// is generally faster and cheaper for video, so both are offered and the user
// picks per generation.
type ProviderFal struct {
	keyFn func() string
	http  *http.Client
}

func NewFalProvider(keyFn func() string) *ProviderFal {
	return &ProviderFal{
		keyFn: keyFn,
		http:  &http.Client{Timeout: 10 * time.Minute},
	}
}

func (p *ProviderFal) Name() string { return "fal" }

func (p *ProviderFal) Configured() bool { return p.key() != "" }

func (p *ProviderFal) key() string {
	if p.keyFn == nil {
		return ""
	}
	return strings.TrimSpace(p.keyFn())
}

func (p *ProviderFal) Kinds() []Kind { return []Kind{KindImage, KindVideo, KindAudio} }

// falCatalog mirrors replicateCatalog: a curated starting set, with custom
// model IDs accepted from the UI for anything not listed.
var falCatalog = map[Kind][]Model{
	KindImage: {
		{ID: "fal-ai/flux-pro/v1.1-ultra", Name: "FLUX 1.1 Pro Ultra", Description: "Highest resolution FLUX tier."},
		{ID: "fal-ai/flux/schnell", Name: "FLUX Schnell", Description: "Very fast drafts, low cost."},
		{ID: "fal-ai/flux/dev", Name: "FLUX Dev", Description: "Balanced quality and speed."},
		{ID: "fal-ai/recraft-v3", Name: "Recraft v3", Description: "Design and vector-style output."},
		{ID: "fal-ai/ideogram/v3", Name: "Ideogram v3", Description: "Strong text and logo rendering."},
	},
	KindVideo: {
		{ID: "fal-ai/veo3", Name: "Veo 3", Description: "Google's flagship video model, with audio.", Durations: []int{4, 6, 8}},
		{ID: "fal-ai/kling-video/v2.1/standard/text-to-video", Name: "Kling v2.1", Description: "Strong motion, good value.", Durations: []int{5, 10}},
		{ID: "fal-ai/minimax/video-01", Name: "MiniMax Video-01", Description: "Cinematic character work.", Durations: []int{6}},
		{ID: "fal-ai/ltx-video", Name: "LTX Video", Description: "Fast open model for iteration.", Durations: []int{5}},
	},
	KindAudio: {
		{ID: "fal-ai/minimax-music", Name: "MiniMax Music", Description: "Song generation with vocals.", Durations: []int{30}},
		{ID: "fal-ai/stable-audio", Name: "Stable Audio", Description: "Instrumental music and sound design.", Durations: []int{10, 30, 47}},
		{ID: "fal-ai/elevenlabs/tts/multilingual-v2", Name: "ElevenLabs TTS", Description: "Text to speech, lifelike voices."},
	},
}

func (p *ProviderFal) Models(ctx context.Context, kind Kind) ([]Model, error) {
	entries := falCatalog[kind]
	out := make([]Model, 0, len(entries))
	for _, m := range entries {
		m.Provider = p.Name()
		m.Kind = kind
		if kind == KindImage {
			m.Sizes = imageSizes
		}
		// These APIs take one image input, and Generate only sends the first —
		// advertising more would silently drop the rest.
		if kind == KindImage || kind == KindVideo {
			m.MaxRefImages = 1
		}
		out = append(out, m)
	}
	return out, nil
}

func (p *ProviderFal) Generate(ctx context.Context, req Request) (*Asset, error) {
	key := p.key()
	if key == "" {
		return nil, fmt.Errorf("fal is not configured — add an API key in Settings")
	}
	if req.Model == "" {
		return nil, fmt.Errorf("a model is required")
	}

	input := map[string]interface{}{"prompt": req.Prompt}
	if req.Kind == KindImage {
		if ar := aspectRatio(req.Size); ar != "" {
			input["aspect_ratio"] = ar
		}
		if w, h := parseSize(req.Size); w > 0 && h > 0 {
			input["image_size"] = map[string]interface{}{"width": w, "height": h}
		}
	}
	if req.Duration > 0 {
		// fal is inconsistent here: video models take a string enum, audio
		// models take seconds as a number. Send both; unknown keys are ignored.
		input["duration"] = req.Duration
		input["seconds_total"] = req.Duration
	}
	if len(req.RefImages) > 0 {
		input["image_url"] = req.RefImages[0]
	}
	for k, v := range req.Params {
		input[k] = v
	}

	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	submitURL := fmt.Sprintf("%s/%s", falQueueAPI, strings.TrimPrefix(req.Model, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, submitURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Key "+key)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("fal request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fal returned %d: %s", resp.StatusCode, describeAPIError(raw))
	}

	var queued struct {
		StatusURL   string `json:"status_url"`
		ResponseURL string `json:"response_url"`
	}
	if err := json.Unmarshal(raw, &queued); err != nil {
		return nil, fmt.Errorf("could not parse fal response: %w", err)
	}
	if queued.ResponseURL == "" {
		return nil, fmt.Errorf("fal did not return a response URL")
	}

	result, err := p.await(ctx, key, queued.StatusURL, queued.ResponseURL)
	if err != nil {
		return nil, err
	}

	outURL, err := falOutputURL(result)
	if err != nil {
		return nil, fmt.Errorf("fal returned no usable output: %w", err)
	}

	return downloadAsset(ctx, p.http, outURL, req.Kind, req.Duration)
}

func (p *ProviderFal) await(ctx context.Context, key, statusURL, responseURL string) (map[string]interface{}, error) {
	const (
		pollInterval = 2 * time.Second
		maxWait      = 9 * time.Minute
	)
	deadline := time.Now().Add(maxWait)

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("fal request timed out after %s", maxWait)
		}

		status, err := p.getJSON(ctx, key, statusURL)
		if err != nil {
			return nil, err
		}

		switch s, _ := status["status"].(string); s {
		case "COMPLETED":
			return p.getJSON(ctx, key, responseURL)
		case "FAILED", "CANCELLED":
			return nil, fmt.Errorf("fal request %s: %s", strings.ToLower(s), errText(status["error"]))
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (p *ProviderFal) getJSON(ctx context.Context, key, url string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Key "+key)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fal poll failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fal poll returned %d: %s", resp.StatusCode, describeAPIError(raw))
	}

	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("could not parse fal response: %w", err)
	}
	return out, nil
}

// falOutputURL digs the file URL out of a fal result. Payload shapes vary by
// model family: {images:[{url}]}, {video:{url}}, {audio_file:{url}}, {url}.
func falOutputURL(result map[string]interface{}) (string, error) {
	for _, key := range []string{"images", "video", "audio", "audio_file", "audio_url", "image", "url", "output"} {
		v, ok := result[key]
		if !ok {
			continue
		}
		if u := extractURL(v); u != "" {
			return u, nil
		}
	}
	return "", fmt.Errorf("unrecognised output shape")
}

func extractURL(v interface{}) string {
	switch t := v.(type) {
	case string:
		if strings.HasPrefix(t, "http") || strings.HasPrefix(t, "data:") {
			return t
		}
	case map[string]interface{}:
		if u, ok := t["url"].(string); ok && u != "" {
			return u
		}
	case []interface{}:
		for _, item := range t {
			if u := extractURL(item); u != "" {
				return u
			}
		}
	}
	return ""
}
