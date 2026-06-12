# SUBSCRIPTION CLI PROVIDERS (Claude Code + Codex)

## Context

OpenPaw currently routes **all** LLM traffic through OpenRouter (per-token billing). Users with Claude Max or ChatGPT subscriptions already pay for inference via the `claude` and `codex` CLIs installed locally. This feature adds those CLIs as **additive, opt-in LLM providers** so chat/agent/gateway traffic can run off the local subscription instead of OpenRouter credits.

**OpenRouter is NOT removed or changed** — it remains the default provider, the image-generation backend in all modes, and a fully supported option. No behavior change unless the user explicitly switches provider in Settings.

No tmux/PTY needed: both CLIs have headless modes (`claude -p --output-format stream-json`, `codex exec --json`) with session resume.

## Decisions (user-confirmed)

1. **Global provider setting only** (`openrouter` | `claude-code` | `codex`) — no per-agent overrides.
2. **MCP bridge** — OpenPaw tools (memory, todos, delegation, image gen, browser, call_tool) exposed to CLIs via MCP for full parity.
3. **Both Claude Code and Codex in v1.**
4. **Chat + gateway via CLI; image gen stays on OpenRouter** (gracefully disabled if no key — already gated on `client.IsConfigured()`).
5. **Additive** — OpenRouter path stays byte-identical and default.

## Verified architecture facts

- Only two inference entry points: `(*llm.Client).RunAgentLoop(ctx, AgentConfig, msg)` and `RunOneShot(ctx, model, system, prompt)` (`internal/llm/agent_loop.go`).
- `AgentConfig` carries `Model, System, History, Tools, WorkDir, SandboxPaths, MaxTurns, ExtraTools, ExtraHandlers, OnEvent`.
- `StreamEvent` (`internal/llm/types.go`) already has `SessionID, NumTurns, Usage, TotalCostUSD` + event constants `init, text_delta, tool_start, tool_delta, tool_end, result, error`. Frontend consumes these verbatim via websocket `agent_stream` — **no frontend streaming changes** if adapters emit the same events.
- `RunAgentLoop` call sites: `agents/gateway.go:108` (GatewayAnalyze), `gateway.go:413` (RoleChat), `agents/delegate.go:312`, `agents/spawn.go:141`, `heartbeat/heartbeat.go:738`.
- `RunOneShot` call sites: `gateway.go:200` (summarize), `handlers/auth.go:520` (soul gen, hardcodes gemini), `handlers/chat_threads.go:138` (titles), `handlers/chat.go:541` (compaction).
- OpenPaw tools are injected as `ExtraTools []ToolDef` + `ExtraHandlers map[string]ToolHandler` (`func(ctx, workDir, json.RawMessage) ToolResult`) — maps 1:1 onto MCP tools. Handlers are closures with agentSlug/threadID baked in → attribution is free.
- Existing exec pattern: `internal/toollibrary/catalog/claude-code/claude.go` (exec.CommandContext + `claude -p`).
- Settings: key/value table, `upsertSetting`, secrets encrypted. Startup wiring at `cmd/openpaw/main.go:191-204`.
- Image gen gate already exists: `gateway.go:349` `m.client.IsConfigured()`.

## Phase 1 — Provider abstraction (`internal/llm/provider.go`)

```go
type Provider interface {
    Name() string        // "openrouter" | "claude-code" | "codex"
    IsConfigured() bool
    RunAgentLoop(ctx context.Context, cfg AgentConfig, userMessage string) (*AgentResult, error)
    RunOneShot(ctx context.Context, model, system, prompt string) (string, *UsageInfo, error)
    ResolveModel(name, fallbackTier string) string
    ListModels(ctx context.Context) ([]ModelInfo, error)
}

type ProviderRouter struct { // active selected by settings, hot-swappable
    mu sync.RWMutex; active string
    openrouter *Client; providers map[string]Provider
}
// Active(), SetActive(name), OpenRouter() — OpenRouter always reachable for image gen/balance
```

- `*Client` implements `Provider` trivially (Name, ResolveModel→existing `llm.ResolveModel`, ListModels→cached fetch). **Zero behavior change.**
- **Model tiers:** `haiku|sonnet|opus` are canonical cross-provider tiers. Claude adapter: tier → `--model haiku|sonnet|opus`; maps `anthropic/claude-*` IDs back to tiers, unknown → sonnet. Codex: haiku→`gpt-5.1-codex-mini`, sonnet→`gpt-5.1-codex`, opus→`gpt-5.1-codex-max` (verify IDs at impl). Existing `gateway_model`/`builder_model`/per-agent model settings keep working across provider flips.
- Call-site changes (mechanical): `agents.Manager` gains `providers *llm.ProviderRouter`; `m.client.RunAgentLoop` → `m.Provider().RunAgentLoop` in gateway/delegate/spawn/heartbeat; `RunOneShot` sites likewise; `Client()` accessor kept for image/vision (OpenRouter only). `auth.go` soul gen: replace hardcoded gemini with `Provider().RunOneShot(...)`. `agents.CheckAPIKey` → `CheckProvider(router)`.

