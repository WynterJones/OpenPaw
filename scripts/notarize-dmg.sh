#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "DMG notarization is only supported on macOS" >&2
    exit 1
fi

: "${APPLE_ID:?APPLE_ID is required}"
: "${APPLE_PASSWORD:?APPLE_PASSWORD is required}"
: "${APPLE_TEAM_ID:?APPLE_TEAM_ID is required}"

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DMG_PATH="${1:-}"
if [[ -z "$DMG_PATH" ]]; then
    RELEASE_VERSION="$(tr -d '[:space:]' < "$PROJECT_ROOT/VERSION")"
    DMG_PATH="$(find "$PROJECT_ROOT/desktop/src-tauri/target/release/bundle/dmg" \
        -maxdepth 1 -type f -name "*_${RELEASE_VERSION}_*.dmg" -print -quit)"
fi
if [[ -z "$DMG_PATH" || ! -f "$DMG_PATH" ]]; then
    echo "No DMG for OpenPaw $(cat "$PROJECT_ROOT/VERSION") found to notarize" >&2
    exit 1
fi

echo "Submitting $(basename "$DMG_PATH") to Apple notary service..."
xcrun notarytool submit "$DMG_PATH" \
    --apple-id "$APPLE_ID" \
    --password "$APPLE_PASSWORD" \
    --team-id "$APPLE_TEAM_ID" \
    --wait

echo "Stapling notarization ticket to $(basename "$DMG_PATH")..."
xcrun stapler staple "$DMG_PATH"
xcrun stapler validate "$DMG_PATH"
spctl --assess --type open --context context:primary-signature --verbose=4 "$DMG_PATH"
