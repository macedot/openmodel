# openmodel

A Go-based HTTP proxy server that is 100% compatible with Ollama's API. Acts as a drop-in replacement for Ollama, providing multi-model fallback across different providers.

## Features

- **100% Ollama API Compatible**: Drop-in replacement for Ollama
- **Multi-Provider Support**: OpenCode Zen, Ollama (extensible)
- **Automatic Fallback**: Tries providers in sequence on failure
- **Progressive Timeout**: Backs off when all providers are exhausted
- **Dual API Support**: Native Ollama API + OpenAI-compatible endpoints

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

### Manual Install

Build from source:

```bash
git clone https://github.com/macedot/openmodel.git
cd openmodel
go build
sudo mv openmodel /usr/local/bin/
```


## Configuration

Create `~/.config/openmodel/config.json`:

```json
{
  "server": {
    "port": 11435,
    "host": "localhost"
  },
  "providers": {
    "ollama": {
      "type": "ollama",
      "url": "http://localhost:11434"
    },
    "zen": {
      "type": "opencodezen",
      "url": "https://api.opencode.ai/v1",
      "apiKey": "${OPENCODE_API_KEY}"
    }
  },
  "models": {
    "glm": [
      { "provider": "zen", "model": "opencode-go/glm-5" },
      { "provider": "ollama", "model": "glm-5" }
    ]
  },
  "thresholds": {
    "failures_before_switch": 3,
    "initial_timeout_ms": 10000,
    "max_timeout_ms": 300000
  }
}
```

## Usage

```bash
# Start the server
./openmodel

# In another terminal, use it like Ollama:
curl http://localhost:11435/
curl http://localhost:11435/api/tags
curl http://localhost:11435/api/chat -d '{"model":"glm","messages":[{"role":"user","content":"hello"}]}'

# Or use OpenAI-compatible endpoints:
curl http://localhost:11435/v1/models
curl http://localhost:11435/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"glm","messages":[{"role":"user","content":"hello"}]}'
```

## API Endpoints

### Ollama Native API
- `GET /` - Server status
- `GET /api/version` - Version info
- `GET /api/tags` - List models
- `POST /api/chat` - Chat completion (NDJSON streaming)
- `POST /api/generate` - Text generation (NDJSON streaming)
- `POST /api/embed` - Create embeddings

### OpenAI-Compatible API
- `GET /v1/models` - List models
- `POST /v1/chat/completions` - Chat completion (SSE streaming)

## License

MIT