## Phase 2 — MCP bridge (`internal/mcp/`, new package)

**Streamable-HTTP MCP endpoint on the existing chi server: `/mcp/{token}`** (not stdio — handlers are closures over live in-process state). Use `github.com/modelcontextprotocol/go-sdk` StreamableHTTPHandler; fallback: hand-rolled minimal JSON-RPC (`initialize`, `tools/list`, `tools/call`) — tools only.

- `internal/mcp/registry.go` — per-run session registry: token (crypto/rand 32B hex) → `{AgentSlug, ThreadID, WorkDir, Tools []llm.ToolDef, Handlers map[string]llm.ToolHandler, Expires}`. Adapter creates session before exec, releases in defer.
- `tools/list` → `Session.Tools` as MCP schema; `tools/call` → `Handlers[name](ctx, workDir, args)` → text content + isError.
- Mount in `internal/server/server.go` **unauthenticated** group (like `/ws`); unguessable token + loopback = auth. 401 on unknown/expired.
- **FS/bash tools NOT bridged** — CLIs have superior native tools. `--allowedTools "Read,Write,Edit,mcp__openpaw__*"`; `cmd.Dir` = agent sandbox.
- **No event emission from bridge** — the CLI's own stream-json reports MCP tool calls; adapter translates those to EventToolStart/End (single event source; widgets/audit keep working).

## Phase 3 — Claude Code adapter (`internal/llm/provider_claude.go` + `provider_claude_stream.go`)

- Args: `claude -p --output-format stream-json --verbose --model <tier> --max-turns <n> --system-prompt <cfg.System>`; prompt via **stdin** (ARG_MAX safety). MCP: `--mcp-config '{"mcpServers":{"openpaw":{"type":"http","url":"<url>"}}}'`, `--permission-mode bypassPermissions`.
- Session continuity: `thread_provider_sessions` lookup → `--resume <id>` + new message only; on miss/failure → fresh `--session-id <uuid>` + replay `cfg.History` (lossless). Persist session id from init/result event. Add optional `SessionKey{ThreadID, AgentSlug}` to `AgentConfig` (nil = always fresh: gateway/spawn/delegate).
- Process mgmt: `exec.CommandContext`, process-group kill (Setpgid + negative pid), `WaitDelay=10s`, `cmd.Env=os.Environ()` (HOME needed for subscription auth), semaphore default 3 concurrent.
- JSONL → StreamEvent: `system/init`→EventInit(+session_id); `assistant` text blocks→EventTextDelta; `tool_use`→EventToolStart; `user` tool_result→EventToolEnd; `result`→EventResult + AgentResult (tokens from usage, NumTurns, TotalCostUSD=0, StopReason success→stop / error_max_turns→max_turns). Tolerant decoder — unknown lines ignored. Scan MCP `generate_image` results for ImageURL.
- `RunOneShot`: `claude -p --output-format json --max-turns 1 --strict-mcp-config --mcp-config '{}'`, parse `.result`.
- `IsConfigured`: `exec.LookPath` + version probe, cached 60s.

## Phase 4 — Codex adapter (`internal/llm/provider_codex.go`)

- `codex exec --json -m <model> -C <workDir> --skip-git-repo-check --sandbox workspace-write -c 'mcp_servers.openpaw.url="<url>"' -` (prompt via stdin).
- No system-prompt flag → prepend system + replayed history in delimited blocks (`## SYSTEM INSTRUCTIONS / ## CONVERSATION SO FAR / ## NEW MESSAGE`). Documented fidelity limitation.
- Resume: `codex exec resume <session-id> --json`; same table, `provider='codex'`; same fresh fallback.
- Events (tolerant; record real fixtures during impl): `thread.started`→EventInit; `item.completed{agent_message}`→EventTextDelta; `item.started/completed{command_execution|mcp_tool_call|file_change}`→EventToolStart/End; `turn.completed{usage}`→tokens; final→EventResult.
- `--sandbox danger-full-access` only when `cfg.SandboxPaths` empty (mirrors current trust level).
- `IsConfigured`: `codex --version` + `codex login status`, cached 60s.

## Phase 5 — Migration, settings, wiring

