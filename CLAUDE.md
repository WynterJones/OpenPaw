# OpenPaw

## MANDATORY: Issue-First Workflow

**ALWAYS create beads issues BEFORE starting work.** This ensures full visibility and tracking.

```bash
# Single task
bd create "Task description" -t bug|feature|task -p 0-4
bd update <id> --status in_progress
# Do the work
bd close <id> --reason "What was done"

# Multiple tasks - log ALL first, then work through
bd create "Task 1" -t feature && bd create "Task 2" -t task
bd list --status open
```

**NO EXCEPTIONS**: Every task gets an issue.

---

## Architecture & Build Pipeline

OpenPaw is a **single-binary Go application** with an embedded React frontend.

### Stack
| Layer | Tech | Location |
|-------|------|----------|
| Backend | Go 1.25 + chi router + SQLite | `cmd/openpaw/`, `internal/` |
| Frontend | React 19 + TypeScript 5.9 + Tailwind v4 | `web/frontend/src/` |
| Embedding | `go:embed all:frontend/dist` | `web/embed.go` |
| AI Engine | Claude Code CLI (shells out to `claude`) | `internal/agents/` |
| Database | SQLite (WAL mode, embedded migrations) | `internal/database/` |

### Build Order (IMPORTANT)

The frontend is embedded into the Go binary via `go:embed`. This means:

1. **Frontend changes require rebuilding the frontend FIRST**: `cd web/frontend && npm run build`
2. **Then rebuild the Go binary**: `CGO_ENABLED=1 go build -o openpaw ./cmd/openpaw`
3. **Or use**: `just build` (does both in order)

```bash
# After frontend-only changes:
just frontend-build    # Rebuild React → web/frontend/dist/

# After Go-only changes:
just go-build          # Rebuild binary (uses existing dist/)

# After changes to both:
just build             # Frontend build → Go build (full pipeline)

# Build and run immediately:
just run               # Full build + execute binary
```

### Development Mode

For development, run frontend and backend separately:
- `just dev` — Vite dev server on `:5173` with HMR (proxies `/api` to `:8080`)
- `just serve` — Go backend on `:8080`
- `just dev-full` — Both in parallel

### Database

- SQLite file at `./data/openpaw.db`
- Migrations in `internal/database/migrations/` (numbered `NNN_name.sql`)
- Auto-run on startup, tracked in `schema_migrations` table
- `just db-info` / `just db-reset` / `just db-migrations`

### Key Conventions

- **No ORM** — raw SQL with `?` placeholders
- **UUIDs** for all IDs (`github.com/google/uuid`)
- **JSON responses** via `writeJSON()` / `writeError()` helpers
- **Audit logging** via `logAudit()` for significant actions
- **CSS custom properties** (`var(--op-*)`) for all colors — never hardcode
- **Lucide React** is the only icon library
- **API wrapper** in `web/frontend/src/lib/api.ts` — never use raw `fetch`

---

## Skills (Auto-Activate on Phrases)

Skills auto-activate when you use these phrases. Workflow details are in `.claude/skills/`.

| Phrase | Skill | What Happens |
|--------|-------|--------------|
| **open the farm** | farm-audit | Audit systems, update FARMHOUSE.md |
| **count the herd** | farm-inspect | Full code inspection (no push) |
| **go to market** | market | i18n + accessibility audit |
| **go to production** | production | BROWNFIELD update, strategy check |
| **I have an idea for...** | garden | Plant idea in GARDEN.md |
| **water the garden** | garden | Generate 10 new ideas |
| **compost this...** | garden | Move idea to COMPOST.md |
| **let's research...** | research | Create/update _RESEARCH/ doc |

---

## Slash Commands (Explicit)

| Command | What It Does |
|---------|--------------|
| `/push` | Lint, test, build, commit, push |
| `/office` | Interactive strategy setup |

---

## Project-Specific Agents

| Agent | Purpose |
|-------|---------|
| `fullstack-agent` | End-to-end features: migration → handler → route → page → nav |
| `go-handler-agent` | Go HTTP handlers following chi/helpers/audit patterns |
| `page-builder` | React pages with design tokens and existing component library |
| `migration-agent` | SQLite migrations following numbered convention |
| `prompt-engineer` | AI agent system prompts and gateway routing logic |
| `theme-agent` | OKLCH color system, design tokens, theming pipeline |
| `websocket-agent` | WebSocket hub extensions and real-time features |

---

## Plan Mode Protocol

When entering Plan Mode:

1. **Save Plan**: Write to `_PLANS/<FEATURE_NAME>.md` (SCREAMING_SNAKE_CASE)
2. **Exit & Create Epic**: Create beads Epic + child issues
3. **Confirm**: Ask "Ready to start implementing?" - wait for explicit yes

---

## Project Configuration

- **Lint:** `cd web/frontend && npm run lint`
- **Vet:** `go vet ./...`
- **Test:** `go test ./...`
- **Frontend Build:** `cd web/frontend && npm run build`
- **Go Build:** `CGO_ENABLED=1 go build -o openpaw ./cmd/openpaw`
- **Full Build:** `just build` (frontend → Go)
- **Dead Code:** `cd web/frontend && npx knip`
- **Quality Gate:** `just quality` (lint + vet + test + build)

---

## Quick Reference

```bash
just --list        # See all commands
just info          # Show project paths and config
just routes        # Show all API routes
bd list            # See all issues
ls .claude/skills  # See available skills
ls .claude/agents  # See available agents
```
