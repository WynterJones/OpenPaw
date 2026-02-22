package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

const maxUploadSize = 32 << 20 // 32MB

func registerRoutes(r chi.Router) {
	r.Post("/resize", handleResize)
	r.Post("/info", handleInfo)
}

// nearestNeighborResize performs a nearest-neighbor resize of the source image
// to the target dimensions. This is a stdlib-only implementation.
func nearestNeighborResize(src image.Image, targetWidth, targetHeight int) *image.RGBA {
	srcBounds := src.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	for y := 0; y < targetHeight; y++ {
		for x := 0; x < targetWidth; x++ {
			srcX := srcBounds.Min.X + x*srcWidth/targetWidth
			srcY := srcBounds.Min.Y + y*srcHeight/targetHeight
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	return dst
}

// calculateDimensions computes target width and height based on fit mode.
func calculateDimensions(srcWidth, srcHeight, reqWidth, reqHeight int, fit string) (int, int) {
	switch fit {
	case "fill":
		// Stretch to exact dimensions
		return reqWidth, reqHeight

	case "cover":
		// Scale to cover the target area (may exceed bounds on one axis)
		srcAspect := float64(srcWidth) / float64(srcHeight)
		reqAspect := float64(reqWidth) / float64(reqHeight)

		if srcAspect > reqAspect {
			// Source is wider relative to target; scale by height
			return int(float64(reqHeight) * srcAspect), reqHeight
		}
		// Source is taller relative to target; scale by width
		return reqWidth, int(float64(reqWidth) / srcAspect)

	default: // "contain"
		// Scale to fit within the target area, maintaining aspect ratio
		srcAspect := float64(srcWidth) / float64(srcHeight)
		reqAspect := float64(reqWidth) / float64(reqHeight)

		if srcAspect > reqAspect {
			// Source is wider relative to target; constrain by width
			return reqWidth, int(float64(reqWidth) / srcAspect)
		}
		// Source is taller relative to target; constrain by height
		return int(float64(reqHeight) * srcAspect), reqHeight
	}
}

func handleResize(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse multipart form: %v", err))
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "image file is required")
		return
	}
	defer file.Close()

	// Parse width
	widthStr := r.FormValue("width")
	if widthStr == "" {
		writeError(w, http.StatusBadRequest, "width parameter is required")
		return
	}
	reqWidth, err := strconv.Atoi(widthStr)
	if err != nil || reqWidth <= 0 {
		writeError(w, http.StatusBadRequest, "width must be a positive integer")
		return
	}

	// Parse height
	heightStr := r.FormValue("height")
	if heightStr == "" {
		writeError(w, http.StatusBadRequest, "height parameter is required")
		return
	}
	reqHeight, err := strconv.Atoi(heightStr)
	if err != nil || reqHeight <= 0 {
		writeError(w, http.StatusBadRequest, "height must be a positive integer")
		return
	}

	// Parse fit mode
	fit := r.FormValue("fit")
	if fit == "" {
		fit = "contain"
	}
	if fit != "contain" && fit != "cover" && fit != "fill" {
		writeError(w, http.StatusBadRequest, "fit must be one of: contain, cover, fill")
		return
	}

	// Parse output format
	outputFormat := strings.ToLower(r.FormValue("format"))

	// Parse quality
	quality := 85
	if q := r.FormValue("quality"); q != "" {
		parsed, err := strconv.Atoi(q)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "quality must be an integer between 1 and 100")
			return
		}
		quality = parsed
	}

	// Decode image
	src, inputFormat, err := image.Decode(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to decode image: %v", err))
		return
	}

	// Determine output format
	if outputFormat == "" {
		outputFormat = inputFormat
	}
	// Normalize jpeg
	if outputFormat == "jpg" {
		outputFormat = "jpeg"
	}
	if outputFormat != "png" && outputFormat != "jpeg" {
		outputFormat = "png" // default fallback
	}

	srcBounds := src.Bounds()
	origWidth := srcBounds.Dx()
	origHeight := srcBounds.Dy()

	// Calculate target dimensions based on fit mode
	targetWidth, targetHeight := calculateDimensions(origWidth, origHeight, reqWidth, reqHeight, fit)

	// Ensure minimum dimensions
	if targetWidth < 1 {
		targetWidth = 1
	}
	if targetHeight < 1 {
		targetHeight = 1
	}

	// Resize using nearest-neighbor
	resized := nearestNeighborResize(src, targetWidth, targetHeight)

	// For "cover" mode, crop to the requested dimensions
	var finalImage image.Image = resized
	if fit == "cover" && (targetWidth > reqWidth || targetHeight > reqHeight) {
		// Center crop
		cropX := (targetWidth - reqWidth) / 2
		cropY := (targetHeight - reqHeight) / 2
		cropped := image.NewRGBA(image.Rect(0, 0, reqWidth, reqHeight))
		draw.Draw(cropped, cropped.Bounds(), resized, image.Pt(cropX, cropY), draw.Src)
		finalImage = cropped
		targetWidth = reqWidth
		targetHeight = reqHeight
	}

	// Encode to buffer
	var buf bytes.Buffer
	switch outputFormat {
	case "jpeg":
		err = jpeg.Encode(&buf, finalImage, &jpeg.Options{Quality: quality})
	default:
		err = png.Encode(&buf, finalImage)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to encode image: %v", err))
		return
	}

	// Base64 encode
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"original_width":  origWidth,
		"original_height": origHeight,
		"width":           targetWidth,
		"height":          targetHeight,
		"format":          outputFormat,
		"size_bytes":      buf.Len(),
		"image_base64":    encoded,
	})
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse multipart form: %v", err))
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "image file is required")
		return
	}
	defer file.Close()

	config, format, err := image.DecodeConfig(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to decode image: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"width":  config.Width,
		"height": config.Height,
		"format": format,
	})
}
