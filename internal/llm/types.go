package llm

type ClaudeUsage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	CacheRead    int `json:"cache_read_input_tokens,omitempty"`
	CacheCreate  int `json:"cache_creation_input_tokens,omitempty"`
}

type StreamEvent struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text,omitempty"`
	ToolName     string                 `json:"tool_name,omitempty"`
	ToolID       string                 `json:"tool_id,omitempty"`
	ToolInput    map[string]interface{} `json:"tool_input,omitempty"`
	ToolOutput   string                 `json:"tool_output,omitempty"`
	TotalCostUSD float64                `json:"total_cost_usd,omitempty"`
	Usage        *ClaudeUsage           `json:"usage,omitempty"`
	Result       string                 `json:"result,omitempty"`
	Error        string                 `json:"error,omitempty"`
	SessionID    string                 `json:"session_id,omitempty"`
	NumTurns     int                    `json:"num_turns,omitempty"`
}

const (
	EventTextDelta    = "text_delta"
	EventToolStart    = "tool_start"
	EventToolDelta    = "tool_delta"
	EventToolEnd      = "tool_end"
	EventTurnComplete = "turn_complete"
	EventResult       = "result"
	EventError        = "error"
	EventInit         = "init"
)
