# Brownfield Status

> What's already implemented. Focus on WHAT exists, not HOW it works.
> Updated during "go to production" to track solidified features.

**Last Updated:** 2026-02-20
**Status:** Active development
**Implemented Features:** 22

---

## Current State

_High-level summary of what the app currently does_

OpenPaw is a fully functional single-binary Go application with an embedded React frontend. It provides a conversational AI interface where a gateway agent ("Pounce") routes user messages to specialist agents. These agents can build Go-based tools (compiled microservices), create dashboards, manage browser sessions, run scheduled tasks, and operate heartbeat monitoring -- all from chat. The system includes full authentication, encrypted secrets management, a context file system, activity logging with cost tracking, and a comprehensive settings panel with theming.

---

## Solidified Features

_Features that are complete and stable_

### Authentication and Setup
**Status:** Complete
**Added:** Pre-2026-02-18

Four-step setup wizard for first-time configuration. JWT-based authentication with HttpOnly cookies and CSRF protection.

**Capabilities:**
- Setup wizard: Welcome screen, admin account creation, OpenRouter API key, server configuration (name, bind address, port)
- Login with username/password, "Remember me for 30 days" option
- Auto-redirect to setup if no admin user exists
- CSRF token protection on all mutating requests
- Session timeout configurable in settings

### Chat System
**Status:** Complete
**Added:** Pre-2026-02-18

Thread-based conversational interface with real-time streaming, gateway routing, and multi-agent support.

**Capabilities:**
- Thread management with sidebar list, search, pagination, and delete
- Real-time streaming via WebSocket (text_delta, tool_start, tool_end, result events)
- Gateway agent "Pounce" analyzes every message and routes to appropriate specialist
- @mention autocomplete to bring specific agents into conversation
- !! autocomplete to insert context files inline
- File attachments via drag-and-drop or upload button
- Thread members panel showing participating agents
- Context window usage bar (percentage of token limit)
- Compact chat (summarize/compress thread history)
- Stop generation button
- Per-thread cost and token tracking
- Active thread indicator in sidebar
- Widget rendering from tool output (charts, tables, etc.)
- Gateway thinking indicator during routing

### Agent Roles
**Status:** Complete
**Added:** Pre-2026-02-18

Configurable AI agent personas with identity files, memory, skills, and tool access.

**Capabilities:**
- Create agents with name, description, system prompt, avatar (6 presets + custom upload)
- Gateway agent ("Pounce" / "builder" slug) always active, routes all conversations
- Identity file system per agent: SOUL.md (personality), USER.md (learned user info), AGENTS.md (runbook), BOOT.md (startup instructions), HEARTBEAT.md (periodic check instructions)
- Gateway has simplified identity: SOUL.md, USER.md, HEARTBEAT.md
- Per-agent memory (memory.md + daily logs)
- Per-agent skill assignment (from global skills pool)
- Per-agent tool access (owned tools + granted tools)
- Per-agent model selection with searchable model picker
- Enable/disable toggle per agent
- Identity initialization (first-time setup for identity files)
- Grid and list view with pagination on agent list page
- Agent deletion with confirmation modal

### Tools System
**Status:** Complete
**Added:** Pre-2026-02-18

Go-based microservice tools with full lifecycle management and a library catalog.

**Capabilities:**
- Two tabs: "My Tools" (installed) and "Library" (catalog)
- Tool lifecycle: compile, start, stop, restart, enable/disable
- Tool detail view with status/port/PID info cards
- Inline-editable name and description
- Endpoints display with method, path, and description
- Capabilities metadata display
- Integrity panel with source hash verification
- Import tools from .zip files, export tools as .zip
- Tool library catalog with categories and search
- Install tools from library with version tracking
- Owner agent assignment
- Grid and list view with pagination and search
- Tool process auto-start on server boot

### Dashboards
**Status:** Complete
**Added:** Pre-2026-02-18

Two dashboard types with data refresh and tool integration.

**Capabilities:**
- Block dashboards: widget grid layout with configurable columns and gap
- Custom dashboards: sandboxed iframe running HTML/JS with postMessage API
- Dashboard switcher dropdown with type badges (Block/Custom)
- Refresh data button for block dashboards
- Custom dashboard bridge: callTool, getTools actions via postMessage
- Theme variable injection into custom dashboards (OKLCH color system)
- Last-viewed dashboard persistence via localStorage
- Empty state suggests creating dashboards via chat

