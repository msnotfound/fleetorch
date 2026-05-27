#!/bin/sh
# fleetorch installer — detects OS/arch and downloads the latest release binary.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/msnotfound/fleetorch/main/scripts/install.sh | sh
#
# Environment overrides:
#   FLEETORCH_VERSION   — pin a specific tag (default: latest)
#   FLEETORCH_BIN_DIR   — install destination (default: ~/.local/bin, falls back to /usr/local/bin)

set -eu

REPO="msnotfound/fleetorch"
VERSION="${FLEETORCH_VERSION:-latest}"

die() { printf 'error: %s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*"; }

# Detect OS
uname_os=$(uname -s 2>/dev/null || echo unknown)
case "$uname_os" in
    Linux) OS=linux ;;
    Darwin) OS=macos ;;
    MINGW*|MSYS*|CYGWIN*) die "Windows detected. Please download the .zip from https://github.com/$REPO/releases manually." ;;
    *) die "unsupported OS: $uname_os" ;;
esac

# Detect arch
uname_arch=$(uname -m 2>/dev/null || echo unknown)
case "$uname_arch" in
    x86_64|amd64) ARCH=x86_64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) die "unsupported arch: $uname_arch" ;;
esac

# Resolve version
if [ "$VERSION" = "latest" ]; then
    VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
        | grep -m1 '"tag_name"' \
        | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
    [ -n "$VERSION" ] || die "could not resolve latest version. Set FLEETORCH_VERSION manually."
fi

# Strip leading v for archive name
VERSION_NUM="${VERSION#v}"

ARCHIVE="fleetorch_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/${VERSION}/${ARCHIVE}"

# Pick install dir
if [ -n "${FLEETORCH_BIN_DIR:-}" ]; then
    BIN_DIR="$FLEETORCH_BIN_DIR"
elif [ -w "$HOME/.local/bin" ] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
    BIN_DIR="$HOME/.local/bin"
elif [ -w /usr/local/bin ]; then
    BIN_DIR=/usr/local/bin
else
    die "no writable install dir. Set FLEETORCH_BIN_DIR=/path/to/dir."
fi

# Download + extract
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

info "fleetorch ${VERSION} — downloading for ${OS}/${ARCH}"
info "  $URL"

curl -fsSL "$URL" -o "$TMP/$ARCHIVE" || die "download failed"

tar -xzf "$TMP/$ARCHIVE" -C "$TMP" || die "extract failed"

[ -f "$TMP/fleetorch" ] || die "binary missing from archive"

install -m 0755 "$TMP/fleetorch" "$BIN_DIR/fleetorch"

info "installed: $BIN_DIR/fleetorch"

# PATH hint
case ":$PATH:" in
    *":$BIN_DIR:"*) info "ready. Try: fleetorch --help" ;;
    *) info ""; info "NOTE: $BIN_DIR is not on your \$PATH. Add this to your shell rc:"; info "  export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac
