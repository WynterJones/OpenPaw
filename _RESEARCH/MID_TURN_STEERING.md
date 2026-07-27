# Research: Mid-Turn Steering

> Can a user message sent WHILE an agent is working be picked up during the current run,
> rather than only after it finishes? Answered per-engine (Claude Code CLI, Codex CLI,
> OpenRouter) and mapped onto OpenPaw's actual code paths with file:line hooks.

**Created:** 2026-07-27
**Last Researched:** 2026-07-27
**Status:** Fresh
**Confidence:** High (all three engines' capabilities) / Medium (Codex rollout durability under
an abrupt kill — flagged as needing an empirical test in §3.4)

---

## Summary

**Mid-turn steering is achievable on all three engines, but "mid-turn" never means "mid-inference".**
No engine — not even Claude Code itself — injects tokens into a Claude/GPT HTTP request that is
already in flight. What Claude Code actually does, and what OpenPaw should copy, is:

1. Accept the new message at any time into a **pending-steer slot**.
2. Deliver it at the **next iteration boundary of the agent loop** — after the current
   assistant message and its tool results are complete, before the next inference call.
3. Optionally offer an **interrupt** that forces that boundary to arrive immediately
   (abandoning the in-flight inference but keeping the work already done).

That boundary exists in all three OpenPaw engines, which makes a single application-level
design viable. The engine-specific work is only about *where* the boundary lives: inside
OpenPaw's own Go loop (OpenRouter), or inside a shelled-out CLI process (Claude Code, Codex).

| Engine | Verdict | Mechanism |
|--------|---------|-----------|
| **OpenRouter** (default) | ✅ **Possible** — cleanest of the three | Not at the protocol level (stateless HTTP). Application level: inject a `user` message between iterations of `RunAgentLoop`. OpenPaw owns the loop, so this is fully under our control. |
| **Claude Code CLI** | ✅ **Possible-with-caveats** | Two routes. (a) *Documented, cheap:* interrupt + `--resume <session_id>` with the correction as the new prompt — the session transcript preserves all completed work. (b) *Partially documented, richer:* switch to `--input-format stream-json` and hold stdin open, writing additional `{"type":"user",…}` lines. Queued at the CLI's own turn boundary. Envelope shape is documented only via the SDK types, not the CLI reference. |
| **Codex CLI** | ✅ **Possible — best-supported of the three** | `codex exec` (what OpenPaw uses) is one-shot: ❌. But `codex app-server` has a documented, purpose-built **`turn/steer`** JSON-RPC method that "appends user input to the currently in-flight turn without creating a new turn id". Requires swapping the provider transport from `exec` to the JSON-RPC app-server. |

**Recommendation — three phases:**

1. **Build the application-level "pending steer" plumbing once** (endpoint + `threadSteers` map +
   `AgentConfig.Steer` channel), and implement it natively for **OpenRouter** in
   `internal/llm/agent_loop.go`. That covers OpenPaw's default engine in roughly a day. CLI engines
   return `409` and fall back to today's queue — nothing regresses.
2. **Claude Code: interrupt + resume-with-correction.** No new protocol, reuses the existing
   `SessionStore` resume machinery at `internal/providers/claude.go:134-154`, and Anthropic
   documents that resume restores the full history including tool calls and results — so nothing
   completed is lost.
3. **Codex: migrate to `codex app-server` and call `turn/steer`.** This is the only true in-turn
   steer available anywhere, but it means rewriting the provider's transport.

Keep the existing frontend queue as the default. Steering costs tokens and mutates a running turn,
so it should be an explicit action (a "send now" button / `Cmd+Enter`), not what Enter silently
does.

---

## 1. The distinction that matters: interrupt ≠ steer ≠ queue

These are three different things and the docs treat them separately. Conflating them is the main
way this feature gets designed wrong.

| Term | What it does | Cost of the work already done |
|------|--------------|-------------------------------|
| **Queue** | Message is held and delivered when the current turn ends naturally. | Nothing lost. But you wait for the whole turn. |
| **Steer** | Message is delivered at the next *iteration* boundary inside the turn (after the current tool call), without ending the turn. | Nothing lost. This is what the user asked for. |
| **Interrupt** | The current inference/tool call is aborted. | The in-flight inference is lost; everything completed before it is kept. |

Claude Code's own UI implements all three:

> `Esc` — **Interrupt Claude, or close a dialog.** "Stop the current response or tool call
> mid-turn so you can redirect. **Claude keeps the work done so far.**"
> — [interactive-mode](https://code.claude.com/docs/en/interactive-mode) (fetched 2026-07-27)

> `/btw` — "**Available while Claude is working**: you can run `/btw` even while Claude is
> processing a response. The side question runs independently and doesn't interrupt the main turn."
> — [interactive-mode](https://code.claude.com/docs/en/interactive-mode), §"Side questions with /btw"

DOCUMENTED. The `Esc` wording — *"mid-turn so you can redirect"* and *"keeps the work done so far"* —
is the single most useful sentence in this whole research: it is Anthropic describing the
interrupt-then-redirect pattern as the intended steering flow, not mid-inference injection.

---

## 2. Engine 1 — Claude Code CLI + Agent SDK

### 2.1 Verdict

**Possible-with-caveats.** Two viable mechanisms, in increasing order of implementation cost.

### 2.2 What is DOCUMENTED

**a) Streaming input mode exists and is the recommended mode.**

> "Streaming input mode is the **preferred** way to use the Claude Agent SDK… It allows the agent
> to operate as a long lived process that takes in user input, **handles interruptions**, surfaces
> permission requests, and handles session management."
>
> Benefits list includes: "**Queued Messages** — Send multiple messages that process sequentially,
> with ability to interrupt."
>
> Single-message input mode explicitly does **not** support: "Dynamic message queueing",
> "Real-time interruption".
>
> — [Streaming Input](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode) (fetched 2026-07-27)

The page's sequence diagram literally shows `App->>Agent: Queue Message 3` followed by
`App->>Agent: Interrupt/Cancel`, with `Note over App,Agent: Session stays alive`.

**b) The CLI flags for it.**

