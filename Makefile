.PHONY: all build build-macos-arm build-linux-amd64 test check fmt fmt-check clean install-hooks help

all: build

build:
	$(MAKE) -C src build

build-macos-arm:
	$(MAKE) -C src build-macos-arm

build-linux-amd64:
	$(MAKE) -C src build-linux-amd64

test:
	$(MAKE) -C src test

check:
	$(MAKE) -C src check

fmt:
	$(MAKE) -C src fmt

fmt-check:
	$(MAKE) -C src fmt-check

clean:
	$(MAKE) -C src clean

install-hooks:
	@mkdir -p .git/hooks
	@cp .githooks/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed into .git/hooks/pre-commit"

help:
	@echo "Usage:"
	@echo "  make                 - Build for current OS/ARCH"
	@echo "  make test            - Run unit tests"
	@echo "  make check           - Run all checks (format, vet, test)"
	@echo "  make fmt             - Format Go code"
	@echo "  make install-hooks   - Install git pre-commit hook"
	@echo "  make build-macos-arm - Build for macOS ARM"
	@echo "  make build-linux-amd64 - Build for Linux AMD64"
	@echo "  make clean           - Clean build artifacts"
