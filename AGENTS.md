# AGENTS.md - Agentic Coding Guidelines

Guidelines for AI agents operating in this repository.

## Project Overview

OpenModel is an OpenAI-compatible proxy server with multi-provider fallback, written in Go 1.24+. Uses Fiber web framework.

## Build Commands

```bash
make build              # Build binary
make test               # Run all tests with race detection
make cover              # Generate coverage report
make check              # Run fmt, vet, and test
make lint               # Lint (requires golangci-lint)
make run                # Build and run
make test-spec          # Run spec compliance tests
make generate           # Generate OpenAPI types
```

### Single Test Commands

```bash
go test -v -run TestFunctionName ./path/to/package
go test -v -run "^TestFunctionName$" ./internal/server
go test -v -race ./internal/server/...
```

## Code Style

### Naming Conventions

- **Files**: `snake_case.go`, `*_test.go` for tests
- **Types**: `PascalCase` (e.g., `Server`, `ChatCompletionRequest`)
- **Functions/Variables**: `camelCase` (e.g., `handleRoot`, `formatProviderKey`)
- **Constants**: `PascalCase` exported, `camelCase` unexported
- **Interfaces**: Action + `er` suffix (e.g., `Reader`, `Handler`)

### Imports

Organize in three groups with blank lines between:

```go
import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"

	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/logger"
)
```

### HTTP Handlers (Fiber)

Use `*fiber.Ctx` and `fiber.Status*` constants:

```go
func (s *Server) handleHealth(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
}
```

### Error Handling

```go
if err != nil {
	return fmt.Errorf("failed to encode response: %w", err)
}
```

- Return errors explicitly with context
- Use `%w` for wrapped errors
- Log errors at the appropriate level

### Testing

- Table-driven tests with named cases
- Use `testify/assert` and `testify/require`
- Test name pattern: `Test<FunctionName>_<Scenario>`

```go
func TestHandleHealth(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		wantErr bool
	}{
		{"valid model", "gpt-4", false},
		{"empty model", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := srv.validateModel(tt.model)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
```

### Logging

Use the internal logger with structured context:

```go
logger.Info("request", "request_id", id, "method", c.Method(), "path", c.Path())
logger.Error("failed", "error", err)
```

### Types and Structs

```go
// ChatCompletionRequest is sent to /v1/chat/completions
type ChatCompletionRequest struct {
	Model       string  `json:"model"`
	Messages    []ChatCompletionMessage `json:"messages"`
	Temperature *float64 `json:"temperature,omitempty"` // Pointer for optional
	Stream      bool     `json:"stream,omitempty"`
	Extra       map[string]any `json:"-"` // Provider-specific fields
}
```

- Use pointers for optional fields
- Add documentation comments for exported types
- Use `json:"-"` for internal/excluded fields

## File Organization

```
cmd/main.go                 # Entry point
internal/
  api/anthropic/            # Anthropic API types and validation
  api/openai/               # OpenAI API types and validation
    generated/              # OpenAPI-generated types (do not edit)
  config/                   # Configuration loading
  logger/                   # Logging utilities
  provider/                 # Provider abstraction
  server/                   # HTTP server and handlers
    constants.go            # Server constants
    handlers.go              # Handler stubs/documentation
    handlers_openai.go       # OpenAI endpoint handlers
    handlers_claude.go       # Anthropic endpoint handlers
    handlers_utils.go        # Handler utilities
    failover.go              # Provider failover logic
    streaming.go             # SSE streaming handling
    server.go                # Server setup
    ratelimit.go            # Rate limiting middleware
  state/                    # State management
```

## Configuration

### HTTP Client Settings

```json
{
  "http": {
    "timeout_seconds": 120,
    "max_idle_conns": 100,
    "max_idle_conns_per_host": 100
  }
}
```

### Rate Limiting

```json
{
  "rate_limit": {
    "enabled": true,
    "requests_per_second": 10,
    "burst": 20
  }
}
```

## Common Tasks

### Adding an API Endpoint

1. Define types in `internal/api/<format>/types.go`
2. Add validation in `internal/api/<format>/validation.go`
3. Add handler in `internal/server/handlers_<format>.go`
4. Register route in `internal/server/server.go`
5. Add tests

### Adding a Provider

1. Implement `provider.Provider` interface in `internal/provider/`
2. Add configuration in `internal/config/config.go`
3. Add tests with mock implementations

## Key Patterns

- Environment variables: `${VAR}` syntax expands from env vars
- Schema validation: `$schema` field triggers remote validation
- Failover: Track failures per provider with progressive backoff
- Streaming: SSE with `text/event-stream`, flush after each chunk