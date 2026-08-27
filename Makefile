.PHONY: all build build-macos-arm build-linux-amd64 test check fmt fmt-check secret-scan clean install-hooks help

all: build

build:
	$(MAKE) -C src build

build-macos-arm:
	$(MAKE) -C src build-macos-arm

build-linux-amd64:
	$(MAKE) -C src build-linux-amd64

test:
	$(MAKE) -C src test

secret-scan:
	@if command -v trivy >/dev/null 2>&1; then \
		echo "Scanning repository for secrets with Trivy..."; \
		trivy fs --scanners secret --exit-code 1 .; \
	else \
		echo "Warning: Trivy not installed. Install it via 'brew install trivy' for secret scanning."; \
	fi

check: fmt-check secret-scan test
	$(MAKE) -C src check

fmt:
	$(MAKE) -C src fmt

fmt-check:
	$(MAKE) -C src fmt-check

clean:
	$(MAKE) -C src clean

install-hooks:
	@if command -v pre-commit >/dev/null 2>&1; then \
		pre-commit install; \
		echo "Pre-commit hook installed via pre-commit framework"; \
	else \
		mkdir -p .git/hooks; \
		cp .githooks/pre-commit .git/hooks/pre-commit; \
		chmod +x .git/hooks/pre-commit; \
		echo "Pre-commit hook installed into .git/hooks/pre-commit"; \
	fi

help:
	@echo "Usage:"
	@echo "  make                 - Build for current OS/ARCH"
	@echo "  make test            - Run unit tests"
	@echo "  make secret-scan     - Scan repository for leaked secrets using Trivy"
	@echo "  make check           - Run all checks (format, secret scan, vet, test)"
	@echo "  make fmt             - Format Go code"
	@echo "  make install-hooks   - Install git pre-commit hook"
	@echo "  make build-macos-arm - Build for macOS ARM"
	@echo "  make build-linux-amd64 - Build for Linux AMD64"
	@echo "  make clean           - Clean build artifacts"
