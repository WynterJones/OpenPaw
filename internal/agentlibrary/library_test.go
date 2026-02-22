package agentlibrary

import (
	"encoding/json"
	"testing"
)

func TestLoadRegistry(t *testing.T) {
	agents, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	if len(agents) == 0 {
		t.Fatal("LoadRegistry() returned empty list")
	}
	if len(agents) != 20 {
		t.Errorf("LoadRegistry() returned %d agents, want 20", len(agents))
	}

	slugs := make(map[string]bool)
	for i, agent := range agents {
		if agent.Slug == "" {
			t.Errorf("agents[%d]: slug is empty", i)
		}
		if agent.Name == "" {
			t.Errorf("agents[%d] (%s): name is empty", i, agent.Slug)
		}
		if agent.Description == "" {
			t.Errorf("agents[%d] (%s): description is empty", i, agent.Slug)
		}
		if agent.Version == "" {
			t.Errorf("agents[%d] (%s): version is empty", i, agent.Slug)
		}
		if agent.Category == "" {
			t.Errorf("agents[%d] (%s): category is empty", i, agent.Slug)
		}
		if agent.Icon == "" {
			t.Errorf("agents[%d] (%s): icon is empty", i, agent.Slug)
		}
		if agent.Model == "" {
			t.Errorf("agents[%d] (%s): model is empty", i, agent.Slug)
		}
		if agent.AvatarPath == "" {
			t.Errorf("agents[%d] (%s): avatar_path is empty", i, agent.Slug)
		}
		if len(agent.Tags) == 0 {
			t.Errorf("agents[%d] (%s): tags is empty", i, agent.Slug)
		}
		if slugs[agent.Slug] {
			t.Errorf("agents[%d]: duplicate slug %q", i, agent.Slug)
		}
		slugs[agent.Slug] = true
	}
}

func TestLoadRegistryJSON(t *testing.T) {
	agents, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	data, err := json.Marshal(agents)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	var roundTripped []CatalogAgent
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	if len(roundTripped) != len(agents) {
		t.Errorf("round-trip length mismatch: got %d, want %d", len(roundTripped), len(agents))
	}
}

func TestLoadRegistryModels(t *testing.T) {
	agents, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	validModels := map[string]bool{"sonnet": true, "haiku": true, "opus": true}
	for _, agent := range agents {
		if !validModels[agent.Model] {
			t.Errorf("agent %q: invalid model %q (expected sonnet, haiku, or opus)", agent.Slug, agent.Model)
		}
	}
}

func TestLoadRegistryCategories(t *testing.T) {
	agents, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	validCategories := map[string]bool{
		"Productivity": true,
		"Engineering":  true,
		"Content":      true,
		"Data":         true,
		"Ops":          true,
		"Personal":     true,
		"Design":       true,
		"Business":     true,
		"Support":      true,
		"Creative":     true,
	}
	for _, agent := range agents {
		if !validCategories[agent.Category] {
			t.Errorf("agent %q: unexpected category %q", agent.Slug, agent.Category)
		}
	}
}

func TestGetCatalogAgent(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		agent, err := GetCatalogAgent("researcher")
		if err != nil {
			t.Fatalf("GetCatalogAgent(researcher) error: %v", err)
		}
		if agent.Slug != "researcher" {
			t.Errorf("slug = %q, want %q", agent.Slug, "researcher")
		}
		if agent.Name == "" {
			t.Error("name should not be empty")
		}
		if agent.Model == "" {
			t.Error("model should not be empty")
		}
	})

	t.Run("all slugs resolvable", func(t *testing.T) {
		agents, _ := LoadRegistry()
		for _, a := range agents {
			found, err := GetCatalogAgent(a.Slug)
			if err != nil {
				t.Errorf("GetCatalogAgent(%q) error: %v", a.Slug, err)
				continue
			}
			if found.Name != a.Name {
				t.Errorf("GetCatalogAgent(%q).Name = %q, want %q", a.Slug, found.Name, a.Name)
			}
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := GetCatalogAgent("nonexistent-agent-xyz")
		if err == nil {
			t.Error("expected error for nonexistent slug")
		}
	})
}

func TestGetTemplateFiles(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		files, err := GetTemplateFiles("researcher")
		if err != nil {
			t.Fatalf("GetTemplateFiles(researcher) error: %v", err)
		}
		if len(files) == 0 {
			t.Fatal("GetTemplateFiles(researcher) returned no files")
		}

		// Each agent should have SOUL.md, AGENTS.md, BOOT.md
		for _, required := range []string{"SOUL.md", "AGENTS.md", "BOOT.md"} {
			if _, ok := files[required]; !ok {
				t.Errorf("missing required file %q", required)
			}
		}
	})

	t.Run("content not empty", func(t *testing.T) {
		files, err := GetTemplateFiles("researcher")
		if err != nil {
			t.Fatalf("GetTemplateFiles(researcher) error: %v", err)
		}
		for name, content := range files {
			if len(content) == 0 {
				t.Errorf("file %q is empty", name)
			}
		}
	})

	t.Run("nonexistent returns empty", func(t *testing.T) {
		files, err := GetTemplateFiles("nonexistent-agent-xyz")
		if err == nil && len(files) != 0 {
			t.Errorf("expected 0 files or error for nonexistent slug, got %d files", len(files))
		}
	})
}

func TestAllAgentsHaveTemplateFiles(t *testing.T) {
	agents, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}

	requiredFiles := []string{"SOUL.md", "AGENTS.md", "BOOT.md"}

	for _, agent := range agents {
		t.Run(agent.Slug, func(t *testing.T) {
			files, err := GetTemplateFiles(agent.Slug)
			if err != nil {
				t.Fatalf("GetTemplateFiles(%q) error: %v", agent.Slug, err)
			}
			if len(files) == 0 {
				t.Fatalf("agent %q has no template files", agent.Slug)
			}
			for _, required := range requiredFiles {
				content, ok := files[required]
				if !ok {
					t.Errorf("agent %q missing %s", agent.Slug, required)
					continue
				}
				if len(content) < 10 {
					t.Errorf("agent %q: %s is too short (%d bytes)", agent.Slug, required, len(content))
				}
			}
		})
	}
}

func TestCatalogAgentInstalledDefaultFalse(t *testing.T) {
	agents, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	for _, agent := range agents {
		if agent.Installed {
			t.Errorf("agent %q: Installed should default to false from registry", agent.Slug)
		}
	}
}

func TestExpectedSlugs(t *testing.T) {
	agents, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}

	expected := []string{
		"researcher", "code-reviewer", "writer", "data-analyst", "debugger",
		"project-manager", "ops-engineer", "learning-coach", "security-auditor", "summarizer",
		"api-designer", "database-architect", "ux-designer", "compliance-advisor",
		"marketing-strategist", "customer-support", "translator", "brainstormer",
		"system-architect", "test-engineer",
	}

	slugSet := make(map[string]bool)
	for _, a := range agents {
		slugSet[a.Slug] = true
	}

	for _, slug := range expected {
		if !slugSet[slug] {
			t.Errorf("expected slug %q not found in registry", slug)
		}
	}
}
