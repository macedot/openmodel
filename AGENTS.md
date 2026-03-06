# AGENTS.md

## Build/Lint/Test Commands

```bash
# Build
make build              # Build binary to ./openmodel
make build VERSION=v1.0.0  # Build with custom version

# Test
make test               # Run all tests with race detection
go test -race -v ./...  # Run all tests with verbose output

# Run single test
go test -race -v -run TestName ./path/to/package
go test -race -v -run TestHandleRoot ./internal/server
go test -race -v -run "TestChat" ./internal/provider  # Pattern matching

# Coverage
make cover              # Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out  # View HTML report

# Lint and format
gofmt -w .              # Format all Go files
go vet ./...            # Run go vet
make lint               # Run golangci-lint (requires installation)
make check              # Run fmt, vet, and test

# Generate code
make generate           # Regenerate OpenAPI types from spec

# Docker
make docker-build       # Build Docker image
docker build --build-arg VERSION=v1.0.0 -t ghcr.io/macedot/openmodel:v1.0.0 .

# Release
make tag VERSION=v1.0.0     # Create git tag
make release VERSION=v1.0.0 # Create and push tag (triggers CI)
```

## Code Style Guidelines

### Imports
Group imports in order: standard library, external packages, internal packages.

```go
import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
)
```

### Formatting
- Use `gofmt` for all formatting - no custom style arguments
- Run `gofmt -w .` before committing

### Types
**Generated Types (DO NOT EDIT):**
- `internal/api/openai/generated/types.gen.go` - auto-generated from OpenAPI spec
- Regenerate with `make generate` when spec changes

**Hand-written Types:**
- `internal/api/openai/types.go` - hand-written types for streaming, helpers
- Use pointers for optional fields (`*float64`, `*int`)
- Use `any` for flexible fields (`any` not `interface{}`)

### Naming Conventions
**Packages:** Short, lowercase, single word: `config`, `logger`, `provider`, `server`, `state`.

**Types:** PascalCase for exported (`ChatCompletionRequest`), camelCase for unexported. Interfaces: `Provider` (noun, not `ProviderInterface`). Implementations: `OpenAIProvider`.

**Functions:** PascalCase for exported (`NewOpenAIProvider()`), camelCase for unexported. Constructors: `New<Type>()` pattern. Getters: no `Get` prefix - use `Name()` not `GetName()`.

### Error Handling
Always wrap errors with context, never ignore:

```go
// Good - wrap with context
if err := json.Unmarshal(data, &req); err != nil {
	return fmt.Errorf("failed to parse request: %w", err)
}

// Good - custom error types for validation
return ValidationError{Field: "model", Message: "is required"}

// Bad - ignore error or return without context
json.Marshal(req)  // NO!
return err         // NO! Add context
```

Never ignore JSON encoding errors. Check the error from `json.NewEncoder(w).Encode()`.

### Testing Patterns
Use `stretchr/testify` for assertions. Table-driven tests preferred:

```go
func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *Request
		wantErr bool
	}{
		{"valid request", &Request{Model: "test"}, false},
		{"empty model", &Request{Model: ""}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequest(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
```

Run tests with race detection: `go test -race ./...`

### Package Organization
```
cmd/                    # Entry point
  main.go               # CLI with serve/test/models commands
internal/
  api/openai/           # OpenAI types + validation
    generated/          # Auto-generated types (DO NOT EDIT)
    types.go            # Hand-written types
    validation.go       # Request validation
  config/               # JSON config with schema validation
  logger/               # Structured logging (slog wrapper)
  provider/             # Provider interface + OpenAI impl
  server/               # HTTP handlers + routing
    handlers.go         # HTTP handlers
    handlers_helpers.go # Extracted helper functions
    routes.go           # Route registration
  state/                # Failure tracking for fallback
```

### Validation
- Validation functions in `internal/api/openai/validation.go`
- Return `ValidationError` with field and message
- Use raw bytes validation: `ValidateChatCompletionRequest(body []byte)` not struct validation

### Configuration
- Config file: `openmodel.json` (searches current dir first, then `~/.config/openmodel/openmodel.json`)
- Current directory config has higher priority when merging with user config
- Default port: **12345**
- Supports `${VAR}` env var expansion in provider configs
- Schema validation required (`$schema` field must be present)
- Env vars override config: `OPENMODEL_LOG_LEVEL`, `OPENMODEL_LOG_FORMAT`

### Config File Format

```json
{
  "$schema": "https://raw.githubusercontent.com/macedot/openmodel/master/openmodel.schema.json",
  "server": { "port": 12345, "host": "localhost" },
  "providers": {
    "ollama": {
      "url": "http://localhost:11434/v1",
      "models": ["glm-4", "llama2"],
      "thresholds": { "failures_before_switch": 3 }
    }
  },
  "models": {
    "any": {
      "strategy": "round-robin",
      "providers": ["ollama/glm-4", "openai/gpt-4"]
    },
    "glm-4": {
      "providers": ["glm-4"]
    }
  },
  "thresholds": { "failures_before_switch": 3 }
}
```

**Key features:**
- **Own models**: Use just model name (e.g., `"glm-4"`) - auto-resolves to first provider with that model
- **Per-model strategy**: `"fallback"` (default), `"round-robin"`, `"random"`
- **Provider-specific thresholds**: Override global thresholds per provider
- **Circular reference detection**: Prevents infinite loops in model resolution

### HTTP Handlers
- Register routes in `routes.go`, implement in `handlers.go`
- Use helper functions from `handlers_helpers.go`:
  - `readAndValidateRequest()`, `findProviderWithFailover()`, `executeWithFailover()`
  - `handleProviderError()` / `handleProviderSuccess()` - manages state
  - `drainStream[T]()` - prevents goroutine leaks in streaming
- Request ID: X-Request-ID header added for tracing (reused or generated)

### Architecture Patterns
- **DRY:** Extract common logic to helper functions. Use generic functions.
- **KISS:** Prefer simple, readable code. Avoid premature optimization.
- **TDD:** Write tests before or alongside implementation. Aim for 80%+ coverage.

### Security
- HTTP server hardening: MaxHeaderBytes 1MB, ReadTimeout 30s, WriteTimeout 120s, IdleTimeout 120s
- Docker runs as non-root user (`nonroot:nonroot`)
