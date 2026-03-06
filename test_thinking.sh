#!/bin/bash
# Test script for thinking-capable models

PORT=${PORT:-12345}
MODEL=${MODEL:-qwen3}

time curl -v -s http://127.0.0.1:${PORT}/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL}"'",
    "messages": [{"role": "user", "content": "What is 17 × 23?"}],
    "stream": true
  }'