- Migration `internal/database/migrations/045_llm_provider.sql` (use next free number):
```sql
CREATE TABLE IF NOT EXISTS thread_provider_sessions (
    thread_id  TEXT NOT NULL,
    agent_slug TEXT NOT NULL,
    provider   TEXT NOT NULL,
    session_id TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (thread_id, agent_slug, provider)
);
```
- `llm_provider` setting via existing key/value upsert (no migration needed). SessionStore: raw SQL `?` placeholders; delete rows on thread deletion (`chat_threads.go`).
- `cmd/openpaw/main.go`: read `llm_provider` (default **openrouter**), build mcp.Registry + both CLI providers (after port known), assemble ProviderRouter → NewManager. If chosen CLI probe fails at startup: warn, keep configured provider, surface errors per-request — never silently fall back.
- Endpoints (`handlers/settings.go`):
  - `GET /settings/llm-provider` → `{active, providers:{openrouter:{configured,source}, "claude-code":{available,version,path}, codex:{available,version,logged_in}}}`
  - `PUT /settings/llm-provider` `{provider}` → validate probe, `router.SetActive`, upsert, `logAudit`
  - `GET /settings/available-models` → branch on active provider (openrouter=fetched list; CLIs=static tier lists), same `{id,name}` contract
  - `GET /system/balance` → non-openrouter: `200 {provider, subscription:true}` for header chip
  - `GET /system/prerequisites` → `api_key_configured || activeProviderReady`

## Phase 6 — Gateway via CLI

In `GatewayAnalyze`: `if Provider().Name() != "openrouter"` → `RunOneShot` with haiku-tier model, same gateway prompt + "Respond with ONLY a JSON object" instruction, no tools/MCP. Existing brace-extraction fallback parsing (gateway.go:136-147) tolerates messy output. OpenRouter path untouched. Tradeoffs: gateway todo tools unavailable on CLI; routing latency ~3-6s (CLI cold start) — note in docs.

## Phase 7 — Frontend

- `Settings.tsx`: "LLM Provider" card above API-key section — 3 radio cards (OpenRouter / Claude Code / Codex) with live status badges from `GET /settings/llm-provider`; CLI selected → OpenRouter key field relabeled "optional — used for image generation only"; model dropdowns refetch on provider change. Lucide icons, `var(--op-*)` tokens.
- `Setup.tsx`: API-key step detects CLIs → "Use your Claude Max / ChatGPT subscription instead" buttons that set provider and skip key (mirrors env-var skip ~line 184).
- `useOpenRouterBalance.ts` + `Header.tsx`: handle `{provider, subscription:true}` — suppress polling, show provider chip.
- All API calls via `lib/api.ts` wrapper.

## Phase 8 — Degradation & errors

- `image_tool.go` error string → "Image generation requires an OpenRouter API key (Settings → API)".
- Typed errors `ErrProviderUnavailable` / `ErrProviderUnauthenticated` (`internal/llm/errors.go`) with fix hints ("run `claude` and log in" / "run `codex login`"); existing callers surface err.Error() into chat.
- Shutdown: contexts already flow; verify spawn's agentCtx cancellation kills process groups.

## Verification

1. `just quality` (lint + vet + test + build).
2. Unit tests: table-driven JSONL parser fixtures (record real `claude -p --output-format stream-json --verbose` and `codex exec --json` output first), tier mapping, MCP tools/list+call round-trip via httptest. Fake-CLI shell script via injected binPath for deterministic CI.
3. Manual on :41295:
   - (a) provider=openrouter regression: chat, routing, builder spawn, image gen unchanged
   - (b) switch to claude-code: agent saves memory + creates todo (MCP bridge + attribution), widget rendering, multi-turn in same thread (--resume), delete session row mid-thread (replay fallback)
   - (c) same for codex
   - (d) remove `claude` from PATH → chat error message + Settings badge
   - (e) fresh-DB Setup choosing CLI path, no OpenRouter key: image tool absent, header chip shown

## Risks (ordered)

1. **Codex HTTP-MCP support** is version-dependent → probe version; fallback: launch codex without MCP + prompt note, or 30-line `openpaw mcp-stdio <token>` stdio↔HTTP proxy subcommand. Build only if needed.
2. **Codex JSONL shapes** → tolerant parser + real fixtures recorded in Phase 4.
3. **Resume edge cases** (flag combos with --resume) → history-replay fallback always available; test explicitly.
4. **Gateway latency** (CLI cold start per message) → acceptable v1; possible future bypass for single-agent threads.
5. **Concurrency vs subscription rate limits** → semaphore (3); ensure delegate fan-out doesn't deadlock (only the exec holds a slot).

## Critical files

- `internal/llm/agent_loop.go`, `internal/llm/types.go`, `internal/llm/models.go`
- `internal/agents/gateway.go`, `internal/agents/manager.go`, `internal/agents/spawn.go`, `internal/agents/delegate.go`
- `internal/handlers/settings.go`, `internal/handlers/system.go`, `internal/handlers/auth.go`
- `cmd/openpaw/main.go`, `internal/server/server.go`
- `web/frontend/src/pages/Settings.tsx`, `Setup.tsx`, `hooks/useOpenRouterBalance.ts`, `components/Header.tsx`
