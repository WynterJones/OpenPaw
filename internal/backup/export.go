package backup

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openpaw/openpaw/internal/agents"
	"github.com/openpaw/openpaw/internal/database"

	_ "github.com/mattn/go-sqlite3"
)

type Manifest struct {
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Files     []string  `json:"files"`
	Stats     Stats     `json:"stats"`
}

type Stats struct {
	AgentRoles      int `json:"agent_roles"`
	Tools           int `json:"tools"`
	Dashboards      int `json:"dashboards"`
	Schedules       int `json:"schedules"`
	Context         int `json:"context_files"`
	Skills          int `json:"skills"`
	Memories        int `json:"memories"`
	Users           int `json:"users"`
	AgentToolAccess int `json:"agent_tool_access"`
	Secrets         int `json:"secrets"`
	Avatars         int `json:"avatars"`
	Backgrounds     int `json:"backgrounds"`
	ChatThreads     int `json:"chat_threads"`
	ChatMessages    int `json:"chat_messages"`
	ChatAttachments int `json:"chat_attachments"`
	ThreadMembers   int `json:"thread_members"`
	Notifications   int `json:"notifications"`
	SystemStats     int `json:"system_stats"`
	BrowserSessions int `json:"browser_sessions"`
	AgentSkills     int `json:"agent_skills"`
	Workbenches     int `json:"workbenches"`
	Projects        int `json:"projects"`
	AgentTasks      int `json:"agent_tasks"`
	TodoLists       int `json:"todo_lists"`
	TodoItems       int `json:"todo_items"`
}

func exportData(db *database.DB, dataDir, destDir string) (int, error) {
	var files []string
	var stats Stats

	// Settings
	if err := os.MkdirAll(filepath.Join(destDir, "settings"), 0755); err != nil {
		return 0, fmt.Errorf("create settings dir: %w", err)
	}

	if n, err := exportSettings(db, destDir); err == nil {
		files = append(files, n...)
	}

	// Users
	if n, err := exportUsers(db, destDir); err == nil {
		files = append(files, n...)
		stats.Users = len(n)
	}

	// Agent roles
	if n, err := exportAgentRoles(db, dataDir, destDir); err == nil {
		files = append(files, n...)
		stats.AgentRoles = countPrefix(n, "agent_roles/")
	}

	// Agent tool access
	if n, err := exportAgentToolAccess(db, destDir); err == nil {
		files = append(files, n...)
		stats.AgentToolAccess = len(n)
	}

	// Secrets (metadata only)
	if n, err := exportSecretsMeta(db, destDir); err == nil {
		files = append(files, n...)
		stats.Secrets = len(n)
	}

	// Context
	if n, err := exportContext(db, destDir); err == nil {
		files = append(files, n...)
		stats.Context = countPrefix(n, "context/")
	}

	// Dashboards
	if n, err := exportDashboards(db, destDir); err == nil {
		files = append(files, n...)
		stats.Dashboards = countPrefix(n, "dashboards/")
	}

	// Tools
	if n, err := exportTools(db, destDir); err == nil {
		files = append(files, n...)
		stats.Tools = countPrefix(n, "tools/")
	}

	// Schedules
	if n, err := exportSchedules(db, destDir); err == nil {
		files = append(files, n...)
		stats.Schedules = len(n)
	}

	// Skills (global)
	if n, err := exportSkills(dataDir, destDir); err == nil {
		files = append(files, n...)
		stats.Skills = countPrefix(n, "skills/")
	}

	// Agent skills (per-agent)
	if n, err := exportAgentSkills(dataDir, destDir); err == nil {
		files = append(files, n...)
		stats.AgentSkills = countPrefix(n, "agent_skills/")
	}

	// Agent memories (from separate SQLite databases)
	if n, err := exportMemories(dataDir, destDir); err == nil {
		files = append(files, n...)
		stats.Memories = countPrefix(n, "memories/")
	}

	// Context file blobs (actual files on disk)
	if n, err := exportContextBlobs(dataDir, destDir); err == nil {
		files = append(files, n...)
	}

	// Chat history
	if n, err := exportChatHistory(db, dataDir, destDir); err == nil {
		files = append(files, n...)
		stats.ChatThreads = countPrefix(n, "chat/threads")
		stats.ThreadMembers = countPrefix(n, "chat/thread_members")
		stats.ChatMessages = countPrefix(n, "chat/messages/")
		stats.ChatAttachments = countPrefix(n, "chat/attachments/")
	}

	// Notifications
	if n, err := exportNotifications(db, destDir); err == nil {
		files = append(files, n...)
		stats.Notifications = len(n)
	}

	// System stats
	if n, err := exportSystemStats(db, destDir); err == nil {
		files = append(files, n...)
		stats.SystemStats = len(n)
	}

	// Browser sessions (config only)
	if n, err := exportBrowserSessions(db, destDir); err == nil {
		files = append(files, n...)
		stats.BrowserSessions = len(n)
	}

	// Workbenches
	if n, err := exportWorkbenches(db, destDir); err == nil {
		files = append(files, n...)
		stats.Workbenches = len(n)
	}

	// Projects
	if n, err := exportProjects(db, destDir); err == nil {
		files = append(files, n...)
		stats.Projects = len(n)
	}

	// Agent tasks
	if n, err := exportAgentTasks(db, destDir); err == nil {
		files = append(files, n...)
		stats.AgentTasks = len(n)
	}

	// Todo lists
	if n, lists, items, err := exportTodoLists(db, destDir); err == nil {
		files = append(files, n...)
		stats.TodoLists = lists
		stats.TodoItems = items
	}

	// Avatars
	if n, err := exportAvatars(dataDir, destDir); err == nil {
		files = append(files, n...)
		stats.Avatars = countPrefix(n, "avatars/")
	}

	// Backgrounds
	if n, err := exportBackgrounds(dataDir, destDir); err == nil {
		files = append(files, n...)
		stats.Backgrounds = countPrefix(n, "backgrounds/")
	}

	// Write manifest
	manifest := Manifest{
		Version:   "1.1",
		Timestamp: time.Now().UTC(),
		Files:     files,
		Stats:     stats,
	}
	if err := writeJSONFile(filepath.Join(destDir, "manifest.json"), manifest); err != nil {
		return 0, fmt.Errorf("write manifest: %w", err)
	}

	return len(files) + 1, nil
}

