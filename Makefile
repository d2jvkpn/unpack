BINARY := unpack
TARGET_DIR := target
HOST_GOOS := $(shell go env GOOS)
HOST_GOARCH := $(shell go env GOARCH)

.PHONY: build build-all \
	build-linux build-darwin build-windows \
	build-linux-amd64 build-linux-arm64 \
	build-darwin-arm64 \
	build-windows-amd64 build-windows-arm64

build:
	mkdir -p $(TARGET_DIR)
	GOOS=$(HOST_GOOS) GOARCH=$(HOST_GOARCH) go build -o $(TARGET_DIR)/$(BINARY) .

build-all: build-linux build-darwin build-windows

build-linux: build-linux-amd64 build-linux-arm64

build-darwin: build-darwin-arm64

build-windows: build-windows-amd64 build-windows-arm64

build-linux-amd64:
	mkdir -p $(TARGET_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(TARGET_DIR)/$(BINARY)-linux-amd64 .

build-linux-arm64:
	mkdir -p $(TARGET_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(TARGET_DIR)/$(BINARY)-linux-arm64 .

build-darwin-arm64:
	mkdir -p $(TARGET_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $(TARGET_DIR)/$(BINARY)-darwin-arm64 .

build-windows-amd64:
	mkdir -p $(TARGET_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(TARGET_DIR)/$(BINARY)-windows-amd64.exe .

build-windows-arm64:
	mkdir -p $(TARGET_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -o $(TARGET_DIR)/$(BINARY)-windows-arm64.exe .
