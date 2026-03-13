// Package server implements the HTTP server and handlers
package server

import (
	"time"

	"github.com/macedot/openmodel/internal/endpoints"
)

// HTTP Server defaults
const (
	DefaultReadTimeout    = 30 * time.Second
	DefaultWriteTimeout   = 120 * time.Second
	DefaultIdleTimeout    = 120 * time.Second
	DefaultMaxHeaderBytes = 1 << 20 // 1MB
)

// Request/Response size limits
const (
	DefaultMaxRequestBody = 50 * 1024 * 1024 // 50MB
	MaxResponseBodySize   = 1 * 1024 * 1024  // 1MB
	MaxStreamBufferSize   = 1 * 1024 * 1024  // 1MB

	MaxResponseCaptureSize = 10 * 1024 * 1024 // 10MB - for logging
)

// Rate limiting defaults
const (
	DefaultRequestsPerSecond = 10
	DefaultBurst             = 20
	DefaultCleanupInterval   = 60 * time.Second
)

// HTTP Headers
const (
	HeaderContentType         = "Content-Type"
	HeaderAuthorization       = "Authorization"
	HeaderRequestID           = "X-Request-ID"
	HeaderRetryAfter          = "Retry-After"
	HeaderXForwardedFor       = "X-Forwarded-For"
	HeaderXRealIP             = "X-Real-IP"
	HeaderXRateLimitLimit     = "X-RateLimit-Limit"
	HeaderXRateLimitRemaining = "X-RateLimit-Remaining"
	HeaderAnthropicVersion    = "anthropic-version"
)

// Anthropic API constants
const (
	AnthropicAPIVersion = "2023-06-01"
)

// Content types
const (
	ContentTypeJSON = "application/json"
	ContentTypeSSE  = "text/event-stream"
)

// SSE stream markers
const (
	SSEDataPrefix = "data: "
	SSEDataDone   = "data: [DONE]"
	SSEDataSuffix = "\n\n"
)

// Endpoint aliases - re-exported from endpoints package for convenience

// OpenAI endpoints
const (
	EndpointV1ChatCompletions = endpoints.V1ChatCompletions
	EndpointV1Models          = endpoints.V1Models
)

// Anthropic endpoints
const (
	EndpointV1Messages = endpoints.V1Messages
)

// Internal endpoints
const (
	EndpointRoot   = endpoints.Root
	EndpointHealth = endpoints.Health
)
