package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/memory"
)

// Identity file names
const (
	FileIdentity  = "IDENTITY.md" // deprecated, kept for migration
	FileSoul      = "SOUL.md"
	FileUser      = "USER.md"
	FileAgents    = "AGENTS.md"
	FileTools     = "TOOLS.md" // deprecated, kept for migration
	FileHeartbeat = "HEARTBEAT.md"
	FileBoot      = "BOOT.md"
	FileMemory    = "memory/memory.md"
)

// Gateway-specific files
const (
	FileBootstrap = "BOOTSTRAP.md"
)

var identityFiles = []string{
	FileSoul, FileUser, FileAgents,
	FileHeartbeat, FileBoot,
}

// MemoryFile represents a file in the agent's memory directory.
type MemoryFile struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	UpdatedAt string `json:"updated_at"`
}

// Default templates for identity files
var defaultTemplates = map[string]string{
	FileSoul: `# %s

**Emoji**:
**Vibe**:

---

%s`,
	FileUser: `# User Preferences

(The human hasn't told you anything about themselves yet. Update this file as you learn.)
`,
	FileAgents: `# Operating Runbook

## Session Rules
- Be concise and helpful
- Ask clarifying questions when requirements are ambiguous
- Update memory after significant interactions

## Response Style
- Match the user's energy and formality level
- Use markdown formatting when it aids readability

## Memory Management
- After significant conversations, update memory/memory.md with brief bullet-point notes
- Format: ` + "`- [topic]: key takeaway`" + `
- Keep notes concise and factual
- Review your memory at session start
`,
	FileHeartbeat: ``,
	FileBoot: `# Startup

When starting a new session:
1. Read your memory files to recall context
2. Greet the user appropriately
3. Check if there are pending tasks from previous sessions
`,
}

// Default gateway SOUL.md template
var defaultGatewaySoul = `# Soul

You are Pounce, the OpenPaw Gateway.

## Personality
- Friendly, resourceful, and concise
- You guide users to the right agents or build things for them
- Warm but not wordy

## Speaking Style
- Use markdown for clarity
- Always suggest concrete next steps
- Reference agents by name when recommending them
`

// Default gateway USER.md template
var defaultGatewayUser = `# User

(Nothing learned about the user yet. This file updates as conversations happen.)
`

// Default gateway HEARTBEAT.md template
var defaultGatewayHeartbeat = ``

// Default gateway BOOTSTRAP.md template
var defaultGatewayBootstrap = `# Bootstrap

First-time setup mode. Gather information from the user to personalize the gateway.
`

// AgentDir computes the agent directory path.
func AgentDir(dataDir, slug string) string {
	return filepath.Join(dataDir, "agents", slug)
}

// InitAgentDir creates the agent directory with default identity files.
func InitAgentDir(dataDir, slug, name, soulContent string) error {
	dir := AgentDir(dataDir, slug)

	if err := os.MkdirAll(filepath.Join(dir, "memory"), 0755); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}

	for _, fname := range identityFiles {
		path := filepath.Join(dir, fname)
		if _, err := os.Stat(path); err == nil {
			continue // don't overwrite existing
		}

		tmpl := defaultTemplates[fname]
		var content string
		switch fname {
		case FileSoul:
			if soulContent != "" {
				content = fmt.Sprintf(tmpl, name, soulContent)
			} else {
				content = fmt.Sprintf(tmpl, name, fmt.Sprintf("You are %s, a helpful AI assistant.", name))
			}
		default:
			content = tmpl
		}

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", fname, err)
		}
	}

	// Create empty memory file
	memPath := filepath.Join(dir, FileMemory)
	if _, err := os.Stat(memPath); os.IsNotExist(err) {
		os.WriteFile(memPath, []byte("# Memory\n\n"), 0644)
	}

	return nil
}

// ReadAllFiles reads all identity files for an agent into a map.
func ReadAllFiles(dataDir, slug string) (map[string]string, error) {
	dir := AgentDir(dataDir, slug)
	result := make(map[string]string)

	for _, fname := range identityFiles {
		content, err := os.ReadFile(filepath.Join(dir, fname))
		if err != nil {
			if os.IsNotExist(err) {
				result[fname] = ""
				continue
			}
			return nil, fmt.Errorf("read %s: %w", fname, err)
		}
		result[fname] = string(content)
	}

	// Read memory file
	memContent, err := os.ReadFile(filepath.Join(dir, FileMemory))
	if err == nil {
		result[FileMemory] = string(memContent)
	} else {
		result[FileMemory] = ""
	}

	return result, nil
}

