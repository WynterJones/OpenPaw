package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type HistoryMessage struct {
	Role    string
	Content string
}

type AgentConfig struct {
	Model         string
	System        string
	MaxTokens     int64
	MaxTurns      int
	Tools         []string
	WorkDir       string
	SandboxPaths  []string
	OnEvent       func(StreamEvent)
	History       []HistoryMessage
	ExtraTools    []ToolDef
	ExtraHandlers map[string]ToolHandler
}

type AgentResult struct {
	Text         string
	InputTokens  int64
	OutputTokens int64
	TotalCostUSD float64
	NumTurns     int
	StopReason   string
}

type UsageInfo struct {
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
}

func (c *Client) RunAgentLoop(ctx context.Context, cfg AgentConfig, userMessage string) (*AgentResult, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("API client not configured")
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = MaxTokensForModel(cfg.Model)
	}
	maxTurns := cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 300
	}

	tools := BuildToolDefs(cfg.Tools)
	tools = append(tools, cfg.ExtraTools...)
	var executor *ToolExecutor
	if len(cfg.SandboxPaths) > 0 {
		executor = NewSandboxedToolExecutor(cfg.WorkDir, cfg.SandboxPaths, cfg.Tools)
	} else {
		executor = NewToolExecutor(cfg.WorkDir)
	}
	for name, handler := range cfg.ExtraHandlers {
		executor.handlers[name] = handler
	}

	// Build messages
	var messages []ChatMessage
	if cfg.System != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: cfg.System})
	}
	for _, h := range cfg.History {
		messages = append(messages, ChatMessage{Role: h.Role, Content: h.Content})
	}
	messages = append(messages, ChatMessage{Role: "user", Content: userMessage})

	var totalInput, totalOutput int64
	var textBuf strings.Builder
	numTurns := 0
	lastStopReason := ""

	emit := func(ev StreamEvent) {
		if cfg.OnEvent != nil {
			cfg.OnEvent(ev)
		}
	}

	// Truncate large tool results from older turns to control token growth.
	// Keeps the last turn's tool results intact but trims older ones.
	truncateOldToolResults := func() {
		if len(messages) < 6 {
			return
		}
		// Find the last assistant message index (start of current turn's results)
		lastAssistantIdx := -1
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "assistant" {
				lastAssistantIdx = i
				break
			}
		}
		for i := 0; i < lastAssistantIdx; i++ {
			if messages[i].Role == "tool" && len(messages[i].Content) > 2000 {
				// Check for base64 data patterns or just large content
				content := messages[i].Content
				if strings.Contains(content, "\"screenshot\"") || len(content) > 5000 {
					messages[i].Content = content[:500] + "\n...[truncated large tool output]..."
				} else {
					messages[i].Content = content[:2000] + "\n...[truncated]..."
				}
			}
		}
	}

	for numTurns < maxTurns {
		select {
		case <-ctx.Done():
			return &AgentResult{
				Text:         textBuf.String(),
				InputTokens:  totalInput,
				OutputTokens: totalOutput,
				TotalCostUSD: CalculateCost(cfg.Model, totalInput, totalOutput),
				NumTurns:     numTurns,
				StopReason:   "cancelled",
			}, ctx.Err()
		default:
		}

		numTurns++

		// Trim old large tool results before each API call
		truncateOldToolResults()

		reqBody := ChatCompletionRequest{
			Model:     cfg.Model,
			Messages:  messages,
			MaxTokens: maxTokens,
		}
		if len(tools) > 0 {
			reqBody.Tools = tools
		}

		// Stream the response
		resp, err := c.doStreamRequest(ctx, reqBody)
		if err != nil {
			if IsAuthError(err) {
				return nil, fmt.Errorf("API key invalid or expired")
			}
			return nil, fmt.Errorf("API stream error: %w", err)
		}

		streamRes, streamErr := processSSEStream(resp.Body, emit, cfg.Model)
		resp.Body.Close()

		if streamErr != nil {
			return nil, fmt.Errorf("stream processing error: %w", streamErr)
		}

		totalInput += streamRes.InputTokens
		totalOutput += streamRes.OutputTokens
		lastStopReason = streamRes.FinishReason

		if streamRes.Content != "" {
			if textBuf.Len() > 0 {
				textBuf.WriteString("\n\n")
			}
			textBuf.WriteString(streamRes.Content)
		}

		// Append assistant message to conversation
		assistantMsg := ChatMessage{
			Role:    "assistant",
			Content: streamRes.Content,
		}
		if len(streamRes.ToolCalls) > 0 {
			assistantMsg.ToolCalls = streamRes.ToolCalls
		}
		messages = append(messages, assistantMsg)

		// Check if we need to execute tools
		if streamRes.FinishReason != "tool_calls" || len(streamRes.ToolCalls) == 0 {
			break
		}

		if len(tools) == 0 {
			break
		}

		// Execute tool calls and add results
		for _, tc := range streamRes.ToolCalls {
			inputJSON := []byte(tc.Function.Arguments)

			var toolInput map[string]interface{}
			if len(inputJSON) > 0 {
				json.Unmarshal(inputJSON, &toolInput)
			}

			emit(StreamEvent{
				Type:      EventToolStart,
				ToolName:  tc.Function.Name,
				ToolID:    tc.ID,
				ToolInput: toolInput,
			})

			result := executor.Execute(ctx, tc.Function.Name, inputJSON)

			emit(StreamEvent{
				Type:       EventToolEnd,
				ToolName:   tc.Function.Name,
				ToolID:     tc.ID,
				ToolOutput: result.Output,
			})

			messages = append(messages, ChatMessage{
				Role:       "tool",
				Content:    result.Output,
				ToolCallID: tc.ID,
			})
		}
	}

	// Detect if we hit max turns while the model still wanted to use tools
	stopReason := lastStopReason
	if numTurns >= maxTurns && lastStopReason == "tool_calls" {
		stopReason = "max_turns"
	}

	cost := CalculateCost(cfg.Model, totalInput, totalOutput)

	emit(StreamEvent{
		Type:         EventResult,
		Result:       textBuf.String(),
		TotalCostUSD: cost,
		Usage: &ClaudeUsage{
			InputTokens:  int(totalInput),
			OutputTokens: int(totalOutput),
		},
		NumTurns: numTurns,
	})

	return &AgentResult{
		Text:         textBuf.String(),
		InputTokens:  totalInput,
		OutputTokens: totalOutput,
		TotalCostUSD: cost,
		NumTurns:     numTurns,
		StopReason:   stopReason,
	}, nil
}

func (c *Client) RunOneShot(ctx context.Context, model string, system, prompt string) (string, *UsageInfo, error) {
	if !c.IsConfigured() {
		return "", nil, fmt.Errorf("API client not configured")
	}

	maxTokens := MaxTokensForModel(model)

	var messages []ChatMessage
	if system != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: system})
	}
	messages = append(messages, ChatMessage{Role: "user", Content: prompt})

	result, err := c.doRequest(ctx, ChatCompletionRequest{
		Model:     model,
		Messages:  messages,
		MaxTokens: maxTokens,
	})
	if err != nil {
		return "", nil, fmt.Errorf("API error: %w", err)
	}

	var text string
	if len(result.Choices) > 0 {
		text = result.Choices[0].Message.Content
	}

	usage := &UsageInfo{
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
		CostUSD:      CalculateCost(model, result.Usage.PromptTokens, result.Usage.CompletionTokens),
	}

	return text, usage, nil
}
