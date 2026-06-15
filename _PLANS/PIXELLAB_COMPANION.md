# PixelLab Pixel-Art Companions

Port the wynter-code "PixelLab Adventurer" concept to OpenPaw: generate pixel-art
characters, animate them per chat state (idle / thinking / toolcall / responding),
and pin them as movable floating companions in chat that react to live agent
activity. Also: pre-fill the username on the login screen so you can see which
user is installed.

Reference (read-only): `/Users/wynterjones/Work/SYSTEM/wynter-code` —
`src/services/pixellabClient.ts`, `src/stores/adventurerStore.ts`,
`src/components/adventurer/*`, `src/hooks/useAdventurerActivity.ts`,
`src/components/tools/pixellab/PixellabAdventurerPopup.tsx`.

## Decisions (confirmed with user)

1. **API key**: encrypted server-side in the `settings` table (like the OpenRouter
   key). A generic Go proxy injects it; the key never reaches the browser.
2. **Companion model**: a library of characters; each can optionally be assigned to
   an agent role. The pinned chat companion swaps to the active agent's character
   while it responds, falling back to a default otherwise.
3. **Pinned count**: multiple companions can be pinned, each independently draggable
   with a remembered position.

## Architecture overview

- **Backend (Go)**: encrypted PixelLab key in `settings`; a thin authenticated proxy
  to `https://api.pixellab.ai/v2`; character CRUD that writes sprite frames to disk
  under the data dir and serves them; one migration for character metadata.
- **Frontend (React/TS)**: port the PixelLab client (now routed through the proxy);
  a creation wizard; a companion store + sprite animation; a WS-driven activity hook;
  a draggable floating-companions overlay mounted in `Layout`; a Settings tab.
- **Login**: expose the installed username from the public setup-status endpoint and
  pre-fill the login form.

PixelLab flow is unchanged from the reference: generate 3 pixflux images → pick one →
`create-character-v3` (poll job) → `animate-character` for 4 default emotes
(idle, walk, wave, cheer) → poll jobs → base64 PNG frames. Mood→clip mapping:
`idle→idle`, `thinking→wave`, `toolcall→walk`, `responding→cheer` (tunable).

## WebSocket → mood mapping (shared `useWebSocket` hook)

Subscribe via `web/frontend/src/lib/useWebSocket.ts` (shared connection — no need to
edit `Chat.tsx`). Derive mood from message types already broadcast:
- `agent_status` payload.status ∈ {routing, analyzing, thinking, spawning, compacting}
  or `gateway_thinking` → **thinking**
- `agent_stream` payload.event.type === `tool_start` → **toolcall** (until `tool_end`)
- `agent_stream` payload.event.type === `text_delta` → **responding**
- `agent_completed` or status === `done` → decay to **idle**
- Track active agent slug from the payload to drive per-agent character swap.

Decay timers mirror the reference (active ~1.5s, settle to idle).

## Files to create

Backend:
- `internal/database/migrations/047_pixellab_companion.sql`
- `internal/handlers/pixellab.go` — `PixelLabHandler`

Frontend:
- `web/frontend/src/lib/pixellabClient.ts` — proxied PixelLab client
- `web/frontend/src/stores/companionStore.ts` — characters + UI state (zustand)
- `web/frontend/src/components/companion/SpriteAnimation.tsx`
- `web/frontend/src/components/companion/CompanionWizard.tsx`
- `web/frontend/src/components/companion/ChatCompanions.tsx` — draggable overlay
- `web/frontend/src/hooks/useCompanionActivity.ts` — WS → mood/active-agent

## Files to modify

- `internal/server/server.go` — construct `PixelLabHandler`; register routes in the
  protected group; register the public sprite-serve route.
- `internal/handlers/settings.go` *or* new handler — PixelLab key get/update
  (reuse `secretsMgr.Encrypt` + `upsertSetting` pattern).
- `internal/handlers/setup.go` (status handler) — include installed `username`.
- `web/frontend/src/lib/types.ts` — `PixelLabCharacter`, `AnimationClip`,
  companion mood types.
- `web/frontend/src/pages/Settings.tsx` — new "Companion" tab (key field + character
  manager launching the wizard).
- `web/frontend/src/pages/Login.tsx` — pre-fill username from setup status.
- `web/frontend/src/components/Layout.tsx` — mount `<ChatCompanions />` overlay.

## API surface (all under `/api/v1`, protected unless noted)

- `GET  /settings/pixellab-api-key` → `{ configured }`
- `PUT  /settings/pixellab-api-key` → encrypt + store (validate via `/balance`)
- `POST /pixellab/proxy` → `{ path, method, body }`; forwards to PixelLab with the
  decrypted key. Path allow-list: `/balance`, `/create-image-pixflux`,
  `/create-character-v3`, `/animate-character`, `/background-jobs/*`.
- `GET  /pixellab/characters` → list
- `POST /pixellab/characters` → save finished character (name, pixellab character id,
  base sprite + animation clips as base64); writes frames to disk, returns URLs
- `POST /pixellab/characters/{id}/animations` → add an extra emote
- `PUT  /pixellab/characters/{id}` → pin/unpin, assign agent slug
- `DELETE /pixellab/characters/{id}` → remove row + on-disk frames
- `GET  /pixellab/sprites/{id}/{clip}/{frame}.png` → serve a PNG frame

## Data model (migration 047)

```sql
CREATE TABLE IF NOT EXISTS pixellab_characters (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL DEFAULT '',
    name          TEXT NOT NULL,
    pixellab_id   TEXT NOT NULL,            -- reusable PixelLab character id
    base_path     TEXT NOT NULL DEFAULT '', -- base sprite frame on disk
    animations    TEXT NOT NULL DEFAULT '[]', -- JSON: [{id,name,fps,frames:[paths]}]
    pinned        INTEGER NOT NULL DEFAULT 0,
    agent_slug    TEXT NOT NULL DEFAULT '',  -- optional assigned agent
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Frames written to `{dataDir}/pixellab/{characterId}/{clipId}/frame_N.png`; the
`animations` JSON stores relative paths, served via `/pixellab/sprites/...`.

## Disk / storage

Reuse the `UploadBackground` write pattern (validate magic bytes, `os.MkdirAll`,
`os.Create`). Reuse the `ServeBackground` serve pattern (sanitize path segments with
`filepath.Base`, `http.ServeFile`). Cleanup on delete with `os.RemoveAll`.

## Login username autofill

`setup.go` status handler returns the first/primary user's `username` (public — it is
acceptable for a self-hosted single-user app and is exactly what the user wants to
see). `Login.tsx` sets `username` state from `/setup/status` in its existing effect.

## Build & verification

Per OpenPaw CLAUDE.md:
- `cd web/frontend && npm run lint`
- `go vet -tags fts5 ./...`
- `go test -tags fts5 ./...`
- `just build` (frontend → Go) — frontend must rebuild before the Go binary embeds it.

## Risks / notes

- PixelLab calls are slow + credit-metered; keep the wizard's sequential animation
  and graceful per-emote fallback to a static frame (from the reference).
- The proxy must allow-list PixelLab paths and only ever target the fixed host.
- Sprite `<img>` tags hit an authenticated route; same-origin cookies are sent
  automatically, so this works without a public route.
- Companions live in `Layout` so they persist across navigation and float over chat.
- Keep frame counts small (4 per emote, 64×64) to bound disk + render cost.
