package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

func handleNotebookEdit(_ context.Context, workDir string, input json.RawMessage) ToolResult {
	var params struct {
		NotebookPath string `json:"notebook_path"`
		NewSource    string `json:"new_source"`
		CellNumber   int    `json:"cell_number"`
		EditMode     string `json:"edit_mode"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
	}

	path := resolvePath(workDir, params.NotebookPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Output: "Error reading notebook: " + err.Error(), IsError: true}
	}

	var notebook map[string]interface{}
	if err := json.Unmarshal(data, &notebook); err != nil {
		return ToolResult{Output: "Error parsing notebook JSON: " + err.Error(), IsError: true}
	}

	cells, ok := notebook["cells"].([]interface{})
	if !ok {
		return ToolResult{Output: "Invalid notebook format: no cells array", IsError: true}
	}

	mode := params.EditMode
	if mode == "" {
		mode = "replace"
	}

	sourceLines := []interface{}{params.NewSource}

	switch mode {
	case "replace":
		if params.CellNumber < 0 || params.CellNumber >= len(cells) {
			return ToolResult{Output: fmt.Sprintf("Cell number %d out of range (0-%d)", params.CellNumber, len(cells)-1), IsError: true}
		}
		cell, ok := cells[params.CellNumber].(map[string]interface{})
		if !ok {
			return ToolResult{Output: "Invalid cell format", IsError: true}
		}
		cell["source"] = sourceLines
	case "insert":
		newCell := map[string]interface{}{
			"cell_type": "code",
			"source":    sourceLines,
			"metadata":  map[string]interface{}{},
			"outputs":   []interface{}{},
		}
		idx := params.CellNumber
		if idx > len(cells) {
			idx = len(cells)
		}
		cells = append(cells[:idx], append([]interface{}{newCell}, cells[idx:]...)...)
		notebook["cells"] = cells
	case "delete":
		if params.CellNumber < 0 || params.CellNumber >= len(cells) {
			return ToolResult{Output: fmt.Sprintf("Cell number %d out of range", params.CellNumber), IsError: true}
		}
		cells = append(cells[:params.CellNumber], cells[params.CellNumber+1:]...)
		notebook["cells"] = cells
	default:
		return ToolResult{Output: "Unknown edit_mode: " + mode, IsError: true}
	}

	out, err := json.MarshalIndent(notebook, "", " ")
	if err != nil {
		return ToolResult{Output: "Error serializing notebook: " + err.Error(), IsError: true}
	}

	if err := os.WriteFile(path, out, 0644); err != nil {
		return ToolResult{Output: "Error writing notebook: " + err.Error(), IsError: true}
	}

	return ToolResult{Output: fmt.Sprintf("Notebook %s: cell %d", mode, params.CellNumber)}
}
