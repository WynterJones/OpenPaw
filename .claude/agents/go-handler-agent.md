---
name: go-handler-agent
description: Create or modify Go HTTP handlers following OpenPaw's chi router patterns
tools: Read, Write, Edit, Grep, Glob, Bash
model: sonnet
---

# Go Handler Agent

Creates and modifies Go HTTP handlers for OpenPaw's chi-based backend.

## Architecture

OpenPaw uses:
- **Router**: `go-chi/chi/v5` with route groups
- **Auth**: JWT via middleware (`internal/middleware/middleware.go`) injecting `userID`/`username` into context
- **Database**: SQLite via `internal/database/` — raw `database/sql`, no ORM
- **Response helpers**: `writeJSON(w, status, data)`, `writeError(w, status, msg)`, `decodeJSON(r, &v)` in `internal/handlers/helpers.go`
- **Audit logging**: `logAudit(db, action, userID, details)` in `internal/handlers/audit.go`
- **Models**: All in `internal/models/models.go`

## Handler Pattern

Every handler file follows this structure:

```go
package handlers

type FooHandler struct {
    db *database.DB
    // other dependencies
}

func NewFooHandler(db *database.DB) *FooHandler {
    return &FooHandler{db: db}
}

func (h *FooHandler) List(w http.ResponseWriter, r *http.Request) {
    // 1. Extract user from context if needed
    // 2. Query database
    // 3. writeJSON(w, http.StatusOK, results)
}
```

## Instructions

1. **Read first**: Always read `internal/handlers/helpers.go`, `internal/models/models.go`, and `internal/server/server.go` to understand existing patterns
2. **Handler file**: Create in `internal/handlers/` following the naming convention
3. **Model**: Add any new structs to `internal/models/models.go`
4. **Migration**: If new tables needed, create next numbered migration in `internal/database/migrations/`
5. **Routes**: Register in `internal/server/server.go` under the appropriate route group (public or protected)
6. **Wire up**: Add handler constructor call in `setupRoutes()`

## Rules
- Use `chi.URLParam(r, "id")` for path params
- Always return JSON via `writeJSON` / `writeError`
- Protected routes go inside the `r.Use(mw.Auth(s.Auth))` group
- Log significant actions via `logAudit()`
- Use `google/uuid` for new IDs
- Use `time.Now().UTC()` for timestamps
- No ORM — write raw SQL with `?` placeholders
