// Package backend implements backend client connections
package backend

import (
	"context"
	"net/http"
)

// Client represents a backend API client
type Client interface {
	ChatCompletions(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error)
	ListModels(ctx context.Context) (*ModelList, error)
}

// OllamaClient implements Client for Ollama
type OllamaClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewOllamaClient creates a new Ollama client
func NewOllamaClient(baseURL string) *OllamaClient {
	return &OllamaClient{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

// ChatCompletions sends a chat completion request
func (c *OllamaClient) ChatCompletions(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// TODO: Implement
	return nil, nil
}

// ListModels lists available models
func (c *OllamaClient) ListModels(ctx context.Context) (*ModelList, error) {
	// TODO: Implement
	return nil, nil
}

// OpenCodeZenClient implements Client for OpenCode Zen
type OpenCodeZenClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenCodeZenClient creates a new OpenCode Zen client
func NewOpenCodeZenClient(baseURL, apiKey string) *OpenCodeZenClient {
	return &OpenCodeZenClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// ChatCompletions sends a chat completion request
func (c *OpenCodeZenClient) ChatCompletions(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// TODO: Implement
	return nil, nil
}

// ListModels lists available models
func (c *OpenCodeZenClient) ListModels(ctx context.Context) (*ModelList, error) {
	// TODO: Implement
	return nil, nil
}

// Types will be moved to types.go when fully implemented
type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ModelList struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

type ModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}
