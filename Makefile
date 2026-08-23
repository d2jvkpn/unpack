# HOST_OS := $(shell go env GOOS)
# HOST_ARCH := $(shell go env GOARCH)
HOST_OS := $(shell uname -s | tr A-Z a-z)
HOST_ARCH := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

BINARY := unpack

COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)

.PHONY: build build-all \
	build-linux build-darwin build-windows \
	build-linux-amd64 build-linux-arm64 \
	build-darwin-arm64 \
	build-windows-amd64 build-windows-arm64

BINARY_EXT :=
ifeq ($(HOST_OS),windows)
BINARY_EXT := .exe
endif

build:
	mkdir -p target
	GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build -ldflags "$(LDFLAGS)" -o target/$(BINARY)$(BINARY_EXT) .

build-linux-amd64:
	mkdir -p target
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o target/$(BINARY)-linux-amd64 .

build-linux-arm64:
	mkdir -p target
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o target/$(BINARY)-linux-arm64 .

build-darwin-arm64:
	mkdir -p target
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o target/$(BINARY)-darwin-arm64 .

build-windows-amd64:
	mkdir -p target
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o target/$(BINARY)-windows-amd64.exe .

build-windows-arm64:
	mkdir -p target
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o target/$(BINARY)-windows-arm64.exe .

build-linux: build-linux-amd64 build-linux-arm64

build-darwin: build-darwin-arm64

build-windows: build-windows-amd64 build-windows-arm64

build-all: build-linux build-darwin build-windows
