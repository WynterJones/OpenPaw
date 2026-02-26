package fal

// GenerateOpts holds optional parameters for image generation.
type GenerateOpts struct {
	NumInferenceSteps int    `json:"num_inference_steps,omitempty"`
	GuidanceScale     float64 `json:"guidance_scale,omitempty"`
	Seed              int    `json:"seed,omitempty"`
	NumImages         int    `json:"num_images,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
}

// GenerateRequest is the request body sent to the FAL API.
type GenerateRequest struct {
	Prompt            string  `json:"prompt"`
	ImageSize         string  `json:"image_size,omitempty"`
	NumInferenceSteps int     `json:"num_inference_steps,omitempty"`
	GuidanceScale     float64 `json:"guidance_scale,omitempty"`
	Seed              int     `json:"seed,omitempty"`
	NumImages         int     `json:"num_images,omitempty"`
	OutputFormat      string  `json:"output_format,omitempty"`
}

// GenerateResponse is the response from the FAL API.
type GenerateResponse struct {
	Images []ImageData `json:"images"`
	Seed   int         `json:"seed"`
	Prompt string      `json:"prompt"`
}

// ImageData represents a single generated image.
type ImageData struct {
	URL         string `json:"url"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	ContentType string `json:"content_type"`
}

// Result is the processed result returned to callers.
type Result struct {
	Images []ImageData `json:"images"`
	Seed   int         `json:"seed"`
	Prompt string      `json:"prompt"`
}

// SupportedModels maps friendly model names to FAL API endpoints.
var SupportedModels = map[string]string{
	"flux-dev":     "fal-ai/flux/dev",
	"flux-schnell": "fal-ai/flux/schnell",
	"flux-pro":     "fal-ai/flux-pro/v1.1-ultra",
}
