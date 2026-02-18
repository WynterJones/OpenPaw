# User Onboarding

> Living document for first-time user experience: tours, popups, modals, tooltips, and progressive disclosure.
> Track what users see when they first use your product.

**Last Updated:** 2026-02-20
**Status:** Active
**Onboarding Steps:** 4

---

## Onboarding Flow Overview

```
First Visit --> Setup Wizard (4 steps) --> Auto-login --> Chat (empty state) --> First conversation --> Agent builds something --> Success moment
```

---

## Onboarding Elements

### Welcome Experience
_What does the user see immediately after signup/first visit?_

| Element | Type | Content | Status |
|---------|------|---------|--------|
| Setup Step 0: Welcome | Full-page screen | Large app icon, "OpenPaw" title, "AI-powered internal tool factory" subtitle, "Get started" button | Implemented |
| Setup Step 1: Admin Account | Form | Username (3+ chars), password (8+ chars, requires uppercase + lowercase + digit), confirm password | Implemented |
| Setup Step 2: API Key | Form | OpenRouter API key input, auto-detects OPENROUTER_API_KEY env var, "Get a free API key" link | Implemented |
| Setup Step 3: Server Config | Form | App name (defaults to "OpenPaw"), bind address (127.0.0.1), port (41295) | Implemented |
| Post-Setup Redirect | Auto-redirect | After wizard completion, auto-logs in user and redirects to Chat page | Implemented |

### Guided Tours
_Step-by-step tours that walk users through features_

| Tour Name | Steps | Trigger | Status |
|-----------|-------|---------|--------|
| N/A | - | - | Not implemented. Users are guided via empty states and chat suggestions instead. |

### Tooltips & Hints
_Contextual help that appears on specific elements_

| Element | Tooltip Text | Trigger | Status |
|---------|--------------|---------|--------|
| Refresh button (Dashboards) | "Refresh data" | Hover / aria-label | Implemented |
| Dashboard switcher | "Select dashboard" | aria-label | Implemented |
| Sidebar collapse button | "Toggle sidebar" | Hover | Implemented |
| Notification bell | Unread count badge | Persistent when unread > 0 | Implemented |
| Tool status indicators | Status text (running/stopped/error) | Visual badge | Implemented |
| Agent enable/disable | Toggle with visual state | Click | Implemented |
| Context window bar | Shows percentage of token limit used | Visual in chat | Implemented |
| Gateway shield badge | Indicates always-active gateway agent | Visual on agent card | Implemented |

### Modals & Popups
_Modal dialogs that appear during onboarding and regular use_

| Modal Name | Purpose | Trigger | Status |
|------------|---------|---------|--------|
| Create Agent | Name, description, system prompt, avatar selection (6 presets + upload) | "New Agent" button on Agents page | Implemented |
| Initialize Identity | First-time identity file setup for an agent | Click on uninitialized agent | Implemented |
| Create Schedule | New schedule form (type selector, cron, tool/agent selection) | "New Schedule" button on Scheduler page | Implemented |
| Create Secret | Name, value, description for encrypted credential | "New Secret" button on Secrets page | Implemented |
| Create Skill | Name and markdown content for global skill | "New Skill" button on Skills page | Implemented |
| Create Browser Session | Name, owner agent, headless toggle | "New Session" button on Browser page | Implemented |
| Create Folder | Folder name for context file tree | "New Folder" button on Context page | Implemented |
| Delete Confirmation | Type "DELETE" to confirm destructive actions | Delete all data / Delete account in Settings | Implemented |
| Delete Agent Confirmation | Confirm agent deletion | Delete button on agent edit page | Implemented |
| Design System Preview | Live preview of all design tokens and components | Preview button in Settings > Design tab | Implemented |
| Model Picker | Searchable list of available LLM models | Model selection in agent edit | Implemented |
| Skill Picker | Select global skills to assign to an agent | "Add Skill" in agent edit | Implemented |
| Tool Picker | Grant tool access to an agent | "Add Tool" in agent edit | Implemented |
| Execution Detail | Expanded view of schedule or heartbeat execution | Click execution row | Implemented |
| Thread Members | List of agents participating in chat thread | Members button in chat | Implemented |
| Tool Import | Upload .zip file to import a tool | "Import" button on Tools page | Implemented |

### Empty States
_What users see before they have data_

| Screen | Empty State Message | CTA | Status |
|--------|---------------------|-----|--------|
| Chat (no threads) | "Start a conversation" / thread list empty | "New Chat" button | Implemented |
| Agents (beyond gateway) | Only gateway "Pounce" card shown | "New Agent" button | Implemented |
| Tools (My Tools) | "No tools yet" with wrench icon | "Create your first tool by asking in Chat" | Implemented |
| Tools (Library) | "No tools in the library yet" | Library catalog auto-populates | Implemented |
| Dashboards (none) | "No dashboards yet" with layout icon | 'Create your first dashboard by asking in Chat. Try: "Create a dashboard that monitors my weather tool"' | Implemented |
| Dashboards (no widgets) | "No widgets yet" | "This dashboard has no widgets configured. Update it by asking in Chat." | Implemented |
| Secrets (none) | "No secrets yet" with key icon | "New Secret" button | Implemented |
| Scheduler (none) | "No schedules yet" with clock icon | "New Schedule" button | Implemented |
| Skills (none) | "No skills yet" with sparkles icon | "New Skill" button | Implemented |
| Context (no files) | Empty file tree sidebar | Upload button or drag-and-drop area | Implemented |
| Browser (no sessions) | "No browser sessions" with globe icon | "New Session" button | Implemented |
| Heartbeat (no executions) | Empty execution list | Enable heartbeat toggle + enable per-agent | Implemented |
| Logs (no activity) | Empty log table | Activity auto-populates from system use | Implemented |

---

## Success Metrics

| Metric | Current | Target |
|--------|---------|--------|
| Onboarding completion rate | ~100% (4 required steps) | 100% |
| Time to first value | ~5 min (setup + first chat) | < 5 min |
| Drop-off points | Step 2 (API key, if user doesn't have one) | Minimize via env var detection |

---

## Onboarding Gaps and Opportunities

_Areas where onboarding could be improved_

| Gap | Description | Priority |
|-----|-------------|----------|
| No guided tour | New users must explore the sidebar on their own after setup | Medium |
| Chat doesn't pre-seed suggestions | First chat thread could show example prompts (e.g., "Build me a...", "Create a dashboard for...") | Medium |
| Tool lifecycle not explained | Users may not understand compile > start > use flow | Low |
| Agent identity not explained | The Soul/User/Runbook/Boot/Heartbeat file system is complex for newcomers | Medium |
| No onboarding checklist | Could show a progress checklist (create agent, build tool, create dashboard, set schedule) | Low |
| Context system purpose unclear | Users may not understand why/when to upload context files | Low |

---

## Changelog

| Date | Change | Reason |
|------|--------|--------|
| 2026-02-18 | Initial onboarding setup | Created via farmwork init |
| 2026-02-20 | Comprehensive update from codebase analysis | Documented all setup steps, empty states, modals, tooltips, and gaps |

---

## Related Documents

- [GREENFIELD.md](./GREENFIELD.md) - Vision and strategy
- [BROWNFIELD.md](./BROWNFIELD.md) - What's already implemented
- [USER_GUIDE.md](./USER_GUIDE.md) - Feature documentation