func exportSettings(db *database.DB, destDir string) ([]string, error) {
	var files []string

	// Sensitive keys to exclude
	sensitive := map[string]bool{
		"jwt_secret": true, "encryption_key": true,
		"openrouter_api_key": true, "backup_auth_token": true,
	}

	rows, err := db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	general := map[string]string{}
	design := map[string]string{}
	models := map[string]string{}

	for rows.Next() {
		var key, val string
		if rows.Scan(&key, &val) != nil {
			continue
		}
		if sensitive[key] {
			continue
		}
		switch {
		case strings.HasPrefix(key, "design_"):
			design[key] = val
		case strings.HasPrefix(key, "gateway_model") || strings.HasPrefix(key, "builder_model") ||
			key == "max_turns" || key == "agent_timeout_min":
			models[key] = val
		default:
			general[key] = val
		}
	}

	if len(general) > 0 {
		path := "settings/general.json"
		if err := writeJSONFile(filepath.Join(destDir, path), general); err == nil {
			files = append(files, path)
		}
	}
	if len(design) > 0 {
		path := "settings/design.json"
		if err := writeJSONFile(filepath.Join(destDir, path), design); err == nil {
			files = append(files, path)
		}
	}
	if len(models) > 0 {
		path := "settings/models.json"
		if err := writeJSONFile(filepath.Join(destDir, path), models); err == nil {
			files = append(files, path)
		}
	}

	return files, nil
}

