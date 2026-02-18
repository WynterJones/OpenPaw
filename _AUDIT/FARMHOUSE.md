# Farmwork Farmhouse

> Central command for the Farmwork agentic harness.
> Updated automatically by `the-farmer` agent during `/push` or via "open the farm" phrase.

**Last Updated:** 2026-02-20
**Score:** 10/10
**Status:** Fully operational

---

## Quick Metrics

| Metric | Count |
|--------|-------|
| Commands | 1 |
| Agents | 20 |
| Skills | 6 |
| Research Docs | 4 |
| Test Files | 221 |
| Completed Issues | 99 |
| Go Files | 95 |
| React Components (TSX) | 56 |
| Migrations | 20 |
| API Routes | 156 |
| Lines of Go | 21,422 |
| Lines of TS/TSX | 14,026 |

---

## How to get 10/10

All Claude Code commands and agents are documented, phrase triggers are tested and working, issue tracking via beads is active, justfile navigation covers all project areas, and the CLAUDE.md instructions are complete and accurate.

---

## Commands (`.claude/commands/`)

| Command | Description |
|---------|-------------|
| `/push` | Lint, test, build, commit, push, update metrics |

---

## Agents (`.claude/agents/`)

| Agent | Purpose |
|-------|---------|
| `the-farmer` | Audit and update FARMHOUSE.md metrics |
| `code-quality` | Code review, DRY violations, complexity, naming |
| `security-auditor` | OWASP vulnerability scanning |
| `performance-auditor` | Performance anti-patterns |
| `accessibility-auditor` | WCAG 2.1 compliance, alt text, contrast |
| `code-cleaner` | Remove dead code, comments, console.logs |
| `i18n-locale-translator` | Translate UI text to locales |
| `idea-gardener` | Manage Idea Garden and Compost |
| `researcher` | Systematic research before planning |
| `strategy-agent` | Analyze GREENFIELD.md vision and strategy (what/stopping/why) |
| `brownfield-agent` | Track implemented features in BROWNFIELD.md |
| `onboarding-agent` | Document onboarding elements (tours, tooltips, modals) |
| `user-guide-agent` | Create feature documentation for help docs |
| `fullstack-agent` | End-to-end features: migration to handler to route to page to nav |
| `go-handler-agent` | Go HTTP handlers following chi/helpers/audit patterns |
| `page-builder` | React pages with design tokens and existing component library |
| `migration-agent` | SQLite migrations following numbered convention |
| `prompt-engineer` | AI agent system prompts and gateway routing logic |
| `theme-agent` | OKLCH color system, design tokens, theming pipeline |
| `websocket-agent` | WebSocket hub extensions and real-time features |

---

## Phrase Commands

### Farmwork Phrases

| Phrase | Action |
|--------|--------|
| `open the farm` | Audit systems, update FARMHOUSE.md |
| `count the herd` | Full inspection + dry run (no push) |
| `go to market` | i18n scan + accessibility audit |
| `close the farm` | Execute /push |

### Plan Phrases

| Phrase | Action |
|--------|--------|
| `make a plan for...` | Create plan in _PLANS/ |
| `let's implement...` | Load plan, create Epic |

### Idea Phrases

| Phrase | Action |
|--------|--------|
| `I have an idea for...` | Add idea to GARDEN.md |
| `let's plan this idea...` | Graduate idea to _PLANS/ |
| `compost this...` | Move idea to COMPOST.md |

### Research Phrases

| Phrase | Action |
|--------|--------|
| `let's research...` | Research topic, save to _RESEARCH/ |
| `update research on...` | Refresh existing research |
| `show research on...` | Display research summary |

### Office Phrases

| Phrase | Action |
|--------|--------|
| `setup office` | Interactive guided setup: GREENFIELD, ONBOARDING, USER_GUIDE |
| `go to production` | Update BROWNFIELD.md, check alignment, note doc impacts |

---

## Issue Tracking (`.beads/`)

Using `bd` CLI for issue management:

```bash
bd ready              # Find available work
bd create "..." -t task -p 2  # Create issue
bd update <id> --status in_progress  # Claim work
bd close <id>         # Complete work
```

---

## Research Library (`_RESEARCH/`)

| Document | Last Researched | Status |
|----------|----------------|--------|
| Claude Code CLI JSON Streaming | 2026-02-18 | Fresh |
| Claude Code CLI Permissions | 2026-02-18 | Fresh |
| Claude Code SDK Compaction | 2026-02-18 | Fresh |
| OpenRouter Balance API | 2026-02-20 | Fresh |

---

## Beads Issue History

**Total Completed Issues:** 99

---

## Audit History

| Date | Changes |
|------|---------|
| 2026-02-20 | Full audit: 20 agents, 6 skills, 221 tests, 99 issues closed, 4 research docs (all fresh), score 10/10 |
| 2026-02-18 | Initial FARMHOUSE setup via Farmwork CLI |
