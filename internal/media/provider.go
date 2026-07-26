// Package media generates images, video and audio through pluggable providers.
//
// OpenPaw's chat models come from OpenRouter, but OpenRouter only emits text
// and images — it has no text-to-video or text-to-music models at all. So
// Studio talks to a small provider interface instead of to OpenRouter
// directly: OpenRouter covers images, Replicate and fal cover video and audio,
// and each one is optional. A provider with no API key configured simply
// reports itself unavailable and drops out of the picker.
package media

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Kind is the sort of asset being produced. It maps 1:1 onto the media table's
// media_type column.
type Kind string

const (
	KindImage Kind = "image"
	KindVideo Kind = "video"
	KindAudio Kind = "audio"
)

func ParseKind(s string) (Kind, error) {
	switch Kind(strings.ToLower(strings.TrimSpace(s))) {
	case KindImage:
		return KindImage, nil
	case KindVideo:
		return KindVideo, nil
	case KindAudio:
		return KindAudio, nil
	}
	return "", fmt.Errorf("unsupported media type %q (want image, video or audio)", s)
}

// Model is one selectable entry in Studio's model picker.
type Model struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Provider    string   `json:"provider"`
	Kind        Kind     `json:"kind"`
	Description string   `json:"description,omitempty"`
	// Sizes/Durations drive the option lists next to the picker. Empty means
	// the model doesn't take that parameter and the control stays hidden.
	Sizes     []string `json:"sizes,omitempty"`
	Durations []int    `json:"durations,omitempty"`
	// MaxRefImages is how many reference images this model actually uses. Zero
	// hides the reference picker entirely. It is deliberately honest rather
	// than optimistic: Replicate and fal image inputs take a single image, so
	// offering three slots there would silently discard two.
	MaxRefImages int `json:"max_ref_images"`
}

// Request is one generation job. Count is handled by the caller, not the
// provider: some APIs take an n parameter and some don't, and running the
// single-asset path N times is uniform and keeps partial results on failure.
type Request struct {
	Kind      Kind
	Prompt    string
	Model     string
	Size      string
	Duration  int
	RefImages []string
	// Params carries provider-specific extras straight through from the UI
	// (aspect_ratio, negative_prompt, seed…) without this package needing to
	// know every model's schema.
	Params map[string]interface{}
}

// Asset is one produced file, already in memory. Providers return bytes rather
// than URLs so the store layer can persist everything the same way regardless
// of whether the provider handed back base64 or a signed download link.
type Asset struct {
	Data          []byte
	MimeType      string
	Ext           string
	Width         int
	Height        int
	DurationMS    int
	RevisedPrompt string
}

// Provider is one generation backend.
type Provider interface {
	Name() string
	// Configured reports whether this provider has credentials. Unconfigured
	// providers are still listed by the API so the UI can show them greyed out
	// with a "needs an API key" hint rather than hiding them silently.
	Configured() bool
	// Kinds lists what this provider can produce.
	Kinds() []Kind
	Models(ctx context.Context, kind Kind) ([]Model, error)
	Generate(ctx context.Context, req Request) (*Asset, error)
}

// Registry holds every known provider. Keys are updated in place when the user
// saves them in Settings, so a newly-added key takes effect without a restart.
type Registry struct {
	mu    sync.RWMutex
	order []string
	byName map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Provider)}
}

func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byName[p.Name()]; !exists {
		r.order = append(r.order, p.Name())
	}
	r.byName[p.Name()] = p
}

func (r *Registry) Get(name string) Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byName[name]
}

// All returns providers in registration order, which is also the order the
// picker shows them in.
func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.byName[n])
	}
	return out
}

// Supports reports whether a configured provider exists for this kind — what
// the UI uses to decide if the Video or Audio tab is usable.
func (r *Registry) Supports(kind Kind) bool {
	for _, p := range r.All() {
		if !p.Configured() {
			continue
		}
		for _, k := range p.Kinds() {
			if k == kind {
				return true
			}
		}
	}
	return false
}

// Resolve picks the provider for a request. An explicit name wins; otherwise
// the first configured provider that handles the kind is used, so a user who
// has only set one key never has to think about providers at all.
func (r *Registry) Resolve(name string, kind Kind) (Provider, error) {
	if name != "" {
		p := r.Get(name)
		if p == nil {
			return nil, fmt.Errorf("unknown provider %q", name)
		}
		if !p.Configured() {
			return nil, fmt.Errorf("%s is not configured — add its API key in Settings", p.Name())
		}
		if !supportsKind(p, kind) {
			return nil, fmt.Errorf("%s cannot generate %s", p.Name(), kind)
		}
		return p, nil
	}

	for _, p := range r.All() {
		if p.Configured() && supportsKind(p, kind) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no configured provider can generate %s — add a Replicate or fal API key in Settings", kind)
}

func supportsKind(p Provider, kind Kind) bool {
	for _, k := range p.Kinds() {
		if k == kind {
			return true
		}
	}
	return false
}

// ModelsFor gathers the model lists of every configured provider that handles
// the kind. A provider whose catalog lookup fails is skipped rather than
// failing the whole call — one dead API shouldn't empty the picker.
//
// The result is NOT re-sorted. Iterating in registration order already groups
// by provider, and each provider ranks its own models so the best default
// comes first — re-sorting here by name would throw that away.
func (r *Registry) ModelsFor(ctx context.Context, kind Kind) []Model {
	var out []Model
	for _, p := range r.All() {
		if !p.Configured() || !supportsKind(p, kind) {
			continue
		}
		models, err := p.Models(ctx, kind)
		if err != nil {
			continue
		}
		out = append(out, models...)
	}
	return out
}
