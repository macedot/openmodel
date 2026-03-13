#!/bin/bash
source ./.env
time curl -v -s https://api.minimax.io/v1/chat/completions \
    -H "Authorization: Bearer ${API_KEY_MINIMAX}" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "MiniMax-M2.5",
        "messages": [{"role": "user", "content": "Say hello"}]
    }'
