package skilllibrary

import (
	"encoding/json"
	"testing"
)

func TestLoadRegistry(t *testing.T) {
	skills, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("LoadRegistry() returned empty list")
	}
	if len(skills) != 10 {
		t.Errorf("LoadRegistry() returned %d skills, want 10", len(skills))
	}

	slugs := make(map[string]bool)
	for i, skill := range skills {
		if skill.Slug == "" {
			t.Errorf("skills[%d]: slug is empty", i)
		}
		if skill.Name == "" {
			t.Errorf("skills[%d] (%s): name is empty", i, skill.Slug)
		}
		if skill.Description == "" {
			t.Errorf("skills[%d] (%s): description is empty", i, skill.Slug)
		}
		if skill.Version == "" {
			t.Errorf("skills[%d] (%s): version is empty", i, skill.Slug)
		}
		if skill.Category == "" {
			t.Errorf("skills[%d] (%s): category is empty", i, skill.Slug)
		}
		if skill.Icon == "" {
			t.Errorf("skills[%d] (%s): icon is empty", i, skill.Slug)
		}
		if len(skill.Tags) == 0 {
			t.Errorf("skills[%d] (%s): tags is empty", i, skill.Slug)
		}
		if slugs[skill.Slug] {
			t.Errorf("skills[%d]: duplicate slug %q", i, skill.Slug)
		}
		slugs[skill.Slug] = true
	}
}

func TestLoadRegistryJSON(t *testing.T) {
	skills, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	data, err := json.Marshal(skills)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	var roundTripped []CatalogSkill
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	if len(roundTripped) != len(skills) {
		t.Errorf("round-trip length mismatch: got %d, want %d", len(roundTripped), len(skills))
	}
}

func TestLoadRegistryCategories(t *testing.T) {
	skills, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	validCategories := map[string]bool{
		"Meta":         true,
		"Coding":       true,
		"Writing":      true,
		"Data":         true,
		"Productivity": true,
		"Media":        true,
	}
	for _, skill := range skills {
		if !validCategories[skill.Category] {
			t.Errorf("skill %q: unexpected category %q", skill.Slug, skill.Category)
		}
	}
}

func TestGetCatalogSkill(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		skill, err := GetCatalogSkill("code-review")
		if err != nil {
			t.Fatalf("GetCatalogSkill(code-review) error: %v", err)
		}
		if skill.Slug != "code-review" {
			t.Errorf("slug = %q, want %q", skill.Slug, "code-review")
		}
		if skill.Name == "" {
			t.Error("name should not be empty")
		}
	})

	t.Run("all slugs resolvable", func(t *testing.T) {
		skills, _ := LoadRegistry()
		for _, s := range skills {
			found, err := GetCatalogSkill(s.Slug)
			if err != nil {
				t.Errorf("GetCatalogSkill(%q) error: %v", s.Slug, err)
				continue
			}
			if found.Name != s.Name {
				t.Errorf("GetCatalogSkill(%q).Name = %q, want %q", s.Slug, found.Name, s.Name)
			}
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := GetCatalogSkill("nonexistent-skill-xyz")
		if err == nil {
			t.Error("expected error for nonexistent slug")
		}
	})
}

func TestAllSkillsHaveContent(t *testing.T) {
	skills, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}

	for _, skill := range skills {
		t.Run(skill.Slug, func(t *testing.T) {
			content, err := catalogFS.ReadFile("catalog/" + skill.Slug + "/SKILL.md")
			if err != nil {
				t.Fatalf("read SKILL.md for %q: %v", skill.Slug, err)
			}
			if len(content) < 10 {
				t.Errorf("skill %q: SKILL.md is too short (%d bytes)", skill.Slug, len(content))
			}
		})
	}
}

func TestCatalogSkillInstalledDefaultFalse(t *testing.T) {
	skills, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	for _, skill := range skills {
		if skill.Installed {
			t.Errorf("skill %q: Installed should default to false from registry", skill.Slug)
		}
	}
}

func TestExpectedSlugs(t *testing.T) {
	skills, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}

	expected := []string{
		"skill-creator", "code-review", "technical-writing", "data-analyst",
		"research-assistant", "test-writer", "image-processing", "video-processing",
		"pdf-tools", "api-integration",
	}

	slugSet := make(map[string]bool)
	for _, s := range skills {
		slugSet[s.Slug] = true
	}

	for _, slug := range expected {
		if !slugSet[slug] {
			t.Errorf("expected slug %q not found in registry", slug)
		}
	}
}

func TestUsesToolsConsistency(t *testing.T) {
	skills, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	for _, skill := range skills {
		if skill.UsesTools && len(skill.RequiredTools) == 0 {
			t.Errorf("skill %q: uses_tools=true but required_tools is empty", skill.Slug)
		}
		if !skill.UsesTools && len(skill.RequiredTools) > 0 {
			t.Errorf("skill %q: uses_tools=false but required_tools has %d entries", skill.Slug, len(skill.RequiredTools))
		}
	}
}
