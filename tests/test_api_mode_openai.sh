#!/bin/bash
# Test api_mode=openai - Anthropic endpoint with OpenAI conversion
# Config must have a model with "api_mode": "openai"
# The proxy receives Anthropic format, converts to OpenAI for upstream, then converts response back
PORT=${PORT:-12345}
time curl -v -s http://127.0.0.1:${PORT}/v1/messages \
    -H "Content-Type: application/json" \
    -H "anthropic-version: 2023-06-01" \
    -d '{
        "model": "openai-model",
        "max_tokens": 300,
        "messages": [{"role": "user", "content": "Say hello"}]
    }'