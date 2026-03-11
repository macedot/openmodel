// Package endpoints defines API endpoint constants used across the application.
package endpoints

// OpenAI endpoints (OpenAI-compatible API paths with /v1 prefix)
const (
	V1ChatCompletions = "/v1/chat/completions"
	V1Models          = "/v1/models"
)

// Anthropic endpoints (Claude API paths)
const (
	V1Messages = "/v1/messages"
)

// Internal endpoints (server routes)
const (
	Root   = "/"
	Health = "/health"
)