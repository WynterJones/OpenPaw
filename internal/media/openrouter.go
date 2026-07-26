package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"sort"
	"strings"

	// Registered for their DecodeConfig side effect only: reading the real
	// dimensions and format back off the returned bytes.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	llm "github.com/openpaw/openpaw/internal/llm"
)

// ProviderOpenRouter generates images through the OpenRouter chat completions
// API, which returns them inline on models with an image output modality.
// It deliberately advertises images only — OpenRouter has no video or audio
// generation models.
type ProviderOpenRouter struct {
	client *llm.Client
}

func NewOpenRouterProvider(client *llm.Client) *ProviderOpenRouter {
	return &ProviderOpenRouter{client: client}
}

func (p *ProviderOpenRouter) Name() string { return "openrouter" }

func (p *ProviderOpenRouter) Configured() bool {
	return p.client != nil && p.client.IsConfigured()
}

func (p *ProviderOpenRouter) Kinds() []Kind { return []Kind{KindImage} }

// imageSizes are the aspect ratios the image models reliably honour. The
// models infer dimensions from the prompt rather than taking a size parameter,
// so this is passed as a hint, not a hard constraint.
var imageSizes = []string{"1024x1024", "1536x1024", "1024x1536"}

func (p *ProviderOpenRouter) Models(ctx context.Context, kind Kind) ([]Model, error) {
	if kind != KindImage {
		return nil, nil
	}
	if !p.Configured() {
		return nil, fmt.Errorf("openrouter is not configured")
	}

	infos, err := p.client.GetImageModels(ctx)
	if err != nil {
		// The live catalog is the source of truth for "latest models", but a
		// network blip shouldn't leave the picker empty — fall back to the
		// known-good list the agent image tool already uses.
		return fallbackImageModels(), nil
	}

	// Known-good ids first, then true image models, then chat models that can
	// also emit an image. Whatever lands first becomes the picker's default, so
	// the ordering has to put a reliable generator at the top.
	rank := map[string]int{}
	for i, id := range llm.ImageGenModels {
		rank[id] = i
	}

	type entry struct {
		model      Model
		known      bool
		knownRank  int
		imageFirst bool
	}
	entries := make([]entry, 0, len(infos))
	for _, m := range infos {
		r, known := rank[m.ID]
		entries = append(entries, entry{
			model: Model{
				ID:          m.ID,
				Name:        cleanModelName(m.Name, m.ID),
				Provider:    p.Name(),
				Kind:        KindImage,
				Description: truncate(m.Description, 180),
				Sizes:       imageSizes,
				// Chat-shaped image models take references as extra content
				// parts, so several can be sent in one request.
				MaxRefImages: refImageSlots(m.AcceptsImageInput()),
			},
			known:      known,
			knownRank:  r,
			imageFirst: m.IsImageFirst(),
		})
	}
	if len(entries) == 0 {
		return fallbackImageModels(), nil
	}

	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.known != b.known {
			return a.known
		}
		if a.known && b.known {
			return a.knownRank < b.knownRank
		}
		if a.imageFirst != b.imageFirst {
			return a.imageFirst
		}
		return a.model.Name < b.model.Name
	})

	out := make([]Model, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.model)
	}
	return out, nil
}

// maxOpenRouterRefImages caps how many reference images the editor offers.
// Three is enough for style + subject + composition without bloating a request.
const maxOpenRouterRefImages = 3

func refImageSlots(accepts bool) int {
	if accepts {
		return maxOpenRouterRefImages
	}
	return 0
}

func fallbackImageModels() []Model {
	out := make([]Model, 0, len(llm.ImageGenModels))
	for _, id := range llm.ImageGenModels {
		out = append(out, Model{
			ID:       id,
			Name:     cleanModelName("", id),
			Provider: "openrouter",
			Kind:     KindImage,
			Sizes:    imageSizes,
		})
	}
	return out
}

func (p *ProviderOpenRouter) Generate(ctx context.Context, req Request) (*Asset, error) {
	if req.Kind != KindImage {
		return nil, fmt.Errorf("openrouter cannot generate %s — it only produces images. Use Replicate or fal", req.Kind)
	}
	if !p.Configured() {
		return nil, fmt.Errorf("openrouter is not configured — add an API key in Settings")
	}

	size := req.Size
	if size == "" {
		size = imageSizes[0]
	}

	result, err := p.client.GenerateImage(ctx, req.Model, req.Prompt, size, req.RefImages)
	if err != nil {
		return nil, err
	}
	if result.Base64 == "" {
		return nil, fmt.Errorf("model returned no image data")
	}

	data, err := base64.StdEncoding.DecodeString(result.Base64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image data: %w", err)
	}

	// Models treat the size as a hint and routinely return a different format
	// and aspect ratio than asked for, so both are read back off the bytes.
	// Getting this wrong means a JPEG saved as .png and a wrong size on record.
	mimeType := result.MimeType
	if mimeType == "" {
		mimeType = "image/png"
	}
	w, h := parseSize(size)
	if cfg, format, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		w, h = cfg.Width, cfg.Height
		if detected := mimeForFormat(format); detected != "" {
			mimeType = detected
		}
	}

	return &Asset{
		Data:          data,
		MimeType:      mimeType,
		Ext:           extensionFor(mimeType, KindImage),
		Width:         w,
		Height:        h,
		RevisedPrompt: result.RevisedPrompt,
	}, nil
}

// mimeForFormat maps image.DecodeConfig's format name to a MIME type. Only the
// formats whose decoders are registered above can appear here — WebP is not
// among them (it would mean a new dependency for dimensions alone), so a WebP
// result keeps the MIME type the data URI declared and its size hint.
func mimeForFormat(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	}
	return ""
}

func parseSize(size string) (int, int) {
	var w, h int
	if n, _ := fmt.Sscanf(size, "%dx%d", &w, &h); n == 2 && w > 0 && h > 0 {
		return w, h
	}
	return 0, 0
}

// cleanModelName turns "Google: Gemini 3 Pro Image Preview" into something
// short enough for a narrow picker, falling back to the ID's last segment.
func cleanModelName(name, id string) string {
	if name == "" {
		if idx := strings.LastIndex(id, "/"); idx >= 0 {
			return id[idx+1:]
		}
		return id
	}
	if idx := strings.Index(name, ": "); idx >= 0 {
		return name[idx+2:]
	}
	return name
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
