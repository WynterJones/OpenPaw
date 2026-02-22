package agents

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"
)

//go:embed template/*
var templateFS embed.FS

//go:embed dashboard_template/*
var dashboardTemplateFS embed.FS

// TemplateData holds the values injected into .tmpl files during scaffolding.
type TemplateData struct {
	ToolID      string
	Name        string
	Description string
	SlugName    string
	BinaryName  string
	CreatedAt   string
}

// NewTemplateData builds TemplateData from work order fields.
func NewTemplateData(toolID, name, description string) TemplateData {
	slug := slugify(name)
	return TemplateData{
		ToolID:      toolID,
		Name:        name,
		Description: description,
		SlugName:    slug,
		BinaryName:  slug,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
}

// ScaffoldToolDir copies the embedded template into targetDir.
// Files ending in .tmpl are rendered with text/template (suffix stripped).
// All other files are copied verbatim.
func ScaffoldToolDir(targetDir string, data TemplateData) error {
	funcMap := template.FuncMap{
		"jsonEscape": jsonEscape,
	}

	return fs.WalkDir(templateFS, "template", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Strip the leading "template/" prefix to get the relative path
		rel, err := filepath.Rel("template", path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		destPath := filepath.Join(targetDir, rel)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		content, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded file %s: %w", path, err)
		}

		if strings.HasSuffix(rel, ".tmpl") {
			// Render template and strip .tmpl suffix
			destPath = strings.TrimSuffix(destPath, ".tmpl")

			tmpl, err := template.New(rel).Funcs(funcMap).Parse(string(content))
			if err != nil {
				return fmt.Errorf("parse template %s: %w", path, err)
			}

			f, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("create %s: %w", destPath, err)
			}
			defer f.Close()

			if err := tmpl.Execute(f, data); err != nil {
				return fmt.Errorf("execute template %s: %w", path, err)
			}
		} else {
			// Copy verbatim
			if err := os.WriteFile(destPath, content, 0644); err != nil {
				return fmt.Errorf("write %s: %w", destPath, err)
			}
		}

		return nil
	})
}

// DashboardTemplateData holds the values injected into dashboard .tmpl files.
type DashboardTemplateData struct {
	DashboardID string
	Name        string
	Description string
}

// ScaffoldDashboardDir copies the embedded dashboard template into targetDir.
func ScaffoldDashboardDir(targetDir string, data DashboardTemplateData) error {
	return fs.WalkDir(dashboardTemplateFS, "dashboard_template", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel("dashboard_template", path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		destPath := filepath.Join(targetDir, rel)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		content, err := dashboardTemplateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded file %s: %w", path, err)
		}

		if strings.HasSuffix(rel, ".tmpl") {
			destPath = strings.TrimSuffix(destPath, ".tmpl")

			tmpl, err := template.New(rel).Parse(string(content))
			if err != nil {
				return fmt.Errorf("parse template %s: %w", path, err)
			}

			f, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("create %s: %w", destPath, err)
			}
			defer f.Close()

			if err := tmpl.Execute(f, data); err != nil {
				return fmt.Errorf("execute template %s: %w", path, err)
			}
		} else {
			if err := os.WriteFile(destPath, content, 0644); err != nil {
				return fmt.Errorf("write %s: %w", destPath, err)
			}
		}

		return nil
	})
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a name like "Weather API Tool" to "weather-api-tool".
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// jsonEscape escapes a string for safe embedding in JSON values.
func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
