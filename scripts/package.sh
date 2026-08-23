#!/bin/sh
# Package binaries built by `make build-all` into per-platform zip archives
# under target/releases/. Each zip contains a single entry named after the
# binary (unpack or unpack.exe), not the platform-suffixed build filename.
#
# Usage: scripts/package.sh (invoked automatically by `make package`)

set -eu

BINARY="unpack"
ROOT_DIR="$(pwd)"

mkdir -p "target/releases"

package_one() {
    plat="$1"
    src="$2"
    entry="$3"
    stage="target/releases/$plat"

    mkdir -p "$stage"
    mv "$src" "$stage/$entry"
    (cd "$stage" && zip -q "$ROOT_DIR/target/releases/$BINARY-$plat.zip" "$entry")
    rm -rf "$stage"
}

for plat in linux-amd64 linux-arm64 darwin-arm64; do
    package_one "$plat" "target/releases/$BINARY-$plat" "$BINARY"
done

for plat in windows-amd64 windows-arm64; do
    package_one "$plat" "target/releases/$BINARY-$plat.exe" "$BINARY.exe"
done
