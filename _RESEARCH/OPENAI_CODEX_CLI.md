# Research: OpenAI Codex CLI

> Systematic research findings on OpenAI's Codex CLI tool, focusing on programmatic/non-interactive usage,
> streaming output, and structured output modes. Useful for understanding the competitive landscape and
> potentially integrating Codex as an alternative AI engine in OpenPaw.

**Created:** 2026-02-23
**Last Researched:** 2026-02-23
**Status:** Fresh
**Confidence:** High

---

## Summary

OpenAI's Codex CLI is an open-source coding agent that runs locally in the terminal. It has two implementations: a legacy TypeScript/Node.js version and a newer Rust implementation (now the primary one). For non-interactive/programmatic use, the Rust version provides `codex exec` (analogous to `claude -p` in Claude Code), with `--json` for JSONL streaming output and `--output-last-message` for capturing final results. The legacy TypeScript version used `-q`/`--quiet` for non-interactive mode. The tool supports piping prompts via stdin, structured output schemas, and rich event streaming.

---

## Official Documentation

| Resource | URL | Notes |
|----------|-----|-------|
| GitHub Repository | https://github.com/openai/codex | Open-source, Rust + TypeScript |
| Official Docs | https://developers.openai.com/codex | Primary documentation site |
| CLI Reference | https://developers.openai.com/codex/cli/reference | Command-line flags reference |
| CLI Features | https://developers.openai.com/codex/cli/features | Feature documentation |
| Config Reference | https://developers.openai.com/codex/config-reference | config.toml settings |
| npm Package | https://www.npmjs.com/package/@openai/codex | `npm i -g @openai/codex` |

---

## Two Implementations

### Rust Implementation (Current/Primary)
- Location: `codex-rs/` in the repository
- Binary distributed via npm, Homebrew, or GitHub Releases
- Built with Ratatui for TUI, tokio for async
- The `codex exec` subcommand is the non-interactive mode

### TypeScript/Node.js Implementation (Legacy)
- Location: `codex-cli/` in the repository
- Marked as "superseded by the Rust implementation"
- Used `-q`/`--quiet` flag for non-interactive mode
- Environment variable: `CODEX_QUIET_MODE=1`

---

## Non-Interactive / Pipe Mode

### Rust CLI: `codex exec` (Primary Method)

The `codex exec` subcommand (alias: `codex e`) is the equivalent of `claude -p` for Claude Code. It runs the agent headlessly without the interactive TUI.

```bash
# Basic non-interactive execution
codex exec "fix the CI failure"

# Pipe prompt via stdin (use "-" as prompt)
echo "refactor the auth module" | codex exec -

# With model selection
codex exec -m gpt-5.3-codex "describe this project"

# Ephemeral mode (no session persistence)
codex exec --ephemeral "quick one-off task"

# Full auto mode (no approval prompts, sandboxed)
codex exec --full-auto "fix all linting errors"

# YOLO mode (no approvals, no sandbox - dangerous)
codex exec --yolo "deploy to production"
```

**Key behaviors of `codex exec`:**
- Defaults to `AskForApproval::Never` (bypasses interactive approvals)
- Elicitation requests (interactive prompts) are automatically cancelled
- Agent runs autonomously until completion, then exits
- Output is printed directly to stdout
- Errors go to stderr
- Exit code 1 on fatal errors

### Legacy TypeScript CLI: `-q` / `--quiet`

```bash
# Legacy quiet/non-interactive mode
codex -q "fix the bug in auth.ts"

# Via environment variable
CODEX_QUIET_MODE=1 codex "fix the bug"
```

---

## Streaming & JSON Output

### JSONL Streaming (`--json`)

```bash
# Stream events as JSON Lines
codex exec --json "implement the feature"

# Combine with other flags
codex exec --json --full-auto "fix all tests"
```

When `--json` is active, the tool emits **one JSON object per line** (JSONL format) to stdout. Each line is a complete, independently parseable JSON object.

### Event Types in JSONL Output

The JSONL stream emits these `ThreadEvent` types:

