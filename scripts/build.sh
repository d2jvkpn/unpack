#!/bin/sh
# Build the unpack binary into target/releases/.
#
# Usage:
#   scripts/build.sh              # host GOOS/GOARCH
#   scripts/build.sh linux-amd64  # cross-compile for <os>-<arch>, CGO_ENABLED=0

set -eu

BINARY="unpack"
TARGET_DIR="target/releases"

COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-X unpack/internal/unpack.commit=$COMMIT -X unpack/internal/unpack.buildTime=$BUILD_TIME"

mkdir -p "$TARGET_DIR"

plat="${1:-}"

if [ -z "$plat" ]; then
    #os="$(go env GOOS)"
    #arch="$(go env GOARCH)"
    os=$(uname -s | tr A-Z a-z)
    arch=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

    ext=""
    [ "$os" = "windows" ] && ext=".exe"

    GOOS="$os" GOARCH="$arch" go build -ldflags "$LDFLAGS" -o "$TARGET_DIR/$BINARY$ext" .
    exit 0
fi

os="${plat%-*}"
arch="${plat#*-}"
ext=""
[ "$os" = "windows" ] && ext=".exe"

CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -ldflags "$LDFLAGS" \
    -o "$TARGET_DIR/$BINARY-$plat$ext" .
