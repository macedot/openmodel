#!/bin/bash
# Test passthrough mode (no api_mode set) - should work as before
# These tests verify the default behavior when api_mode is not configured
PORT=${PORT:-12345}

echo "=== Testing OpenAI endpoint (passthrough) ==="
echo "Request: POST /v1/chat/completions with model 'default'"
time curl -v -s http://127.0.0.1:${PORT}/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d '{
        "model": "default",
        "max_tokens": 300,
        "messages": [{"role": "user", "content": "Say hello"}]
    }'
echo ""
echo ""

echo "=== Testing Anthropic endpoint (passthrough) ==="
echo "Request: POST /v1/messages with model 'default'"
time curl -v -s http://127.0.0.1:${PORT}/v1/messages \
    -H "Content-Type: application/json" \
    -H "anthropic-version: 2023-06-01" \
    -d '{
        "model": "default",
        "max_tokens": 300,
        "messages": [{"role": "user", "content": "Say hello"}]
    }'