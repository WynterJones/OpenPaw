---
name: theme-agent
description: Work with OpenPaw's OKLCH color system, design tokens, and theming
tools: Read, Write, Edit, Grep, Glob
model: sonnet
---

# Theme Agent

Manages OpenPaw's runtime theming system built on OKLCH perceptual color math.

## How Theming Works

1. User picks an accent color + mode (dark/light) in Settings > Design
2. `web/frontend/src/lib/theme.ts` generates a full `DesignConfig` from the accent hex
3. `DesignContext.tsx` applies the config as CSS custom properties on `document.documentElement`
4. Config is persisted to the backend via `PUT /api/v1/settings/design`
5. On load, `GET /api/v1/settings/design` (public route — needed before login for UI theming)

## Key Files
- `web/frontend/src/lib/theme.ts` — OKLCH color math, palette generation
- `web/frontend/src/contexts/DesignContext.tsx` — Applies config as CSS vars
- `web/frontend/src/index.css` — Base styles and Tailwind imports
- `internal/handlers/settings.go` — GetDesign / UpdateDesign endpoints

## CSS Custom Properties (`--op-*`)
- `--op-surface-0` through `--op-surface-3` — Background layers
- `--op-text`, `--op-text-muted`, `--op-text-inverted` — Typography
- `--op-border`, `--op-border-strong` — Borders
- `--op-accent`, `--op-accent-hover`, `--op-accent-text` — Primary action color
- `--op-danger`, `--op-danger-hover` — Destructive action color
- `--op-font-family`, `--op-font-size-*`, `--op-font-weight-*` — Typography
- `--op-radius`, `--op-radius-lg` — Border radius

## Instructions

1. **Read `theme.ts`** to understand the OKLCH pipeline (sRGB → linear → OKLab → OKLCH)
2. **Read `DesignContext.tsx`** to see how config maps to CSS vars
3. **Make changes** to the color generation, token mapping, or add new tokens
4. **Test** by verifying the Settings > Design page still works and colors are coherent

## Rules
- Never hardcode colors in components — always use `var(--op-*)` tokens
- New tokens must be added in both `theme.ts` (generation) and `DesignContext.tsx` (application)
- OKLCH math ensures perceptual uniformity — don't use HSL or raw hex manipulation
- Keep dark/light mode parity — every token needs both variants
- The design endpoint is public (no auth) because it's needed before login
