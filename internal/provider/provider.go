// Package provider defines the provider interface and implementations
package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/macedot/openmodel/internal/api/ollama"
	"github.com/macedot/openmodel/internal/api/openai"
)

// Provider defines the interface for LLM providers
type Provider interface {
	// Name returns the provider name
	Name() string

	// Chat sends a chat completion request and returns the response
	Chat(ctx context.Context, model string, messages []ollama.Message, opts *ollama.Options) (*ollama.ChatResponse, error)

	// StreamChat sends a chat request and streams the response
	StreamChat(ctx context.Context, model string, messages []ollama.Message, opts *ollama.Options) (<-chan ollama.ChatResponse, error)

	// Generate sends a generation request and returns the response
	Generate(ctx context.Context, model string, prompt string, opts *ollama.Options) (*ollama.GenerateResponse, error)

	// StreamGenerate sends a generation request and streams the response
	StreamGenerate(ctx context.Context, model string, prompt string, opts *ollama.Options) (<-chan ollama.GenerateResponse, error)

	// ListModels returns the list of available models
	ListModels(ctx context.Context) ([]ollama.ListModelResponse, error)

	// Embed creates embeddings for the given input
	Embed(ctx context.Context, model string, input []string) (*ollama.EmbedResponse, error)
}

// OllamaProvider implements Provider for Ollama
type OllamaProvider struct {
	baseURL    string
	httpClient *http.Client
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider(baseURL string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaProvider{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{},
	}
}

// Name returns the provider name
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// Chat sends a chat completion request
func (p *OllamaProvider) Chat(ctx context.Context, model string, messages []ollama.Message, opts *ollama.Options) (*ollama.ChatResponse, error) {
	req := ollama.ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   boolPtr(false),
		Options:  opts,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if se := ollama.ParseStatusError(body); se != nil {
			return nil, se
		}
		return nil, fmt.Errorf("ollama request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ollama.ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

// StreamChat sends a chat request and streams the response
func (p *OllamaProvider) StreamChat(ctx context.Context, model string, messages []ollama.Message, opts *ollama.Options) (<-chan ollama.ChatResponse, error) {
	req := ollama.ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   boolPtr(true),
		Options:  opts,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if se := ollama.ParseStatusError(body); se != nil {
			return nil, se
		}
		return nil, fmt.Errorf("ollama request failed with status %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan ollama.ChatResponse, 10)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			var chatResp ollama.ChatResponse
			if err := json.Unmarshal(scanner.Bytes(), &chatResp); err != nil {
				continue // Skip malformed lines
			}
			ch <- chatResp
			if chatResp.Done {
				break
			}
		}
	}()

	return ch, nil
}

// Generate sends a generation request
func (p *OllamaProvider) Generate(ctx context.Context, model string, prompt string, opts *ollama.Options) (*ollama.GenerateResponse, error) {
	req := ollama.GenerateRequest{
		Model:   model,
		Prompt:  prompt,
		Stream:  boolPtr(false),
		Options: opts,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if se := ollama.ParseStatusError(body); se != nil {
			return nil, se
		}
		return nil, fmt.Errorf("ollama request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var genResp ollama.GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &genResp, nil
}

// StreamGenerate sends a generation request and streams the response
func (p *OllamaProvider) StreamGenerate(ctx context.Context, model string, prompt string, opts *ollama.Options) (<-chan ollama.GenerateResponse, error) {
	req := ollama.GenerateRequest{
		Model:   model,
		Prompt:  prompt,
		Stream:  boolPtr(true),
		Options: opts,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if se := ollama.ParseStatusError(body); se != nil {
			return nil, se
		}
		return nil, fmt.Errorf("ollama request failed with status %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan ollama.GenerateResponse, 10)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			var genResp ollama.GenerateResponse
			if err := json.Unmarshal(scanner.Bytes(), &genResp); err != nil {
				continue // Skip malformed lines
			}
			ch <- genResp
			if genResp.Done {
				break
			}
		}
	}()

	return ch, nil
}

// ListModels returns the list of available models
func (p *OllamaProvider) ListModels(ctx context.Context) ([]ollama.ListModelResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var listResp ollama.ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return listResp.Models, nil
}

// Embed creates embeddings for the given input
func (p *OllamaProvider) Embed(ctx context.Context, model string, input []string) (*ollama.EmbedResponse, error) {
	req := ollama.EmbedRequest{
		Model: model,
		Input: input,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if se := ollama.ParseStatusError(body); se != nil {
			return nil, se
		}
		return nil, fmt.Errorf("ollama request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var embedResp ollama.EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &embedResp, nil
}

// OpenCodeZenProvider implements Provider for OpenCodeZen API (OpenAI-compatible)
type OpenCodeZenProvider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenCodeZenProvider creates a new OpenCodeZen provider
func NewOpenCodeZenProvider(apiKey string) *OpenCodeZenProvider {
	return &OpenCodeZenProvider{
		baseURL:    "https://api.opencode.ai/v1",
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// NewOpenCodeZenProviderWithURL creates a new OpenCodeZen provider with custom URL
func NewOpenCodeZenProviderWithURL(baseURL, apiKey string) *OpenCodeZenProvider {
	if baseURL == "" {
		baseURL = "https://api.opencode.ai/v1"
	}
	return &OpenCodeZenProvider{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// Name returns the provider name
func (p *OpenCodeZenProvider) Name() string {
	return "opencodezen"
}

// Chat sends a chat completion request using OpenAI-compatible API
func (p *OpenCodeZenProvider) Chat(ctx context.Context, model string, messages []ollama.Message, opts *ollama.Options) (*ollama.ChatResponse, error) {
	// Convert Ollama messages to OpenAI format
	openAIMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		openAIMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	req := openai.ChatCompletionRequest{
		Model:    model,
		Messages: openAIMessages,
	}

	// Convert options if provided
	if opts != nil {
		req.Temperature = &opts.Temperature
		req.TopP = &opts.TopP
		req.Seed = &opts.Seed
		if opts.Stop != nil {
			req.Stop = opts.Stop
		}
		if opts.NumPredict > 0 {
			req.MaxTokens = &opts.NumPredict
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if er := openai.ParseErrorResponse(body); er != nil {
			return nil, er
		}
		return nil, fmt.Errorf("opencodezen request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp openai.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert OpenAI response to Ollama format
	return p.convertChatResponse(&chatResp), nil
}

// StreamChat sends a chat request and streams the response
func (p *OpenCodeZenProvider) StreamChat(ctx context.Context, model string, messages []ollama.Message, opts *ollama.Options) (<-chan ollama.ChatResponse, error) {
	// Convert Ollama messages to OpenAI format
	openAIMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		openAIMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	req := openai.ChatCompletionRequest{
		Model:    model,
		Messages: openAIMessages,
		Stream:   true,
	}

	if opts != nil {
		req.Temperature = &opts.Temperature
		req.TopP = &opts.TopP
		req.Seed = &opts.Seed
		if opts.Stop != nil {
			req.Stop = opts.Stop
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if er := openai.ParseErrorResponse(body); er != nil {
			return nil, er
		}
		return nil, fmt.Errorf("opencodezen request failed with status %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan ollama.ChatResponse, 10)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if openai.IsStreamDone(data) {
				break
			}

			chunk, err := openai.StreamResponseToChunk([]byte(data))
			if err != nil {
				continue
			}

			// Convert to Ollama format
			for _, choice := range chunk.Choices {
				chatResp := ollama.ChatResponse{
					Model:     chunk.Model,
					CreatedAt: time.Unix(chunk.Created, 0),
					Message: &ollama.Message{
						Role:    choice.Delta.Role,
						Content: choice.Delta.Content,
					},
					Done: choice.FinishReason != nil,
				}
				ch <- chatResp
			}
		}
	}()

	return ch, nil
}

// Generate is not directly supported by OpenAI-compatible APIs
// It converts to a chat request with a single user message
func (p *OpenCodeZenProvider) Generate(ctx context.Context, model string, prompt string, opts *ollama.Options) (*ollama.GenerateResponse, error) {
	// Convert generate to chat
	messages := []ollama.Message{
		{Role: "user", Content: prompt},
	}

	chatResp, err := p.Chat(ctx, model, messages, opts)
	if err != nil {
		return nil, err
	}

	// Convert chat response to generate response
	return &ollama.GenerateResponse{
		Model:     chatResp.Model,
		CreatedAt: chatResp.CreatedAt,
		Response:  chatResp.Message.Content,
		Done:      chatResp.Done,
		Metrics:   chatResp.Metrics,
	}, nil
}

// StreamGenerate sends a generation request and streams the response
func (p *OpenCodeZenProvider) StreamGenerate(ctx context.Context, model string, prompt string, opts *ollama.Options) (<-chan ollama.GenerateResponse, error) {
	// Convert generate to chat
	messages := []ollama.Message{
		{Role: "user", Content: prompt},
	}

	chatCh, err := p.StreamChat(ctx, model, messages, opts)
	if err != nil {
		return nil, err
	}

	genCh := make(chan ollama.GenerateResponse, 10)
	go func() {
		defer close(genCh)
		for chatResp := range chatCh {
			genCh <- ollama.GenerateResponse{
				Model:     chatResp.Model,
				CreatedAt: chatResp.CreatedAt,
				Response:  chatResp.Message.Content,
				Done:      chatResp.Done,
				Metrics:   chatResp.Metrics,
			}
		}
	}()

	return genCh, nil
}

// ListModels returns the list of available models
func (p *OpenCodeZenProvider) ListModels(ctx context.Context) ([]ollama.ListModelResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencodezen request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var modelList openai.ModelList
	if err := json.NewDecoder(resp.Body).Decode(&modelList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to Ollama format
	models := make([]ollama.ListModelResponse, len(modelList.Data))
	for i, m := range modelList.Data {
		models[i] = ollama.ListModelResponse{
			Name:  m.ID,
			Model: m.ID,
		}
	}

	return models, nil
}

// Embed creates embeddings using the OpenAI-compatible API
func (p *OpenCodeZenProvider) Embed(ctx context.Context, model string, input []string) (*ollama.EmbedResponse, error) {
	req := openai.EmbeddingRequest{
		Model: model,
		Input: input,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if er := openai.ParseErrorResponse(body); er != nil {
			return nil, er
		}
		return nil, fmt.Errorf("opencodezen request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var embedResp openai.EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to Ollama format
	embeddings := make([][]float64, len(embedResp.Data))
	for i, d := range embedResp.Data {
		embeddings[i] = d.Embedding
	}

	return &ollama.EmbedResponse{
		Model:      embedResp.Model,
		Embeddings: embeddings,
	}, nil
}

// convertChatResponse converts OpenAI response to Ollama format
func (p *OpenCodeZenProvider) convertChatResponse(resp *openai.ChatCompletionResponse) *ollama.ChatResponse {
	if len(resp.Choices) == 0 {
		return &ollama.ChatResponse{
			Model: resp.Model,
			Done:  true,
		}
	}

	choice := resp.Choices[0]
	var message *ollama.Message
	if choice.Message != nil {
		message = &ollama.Message{
			Role:    choice.Message.Role,
			Content: choice.Message.Content,
		}
	}

	var metrics *ollama.Metrics
	if resp.Usage != nil {
		metrics = &ollama.Metrics{
			PromptEvalCount: resp.Usage.PromptTokens,
			EvalCount:       resp.Usage.CompletionTokens,
		}
	}

	return &ollama.ChatResponse{
		Model:     resp.Model,
		CreatedAt: time.Unix(resp.Created, 0),
		Message:   message,
		Done:      choice.FinishReason != "",
		Metrics:   metrics,
	}
}

// boolPtr returns a pointer to a bool
func boolPtr(b bool) *bool {
	return &b
}
