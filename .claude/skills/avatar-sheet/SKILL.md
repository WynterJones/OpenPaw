---
name: avatar-sheet
description: Slice a generated grid/contact sheet of avatar images into individual avatar files and register them in the app. Use when the user supplies a sheet of avatars (a 3x4 grid of character tiles, etc.) and wants them added as new avatar options, or says "cut these up", "add these avatars", "slice this sheet".
allowed-tools: Bash, Read, Edit, Glob, Grep
---

# Avatar Sheet Skill

Turns an AI-generated grid of avatar tiles into numbered avatar files the app can
offer, then wires them into the preset lists.

## When to Use
- The user drops one or more image sheets (grids of character tiles) and wants
  them available as agent avatars.
- Adding to the existing `/avatars/avatar-N.webp` set.

## Where things live

| Thing | Path |
|---|---|
| Avatar files | `web/frontend/public/avatars/avatar-<N>.webp` |
| Format | 256x256 WebP, full-bleed square |
| Preset list (agent edit) | `web/frontend/src/pages/AgentEdit.tsx` — `PRESET_AVATARS` |
| Preset list (first-run setup) | `web/frontend/src/pages/Setup.tsx` — `PRESET_AVATARS` |
| Quick-pick list (create modal) | `web/frontend/src/pages/Agents.tsx` — hardcoded first few |

Both `PRESET_AVATARS` are `Array.from({ length: N }, ...)` — adding avatars means
bumping that single `N` in **both** files, not editing a list.

## Steps

1. **Inspect the sheet.** Read the image to count columns/rows and note the
   subject order. Do not assume 3x4.

2. **Find the next free index:**
   ```bash
   ls web/frontend/public/avatars/ | sed 's/[^0-9]//g' | sort -n | tail -1
   ```

3. **Cut**, one invocation per sheet, continuing the numbering:
   ```bash
   bash .claude/skills/avatar-sheet/cut-sheet.sh <sheet.png> <cols> <rows> \
     web/frontend/public/avatars <start-index>
   ```
   Cut to a scratch directory first if you want to eyeball before installing.

4. **Verify visually — do not skip this.** Montage the output and Read it:
   ```bash
   magick montage <dir>/avatar-{46..57}.webp -tile 3x4 -geometry 160x160+3+3 \
     -background '#0a0a0a' /tmp/check.png
   ```
   Confirm the order matches the source sheet and nothing is clipped. To catch
   gutter slivers, montage on `-background '#00ff00'` — any green bleeding into a
   tile means the crop ran past the card edge.

5. **Register**: bump `length: N` in `AgentEdit.tsx` and `Setup.tsx` to the new
   highest index.

6. **Build**: `cd web/frontend && npx tsc -b && npm run build` (avatars are
   served from `public/`, but the frontend must be rebuilt for the embedded
   binary to see them).

## Why the script detects geometry instead of dividing the canvas

**These sheets are not perfect grids.** Generated sheets vary in outer margin and
gutter width, and individual tiles differ by a pixel or two in size and position.
Slicing by `width/cols` puts a sliver of the neighbouring card in every crop and
drifts further across the sheet. The script instead thresholds the image, finds
each card with connected-component analysis, and crops from each card's own
measured origin.

Two traps that produced visibly wrong output, both now handled:

- **`area-threshold` MERGES, it does not filter.** Components below the threshold
  are absorbed into their surroundings, so a value near one cell's area collapses
  the entire grid into a single blob. Keep it well under a cell (the script uses
  cell/8 and sweeps cell/16, cell/4).

- **Rows need a tolerance, not an exact sort.** Tiles in one visual row can sit
  at y=37 and y=38. Sorting by `(y, x)` then puts the y=38 middle tile *after*
  the y=37 right-hand tile and the row comes out shuffled — subtly wrong output
  that compiles and looks plausible. The script buckets y into rows with a
  half-tile tolerance before ordering by `(row, x)`.

The script refuses to write anything unless it detects exactly `cols*rows` tiles,
so a bad threshold fails loudly instead of producing misaligned avatars.

## Notes
- Tiles keep their rounded-corner artwork; the corners are dark and the UI
  renders avatars inside a rounded container, so they read as full-bleed.
- `-resize NxN!` forces the square: source tiles are near-square but not exact,
  and avatars must all match at 256x256.