// ReadFile reads a single identity file.
func ReadIdentityFile(dataDir, slug, filename string) (string, error) {
	if !isAllowedFile(filename) {
		return "", fmt.Errorf("file not allowed: %s", filename)
	}
	path := filepath.Join(AgentDir(dataDir, slug), filename)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// WriteIdentityFile writes a single identity file with path validation.
func WriteIdentityFile(dataDir, slug, filename, content string) error {
	if !isAllowedFile(filename) {
		return fmt.Errorf("file not allowed: %s", filename)
	}

	// Prevent path traversal
	if strings.Contains(filename, "..") {
		return fmt.Errorf("path traversal not allowed")
	}

	path := filepath.Join(AgentDir(dataDir, slug), filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// isAllowedFile checks if a filename is in the allowed set.
func isAllowedFile(filename string) bool {
	// Identity files
	for _, f := range identityFiles {
		if filename == f {
			return true
		}
	}
	// Memory files
	if filename == FileMemory {
		return true
	}
	if strings.HasPrefix(filename, "memory/") && strings.HasSuffix(filename, ".md") {
		return true
	}
	// Skill files
	if strings.HasPrefix(filename, "skills/") && strings.HasSuffix(filename, "/SKILL.md") {
		return true
	}
	return false
}

// AssembleSystemPrompt reads identity files and builds the system prompt.
func AssembleSystemPrompt(dataDir, slug string) (string, error) {
	dir := AgentDir(dataDir, slug)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", fmt.Errorf("agent directory not found: %s", slug)
	}

	type section struct {
		label    string
		filename string
	}

	sections := []section{
		{"SOUL", FileSoul},
		{"OPERATING PROCEDURES", FileAgents},
		{"USER PREFERENCES", FileUser},
	}

	var parts []string

	for _, s := range sections {
		content, err := os.ReadFile(filepath.Join(dir, s.filename))
		if err != nil || strings.TrimSpace(string(content)) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("## %s\n\n%s", s.label, strings.TrimSpace(string(content))))
	}

	// NOTE: Long-term memory is now injected by the memory system (internal/memory/)
	// via agent_manager.go. The memory DB replaces memory/memory.md and daily logs.

	// Skills — progressive disclosure: show only name + description
	skillsDir := filepath.Join(dir, "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil {
		var skillLines []string
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillPath := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
			skillContent, err := os.ReadFile(skillPath)
			if err != nil || strings.TrimSpace(string(skillContent)) == "" {
				continue
			}
			skill := BuildSkillFromFile(entry.Name(), string(skillContent))
			desc := skill.Description
			if desc == "" {
				desc = "No description"
			}
			skillLines = append(skillLines, fmt.Sprintf("- **%s**: %s", skill.Name, desc))
		}
		if len(skillLines) > 0 {
			parts = append(parts, fmt.Sprintf("## AVAILABLE SKILLS\n\n%s\n\nTo use a skill, read its full SKILL.md from: %s/<skill-name>/SKILL.md",
				strings.Join(skillLines, "\n"), skillsDir))
		}
	}

	// Boot instructions (last = freshest context)
	bootContent, err := os.ReadFile(filepath.Join(dir, FileBoot))
	if err == nil && strings.TrimSpace(string(bootContent)) != "" {
		parts = append(parts, fmt.Sprintf("## BOOT\n\n%s", strings.TrimSpace(string(bootContent))))
	}

	// Agent self-editing instructions
	agentDir := AgentDir(dataDir, slug)
	parts = append(parts, fmt.Sprintf(`## WORKSPACE

Your identity files are at: %s
You can read and update your own files using the Read, Write, and Edit tools.

Key files you can modify:
- SOUL.md, USER.md, AGENTS.md, BOOT.md — identity and behavior
- skills/*/SKILL.md — your skill definitions

Your memories are stored in a database. Use the memory_* tools to save, search, list, update, and forget memories.
You CANNOT modify files outside your workspace directory.`, agentDir))

	if len(parts) == 0 {
		return "", fmt.Errorf("no identity files found for agent: %s", slug)
	}

	return strings.Join(parts, "\n\n---\n\n"), nil
}

// ListMemoryFiles lists files in the agent's memory directory.
func ListMemoryFiles(dataDir, slug string) ([]MemoryFile, error) {
	memDir := filepath.Join(AgentDir(dataDir, slug), "memory")
	entries, err := os.ReadDir(memDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []MemoryFile{}, nil
		}
		return nil, err
	}

	var files []MemoryFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, MemoryFile{
			Name:      entry.Name(),
			Size:      info.Size(),
			UpdatedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name > files[j].Name // newest first
	})

	return files, nil
}

// --- Gateway Identity File System ---

// GatewayDirPath returns the gateway identity directory path.
func GatewayDirPath(dataDir string) string {
	return filepath.Join(dataDir, "gateway")
}

