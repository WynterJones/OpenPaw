---
name: fullstack-agent
description: Build full-stack features end-to-end — Go handler + migration + React page + route wiring
tools: Read, Write, Edit, Grep, Glob, Bash
model: opus
---

# Full-Stack Agent

Builds complete features across the entire OpenPaw stack: database migration, Go handler, API routes, React page, and navigation wiring.

## The Stack

| Layer | Tech | Location |
|-------|------|----------|
| Database | SQLite (WAL mode) | `internal/database/migrations/` |
| Models | Go structs | `internal/models/models.go` |
| Handlers | Go + chi router | `internal/handlers/` |
| Routes | chi route groups | `internal/server/server.go` |
| API client | TypeScript fetch wrapper | `web/frontend/src/lib/api.ts` |
| Pages | React + TypeScript | `web/frontend/src/pages/` |
| Components | React + Tailwind + CSS vars | `web/frontend/src/components/` |
| Navigation | Sidebar + BottomNav | `web/frontend/src/components/` |
| Routing | React Router v7 | `web/frontend/src/App.tsx` |

## Build Order

Always build features in this order:

1. **Migration** — `internal/database/migrations/NNN_feature.sql`
2. **Model** — Add structs to `internal/models/models.go`
3. **Handler** — New file in `internal/handlers/feature.go`
4. **Routes** — Wire handler in `internal/server/server.go`
5. **API types** — Add TypeScript types in `web/frontend/src/lib/api.ts`
6. **Page** — Create `web/frontend/src/pages/Feature.tsx`
7. **App route** — Add to `web/frontend/src/App.tsx`
8. **Navigation** — Add to `Sidebar.tsx` and `BottomNav.tsx`

## Instructions

1. **Read the full stack first**: Read `server.go`, `models.go`, `helpers.go`, `api.ts`, `App.tsx`, and at least one existing page + handler pair to absorb all patterns
2. **Follow the build order** above strictly
3. **Test the build**: Run `cd web/frontend && npm run build` to verify frontend compiles, then `go vet ./...` for Go

## Key Patterns
- Go handlers use `writeJSON()`, `writeError()`, `decodeJSON()` helpers
- All IDs are UUIDs (`github.com/google/uuid`)
- Frontend uses `var(--op-*)` CSS custom properties for all colors
- API wrapper auto-redirects to `/login` on 401
- Protected routes go inside the `mw.Auth()` middleware group

## Rules
- Always create migration BEFORE referencing new tables in handlers
- Match existing code style exactly — no new patterns or abstractions
- Keep handlers thin — validation, DB query, response
- Frontend types must match Go model fields
- Test both `go vet ./...` and frontend `npm run build` when done
