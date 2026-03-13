.PHONY: build test run clean install uninstall tag release help cover check generate docker-build docker-run

# Variables
BINARY_NAME=openmodel
BUILD_DIR=.
CMD_DIR=./cmd
GO:=$(shell which go)
GOFLAGS=-v
DOCKER_IMAGE=ghcr.io/macedot/openmodel

# Get git version for builds
GIT_VERSION:=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Default target
help:
	@echo "openmodel - OpenAI-compatible proxy server with multi-provider fallback"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build          Build the binary (default)"
	@echo "  test           Run all tests with race detection"
	@echo "  cover          Generate coverage report"
	@echo "  check          Run fmt, vet, and test"
	@echo "  run            Build and run the server"
	@echo "  clean          Remove built binaries"
	@echo "  install        Install to /usr/local/bin"
	@echo "  uninstall      Remove from /usr/local/bin"
	@echo "  tag            Create a git tag (usage: make tag VERSION=v0.1.0)"
	@echo "  release        Create a release (tag + push tag)"
	@echo ""
	@echo "Docker targets:"
	@echo "  docker-build   Build Docker image"
	@echo "  docker-run     Run Docker image (pass parameters with DOCKER_ARGS=...)"
	@echo ""

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(GOFLAGS) -ldflags="-s -w -X main.Version=$(or $(VERSION),$(GIT_VERSION)) -X main.BuildDate=$(shell date -u +%Y-%m-%d)" -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)

# Run all tests
test:
	@echo "Running tests..."
	$(GO) test -race -v -cover ./...

# Generate coverage report
cover:
	@echo "Generating coverage report..."
	$(GO) test -coverprofile=coverage.out ./...
	@echo "Coverage: $$(go tool cover -func=coverage.out | tail -1 | awk '{print $$3}')"
	@echo "View HTML report: go tool cover -html=coverage.out"

# All-in-one check
check: fmt vet test
	@echo "All checks passed!"

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
install:
	@echo "Building $(BINARY_NAME)..."
	$(GO) build -ldflags="-s -w -X main.Version=$(or $(VERSION),$(GIT_VERSION)) -X main.BuildDate=$(shell date -u +%Y-%m-%d)" -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	@if [ -w /usr/local/bin ]; then \
		install -m 755 $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/; \
	else \
		echo "Using sudo for installation..."; \
		sudo install -m 755 $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/; \
	fi

# Uninstall from system
uninstall:
	@echo "Uninstalling $(BINARY_NAME) from /usr/local/bin..."
	@if [ -w /usr/local/bin ]; then \
		rm -f /usr/local/bin/$(BINARY_NAME); \
		echo "Uninstalled successfully"; \
	else \
		echo "Using sudo for uninstallation..."; \
		sudo rm -f /usr/local/bin/$(BINARY_NAME); \
		echo "Uninstalled successfully"; \
	fi

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

vet:
	@echo "Running vet..."
	$(GO) vet ./...

lint:
	@echo "Linting..."
	@which golangci-lint > /dev/null || (echo "Install golangci-lint first" && exit 1)
	golangci-lint run ./...

# Generate OpenAPI types from core spec
generate: api/openai/openapi.yaml
	@echo "Generating OpenAPI types..."
	@which oapi-codegen > /dev/null || (echo "Install oapi-codegen first: go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest" && exit 1)
	oapi-codegen -config api/openai/config.yaml api/openai/openapi.yaml > internal/api/openai/generated/types.gen.go

# Download full OpenAPI spec and extract core types
download-spec:
	@echo "Downloading OpenAI OpenAPI spec..."
	@which curl > /dev/null || (echo "Install curl first" && exit 1)
	curl -sL "https://app.stainless.com/api/spec/documented/openai/openapi.documented.yml" -o api/openai/openapi-full.yaml
	@echo "Extracting core types..."
	python3 api/openai/extract-core.py api/openai/openapi-full.yaml api/openai/openapi.yaml
	@echo "Running make generate..."
	$(MAKE) generate

# Run spec compliance tests
test-spec:
	@echo "Running spec compliance tests..."
	$(GO) test -v -run "SpecCompliance|BackwardCompatibility" ./internal/api/openai/...

# Docker targets
docker-build:
	@echo "Building Docker image..."
	docker build --build-arg VERSION=$(or $(VERSION),$(GIT_VERSION)) -t $(DOCKER_IMAGE):$(or $(VERSION),$(GIT_VERSION)) .
	@echo "Image built: $(DOCKER_IMAGE):$(or $(VERSION),$(GIT_VERSION))"

docker-run:
	@echo "Running Docker image..."
	docker run --rm $(DOCKER_IMAGE):$(or $(VERSION),$(GIT_VERSION)) $(DOCKER_ARGS)
