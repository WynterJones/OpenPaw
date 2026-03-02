# Research: Google Gemini CLI -- Programmatic & Non-Interactive Usage

> Systematic research findings for using Google's Gemini CLI tool programmatically,
> in non-interactive/pipe mode, with streaming output, and structured JSON responses.
> This is a living document -- updated periodically as new information emerges.

**Created:** 2026-02-23
**Last Researched:** 2026-02-23
**Status:** Fresh
**Confidence:** High (verified by running actual commands against installed v0.26.0)

---

## Summary

Google Gemini CLI (`@google/gemini-cli`) is an open-source, npm-distributed AI agent for the terminal. It supports **three output formats** (`text`, `json`, `stream-json`) via the `-o`/`--output-format` flag, and enters **headless/non-interactive mode** when given a positional argument or `-p`/`--prompt` flag, or when stdin is not a TTY. The streaming JSON format (`stream-json`) emits newline-delimited JSON events in real-time, closely analogous to Claude Code's `--output-format stream-json`. The CLI supports stdin piping, file inclusion via `@` syntax, session resumption, model selection, approval modes, and sandbox execution.

---

## Official Documentation

| Resource | URL | Notes |
|----------|-----|-------|
| GitHub Repository | https://github.com/google-gemini/gemini-cli | Apache 2.0, monorepo |
| Documentation Site | https://geminicli.com/docs/ | Official docs |
| NPM Package | https://www.npmjs.com/package/@google/gemini-cli | `@google/gemini-cli` |
| CLI Reference | `docs/cli/cli-reference.md` in repo | Full flag reference |
| Headless Mode Reference | `docs/cli/headless.md` in repo | Output schemas |
| Automation Tutorial | `docs/cli/tutorials/automation.md` in repo | Scripting examples |

---

## Installation

```bash
# npm (global)
npm install -g @google/gemini-cli

# Homebrew (macOS/Linux)
brew install gemini-cli

# npx (no install)
npx @google/gemini-cli

# Release channels
npm install -g @google/gemini-cli@latest    # Stable (Tuesdays)
npm install -g @google/gemini-cli@preview   # Preview (Tuesdays)
npm install -g @google/gemini-cli@nightly   # Nightly (daily)
```

Current installed version: **0.26.0**

---

## Non-Interactive / Headless Mode

Headless mode is triggered in three ways:

### 1. Positional Argument (Preferred, One-Shot)

```bash
gemini "Explain the architecture of this codebase"
```

This is the **recommended** approach. The `-p` flag is marked as **deprecated** in favor of positional arguments.

### 2. The `-p` / `--prompt` Flag (Deprecated but Functional)

```bash
gemini -p "Explain the architecture of this codebase"
```

When stdin is also provided, the `-p` prompt is **appended** to the stdin content:

```bash
echo "Some context" | gemini -p "Analyze the above"
# Equivalent to prompt: "Some context\n\nAnalyze the above"
```

### 3. Piped stdin (Non-TTY Detection)

```bash
cat error.log | gemini "Explain why this failed"
echo "What is 2+2?" | gemini
```

When stdin is not a TTY, the CLI reads all stdin content and uses it as context. If a positional argument or `-p` is also provided, the stdin content is prepended to the prompt.

### 4. Interactive After Prompt (`-i` / `--prompt-interactive`)

```bash
gemini -i "What is the purpose of this project?"
```

Executes the prompt and then **stays in interactive mode** for follow-up questions. Cannot be used with piped stdin.

---

## Output Formats (`-o` / `--output-format`)

### `text` (Default)

Plain text output to stdout. Errors and warnings go to stderr.

```bash
gemini "Say hello" -o text
# Output: hello
```

- Response text streams incrementally to stdout
- A trailing newline is ensured
- Tool execution output goes to stderr as `[WARNING]` or `[ERROR]` prefixed lines
- Exit codes: 0 (success), 1 (error), 42 (input error), 53 (turn limit exceeded)

### `json` (Structured)

Returns a **single JSON object** after the entire response completes. Useful for extracting structured data.

```bash
gemini "Say hello" -o json
```

**Schema:**