func exportAgentRoles(db *database.DB, dataDir, destDir string) ([]string, error) {
	var files []string

	if err := os.MkdirAll(filepath.Join(destDir, "agent_roles"), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(destDir, "agent_files"), 0755); err != nil {
		return nil, err
	}

	rows, err := db.Query(
		`SELECT id, slug, name, description, system_prompt, model, avatar_path, avatar_description, enabled, sort_order, is_preset, heartbeat_enabled, identity_initialized, library_slug, library_version, folder, created_at, updated_at
		 FROM agent_roles ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id, slug, name, desc, prompt, model, avatar, avatarDesc string
			librarySlug, libraryVersion, folder                     string
			enabled, sortOrder, isPreset, hbEnabled, identityInit   int
			createdAt, updatedAt                                    time.Time
		)
		if rows.Scan(&id, &slug, &name, &desc, &prompt, &model, &avatar, &avatarDesc, &enabled, &sortOrder, &isPreset, &hbEnabled, &identityInit, &librarySlug, &libraryVersion, &folder, &createdAt, &updatedAt) != nil {
			continue
		}

		role := map[string]interface{}{
			"id": id, "slug": slug, "name": name, "description": desc,
			"system_prompt": prompt, "model": model, "avatar_path": avatar,
			"avatar_description": avatarDesc,
			"enabled": enabled == 1, "sort_order": sortOrder,
			"is_preset": isPreset == 1, "heartbeat_enabled": hbEnabled == 1,
			"identity_initialized": identityInit == 1,
			"library_slug": librarySlug, "library_version": libraryVersion,
			"folder": folder,
			"created_at": createdAt, "updated_at": updatedAt,
		}

		path := fmt.Sprintf("agent_roles/%s.json", slug)
		if err := writeJSONFile(filepath.Join(destDir, path), role); err == nil {
			files = append(files, path)
		}

		// Export identity files from disk
		agentDir := agents.AgentDir(dataDir, slug)
		for _, fname := range []string{agents.FileSoul, agents.FileUser, agents.FileRunbook, agents.FileHeartbeat, agents.FileBoot} {
			content, err := os.ReadFile(filepath.Join(agentDir, fname))
			if err != nil || len(strings.TrimSpace(string(content))) == 0 {
				continue
			}
			agentFilesDir := filepath.Join(destDir, "agent_files", slug)
			os.MkdirAll(agentFilesDir, 0755)
			fpath := fmt.Sprintf("agent_files/%s/%s", slug, fname)
			if err := os.WriteFile(filepath.Join(destDir, fpath), content, 0644); err == nil {
				files = append(files, fpath)
			}
		}
	}

	return files, nil
}

func exportContext(db *database.DB, destDir string) ([]string, error) {
	var files []string

	if err := os.MkdirAll(filepath.Join(destDir, "context"), 0755); err != nil {
		return nil, err
	}

	// Folders
	rows, err := db.Query("SELECT id, parent_id, name, sort_order, created_at, updated_at FROM context_folders ORDER BY sort_order")
	if err == nil {
		defer rows.Close()
		var folders []map[string]interface{}
		for rows.Next() {
			var id, name string
			var parentID *string
			var sortOrder int
			var createdAt, updatedAt time.Time
			if rows.Scan(&id, &parentID, &name, &sortOrder, &createdAt, &updatedAt) != nil {
				continue
			}
			f := map[string]interface{}{
				"id": id, "parent_id": parentID, "name": name,
				"sort_order": sortOrder, "created_at": createdAt, "updated_at": updatedAt,
			}
			folders = append(folders, f)
		}
		rows.Close()
		if len(folders) > 0 {
			path := "context/folders.json"
			if err := writeJSONFile(filepath.Join(destDir, path), folders); err == nil {
				files = append(files, path)
			}
		}
	}

	// Files (with content from context_store if text-based)
	fRows, err := db.Query("SELECT id, folder_id, name, filename, mime_type, size_bytes, is_about_you, created_at, updated_at FROM context_files ORDER BY name")
	if err == nil {
		defer fRows.Close()
		var contextFiles []map[string]interface{}
		for fRows.Next() {
			var id, name, filename, mimeType string
			var folderID *string
			var sizeBytes, isAboutYou int
			var createdAt, updatedAt time.Time
			if fRows.Scan(&id, &folderID, &name, &filename, &mimeType, &sizeBytes, &isAboutYou, &createdAt, &updatedAt) != nil {
				continue
			}
			f := map[string]interface{}{
				"id": id, "folder_id": folderID, "name": name, "filename": filename,
				"mime_type": mimeType, "size_bytes": sizeBytes,
				"is_about_you": isAboutYou == 1,
				"created_at": createdAt, "updated_at": updatedAt,
			}
			contextFiles = append(contextFiles, f)
		}
		fRows.Close()
		if len(contextFiles) > 0 {
			path := "context/files.json"
			if err := writeJSONFile(filepath.Join(destDir, path), contextFiles); err == nil {
				files = append(files, path)
			}
		}
	}

	return files, nil
}

func exportDashboards(db *database.DB, destDir string) ([]string, error) {
	var files []string

	if err := os.MkdirAll(filepath.Join(destDir, "dashboards"), 0755); err != nil {
		return nil, err
	}

	rows, err := db.Query("SELECT id, name, description, dashboard_type, owner_agent_slug, bg_image, layout, widgets, created_at, updated_at FROM dashboards")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, desc, dashType, ownerSlug, bgImage, layout, widgets string
		var createdAt, updatedAt time.Time
		if rows.Scan(&id, &name, &desc, &dashType, &ownerSlug, &bgImage, &layout, &widgets, &createdAt, &updatedAt) != nil {
			continue
		}
		d := map[string]interface{}{
			"id": id, "name": name, "description": desc,
			"dashboard_type": dashType, "owner_agent_slug": ownerSlug,
			"bg_image": bgImage, "layout": json.RawMessage(layout),
			"widgets": json.RawMessage(widgets),
			"created_at": createdAt, "updated_at": updatedAt,
		}
		path := fmt.Sprintf("dashboards/%s.json", id)
		if err := writeJSONFile(filepath.Join(destDir, path), d); err == nil {
			files = append(files, path)
		}
	}

	return files, nil
}

func exportTools(db *database.DB, destDir string) ([]string, error) {
	var files []string

	if err := os.MkdirAll(filepath.Join(destDir, "tools"), 0755); err != nil {
		return nil, err
	}

	rows, err := db.Query("SELECT id, name, description, type, config, enabled, status, capabilities, owner_agent_slug, library_slug, library_version, source_hash, binary_hash, folder, created_at, updated_at FROM tools WHERE deleted_at IS NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, desc, typ, config, status string
		var capabilities, ownerSlug, libSlug, libVer, srcHash, binHash, folder string
		var enabled int
		var createdAt, updatedAt time.Time
		if rows.Scan(&id, &name, &desc, &typ, &config, &enabled, &status, &capabilities, &ownerSlug, &libSlug, &libVer, &srcHash, &binHash, &folder, &createdAt, &updatedAt) != nil {
			continue
		}
		t := map[string]interface{}{
			"id": id, "name": name, "description": desc, "type": typ,
			"config": json.RawMessage(config), "enabled": enabled == 1,
			"status": status, "capabilities": capabilities,
			"owner_agent_slug": ownerSlug, "library_slug": libSlug,
			"library_version": libVer, "source_hash": srcHash,
			"binary_hash": binHash, "folder": folder,
			"created_at": createdAt, "updated_at": updatedAt,
		}
		path := fmt.Sprintf("tools/%s.json", id)
		if err := writeJSONFile(filepath.Join(destDir, path), t); err == nil {
			files = append(files, path)
		}
	}

	return files, nil
}

func exportSchedules(db *database.DB, destDir string) ([]string, error) {
	rows, err := db.Query("SELECT id, name, description, cron_expr, tool_id, action, payload, enabled, type, agent_role_slug, prompt_content, thread_id, dashboard_id, widget_id, browser_session_id, browser_instructions, created_at, updated_at FROM schedules")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []map[string]interface{}
	for rows.Next() {
		var id, name, desc, cron, toolID, action, payload string
		var typ, agentSlug, promptContent, threadID string
		var dashboardID, widgetID, browserSessionID, browserInstructions string
		var enabled int
		var createdAt, updatedAt time.Time
		if rows.Scan(&id, &name, &desc, &cron, &toolID, &action, &payload, &enabled, &typ, &agentSlug, &promptContent, &threadID, &dashboardID, &widgetID, &browserSessionID, &browserInstructions, &createdAt, &updatedAt) != nil {
			continue
		}
		s := map[string]interface{}{
			"id": id, "name": name, "description": desc, "cron_expr": cron,
			"tool_id": toolID, "action": action, "payload": json.RawMessage(payload),
			"enabled": enabled == 1, "type": typ, "agent_role_slug": agentSlug,
			"prompt_content": promptContent, "thread_id": threadID,
			"dashboard_id": dashboardID, "widget_id": widgetID,
			"browser_session_id": browserSessionID,
			"browser_instructions": browserInstructions,
			"created_at": createdAt, "updated_at": updatedAt,
		}
		schedules = append(schedules, s)
	}

	if len(schedules) == 0 {
		return nil, nil
	}

	path := "schedules.json"
	if err := writeJSONFile(filepath.Join(destDir, path), schedules); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func exportSkills(dataDir, destDir string) ([]string, error) {
	var files []string

	skillsDir := filepath.Join(dataDir, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, nil // skills dir may not exist
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		content, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}
		destSkillDir := filepath.Join(destDir, "skills", entry.Name())
		os.MkdirAll(destSkillDir, 0755)
		path := fmt.Sprintf("skills/%s/SKILL.md", entry.Name())
		if err := os.WriteFile(filepath.Join(destDir, path), content, 0644); err == nil {
			files = append(files, path)
		}
	}

	return files, nil
}

func exportMemories(dataDir, destDir string) ([]string, error) {
	var files []string

	if err := os.MkdirAll(filepath.Join(destDir, "memories"), 0755); err != nil {
		return nil, err
	}

	// Find all memory.db files under agents/ and gateway/
	searchDirs := []struct {
		dir  string
		slug string
	}{
		{filepath.Join(dataDir, "gateway"), "gateway"},
	}

	// Discover agent slugs
	agentsDir := filepath.Join(dataDir, "agents")
	entries, _ := os.ReadDir(agentsDir)
	for _, e := range entries {
		if e.IsDir() {
			searchDirs = append(searchDirs, struct {
				dir  string
				slug string
			}{filepath.Join(agentsDir, e.Name()), e.Name()})
		}
	}

	for _, sd := range searchDirs {
		dbPath := filepath.Join(sd.dir, "memory.db")
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}

		memories, err := readMemoriesFromDB(dbPath)
		if err != nil || len(memories) == 0 {
			continue
		}

		path := fmt.Sprintf("memories/%s.json", sd.slug)
		if err := writeJSONFile(filepath.Join(destDir, path), memories); err == nil {
			files = append(files, path)
		}
	}

	return files, nil
}

func readMemoriesFromDB(dbPath string) ([]map[string]interface{}, error) {
	db, err := sql.Open("sqlite3", dbPath+"?mode=ro&_journal_mode=WAL&_busy_timeout=3000")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT id, content, summary, category, importance, source, tags, access_count, archived, created_at, updated_at
		 FROM memories WHERE archived = 0 ORDER BY importance DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []map[string]interface{}
	for rows.Next() {
		var id, content, summary, category, source, tags string
		var importance, accessCount, archived int
		var createdAt, updatedAt string
		if rows.Scan(&id, &content, &summary, &category, &importance, &source, &tags, &accessCount, &archived, &createdAt, &updatedAt) != nil {
			continue
		}
		m := map[string]interface{}{
			"id": id, "content": content, "summary": summary,
			"category": category, "importance": importance,
			"source": source, "tags": tags,
			"access_count": accessCount, "archived": archived == 1,
			"created_at": createdAt, "updated_at": updatedAt,
		}
		memories = append(memories, m)
	}

	return memories, nil
}

func exportContextBlobs(dataDir, destDir string) ([]string, error) {
	var files []string

	contextDir := filepath.Join(dataDir, "context")
	entries, err := os.ReadDir(contextDir)
	if err != nil {
		return nil, nil // context dir may not exist
	}

	if err := os.MkdirAll(filepath.Join(destDir, "context", "blobs"), 0755); err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		srcPath := filepath.Join(contextDir, entry.Name())
		dstPath := filepath.Join(destDir, "context", "blobs", entry.Name())
		if err := copyFile(srcPath, dstPath); err == nil {
			files = append(files, "context/blobs/"+entry.Name())
		}
	}

	return files, nil
}

func exportUsers(db *database.DB, destDir string) ([]string, error) {
	rows, err := db.Query("SELECT id, username, display_name, avatar_path, created_at, updated_at FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var id, username, displayName, avatarPath string
		var createdAt, updatedAt time.Time
		if rows.Scan(&id, &username, &displayName, &avatarPath, &createdAt, &updatedAt) != nil {
			continue
		}
		users = append(users, map[string]interface{}{
			"id": id, "username": username, "display_name": displayName,
			"avatar_path": avatarPath, "created_at": createdAt, "updated_at": updatedAt,
		})
	}

	if len(users) == 0 {
		return nil, nil
	}

	path := "users.json"
	if err := writeJSONFile(filepath.Join(destDir, path), users); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func exportAgentToolAccess(db *database.DB, destDir string) ([]string, error) {
	rows, err := db.Query("SELECT id, agent_role_slug, tool_id, granted_at FROM agent_tool_access")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var grants []map[string]interface{}
	for rows.Next() {
		var id, slug, toolID string
		var grantedAt time.Time
		if rows.Scan(&id, &slug, &toolID, &grantedAt) != nil {
			continue
		}
		grants = append(grants, map[string]interface{}{
			"id": id, "agent_role_slug": slug, "tool_id": toolID, "granted_at": grantedAt,
		})
	}

	if len(grants) == 0 {
		return nil, nil
	}

	path := "agent_tool_access.json"
	if err := writeJSONFile(filepath.Join(destDir, path), grants); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func exportSecretsMeta(db *database.DB, destDir string) ([]string, error) {
	rows, err := db.Query("SELECT id, name, description, tool_id, created_at, updated_at FROM secrets")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []map[string]interface{}
	for rows.Next() {
		var id, name, desc, toolID string
		var createdAt, updatedAt time.Time
		if rows.Scan(&id, &name, &desc, &toolID, &createdAt, &updatedAt) != nil {
			continue
		}
		secrets = append(secrets, map[string]interface{}{
			"id": id, "name": name, "description": desc, "tool_id": toolID,
			"created_at": createdAt, "updated_at": updatedAt,
		})
	}

	if len(secrets) == 0 {
		return nil, nil
	}

	path := "secrets.json"
	if err := writeJSONFile(filepath.Join(destDir, path), secrets); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func exportAvatars(dataDir, destDir string) ([]string, error) {
	return copyDirFiles(filepath.Join(dataDir, "avatars"), filepath.Join(destDir, "avatars"), "avatars/")
}

func exportBackgrounds(dataDir, destDir string) ([]string, error) {
	return copyDirFiles(filepath.Join(dataDir, "backgrounds"), filepath.Join(destDir, "backgrounds"), "backgrounds/")
}

func copyDirFiles(srcDir, dstDir, prefix string) ([]string, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, nil // dir may not exist
	}

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(srcDir, entry.Name()), filepath.Join(dstDir, entry.Name())); err == nil {
			files = append(files, prefix+entry.Name())
		}
	}
	return files, nil
}

func exportChatHistory(db *database.DB, dataDir, destDir string) ([]string, error) {
	var files []string

	// Threads
	rows, err := db.Query("SELECT id, title, created_at, updated_at FROM chat_threads ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threads []map[string]interface{}
	var threadIDs []string
	for rows.Next() {
		var id, title string
		var createdAt, updatedAt time.Time
		if rows.Scan(&id, &title, &createdAt, &updatedAt) != nil {
			continue
		}
		threads = append(threads, map[string]interface{}{
			"id": id, "title": title, "created_at": createdAt, "updated_at": updatedAt,
		})
		threadIDs = append(threadIDs, id)
	}
	rows.Close()

	if len(threads) > 0 {
		os.MkdirAll(filepath.Join(destDir, "chat"), 0755)
		path := "chat/threads.json"
		if err := writeJSONFile(filepath.Join(destDir, path), threads); err == nil {
			files = append(files, path)
		}
	}

	// Thread members
	mRows, err := db.Query("SELECT thread_id, agent_role_slug, joined_at FROM thread_members ORDER BY thread_id")
	if err == nil {
		defer mRows.Close()
		var members []map[string]interface{}
		for mRows.Next() {
			var threadID, slug string
			var joinedAt time.Time
			if mRows.Scan(&threadID, &slug, &joinedAt) != nil {
				continue
			}
			members = append(members, map[string]interface{}{
				"thread_id": threadID, "agent_role_slug": slug, "joined_at": joinedAt,
			})
		}
		mRows.Close()
		if len(members) > 0 {
			os.MkdirAll(filepath.Join(destDir, "chat"), 0755)
			path := "chat/thread_members.json"
			if err := writeJSONFile(filepath.Join(destDir, path), members); err == nil {
				files = append(files, path)
			}
		}
	}

	// Messages per thread
	if len(threadIDs) > 0 {
		os.MkdirAll(filepath.Join(destDir, "chat", "messages"), 0755)
	}
	for _, tid := range threadIDs {
		msgRows, err := db.Query(
			"SELECT id, thread_id, role, content, agent_role_slug, cost_usd, input_tokens, output_tokens, widget_data, image_url, created_at FROM chat_messages WHERE thread_id = ? ORDER BY created_at", tid)
		if err != nil {
			continue
		}
		var messages []map[string]interface{}
		for msgRows.Next() {
			var id, threadID, role, content, agentSlug string
			var costUSD float64
			var inputTokens, outputTokens int
			var widgetData, imageURL *string
			var createdAt time.Time
			if msgRows.Scan(&id, &threadID, &role, &content, &agentSlug, &costUSD, &inputTokens, &outputTokens, &widgetData, &imageURL, &createdAt) != nil {
				continue
			}
			m := map[string]interface{}{
				"id": id, "thread_id": threadID, "role": role, "content": content,
				"agent_role_slug": agentSlug, "cost_usd": costUSD,
				"input_tokens": inputTokens, "output_tokens": outputTokens,
				"widget_data": widgetData, "image_url": imageURL,
				"created_at": createdAt,
			}
			messages = append(messages, m)
		}
		msgRows.Close()
		if len(messages) > 0 {
			path := fmt.Sprintf("chat/messages/%s.json", tid)
			if err := writeJSONFile(filepath.Join(destDir, path), messages); err == nil {
				files = append(files, path)
			}
		}
	}

	// Attachments metadata
	aRows, err := db.Query("SELECT id, message_id, filename, original_name, mime_type, size_bytes, created_at FROM chat_attachments ORDER BY created_at")
	if err == nil {
		defer aRows.Close()
		var attachments []map[string]interface{}
		for aRows.Next() {
			var id, msgID, filename, origName, mimeType string
			var sizeBytes int
			var createdAt time.Time
			if aRows.Scan(&id, &msgID, &filename, &origName, &mimeType, &sizeBytes, &createdAt) != nil {
				continue
			}
			attachments = append(attachments, map[string]interface{}{
				"id": id, "message_id": msgID, "filename": filename,
				"original_name": origName, "mime_type": mimeType,
				"size_bytes": sizeBytes, "created_at": createdAt,
			})
		}
		aRows.Close()
		if len(attachments) > 0 {
			os.MkdirAll(filepath.Join(destDir, "chat", "attachments"), 0755)
			path := "chat/attachments/metadata.json"
			if err := writeJSONFile(filepath.Join(destDir, path), attachments); err == nil {
				files = append(files, path)
			}

			// Copy attachment blobs
			blobSrc := filepath.Join(dataDir, "chat-attachments")
			blobDst := filepath.Join(destDir, "chat", "attachments", "blobs")
			os.MkdirAll(blobDst, 0755)
			for _, a := range attachments {
				fname := a["filename"].(string)
				if err := copyFile(filepath.Join(blobSrc, fname), filepath.Join(blobDst, fname)); err == nil {
					files = append(files, "chat/attachments/blobs/"+fname)
				}
			}
		}
	}

	return files, nil
}

func exportNotifications(db *database.DB, destDir string) ([]string, error) {
	rows, err := db.Query("SELECT id, title, body, priority, source_agent_slug, source_type, link, read, dismissed, created_at FROM notifications ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifs []map[string]interface{}
	for rows.Next() {
		var id, title, body, priority, srcSlug, srcType, link string
		var isRead, dismissed int
		var createdAt time.Time
		if rows.Scan(&id, &title, &body, &priority, &srcSlug, &srcType, &link, &isRead, &dismissed, &createdAt) != nil {
			continue
		}
		notifs = append(notifs, map[string]interface{}{
			"id": id, "title": title, "body": body, "priority": priority,
			"source_agent_slug": srcSlug, "source_type": srcType, "link": link,
			"read": isRead == 1, "dismissed": dismissed == 1,
			"created_at": createdAt,
		})
	}

	if len(notifs) == 0 {
		return nil, nil
	}

	path := "notifications.json"
	if err := writeJSONFile(filepath.Join(destDir, path), notifs); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func exportSystemStats(db *database.DB, destDir string) ([]string, error) {
	rows, err := db.Query("SELECT key, value FROM system_stats")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := map[string]float64{}
	for rows.Next() {
		var key string
		var value float64
		if rows.Scan(&key, &value) != nil {
			continue
		}
		stats[key] = value
	}

	if len(stats) == 0 {
		return nil, nil
	}

	path := "system_stats.json"
	if err := writeJSONFile(filepath.Join(destDir, path), stats); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func exportBrowserSessions(db *database.DB, destDir string) ([]string, error) {
	rows, err := db.Query("SELECT id, name, headless, owner_agent_slug, created_at, updated_at FROM browser_sessions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []map[string]interface{}
	for rows.Next() {
		var id, name, ownerSlug string
		var headless int
		var createdAt, updatedAt time.Time
		if rows.Scan(&id, &name, &headless, &ownerSlug, &createdAt, &updatedAt) != nil {
			continue
		}
		sessions = append(sessions, map[string]interface{}{
			"id": id, "name": name, "headless": headless == 1,
			"owner_agent_slug": ownerSlug,
			"created_at": createdAt, "updated_at": updatedAt,
		})
	}

	if len(sessions) == 0 {
		return nil, nil
	}

	path := "browser_sessions.json"
	if err := writeJSONFile(filepath.Join(destDir, path), sessions); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func exportAgentSkills(dataDir, destDir string) ([]string, error) {
	var files []string

	agentsDir := filepath.Join(dataDir, "agents")
	agentEntries, err := os.ReadDir(agentsDir)
	if err != nil {
		return nil, nil // agents dir may not exist
	}

	for _, agentEntry := range agentEntries {
		if !agentEntry.IsDir() {
			continue
		}
		slug := agentEntry.Name()
		skillsDir := filepath.Join(agentsDir, slug, "skills")
		skillEntries, err := os.ReadDir(skillsDir)
		if err != nil {
			continue
		}
		for _, skillEntry := range skillEntries {
			if !skillEntry.IsDir() {
				continue
			}
			skillFile := filepath.Join(skillsDir, skillEntry.Name(), "SKILL.md")
			content, err := os.ReadFile(skillFile)
			if err != nil {
				continue
			}
			destSkillDir := filepath.Join(destDir, "agent_skills", slug, skillEntry.Name())
			os.MkdirAll(destSkillDir, 0755)
			path := fmt.Sprintf("agent_skills/%s/%s/SKILL.md", slug, skillEntry.Name())
			if err := os.WriteFile(filepath.Join(destDir, path), content, 0644); err == nil {
				files = append(files, path)
			}
		}
	}

	return files, nil
}

func exportWorkbenches(db *database.DB, destDir string) ([]string, error) {
	rows, err := db.Query("SELECT id, name, sort_order, color, created_at FROM workbenches ORDER BY sort_order")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workbenches []map[string]interface{}
	for rows.Next() {
		var id, name, color string
		var sortOrder int
		var createdAt time.Time
		if rows.Scan(&id, &name, &sortOrder, &color, &createdAt) != nil {
			continue
		}
		workbenches = append(workbenches, map[string]interface{}{
			"id": id, "name": name, "sort_order": sortOrder, "color": color, "created_at": createdAt,
		})
	}

	if len(workbenches) == 0 {
		return nil, nil
	}

	path := "workbenches.json"
	if err := writeJSONFile(filepath.Join(destDir, path), workbenches); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func exportProjects(db *database.DB, destDir string) ([]string, error) {
	rows, err := db.Query("SELECT id, name, color, created_at FROM projects ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type projectExport struct {
		ID        string                   `json:"id"`
		Name      string                   `json:"name"`
		Color     string                   `json:"color"`
		CreatedAt time.Time                `json:"created_at"`
		Repos     []map[string]interface{} `json:"repos"`
	}

	var projects []projectExport
	for rows.Next() {
		var p projectExport
		if rows.Scan(&p.ID, &p.Name, &p.Color, &p.CreatedAt) != nil {
			continue
		}
		p.Repos = []map[string]interface{}{}
		projects = append(projects, p)
	}

	if len(projects) == 0 {
		return nil, nil
	}

	// Load repos
	repoRows, err := db.Query("SELECT id, project_id, name, folder_path, command, sort_order FROM project_repos ORDER BY sort_order")
	if err == nil {
		defer repoRows.Close()
		repoMap := make(map[string][]map[string]interface{})
		for repoRows.Next() {
			var id, projectID, name, folderPath, command string
			var sortOrder int
			if repoRows.Scan(&id, &projectID, &name, &folderPath, &command, &sortOrder) != nil {
				continue
			}
			repoMap[projectID] = append(repoMap[projectID], map[string]interface{}{
				"id": id, "project_id": projectID, "name": name,
				"folder_path": folderPath, "command": command, "sort_order": sortOrder,
			})
		}
		for i := range projects {
			if repos, ok := repoMap[projects[i].ID]; ok {
				projects[i].Repos = repos
			}
		}
	}

	path := "projects.json"
	if err := writeJSONFile(filepath.Join(destDir, path), projects); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func exportAgentTasks(db *database.DB, destDir string) ([]string, error) {
	rows, err := db.Query("SELECT id, agent_role_slug, title, description, status, sort_order, created_at, updated_at FROM agent_tasks ORDER BY agent_role_slug, sort_order")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []map[string]interface{}
	for rows.Next() {
		var id, slug, title, desc, status string
		var sortOrder int
		var createdAt, updatedAt time.Time
		if rows.Scan(&id, &slug, &title, &desc, &status, &sortOrder, &createdAt, &updatedAt) != nil {
			continue
		}
		tasks = append(tasks, map[string]interface{}{
			"id": id, "agent_role_slug": slug, "title": title,
			"description": desc, "status": status, "sort_order": sortOrder,
			"created_at": createdAt, "updated_at": updatedAt,
		})
	}

	if len(tasks) == 0 {
		return nil, nil
	}

	path := "agent_tasks.json"
	if err := writeJSONFile(filepath.Join(destDir, path), tasks); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func exportTodoLists(db *database.DB, destDir string) ([]string, int, int, error) {
	// Export lists
	listRows, err := db.Query("SELECT id, name, description, color, sort_order, created_at, updated_at FROM todo_lists ORDER BY sort_order")
	if err != nil {
		return nil, 0, 0, err
	}
	defer listRows.Close()

	var lists []map[string]interface{}
	for listRows.Next() {
		var id, name, desc, color string
		var sortOrder int
		var createdAt, updatedAt time.Time
		if listRows.Scan(&id, &name, &desc, &color, &sortOrder, &createdAt, &updatedAt) != nil {
			continue
		}
		lists = append(lists, map[string]interface{}{
			"id": id, "name": name, "description": desc, "color": color,
			"sort_order": sortOrder, "created_at": createdAt, "updated_at": updatedAt,
		})
	}
	listRows.Close()

	if len(lists) == 0 {
		return nil, 0, 0, nil
	}

	// Export items
	itemRows, err := db.Query(
		`SELECT id, list_id, title, notes, completed, sort_order, due_date,
		        last_actor_agent_slug, last_actor_note, created_at, updated_at, completed_at
		 FROM todo_items ORDER BY list_id, sort_order`)
	if err != nil {
		return nil, 0, 0, err
	}
	defer itemRows.Close()

	var items []map[string]interface{}
	for itemRows.Next() {
		var id, listID, title, notes, lastActorNote string
		var completed, sortOrder int
		var dueDate, lastActorSlug, completedAt *string
		var createdAt, updatedAt time.Time
		if itemRows.Scan(&id, &listID, &title, &notes, &completed, &sortOrder, &dueDate,
			&lastActorSlug, &lastActorNote, &createdAt, &updatedAt, &completedAt) != nil {
			continue
		}
		items = append(items, map[string]interface{}{
			"id": id, "list_id": listID, "title": title, "notes": notes,
			"completed": completed == 1, "sort_order": sortOrder,
			"due_date": dueDate, "last_actor_agent_slug": lastActorSlug,
			"last_actor_note": lastActorNote,
			"created_at": createdAt, "updated_at": updatedAt, "completed_at": completedAt,
		})
	}

	var files []string

	path := "todo_lists.json"
	if err := writeJSONFile(filepath.Join(destDir, path), map[string]interface{}{
		"lists": lists,
		"items": items,
	}); err != nil {
		return nil, 0, 0, err
	}
	files = append(files, path)

	return files, len(lists), len(items), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func writeJSONFile(path string, data interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func countPrefix(files []string, prefix string) int {
	n := 0
	for _, f := range files {
		if strings.HasPrefix(f, prefix) {
			n++
		}
	}
	return n
}
