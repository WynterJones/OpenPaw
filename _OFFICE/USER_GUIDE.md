# User Guide

> Living documentation for features and how to use them.
> Grows over time to eventually become help docs.
> Each feature gets a short block with bullet list instructions.

**Last Updated:** 2026-02-20
**Status:** Active
**Features Documented:** 15

---

## Quick Start

_Minimal steps to get started with the product_

1. Run the OpenPaw binary -- it starts on `http://localhost:41295` by default
2. Complete the 4-step setup wizard: welcome, create admin account, enter OpenRouter API key, configure server
3. You land on the Chat page -- type a message to talk to Pounce (the gateway agent)
4. Try: "Build me a tool that checks the weather" -- the agent will create, compile, and start a Go microservice
5. Explore the sidebar to see your tools, dashboards, agents, and more

---

## Features

### Chat
The primary interface for interacting with AI agents. All conversations go through the gateway agent "Pounce," which routes to specialist agents as needed.

**How to use:**
- Click "New Chat" or the + button to start a new thread
- Type a message and press Enter (or click Send) to send
- Type `@` followed by an agent name to bring a specific agent into the conversation
- Type `!!` to search and insert context files inline
- Drag and drop files onto the chat to attach them, or click the attachment button
- Click the members icon to see which agents are in the current thread
- The context window bar at the top shows how much of the token limit is used
- Click "Compact" to summarize and compress long thread histories
- Click the stop button to halt a response mid-generation
- Thread cost and token usage are shown in the thread stats bar

**Tips:**
- The gateway decides which agent is best suited for each message -- you do not need to manually route
- Use @mentions when you specifically want a particular agent to respond
- Attach images, code files, or documents for agents to analyze
- Each thread maintains its own conversation history and agent participation

**Related:** Agents, Context, Skills

---

### Agents
AI agent roles with persistent identity, memory, and configurable capabilities. Each agent has a distinct personality and area of expertise.

**How to use:**
- Go to the Agents page from the sidebar
- The gateway agent "Pounce" is always shown first with a shield badge (cannot be disabled)
- Click "New Agent" to create a specialist agent
- Choose from 6 preset avatars or upload a custom image (PNG, JPEG, WebP)
- Fill in name, description, and system prompt
- Toggle the enable/disable switch to control whether an agent is available for routing
- Click an agent card to open the editor
- Switch between Grid and List views using the toggle in the header

**Tips:**
- The gateway agent's model is configured in Settings > AI Models (Gateway Model)
- Individual agent models can be set in the agent editor (Model Picker)
- Create agents for specific domains: one for code, one for data analysis, one for writing, etc.

**Related:** Chat, Tools, Skills

---

### Agent Editor
Deep configuration for individual agents including identity files, memory, skills, and tool access.

**How to use:**
- Click any agent card on the Agents page to open the editor
- **Identity Files** (tabs at top):
  - **Soul** (SOUL.md): Define the agent's personality, name, tone, and values
  - **User** (USER.md): Information the agent learns about you over time
  - **Runbook** (AGENTS.md): Operating procedures, session rules, response style
  - **Boot** (BOOT.md): Startup instructions run at the beginning of each session
  - **Heartbeat** (HEARTBEAT.md): Periodic self-check instructions (empty = skip heartbeat)
- **Memory tab**: View and edit the agent's persistent memory notes and daily logs
- **Skills section**: Assign global skills to this agent using the skill picker
- **Tools section**: View owned tools (built by this agent) and granted tools; add tool access via the tool picker
- Edit the agent's name, description, model, and avatar from the sidebar panel
- Click "Delete Agent" at the bottom to remove (requires confirmation)

**Tips:**
- Identity files are markdown -- use headings, lists, and formatting freely
- If identity is not initialized, click "Initialize Identity" to create the default files
- Tab shows a * indicator when a file has unsaved changes
- The Tab key inserts spaces in the editor (does not change focus)
- For the gateway agent, the editor has a simplified view: Soul, User, and Heartbeat tabs only

**Related:** Agents, Skills, Tools

---

### Tools
Go-based microservice tools that agents build and manage. Each tool compiles to a binary and runs as a separate process with its own port.

**How to use:**
- Go to the Tools page from the sidebar
- **My Tools tab**: Shows installed tools with status indicators
  - Click a tool card to view details (status, port, PID, endpoints, capabilities)
  - Use action buttons: Compile, Start, Stop, Restart, Enable/Disable, Export, Delete
  - Click the name or description to edit them inline
  - View the integrity panel to verify source hash
