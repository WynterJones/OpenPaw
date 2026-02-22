package agents

import (
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
)

type PresetRole struct {
	Slug        string
	Name        string
	Description string
	SystemPrompt string
	Model       string
	AvatarPath  string
	SortOrder   int
}

var PresetRoles = []PresetRole{
	{
		Slug:        "builder",
		Name:        "Pounce",
		Description: "Gateway & Builder — Routes conversations, builds tools, dashboards, and agents.",
		SystemPrompt: `You are Pounce, the Tool Builder at OpenPaw. You specialize in building internal tools from natural language descriptions. Users describe what they need, and you design, code, test, and deploy it.

Your personality: Resourceful, enthusiastic, and hands-on. You turn ideas into working tools quickly. You ask clarifying questions when needed, then get to work.

Guidelines:
- Build tools iteratively — get a working version first, then refine
- Write clean, production-ready code with proper error handling
- Explain what you're building and why as you go
- Suggest improvements and edge cases the user might not have considered
- Keep tools simple, focused, and easy to maintain`,
		Model:      "sonnet",
		AvatarPath: "/avatars/avatar-4.webp",
		SortOrder:  1,
	},
}

func SeedPresetRoles(db *database.DB, enabledSlugs []string) error {
	enabledMap := make(map[string]bool)
	for _, s := range enabledSlugs {
		enabledMap[s] = true
	}

	now := time.Now().UTC()
	for _, role := range PresetRoles {
		enabled := 1
		if len(enabledSlugs) > 0 && !enabledMap[role.Slug] {
			enabled = 0
		}

		id := uuid.New().String()
		_, err := db.Exec(
			`INSERT OR IGNORE INTO agent_roles (id, slug, name, description, system_prompt, model, avatar_path, enabled, sort_order, is_preset, identity_initialized, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, ?, ?)`,
			id, role.Slug, role.Name, role.Description, role.SystemPrompt, role.Model,
			role.AvatarPath, enabled, role.SortOrder, now, now,
		)
		if err != nil {
			logger.Error("Failed to seed role %s: %v", role.Slug, err)
			return err
		}
	}

	logger.Info("Seeded %d preset agent roles", len(PresetRoles))
	return nil
}
