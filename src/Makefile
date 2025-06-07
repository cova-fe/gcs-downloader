# Define the application name
APP_NAME := gcs-downloader

# Go source file
GO_SOURCE := gcs-downloader.go

# Read version from VERSION file
VERSION := $(shell cat VERSION 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date +"%Y-%m-%d %H:%M:%S %Z")

# LDFLAGS for injecting version and build time
LDFLAGS := -X "main.version=$(VERSION)" -X "main.buildTime=$(BUILD_TIME)"

# Default build target
.PHONY: all
all: build

# Build for the current OS and architecture
.PHONY: build
build:
	@echo "Building $(APP_NAME) for current OS/ARCH..."
	go build -o $(APP_NAME) -ldflags '$(LDFLAGS)' $(GO_SOURCE)
	@echo "Build complete: $(APP_NAME)"

# Build for macOS ARM (darwin/arm64)
.PHONY: build-macos-arm
build-macos-arm:
	@echo "Building $(APP_NAME) for macOS ARM (darwin/arm64)..."
	GOOS=darwin GOARCH=arm64 go build -o $(APP_NAME)-macos-arm64 -ldflags '$(LDFLAGS)' $(GO_SOURCE)
	@echo "Build complete: $(APP_NAME)-macos-arm64"

# Build for Linux AMD64 (linux/amd64)
.PHONY: build-linux-amd64
build-linux-amd64:
	@echo "Building $(APP_NAME) for Linux AMD64 (linux/amd64)..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(APP_NAME)-linux-amd64 -ldflags '$(LDFLAGS)' $(GO_SOURCE)
	@echo "Build complete: $(APP_NAME)-linux-amd64"

# Clean up build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -f $(APP_NAME) $(APP_NAME)-macos-arm64 $(APP_NAME)-linux-amd64
	@echo "Clean complete."

# Help target
.PHONY: help
help:
	@echo "Usage:"
	@echo "  make                 - Build for the current OS/ARCH"
	@echo "  make build-macos-arm - Build for macOS ARM (darwin/arm64)"
	@echo "  make build-linux-amd64 - Build for Linux AMD64 (linux/amd64)"
	@echo "  make clean           - Remove build artifacts"
	@echo "  make help            - Display this help message"
