package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolvePath(workDir, filePath string) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}
	return filepath.Join(workDir, filePath)
}

func handleRead(_ context.Context, workDir string, input json.RawMessage) ToolResult {
	var params struct {
		FilePath string `json:"file_path"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
	}

	path := resolvePath(workDir, params.FilePath)
	f, err := os.Open(path)
	if err != nil {
		return ToolResult{Output: "Error reading file: " + err.Error(), IsError: true}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var lines []string
	lineNum := 0
	startLine := params.Offset
	if startLine < 1 {
		startLine = 1
	}
	maxLines := params.Limit
	if maxLines <= 0 {
		maxLines = 2000
	}

	for scanner.Scan() {
		lineNum++
		if lineNum < startLine {
			continue
		}
		if len(lines) >= maxLines {
			break
		}
		line := scanner.Text()
		if len(line) > 2000 {
			line = line[:2000] + "..."
		}
		lines = append(lines, fmt.Sprintf("%6d\t%s", lineNum, line))
	}

	if len(lines) == 0 {
		return ToolResult{Output: "(empty file)"}
	}
	return ToolResult{Output: strings.Join(lines, "\n")}
}

func handleWrite(_ context.Context, workDir string, input json.RawMessage) ToolResult {
	var params struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
	}

	path := resolvePath(workDir, params.FilePath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return ToolResult{Output: "Error creating directory: " + err.Error(), IsError: true}
	}
	if err := os.WriteFile(path, []byte(params.Content), 0644); err != nil {
		return ToolResult{Output: "Error writing file: " + err.Error(), IsError: true}
	}
	return ToolResult{Output: fmt.Sprintf("Successfully wrote %d bytes to %s", len(params.Content), params.FilePath)}
}

func handleEdit(_ context.Context, workDir string, input json.RawMessage) ToolResult {
	var params struct {
		FilePath string `json:"file_path"`
		OldText  string `json:"old_text"`
		NewText  string `json:"new_text"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
	}

	path := resolvePath(workDir, params.FilePath)
	content, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Output: "Error reading file: " + err.Error(), IsError: true}
	}

	text := string(content)
	count := strings.Count(text, params.OldText)
	if count == 0 {
		return ToolResult{Output: "old_text not found in file", IsError: true}
	}
	if count > 1 {
		return ToolResult{Output: fmt.Sprintf("old_text found %d times — must be unique", count), IsError: true}
	}

	newContent := strings.Replace(text, params.OldText, params.NewText, 1)
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return ToolResult{Output: "Error writing file: " + err.Error(), IsError: true}
	}
	return ToolResult{Output: "Edit applied successfully"}
}