| Event Type | Fields | Description |
|-----------|--------|-------------|
| `ThreadStarted` | `thread_id` | Session identifier, emitted at start |
| `TurnStarted` | (empty) | Marks the beginning of an agent turn |
| `ItemStarted` | `item: {id, details}` | An item (message, command, etc.) has started |
| `ItemUpdated` | `item: {id, details}` | Partial update to an in-progress item |
| `ItemCompleted` | `item: {id, details}` | An item has finished |
| `TurnCompleted` | `usage: {input_tokens, cached_input_tokens, output_tokens}` | Turn finished with token usage |
| `TurnFailed` | `error` | Turn ended with an error |
| `Error` | `message` | General error event |

### Item Detail Variants

Each item has a `details` field that is a discriminated union:

| Detail Type | Fields | Description |
|------------|--------|-------------|
| `AgentMessage` | `text` | Text message from the agent |
| `CommandExecution` | `command`, `aggregated_output`, `exit_code`, `status` | Shell command execution |
| `McpToolCall` | `server`, `tool`, `arguments`, `result`, `error`, `status` | MCP tool invocation |
| `CollabToolCall` | `tool`, `sender_thread_id`, `receiver_thread_ids`, `prompt`, `agents_states`, `status` | Multi-agent collaboration |
| `FileChange` | `changes: [{path, kind}]`, `status` | File modifications |
| `WebSearch` | `id`, `query`, `action` | Web search performed |
| `TodoList` | `items: [{text, completed}]` | Task list updates |
| `Reasoning` | `text` | Internal reasoning output |

### Human-Readable Output (Default)

Without `--json`, `codex exec` uses `EventProcessorWithHumanOutput` which formats events with optional ANSI coloring based on terminal capabilities. Color control via `--color <always|never|auto>`.

---

## Structured Output

### `--output-last-message` / `-o`

Writes the agent's final text message to a file:

```bash
codex exec -o /tmp/result.txt "summarize this codebase"
cat /tmp/result.txt
```

If the agent produces no final message, a warning is printed to stderr.

### `--output-schema`

Constrains the agent's final response to match a JSON Schema:

```bash
# Define expected output shape
cat > /tmp/schema.json << 'EOF'
{
  "type": "object",
  "properties": {
    "summary": { "type": "string" },
    "files_changed": { "type": "array", "items": { "type": "string" } },
    "confidence": { "type": "number" }
  },
  "required": ["summary", "files_changed"]
}
EOF

codex exec --output-schema /tmp/schema.json "analyze and fix the auth bug"
```

This is analogous to OpenAI's structured outputs feature, ensuring the final response conforms to the specified schema.

---

## Complete Flag Reference

### Global Flags (Available on all subcommands)

| Flag | Short | Description |
|------|-------|-------------|
| `--model` | `-m` | Specify which AI model to use |
| `--sandbox` | `-s` | Sandbox policy: `read-only`, `workspace-write`, `danger-full-access` |
| `--full-auto` | | Convenience alias: workspace-write sandbox + no approval prompts |
| `--dangerously-bypass-approvals-and-sandbox` | `--yolo` | Skip ALL approvals and sandboxing |
| `--profile` | `-p` | Configuration profile from config.toml |
| `--cd` | `-C` | Set working directory |
| `--add-dir` | | Additional writable directories |
| `--oss` | | Use open-source model provider |
| `--local-provider` | | Specify local provider (lmstudio, ollama) |
| `--search` | | Enable live web search |
| `-c key=value` | | Override configuration values (repeatable) |
| `--skip-git-repo-check` | | Allow running outside Git repos |

### `codex exec` Specific Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--json` | | Print events as JSONL (one JSON object per line) |
| `--output-last-message` | `-o` | Write agent's final message to a file |
| `--output-schema` | | JSON Schema file for structured final response |
| `--color` | | ANSI color output: `always`, `never`, `auto` |
| `--ephemeral` | | Run without persisting session files to disk |
| `--image` | `-i` | Attach image(s) to prompt (comma-delimited paths, repeatable) |
| `PROMPT` | | Positional: instruction text, or `-` to read from stdin |

