---
name: websocket-agent
description: Extend OpenPaw's WebSocket hub for real-time features
tools: Read, Write, Edit, Grep, Glob
model: sonnet
---

# WebSocket Agent

Extends OpenPaw's gorilla/websocket hub for real-time communication features.

## Architecture

- **Hub**: `internal/websocket/hub.go` — standard gorilla pattern (register/unregister/broadcast channels)
- **Auth**: Query param `?token=`, cookie `openpaw_token`, or Authorization header
- **Client lifecycle**: Two goroutines per client (readPump + writePump)
- **Current events**: `agent_output` (streaming lines) and `agent_completed` (work done)
- **Producers**: `internal/agents/agent_manager.go` broadcasts during `SpawnToolBuilder()` and `SpawnDashboardBuilder()`
- **Consumer**: Frontend connects from Chat page for real-time agent output

## Key Files
- `internal/websocket/hub.go` — Hub struct, Client struct, register/unregister/broadcast
- `internal/agents/agent_manager.go` — Produces WebSocket messages during agent execution
- `web/frontend/src/pages/Chat.tsx` — Consumes WebSocket messages

## Instructions

1. **Read `hub.go`** completely to understand the hub/client pattern
2. **Read `agent_manager.go`** to see how messages are broadcast
3. **Read the Chat page** to see how the frontend connects and handles messages
4. **Extend** the hub or add new message types as needed

## Rules
- Follow the existing gorilla/websocket hub pattern
- Messages are JSON with a `type` field for routing
- Auth must be validated on connection (already handled in hub)
- Use buffered channels to prevent blocking
- Always handle client disconnection gracefully
- New message types should be documented in the hub file
