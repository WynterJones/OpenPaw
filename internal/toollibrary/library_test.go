package toollibrary

import (
	"encoding/json"
	"testing"
)

func TestLoadRegistry(t *testing.T) {
	tools, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("LoadRegistry() returned empty list")
	}

	// Verify each entry has required fields
	slugs := make(map[string]bool)
	for i, tool := range tools {
		if tool.Slug == "" {
			t.Errorf("tools[%d]: slug is empty", i)
		}
		if tool.Name == "" {
			t.Errorf("tools[%d] (%s): name is empty", i, tool.Slug)
		}
		if tool.Description == "" {
			t.Errorf("tools[%d] (%s): description is empty", i, tool.Slug)
		}
		if tool.Version == "" {
			t.Errorf("tools[%d] (%s): version is empty", i, tool.Slug)
		}
		if tool.Category == "" {
			t.Errorf("tools[%d] (%s): category is empty", i, tool.Slug)
		}
		if tool.Icon == "" {
			t.Errorf("tools[%d] (%s): icon is empty", i, tool.Slug)
		}
		if slugs[tool.Slug] {
			t.Errorf("tools[%d]: duplicate slug %q", i, tool.Slug)
		}
		slugs[tool.Slug] = true
	}
}

func TestLoadRegistryJSON(t *testing.T) {
	// Verify the registry can round-trip through JSON
	tools, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	data, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	var roundTripped []CatalogTool
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	if len(roundTripped) != len(tools) {
		t.Errorf("round-trip length mismatch: got %d, want %d", len(roundTripped), len(tools))
	}
}

func TestGetCatalogTool(t *testing.T) {
	// Load registry to get a known slug
	tools, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("no tools in registry")
	}

	firstSlug := tools[0].Slug

	t.Run("found", func(t *testing.T) {
		tool, err := GetCatalogTool(firstSlug)
		if err != nil {
			t.Fatalf("GetCatalogTool(%q) error: %v", firstSlug, err)
		}
		if tool.Slug != firstSlug {
			t.Errorf("slug = %q, want %q", tool.Slug, firstSlug)
		}
		if tool.Name == "" {
			t.Error("name should not be empty")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := GetCatalogTool("nonexistent-tool-xyz")
		if err == nil {
			t.Error("expected error for nonexistent slug")
		}
	})
}

func TestGetSourceFiles(t *testing.T) {
	tools, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("no tools in registry")
	}

	firstSlug := tools[0].Slug

	t.Run("found", func(t *testing.T) {
		files, err := GetSourceFiles(firstSlug)
		if err != nil {
			t.Fatalf("GetSourceFiles(%q) error: %v", firstSlug, err)
		}
		if len(files) == 0 {
			t.Errorf("GetSourceFiles(%q) returned no files", firstSlug)
		}
		// Each tool should have at least a handlers.go
		if _, ok := files["handlers.go"]; !ok {
			t.Errorf("GetSourceFiles(%q) missing handlers.go", firstSlug)
		}
	})

	t.Run("not found", func(t *testing.T) {
		files, err := GetSourceFiles("nonexistent-tool-xyz")
		if err == nil && len(files) != 0 {
			t.Errorf("expected 0 files or error for nonexistent slug, got %d files", len(files))
		}
	})
}

func TestAllToolsHaveSourceFiles(t *testing.T) {
	tools, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}

	for _, tool := range tools {
		t.Run(tool.Slug, func(t *testing.T) {
			files, err := GetSourceFiles(tool.Slug)
			if err != nil {
				t.Fatalf("GetSourceFiles(%q) error: %v", tool.Slug, err)
			}
			if len(files) == 0 {
				t.Errorf("tool %q has no source files", tool.Slug)
			}
		})
	}
}

func TestCatalogToolInstalledDefaultFalse(t *testing.T) {
	tools, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	for _, tool := range tools {
		if tool.Installed {
			t.Errorf("tool %q: Installed should default to false from registry", tool.Slug)
		}
	}
}
