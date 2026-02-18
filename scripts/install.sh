#!/usr/bin/env sh
set -e

REPO="WynterJones/OpenPaw"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS="$(uname -s)"
case "$OS" in
    Darwin) OS="darwin" ;;
    Linux)  OS="linux" ;;
    *)      echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)  ARCH="x64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)             echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

BINARY_NAME="openpaw-${OS}-${ARCH}"

echo "Detecting system: ${OS}/${ARCH}"

# Get latest release tag
echo "Fetching latest release..."
TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$TAG" ]; then
    echo "Error: Could not determine latest release"
    exit 1
fi

echo "Latest release: $TAG"

URL="https://github.com/${REPO}/releases/download/${TAG}/${BINARY_NAME}"

echo "Downloading ${BINARY_NAME}..."
TMPFILE=$(mktemp)
curl -fsSL "$URL" -o "$TMPFILE"

chmod +x "$TMPFILE"

echo "Installing to ${INSTALL_DIR}/openpaw..."
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMPFILE" "${INSTALL_DIR}/openpaw"
else
    sudo mv "$TMPFILE" "${INSTALL_DIR}/openpaw"
fi

echo ""
echo "OpenPaw ${TAG} installed successfully!"
echo "Run 'openpaw' to get started."