### `codex exec resume` Flags

| Flag | Description |
|------|-------------|
| `--last` | Resume most recent session |
| `--all` | Include sessions from outside current directory |
| `PROMPT` | Optional follow-up instruction |

### `codex exec review` Flags

| Flag | Description |
|------|-------------|
| `--uncommitted` | Review uncommitted changes |
| `--base` | Base branch for comparison |
| `--commit` | Specific commit to review |
| `--title` | PR/review title |

### Interactive TUI Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--ask-for-approval` | `-a` | When to require approval: suggest, auto-edit, full-auto |
| `--no-alt-screen` | | Disable alternate screen (preserve terminal scrollback) |
| `--notify` | | Toggle desktop notifications |

### Other Subcommands

| Command | Description |
|---------|-------------|
| `codex` | Launch interactive TUI |
| `codex app` | Launch desktop application |
| `codex resume [SESSION_ID]` | Resume interactive session |
| `codex fork [SESSION_ID]` | Fork a session into new thread |
| `codex mcp-server` | Run Codex as an MCP server |
| `codex mcp add\|list\|get\|remove` | Manage MCP server connections |
| `codex sandbox <platform>` | Debug/test sandbox behavior |
| `codex completion <shell>` | Generate shell completions |
| `codex cloud exec` | Launch remote cloud tasks |

---

## Configuration

### Config File Location
`~/.codex/config.toml` (TOML format, not JSON/YAML despite legacy docs)

### Key Config Settings for Programmatic Use

```toml
# Model selection
model = "gpt-5.3-codex"

# Sandbox default
sandbox_mode = "workspace-write"

# Web search mode
web_search = "live"  # or "cached"

# Plan mode reasoning
plan_mode_reasoning_effort = "medium"
```

