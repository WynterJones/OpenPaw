package toollibrary

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"text/template"
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

// --- Shared helpers for catalog-wide template tests ---

type ManifestEndpoint struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

type ManifestJSON struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Version     string             `json:"version"`
	HealthCheck string             `json:"health_check"`
	Endpoints   []ManifestEndpoint `json:"endpoints"`
	Env         json.RawMessage    `json:"env"`
	Widget      json.RawMessage    `json:"widget"`
}

func extractEnvNames(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var strs []string
	if json.Unmarshal(raw, &strs) == nil {
		return strs
	}
	var objs []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &objs) == nil {
		names := make([]string, len(objs))
		for i, o := range objs {
			names[i] = o.Name
		}
		return names
	}
	return nil
}

func testTemplateData() TemplateData {
	return TemplateData{
		ToolID:      "test-uuid-1234-5678-abcd-ef0123456789",
		Name:        "Test Tool",
		SlugName:    "test-tool",
		Description: "A test tool for unit testing",
	}
}

func testFuncMap() template.FuncMap {
	return template.FuncMap{
		"jsonEscape": func(s string) string {
			b, _ := json.Marshal(s)
			return strings.Trim(string(b), `"`)
		},
	}
}

func renderTemplate(t *testing.T, filename, content string, data TemplateData) string {
	t.Helper()
	tmpl, err := template.New(filename).Funcs(testFuncMap()).Parse(content)
	if err != nil {
		t.Fatalf("parse template %s: %v", filename, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("execute template %s: %v", filename, err)
	}
	return buf.String()
}

func runInDir(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed:\n%s", name, args, out)
	}
}

func TestAllToolsTemplateRendering(t *testing.T) {
	tools, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}

	data := testTemplateData()

	for _, tool := range tools {
		t.Run(tool.Slug, func(t *testing.T) {
			files, err := GetSourceFiles(tool.Slug)
			if err != nil {
				t.Fatalf("GetSourceFiles(%q) error: %v", tool.Slug, err)
			}

			tmplCount := 0
			for filename, content := range files {
				if !strings.HasSuffix(filename, ".tmpl") {
					continue
				}
				tmplCount++
				t.Run(filename, func(t *testing.T) {
					rendered := renderTemplate(t, filename, string(content), data)
					if len(strings.TrimSpace(rendered)) == 0 {
						t.Errorf("template %s rendered to empty output", filename)
					}
				})
			}

			if tmplCount == 0 {
				t.Errorf("tool %q has no .tmpl files", tool.Slug)
			}
		})
	}
}

func TestAllToolsManifestValidation(t *testing.T) {
	tools, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}

	data := testTemplateData()

	for _, tool := range tools {
		t.Run(tool.Slug, func(t *testing.T) {
			files, err := GetSourceFiles(tool.Slug)
			if err != nil {
				t.Fatalf("GetSourceFiles(%q) error: %v", tool.Slug, err)
			}

			manifestTmpl, ok := files["manifest.json.tmpl"]
			if !ok {
				t.Fatalf("tool %q missing manifest.json.tmpl", tool.Slug)
			}

			rendered := renderTemplate(t, "manifest.json.tmpl", string(manifestTmpl), data)

			var manifest ManifestJSON
			if err := json.Unmarshal([]byte(rendered), &manifest); err != nil {
				t.Fatalf("invalid JSON in manifest: %v\nrendered:\n%s", err, rendered)
			}

			if manifest.ID == "" {
				t.Error("manifest.id is empty")
			}
			if manifest.Name == "" {
				t.Error("manifest.name is empty")
			}
			if manifest.Description == "" {
				t.Error("manifest.description is empty")
			}
			if manifest.Version == "" {
				t.Error("manifest.version is empty")
			}
			if manifest.HealthCheck == "" {
				t.Error("manifest.health_check is empty")
			}

			if len(manifest.Endpoints) == 0 {
				t.Error("manifest.endpoints is empty")
			}
			for i, ep := range manifest.Endpoints {
				if ep.Method == "" {
					t.Errorf("endpoint[%d].method is empty", i)
				}
				if ep.Path == "" {
					t.Errorf("endpoint[%d].path is empty", i)
				}
				if ep.Description == "" {
					t.Errorf("endpoint[%d].description is empty", i)
				}
			}

			// Cross-check: manifest env should match registry env
			registryEnv := append([]string{}, tool.Env...)
			manifestEnv := extractEnvNames(manifest.Env)
			sort.Strings(registryEnv)
			sort.Strings(manifestEnv)

			if len(registryEnv) != len(manifestEnv) {
				t.Errorf("env mismatch: registry=%v, manifest=%v", tool.Env, manifestEnv)
			} else {
				for i := range registryEnv {
					if registryEnv[i] != manifestEnv[i] {
						t.Errorf("env mismatch at [%d]: registry=%q, manifest=%q", i, registryEnv[i], manifestEnv[i])
					}
				}
			}
		})
	}
}

func TestAllToolsCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compilation test in short mode")
	}

	tools, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}

	data := testTemplateData()

	for _, tool := range tools {
		t.Run(tool.Slug, func(t *testing.T) {
			t.Parallel()

			files, err := GetSourceFiles(tool.Slug)
			if err != nil {
				t.Fatalf("GetSourceFiles(%q) error: %v", tool.Slug, err)
			}

			dir := t.TempDir()

			for filename, content := range files {
				outName := filename
				var outContent []byte

				if strings.HasSuffix(filename, ".tmpl") {
					outName = strings.TrimSuffix(filename, ".tmpl")
					rendered := renderTemplate(t, filename, string(content), data)
					outContent = []byte(rendered)
				} else {
					outContent = content
				}

				outPath := filepath.Join(dir, outName)
				if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
					t.Fatalf("mkdir for %s: %v", outName, err)
				}
				if err := os.WriteFile(outPath, outContent, 0644); err != nil {
					t.Fatalf("write %s: %v", outName, err)
				}
			}

			runInDir(t, dir, "go", "mod", "tidy")
			runInDir(t, dir, "go", "build", ".")
		})
	}
}