```json
{
  "session_id": "uuid-string",
  "response": "The model's final text answer as a string",
  "stats": {
    "models": {
      "gemini-model-name": {
        "api": {
          "totalRequests": 1,
          "totalErrors": 0,
          "totalLatencyMs": 1234
        },
        "tokens": {
          "input": 1000,
          "prompt": 1000,
          "candidates": 50,
          "total": 1100,
          "cached": 0,
          "thoughts": 100,
          "tool": 0
        }
      }
    },
    "tools": {
      "totalCalls": 0,
      "totalSuccess": 0,
      "totalFail": 0,
      "totalDurationMs": 0,
      "totalDecisions": {
        "accept": 0,
        "reject": 0,
        "modify": 0,
        "auto_accept": 0
      },
      "byName": {}
    },
    "files": {
      "totalLinesAdded": 0,
      "totalLinesRemoved": 0
    }
  }
}
```

**Extracting just the response with jq:**

```bash
gemini "Summarize this file" -o json | jq -r '.response'
```

### `stream-json` (Streaming JSONL)

Returns **newline-delimited JSON events** as they occur. Each line is a complete JSON object. This is the most useful format for programmatic consumption.

```bash
gemini "Say hello" -o stream-json
```

**Event Types:**

#### `init` -- Session initialization

```json
{"type":"init","timestamp":"2026-02-24T03:15:16.134Z","session_id":"uuid","model":"auto-gemini-3"}
```

#### `message` (role: user) -- The user's input

```json
{"type":"message","timestamp":"...","role":"user","content":"Say hello\n\nrespond briefly"}
```

#### `message` (role: assistant, delta: true) -- Streamed response chunks

```json
{"type":"message","timestamp":"...","role":"assistant","content":"Hello. I am ready to assist","delta":true}
{"type":"message","timestamp":"...","role":"assistant","content":" you. How can I help?","delta":true}
```

The `delta: true` flag indicates this is an incremental chunk, not the full message. Concatenate all assistant message chunks to get the full response.

#### `tool_use` -- Tool call request

```json
{"type":"tool_use","timestamp":"...","tool_name":"read_file","tool_id":"call-id","parameters":{"path":"file.ts"}}
```

#### `tool_result` -- Tool execution result

```json
{"type":"tool_result","timestamp":"...","tool_id":"call-id","status":"success","output":"file contents..."}
```

Or on error:

```json
{"type":"tool_result","timestamp":"...","tool_id":"call-id","status":"error","error":{"type":"TOOL_EXECUTION_ERROR","message":"..."}}
```

#### `error` -- Warnings and errors

```json
{"type":"error","timestamp":"...","severity":"warning","message":"Loop detected, stopping execution"}
```

#### `result` -- Final completion event

```json
{"type":"result","timestamp":"...","status":"success","stats":{"total_tokens":10992,"input_tokens":10758,"output_tokens":92,"cached":6849,"input":3909,"duration_ms":3615,"tool_calls":0}}
```

---

## Complete CLI Flags Reference

