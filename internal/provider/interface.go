// Package provider defines the provider interface and implementations
package provider

import (
	"context"

	"github.com/macedot/openmodel/internal/api/openai"
)

// ChatProvider handles chat completion operations
type ChatProvider interface {
	Chat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error)
	StreamChat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan openai.ChatCompletionResponse, error)
	StreamChatRaw(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan []byte, error)
}

// EmbeddingProvider handles embedding operations
type EmbeddingProvider interface {
	Embed(ctx context.Context, model string, input []string) (*openai.EmbeddingResponse, error)
}

// ModerationProvider handles content moderation
type ModerationProvider interface {
	Moderate(ctx context.Context, input string) (*openai.ModerationResponse, error)
}

// ModelLister lists available models
type ModelLister interface {
	ListModels(ctx context.Context) (*openai.ModelList, error)
}

// RawRequester handles raw request forwarding
type RawRequester interface {
	DoRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error)
	DoStreamRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) (<-chan []byte, error)
}

// CompletionProvider handles legacy completion operations
type CompletionProvider interface {
	Complete(ctx context.Context, model string, req *openai.CompletionRequest) (*openai.CompletionResponse, error)
	StreamComplete(ctx context.Context, model string, req *openai.CompletionRequest) (<-chan openai.CompletionResponse, error)
}

// Provider is the full interface combining all capabilities.
// Kept for backward compatibility - OpenAIProvider implements this.
type Provider interface {
	Name() string
	ModelLister
	ChatProvider
	RawRequester
	CompletionProvider
	Embed(ctx context.Context, model string, input []string) (*openai.EmbeddingResponse, error)
	Moderate(ctx context.Context, input string) (*openai.ModerationResponse, error)
}