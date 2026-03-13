# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

OpenModel is a Go-based HTTP proxy server providing OpenAI-compatible and Anthropic-compatible API endpoints with multi-provider fallback, format conversion, and resilience features.

## Build & Test Commands

```bash
make build          # Build the binary
make test           # Run all tests with race detection
make test ARGS="-run TestName"  # Run single test
make check          # Run fmt, vet, and test
make cover          # Generate coverage report
make docker-build   # Build Docker image
make clean          # Remove built binaries
make lint           # Run golangci-lint (requires installation)
make generate       # Generate OpenAPI types from spec
```

## Architecture

### Core Components

- **cmd/** - CLI entry point, commands (serve, models, config, bench)
- **internal/server/** - HTTP server using Fiber, routing, handlers
- **internal/provider/** - Provider interface and implementations (OpenAI, Anthropic, etc.)
- **internal/config/** - Configuration loading and JSON schema validation
- **internal/api/openai/** - OpenAI API types and validation
- **internal/api/anthropic/** - Anthropic API types and conversion
- **internal/state/** - Runtime state management (models, provider failures)
- **internal/logger/** - Structured JSON logging

### Request Flow

1. Fiber HTTP server receives requests at `/v1/chat/completions`, `/v1/messages`, etc.
2. Handlers (`handlers_openai.go`, `handlers_claude.go`) parse requests
3. Converter converts between OpenAI/Anthropic formats based on provider's `api_mode`
4. Provider layer executes requests with failover logic (`failover.go`)
5. Streaming handled via `streaming.go`

### Key Interfaces

- `provider.Provider` - Full provider interface (Name, BaseURL, APIMode, ListModels, Chat, StreamChat, etc.)
- `provider.ChatProvider`, `provider.CompletionProvider`, `provider.EmbeddingProvider` - Feature-specific interfaces
- `server.ProviderMap` - Runtime provider registry with hot-reload support

### Configuration

Config is validated against JSON schema. Key structure:
- `server` - port, host
- `providers` - map of provider name to config (url, api_mode, api_key, models)
- `models` - map of model alias to config (strategy: fallback/round-robin/random, providers list)
- `thresholds` - failure tracking configuration
- `rate_limit` - token bucket rate limiting

### Testing Patterns

- Tests use mock HTTP servers with `testTransport` in `provider_test.go`
- Server tests use Fiber's test client
- Provider tests must set headers BEFORE calling WriteHeader to avoid data races

## Important Notes

- Go 1.26+ required (check go.mod)
- Tests run with `-race` flag in CI
- OpenAPI types generated via `oapi-codegen` - edit `api/openai/openapi.yaml` then run `make generate`
