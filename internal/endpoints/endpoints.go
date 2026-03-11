// Package endpoints defines API endpoint constants used across the application.
package endpoints

// Server endpoint paths (routes exposed by this proxy)
const (
	// OpenAI-compatible endpoints
	V1ChatCompletions = "/v1/chat/completions"
	V1Completions     = "/v1/completions"
	V1Embeddings      = "/v1/embeddings"
	V1Models          = "/v1/models"

	// Anthropic-compatible endpoints
	V1Messages = "/v1/messages"

	// Server endpoints
	Root   = "/"
	Health = "/health"
)

// Provider endpoint paths (used for upstream provider calls)
// These are relative to the provider's base URL
const (
	// OpenAI-style endpoints (without /v1 prefix for some providers)
	ChatCompletions = "/chat/completions"
	Completions     = "/completions"
	Embeddings      = "/embeddings"
	Models          = "/models"

	// Full paths with /v1 prefix (used by some providers)
	// Note: V1ChatCompletions is defined above for server routes
	V1CompletionsPath = "/v1/completions"
	V1EmbeddingsPath  = "/v1/embeddings"
	V1ModelsPath      = "/v1/models"
	V1MessagesPath    = "/v1/messages"
)