> `--input-format` — "Specify input format for print mode (options: `text`, `stream-json`)"
>
> `--replay-user-messages` — "Re-emit user messages from stdin back on stdout for acknowledgment.
> Requires `--input-format stream-json` and `--output-format stream-json`"
>
> "**Key note on streaming input:** The `--input-format stream-json` option enables accepting
> stream-json formatted messages from stdin in print mode, particularly useful with
> `--replay-user-messages` for acknowledgment of **multiple interactive messages**."
>
> — [CLI reference](https://code.claude.com/docs/en/cli-reference) (fetched 2026-07-27)

**c) The SDK proves an in-CLI user-message queue exists.**

```typescript
interface Query extends AsyncGenerator<SDKMessage, void> {
  interrupt(): Promise<SDKControlInterruptResponse | undefined>;
  streamInput(stream: AsyncIterable<SDKUserMessage>): Promise<void>;
  setModel(model?: string): Promise<void>;
  setPermissionMode(mode: PermissionMode): Promise<void>;
  applyFlagSettings(settings: {...}): Promise<void>;
  stopTask(taskId: string): Promise<void>;
  close(): void;
  // …
}

type SDKControlInterruptResponse = { still_queued: string[] };
```

> `interrupt()` — "Interrupts the query. **Only available in streaming input mode.** When the CLI
> advertises the `interrupt_receipt_v1` capability in `SDKSystemMessage.capabilities`, resolves
> with an `SDKControlInterruptResponse` **listing the queued messages that survive the interrupt**.
> Resolves `undefined` on CLIs before v2.1.205."
>
> `streamInput(stream)` — "Stream input messages to the query for multi-turn conversations."
>
> — [Agent SDK TypeScript reference](https://code.claude.com/docs/en/agent-sdk/typescript) (fetched 2026-07-27)

The existence of `still_queued: string[]` is decisive: the CLI maintains a queue of user messages
received *while a turn is running*, and that queue survives an interrupt. That is exactly the
steering primitive.

**d) The changelog confirms mid-work messages are a real, delivered thing.**

> **v2.1.203** — "Fixed a message sent **while Claude was working** being silently lost when the
> turn ended at the `--max-turns` limit"
>
> **v2.1.208** — "Fixed stream-json input killing the session on blank CRLF or whitespace-only
> lines from Windows-style SDK hosts"
>
> — [claude-code CHANGELOG.md](https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md) (fetched 2026-07-27)

**e) Session resume is lossless.**

> "Sessions are saved **continuously** to local transcript files as you work…"
>
> "**What a resumed session restores** — Conversation history: **the full history, including tool
> calls and results.**"
>
> — [Manage sessions](https://code.claude.com/docs/en/sessions) (fetched 2026-07-27)

This is what makes the cheap interrupt+resume route safe: killing the process mid-turn does not
throw away completed tool calls.

**f) SIGTERM behaviour is defined.**