- **Library tab**: Browse the tool catalog
  - Filter by category using the dropdown
  - Search for tools by name
  - Click "Install" to add a library tool to your instance
- **Import**: Click the import button and upload a .zip file to import a tool
- **Export**: Click the export button on any tool to download it as .zip
- Switch between Grid and List views; use search and pagination

**Tips:**
- Tools auto-start on server boot if they were previously running
- Ask agents to build tools via Chat -- they handle the entire lifecycle
- Each tool runs on its own port; check the detail view for connection info
- The integrity panel shows source and binary hashes for security verification

**Related:** Chat, Dashboards, Scheduler, Secrets

---

### Dashboards
Visual monitoring and data display. Two types: Block dashboards (widget grids) and Custom dashboards (full HTML/JS apps).

**How to use:**
- Go to the Dashboards page from the sidebar
- Use the dropdown switcher to change between dashboards
- **Block dashboards**: Display widgets in a grid layout; click "Refresh" to update data
- **Custom dashboards**: Run as sandboxed iframes with access to the tool API
- Dashboards are typically created by agents through Chat conversation

**Tips:**
- Ask in Chat: "Create a dashboard that monitors my [tool name]"
- Custom dashboards can call tools via postMessage API (callTool, getTools)
- Custom dashboards automatically receive the current theme colors
- The last-viewed dashboard is remembered between sessions
- Block dashboard widgets pull data from tool endpoints

**Related:** Tools, Chat, Scheduler

---

### Secrets
Encrypted credential storage for API keys, passwords, and sensitive configuration.

**How to use:**
- Go to the Secrets page from the sidebar
- Click "New Secret" to create a credential
- Enter a name, the secret value, and an optional description
- Optionally associate the secret with a specific tool
- Click the rotate icon to update a secret's value
- Click "Test Connection" to verify a secret works (for API keys)
- Delete secrets with the trash icon (requires confirmation)
- Switch between Grid and List views; use search and pagination

**Tips:**
- Secret values are encrypted at rest with a key separate from the JWT secret
- Secrets are never shown in plain text in the UI after creation
- Associate secrets with tools so agents know which credentials are available

**Related:** Tools, Settings

---

### Scheduler
Automated task execution on cron schedules. Run tool actions or AI prompts on a recurring basis.

**How to use:**
- Go to the Scheduler page from the sidebar
- Click "New Schedule" to create a scheduled task
- Choose the schedule type:
  - **Tool Action**: Select a tool and action to call on schedule
  - **AI Prompt**: Select an agent and write a prompt to execute on schedule
- Pick a cron preset or enter a custom cron expression:
  - Every hour, Daily at 9am, Weekly on Monday, Monthly on the 1st
  - Every 15 minutes, Every 30 minutes, or Custom
- Toggle enable/disable for each schedule
- Click "Run Now" to execute immediately
- Click a schedule to view execution history (status, duration, output, errors)
- Delete schedules with the trash icon

**Tips:**
- AI Prompt schedules are powerful for periodic reporting, monitoring, or data collection
- Execution history shows exactly what happened and when
- Dashboard refresh and browser automation can also be scheduled

**Related:** Tools, Agents, Dashboards

---

### Context
File management system for providing persistent reference material to agents during conversations.

**How to use:**
- Go to the Context page from the sidebar
- **Upload files**: Drag and drop files onto the page, or click the upload button
- **Create folders**: Click "New Folder" to organize files hierarchically
- **Edit text files**: Click a text file to open it in the built-in editor
- **View images**: Click an image file to view it in the browser
- **Download binaries**: Click a binary file to download it
- **Move files**: Drag files from the sidebar to different folders
- **Rename/Delete**: Right-click a file or folder for the context menu
- **About You**: The top section of the sidebar is for personal context -- information about you that is included in all agent conversations

**Tips:**
- Use `!!` in Chat to search and insert context files inline into your message
- The "About You" section is particularly useful for giving agents your preferences, role, and working style
- Upload project documentation, API specs, or reference materials for agents to access
- Files are stored on the server and persist across sessions

**Related:** Chat, Agents

---

### Browser
Playwright-based browser automation sessions for web interaction tasks.

