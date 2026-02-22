package skilllibrary

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openpaw/openpaw/internal/agents"
)

//go:embed all:catalog
var catalogFS embed.FS

type CatalogSkill struct {
	Slug          string   `json:"slug"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Version       string   `json:"version"`
	Category      string   `json:"category"`
	Icon          string   `json:"icon"`
	Tags          []string `json:"tags"`
	UsesTools     bool     `json:"uses_tools"`
	RequiredTools []string `json:"required_tools,omitempty"`
	Installed     bool     `json:"installed"`
}

func LoadRegistry() ([]CatalogSkill, error) {
	data, err := catalogFS.ReadFile("catalog/registry.json")
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}

	var skills []CatalogSkill
	if err := json.Unmarshal(data, &skills); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	return skills, nil
}

func GetCatalogSkill(slug string) (*CatalogSkill, error) {
	skills, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	for _, s := range skills {
		if s.Slug == slug {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("catalog skill not found: %s", slug)
}

func InstallSkill(dataDir, slug string) error {
	cat, err := GetCatalogSkill(slug)
	if err != nil {
		return err
	}

	content, err := catalogFS.ReadFile("catalog/" + slug + "/SKILL.md")
	if err != nil {
		return fmt.Errorf("read skill content: %w", err)
	}

	if err := agents.WriteGlobalSkill(dataDir, cat.Slug, string(content)); err != nil {
		return fmt.Errorf("write skill: %w", err)
	}
	return nil
}

func IsInstalled(dataDir, slug string) bool {
	path := filepath.Join(dataDir, "skills", slug, "SKILL.md")
	_, err := os.Stat(path)
	return err == nil
}

func MarkInstalled(skills []CatalogSkill, dataDir string) []CatalogSkill {
	result := make([]CatalogSkill, len(skills))
	copy(result, skills)
	for i := range result {
		result[i].Installed = IsInstalled(dataDir, result[i].Slug)
	}
	return result
}