### Secrets Management
**Status:** Complete
**Added:** Pre-2026-02-18

Encrypted credential storage with tool association.

**Capabilities:**
- Create secrets with name, value, and optional description
- Encrypted storage (separate encryption key from JWT)
- Rotate secret values
- Test connection functionality
- Delete with confirmation
- Grid and list view with pagination and search
- Optional tool association

### Scheduler
**Status:** Complete
**Added:** Pre-2026-02-18

Cron-based task scheduling with two execution types.

**Capabilities:**
- Two schedule types: Tool Action (select tool + action) and AI Prompt (select agent + prompt)
- Cron presets: every hour, daily 9am, weekly Monday, monthly 1st, every 15 min, every 30 min, custom expression
- Enable/disable toggle per schedule
- Run Now button for immediate execution
- Execution history table with status, duration, started/finished times, output
- Schedule detail view with expandable execution details
- Dashboard refresh schedules and browser automation schedules supported
- Delete with confirmation

### Context System
**Status:** Complete
**Added:** Pre-2026-02-18

File management system for providing persistent context to agents.

**Capabilities:**
- File tree sidebar with hierarchical folders
- Upload files via drag-and-drop or button (multiple file support)
- Text file editor with in-browser editing
- Image file viewer
- Binary file download
- "About You" section (personal context included in all agent conversations)
- Create folders, rename files/folders, delete
- Move files between folders via drag-and-drop
- Right-click context menu on files
- Folder expansion state tracking

### Browser Automation
**Status:** Complete
**Added:** Pre-2026-02-18

Playwright-based headless browser sessions with human takeover capability.

**Capabilities:**
- Create browser sessions with name and optional owner agent
- Headless mode toggle
- Start, stop, delete sessions
- Screenshot viewer (BrowserViewer component)
- Action bar for sending commands (BrowserActionBar)
- Human control: take/release control of browser session
- Action log showing browser activity
- Owner agent assignment
- Grid and list view
- Real-time status updates via WebSocket

### Heartbeat Monitor
**Status:** Complete
**Added:** Pre-2026-02-18

Periodic agent check-in system with configurable schedules and active hours.

**Capabilities:**
- Global heartbeat enable/disable toggle
- Configurable interval: 15 min, 30 min, 1 hour, 2 hours, 4 hours, 8 hours, 12 hours, 24 hours
- Active hours with start/end time and timezone selection
- Per-agent heartbeat enable/disable
- Run Now button per agent
- Execution history with search and pagination
- Execution detail modal: actions taken, output, errors, cost, input/output tokens, duration
- Real-time status updates via WebSocket
- Auto-start on server boot

### Skills System
**Status:** Complete
**Added:** Pre-2026-02-18

Global markdown-based skill documents assignable to agents.

**Capabilities:**
- Create skills with name and markdown content (YAML frontmatter for metadata)
- Edit skills in full-page textarea editor
- Delete skills with confirmation
- Grid and list view with pagination
- Per-agent skill assignment via agent edit page

### Activity Logs
**Status:** Complete
**Added:** Pre-2026-02-18

Comprehensive activity logging with cost and token tracking.

**Capabilities:**
- Stats cards: total cost (USD), total tokens, total activity count
- 12 category filters (auth, tools, secrets, agents, schedules, dashboards, chat, system, skills, context, browser, heartbeat)
- 40+ action type filters
- Full-text search across log entries
- Auto-refresh toggle
- Pagination (50 entries per page)
- Color-coded category and action badges
- Detailed log entries with user, target, timestamp, and details

### Settings
**Status:** Complete
**Added:** Pre-2026-02-18

Comprehensive settings panel with 10 tabs.