| Flag | Alias | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--debug` | `-d` | boolean | `false` | Debug mode with verbose logging |
| `--version` | `-v` | -- | -- | Show version and exit |
| `--help` | `-h` | -- | -- | Show help and exit |
| `--model` | `-m` | string | `auto` | Model to use (see Model Aliases below) |
| `--prompt` | `-p` | string | -- | Prompt text, appended to stdin. **Deprecated: use positional args** |
| `--prompt-interactive` | `-i` | string | -- | Execute prompt then stay interactive |
| `--sandbox` | `-s` | boolean | `false` | Run in sandboxed environment |
| `--approval-mode` | -- | string | `default` | `default`, `auto_edit`, `yolo`, `plan` (plan requires experimental flag) |
| `--yolo` | `-y` | boolean | `false` | **Deprecated.** Auto-approve all. Use `--approval-mode=yolo` |
| `--experimental-acp` | -- | boolean | -- | ACP (Agent Code Pilot) mode |
| `--allowed-mcp-server-names` | -- | array | -- | Restrict which MCP servers can be used |
| `--allowed-tools` | -- | array | -- | **Deprecated.** Tools allowed without confirmation |
| `--extensions` | `-e` | array | -- | Limit which extensions to load |
| `--list-extensions` | `-l` | boolean | -- | List extensions and exit |
| `--resume` | `-r` | string | -- | Resume session: `"latest"` or index number |
| `--list-sessions` | -- | boolean | -- | List sessions for current project |
| `--delete-session` | -- | string | -- | Delete session by index |
| `--include-directories` | -- | array | -- | Additional workspace directories |
| `--screen-reader` | -- | boolean | -- | Accessibility: screen reader mode |
| `--output-format` | `-o` | string | `text` | `text`, `json`, `stream-json` |

### Model Aliases

| Alias | Resolves To | Description |
|-------|-------------|-------------|
| `auto` | `gemini-2.5-pro` or `gemini-3-pro-preview` | Default. Preview model if preview features enabled |
| `pro` | Same as auto | Complex reasoning tasks |
| `flash` | `gemini-2.5-flash` | Fast, balanced |
| `flash-lite` | `gemini-2.5-flash-lite` | Fastest, simple tasks |

You can also specify concrete model names directly: `gemini -m gemini-2.5-flash`

---

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `GEMINI_API_KEY` | API key for Gemini API authentication |
| `GEMINI_MODEL` | Default model (overridden by `--model` flag) |
| `GEMINI_SANDBOX` | Sandbox command (e.g., `docker`, `podman`, `true`) |
| `GEMINI_SANDBOX_IMAGE` | Custom sandbox container image |
| `GEMINI_CLI_NO_RELAUNCH` | Prevent memory-based child process relaunch |
| `GEMINI_CLI_USE_COMPUTE_ADC` | Use Compute Engine ADC auth |
| `GEMINI_DEFAULT_AUTH_TYPE` | Default authentication type |
| `GEMINI_CLI_SYSTEM_SETTINGS_PATH` | Custom system settings path |
| `GOOGLE_API_KEY` | Alternative API key (Vertex AI) |
| `GOOGLE_CLOUD_PROJECT` | Google Cloud project for Code Assist |
| `GOOGLE_CLOUD_LOCATION` | Google Cloud region |
| `GOOGLE_GENAI_USE_VERTEXAI` | Enable Vertex AI mode |
| `HTTPS_PROXY` / `HTTP_PROXY` | Proxy configuration |
| `NO_PROXY` | Proxy bypass |
| `NO_BROWSER` | Prevent browser opening for auth |
| `DEBUG` / `DEBUG_MODE` | Enable debug mode |
| `CLOUD_SHELL` | Detect Google Cloud Shell |

---

## Comparison with Claude Code CLI

| Feature | Claude Code (`claude`) | Gemini CLI (`gemini`) |
|---------|----------------------|----------------------|
| **Non-interactive flag** | `claude -p "prompt"` | `gemini "prompt"` (positional) or `gemini -p "prompt"` (deprecated) |
| **Stdin piping** | `echo "x" \| claude -p "y"` | `echo "x" \| gemini "y"` or `echo "x" \| gemini -p "y"` |
| **Text output** | `--output-format text` | `-o text` (default) |
| **JSON output** | `--output-format json` | `-o json` |
| **Streaming JSON** | `--output-format stream-json` | `-o stream-json` |
| **Model selection** | `--model claude-sonnet-4-20250514` | `-m gemini-2.5-flash` |
| **Auto-approve** | `--dangerously-skip-permissions` | `--approval-mode yolo` or `-y` |
| **Max turns** | `--max-turns N` | Via settings: `model.maxSessionTurns` |
| **Session resume** | `--resume` | `-r latest` or `-r <session-id>` |
| **MCP support** | Yes (via config) | Yes (`gemini mcp add/remove/list`) |
| **File inclusion** | Automatic context | `@filepath` syntax in prompts |
| **System prompt** | `--system-prompt` | Via GEMINI.md files |
| **Package** | `@anthropic-ai/claude-code` | `@google/gemini-cli` |

---

## Practical Scripting Patterns

### Basic One-Shot Query

```bash
gemini "Explain the architecture of this codebase" -o text
```

### Pipe File Content for Analysis

```bash
cat error.log | gemini "Explain why this failed"
```

### Generate Commit Messages

```bash
git diff --staged | gemini "Write a concise Conventional Commit message. Output ONLY the message."
```

### Extract Structured Data with jq

```bash
gemini "Return a JSON object with keys 'version' and 'deps' from @package.json" -o json | jq -r '.response'
```

### Stream and Process Events

```bash
gemini "Run tests and analyze results" -o stream-json | while IFS= read -r line; do
  type=$(echo "$line" | jq -r '.type')
  case "$type" in
    message)
      role=$(echo "$line" | jq -r '.role')
      content=$(echo "$line" | jq -r '.content')
      if [ "$role" = "assistant" ]; then
        echo -n "$content"
      fi
      ;;
    result)
      echo ""
      echo "Done! Tokens: $(echo "$line" | jq -r '.stats.total_tokens')"
      ;;
  esac
