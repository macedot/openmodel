#!/bin/bash
# Test api_mode=anthropic - OpenAI endpoint with Anthropic conversion
# Config must have a model with "api_mode": "anthropic"
# The proxy receives OpenAI format, converts to Anthropic for upstream, then converts response back
PORT=${PORT:-12345}
time curl -v -s http://127.0.0.1:${PORT}/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d '{
        "model": "anthropic-model",
        "max_tokens": 300,
        "messages": [{"role": "user", "content": "Say hello"}]
    }'