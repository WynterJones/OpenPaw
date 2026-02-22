package llm

import (
	"encoding/json"
	"strings"
	"sync"
)

type WidgetPayload struct {
	Type     string          `json:"type"`
	Title    string          `json:"title,omitempty"`
	ToolID   string          `json:"tool_id,omitempty"`
	ToolName string          `json:"tool_name,omitempty"`
	Endpoint string          `json:"endpoint,omitempty"`
	Data     json.RawMessage `json:"data"`
}

type WidgetCollector struct {
	mu        sync.Mutex
	widgets   []WidgetPayload
	endpoints map[string]string // tool_id -> endpoint path
}

func NewWidgetCollector() *WidgetCollector {
	return &WidgetCollector{
		endpoints: make(map[string]string),
	}
}

func (wc *WidgetCollector) TrackToolStart(toolName, toolID string, toolInput map[string]interface{}) {
	if toolName != "call_tool" || toolID == "" {
		return
	}
	if ep, ok := toolInput["endpoint"].(string); ok && ep != "" {
		wc.mu.Lock()
		wc.endpoints[toolID] = ep
		wc.mu.Unlock()
	}
}

func (wc *WidgetCollector) Collect(toolName, toolID, output string) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return false
	}

	// Extract __tool_uuid if present (injected by makeCallToolHandler)
	realToolID := toolID
	if uuidRaw, ok := raw["__tool_uuid"]; ok {
		var uuid string
		if json.Unmarshal(uuidRaw, &uuid) == nil && uuid != "" {
			realToolID = uuid
		}
		delete(raw, "__tool_uuid")
	}

	var widgetType, widgetTitle string

	if widgetMeta, ok := raw["__widget"]; ok {
		// Explicit __widget metadata provided
		var meta struct {
			Type  string `json:"type"`
			Title string `json:"title"`
		}
		if err := json.Unmarshal(widgetMeta, &meta); err != nil {
			return false
		}
		widgetType = meta.Type
		widgetTitle = meta.Title
		delete(raw, "__widget")
	} else if toolName == "call_tool" {
		// Auto-detect widget type from data shape
		widgetType = DetectWidgetType(raw)
	} else {
		return false
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return false
	}

	wc.mu.Lock()
	defer wc.mu.Unlock()

	endpoint := wc.endpoints[toolID]

	wc.widgets = append(wc.widgets, WidgetPayload{
		Type:     widgetType,
		Title:    widgetTitle,
		ToolID:   realToolID,
		ToolName: toolName,
		Endpoint: endpoint,
		Data:     data,
	})

	return true
}

// DetectWidgetType inspects parsed JSON keys to pick the best built-in widget.
func DetectWidgetType(raw map[string]json.RawMessage) string {
	has := func(key string) bool { _, ok := raw[key]; return ok }

	// data-table: has columns + rows arrays
	if has("columns") && has("rows") {
		return "data-table"
	}
	// status-card: has label + status
	if has("label") && has("status") {
		return "status-card"
	}
	// metric-card: has label + value
	if has("label") && has("value") {
		return "metric-card"
	}

	// Media detection: audio, video, or file based on content_type or file extension
	ct := extractString(raw, "content_type")
	filePath := findFilePathValue(raw)
	ext := fileExtension(filePath)

	if strings.HasPrefix(ct, "audio/") || isAudioExt(ext) {
		return "audio-player"
	}
	if strings.HasPrefix(ct, "video/") || isVideoExt(ext) {
		return "video-player"
	}
	if ext == ".pdf" || isFileExt(ext) {
		return "file-preview"
	}

	// key-value: flat object with all scalar values
	allScalar := true
	for _, v := range raw {
		trimmed := trimBytes(v)
		if len(trimmed) == 0 {
			allScalar = false
			break
		}
		first := trimmed[0]
		if first == '{' || first == '[' {
			allScalar = false
			break
		}
	}
	if len(raw) > 0 && allScalar {
		return "key-value"
	}

	return "json-viewer"
}

func extractString(raw map[string]json.RawMessage, key string) string {
	v, ok := raw[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return ""
}

func findFilePathValue(raw map[string]json.RawMessage) string {
	for _, key := range []string{"audio_file", "file_path", "path", "url", "file", "filename", "video_file"} {
		if s := extractString(raw, key); s != "" {
			return s
		}
	}
	return ""
}

func fileExtension(path string) string {
	dot := strings.LastIndex(path, ".")
	if dot == -1 {
		return ""
	}
	ext := strings.ToLower(path[dot:])
	if i := strings.IndexAny(ext, "?#"); i != -1 {
		ext = ext[:i]
	}
	return ext
}

func isAudioExt(ext string) bool {
	switch ext {
	case ".mp3", ".wav", ".ogg", ".opus", ".aac", ".flac", ".m4a", ".wma":
		return true
	}
	return false
}

func isVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".webm", ".mov", ".avi", ".mkv", ".m4v":
		return true
	}
	return false
}

func isFileExt(ext string) bool {
	switch ext {
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".csv", ".txt", ".json", ".xml", ".html", ".zip", ".tar", ".gz":
		return true
	}
	return false
}

func trimBytes(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\n' || b[0] == '\r') {
		b = b[1:]
	}
	return b
}

func (wc *WidgetCollector) JSON() string {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	if len(wc.widgets) == 0 {
		return ""
	}

	b, err := json.Marshal(wc.widgets)
	if err != nil {
		return ""
	}
	return string(b)
}
