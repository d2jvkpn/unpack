.PHONY: build build-all \
	build-linux build-darwin build-windows \
	build-linux-amd64 build-linux-arm64 \
	build-darwin-arm64 \
	build-windows-amd64 build-windows-arm64 \
	package

check:
	go fmt ./...
	go vet ./...

build:
	./scripts/build.sh

build-linux-amd64:
	./scripts/build.sh linux-amd64

build-linux-arm64:
	./scripts/build.sh linux-arm64

build-darwin-arm64:
	./scripts/build.sh darwin-arm64

build-windows-amd64:
	./scripts/build.sh windows-amd64

build-windows-arm64:
	./scripts/build.sh windows-arm64

build-linux: build-linux-amd64 build-linux-arm64

build-darwin: build-darwin-arm64

build-windows: build-windows-amd64 build-windows-arm64

build-all: build-linux build-darwin build-windows

package: build-all
	./scripts/package.sh
