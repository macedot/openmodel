# AGENTS.md - Agentic Coding Guidelines

Guidelines for AI agents operating in this repository.

## Project Overview

OpenModel is an OpenAI-compatible proxy server with multi-provider fallback, written in Go 1.26+. Uses Fiber web framework with structured slog logging.

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
# Run single test function
go test -v -run TestFunctionName ./path/to/package

# Run single test with exact match
go test -v -run "^TestFunctionName$" ./internal/server

# Run all tests in a package
go test -v -race ./internal/server/...
```

## Code Style

### Naming Conventions

- **Files**: `snake_case.go`, `*_test.go` for tests
- **Types**: `PascalCase` (e.g., `Server`, `ChatCompletionRequest`)
- **Functions/Variables**: `camelCase` (e.g., `handleRoot`, `formatProviderKey`)
- **Constants**: `PascalCase` exported, `camelCase` unexported
- **Interfaces**: Action + `er` suffix (e.g., `Provider`, `Handler`)

### Imports

Organize imports in three groups with blank lines between:

```go
import (
	// Standard library
	"context"
	"encoding/json"
	"fmt"

	// External packages
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"

	// Internal packages
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

Error responses: use helper functions returning structured JSON:

```go
func handleError(c *fiber.Ctx, message string, status int) error {
	return c.Status(status).JSON(fiber.Map{"error": message})
}
```

### Error Handling

```go
if err != nil {
	return fmt.Errorf("failed to encode response: %w", err)
}
```

Key patterns:
- Return errors explicitly with context using `fmt.Errorf`
- Use `%w` for wrapped errors (enables error chain)
- Log errors at the appropriate level with structured context
- Don't suppress errors with empty catch blocks

### Testing

Use table-driven tests with testify. Pattern: `Test<FunctionName>_<Scenario>`:

```go
func TestExtractModelFromRequestBody(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected string
		wantErr  bool
	}{
		{
			name:     "valid model",
			body:     []byte(`{"model": "gpt-4", "messages": []}`),
			expected: "gpt-4",
			wantErr:  false,
		},
		{
			name:     "empty body",
			body:     []byte(``),
			expected: "",
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractModelFromRequest(tt.body)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

- `testify/assert` for assertions in tests
- `testify/require` for assertions that should stop test on failure
- Table-driven tests with named subtests (t.Run)
- Mock implementations as separate structs in test files

### Logging

Use structured logging with slog key-value pairs:

```go
// Info level
logger.Info("request",
	"request_id", id,
	"method", c.Method(),
	"path", c.Path())

// Error level
logger.Error("failed",
	"error", err,
	"context", additionalInfo)

// Debug level (only shown when configured)
logger.Debug("provider_call",
	"provider", name,
	"model", model)
```

Levels: `trace`, `debug`, `info`, `warn`, `error`

### Types and Structs

```go
// ChatCompletionRequest is sent to /v1/chat/completions
type ChatCompletionRequest struct {
	Model       string                  `json:"model"`
	Messages    []ChatCompletionMessage `json:"messages"`
	Temperature *float64                `json:"temperature,omitempty"` // Pointer for optional
	Stream      bool                    `json:"stream,omitempty"`
	Extra       map[string]any          `json:"-"` // Provider-specific fields, excluded from JSON
}
```

Key patterns:
- Use pointers (`*Type`) for optional JSON fields
- Use `json:"-"` for internal/excluded fields
- Use `json:",omitempty"` to omit empty optional fields
- Add documentation comments for exported types
- Use `json:"field,omitempty"` for optional fields that should be omitted when empty
- Use `Extra map[string]any` pattern for provider-specific extension fields

### Configuration

Environment variables: `${VAR}` syntax expands from env vars in config:

```go
// Environment variable expansion
config := struct {
	URL string `json:"url"`
}{
	URL: "${OPENAI_API_KEY}", // Expanded from env var
}
```

Schema validation: `$schema` field triggers remote validation:

```json
{
  "$schema": "https://raw.githubusercontent.com/macedot/openmodel/master/openmodel.schema.json",
  "server": { "port": 12345 }
}
```

Config loading with validation:

```go
cfg, err := config.Load(configPath)
if err != nil {
	log.Fatalf("Failed to load config: %v", err)
}

// Validate provider references
if err := cfg.ValidateProviderReferences(); err != nil {
	log.Fatalf("Configuration error: %v", err)
}
```

## File Organization

```
cmd/main.go                 # Entry point
internal/
  api/anthropic/            # Anthropic API types and validation
  api/openai/               # OpenAI API types and validation
    generated/              # OpenAPI-generated types (do not edit)
  config/                   # Configuration loading and validation
  logger/                   # Structured logging utilities
  provider/                 # Provider abstraction interface
  server/                   # HTTP server and handlers
    constants.go            # Server constants
    handlers.go             # Handler stubs/documentation
    handlers_openai.go      # OpenAI endpoint handlers
    handlers_claude.go      # Anthropic endpoint handlers
    handlers_utils.go       # Handler utilities
    failover.go             # Provider failover logic
    streaming.go            # SSE streaming handling
    server.go               # Server setup
    ratelimit.go            # Rate limiting middleware
  state/                    # State management
```

## Common Tasks

### Adding an API Endpoint

1. Define types in `internal/api/<format>/types.go`
2. Add validation in `internal/api/<format>/validation.go`
3. Add handler in `internal/server/handlers_<format>.go`
4. Register route in `internal/server/server.go`
5. Add tests in `internal/server/handlers_test.go`

### Adding a Provider

1. Implement `provider.Provider` interface in `internal/provider/`
2. Add configuration struct to `internal/config/config.go`
3. Add tests with mock implementations

## Key Patterns

- **Failover**: Track failures per provider with progressive backoff
- **Streaming**: SSE with `text/event-stream`, flush after each chunk
- **Context cancellation**: Pass context through request chain
- **Error wrapping**: Use `fmt.Errorf("context: %w", err)` for chains
- **Request IDs**: Generate unique IDs for request tracing
- **Rate limiting**: Token bucket per-IP with trusted proxy support

## Hot Reload

OpenModel supports hot reload of configuration. When the config file changes:

1. **File Watcher**: Uses fsnotify to watch for config file changes
2. **SIGHUP Signal**: Send `SIGHUP` to reload config manually
3. **Validation**: Only valid configurations are applied
4. **Zero Downtime**: Providers are atomically swapped without dropping requests

### Example

```bash
# Edit the config
vim ~/.config/openmodel/config.json

# The server automatically reloads if the config is valid
# Or manually reload:
pkill -SIGHUP openmodel
```

### Trace Files

When trace level logging is enabled (`log_level: "trace"`), the server writes trace files for debugging:

```
trace-config-reload-20060102-150405.json
trace-*.json
```

These files are prefixed with `trace-` and contain detailed information about operations.


## Testing Patterns

### Mock Provider for Testing

```go
type mockProvider struct {
	name string
	doRequest func(ctx interface{}, endpoint string, body []byte, headers map[string]string) ([]byte, error)
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) DoRequest(ctx interface{}, endpoint string, body []byte, headers map[string]string) ([]byte, error) {
	if m.doRequest != nil {
		return m.doRequest(ctx, endpoint, body, headers)
	}
	return nil, nil
}
```

### Fiber Handler Testing

```go
func TestHandleHealth(t *testing.T) {
	app := fiber.New()
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}
```