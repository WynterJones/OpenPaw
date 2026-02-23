package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var skillNameRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// SkillMeta holds parsed YAML frontmatter fields from a SKILL.md file.
type SkillMeta struct {
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
	AllowedTools string `json:"allowed_tools,omitempty"`
	Folder       string `json:"folder,omitempty"`
}

// Skill represents a skill definition.
type Skill struct {
	Name         string `json:"name"`
	Content      string `json:"content,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Description  string `json:"description,omitempty"`
	AllowedTools string `json:"allowed_tools,omitempty"`
	Folder       string `json:"folder,omitempty"`
}

// globalSkillsDir returns the path to the global skills directory.
func globalSkillsDir(dataDir string) string {
	return filepath.Join(dataDir, "skills")
}

// agentSkillsDir returns the path to an agent's skills directory.
func agentSkillsDir(dataDir, agentSlug string) string {
	return filepath.Join(AgentDir(dataDir, agentSlug), "skills")
}

// ListGlobalSkills scans data/skills/*/SKILL.md and returns all global skills.
func ListGlobalSkills(dataDir string) ([]Skill, error) {
	dir := globalSkillsDir(dataDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create skills dir: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	skills := []Skill{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			continue
		}
		skills = append(skills, BuildSkillFromFile(entry.Name(), string(content)))
	}
	return skills, nil
}

// GetGlobalSkill reads a single global skill.
func GetGlobalSkill(dataDir, name string) (string, error) {
	if !IsValidSkillName(name) {
		return "", fmt.Errorf("invalid skill name: %s", name)
	}
	path := filepath.Join(globalSkillsDir(dataDir), name, "SKILL.md")
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read skill: %w", err)
	}
	return string(content), nil
}

// WriteGlobalSkill creates or updates a global skill.
func WriteGlobalSkill(dataDir, name, content string) error {
	if !IsValidSkillName(name) {
		return fmt.Errorf("invalid skill name: %s", name)
	}
	dir := filepath.Join(globalSkillsDir(dataDir), name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644)
}

// DeleteGlobalSkill removes a global skill.
func DeleteGlobalSkill(dataDir, name string) error {
	if !IsValidSkillName(name) {
		return fmt.Errorf("invalid skill name: %s", name)
	}
	dir := filepath.Join(globalSkillsDir(dataDir), name)
	return os.RemoveAll(dir)
}

// AddSkillToAgent copies a global skill into an agent's workspace.
func AddSkillToAgent(dataDir, skillName, agentSlug string) error {
	if !IsValidSkillName(skillName) {
		return fmt.Errorf("invalid skill name: %s", skillName)
	}

	// Read global skill
	content, err := GetGlobalSkill(dataDir, skillName)
	if err != nil {
		return fmt.Errorf("read global skill: %w", err)
	}

	// Write to agent's skills dir
	agentSkillDir := filepath.Join(agentSkillsDir(dataDir, agentSlug), skillName)
	if err := os.MkdirAll(agentSkillDir, 0755); err != nil {
		return fmt.Errorf("create agent skill dir: %w", err)
	}
	return os.WriteFile(filepath.Join(agentSkillDir, "SKILL.md"), []byte(content), 0644)
}

// RemoveSkillFromAgent removes a skill from an agent's workspace.
func RemoveSkillFromAgent(dataDir, skillName, agentSlug string) error {
	if !IsValidSkillName(skillName) {
		return fmt.Errorf("invalid skill name: %s", skillName)
	}
	dir := filepath.Join(agentSkillsDir(dataDir, agentSlug), skillName)
	return os.RemoveAll(dir)
}

// PublishAgentSkill copies an agent's skill to the global library.
func PublishAgentSkill(dataDir, skillName, agentSlug string) error {
	if !IsValidSkillName(skillName) {
		return fmt.Errorf("invalid skill name: %s", skillName)
	}

	agentSkillPath := filepath.Join(agentSkillsDir(dataDir, agentSlug), skillName, "SKILL.md")
	content, err := os.ReadFile(agentSkillPath)
	if err != nil {
		return fmt.Errorf("read agent skill: %w", err)
	}

	return WriteGlobalSkill(dataDir, skillName, string(content))
}

// ListAgentSkills lists skills installed in an agent's workspace.
func ListAgentSkills(dataDir, agentSlug string) ([]Skill, error) {
	dir := agentSkillsDir(dataDir, agentSlug)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Skill{}, nil
		}
		return nil, fmt.Errorf("read agent skills dir: %w", err)
	}

	skills := []Skill{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			continue
		}
		skills = append(skills, BuildSkillFromFile(entry.Name(), string(content)))
	}
	return skills, nil
}

// IsValidSkillName checks that a skill name is safe for use as a directory name.
func IsValidSkillName(name string) bool {
	if name == "" || strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	return skillNameRegex.MatchString(name)
}

// ParseFrontmatter splits a SKILL.md into YAML frontmatter metadata and body.
// Returns empty meta + full content if no frontmatter delimiters found.
func ParseFrontmatter(content string) (SkillMeta, string) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return SkillMeta{}, content
	}

	// Find closing ---
	rest := trimmed[3:]
	// Skip optional newline after opening ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	closeIdx := strings.Index(rest, "\n---")
	if closeIdx < 0 {
		return SkillMeta{}, content
	}

	yamlBlock := rest[:closeIdx]
	body := strings.TrimSpace(rest[closeIdx+4:]) // skip \n---

	meta := SkillMeta{}
	for _, line := range strings.Split(yamlBlock, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		val := strings.TrimSpace(line[colonIdx+1:])
		// Strip surrounding quotes
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		switch key {
		case "name":
			meta.Name = val
		case "description":
			meta.Description = val
		case "allowed_tools", "allowedTools":
			meta.AllowedTools = val
		case "folder":
			meta.Folder = val
		}
	}

	return meta, body
}

// BuildSkillFromFile parses a SKILL.md file and returns a populated Skill struct.
// Falls back to firstLine() for legacy skills without frontmatter.
func BuildSkillFromFile(name, content string) Skill {
	meta, body := ParseFrontmatter(content)
	desc := meta.Description
	if desc == "" {
		desc = firstLine(body)
	}
	return Skill{
		Name:         name,
		Content:      content,
		Summary:      firstLine(body),
		Description:  desc,
		AllowedTools: meta.AllowedTools,
		Folder:       meta.Folder,
	}
}

// BuildFrontmatter generates a valid SKILL.md with YAML frontmatter.
func BuildFrontmatter(name, description, body string) string {
	return BuildFrontmatterFromMeta(SkillMeta{Name: name, Description: description}, body)
}

// BuildFrontmatterFromMeta generates SKILL.md with YAML frontmatter from a SkillMeta.
func BuildFrontmatterFromMeta(meta SkillMeta, body string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	if meta.Name != "" {
		sb.WriteString(fmt.Sprintf("name: %s\n", meta.Name))
	}
	if meta.Description != "" {
		sb.WriteString(fmt.Sprintf("description: %s\n", meta.Description))
	}
	if meta.AllowedTools != "" {
		sb.WriteString(fmt.Sprintf("allowed_tools: %s\n", meta.AllowedTools))
	}
	if meta.Folder != "" {
		sb.WriteString(fmt.Sprintf("folder: %s\n", meta.Folder))
	}
	sb.WriteString("---\n\n")
	sb.WriteString(body)
	return sb.String()
}

// firstLine returns the first non-empty line of text, stripped of markdown heading markers.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "# ")
		if line != "" {
			return line
		}
	}
	return ""
}
