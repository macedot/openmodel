# Implementation Plan: Own Models + Selection Strategies

## Goal
Implement:
1. Own models - model names without provider prefix (e.g., "gpt-4" instead of "openai/gpt-4")
2. Global thresholds with provider override
3. Selection strategies: fallback, round-robin, random

## Config Schema Changes

### 1. Own Models
- Keep current "provider/model" syntax in models map
- Add provider-level `models` array to define available models per provider
- When looking up a model alias, if no "provider/" prefix found, search providers' models lists

### 2. Thresholds
- Keep global `thresholds` as default
- Provider `thresholds` already supported - make it override global
- Add helper `GetThresholds(providerName)` that returns provider-specific or global

### 3. Selection Strategy
Add to Config:
```go
SelectionStrategy string `json:"selection_strategy"` // "fallback" | "round-robin" | "random"
```

Schema:
```json
"selection_strategy": {
  "type": "string",
  "enum": ["fallback", "round-robin", "random"],
  "default": "fallback"
}
```

## Go Code Changes

### 1. Config (`internal/config/config.go`)
- Add `SelectionStrategy` to Config struct
- Add `GetThresholds(providerName)` method that returns provider thresholds or global

### 2. State (`internal/state/state.go`)
- Add `roundRobinIndex` map for round-robin tracking per model
- Add `NextRoundRobin(model string, total int) int` method

### 3. Server/Handlers (`internal/server/handlers_helpers.go`)
- Update `findProviderWithFailover` to support strategies:
  - fallback: current behavior
  - round-robin: use state.RoundRobinIndex
  - random: shuffle or pick random from available

## Config File Example

```json
{
  "selection_strategy": "round-robin",
  "thresholds": {
    "failures_before_switch": 3,
    "initial_timeout_ms": 10000,
    "max_timeout_ms": 300000
  },
  "providers": {
    "ollama": {
      "url": "...",
      "models": ["glm-5", "llama2"],
      "thresholds": {
        "failures_before_switch": 5
      }
    }
  },
  "models": {
    "any": ["ollama/glm-5", "opencode/gpt-4"],
    "glm-5": ["glm-5"],  // own model - maps to provider with that model
    "gpt-4": ["opencode/gpt-4"]
  }
}
```

## Tasks

- [ ] 1. Update config.schema.json - add selection_strategy, refine thresholds
- [ ] 2. Update config.go - add SelectionStrategy, GetThresholds helper
- [ ] 3. Update state.go - add round-robin tracking
- [ ] 4. Update handlers_helpers.go - implement selection strategies
- [ ] 5. Update config.json with new features
- [ ] 6. Run tests and verify
