package llm

import (
	"encoding/json"
	"sync"
)

type WidgetPayload struct {
	Type     string          `json:"type"`
	Title    string          `json:"title,omitempty"`
	ToolID   string          `json:"tool_id,omitempty"`
	ToolName string          `json:"tool_name,omitempty"`
	Data     json.RawMessage `json:"data"`
}

type WidgetCollector struct {
	mu      sync.Mutex
	widgets []WidgetPayload
}

func NewWidgetCollector() *WidgetCollector {
	return &WidgetCollector{}
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

	wc.widgets = append(wc.widgets, WidgetPayload{
		Type:     widgetType,
		Title:    widgetTitle,
		ToolID:   realToolID,
		ToolName: toolName,
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
