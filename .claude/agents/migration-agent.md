---
name: migration-agent
description: Create SQLite database migrations following OpenPaw's numbered convention
tools: Read, Write, Edit, Grep, Glob
model: sonnet
---

# Migration Agent

Creates SQLite migration files for OpenPaw's embedded migration system.

## How Migrations Work

- Located in `internal/database/migrations/`
- Named `NNN_description.sql` (zero-padded 3 digits)
- Embedded via `//go:embed migrations/*.sql` in `internal/database/database.go`
- Run sequentially on startup, tracked in `schema_migrations` table
- SQLite dialect only — no Postgres/MySQL features

## Existing Schema

Read `internal/database/migrations/` to see all existing tables. Key tables:
- `users` — single admin user
- `tools` — registered tools (soft-delete via `deleted_at`)
- `secrets` — AES-encrypted credentials
- `schedules` — cron jobs tied to tools
- `dashboards` — dashboard configs (JSON columns)
- `audit_logs` — full audit trail
- `chat_threads` / `chat_messages` — conversation threads
- `settings` — key-value store
- `agents` / `work_orders` — Claude Code agent tracking
- `agent_roles` — AI personas

## Instructions

1. **Read existing migrations** in `internal/database/migrations/` to find the next number
2. **Read `internal/models/models.go`** to understand existing model structs
3. **Create migration file** with the next sequential number
4. **Update models** in `internal/models/models.go` if new tables/columns are added

## Rules
- Always use `IF NOT EXISTS` for CREATE TABLE
- Use `TEXT` for strings, `INTEGER` for ints/bools, `DATETIME` for timestamps
- Foreign keys: `REFERENCES table(column) ON DELETE CASCADE`
- Default timestamps: `DEFAULT CURRENT_TIMESTAMP`
- UUIDs stored as `TEXT PRIMARY KEY`
- For ALTER TABLE, SQLite is limited — no DROP COLUMN, no MODIFY COLUMN
- Keep migrations idempotent where possible
- Test by running `go run ./cmd/openpaw` and checking startup logs
