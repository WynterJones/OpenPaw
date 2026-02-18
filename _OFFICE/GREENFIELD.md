# Greenfield Vision

> Your product vision and strategic direction. Focus on WHAT you're building, not HOW.
> This is a living document that adapts as your understanding evolves.

**Project Name:** OpenPaw
**Last Updated:** 2026-02-20
**Status:** Active development
**Confidence:** High

---

## Core Idea

_What is this product in one sentence?_

OpenPaw is a self-hosted, AI-powered internal tool factory that lets you create, manage, and orchestrate specialist AI agents, custom tools, browser automations, and live dashboards from a single conversational interface.

---

## Problem Being Solved

_What pain point or need does this address?_

**The Problem:**
Building and maintaining internal tools, automations, and monitoring dashboards requires juggling multiple platforms, writing boilerplate code, managing deployments, and coordinating between different services. AI assistants exist in isolation -- they can answer questions but cannot build persistent infrastructure, run scheduled tasks, monitor systems, or operate browsers on your behalf.

**Why It Matters:**
Teams and individuals waste enormous time on repetitive operational work: checking services, collecting data, running reports, and building one-off tools. A unified platform where you can describe what you need in natural language and have AI agents build, deploy, and maintain it would collapse the gap between intention and execution. The result is a personal operations center that grows smarter and more capable over time.

---

## The Game Loop (Strategy)

> Treat your product like a game. What keeps users engaged?

### 1. What are they doing?
_The primary action or loop users engage in_

**Core Action:**
Users converse with AI agents in a chat interface. Through conversation, they create tools (Go-based microservices), build dashboards to visualize data, schedule automated tasks, set up browser automations, and configure agent behaviors. Each conversation can produce persistent, running infrastructure.

### 2. What's stopping them?
_Friction, obstacles, or pain points_

**Current Blockers:**
- Learning the agent system (which agent does what, how to route requests)
- Understanding the tool lifecycle (compile, start, configure endpoints)
- Configuring integrations (API keys, secrets, network access)

### 3. Why are they doing it?
_Underlying motivation and rewards_

**User Motivation:**
The reward is watching a single conversation turn into a running tool, a live dashboard, or an automated workflow. The system remembers, learns, and accumulates capability. Each tool built expands what the agents can do. The factory compounds -- agents use tools to build better tools.

---

## Vision Loop

```
Chat with Agent --> Agent builds/configures something --> See it running (tool, dashboard, schedule) --> Use the output, request more --> Loop back
```

---

## Strategic Pillars

_Key principles that guide product decisions_

1. **Single Binary, Zero Ops** - Everything ships as one Go binary with embedded frontend. No Docker, no cloud dependencies, no microservice sprawl. SQLite for data. Run it anywhere.
2. **Conversation-First Creation** - Every feature should be accessible through natural language in chat. The UI is for monitoring and fine-tuning, not for primary creation. Agents build the tools, dashboards, and automations.
3. **Agent Identity and Memory** - Agents are not stateless prompt templates. Each has a soul (personality), memory (learned context), skills (capabilities), and tools (access). They remember users and accumulate knowledge over sessions.
4. **Compounding Factory** - Tools that agents build become available to agents. Dashboards visualize tool output. Schedules automate tool calls. Browser sessions extend reach to the web. Each piece makes the whole system more capable.
5. **Local-First Privacy** - All data stays on the user's machine. Encrypted secrets, no telemetry, no cloud accounts required beyond the LLM API key. Network access is opt-in (LAN, Tailscale).

---

## Success Metrics

| Metric | Current | Target | Notes |
|--------|---------|--------|-------|
| Active tools running | Varies per install | 5+ per user | Tools that agents have built and are serving |
| Agent roles configured | 1 (Gateway) seeded | 3-5 specialist agents | Users should create domain-specific agents |
| Dashboards in use | 0 default | 2+ per user | Both block and custom dashboard types |
| Scheduled tasks | 0 default | 3+ per user | Mix of tool actions and AI prompts |
| Time to first tool | N/A | < 15 minutes | From fresh install to first running tool |
| Chat threads | 0 | Regular daily use | Indicates the system is the user's operations hub |

---

## Strategy Changelog

| Date | Change | Previous | Reason |
|------|--------|----------|--------|
| 2026-02-18 | Initial vision setup | - | Created via farmwork init |
| 2026-02-20 | Filled vision from codebase analysis | Template placeholders | Comprehensive code review of all features |

---

## Related Documents

- [BROWNFIELD.md](./BROWNFIELD.md) - What's already implemented
- [ONBOARDING.md](./ONBOARDING.md) - First-time user experience
- [USER_GUIDE.md](./USER_GUIDE.md) - Feature documentation
