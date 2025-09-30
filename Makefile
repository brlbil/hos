.PHONY: build test lint clean check help all build-hos build-hosd build-all cli-test

# Default target
all: build

# Build commands
build: build-hos build-hosd

build-all:
	@echo "Building all packages..."
	go build ./...

build-hos:
	@echo "Building hos client..."
	go build -o bin/hos ./cmd/hos

build-hosd:
	@echo "Building hosd server..."
	go build -o bin/hosd ./cmd/hosd

# Test commands
test:
	@echo "Running tests..."
	go test ./...

# Code quality commands
lint:
	@echo "Running golangci-lint..."
	golangci-lint run --verbose --modules-download-mode=vendor

# Clean up
clean:
	@echo "Cleaning up..."
	rm -rf bin/

# Test commands
cli-test: build
	@echo "Running CLI integration tests..."
	./scripts/cli-test.sh

# Complete check (all quality checks)
check: test cli-test lint
	@echo "All checks passed!"

# Help
help:
	@echo "Available commands:"
	@echo "  build        - Build both hos and hosd binaries"
	@echo "  build-all    - Build all packages"
	@echo "  build-hos    - Build only hos client"
	@echo "  build-hosd   - Build only hosd server"
	@echo "  test         - Run unit tests"
	@echo "  cli-test     - Run CLI integration tests"
	@echo "  lint         - Run golangci-lint"
	@echo "  clean        - Remove built binaries and reports"
	@echo "  check        - Run all quality checks"
	@echo "  help         - Show this help message"
