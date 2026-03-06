# openmodel

A Go-based HTTP proxy server providing OpenAI-compatible API endpoints with multi-provider fallback support.

## Features

- **OpenAI-Compatible API**: Works with any OpenAI-compatible client
- **Multi-Provider Support**: Configure multiple providers (OpenAI, Ollama, OpenCode, etc.)
- **Automatic Fallback**: Tries providers in sequence on failure
- **Progressive Timeout**: Backs off when all providers are exhausted

## Installation

### Quick Install (Linux)

Install openmodel with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/macedot/openmodel/main/install.sh | sh
```

This will:
- Detect your system architecture
- Download the latest binary from GitHub Releases
- Install to `/usr/local/bin/openmodel`
- Create a systemd service (optional)

### Docker

Pull the latest image:

```bash
docker pull ghcr.io/macedot/openmodel:latest
```

Run with mounted config:

```bash
docker run -d \
  -p 12345:12345 \
  -v ~/.config/openmodel/config.json:/root/.config/openmodel/config.json:ro \
  ghcr.io/macedot/openmodel:latest
```

Or use docker-compose:

```bash
# Create config file first
mkdir -p ~/.config/openmodel
cp openmodel.json.example ~/.config/openmodel/openmodel.json
# Edit config with your API keys

# Start with docker-compose
docker-compose up -d
```

### Manual Install

Build from source:

```bash
git clone https://github.com/macedot/openmodel.git
cd openmodel
make build
sudo make install
```


## Configuration

Create `~/.config/openmodel/config.json`:

```json
{
  "$schema": "https://raw.githubusercontent.com/macedot/openmodel/master/config.schema.json",
  "server": {
    "port": 12345,
    "host": "localhost"
  },
  "providers": {
    "local": {
      "url": "http://localhost:11434/v1",
      "apiKey": ""
    },
    "openai": {
      "url": "https://api.openai.com/v1",
      "apiKey": "${OPENAI_API_KEY}"
    }
  },
  "models": {
    "gpt-4": [
      { "provider": "openai", "model": "gpt-4" }
    ],
    "llama2": [
      { "provider": "local", "model": "llama2" }
    ]
  },
  "thresholds": {
    "failures_before_switch": 3,
    "initial_timeout_ms": 10000,
    "max_timeout_ms": 300000
  },
  "log_level": "info",
  "log_format": "text"
}
```

## Usage

```bash
# Start the server
./openmodel

# In another terminal, use OpenAI-compatible endpoints:
curl http://localhost:12345/v1/models
curl http://localhost:12345/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}'
```

## Default Port

The default port is **12345**. You can override this in your config:

```json
{
  "server": {
    "port": 12345,
    "host": "localhost"
  }
}
```

## API Endpoints

### OpenAI-Compatible API
- `GET /v1/models` - List available models
- `GET /v1/models/{model}` - Get model info
- `POST /v1/chat/completions` - Chat completion (SSE streaming supported)
- `POST /v1/completions` - Text completion (SSE streaming supported)
- `POST /v1/embeddings` - Create embeddings

### Server Endpoints
- `GET /` - Server status
- `GET /health` - Health check endpoint (for Docker healthchecks)

openmodel acts as a reverse proxy that:
1. Accepts requests at OpenAI-compatible endpoints
2. Routes to configured providers in fallback order
3. Tracks provider failures and automatically switches
4. Implements progressive timeout on complete failure

## Development

```bash
# Build locally
make build

# Build Docker image
make docker-build

# Run tests
make test

# Run single test
go test -race -v -run TestHandleRoot ./internal/server

# Generate coverage report
make cover

# Full check (fmt, vet, test)
make check

# Lint (requires golangci-lint)
make lint
```

## Test Coverage

Current test coverage: **77.2%**

| Package | Coverage |
|---------|----------|
| internal/logger | 100% |
| internal/state | 100% |
| internal/provider | 87.9% |
| internal/api/openai | 84.5% |
| internal/server | 82.7% |
| internal/config | 83.5% |
| cmd | 31.0% |

**All core packages ≥80% coverage** ✓

## Releasing

Create a new release:

```bash
# Create and push a tag
make release VERSION=v1.0.0

# This triggers GitHub Actions to:
# - Run tests
# - Build binaries (amd64, arm64)
# - Build and push Docker image to ghcr.io
# - Create GitHub release with binaries
```

## License

MIT
