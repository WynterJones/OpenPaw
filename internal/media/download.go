package media

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
)

// maxAssetBytes caps what a provider can hand back. Generous enough for a
// minute of 1080p video, small enough that a runaway response can't exhaust
// memory — assets are held in RAM before being written.
const maxAssetBytes = 256 << 20

// downloadAsset fetches a provider's output URL into memory and labels it with
// a MIME type and extension. Hosted providers return signed URLs rather than
// inline data, so this is the common tail of every non-OpenRouter generation.
func downloadAsset(ctx context.Context, client *http.Client, url string, kind Kind, durationSec int) (*Asset, error) {
	asset := &Asset{DurationMS: durationSec * 1000}

	if strings.HasPrefix(url, "data:") {
		data, mimeType, err := decodeDataURI(url)
		if err != nil {
			return nil, err
		}
		asset.Data = data
		asset.MimeType = mimeType
		asset.Ext = extensionFor(mimeType, kind)
		return asset, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download generated file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("downloading generated file returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read generated file: %w", err)
	}
	if len(data) > maxAssetBytes {
		return nil, fmt.Errorf("generated file exceeds the %d MB limit", maxAssetBytes>>20)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("generated file was empty")
	}

	mimeType := resp.Header.Get("Content-Type")
	if idx := strings.Index(mimeType, ";"); idx >= 0 {
		mimeType = mimeType[:idx]
	}
	mimeType = strings.TrimSpace(mimeType)

	// Signed URLs often serve a generic octet-stream, in which case the path
	// extension is the better signal.
	if mimeType == "" || mimeType == "application/octet-stream" || mimeType == "binary/octet-stream" {
		if ext := path.Ext(strippedPath(url)); ext != "" {
			if byExt := mime.TypeByExtension(ext); byExt != "" {
				mimeType = strings.Split(byExt, ";")[0]
			}
		}
	}
	if mimeType == "" {
		mimeType = defaultMime(kind)
	}

	asset.Data = data
	asset.MimeType = mimeType
	asset.Ext = extensionFor(mimeType, kind)
	return asset, nil
}

func decodeDataURI(uri string) ([]byte, string, error) {
	comma := strings.Index(uri, ",")
	if comma < 0 {
		return nil, "", fmt.Errorf("malformed data URI")
	}
	header := uri[5:comma]
	mimeType := header
	if idx := strings.Index(header, ";"); idx >= 0 {
		mimeType = header[:idx]
	}
	if !strings.Contains(header, "base64") {
		return []byte(uri[comma+1:]), mimeType, nil
	}
	data, err := base64.StdEncoding.DecodeString(uri[comma+1:])
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode data URI: %w", err)
	}
	return data, mimeType, nil
}

// strippedPath drops the query string so path.Ext doesn't pick up a signature
// parameter instead of the file extension.
func strippedPath(url string) string {
	if idx := strings.IndexAny(url, "?#"); idx >= 0 {
		return url[:idx]
	}
	return url
}

func defaultMime(kind Kind) string {
	switch kind {
	case KindVideo:
		return "video/mp4"
	case KindAudio:
		return "audio/mpeg"
	default:
		return "image/png"
	}
}

// extensionFor maps a MIME type to a file extension, preferring the formats
// the browser's <img>/<video>/<audio> elements play natively.
func extensionFor(mimeType string, kind Kind) string {
	switch strings.ToLower(mimeType) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "audio/mp4", "audio/x-m4a":
		return ".m4a"
	}
	if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	switch kind {
	case KindVideo:
		return ".mp4"
	case KindAudio:
		return ".mp3"
	default:
		return ".png"
	}
}
