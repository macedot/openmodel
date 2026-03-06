#!/bin/bash
# Test script for chat completions endpoint
PORT=${PORT:-12345}
time curl -v -s http://127.0.0.1:${PORT}/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "smart",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful coding assistant. Provide clean code with comments and consider edge cases."
      },
      {
        "role": "user",
        "content": "Write a clean, efficient Python function that finds the longest palindromic substring in a given string. Include comments and handle edge cases like empty string or single character."
      }
    ],
    "temperature": 0.4,
    "max_tokens": 10000,
    "stream": false
  }' | jq .