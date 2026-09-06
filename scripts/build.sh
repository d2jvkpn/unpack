#!/bin/sh
# Build the unpack binary into target/releases/.
#
# Usage:
#   scripts/build.sh              # host GOOS/GOARCH
#   scripts/build.sh linux-amd64  # cross-compile for <os>-<arch>, CGO_ENABLED=0

set -eu

BINARY="unpack"
RELEASES_DIR="target/releases"

BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-X main.buildTime=$BUILD_TIME"

mkdir -p "$RELEASES_DIR"

plat="${1:-}"

if [ -z "$plat" ]; then
    #os="$(go env GOOS)"
    #arch="$(go env GOARCH)"
    os=$(uname -s | tr A-Z a-z)
    arch=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

    ext=""
    [ "$os" = "windows" ] && ext=".exe"

    GOOS="$os" GOARCH="$arch" go build \
        -trimpath \
        -ldflags "$LDFLAGS" \
        -o "target/$BINARY$ext" .
    exit 0
fi

os="${plat%-*}"
arch="${plat#*-}"
ext=""
[ "$os" = "windows" ] && ext=".exe"

CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build \
  -trimpath \
  -ldflags "$LDFLAGS" \
  -o "$RELEASES_DIR/$BINARY-$plat$ext" .