// InitGatewayDir creates the gateway directory with default files.
// Creates BOOTSTRAP.md only if SOUL.md doesn't exist (fresh install).
func InitGatewayDir(dataDir string) error {
	dir := GatewayDirPath(dataDir)

	if err := os.MkdirAll(filepath.Join(dir, "memory"), 0755); err != nil {
		return fmt.Errorf("create gateway dir: %w", err)
	}

	soulPath := filepath.Join(dir, FileSoul)
	isFresh := false
	if _, err := os.Stat(soulPath); os.IsNotExist(err) {
		isFresh = true
	}

	// Write default files (skip existing)
	defaults := map[string]string{
		FileSoul:      defaultGatewaySoul,
		FileUser:      defaultGatewayUser,
		FileHeartbeat: defaultGatewayHeartbeat,
	}
	for fname, content := range defaults {
		path := filepath.Join(dir, fname)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write gateway %s: %w", fname, err)
		}
	}

	// Create empty memory file
	memPath := filepath.Join(dir, FileMemory)
	if _, err := os.Stat(memPath); os.IsNotExist(err) {
		os.WriteFile(memPath, []byte("# Memory\n\n"), 0644)
	}

	// Only create BOOTSTRAP.md on fresh install
	if isFresh {
		bootstrapPath := filepath.Join(dir, FileBootstrap)
		if _, err := os.Stat(bootstrapPath); os.IsNotExist(err) {
			os.WriteFile(bootstrapPath, []byte(defaultGatewayBootstrap), 0644)
		}
		logger.Info("Gateway: fresh install detected, bootstrap mode enabled")
	}

	return nil
}

// ReadGatewayFiles reads all gateway identity files into a map.
func ReadGatewayFiles(dataDir string) (map[string]string, error) {
	dir := GatewayDirPath(dataDir)
	result := make(map[string]string)

	gatewayFiles := []string{FileSoul, FileUser, FileHeartbeat}
	for _, fname := range gatewayFiles {
		content, err := os.ReadFile(filepath.Join(dir, fname))
		if err != nil {
			if os.IsNotExist(err) {
				result[fname] = ""
				continue
			}
			return nil, fmt.Errorf("read gateway %s: %w", fname, err)
		}
		result[fname] = string(content)
	}

	// Read memory file
	memContent, err := os.ReadFile(filepath.Join(dir, FileMemory))
	if err == nil {
		result[FileMemory] = string(memContent)
	} else {
		result[FileMemory] = ""
	}

	return result, nil
}