> "If you stop a `claude -p` run with SIGTERM… Claude Code **aborts the in-progress turn**,
> terminates the process tree of any running Bash command, runs `SessionEnd` hooks, and exits with
> code 143."
>
> — [Run Claude Code programmatically](https://code.claude.com/docs/en/headless) (fetched 2026-07-27)

### 2.3 What is INFERRED (not in the CLI reference)

- **The stdin envelope.** The CLI reference never shows it. It is inferable from the SDK's
  `SDKUserMessage` examples on the Streaming Input page:
  ```json
  {"type":"user","message":{"role":"user","content":"Focus on the auth layer"},"parent_tool_use_id":null}
  ```
  one JSON object per line. There is an open issue tracking exactly this gap:
  [anthropics/claude-code#24594](https://github.com/anthropics/claude-code/issues/24594) —
  "`--input-format stream-json` usage is undocumented beyond the CLI flags table". INFERRED.
- **Whether a message written to stdin mid-turn is delivered at the next *tool* boundary or only
  at the next *turn* boundary.** The docs say "process sequentially" and the changelog says a
  message sent while working is delivered "when the turn ended". Read together, the most defensible
  reading is: **next turn boundary, not next tool-call boundary**, unless you also call
  `interrupt()` to force the boundary early. INFERRED — treat "steer lands within seconds" as
  false and "steer lands when the current turn wraps, or immediately on interrupt" as true.
- **Whether `-p` + `--input-format stream-json` keeps the process alive past the first `result`
  line.** The streaming-input docs describe a "long lived process", which implies yes (until stdin
  closes). OpenPaw's current reader treats the first `result` as final
  (`internal/providers/claude.go:308-314`), so this would need to change. INFERRED.
- **Control-protocol envelopes on stdin** (`{"type":"control_request","request":{"subtype":"interrupt"}}`)
  exist in the SDK transport but are not in any Anthropic-published CLI doc. Community
  reverse-engineering: [Roasbeef/claude-agent-sdk-go docs/cli-protocol.md](https://github.com/Roasbeef/claude-agent-sdk-go/blob/main/docs/cli-protocol.md).
  INFERRED — do not build on this without runtime feature-detection via the `capabilities` array on
  the `system/init` event (documented in [headless](https://code.claude.com/docs/en/headless):
  "an optional `capabilities` array of strings naming the protocol behaviors this Claude Code
  version implements, such as `interrupt_receipt_v1`. Check it to feature-detect instead of
  comparing version strings").

### 2.4 Python SDK note

`ClaudeSDKClient.query()` accepts an `AsyncIterable[dict]`, so streaming input exists in Python
too. But it is **strictly sequential**: you must drain `receive_response()` before the next
`query()`.

> "**Buffer behavior after interrupt:** `interrupt()` sends a stop signal but does not clear the
> message buffer… **You must drain them with `receive_response()` before reading the response to a
> new query.**"
> — [Agent SDK Python reference](https://code.claude.com/docs/en/agent-sdk/python) (fetched 2026-07-27)

Not relevant to OpenPaw (Go, shells out to the CLI) but worth knowing: even Anthropic's own SDKs do
not offer true concurrent mid-inference injection.

---

## 3. Engine 2 — Codex CLI

### 3.1 Verdict

- **`codex exec` (what OpenPaw uses today): ❌ not possible.** One-shot. The prompt is read from
  stdin (or argv), stdin is consumed, the process runs to completion. No documented flag accepts a
  second message.
- **`codex app-server` (JSON-RPC): ✅ possible, and it is the only engine of the three with a
  *first-class, purpose-built, documented* steering method.**

**This is the headline finding of the whole report.** OpenAI shipped exactly the primitive this
feature needs, named exactly what you'd name it.

### 3.2 `turn/steer` — DOCUMENTED

`codex app-server` is a long-lived bidirectional JSON-RPC process (stdio by default,
`--listen ws://IP:PORT` optionally) hosting Codex threads. Its protocol is organised as
**Thread → Turn → Items**.

> **`turn/steer`** — "**append user input to the currently in-flight turn without creating a new
> turn id**". Appends user input to an active in-flight turn without creating a new turn. Requires
> a matching `expectedTurnId`. Does **not** emit a new `turn/started` notification.
>
> — [Codex App Server docs](https://learn.chatgpt.com/docs/app-server) (fetched 2026-07-27;
> `developers.openai.com/codex/app-server` 308-redirects here)

Request/response shape (corroborated by
[codex-rs/app-server/README.md](https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md)):

| Method | Params | Returns |
|--------|--------|---------|
| `thread/start` | `model`, `cwd`, `approvalPolicy`, `personality` | thread `{id, preview, …}` |
| `thread/resume` | `threadId` + optional overrides | thread object; emits `thread/started` |
| `thread/fork` | `threadId`, optional `lastTurnId` | new thread id |
| `turn/start` | `threadId`, `input[]` (text/images/skills), optional `clientUserMessageId` | `{turn: {id, status, items, error}}` |
| **`turn/steer`** | **`threadId`, `expectedTurnId` (required), `input[]`, optional `clientUserMessageId`** | **`{turnId: "turn_456"}`** |
| `turn/interrupt` | `threadId`, `turnId` | `{}`; emits `turn/completed` with `status: "interrupted"` |

Streaming notifications on stdout: `item/started`, `item/completed`,
`item/agentMessage/delta`, `turn/started`, `turn/completed`.

Schema is machine-generatable, which removes the guesswork entirely:

```bash
codex app-server generate-ts --out ./schema           # TypeScript definitions
codex app-server generate-json-schema --out ./schema  # JSON Schema bundle
```

**Minimum version: `rust-v0.99.0`** (published 2026-02-11) — release notes:
"#10821 feat(app-server): turn/steer API"
([rust-v0.99.0](https://github.com/openai/codex/releases/tag/rust-v0.99.0)). Note this is
`codex app-server`, **not** `codex mcp-server` — the MCP surface does not expose it. OpenPaw's
`probeState` (`internal/providers/shared.go:39`) already captures `codex --version`, so this can be
feature-gated without new machinery.

### 3.2b Codex's own TUI: `Enter` = steer, `Tab` = queue — DOCUMENTED

Codex resolved the same UX question OpenPaw is facing, and landed on the *opposite* default from
what §7.3 below recommends. Worth knowing before choosing:

> "**Steer mode is now stable and enabled by default**, so `Enter` sends immediately during running
> tasks while `Tab` explicitly queues follow-up input. (#10690)"
> — [rust-v0.98.0](https://github.com/openai/codex/releases/tag/rust-v0.98.0), published 2026-02-05

The implementing PR spells out both modes: with steer enabled (the new default) `Enter` submits
immediately even when a task is running and `Tab` queues; with steer disabled (old behaviour)
`Enter` queues. [rust-v0.106.0](https://github.com/openai/codex/releases/tag/rust-v0.106.0)
(2026-02-26) then removed the toggle entirely — "standardized on the always-on steer path in the
TUI composer (#12026)". So in current Codex, steer-on-Enter is non-configurable.

Note the ordering: TUI steering (v0.98.0) **predates** the `turn/steer` RPC (v0.99.0) by a week —
the TUI first steered through the in-process core API, and the RPC was extracted afterwards.

### 3.2c Timing: `turn/steer` does NOT preempt the in-flight model response

DOCUMENTED via the repo's own committed test snapshot. Codex core has a test named exactly
`user_input_does_not_preempt_after_reasoning_item`
(`codex-rs/core/tests/suite/pending_input.rs`), whose snapshot shows the steered message appearing
only in the **second** model request, after the tool-call output:

```
## Second request
02:message/user:first prompt
03:reasoning  04:function_call/shell  05:message/assistant
06:function_call_output
07:message/user:second prompt     <-- the steer lands HERE
```

**Floor latency for a Codex steer is therefore "current model response + current tool call"** —
identical to the boundary the OpenRouter design uses in §6.3. `Esc` (interrupt) remains the only way
to beat that boundary, and it does so by interrupting and resubmitting, not by injecting.

This is a strong independent confirmation of the report's central claim: even the engine with a
first-class `turn/steer` primitive delivers at an agent-loop boundary, never mid-inference.

Codex also falls back to queue-only (no steer) in several states worth mirroring: explicit `Tab`,
slash-commands and `!` shell prompts, during a manual `/compact`, while a user shell command runs,
during review, and before the session is configured. OpenPaw has the same shape of exclusions —
see the auto-compaction risk in §8.2.

### 3.3 What this costs OpenPaw

`internal/providers/codex.go` would have to change from "spawn `codex exec`, read JSONL, wait"
(`codex.go:147-283`) to "keep one `codex app-server` process, speak JSON-RPC, map threads to
OpenPaw threads". That is a rewrite of the provider, not a patch:

- process lifecycle moves from per-turn to long-lived (breaks the `maxConcurrentCLI` semaphore
  model at `internal/providers/shared.go:21`, which is per-*process*)
- `runJSONL` (`shared.go:92`) is unusable — it's line-in/line-out with a one-shot stdin
- the MCP bridge is currently passed via `-c mcp_servers.openpaw.url=…` at `codex.go:176`; it would
  move into `thread/start` config
- OpenPaw must track the live `turnId` per thread to satisfy `expectedTurnId`

### 3.4 The cheap Codex route (consistent with §6.4)

`codex exec resume <thread-id>` already works in OpenPaw (`codex.go:149-151`), and the thread id
arrives early on the `thread.started` event (`codex.go:225-230`). So the same interrupt+resume
pattern as Claude Code applies without touching the transport.

**Caveat — INFERRED, and weaker than the Claude Code equivalent.** Anthropic explicitly documents
that sessions are "saved continuously" and that resume restores "the full history, including tool
calls and results". I found **no equivalent guarantee** for Codex rollout files under an abrupt
SIGTERM/SIGKILL mid-turn. If Codex only flushes at turn completion, an interrupt would lose the
partial turn — which would make interrupt+resume on Codex *worse* than the current queue. **Verify
empirically before shipping** (kill a `codex exec` mid-tool-call, then `codex exec resume` and check
whether the completed tool calls are present).

---

## 4. Engine 3 — OpenRouter

### 4.1 Verdict at the protocol level: **not possible, as expected**

`POST /api/v1/chat/completions` is a single stateless HTTP request. Once the request body is sent,
the only control the client has is to **abort the connection**.

> "Streaming requests can be cancelled by aborting the connection. For supported providers, this
> immediately stops model processing and billing."
>
> Cancellation supported: OpenAI, Azure, Anthropic, Fireworks, DeepInfra, Together, Cohere,
> Hyperbolic, XAI, Cloudflare, DeepSeek, and others.
> Not supported: AWS Bedrock, Groq, Google, Google AI Studio, Minimax, HuggingFace, Replicate,
> Perplexity, Mistral, AI21, SambaNova, and others.
>
> — [OpenRouter API Streaming](https://openrouter.ai/docs/api-reference/streaming) (fetched 2026-07-27)

DOCUMENTED: the page covers *only* cancellation. There is no mechanism, parameter, or header for
adding input to an in-flight generation. The equivalent OpenRouter help-centre article
("How do I cancel a streaming request, and which providers stop billing when I do?") likewise
describes `AbortController` and billing only. This is a property of the OpenAI-compatible
chat-completions shape generally, not an OpenRouter limitation.

### 4.2 Verdict at the application level: **possible, and this is the good news**

OpenPaw owns the agent loop for OpenRouter. `internal/llm/agent_loop.go:136` is a plain Go `for`
loop; between iterations we can append anything we like to `messages` before the next
`doStreamRequest`. This is strictly *more* capable than the CLI engines, where the loop lives
inside someone else's process.

Practical consequence: **OpenRouter — OpenPaw's default engine — is the easiest of the three to
steer, despite having the least capable protocol.**

### 4.3 Cross-check: no inference API anywhere supports mid-request injection

This is not an OpenRouter quirk. Checked against OpenAI's own published OpenAPI spec
([`openai-openapi`](https://github.com/openai/openai-openapi), `info.version: 2.3.0`), since the
Responses API is what Codex itself runs on:

- Complete surface for a response: `POST /responses`, `GET|DELETE /responses/{id}`,
  `POST /responses/{id}/cancel`, `GET` (only) `/responses/{id}/input_items`,
  `/responses/input_tokens`, `/responses/compact`. **There is no append, patch, or steer on a live
  response.**
- Grepping the full spec for `steer`, `interject`, `mid-turn`, `in-flight`: **zero matches**.
- Cancellation is *more* restricted than OpenRouter's: "Only responses created with the `background`
  parameter set to `true` can be cancelled." For a synchronous response the documented remedy is to
  terminate the connection.
- `starting_after` is a GET query parameter — a replay cursor over `sequence_number`. Reattaching to
  a stream does not touch generation.
- Even the Realtime API *cancels* rather than injects: "The server will automatically cancel any
  in-progress model response."

**Conclusion:** every steering feature in every agent CLI — Claude Code's queue, Codex's
`turn/steer` — is necessarily a *client-side agent-loop* feature. Which is exactly why OpenPaw can
implement one at the same layer, and why the floor latency is always "current inference + current
tool call" regardless of engine.

---

## 5. How OpenPaw works today (the map)

### 5.1 A user message entering a turn

| Step | Location |
|------|----------|
| Frontend composer → `POST /chat/threads/{id}/messages` | `web/frontend/src/pages/Chat.tsx:1354` |
| Route registration | `internal/server/server.go:417` |
| Handler saves the row, then **fires routing in a goroutine and returns 201 immediately** | `internal/handlers/chat.go:501`, insert at `:538`, `go h.handleAgentRouting(...)` at `:572` |
| Routing lifecycle; registers the cancel func | `internal/handlers/chat_routing.go:181-188` |
| Role chat → provider | `internal/handlers/chat_routing.go:620` → `internal/agents/gateway.go:667` |

The HTTP request returns before the agent does any work. There is no request-scoped context tying a
message to the turn — everything is coordinated through `threadID`. **That is convenient: a steer
endpoint can address the running turn purely by thread ID.**

### 5.2 The existing stop mechanism

- `h.threadCancels sync.Map // map[threadID]context.CancelFunc` — `internal/handlers/chat.go:36`
- Stored for the whole routing lifecycle — `internal/handlers/chat_routing.go:184`, deleted at `:187`
- `StopThread` — `internal/handlers/chat.go:1314`:
  - snapshots the half-written reply from `GetStreamState` *before* cancelling (`:1324-1327`)
  - `LoadAndDelete` + `cancel()` (`:1330-1335`)
  - stops any builder agent (`:1345-1352`)
  - saves the partial reply rather than discarding it (`:1357-1366`)
- Route: `internal/server/server.go:428`
- Frontend: `stopThread()` at `web/frontend/src/pages/Chat.tsx:1467`, wired to the stop button at `:2229`

`threadCancels` is also the source of truth for "is this thread active" — `ThreadStatus`
(`chat.go:353`) and `ActiveThreads` (`chat.go:392`).

**This is a good foundation.** A steer map sits directly alongside it with the same lifecycle.

### 5.3 The OpenRouter agent loop

`internal/llm/agent_loop.go`:

```
:61   func (c *Client) RunAgentLoop(ctx, cfg AgentConfig, userMessage string)
:88-95    builds []ChatMessage = system + cfg.History + user
:136  for numTurns < maxTurns {
:137-148    select { case <-ctx.Done(): return "cancelled" ; default: }   <-- INJECTION POINT A
:153        truncateOldToolResults()
:165        resp, err := c.doStreamRequest(ctx, reqBody)
:173        streamRes, streamErr := processSSEStream(resp.Body, emit, cfg.Model)
:199        messages = append(messages, assistantMsg)
:202        if streamRes.FinishReason != "tool_calls" || len(ToolCalls) == 0 { break }   <-- INJECTION POINT B
:211-280    for each tool call: emit tool_start, executor.Execute, emit tool_end,
            append ChatMessage{Role:"tool", ToolCallID: tc.ID}
:281  }                                                                  <-- INJECTION POINT C
```

Note `ctx` is already checked at the top of every iteration (`:137`) — the loop **already has the
exact structure a steer needs**. Streaming is consumed synchronously inside `processSSEStream`
(`internal/llm/streaming.go:45`), which is where a "steer forces an early boundary" variant would
have to abort the body read.

### 5.4 The CLI providers are strictly one-shot

`internal/providers/shared.go:92`:

```go
func runJSONL(cmd *exec.Cmd, stdin string, onLine func(line []byte)) (string, error) {
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)   // :94  <-- stdin is a fixed string, closed on EOF
	}
	...
	for scanner.Scan() { onLine(scanner.Bytes()) }   // :113-119
	waitErr := cmd.Wait()                             // :122
}
```

**`strings.NewReader` is the whole story: stdin is closed as soon as the prompt is consumed.**
Neither CLI provider can receive a second message today.

**Claude** — `internal/providers/claude.go:177` `runOnce`:
- args at `:178`: `-p --output-format stream-json --verbose --strict-mcp-config` — note
  **no `--input-format`**, so stdin is plain text.
- resume: `--resume <id>` at `:191`, session id captured from the `system/init` line at `:253-261`
  and from `result` at `:311-313`, persisted via `p.store.PutProviderSession` at `:151-153`.
- a resume failure already falls back to a fresh full-history replay — `:140-146`. Good precedent.
- `maxConcurrentCLI = 3` semaphore at `internal/providers/shared.go:21`, acquired at `claude.go:129`.

**Codex** — `internal/providers/codex.go:147` `runOnce`:
- args at `:148-193`: `exec [resume <id>] --json --skip-git-repo-check -m <model> --sandbox … -`
- the trailing `-` at `:193` makes codex read the prompt from stdin — same one-shot reader.
- no system-prompt flag, so system + history are embedded in the prompt at `:179-190`.

### 5.5 The frontend queue (already built, and correct)

`web/frontend/src/pages/Chat.tsx`:
- `const busy = sending || thinking || isStreaming;` — `:661`
- `takeComposerSnapshot()` empties the composer at Enter regardless of busy — `:1243`
- `sendMessage()` — `:1368`: `if (busy) { setQueue(prev => [...prev, msg]); return; }`
- drain-one-per-turn effect — `:1386-1392`
- queued-message chips above the composer — `:2019-2050`, with per-item remove at `:2047`
- dedicated queue button — `:2215-2223`

This is entirely client-side. The backend has no idea a queue exists. Steering therefore does **not**
conflict with it — it's a second, explicit destination for a composer snapshot.

### 5.6 The WebSocket layer

- `h.agentManager.Broadcast(msgType, payload)` — `internal/agents/manager.go:133`
- `broadcastStatus(threadID, status, message)` → `agent_status` / `models.WSAgentStatus` —
  `internal/handlers/chat_routing.go:145-151`, type at `internal/models/ws_events.go:7`
- Frontend consumers: `useWebSocket` in `web/frontend/src/lib/useWebSocket.ts`, e.g.
  `web/frontend/src/hooks/useCompanionActivity.ts:55-85`

Broadcast is a fire-and-hose to all clients keyed by `thread_id` in the payload — adding a
`steer_accepted` status needs no new infrastructure.

---

## 6. Proposed design (works across all three engines)

### 6.1 The shape

One new concept: a **pending steer** attached to a running thread. Delivered at the next agent-loop
iteration boundary. Never delivered mid-inference, never delivered between an assistant
`tool_calls` message and its `tool` results.

```
POST /chat/threads/{id}/steer   {content, agent_role_slug}
        │
        ├─ 409 if no turn is running  →  frontend falls back to today's queue
        │
        ├─ persist as a normal chat_messages row (role=user)   [transcript honesty]
        ├─ broadcast agent_status "steered"
        └─ push onto h.threadSteers[threadID]  (buffered chan string, cap 4)
                     │
                     ▼
          consumed by the running turn at the next iteration boundary
```

### 6.2 Backend plumbing (engine-agnostic)

| Change | Where |
|--------|-------|
| `threadSteers sync.Map // map[threadID]chan string` next to `threadCancels` | `internal/handlers/chat.go:36` |
| create/delete the channel with the same lifecycle as the cancel func | `internal/handlers/chat_routing.go:184` and `:187` |
| `SteerThread` handler (mirrors `StopThread`'s structure) | new, next to `internal/handlers/chat.go:1314` |
| `r.Post("/threads/{id}/steer", chatHandler.SteerThread)` | `internal/server/server.go:429` (beside the existing `/stop` at `:428`) |
| `Steer <-chan string` field on `AgentConfig` | `internal/llm/agent_loop.go:17-43` (next to `OnEvent` at `:36`) |
| populate `cfg.Steer` before dispatch | `internal/agents/gateway.go:666`, immediately before `provider.RunAgentLoop(ctx, cfg, userMessage)` at `:667` |
| expose `"steerable": true/false` on thread status so the UI knows | `internal/handlers/chat.go:319` `ThreadStatus` |

`AgentConfig` already carries per-run callbacks (`OnEvent func(StreamEvent)` at `agent_loop.go:36`),
so adding a channel is consistent with the existing design rather than a new pattern.

### 6.3 OpenRouter implementation — the reference implementation

Insert at **injection point C** (`internal/llm/agent_loop.go:280`, immediately after the tool-result
append loop closes and before the `for` body ends) and at **injection point B**
(`agent_loop.go:202`, the `break` on a finished turn):

```go
// after the tool-result loop, before looping back — agent_loop.go:~280
drainSteer := func() bool {
	injected := false
	for {
		select {
		case s := <-cfg.Steer:
			if strings.TrimSpace(s) == "" { continue }
			messages = append(messages, ChatMessage{
				Role:    "user",
				Content: "[The user sent this while you were working — adjust course now]\n" + s,
			})
			emit(StreamEvent{Type: EventSteer, Text: s}) // new const, add beside EventInit at internal/llm/types.go:33
			injected = true
		default:
			return injected
		}
	}
}
```

Then at `agent_loop.go:202`, change

```go
if streamRes.FinishReason != "tool_calls" || len(streamRes.ToolCalls) == 0 {
	break
}
```

to

```go
if streamRes.FinishReason != "tool_calls" || len(streamRes.ToolCalls) == 0 {
	if drainSteer() {
		continue   // steer arrived just as the turn was ending — answer it in the same turn
	}
	break
}
```

Two properties this buys:

1. **Steer lands after the current tool cycle**, so `messages` is always in a valid
   assistant-tool_calls → tool-results → user order. No provider will reject it.
2. **A steer that races the end of the turn is not lost** — it extends the turn instead. This is
   precisely the bug Anthropic fixed in Claude Code v2.1.203, worth avoiding by construction.

Latency: bounded by one tool call. In the motivating case ("20 seconds in, correct a detail") the
correction lands at the end of whatever tool is currently running.

**Optional phase 2 — force the boundary.** To make a steer land *during* a long inference rather
than after it, `processSSEStream` (`internal/llm/streaming.go:45`) would need to stop reading early.
The clean way is to give the per-iteration request its own `context.WithCancel` derived from `ctx`,
and cancel it when a steer arrives — `doStreamRequest` (`internal/llm/client.go:168`) already takes
a ctx, and aborting the body read is exactly OpenRouter's documented cancellation path. The partial
assistant text from `processSSEStream` is still returned, so it can be appended before the steer.
Recommend deferring this; phase 1 is enough for the motivating case.

### 6.4 Claude Code implementation — route (a), interrupt + resume (RECOMMENDED)

No new protocol, no change to `runJSONL`, and it reuses machinery OpenPaw already has.

In `internal/providers/claude.go:139`, `RunAgentLoop` currently does one `runOnce`. Wrap it:

```go
for {
    result, sessionID, err := p.runOnce(runCtx, cfg, userMessage, resumeID)
    // ... existing resume-failure fallback at :140-146 unchanged ...
    if steer, ok := takeSteer(cfg.Steer); ok && sessionID != "" {
        p.store.PutProviderSession(...)          // persist the session we just learned
        resumeID   = sessionID                    // continue the SAME session
        userMessage = steer                       // the correction becomes the next prompt
        accumulate(result)                        // keep text/tokens/cost from the aborted leg
        continue
    }
    return result, nil
}
```

with `runCtx` being a `context.WithCancel(ctx)` that a steer-watcher goroutine cancels, so the CLI
is SIGKILLed (via `exec.CommandContext` at `claude.go:225`) the moment a steer arrives.

Why this is safe:
- **The session id is available early.** It arrives on the `system/init` line and is captured at
  `internal/providers/claude.go:253-260`, long before the turn finishes. So even a steer 2 seconds
  into a 5-minute turn has a session to resume.
- **Nothing completed is lost.** "Sessions are saved continuously… A resumed session restores…
  the full history, including tool calls and results." ([sessions docs](https://code.claude.com/docs/en/sessions))
- **The failure mode is already handled.** `claude.go:140-146` already falls back to a fresh
  full-history replay if resume fails.

Costs and caveats:
- One extra process spawn per steer, plus re-reading the session context (prompt-cache should
  absorb most of it).
- Re-acquires the `maxConcurrentCLI = 3` semaphore (`shared.go:21`, acquired at `claude.go:129`).
  Under fan-out this can *wait*. Fix: release-and-reacquire explicitly, or hold the slot across the
  whole steer loop rather than per-`runOnce`.
- `--resume` resolution is scoped to the project directory
  ([sessions docs](https://code.claude.com/docs/en/sessions): "session ID lookup is scoped to the
  current project directory and its git worktrees"). OpenPaw already pins a stable per-thread cwd
  via `resolveWorkDir` (`claude.go:76-92`), so this holds — **as long as the workspace dir doesn't
  change mid-thread.**
- `--mcp-config` is **not** restored on resume ([sessions docs](https://code.claude.com/docs/en/sessions):
  "If the session depended on `--mcp-config`… pass them again when you resume"). `runOnce` rebuilds
  the MCP config on every call (`claude.go:203-215`), so this is already correct — but note the
  bridge token changes, which is fine since `p.registry.Release` is deferred per call.
- The `result` line's `NumTokens`/cost are per-leg; the wrapper must sum them or the UI will
  under-report.

### 6.5 Claude Code implementation — route (b), live stdin (NOT recommended first)

Change `runJSONL` (`internal/providers/shared.go:92`) to expose `cmd.StdinPipe()` instead of
`strings.NewReader`, add `--input-format stream-json` to the args at `claude.go:178`, write the
first prompt as a JSON line, keep the pipe open, and write each steer as:

```json
{"type":"user","message":{"role":"user","content":"Focus on the auth layer"},"parent_tool_use_id":null}
```

Then close stdin when the turn is done. Add `--replay-user-messages` so OpenPaw can confirm each
steer was actually ingested rather than guessing.

Why not first: it changes the whole CLI lifecycle. `runOnce` currently treats the first `result`
line as terminal (`claude.go:308-314`); with a held-open stdin there is a `result` per turn, so the
reader must be rewritten to loop until *we* close stdin. Combined with an under-documented envelope
(issue #24594) and a known Windows CRLF crash fixed only in v2.1.208, that's a lot of surface area
for the same user-visible outcome as route (a). Revisit once Anthropic documents the stdin protocol.

### 6.6 Codex implementation

**Phase 1 — cheap, consistent with §6.4.** `codex exec resume <thread-id>` (`codex.go:149-151`)
with the correction as the new prompt, driven by the same `cfg.Steer` channel. Thread id is
available early from `thread.started` (`codex.go:225-230`). **Gate on the empirical check in §3.4** —
if a SIGTERMed `codex exec` loses its partial turn, do not ship this; return `409` and queue instead.

**Phase 2 — the right answer, eventually.** Migrate `CodexProvider` to `codex app-server`
(**v0.99.0+**; feature-gate off the version already captured by `probeState.probe` at
`internal/providers/shared.go:39`) and use `turn/steer` directly. This is the only engine where a
steer lands inside the running turn with no process restart and no lost work — though note per
§3.2c it still arrives at the next model request, after the current tool call:

```jsonc
// 1. once, at provider start
//    exec: codex app-server            (stdio JSON-RPC)
// 2. per OpenPaw thread
{"jsonrpc":"2.0","id":1,"method":"thread/start","params":{"model":"gpt-5.4","cwd":"<workspaceDir>","approvalPolicy":"never"}}
// 3. per user message
{"jsonrpc":"2.0","id":2,"method":"turn/start","params":{"threadId":"th_1","input":[{"type":"text","text":"..."}]}}
// 4. THE STEER — while turn is still running
{"jsonrpc":"2.0","id":3,"method":"turn/steer","params":{"threadId":"th_1","expectedTurnId":"turn_9","input":[{"type":"text","text":"actually, use sonnet-style headings"}]}}
//    -> {"turnId":"turn_9"}   (same turn id; no new turn/started notification)
```

OpenPaw must track the live `turnId` per thread (from the `turn/start` response and
`turn/started` notifications) to satisfy the required `expectedTurnId`. Generate the exact types
first with `codex app-server generate-ts --out ./schema` rather than hand-writing envelopes.

Scope: this is a provider rewrite (see §3.3), not a patch — the `runJSONL` helper
(`internal/providers/shared.go:92`) and the per-process `maxConcurrentCLI` semaphore (`shared.go:21`)
both assume one process per turn.

---

## 7. Frontend and transport changes

### 7.1 API

- `POST /api/v1/chat/threads/{id}/steer` — `internal/server/server.go:429`.
  `201` = accepted for in-flight delivery. `409` = no live turn, caller should queue instead.
- `GET /chat/threads/{id}/status` gains `"steerable": bool` — `internal/handlers/chat.go:319`.

### 7.2 WebSocket

New `agent_status` status value, no new event type needed
(`internal/models/ws_events.go:7`, broadcast via `broadcastStatus` at
`internal/handlers/chat_routing.go:145`):

```go
h.broadcastStatus(threadID, "steered", "Course correction sent…")
```

The frontend already switches on `agent_status` (`web/frontend/src/hooks/useCompanionActivity.ts:62`),
so this only needs a new case. **Important:** the steer must also arrive as a normal user message in
the transcript — the existing `message_saved` broadcast at `chat_routing.go:177` covers that if
`SteerThread` inserts the row the way `SendMessage` does at `chat.go:538`.

### 7.3 Chat.tsx

Keep the queue as the default. Add steering as an explicit second action — a steer costs tokens and
changes an in-flight turn, so it should not be what Enter silently does.

**Counterpoint worth weighing:** Codex went the other way and made steering the *default* —
`Enter` steers, `Tab` queues, non-configurable since v0.106.0 (§3.2b). The argument for their
choice is that steering is what a user typing mid-task almost always means. The argument for the
conservative default here is that OpenPaw's three engines have *different* steer reliability
(OpenRouter native, Claude Code via process restart, Codex gated on version), so silently steering
would behave inconsistently across engines in a way Codex never has to worry about. Revisit once
all three paths are native.

- `sendMessage()` at `:1368` — leave the `if (busy) → queue` default intact.
- Add `steerMessage()` beside `stopThread()` at `:1467`, POSTing to `/steer`, and on `409` falling
  through to `setQueue(...)` so nothing is ever dropped.
- Bind it to `Cmd/Ctrl+Enter` while `busy`, and add a "Send now" button beside the existing queue
  button at `:2215-2223`.
- Queued chips at `:2019-2050` gain a per-item "send now →" that calls `steerMessage`.
- Gate the affordance on `steerable` from `ThreadStatus`, so engines/paths that can't steer show
  only the queue and the UI never promises something the backend can't do.

---

## 8. Difficulty and risk

### 8.1 Effort

| Piece | Size | Notes |
|-------|------|-------|
| Backend plumbing (`threadSteers`, endpoint, route, `AgentConfig.Steer`) | **S** | ~150 lines, mirrors `StopThread` almost exactly |
| OpenRouter loop injection (`agent_loop.go`) | **S** | ~25 lines at points B and C |
| Frontend steer action + `steerable` gating | **S–M** | Chat.tsx is already the right shape |
| Claude Code interrupt+resume wrapper | **M** | ~120 lines; semaphore + cost accumulation are the fiddly bits |
| Codex `exec resume` wrapper | **M** | same pattern — **gated on the §3.4 durability check** |
| Claude Code live-stdin (`--input-format stream-json`) | **L** | rewrites the CLI provider lifecycle; defer |
| Codex `app-server` migration (`turn/steer`) | **L** | full provider rewrite, but the only true in-turn steer; best long-term target |
| Force-boundary mid-inference cancel (phase 2) | **M** | touches `streaming.go` + `client.go` |

A useful first cut — OpenRouter steering end-to-end, with the CLI engines returning `409` and
falling back to the existing queue — is roughly **one focused day**. That already covers OpenPaw's
default engine.

### 8.2 Risks

**Message-order corruption (OpenRouter).** Injecting a `user` message between an assistant
`tool_calls` message and its `tool` results produces a request most providers reject. *Mitigation:*
only drain at point C (after the whole tool-result loop at `agent_loop.go:280`) and point B. Never
inside the `for _, tc := range streamRes.ToolCalls` loop at `:211`.

**Partially-complete tool call on interrupt (CLI engines).** SIGKILL during a `Bash` tool leaves
the side effects (files written, commands run) but no `tool_result` in the transcript. Claude Code's
documented SIGTERM path says it "terminates the process tree of any running Bash command"
([headless](https://code.claude.com/docs/en/headless)) — so prefer SIGTERM over SIGKILL. Even so,
the resumed session may show a tool call with no result. *Mitigation:* prefix the steer prompt with
a note ("your previous tool call may not have completed — verify before continuing"), and prefer
draining at a natural boundary when the turn is nearly done.

**Race with streaming output.** `StopThread` already documents this hazard at
`internal/handlers/chat.go:1319-1321` — cancelling wakes the routing goroutine, which clears stream
state on the way out, so the partial text must be snapshotted *first*. A steer path that cancels a
sub-context hits the same race. *Mitigation:* reuse the same snapshot-before-cancel ordering, and
do not call `ClearStreamState` on a steer (unlike stop, the turn continues).

**Double-delivery / lost steer.** If the turn ends between the `409` check and the channel push, the
steer lands in a channel nobody reads. *Mitigation:* buffered channel + `handleAgentRouting`'s
deferred cleanup at `chat_routing.go:185-188` drains any leftovers and re-queues them as a fresh
`handleAgentRouting` call. The user must never silently lose a message.

**Cost attribution.** A steered turn spans multiple inference legs but produces one
`saveAssistantMessage` (`internal/handlers/chat.go:1190`, called from `chat_routing.go:651`).
Tokens/cost must be summed across legs or the UI under-reports. Same issue for `NumTurns`.

**Auto-compaction collision.** `handleAgentRouting` may auto-compact before routing
(`chat_routing.go:191-198`). A steer arriving during compaction has no turn to attach to — it should
`409` and queue.

**Session drift (Claude Code).** `--resume` is directory-scoped. If a thread's workspace changes
mid-conversation, the resume fails — but `claude.go:140-146` already handles that with a
full-history replay, so it degrades to "the steer becomes the next message", which is exactly
today's behaviour. Acceptable.

**Concurrency semaphore.** `maxConcurrentCLI = 3` (`shared.go:21`). A steer-driven re-spawn queues
behind other agents' CLI runs. Hold the slot across the steer loop rather than releasing between
legs.

**Multi-agent turns.** `handleMultiAgentResponse` (`chat_routing.go:234`) runs several agents
sequentially under one `threadCancels` entry. A steer has no obvious single addressee. *Mitigation:*
`409` for multi-agent turns in v1.

---

## 9. Documented vs inferred — quick index

| Claim | Status |
|-------|--------|
| Claude Code streaming input mode exists; queues messages; supports interrupt | **DOCUMENTED** — streaming-vs-single-mode |
| `--input-format stream-json`, `--replay-user-messages` flags exist | **DOCUMENTED** — cli-reference |
| CLI keeps a queue of user messages that survives interrupt (`still_queued`) | **DOCUMENTED** — agent-sdk/typescript |
| A message sent while Claude is working is delivered at turn end | **DOCUMENTED** — CHANGELOG v2.1.203 |
| `Esc` interrupts mid-turn and keeps completed work | **DOCUMENTED** — interactive-mode |
| Resume restores full history including tool calls and results | **DOCUMENTED** — sessions |
| `capabilities` array on `system/init` for feature detection | **DOCUMENTED** — headless |
| Exact stdin JSON envelope for `--input-format stream-json` | **INFERRED** from SDK `SDKUserMessage` examples; gap tracked in claude-code#24594 |
| Whether steers land at tool-boundary vs turn-boundary | **INFERRED** — turn boundary is the defensible reading |
| Whether `-p` + stream-json input survives past the first `result` line | **INFERRED** |
| stdin `control_request` envelopes (interrupt over the wire) | **INFERRED** — community reverse-engineering only |
| Codex `turn/steer` appends input to an in-flight turn; `expectedTurnId` required | **DOCUMENTED** — learn.chatgpt.com/docs/app-server + codex-rs/app-server/README.md |
| `turn/steer` requires codex-rs **v0.99.0+**, `app-server` (not `mcp-server`) | **DOCUMENTED** — rust-v0.99.0 release notes |
| Codex `turn/interrupt`, `thread/resume`, `thread/fork` exist | **DOCUMENTED** — same |
| Codex TUI: `Enter` = steer, `Tab` = queue, non-configurable since v0.106.0 | **DOCUMENTED** — rust-v0.98.0 / rust-v0.106.0 release notes |
| A Codex steer lands in the *next* model request, after the tool-call output — never mid-inference | **DOCUMENTED** — committed test snapshot `codex-rs/core/tests/suite/pending_input.rs`, test `user_input_does_not_preempt_after_reasoning_item` |
| `codex exec` headless has no steering equivalent | **DOCUMENTED by omission** — app-server docs make no mention of `exec` support |
| OpenAI Responses API exposes no append/patch/steer on a live response | **DOCUMENTED** — openai-openapi spec v2.3.0; zero matches for steer/interject/mid-turn/in-flight |
| Codex rollout files preserve a turn killed mid-flight | **UNVERIFIED — must test empirically** (§3.4) |
| OpenRouter cannot accept input mid-request | **DOCUMENTED by omission** — cancellation is the only in-flight control the API exposes |
| The application-level design in §6 | **INFERRED / designed here** — not from any doc |

---

## Sources

**Claude Code / Agent SDK** (all fetched 2026-07-27)
- https://code.claude.com/docs/en/cli-reference
- https://code.claude.com/docs/en/headless
- https://code.claude.com/docs/en/interactive-mode
- https://code.claude.com/docs/en/sessions
- https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode
- https://code.claude.com/docs/en/agent-sdk/streaming-output
- https://code.claude.com/docs/en/agent-sdk/typescript
- https://code.claude.com/docs/en/agent-sdk/python
- https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md
- https://github.com/anthropics/claude-code/issues/24594 (stdin protocol undocumented)
- https://github.com/anthropics/claude-code/issues/36001 (headless raw-mode crash)
- https://github.com/Roasbeef/claude-agent-sdk-go/blob/main/docs/cli-protocol.md (community, reverse-engineered)

**OpenRouter** (fetched 2026-07-27)
- https://openrouter.ai/docs/api-reference/streaming
- https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request

**Codex CLI** (fetched 2026-07-27)
- https://learn.chatgpt.com/docs/app-server (canonical; `https://developers.openai.com/codex/app-server` 308-redirects here)
- https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md
- https://openai.com/index/unlocking-the-codex-harness/ (background on why the App Server exists)
- https://developers.openai.com/codex/cli/reference (`codex exec` flags)
- https://github.com/openai/codex/releases/tag/rust-v0.99.0 (`turn/steer` API, #10821 — minimum version)
- https://github.com/openai/codex/releases/tag/rust-v0.98.0 (TUI steer mode default: Enter steers, Tab queues, #10690)
- https://github.com/openai/codex/releases/tag/rust-v0.106.0 (steer toggle removed, #12026)
- `codex-rs/core/tests/suite/pending_input.rs` — `user_input_does_not_preempt_after_reasoning_item`

**OpenAI Responses API** (fetched 2026-07-27)
- https://github.com/openai/openai-openapi — spec `info.version: 2.3.0`; no append/patch/steer on a live response

**Related OpenPaw research**
- `_RESEARCH/OPENAI_CODEX_CLI.md`
- `_RESEARCH/GEMINI_CLI.md`

---

## Research History

| Date | Researcher | Areas Updated |
|------|------------|---------------|
| 2026-07-27 | researcher agent | Initial research — per-engine steering verdicts, OpenPaw code mapping, application-level design |
| 2026-07-27 | researcher agent | Codex deep-dive — `turn/steer` version floor (v0.99.0), TUI Enter/Tab semantics, test-snapshot proof that steers land at the next model request, OpenAI Responses API negative confirmation |
