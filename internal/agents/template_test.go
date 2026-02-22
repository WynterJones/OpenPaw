package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Weather API Tool", "weather-api-tool"},
		{"my-tool", "my-tool"},
		{"  Spaces Everywhere  ", "spaces-everywhere"},
		{"CamelCase", "camelcase"},
		{"lots___of---symbols!!!", "lots-of-symbols"},
		{"", ""},
		{"single", "single"},
		{"A & B Tool", "a-b-tool"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestJsonEscape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`plain text`, `plain text`},
		{`has "quotes"`, `has \"quotes\"`},
		{"has\nnewline", `has\nnewline`},
		{`back\slash`, `back\\slash`},
		{"tab\there", `tab\there`},
	}
	for _, tt := range tests {
		got := jsonEscape(tt.input)
		if got != tt.want {
			t.Errorf("jsonEscape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNewTemplateData(t *testing.T) {
	td := NewTemplateData("tool-123", "Weather API Tool", "Fetches weather data")
	if td.ToolID != "tool-123" {
		t.Errorf("ToolID = %q, want %q", td.ToolID, "tool-123")
	}
	if td.SlugName != "weather-api-tool" {
		t.Errorf("SlugName = %q, want %q", td.SlugName, "weather-api-tool")
	}
	if td.BinaryName != "weather-api-tool" {
		t.Errorf("BinaryName = %q, want %q", td.BinaryName, "weather-api-tool")
	}
	if td.CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
}

func TestScaffoldToolDir(t *testing.T) {
	dir := t.TempDir()
	data := NewTemplateData("test-tool-1", "Test Tool", "A tool for testing")

	if err := ScaffoldToolDir(dir, data); err != nil {
		t.Fatalf("ScaffoldToolDir() error: %v", err)
	}

	// Verify static files are copied verbatim
	for _, name := range []string{"handlers.go", "Justfile"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", name)
		}
	}

	// Verify .tmpl files are rendered (suffix stripped)
	for _, name := range []string{"main.go", "manifest.json", "go.mod"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected rendered file %s to exist", name)
		}
		// Verify .tmpl version does NOT exist
		tmplPath := path + ".tmpl"
		if _, err := os.Stat(tmplPath); !os.IsNotExist(err) {
			t.Errorf("expected %s.tmpl to NOT exist (should be rendered without suffix)", name)
		}
	}

	// Verify template values were interpolated
	mainContent, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !strings.Contains(string(mainContent), "Test Tool") {
		t.Error("main.go should contain the tool name")
	}
	if !strings.Contains(string(mainContent), "// TODO: Add your routes here") {
		t.Error("main.go should contain the TODO marker")
	}

	// Verify manifest.json has correct values
	manifestContent, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest.json: %v", err)
	}
	manifest := string(manifestContent)
	if !strings.Contains(manifest, `"test-tool-1"`) {
		t.Error("manifest.json should contain the tool ID")
	}
	if !strings.Contains(manifest, `"Test Tool"`) {
		t.Error("manifest.json should contain the tool name")
	}

	// Verify go.mod has slug
	modContent, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !strings.Contains(string(modContent), "openpaw-tool-test-tool") {
		t.Error("go.mod should contain the slugified module name")
	}
}

func TestScaffoldToolDirJsonEscaping(t *testing.T) {
	dir := t.TempDir()
	data := NewTemplateData("tool-2", `Tool with "quotes"`, "Description with \"special\" chars\nand newlines")

	if err := ScaffoldToolDir(dir, data); err != nil {
		t.Fatalf("ScaffoldToolDir() error: %v", err)
	}

	manifestContent, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest.json: %v", err)
	}
	manifest := string(manifestContent)

	// The name should have escaped quotes
	if strings.Contains(manifest, `"Tool with "quotes""`) {
		t.Error("manifest.json should have escaped quotes in tool name")
	}
}