// ReadGatewayFile reads a single gateway file with path validation.
func ReadGatewayFile(dataDir, filename string) (string, error) {
	if !isGatewayAllowedFile(filename) {
		return "", fmt.Errorf("file not allowed: %s", filename)
	}
	if strings.Contains(filename, "..") {
		return "", fmt.Errorf("path traversal not allowed")
	}
	path := filepath.Join(GatewayDirPath(dataDir), filename)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// WriteGatewayFile writes a single gateway file with path validation.
func WriteGatewayFile(dataDir, filename, content string) error {
	if !isGatewayAllowedFile(filename) {
		return fmt.Errorf("file not allowed: %s", filename)
	}
	if strings.Contains(filename, "..") {
		return fmt.Errorf("path traversal not allowed")
	}
	path := filepath.Join(GatewayDirPath(dataDir), filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// ListGatewayMemoryFiles lists files in the gateway's memory directory.
func ListGatewayMemoryFiles(dataDir string) ([]MemoryFile, error) {
	memDir := filepath.Join(GatewayDirPath(dataDir), "memory")
	entries, err := os.ReadDir(memDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []MemoryFile{}, nil
		}
		return nil, err
	}

	var files []MemoryFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, MemoryFile{
			Name:      entry.Name(),
			Size:      info.Size(),
			UpdatedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name > files[j].Name
	})

	return files, nil
}

// AssembleGatewayContext reads gateway identity files and returns a formatted context string.
func AssembleGatewayContext(dataDir string, memMgr *memory.Manager) string {
	dir := GatewayDirPath(dataDir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return ""
	}

	var parts []string

	// SOUL
	if content, err := os.ReadFile(filepath.Join(dir, FileSoul)); err == nil && strings.TrimSpace(string(content)) != "" {
		parts = append(parts, "## YOUR IDENTITY\n\n"+strings.TrimSpace(string(content)))
	}

	// USER
	if content, err := os.ReadFile(filepath.Join(dir, FileUser)); err == nil && strings.TrimSpace(string(content)) != "" {
		parts = append(parts, "## WHAT YOU KNOW ABOUT THE USER\n\n"+strings.TrimSpace(string(content)))
	}

	// Memory — use DB-backed memory if available, fall back to file
	if memMgr != nil {
		memMgr.EnsureMigrated("gateway")
		if memSection := memMgr.BuildMemoryPromptSection("gateway"); memSection != "" {
			parts = append(parts, memSection)
		}
	} else {
		if content, err := os.ReadFile(filepath.Join(dir, FileMemory)); err == nil && strings.TrimSpace(string(content)) != "" {
			parts = append(parts, "## YOUR MEMORY\n\n"+strings.TrimSpace(string(content)))
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// GatewayHasBootstrap checks if the gateway is in bootstrap (first-time setup) mode.
func GatewayHasBootstrap(dataDir string) bool {
	path := filepath.Join(GatewayDirPath(dataDir), FileBootstrap)
	_, err := os.Stat(path)
	return err == nil
}

// DeleteGatewayBootstrap removes the bootstrap file after onboarding is complete.
func DeleteGatewayBootstrap(dataDir string) error {
	path := filepath.Join(GatewayDirPath(dataDir), FileBootstrap)
	return os.Remove(path)
}

// isGatewayAllowedFile checks if a filename is allowed for gateway file operations.
func isGatewayAllowedFile(filename string) bool {
	allowed := map[string]bool{
		FileSoul:      true,
		FileUser:      true,
		FileHeartbeat: true,
	}
	if allowed[filename] {
		return true
	}
	if filename == FileMemory {
		return true
	}
	if strings.HasPrefix(filename, "memory/") && strings.HasSuffix(filename, ".md") {
		return true
	}
	return false
}

// SaveGatewayMemoryNote appends a timestamped note to the gateway's memory file.
func SaveGatewayMemoryNote(dataDir, note string) error {
	if strings.TrimSpace(note) == "" {
		return nil
	}
	memPath := filepath.Join(GatewayDirPath(dataDir), FileMemory)
	if err := os.MkdirAll(filepath.Dir(memPath), 0755); err != nil {
		return err
	}
	timestamp := time.Now().Format("2006-01-02 15:04")
	entry := fmt.Sprintf("- [%s] %s\n", timestamp, strings.TrimSpace(note))

	f, err := os.OpenFile(memPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(entry)
	return err
}

// MigrateIdentityToSoul merges IDENTITY.md content into SOUL.md and removes deprecated files.
func MigrateIdentityToSoul(dataDir string) error {
	agentsDir := filepath.Join(dataDir, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		agentDir := filepath.Join(agentsDir, entry.Name())

		// Merge IDENTITY.md into SOUL.md
		identityPath := filepath.Join(agentDir, FileIdentity)
		if identityContent, err := os.ReadFile(identityPath); err == nil && strings.TrimSpace(string(identityContent)) != "" {
			soulPath := filepath.Join(agentDir, FileSoul)
			soulContent, _ := os.ReadFile(soulPath)
			merged := strings.TrimSpace(string(identityContent)) + "\n\n---\n\n" + strings.TrimSpace(string(soulContent))
			os.WriteFile(soulPath, []byte(merged), 0644)
			os.Remove(identityPath)
			count++
		}

		// Remove TOOLS.md if it exists and is empty
		toolsPath := filepath.Join(agentDir, FileTools)
		if toolsContent, err := os.ReadFile(toolsPath); err == nil {
			if strings.TrimSpace(string(toolsContent)) == "" {
				os.Remove(toolsPath)
			}
		}
	}

	if count > 0 {
		logger.Success("Migrated IDENTITY.md to SOUL.md for %d agent(s)", count)
	}
	return nil
}

// MigrateExistingAgents initializes identity files for agents that haven't been migrated yet.
func MigrateExistingAgents(db *database.DB, dataDir string) error {
	rows, err := db.Query(
		"SELECT slug, name, system_prompt FROM agent_roles WHERE identity_initialized = 0",
	)
	if err != nil {
		return fmt.Errorf("query agents for migration: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var slug, name, systemPrompt string
		if err := rows.Scan(&slug, &name, &systemPrompt); err != nil {
			logger.Error("Failed to scan agent for migration: %v", err)
			continue
		}

		if err := InitAgentDir(dataDir, slug, name, systemPrompt); err != nil {
			logger.Error("Failed to init agent dir for %s: %v", slug, err)
			continue
		}

		_, err := db.Exec("UPDATE agent_roles SET identity_initialized = 1 WHERE slug = ?", slug)
		if err != nil {
			logger.Error("Failed to mark agent %s as initialized: %v", slug, err)
			continue
		}
		count++
	}

	if count > 0 {
		logger.Success("Migrated %d agent(s) to identity file system", count)
	}
	return nil
}
