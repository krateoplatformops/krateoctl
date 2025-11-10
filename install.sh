#!/bin/bash

set -e

REPO="krateoplatformops/krateoctl"
BINARY="krateoctl"

# Usage function
usage() {
  echo "Usage: $0 [version]"
  echo "Example: $0 1.2.3"
  exit 1
}

# Optional version argument
if [[ $# -gt 1 ]]; then
  usage
elif [[ $# -eq 1 ]]; then
  VERSION="$1"
  LATEST_TAG="v$VERSION"  # Assume GitHub tags start with 'v'
else
  # Get latest release tag from GitHub API
  LATEST_TAG=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
  if [[ -z "$LATEST_TAG" ]]; then
    echo "❌ Failed to fetch the latest release tag." && exit 1
  fi
  VERSION="${LATEST_TAG#v}"
fi

# Detect OS
OS="$(uname | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux) OS="linux" ;;
  darwin) OS="darwin" ;;
  msys*|cygwin*|mingw*) OS="windows" ;;
  *) echo "❌ Unsupported OS: $OS" && exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "❌ Unsupported architecture: $ARCH" && exit 1 ;;
esac

# Compose artifact name and URL
EXT="tar.gz"
[[ "$OS" == "windows" ]] && EXT="zip"
ASSET="${BINARY}_${VERSION}_${OS}_${ARCH}.${EXT}"
URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/${ASSET}"

echo "📦 Downloading $ASSET from $LATEST_TAG..."
curl -sL "$URL" -o "$ASSET"

# Create a temporary directory
TMP_DIR=$(mktemp -d)

# Extract
echo "📂 Extracting to $TMP_DIR..."
if [[ "$EXT" == "zip" ]]; then
  unzip -o "$ASSET" -d "$TMP_DIR"
else
  tar -xzf "$ASSET" -C "$TMP_DIR"
fi

# Choose install path
INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  echo "⚠️  No write permission to $INSTALL_DIR. Falling back to \$HOME/.local/bin"
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
  echo "👉 Make sure $INSTALL_DIR is in your PATH"
fi

# Find the binary
BIN_PATH=$(find "$TMP_DIR" -type f -name "$BINARY" -perm -111 | head -n 1)
if [[ -z "$BIN_PATH" ]]; then
  echo "❌ Could not find the '$BINARY' binary after extraction." && exit 1
fi

# Install
echo "🚀 Installing $BINARY to $INSTALL_DIR..."
chmod +x "$BIN_PATH"
mv "$BIN_PATH" "$INSTALL_DIR/$BINARY"

# Cleanup
rm -rf "$ASSET" "$TMP_DIR"

echo "✅ $BINARY $VERSION installed successfully!"
