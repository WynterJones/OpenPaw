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

const replicateAPI = "https://api.replicate.com/v1"

// ProviderReplicate runs Replicate's hosted models. Replicate is the widest
// catalog of the three — it covers all of images, video and music, which is
// why it exists here alongside OpenRouter.
type ProviderReplicate struct {
	keyFn func() string
	http  *http.Client
}

func NewReplicateProvider(keyFn func() string) *ProviderReplicate {
	return &ProviderReplicate{
		keyFn: keyFn,
		// Video jobs routinely run for minutes; the per-request timeout has to
		// clear the whole poll loop, which is bounded separately below.
		http: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (p *ProviderReplicate) Name() string { return "replicate" }

func (p *ProviderReplicate) Configured() bool { return p.key() != "" }

func (p *ProviderReplicate) key() string {
	if p.keyFn == nil {
		return ""
	}
	return strings.TrimSpace(p.keyFn())
}

func (p *ProviderReplicate) Kinds() []Kind { return []Kind{KindImage, KindVideo, KindAudio} }

// replicateCatalog is a curated starting set. Replicate has no stable
// "list the good models for this task" endpoint, and its collections API
// shifts, so the picker ships with known-working slugs and Studio also accepts
// any custom owner/name a user types.
var replicateCatalog = map[Kind][]Model{
	KindImage: {
		{ID: "black-forest-labs/flux-1.1-pro", Name: "FLUX 1.1 Pro", Description: "High quality, strong prompt adherence."},
		{ID: "black-forest-labs/flux-schnell", Name: "FLUX Schnell", Description: "Fast and inexpensive, good for drafts."},
		{ID: "black-forest-labs/flux-dev", Name: "FLUX Dev", Description: "Balanced quality and speed."},
		{ID: "google/imagen-4", Name: "Imagen 4", Description: "Google's image model. Excellent text rendering."},
		{ID: "ideogram-ai/ideogram-v3-turbo", Name: "Ideogram v3 Turbo", Description: "Best-in-class typography and logos."},
		{ID: "recraft-ai/recraft-v3", Name: "Recraft v3", Description: "Vector-leaning illustration and design work."},
		{ID: "stability-ai/stable-diffusion-3.5-large", Name: "Stable Diffusion 3.5 Large", Description: "Open weights, highly steerable."},
	},
	KindVideo: {
		{ID: "google/veo-3", Name: "Veo 3", Description: "Google's flagship video model, with audio.", Durations: []int{4, 6, 8}},
		{ID: "google/veo-3-fast", Name: "Veo 3 Fast", Description: "Cheaper, quicker Veo 3.", Durations: []int{4, 6, 8}},
		{ID: "kwaivgi/kling-v2.1", Name: "Kling v2.1", Description: "Strong motion and camera control.", Durations: []int{5, 10}},
		{ID: "minimax/video-01", Name: "MiniMax Video-01", Description: "Cinematic look, good character consistency.", Durations: []int{6}},
		{ID: "lightricks/ltx-video", Name: "LTX Video", Description: "Fast open model, good for iteration.", Durations: []int{5}},
	},
	KindAudio: {
		{ID: "meta/musicgen", Name: "MusicGen", Description: "Meta's text-to-music model.", Durations: []int{8, 15, 30}},
		{ID: "minimax/music-01", Name: "MiniMax Music-01", Description: "Song generation with vocals.", Durations: []int{30}},
		{ID: "riffusion/riffusion", Name: "Riffusion", Description: "Spectrogram-based music generation.", Durations: []int{8}},
		{ID: "minimax/speech-02-turbo", Name: "Speech-02 Turbo", Description: "Text to speech, many voices."},
	},
}

func (p *ProviderReplicate) Models(ctx context.Context, kind Kind) ([]Model, error) {
	entries := replicateCatalog[kind]
	out := make([]Model, 0, len(entries))
	for _, m := range entries {
		m.Provider = p.Name()
		m.Kind = kind
		if kind == KindImage {
			m.Sizes = imageSizes
		}
		out = append(out, m)
	}
	return out, nil
}

func (p *ProviderReplicate) Generate(ctx context.Context, req Request) (*Asset, error) {
	key := p.key()
	if key == "" {
		return nil, fmt.Errorf("replicate is not configured — add an API token in Settings")
	}
	if req.Model == "" {
		return nil, fmt.Errorf("a model is required")
	}
	if !strings.Contains(req.Model, "/") {
		return nil, fmt.Errorf("replicate models look like owner/name (got %q)", req.Model)
	}

	input := map[string]interface{}{"prompt": req.Prompt}
	if req.Kind == KindImage {
		if ar := aspectRatio(req.Size); ar != "" {
			input["aspect_ratio"] = ar
		}
		input["output_format"] = "png"
	}
	if req.Duration > 0 {
		input["duration"] = req.Duration
	}
	if len(req.RefImages) > 0 {
		// Replicate accepts a data URI or a public URL in image inputs.
		input["image"] = req.RefImages[0]
	}
	for k, v := range req.Params {
		input[k] = v
	}

	body, err := json.Marshal(map[string]interface{}{"input": input})
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/models/%s/predictions", replicateAPI, req.Model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	httpReq.Header.Set("Content-Type", "application/json")
	// Ask Replicate to hold the connection open until the prediction finishes,
	// which resolves most image jobs in a single round trip. Longer jobs come
	// back still running and fall through to the poll loop.
	httpReq.Header.Set("Prefer", "wait=60")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("replicate request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("replicate returned %d: %s", resp.StatusCode, describeAPIError(raw))
	}

	var pred replicatePrediction
	if err := json.Unmarshal(raw, &pred); err != nil {
		return nil, fmt.Errorf("could not parse replicate response: %w", err)
	}

	pred, err = p.await(ctx, key, pred)
	if err != nil {
		return nil, err
	}

	outURL, err := firstURL(pred.Output)
	if err != nil {
		return nil, fmt.Errorf("replicate returned no usable output: %w", err)
	}

	return downloadAsset(ctx, p.http, outURL, req.Kind, req.Duration)
}

type replicatePrediction struct {
	ID     string          `json:"id"`
	Status string          `json:"status"`
	Output json.RawMessage `json:"output"`
	Error  interface{}     `json:"error"`
	URLs   struct {
		Get string `json:"get"`
	} `json:"urls"`
}

// await polls until the prediction reaches a terminal state.
func (p *ProviderReplicate) await(ctx context.Context, key string, pred replicatePrediction) (replicatePrediction, error) {
	const (
		pollInterval = 2 * time.Second
		maxWait      = 9 * time.Minute
	)
	deadline := time.Now().Add(maxWait)

	for {
		switch pred.Status {
		case "succeeded":
			return pred, nil
		case "failed", "canceled":
			return pred, fmt.Errorf("replicate prediction %s: %s", pred.Status, errText(pred.Error))
		}

		if pred.URLs.Get == "" {
			return pred, fmt.Errorf("replicate prediction is %s but gave no status URL", pred.Status)
		}
		if time.Now().After(deadline) {
			return pred, fmt.Errorf("replicate prediction timed out after %s (still %s)", maxWait, pred.Status)
		}

		select {
		case <-ctx.Done():
			return pred, ctx.Err()
		case <-time.After(pollInterval):
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pred.URLs.Get, nil)
		if err != nil {
			return pred, err
		}
		req.Header.Set("Authorization", "Bearer "+key)

		resp, err := p.http.Do(req)
		if err != nil {
			return pred, fmt.Errorf("replicate poll failed: %w", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			return pred, fmt.Errorf("replicate poll returned %d: %s", resp.StatusCode, describeAPIError(raw))
		}
		if err := json.Unmarshal(raw, &pred); err != nil {
			return pred, fmt.Errorf("could not parse replicate poll response: %w", err)
		}
	}
}

// firstURL pulls a downloadable URL out of an output field that may be a bare
// string, an array of strings, or an object with a url/audio/video key.
func firstURL(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("output was empty")
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return s, nil
	}

	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err == nil {
		for _, item := range list {
			if u, err := firstURL(item); err == nil {
				return u, nil
			}
		}
		return "", fmt.Errorf("output array had no URL")
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, k := range []string{"url", "video", "audio", "image", "output"} {
			if v, ok := obj[k].(string); ok && v != "" {
				return v, nil
			}
		}
	}
	return "", fmt.Errorf("unrecognised output shape")
}

func errText(v interface{}) string {
	if v == nil {
		return "no detail given"
	}
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// aspectRatio converts a WxH size hint into the ratio string most hosted
// models expect.
func aspectRatio(size string) string {
	w, h := parseSize(size)
	if w == 0 || h == 0 {
		return ""
	}
	switch {
	case w == h:
		return "1:1"
	case w*2 == h*3:
		return "3:2"
	case h*2 == w*3:
		return "2:3"
	case w*9 == h*16:
		return "16:9"
	case h*9 == w*16:
		return "9:16"
	case w*3 == h*4:
		return "4:3"
	case h*3 == w*4:
		return "3:4"
	}
	if w > h {
		return "16:9"
	}
	if h > w {
		return "9:16"
	}
	return "1:1"
}

// describeAPIError keeps provider error bodies readable in the UI without
// dumping an entire HTML error page into a toast.
func describeAPIError(raw []byte) string {
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, k := range []string{"detail", "error", "message", "title"} {
			if v, ok := obj[k].(string); ok && v != "" {
				return v
			}
		}
	}
	return truncate(string(raw), 300)
}
