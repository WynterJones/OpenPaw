package llm

import (
	"encoding/json"
)

type ToolDef struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

func BuildCallToolDef() ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tool_id":  map[string]interface{}{"type": "string", "description": "The tool's ID (from the AVAILABLE TOOLS section)"},
			"endpoint": map[string]interface{}{"type": "string", "description": "The endpoint path to call (e.g. /weather/London)"},
			"method":   map[string]interface{}{"type": "string", "description": "HTTP method: GET or POST (default GET)", "enum": []string{"GET", "POST"}},
			"payload":  map[string]interface{}{"type": "string", "description": "JSON request body for POST requests"},
		},
		"required": []string{"tool_id", "endpoint"},
	})
	return ToolDef{
		Type: "function",
		Function: FunctionDef{
			Name:        "call_tool",
			Description: "Call one of the user's custom tools by making an HTTP request to it. Use this when the user asks you to do something that one of the available tools can handle.",
			Parameters:  params,
		},
	}
}

func BuildDelegateTaskDef() ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tasks": map[string]interface{}{
				"type":        "array",
				"description": "Array of tasks to delegate to specialist agents. Each task runs in parallel.",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_slug": map[string]interface{}{"type": "string", "description": "The slug of the agent to delegate to (from the DELEGATION section)"},
						"task":       map[string]interface{}{"type": "string", "description": "A clear, specific description of the task for the sub-agent to complete"},
					},
					"required": []string{"agent_slug", "task"},
				},
				"maxItems": 5,
			},
		},
		"required": []string{"tasks"},
	})
	return ToolDef{
		Type: "function",
		Function: FunctionDef{
			Name:        "delegate_task",
			Description: "Delegate tasks to specialist agents who work in parallel. Use when multiple independent subtasks would benefit from specialist expertise or parallel execution. Each agent receives only the task description (no conversation history) and returns its findings.",
			Parameters:  params,
		},
	}
}

func BuildToolDefs(names []string) []ToolDef {
	allowed := make(map[string]bool, len(names))
	for _, n := range names {
		allowed[n] = true
	}

	var tools []ToolDef
	for name, schema := range toolSchemas {
		if !allowed[name] {
			continue
		}
		params, _ := json.Marshal(schema.InputSchema)
		tools = append(tools, ToolDef{
			Type: "function",
			Function: FunctionDef{
				Name:        name,
				Description: schema.Description,
				Parameters:  params,
			},
		})
	}
	return tools
}

type toolSchema struct {
	Description string
	InputSchema map[string]interface{}
}

var toolSchemas = map[string]toolSchema{
	"Read": {
		Description: "Read a file from the filesystem. Returns file contents with line numbers.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": map[string]interface{}{"type": "string", "description": "Absolute path to the file to read"},
				"offset":    map[string]interface{}{"type": "integer", "description": "Line number to start reading from (1-based)"},
				"limit":     map[string]interface{}{"type": "integer", "description": "Number of lines to read"},
			},
			"required": []string{"file_path"},
		},
	},
	"Write": {
		Description: "Write content to a file, creating directories as needed.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": map[string]interface{}{"type": "string", "description": "Absolute path to the file to write"},
				"content":   map[string]interface{}{"type": "string", "description": "Content to write"},
			},
			"required": []string{"file_path", "content"},
		},
	},
	"Edit": {
		Description: "Replace exact text in a file. old_text must match exactly.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": map[string]interface{}{"type": "string", "description": "Absolute path to the file to edit"},
				"old_text":  map[string]interface{}{"type": "string", "description": "Exact text to find and replace"},
				"new_text":  map[string]interface{}{"type": "string", "description": "Text to replace with"},
			},
			"required": []string{"file_path", "old_text", "new_text"},
		},
	},
	"Bash": {
		Description: "Execute a bash command and return stdout+stderr.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{"type": "string", "description": "The bash command to execute"},
				"timeout": map[string]interface{}{"type": "integer", "description": "Timeout in seconds (default 120, max 600)"},
			},
			"required": []string{"command"},
		},
	},
	"Grep": {
		Description: "Search file contents using regex patterns. Uses ripgrep if available.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern":     map[string]interface{}{"type": "string", "description": "Regex pattern to search for"},
				"path":        map[string]interface{}{"type": "string", "description": "Directory or file to search in"},
				"glob":        map[string]interface{}{"type": "string", "description": "Glob pattern to filter files (e.g. *.go)"},
				"output_mode": map[string]interface{}{"type": "string", "description": "Output mode: content, files_with_matches, or count"},
			},
			"required": []string{"pattern"},
		},
	},
	"Glob": {
		Description: "Find files matching a glob pattern.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{"type": "string", "description": "Glob pattern (e.g. **/*.go)"},
				"path":    map[string]interface{}{"type": "string", "description": "Directory to search in"},
			},
			"required": []string{"pattern"},
		},
	},
	"WebFetch": {
		Description: "Fetch content from a URL.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":    map[string]interface{}{"type": "string", "description": "URL to fetch"},
				"prompt": map[string]interface{}{"type": "string", "description": "What to extract from the page"},
			},
			"required": []string{"url"},
		},
	},
	"WebSearch": {
		Description: "Search the web for information.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string", "description": "Search query"},
			},
			"required": []string{"query"},
		},
	},
	"NotebookEdit": {
		Description: "Edit a Jupyter notebook cell.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"notebook_path": map[string]interface{}{"type": "string", "description": "Path to the notebook file"},
				"new_source":    map[string]interface{}{"type": "string", "description": "New cell content"},
				"cell_number":   map[string]interface{}{"type": "integer", "description": "Cell index (0-based)"},
				"edit_mode":     map[string]interface{}{"type": "string", "description": "replace, insert, or delete"},
			},
			"required": []string{"notebook_path", "new_source"},
		},
	},
}
