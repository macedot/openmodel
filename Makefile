.PHONY: build test run clean install tag release help

# Variables
BINARY_NAME=openmodel
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DIR=.
CMD_DIR=./cmd
GO=go
GOFLAGS=-v

# Default target
help:
	@echo "openmodel - Ollama-compatible proxy server"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build     Build the binary (default)"
	@echo "  test      Run all tests"
	@echo "  run       Build and run the server"
	@echo "  clean     Remove built binaries"
	@echo "  install   Install to /usr/local/bin"
	@echo "  tag       Create a git tag (usage: make tag VERSION=v0.1.0)"
	@echo "  release   Create a release (tag + push tag)"
	@echo ""

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(GOFLAGS) -ldflags="-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)

# Run all tests
test:
	@echo "Running tests..."
	$(GO) test -v ./...

# Build and run the server
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_NAME)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	$(GO) clean

# Install to system
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	install -m 755 $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

# Create a git tag
tag:
ifndef VERSION
	@echo "Error: VERSION is required. Usage: make tag VERSION=v0.1.0"
	@exit 1
endif
	@echo "Creating tag $(VERSION)..."
	git tag -a $(VERSION) -m "Release $(VERSION)"
	@echo "Tag $(VERSION) created. Run 'make release' to push."

# Create a release (tag + push)
release:
ifndef VERSION
	@echo "Error: VERSION is required. Usage: make release VERSION=v0.1.0"
	@exit 1
endif
	@echo "Creating and pushing release $(VERSION)..."
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push origin $(VERSION)
	@echo "Release $(VERSION) pushed. Check GitHub Actions for build status."

# Development targets
dev: build
	@echo "Running in development mode..."
	./$(BINARY_NAME)

fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

lint:
	@echo "Linting..."
	@which golangci-lint > /dev/null || (echo "Install golangci-lint first" && exit 1)
	golangci-lint run ./...

# All-in-one check
check: fmt test build
	@echo "All checks passed!"
