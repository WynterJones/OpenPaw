package toollibrary

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
)

//go:embed catalog/*
var catalogFS embed.FS

type CatalogTool struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Category    string   `json:"category"`
	Icon        string   `json:"icon"`
	Tags        []string `json:"tags"`
	Env         []string `json:"env,omitempty"`
	Installed   bool     `json:"installed"`
}

type TemplateData struct {
	ToolID      string
	Name        string
	SlugName    string
	Description string
}

func LoadRegistry() ([]CatalogTool, error) {
	data, err := catalogFS.ReadFile("catalog/registry.json")
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}

	var tools []CatalogTool
	if err := json.Unmarshal(data, &tools); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	return tools, nil
}

func GetCatalogTool(slug string) (*CatalogTool, error) {
	tools, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	for _, t := range tools {
		if t.Slug == slug {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("catalog tool not found: %s", slug)
}

func GetSourceFiles(slug string) (map[string][]byte, error) {
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

func InstallTool(db *database.DB, slug, toolsDir string) (string, error) {
	cat, err := GetCatalogTool(slug)
	if err != nil {
		return "", err
	}

	files, err := GetSourceFiles(slug)
	if err != nil {
		return "", fmt.Errorf("get source files: %w", err)
	}

	toolID := uuid.New().String()
	toolDir := filepath.Join(toolsDir, toolID)
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		return "", fmt.Errorf("create tool dir: %w", err)
	}

	tmplData := TemplateData{
		ToolID:      toolID,
		Name:        cat.Name,
		SlugName:    cat.Slug,
		Description: cat.Description,
	}

	funcMap := template.FuncMap{
		"jsonEscape": func(s string) string {
			b, _ := json.Marshal(s)
			return strings.Trim(string(b), `"`)
		},
	}

	for filename, content := range files {
		outName := filename
		if strings.HasSuffix(filename, ".tmpl") {
			outName = strings.TrimSuffix(filename, ".tmpl")
			tmpl, err := template.New(filename).Funcs(funcMap).Parse(string(content))
			if err != nil {
				os.RemoveAll(toolDir)
				return "", fmt.Errorf("parse template %s: %w", filename, err)
			}
			outPath := filepath.Join(toolDir, outName)
			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				os.RemoveAll(toolDir)
				return "", fmt.Errorf("create dir for %s: %w", outName, err)
			}
			f, err := os.Create(outPath)
			if err != nil {
				os.RemoveAll(toolDir)
				return "", fmt.Errorf("create %s: %w", outName, err)
			}
			if err := tmpl.Execute(f, tmplData); err != nil {
				f.Close()
				os.RemoveAll(toolDir)
				return "", fmt.Errorf("execute template %s: %w", filename, err)
			}
			f.Close()
		} else {
			outPath := filepath.Join(toolDir, outName)
			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				os.RemoveAll(toolDir)
				return "", fmt.Errorf("create dir for %s: %w", outName, err)
			}
			if err := os.WriteFile(outPath, content, 0644); err != nil {
				os.RemoveAll(toolDir)
				return "", fmt.Errorf("write %s: %w", outName, err)
			}
		}
	}

	now := time.Now().UTC()
	_, err = db.Exec(
		`INSERT INTO tools (id, name, description, type, config, enabled, status, library_slug, library_version, created_at, updated_at)
		 VALUES (?, ?, ?, 'library', '{}', 1, 'active', ?, ?, ?, ?)`,
		toolID, cat.Name, cat.Description, cat.Slug, cat.Version, now, now,
	)
	if err != nil {
		os.RemoveAll(toolDir)
		return "", fmt.Errorf("insert tool record: %w", err)
	}

	logger.Success("Installed library tool: %s (%s)", cat.Name, toolID)
	return toolID, nil
}

func IsInstalled(db *database.DB, slug string) (bool, string) {
	var id string
	err := db.QueryRow(
		"SELECT id FROM tools WHERE library_slug = ? AND deleted_at IS NULL LIMIT 1",
		slug,
	).Scan(&id)
	if err != nil {
		return false, ""
	}
	return true, id
}

func MarkInstalled(tools []CatalogTool, db *database.DB) []CatalogTool {
	result := make([]CatalogTool, len(tools))
	copy(result, tools)
	for i := range result {
		installed, _ := IsInstalled(db, result[i].Slug)
		result[i].Installed = installed
	}
	return result
}
