package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func handleWebFetch(ctx context.Context, _ string, input json.RawMessage) ToolResult {
	var params struct {
		URL    string `json:"url"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, "GET", params.URL, nil)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Invalid URL: %v", err), IsError: true}
	}
	req.Header.Set("User-Agent", "OpenPaw/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Fetch error: %v", err), IsError: true}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Read error: %v", err), IsError: true}
	}

	content := string(body)
	// Strip HTML tags for cleaner output
	content = stripHTMLTags(content)
	if len(content) > 50000 {
		content = content[:50000] + "\n... [content truncated]"
	}

	return ToolResult{Output: fmt.Sprintf("URL: %s\nStatus: %d\n\n%s", params.URL, resp.StatusCode, content)}
}

func stripHTMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func handleWebSearch(_ context.Context, _ string, input json.RawMessage) ToolResult {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
	}
	return ToolResult{Output: "Web search is not available in this environment. Try using WebFetch with a specific URL instead."}
}
