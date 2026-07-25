#!/usr/bin/env bash
#
# cut-sheet.sh — slice a grid "contact sheet" of avatars into individual files.
#
#   cut-sheet.sh <sheet-image> <cols> <rows> <out-dir> <start-index> [size]
#
# Tile geometry is DETECTED, not assumed: generated sheets vary in outer margin
# and gutter width, so hardcoding offsets silently shifts every crop. The script
# thresholds the sheet, finds the tile blobs with connected-component analysis,
# and requires exactly cols*rows of them before cutting anything.
#
# Output is <out-dir>/avatar-<n>.webp at <size>x<size> (default 256), numbered
# from <start-index>, in reading order (left→right, top→bottom).
#
# Exits non-zero without writing files if detection disagrees with cols*rows.

set -euo pipefail

if [ "$#" -lt 5 ]; then
  sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
  exit 64
fi

SHEET="$1"
COLS="$2"
ROWS="$3"
OUT_DIR="$4"
START="$5"
SIZE="${6:-256}"

command -v magick >/dev/null || { echo "error: ImageMagick 7 (magick) not found" >&2; exit 69; }
[ -f "$SHEET" ] || { echo "error: no such sheet: $SHEET" >&2; exit 66; }

EXPECTED=$((COLS * ROWS))
# Command substitution, not `read < <(...)`: identify emits no trailing newline,
# so read would hit EOF, return non-zero, and abort under `set -e`.
SHEET_DIMS=$(magick identify -format '%w %h' "$SHEET")
SHEET_W=${SHEET_DIMS%% *}
SHEET_H=${SHEET_DIMS##* }

CELL_AREA=$(( (SHEET_W / COLS) * (SHEET_H / ROWS) ))

# Detect tiles by thresholding the sheet and taking the light blobs. Sheets vary
# in how bright the card interiors are, so sweep a few thresholds and accept the
# first that yields exactly cols*rows tiles instead of trusting one magic number.
#
# area-threshold must stay well BELOW a cell: connected-components MERGES
# components under the threshold into their surroundings, so an over-large value
# collapses the whole grid into a single blob rather than filtering noise.
detect_boxes() {
  local thresh="$1" area_min="$2"
  magick "$SHEET" -colorspace Gray -threshold "${thresh}%" -morphology Close Disk:3 \
    -define connected-components:verbose=true \
    -define connected-components:area-threshold="$area_min" \
    -connected-components 8 null: 2>/dev/null |
    awk '$NF ~ /gray\(255\)/ && $2 ~ /^[0-9]+x[0-9]+\+[0-9]+\+[0-9]+$/ { print $2 }'
}

BOXES=""
COUNT=0
for THRESH in 12 10 15 8 20 25 5; do
  for DIV in 8 16 4; do
    CANDIDATE=$(detect_boxes "$THRESH" $((CELL_AREA / DIV)))
    N=$(printf '%s\n' "$CANDIDATE" | grep -c . || true)
    if [ "$N" -eq "$EXPECTED" ]; then
      BOXES="$CANDIDATE"
      COUNT="$N"
      echo "detected $N tiles (threshold ${THRESH}%, area>${CELL_AREA}/${DIV})"
      break 2
    fi
    [ "$N" -gt "$COUNT" ] && COUNT="$N"
  done
done

if [ -z "$BOXES" ]; then
  echo "error: could not resolve a ${COLS}x${ROWS} grid in $SHEET (best guess: $COUNT tiles)" >&2
  echo "       check the grid size, or that tiles are lighter than their gutters" >&2
  exit 65
fi

# Use the smallest detected width/height for every crop. Blobs bleed a pixel or
# two into the gutter depending on the artwork; the minimum is the size that is
# guaranteed to stay inside all of them.
TILE_DIMS=$(printf '%s\n' "$BOXES" | awk -F'[x+]' '
  NR == 1 { w = $1; h = $2 }
  { if ($1 < w) w = $1; if ($2 < h) h = $2 }
  END { print w, h }')
TILE_W=${TILE_DIMS%% *}
TILE_H=${TILE_DIMS##* }

mkdir -p "$OUT_DIR"

# Reading order (left→right, top→bottom). Tiles in one visual row can differ by
# a pixel or two in Y, so a plain `sort -k Y,X` interleaves columns — a tile at
# y=38 sorts after its neighbour at y=37 and the row comes out shuffled. Bucket
# Y into rows with a half-tile tolerance first, then sort by (row, x).
INDEX="$START"
while read -r X Y; do
  OUT="$OUT_DIR/avatar-${INDEX}.webp"
  magick "$SHEET" -crop "${TILE_W}x${TILE_H}+${X}+${Y}" +repage \
    -resize "${SIZE}x${SIZE}!" -quality 90 "$OUT"
  echo "$OUT  (from +${X}+${Y} ${TILE_W}x${TILE_H})"
  INDEX=$((INDEX + 1))
done < <(printf '%s\n' "$BOXES" |
  awk -F'[x+]' '{ print $3, $4 }' |
  sort -k2,2n |
  awk -v tol="$((TILE_H / 2))" '
    NR == 1 { row = 0; base = $2 }
    NR > 1 && ($2 - base) > tol { row++; base = $2 }
    { print row, $1, $2 }' |
  sort -k1,1n -k2,2n |
  awk '{ print $2, $3 }')

echo "wrote $EXPECTED tiles: avatar-${START}..avatar-$((INDEX - 1)) @ ${SIZE}x${SIZE}"
