#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 1 ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 0.2.0"
    exit 1
fi

NEW_VERSION="$1"

# Validate semver format
if ! echo "$NEW_VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "Error: Version must be in semver format (e.g., 0.2.0)"
    exit 1
fi

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "Bumping version to $NEW_VERSION..."

# 1. VERSION file
echo "$NEW_VERSION" > "$PROJECT_ROOT/VERSION"
echo "  Updated VERSION"

# 2. Tauri config
TAURI_CONF="$PROJECT_ROOT/desktop/src-tauri/tauri.conf.json"
if [ -f "$TAURI_CONF" ]; then
    sed -i '' "s/\"version\": \"[^\"]*\"/\"version\": \"$NEW_VERSION\"/" "$TAURI_CONF" 2>/dev/null || \
    sed -i "s/\"version\": \"[^\"]*\"/\"version\": \"$NEW_VERSION\"/" "$TAURI_CONF"
    echo "  Updated tauri.conf.json"
fi

# 3. Cargo.toml (desktop)
CARGO_TOML="$PROJECT_ROOT/desktop/src-tauri/Cargo.toml"
if [ -f "$CARGO_TOML" ]; then
    sed -i '' "s/^version = \"[^\"]*\"/version = \"$NEW_VERSION\"/" "$CARGO_TOML" 2>/dev/null || \
    sed -i "s/^version = \"[^\"]*\"/version = \"$NEW_VERSION\"/" "$CARGO_TOML"
    echo "  Updated Cargo.toml"
fi

# 4. npm packages
for pkg in openpaw darwin-arm64 darwin-x64 linux-x64 linux-arm64 win32-x64; do
    PKG_JSON="$PROJECT_ROOT/npm/$pkg/package.json"
    if [ -f "$PKG_JSON" ]; then
        sed -i '' "s/\"version\": \"[^\"]*\"/\"version\": \"$NEW_VERSION\"/" "$PKG_JSON" 2>/dev/null || \
        sed -i "s/\"version\": \"[^\"]*\"/\"version\": \"$NEW_VERSION\"/" "$PKG_JSON"
        echo "  Updated npm/$pkg/package.json"
    fi
done

echo ""
echo "Version bumped to $NEW_VERSION"
echo ""
echo "Next steps:"
echo "  git add -A && git commit -m \"Bump version to $NEW_VERSION\""
echo "  git tag v$NEW_VERSION && git push --tags"
