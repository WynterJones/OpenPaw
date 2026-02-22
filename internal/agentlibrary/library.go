package agentlibrary

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
)

//go:embed catalog/*
var catalogFS embed.FS

type CatalogAgent struct {
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	Description string  `json:"description"`
	Version    string   `json:"version"`
	Category   string   `json:"category"`
	Icon       string   `json:"icon"`
	Tags       []string `json:"tags"`
	Model      string   `json:"model"`
	AvatarPath string   `json:"avatar_path"`
	Installed  bool     `json:"installed"`
}

func LoadRegistry() ([]CatalogAgent, error) {
	data, err := catalogFS.ReadFile("catalog/registry.json")
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}

	var agents []CatalogAgent
	if err := json.Unmarshal(data, &agents); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	return agents, nil
}

func GetCatalogAgent(slug string) (*CatalogAgent, error) {
	agentList, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	for _, a := range agentList {
		if a.Slug == slug {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("catalog agent not found: %s", slug)
}

func GetTemplateFiles(slug string) (map[string][]byte, error) {
	prefix := "catalog/" + slug
	files := make(map[string][]byte)

	err := fs.WalkDir(catalogFS, prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(prefix, path)
		data, err := catalogFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		files[rel] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk catalog/%s: %w", slug, err)
	}
	return files, nil
}

func InstallAgent(db *database.DB, slug, dataDir string) (string, error) {
	cat, err := GetCatalogAgent(slug)
	if err != nil {
		return "", err
	}

	templateFiles, err := GetTemplateFiles(slug)
	if err != nil {
		return "", fmt.Errorf("get template files: %w", err)
	}

	id := uuid.New().String()

	// Get next sort_order from agent_roles table
	var maxOrder int
	err = db.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM agent_roles").Scan(&maxOrder)
	if err != nil {
		return "", fmt.Errorf("get max sort_order: %w", err)
	}
	sortOrder := maxOrder + 1

	// Insert the agent_roles record
	now := time.Now().UTC()
	_, err = db.Exec(
		`INSERT INTO agent_roles (id, slug, name, description, system_prompt, model, avatar_path, enabled, sort_order, is_preset, identity_initialized, library_slug, library_version, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, 0, 1, ?, ?, ?, ?)`,
		id, slug, cat.Name, cat.Description, "", cat.Model, cat.AvatarPath, sortOrder, cat.Slug, cat.Version, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("insert agent_roles record: %w", err)
	}

	// Create agent directory structure via InitAgentDir
	if err := agents.InitAgentDir(dataDir, slug, cat.Name, ""); err != nil {
		return "", fmt.Errorf("init agent dir: %w", err)
	}

	// Overwrite identity files with catalog-specific content
	agentDir := agents.AgentDir(dataDir, slug)
	for relPath, content := range templateFiles {
		outPath := filepath.Join(agentDir, relPath)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return "", fmt.Errorf("create dir for %s: %w", relPath, err)
		}
		if err := os.WriteFile(outPath, content, 0644); err != nil {
			return "", fmt.Errorf("write %s: %w", relPath, err)
		}
	}

	logger.Success("Installed library agent: %s (%s)", cat.Name, slug)
	return slug, nil
}

func IsInstalled(db *database.DB, slug string) (bool, string) {
	var existingSlug string
	err := db.QueryRow(
		"SELECT slug FROM agent_roles WHERE library_slug = ? LIMIT 1",
		slug,
	).Scan(&existingSlug)
	if err != nil {
		return false, ""
	}
	return true, existingSlug
}

func MarkInstalled(agentList []CatalogAgent, db *database.DB) []CatalogAgent {
	result := make([]CatalogAgent, len(agentList))
	copy(result, agentList)
	for i := range result {
		installed, _ := IsInstalled(db, result[i].Slug)
		result[i].Installed = installed
	}
	return result
}