done
```

### Use Specific Model

```bash
gemini -m gemini-2.5-flash "Quick question: what is 2+2?" -o text
```

### Auto-Approve All Tool Calls

```bash
gemini "Fix the linting errors in this file" --approval-mode yolo -o text
```

### Resume a Previous Session

```bash
# List sessions
gemini --list-sessions

# Resume latest
gemini -r latest "Continue where we left off"

# Resume by index
gemini -r 5 "Check for type errors"
```

### Include Additional Directories

```bash
gemini --include-directories ../lib,../docs "How do these libraries interact?"
```

### Bulk Processing Script

```bash
#!/bin/bash
for file in *.py; do
  echo "Processing $file..."
  gemini "Generate Markdown docs for @$file" > "${file%.py}.md"
done
```

---

## Authentication for Non-Interactive Use

For CI/CD and scripting, the recommended auth methods are:

1. **API Key** (simplest):
   ```bash
   export GEMINI_API_KEY="your-key"
   gemini "your prompt"
   ```

2. **OAuth (cached)**: If you've previously authenticated interactively, the CLI caches credentials in `~/.gemini/oauth_creds.json` and reuses them in headless mode.

3. **Vertex AI** (enterprise):
   ```bash
   export GOOGLE_API_KEY="your-key"
   export GOOGLE_GENAI_USE_VERTEXAI=true
   gemini "your prompt"
   ```

4. **Compute ADC** (GCE/Cloud Shell):
   ```bash
   export GEMINI_CLI_USE_COMPUTE_ADC=true
   gemini "your prompt"
   ```

---

## Configuration Files

| File | Location | Purpose |
|------|----------|---------|
| `settings.json` | `~/.gemini/settings.json` | User-level settings |
| `settings.json` | `.gemini/settings.json` (project) | Workspace-level settings (overrides user) |
| `GEMINI.md` | `~/.gemini/GEMINI.md` | Global system prompt/context |
| `GEMINI.md` | `.gemini/GEMINI.md` (project) | Project-specific context |
| `oauth_creds.json` | `~/.gemini/oauth_creds.json` | Cached OAuth credentials |
| `trustedFolders.json` | `~/.gemini/trustedFolders.json` | Trusted folder list |

---

## Built-in Tools Available to the Agent

When running in headless mode, the agent has access to these tools:

- `list_directory` -- List files/folders
- `read_file` -- Read file contents
- `search_file_content` -- Grep-like search (uses ripgrep)
- `glob` -- Pattern-based file finding
- `write_file` / `replace` -- Create/modify files
- `run_shell_command` -- Execute non-interactive shell commands
- `google_web_search` -- Internet search
- `delegate_to_agent` -- Spawn sub-agents (codebase_investigator, cli_help)
- `activate_skill` -- Use registered skills
- `save_memory` -- Persist information across sessions

---

## Risks & Gotchas

### Common Pitfalls

1. **`-p` is deprecated** -- Use positional arguments instead: `gemini "prompt"` not `gemini -p "prompt"`. The `-p` flag still works but may be removed in a future version.

2. **Rate limiting** -- Free tier is 60 req/min and 1,000 req/day. The CLI retries with backoff on 429 errors, but batch scripts should add delays.

3. **Multi-model routing** -- The `auto` model alias uses a classifier (flash-lite) to route between flash and pro models. This means you may see two model entries in the JSON stats. For consistent behavior in scripts, specify a concrete model: `-m gemini-2.5-flash`.

4. **`--prompt-interactive` cannot be piped** -- Using `-i` with piped stdin will error: "The --prompt-interactive flag cannot be used when input is piped from stdin."

5. **Plan mode requires experimental flag** -- `--approval-mode plan` requires `experimental.plan` to be enabled in settings. Will error otherwise.

6. **Sandbox mode reads stdin early** -- When sandbox is enabled, stdin is read before entering the sandbox and injected into args. This changes the order of operations.

7. **Tool execution in headless mode** -- Tools still execute in headless mode. Use `--approval-mode default` (the default) if you want approval prompts, but note this requires a TTY. For fully non-interactive use with tools, use `--approval-mode yolo` or `--approval-mode auto_edit`.

8. **EPIPE handling** -- The CLI gracefully handles broken pipes (e.g., `gemini "long response" | head -1`), exiting with code 0.

9. **Context window** -- The models have a 1M token context window, but the CLI compresses context at 50% usage by default (configurable via `model.compressionThreshold`).

10. **stderr vs stdout** -- In text mode, the response goes to stdout. Warnings, errors, and tool status messages go to stderr. This is important for parsing output in scripts.

### Edge Cases

- When both stdin and `-p` are provided, stdin content comes first, then a double newline, then the `-p` content
- The `@filepath` syntax for file inclusion works in both interactive and headless mode
- Session files are stored per-project, so `--list-sessions` shows different results in different directories
- The `stream-json` format does NOT buffer -- events are emitted as they occur, one JSON object per line

---

## Community Insights

### Key Points from GitHub/Community

- The project uses a monorepo structure under `packages/` with `@google/gemini-cli` and `@google/gemini-cli-core`
- Extensions system allows community-built tools (similar to Claude Code's custom tools)
- Skills system provides reusable agent behaviors (similar to Claude Code's custom slash commands)
- Hooks system allows pre/post processing of events (includes migration from Claude Code hooks: `gemini hooks migrate`)
- The CLI uses Ink (React for CLI) for its interactive UI, and a completely separate code path for non-interactive mode

### GitHub Issues / Discussions

| Topic | Details |
|-------|---------|
| Rate limiting (429) | Common on free tier; CLI has built-in retry with backoff |
| Model routing overhead | The classifier call adds latency; specify model directly for speed |
| Large output truncation | Tool output truncated at 40,000 chars by default (configurable) |

---

## Implementation Recommendations for OpenPaw

### Recommended Approach for Integrating Gemini CLI

For OpenPaw's agent system (which currently shells out to `claude`), integrating Gemini CLI would follow a similar pattern:

1. **Command construction**: Use positional arguments (not `-p`) with `-o stream-json` for real-time streaming
2. **Streaming**: Parse JSONL line-by-line, filter for `type: "message"` with `role: "assistant"` and `delta: true`
3. **Auto-approval**: Use `--approval-mode yolo` for automated workflows
4. **Model selection**: Use `-m gemini-2.5-flash` for speed or `-m auto` for quality routing
5. **Error handling**: Check exit codes (0=success, 1=error, 42=input error, 53=turn limit)

### Example Integration Pattern (Go)

```go
cmd := exec.Command("gemini", prompt, "-o", "stream-json", "--approval-mode", "yolo", "-m", "gemini-2.5-flash")
cmd.Dir = workDir
cmd.Stdin = strings.NewReader(context)

stdout, _ := cmd.StdoutPipe()
cmd.Start()

scanner := bufio.NewScanner(stdout)
for scanner.Scan() {
    var event map[string]interface{}
    json.Unmarshal(scanner.Bytes(), &event)
    
    if event["type"] == "message" && event["role"] == "assistant" {
        if delta, ok := event["delta"].(bool); ok && delta {
            // Stream chunk to client
            content := event["content"].(string)
            sendToWebSocket(content)
        }
    }
}
cmd.Wait()
```

### Alternatives Considered

| Approach | Pros | Cons |
|----------|------|------|
| Gemini CLI (`gemini`) | Free tier, open source, good streaming, similar to Claude CLI | Rate limits, model quality varies |
| Gemini API directly | More control, no CLI dependency | More code, manage auth/streaming yourself |
| Claude Code CLI (`claude`) | Already integrated in OpenPaw | Paid, different provider |

---

## Related Research

- OpenPaw agent system: `internal/agents/` (currently uses Claude Code CLI)
- Claude Code CLI reference: similar `-p` and `--output-format` patterns

---

## Research History

| Date | Researcher | Areas Updated |
|------|------------|---------------|
| 2026-02-23 | researcher agent | Initial research -- full CLI analysis with live testing against v0.26.0 |
