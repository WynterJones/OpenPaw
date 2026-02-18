package llm

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// SSE chunk from OpenRouter/OpenAI streaming response.
type sseChunk struct {
	ID      string `json:"id"`
	Choices []struct {
		Delta struct {
			Role      string     `json:"role,omitempty"`
			Content   string     `json:"content,omitempty"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id,omitempty"`
				Type     string `json:"type,omitempty"`
				Function struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage,omitempty"`
}

// streamResult holds accumulated data from processing an SSE stream.
type streamResult struct {
	Content      string
	ToolCalls    []ToolCall
	InputTokens  int64
	OutputTokens int64
	FinishReason string
}

// processSSEStream reads SSE events from the response body and emits StreamEvents.
func processSSEStream(body io.ReadCloser, emit func(StreamEvent), model string) (*streamResult, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	result := &streamResult{}
	var contentBuf strings.Builder

	// Track tool calls being accumulated across chunks
	type toolAccum struct {
		ID       string
		Name     string
		ArgsBuf  strings.Builder
		Emitted  bool // whether we've emitted a tool_start for this
	}
	toolAccums := make(map[int]*toolAccum)

	emitted := false

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk sseChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Emit init event on first chunk
		if !emitted {
			emit(StreamEvent{Type: EventInit})
			emitted = true
		}

		if len(chunk.Choices) == 0 {
			// Check for usage in the final chunk
			if chunk.Usage != nil {
				result.InputTokens = chunk.Usage.PromptTokens
				result.OutputTokens = chunk.Usage.CompletionTokens
			}
			continue
		}

		choice := chunk.Choices[0]

		// Text content delta
		if choice.Delta.Content != "" {
			contentBuf.WriteString(choice.Delta.Content)
			emit(StreamEvent{Type: EventTextDelta, Text: choice.Delta.Content})
		}

		// Tool call deltas
		for _, tc := range choice.Delta.ToolCalls {
			accum, exists := toolAccums[tc.Index]
			if !exists {
				accum = &toolAccum{}
				toolAccums[tc.Index] = accum
			}

			if tc.ID != "" {
				accum.ID = tc.ID
			}
			if tc.Function.Name != "" {
				accum.Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				accum.ArgsBuf.WriteString(tc.Function.Arguments)
			}

			// Emit tool_start when we first see a name
			if accum.Name != "" && !accum.Emitted {
				emit(StreamEvent{
					Type:     EventToolStart,
					ToolName: accum.Name,
					ToolID:   accum.ID,
				})
				accum.Emitted = true
			}

			// Emit tool_delta for argument streaming
			if tc.Function.Arguments != "" && accum.Emitted {
				emit(StreamEvent{
					Type:     EventToolDelta,
					ToolName: accum.Name,
					Text:     tc.Function.Arguments,
				})
			}
		}

		// Finish reason
		if choice.FinishReason != nil {
			result.FinishReason = *choice.FinishReason
		}

		// Usage from final chunk
		if chunk.Usage != nil {
			result.InputTokens = chunk.Usage.PromptTokens
			result.OutputTokens = chunk.Usage.CompletionTokens
		}
	}

	// Build final tool calls (don't emit tool_end here — that happens
	// during actual execution in RunAgentLoop, which has the real output)
	for _, accum := range toolAccums {
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:   accum.ID,
			Type: "function",
			Function: FunctionCall{
				Name:      accum.Name,
				Arguments: accum.ArgsBuf.String(),
			},
		})
	}

	result.Content = contentBuf.String()

	if err := scanner.Err(); err != nil {
		return result, err
	}

	return result, nil
}