### Project Instructions
Codex loads `AGENTS.md` files at three levels (similar to Claude Code's CLAUDE.md):
1. `~/.codex/AGENTS.md` (global)
2. `<project-root>/AGENTS.md` (project)
3. `<cwd>/AGENTS.md` (directory-specific)

### Environment Variables

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | API key for OpenAI provider |
| `RUST_LOG` | Logging verbosity for the Rust binary |
| `CODEX_QUIET_MODE` | (Legacy TS only) Enable quiet mode |

---

## Comparison with Claude Code CLI

| Feature | Codex CLI (Rust) | Claude Code CLI |
|---------|-----------------|-----------------|
| Non-interactive mode | `codex exec "prompt"` | `claude -p "prompt"` |
| Stdin pipe | `echo "prompt" \| codex exec -` | `echo "prompt" \| claude -p` |
| JSON streaming | `codex exec --json` | `claude -p --output-format stream-json` |
| Save final output | `codex exec -o file.txt` | Redirect stdout |
| Structured output | `codex exec --output-schema schema.json` | Not directly supported |
| Model selection | `codex exec -m model-name` | `claude -p --model model-name` |
| Image input | `codex exec -i image.png "prompt"` | Not in pipe mode |
| Session resume | `codex exec resume --last` | `claude -p --continue` |
| Approval bypass | `--full-auto` or `--yolo` | `--dangerously-skip-permissions` |
| Project instructions | `AGENTS.md` | `CLAUDE.md` |
| MCP support | Built-in client and server mode | MCP client support |
| Sandbox | Built-in (Seatbelt/Landlock/Docker) | Built-in sandbox |

---

## Security Concerns

### Sandbox Modes
- **read-only** (default): Most restrictive, agent cannot modify files
- **workspace-write**: Agent can write within workspace, network disabled
- **danger-full-access**: No sandboxing (only for isolated environments)

### Known Security Considerations
- `--yolo` / `--dangerously-bypass-approvals-and-sandbox` disables ALL safety measures
- Network is disabled by default in sandboxed modes (macOS Seatbelt, Linux Landlock/Docker)
- Elicitation requests are auto-cancelled in exec mode (prevents hanging on interactive prompts)
- Session files persist by default (use `--ephemeral` to prevent)

### Authentication
- Primary: ChatGPT sign-in (leverages existing subscription)
- Alternative: OpenAI API key
- Supports multiple providers: Azure, OpenRouter, Gemini, Ollama, Mistral, DeepSeek, xAI, Groq, ArceeAI

---

## Risks & Gotchas

### Common Pitfalls
1. **Legacy vs Rust CLI confusion** - The TypeScript (`codex-cli/`) and Rust (`codex-rs/`) implementations have different flag syntax. The Rust version is current; TypeScript is legacy.
2. **`-q` is legacy only** - The quiet flag only exists in the TypeScript version. Use `codex exec` in the Rust version.
3. **Default sandbox is read-only** - In exec mode, agent cannot write files unless `--full-auto` or `--sandbox workspace-write` is specified.
4. **Session persistence** - By default, `codex exec` persists session files to disk. Use `--ephemeral` to prevent this in CI/automation.
5. **Stdin requires explicit `-`** - Unlike some CLIs, you must pass `-` as the prompt argument to read from stdin.

### Breaking Changes
| Version | Change | Impact |
|---------|--------|--------|
| Rust rewrite | `codex -q` replaced by `codex exec` | All automation scripts using `-q` need updating |
| Rust rewrite | Config format changed from YAML/JSON to TOML | Config files need migration |
| Rust rewrite | `AGENTS.md` replaced `codex.md` for project instructions | Instruction files need renaming |

---

## Community Insights

### Provider Flexibility
- Codex CLI supports multiple AI providers beyond OpenAI (Azure, Gemini, Ollama, Mistral, DeepSeek, xAI, Groq, ArceeAI)
- This is set via `--provider` flag or config.toml
- Local models supported via `--oss` flag with LM Studio or Ollama

### MCP Integration
- Codex can act as both MCP client (connecting to external tools) and MCP server (letting other agents use it)
- `codex mcp-server` command exposes Codex as a tool for other agents
- Useful for multi-agent orchestration scenarios

### Cloud Execution
- `codex cloud exec` launches tasks on remote infrastructure
- Supports `--attempts N` (1-4) for best-of-N execution strategies
- Non-zero exit codes on failure for CI integration

---

## Implementation Recommendations

### For OpenPaw Integration

If integrating Codex CLI as an alternative AI engine alongside Claude Code:

**Recommended Approach:**
Shell out to `codex exec` with `--json` for structured streaming, similar to how Claude Code CLI could be invoked. Parse the JSONL stream for real-time event processing.

```bash
# Example integration pattern
codex exec --json --full-auto --ephemeral -m gpt-5.3-codex "task description"
```

**Key integration points:**
1. Use `codex exec --json` for JSONL event streaming (parse `AgentMessage` items for text, `CommandExecution` for tool use)
2. Use `-o` flag to capture final output to a file
3. Use `--output-schema` for structured responses when needed
4. Use `--ephemeral` in server contexts to avoid session file accumulation
5. Map Codex events to OpenPaw's existing `StreamEvent` types

### Alternatives Considered

| Approach | Pros | Cons |
|----------|------|------|
| Shell out to `codex exec --json` | Rich streaming, full agent capabilities, maintained by OpenAI | External dependency, process management overhead |
| Direct OpenAI API (current pattern) | Direct control, no CLI dependency, lower latency | No agent loop, no tool execution, must implement own tool dispatch |
| OpenAI Responses API with tools | API-native, structured, reliable | Must implement tool execution loop, no local sandbox |

---

## Related Research

- OpenPaw currently uses direct API integration via `internal/llm/` package
- Agent loop implementation in `internal/llm/agent_loop.go`
- Stream events defined in `internal/llm/types.go`
- Claude Code CLI integration referenced in `CLAUDE.md` ("AI Engine: Claude Code CLI")

---

## Research History

| Date | Researcher | Areas Updated |
|------|------------|---------------|
| 2026-02-23 | researcher agent | Initial research - complete CLI reference, exec mode, JSONL streaming, structured output |