**How to use:**
- Go to the Browser page from the sidebar
- Click "New Session" to create a browser session
- Enter a session name and optionally assign an owner agent
- Toggle headless mode (on = no visible browser window on server)
- Click "Start" to launch the browser session
- View the browser via the screenshot viewer
- Use the action bar to send navigation commands
- Click "Take Control" to manually operate the browser (human takeover)
- Click "Release Control" to return control to the agent
- View the action log to see what the browser has done
- Stop or delete sessions when done

**Tips:**
- Browser sessions can be automated via schedules
- Assign an owner agent to let that agent control the browser
- Headless mode is useful for server environments without a display
- Human takeover lets you guide the browser when the agent needs help

**Related:** Agents, Scheduler

---

### Heartbeat Monitor
Periodic agent check-in system. Agents with heartbeat instructions run self-checks on a configurable schedule.

**How to use:**
- Go to the Heartbeat page from the sidebar
- Toggle "Enable Heartbeat" to activate the global system
- Set the check interval (15 min to 24 hours)
- Configure active hours: start time, end time, and timezone
- Toggle heartbeat for individual agents using the switches
- Click "Run Now" next to an agent to trigger an immediate check
- Click an execution entry to see detailed results:
  - Actions taken, output text, errors
  - Cost (USD), input tokens, output tokens
  - Duration (started/finished timestamps)
- Search and paginate through execution history

**Tips:**
- Agents need content in their HEARTBEAT.md file to know what to check
- Set active hours to avoid running checks during off-hours
- Heartbeat costs are tracked per execution for budgeting
- The system auto-starts with the server if previously enabled

**Related:** Agents, Agent Editor (Heartbeat tab), Logs

---

### Skills
Global markdown documents that provide specialized knowledge or instructions to agents.

**How to use:**
- Go to the Skills page from the sidebar
- Click "New Skill" to create a skill document
- Enter a name and write the skill content in markdown
- Use YAML frontmatter for metadata (name, description)
- Edit existing skills by clicking them to open the full-page editor
- Delete skills with the trash icon
- Switch between Grid and List views; use pagination
- To assign a skill to an agent, go to the Agent Editor and use the Skills section

**Tips:**
- Skills are reusable -- write once, assign to multiple agents
- Good skills describe procedures, domain knowledge, or response patterns
- Keep skills focused on one topic or capability each

**Related:** Agents, Agent Editor

---

### Activity Logs
Comprehensive audit trail of all system activity with cost and token tracking.

**How to use:**
- Go to the Logs page from the sidebar
- View the stats cards at the top: Total Cost, Total Tokens, Total Activity
- **Filter by category**: Click category badges to filter (auth, tools, secrets, agents, schedules, dashboards, chat, system, skills, context, browser, heartbeat)
- **Filter by action type**: Click action type badges for more specific filtering (40+ action types)
- **Search**: Type in the search bar to find specific log entries
- **Auto-refresh**: Toggle the auto-refresh switch to keep logs updating in real time
- Navigate pages with the pagination controls (50 entries per page)
- Each log entry shows: timestamp, user, category, action, target, and details

**Tips:**
- Use the cost tracking to monitor AI spending across agents and operations
- The token counts help understand which operations consume the most context
- Filter by "chat" category to see conversation activity and costs
- All significant actions are logged automatically -- no configuration needed

**Related:** Settings, Chat, Agents

---

### Settings
Comprehensive application configuration across 10 tabs.

**How to use:**
- Go to Settings from the sidebar (gear icon at bottom)
- **Profile**: Change your username, update password (requires current password), upload avatar
- **General**: Set app name (shown in header), bind address, port; toggle build confirmation prompts
- **Notifications**: Toggle notification sounds; enable/disable browser push notifications
- **Network**: View LAN access info with QR code for mobile; toggle Tailscale integration
- **AI Models**: Set OpenRouter API key; choose gateway model (routes conversations) and builder model (creates tools/dashboards); set max conversation turns (default 300) and agent timeout (default 60 min)
- **Design**: Pick from 8 accent color presets (Rose, Coral, Amber, Emerald, Teal, Sky, Violet, Slate) or use custom color picker; choose font family (System, Inter, JetBrains Mono, Space Grotesk, DM Sans); preview design system in modal
- **Security**: Set session timeout duration; configure IP allowlist for access control
- **System**: View system info (version, Go version, platform, uptime, DB size, tool count, API key status); export all data; import data from backup
- **About**: View version information (v0.1.0)
- **Danger Zone**: Delete all data (requires typing "DELETE"); delete account (requires typing "DELETE")

