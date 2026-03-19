#!/bin/sh
# Trokky CLI installer
# Usage: curl -sSL https://raw.githubusercontent.com/Trokky/cli/main/install.sh | sh

set -e

REPO="Trokky/cli"
BINARY="trokky"
INSTALL_DIR="/usr/local/bin"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Error: Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  darwin|linux) ;;
  *) echo "Error: Unsupported OS: $OS (use Windows instructions from README)"; exit 1 ;;
esac

# Get latest version
echo "Fetching latest version..."
VERSION=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
if [ -z "$VERSION" ]; then
  echo "Error: Could not determine latest version"
  exit 1
fi
VERSION_NUM=$(echo "$VERSION" | sed 's/^v//')

echo "Installing ${BINARY} ${VERSION} (${OS}/${ARCH})..."

# Download and extract
URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
TMP=$(mktemp -d)
curl -sL "$URL" | tar xz -C "$TMP"

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP/$BINARY" "$INSTALL_DIR/$BINARY"
else
  echo "Need sudo to install to $INSTALL_DIR"
  sudo mv "$TMP/$BINARY" "$INSTALL_DIR/$BINARY"
fi

rm -rf "$TMP"

echo "Installed ${BINARY} ${VERSION} to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "Run 'trokky --help' to get started."
