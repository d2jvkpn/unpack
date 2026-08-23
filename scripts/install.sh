#!/bin/sh
# Install the unpack CLI by downloading a prebuilt binary from GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/d2jvkpn/unpack/main/install.sh | sh
#
# Environment variables:
#   VERSION     Release tag to install, e.g. "v0.1.0" (default: latest release)
#   INSTALL_DIR Directory to install the binary into (default: /usr/local/bin,
#               falling back to $HOME/.local/bin if that is not writable)

set -eu

REPO="d2jvkpn/unpack"
BINARY="unpack"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
    x86_64) arch="amd64" ;;
    aarch64) arch="arm64" ;;
esac

case "${os}-${arch}" in
    linux-amd64|linux-arm64|darwin-arm64) ;;
    *)
        echo "error: unsupported platform ${os}/${arch}" >&2
        echo "unpack currently publishes binaries for: linux/amd64, linux/arm64, darwin/arm64" >&2
        exit 1
        ;;
esac

asset="${BINARY}-${os}-${arch}.zip"

if [ "${VERSION:-}" ]; then
    url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
else
    url="https://github.com/${REPO}/releases/latest/download/${asset}"
fi

if ! command -v unzip >/dev/null 2>&1; then
    echo "error: unzip is required to install unpack but was not found" >&2
    exit 1
fi

install_dir="${INSTALL_DIR:-/usr/local/bin}"
if [ -z "${INSTALL_DIR:-}" ] && [ ! -w "$install_dir" ]; then
    install_dir="$HOME/.local/bin"
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

echo "Downloading ${url}"
curl -fsSL -o "$tmp_dir/$asset" "$url"
unzip -q -o "$tmp_dir/$asset" -d "$tmp_dir"
chmod +x "$tmp_dir/$BINARY"

mkdir -p "$install_dir"
mv "$tmp_dir/$BINARY" "$install_dir/$BINARY"

echo "Installed to $install_dir/$BINARY"
"$install_dir/$BINARY" --version

case ":$PATH:" in
    *":$install_dir:"*) ;;
    *) echo "note: $install_dir is not on your PATH; add it to use '$BINARY' directly" ;;
esac