**Capabilities:**
- **Profile:** Edit username, change password, upload avatar
- **General:** App name, bind address, port, build confirmation toggle
- **Notifications:** Sound toggle, browser push notifications
- **Network:** LAN access with QR code generation, Tailscale integration toggle
- **AI Models:** OpenRouter API key, gateway model selection, builder model selection, max conversation turns (default 300), agent timeout (default 60 min)
- **Design:** 8 accent color presets (Rose, Coral, Amber, Emerald, Teal, Sky, Violet, Slate) + custom color picker, 5 font options (System, Inter, JetBrains Mono, Space Grotesk, DM Sans), design system preview modal
- **Security:** Session timeout, IP allowlist
- **System:** Version info, Go version, platform, uptime, DB size, tool count, API key status, data export/import
- **About:** Version v0.1.0 display
- **Danger:** Delete all data (requires typing DELETE), delete account (requires typing DELETE)

### Notifications
**Status:** Complete
**Added:** Pre-2026-02-18

In-app notification system with bell indicator.

**Capabilities:**
- NotificationBell component in header with unread count badge
- Mark notifications as read/dismissed
- Sound notification toggle
- Browser push notification support
- Notification sources: agents, schedules, heartbeat, system events

### WebSocket Real-Time Updates
**Status:** Complete
**Added:** Pre-2026-02-18

Real-time communication layer for streaming and status updates.

**Capabilities:**
- Chat message streaming (text_delta, tool_start, tool_end, result events)
- Tool status changes
- Browser session updates
- Heartbeat execution status
- Gateway routing events (gateway_thinking)
- Connection status indicator in UI
- Active thread tracking

### Navigation and Layout
**Status:** Complete
**Added:** Pre-2026-02-18

Responsive layout with collapsible sidebar and mobile bottom navigation.

**Capabilities:**
- Collapsible sidebar with grouped navigation sections
- Navigation groups: Featured (Dashboards), Main (Chat, Agents, Browser, Tools, Skills, Context, Scheduler, Heartbeat), Management (Secrets, Logs), Footer (Settings)
- Mobile bottom navigation bar (BottomNav) with 5 key links
- Responsive header with page title and action buttons
- Footer text: "OpenPaw - Agentic Factory"

### Design System
**Status:** Complete
**Added:** Pre-2026-02-18

OKLCH-based color theming with CSS custom properties.

**Capabilities:**
- Full OKLCH color system with CSS custom properties (--op-* namespace)
- Surface hierarchy (op-surface-0 through op-surface-3)
- Border hierarchy (op-border-0, op-border-1)
- Text hierarchy (op-text-0 through op-text-3)
- Accent colors with hover, muted, and text variants
- Danger colors with hover variant
- Spacing scale (op-space-1 through op-space-8)
- Font size scale (xs through 3xl)
- Border radius scale (sm through full)
- Font family customization
- Design system preview modal in settings

---

## Recent Changes

_Features added or removed in recent production cycles_

### Added
| Feature | Date | Notes |
|---------|------|-------|
| Proactive gateway routing | 2026-02-20 | Gateway "Pounce" actively routes conversations |
| Mobile UX improvements | 2026-02-20 | Bottom nav, responsive layouts |
| Pagination system | 2026-02-20 | Consistent pagination across list pages |
| Builder prompt improvements | 2026-02-20 | Enhanced agent prompt engineering |
| Agent card redesign | 2026-02-19 | Grid/list views with updated card styling |
| Network settings tab | 2026-02-18 | LAN access QR, Tailscale toggle |
| Tool process manager | 2026-02-18 | Auto-start, compile, lifecycle management |
| Context system | 2026-02-18 | File tree, upload, editor, About You |
| Agent identity system | 2026-02-17 | SOUL.md, USER.md, AGENTS.md, BOOT.md files |
| Skills system | 2026-02-17 | Global skills, per-agent assignment |
| Enhanced tool executor | 2026-02-17 | Improved tool compilation and execution |

### Removed
| Feature | Date | Reason |
|---------|------|--------|
| Design system page | 2026-02-20 | Moved to modal in Settings > Design tab |
| Audit page (standalone) | 2026-02-20 | Merged into Logs page with enhanced filtering |
| Agent manager (old) | 2026-02-20 | Replaced by new spawn/gateway/postbuild architecture |

### Modified
| Feature | Date | Change |
|---------|------|--------|
| Chat interface | 2026-02-20 | Added thread members, compact chat, context window bar |
| Agent system | 2026-02-20 | Added identity files, memory, heartbeat toggle |
| Settings page | 2026-02-20 | Expanded from 5 to 10 tabs |
| Sidebar navigation | 2026-02-20 | Regrouped with Browser, Heartbeat, Context added |

---

## Workflows

_User workflows that are implemented_

### First-Time Setup
1. Navigate to OpenPaw URL (default localhost:41295)
2. Welcome screen introduces the product
3. Create admin account (username 3+ chars, password 8+ chars with uppercase, lowercase, digit)
4. Enter OpenRouter API key (or auto-detect from OPENROUTER_API_KEY env var)
5. Configure server name, bind address, and port
6. System seeds the "builder" (gateway) agent role
7. Redirected to Chat page

### Build a Tool via Chat
1. Open Chat and start a new thread
2. Describe the tool you want (e.g., "Build me a weather API tool")
3. Gateway routes to builder agent
4. Agent creates tool source code, compiles it, and starts it
5. Tool appears on Tools page with endpoints
6. Tool can be called from dashboards, schedules, or other agents

### Create a Dashboard via Chat
1. In Chat, describe the dashboard you want (e.g., "Create a dashboard that monitors my weather tool")
2. Agent creates either a block dashboard (widget grid) or custom dashboard (HTML/JS)
3. Dashboard appears in Dashboards page via dropdown switcher
4. Block dashboards auto-refresh; custom dashboards have full tool API access

### Schedule Automated Tasks
1. Go to Scheduler page
2. Click "New Schedule"
3. Choose type: Tool Action or AI Prompt
4. For Tool Action: select tool, action, and cron schedule
5. For AI Prompt: select agent, write prompt, set cron schedule
6. Enable schedule; view execution history

### Configure Agent Identity
1. Go to Agents page, click an agent
2. If identity not initialized, click "Initialize Identity"
3. Edit identity files: Soul (personality), User (learned preferences), Runbook (procedures), Boot (startup instructions), Heartbeat (periodic checks)
4. Assign skills from global pool
5. Grant tool access (owned or granted)
6. Edit memory notes

### Set Up Browser Automation
1. Go to Browser page
2. Click "New Session"
3. Name the session, optionally assign an owner agent
4. Toggle headless mode on/off
5. Start the session
6. View screenshots, send commands via action bar
7. Take human control when needed

### Configure Heartbeat Monitoring
1. Go to Heartbeat page
2. Enable global heartbeat toggle
3. Set interval, active hours, and timezone
4. Enable heartbeat for specific agents
5. Agents with HEARTBEAT.md content will run periodic check-ins
6. View execution history with cost/token tracking

---

## Technical Constraints

_Implementation decisions that affect future development_

| Constraint | Reason | Impact |
|------------|--------|--------|
| Single SQLite database | Simplicity, single-binary deployment | No horizontal scaling; WAL mode for concurrent reads |
| Go-embedded frontend | Single binary distribution | Frontend changes require rebuild before Go build |
| OpenRouter as LLM provider | Unified API for multiple model providers | Depends on OpenRouter availability; API key required |
| Claude Code CLI for agents | Leverages existing agent tooling | Requires Claude Code CLI installed on host |
| Tools are Go binaries | Consistent compilation and execution | Tools limited to Go language; requires Go toolchain on host |
| OKLCH color system | Modern, perceptually uniform theming | Browser support for oklch() required (modern browsers only) |
| Cookie-based JWT auth | Security (HttpOnly, no XSS exposure) | CSRF tokens required for all mutating requests |
| Port 41295 default | Avoid conflicts with common services | Must be configured if port is in use |
| Playwright for browser | Full browser automation capability | Requires Playwright and browser binaries installed |

---

## Production History

| Date | Version | Changes Summary |
|------|---------|-----------------|
| 2026-02-18 | Initial | Created via farmwork init |
| 2026-02-20 | v0.1.0 | Comprehensive status update from full codebase analysis |

---

## Related Documents

- [GREENFIELD.md](./GREENFIELD.md) - Vision and strategy
- [ONBOARDING.md](./ONBOARDING.md) - First-time user experience
- [USER_GUIDE.md](./USER_GUIDE.md) - Feature documentation