**Tips:**
- The gateway model handles routing -- a fast, cheap model (like Haiku) works well here
- The builder model handles tool/dashboard creation -- use a more capable model (like Sonnet or Opus)
- The Network tab QR code lets you quickly connect from your phone on the same LAN
- Export data regularly as a backup strategy
- Design changes apply immediately across the entire interface

**Related:** All features (settings affect everything)

---

### Notifications
In-app notification system for agent activity, schedule results, and system events.

**How to use:**
- The notification bell appears in the header
- A badge shows the count of unread notifications
- Click the bell to view notification list
- Click a notification to mark it as read
- Dismiss notifications you no longer need
- Toggle notification sounds in Settings > Notifications
- Enable browser push notifications for alerts when the tab is not focused

**Tips:**
- Notifications come from agents, scheduled task completions, heartbeat results, and system events
- Enable push notifications if you leave OpenPaw running in the background

**Related:** Settings, Scheduler, Heartbeat

---

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| Enter | Send message in chat |
| Tab (in agent editor) | Insert spaces (does not change focus) |
| Drag & Drop | Upload files in Chat or Context page |
| Right-click (Context page) | Open context menu for rename/delete |
| @ (in chat input) | Open agent mention autocomplete |
| !! (in chat input) | Open context file autocomplete |

---

## FAQ

### Common Questions

**Q: What is the gateway agent "Pounce"?**
A: Pounce is the always-active gateway agent that processes every message. It analyzes your request and either responds directly or routes to a specialist agent. You cannot disable it. Edit its personality via the Gateway editor (click the Pounce card on the Agents page).

**Q: How do I add an API key?**
A: Go to Settings > AI Models and enter your OpenRouter API key. Alternatively, set the `OPENROUTER_API_KEY` environment variable before starting the server. The setup wizard also prompts for this on first run.

**Q: What models can I use?**
A: OpenPaw uses OpenRouter, which provides access to models from Anthropic (Claude), OpenAI (GPT), Google (Gemini), Meta (Llama), and many others. Configure gateway and builder models separately in Settings > AI Models.

**Q: Can I access OpenPaw from my phone?**
A: Yes. Go to Settings > Network to see your LAN address and QR code. Scan the QR code from your phone (must be on the same network). Enable Tailscale for remote access outside your LAN.

**Q: How are secrets stored?**
A: Secrets are encrypted at rest using a separate encryption key from the JWT authentication secret. They are never displayed in plain text after creation. The encryption key can be set via the `OPENPAW_ENCRYPTION_KEY` environment variable.

**Q: What happens if I delete all data?**
A: Settings > Danger Zone > "Delete All Data" removes everything: tools, agents, dashboards, schedules, chat history, secrets, context files, and logs. This is irreversible and requires typing "DELETE" to confirm. Your user account remains but is empty.

**Q: Can agents build tools automatically?**
A: Yes. Describe what you need in Chat and the builder agent will create Go source code, compile it into a binary, and start it as a running microservice. The tool then appears on the Tools page with its endpoints.

**Q: How does the heartbeat system work?**
A: When enabled, agents with content in their HEARTBEAT.md file run periodic self-checks. The interval is configurable (15 min to 24 hours). Agents execute their heartbeat instructions, which can include checking tools, updating dashboards, or running maintenance tasks. Results are logged with cost tracking.

**Q: What is the Context system for?**
A: The Context page stores files that agents can reference during conversations. Upload documentation, API specs, configuration files, or any reference material. Use `!!` in chat to insert context files into your message. The "About You" section provides personal context included in all agent interactions.

**Q: How do custom dashboards work?**
A: Custom dashboards are HTML/JS applications served in a sandboxed iframe. They communicate with OpenPaw via the postMessage API to call tools and receive theme updates. They have access to `callTool` and `getTools` actions for fetching data from your installed tools.

---

## Changelog

| Date | Change |
|------|--------|
| 2026-02-18 | Initial user guide setup |
| 2026-02-20 | Comprehensive documentation of all 15 features from codebase analysis |

---

## Related Documents

- [GREENFIELD.md](./GREENFIELD.md) - Vision and strategy
- [BROWNFIELD.md](./BROWNFIELD.md) - What's already implemented
- [ONBOARDING.md](./ONBOARDING.md) - First-time user experience
