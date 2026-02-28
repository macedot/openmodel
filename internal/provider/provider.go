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

	"github.com/macedot/openmodel/internal/api/openai"
)

// Provider defines the interface for LLM providers
type Provider interface {
	// Name returns the provider name
	Name() string

	// Chat sends a chat completion request and returns the response
	Chat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error)

	// StreamChat sends a chat request and streams the response
	StreamChat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan openai.ChatCompletionResponse, error)

	// Complete sends a completion request and returns the response
	Complete(ctx context.Context, model string, req *openai.CompletionRequest) (*openai.CompletionResponse, error)

	// StreamComplete sends a completion request and streams the response
	StreamComplete(ctx context.Context, model string, req *openai.CompletionRequest) (<-chan openai.CompletionResponse, error)

	// Embed creates embeddings for the given input
	Embed(ctx context.Context, model string, input []string) (*openai.EmbeddingResponse, error)
}

// OpenAIProvider implements Provider for OpenAI-compatible APIs
type OpenAIProvider struct {
	name       string
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAIProvider creates a new OpenAI-compatible provider
func NewOpenAIProvider(name, baseURL, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		name:       name,
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// Name returns the provider name
func (p *OpenAIProvider) Name() string {
	return p.name
}

func (p *OpenAIProvider) buildRequest(body []byte, path string) (*http.Request, error) {
	req, err := http.NewRequest("POST", p.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	return req, nil
}

func (p *OpenAIProvider) doRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	resp, err := p.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Chat sends a chat completion request
func (p *OpenAIProvider) Chat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	req := openai.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
	}
	if opts != nil {
		req.Temperature = opts.Temperature
		req.TopP = opts.TopP
		req.N = opts.N
		req.Stream = false
		req.Stop = opts.Stop
		req.MaxTokens = opts.MaxTokens
		req.PresencePenalty = opts.PresencePenalty
		req.FrequencyPenalty = opts.FrequencyPenalty
		req.LogitBias = opts.LogitBias
		req.User = opts.User
		req.ResponseFormat = opts.ResponseFormat
		req.Seed = opts.Seed
		req.Tools = opts.Tools
		req.ToolChoice = opts.ToolChoice
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := p.buildRequest(body, "/chat/completions")
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequest(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		if er := openai.ParseErrorResponse(respBody); er != nil {
			return nil, er
		}
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp openai.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

// StreamChat sends a chat request and streams the response
func (p *OpenAIProvider) StreamChat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan openai.ChatCompletionResponse, error) {
	req := openai.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
		Stream:   true,
	}
	if opts != nil {
		req.Temperature = opts.Temperature
		req.TopP = opts.TopP
		req.N = opts.N
		req.Stop = opts.Stop
		req.MaxTokens = opts.MaxTokens
		req.PresencePenalty = opts.PresencePenalty
		req.FrequencyPenalty = opts.FrequencyPenalty
		req.LogitBias = opts.LogitBias
		req.User = opts.User
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := p.buildRequest(body, "/chat/completions")
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequest(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if er := openai.ParseErrorResponse(respBody); er != nil {
			return nil, er
		}
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan openai.ChatCompletionResponse, 10)
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

			chatResp := openai.ChatCompletionResponse{
				ID:      chunk.ID,
				Object:  chunk.Object,
				Created: chunk.Created,
				Model:   chunk.Model,
			}
			for _, c := range chunk.Choices {
				finishReason := ""
				if c.FinishReason != nil {
					finishReason = *c.FinishReason
				}
				chatResp.Choices = append(chatResp.Choices, openai.ChatCompletionChoice{
					Index:        c.Index,
					Delta:        &c.Delta,
					FinishReason: finishReason,
				})
			}
			ch <- chatResp
		}
	}()

	return ch, nil
}

// Complete sends a completion request
func (p *OpenAIProvider) Complete(ctx context.Context, model string, req *openai.CompletionRequest) (*openai.CompletionResponse, error) {
	req.Model = model
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := p.buildRequest(body, "/completions")
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequest(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		if er := openai.ParseErrorResponse(respBody); er != nil {
			return nil, er
		}
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var compResp openai.CompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&compResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &compResp, nil
}

// StreamComplete sends a completion request and streams the response
func (p *OpenAIProvider) StreamComplete(ctx context.Context, model string, req *openai.CompletionRequest) (<-chan openai.CompletionResponse, error) {
	req.Model = model
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := p.buildRequest(body, "/completions")
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequest(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if er := openai.ParseErrorResponse(respBody); er != nil {
			return nil, er
		}
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan openai.CompletionResponse, 10)
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
			if data == "[DONE]" {
				break
			}

			var compResp openai.CompletionResponse
			if err := json.Unmarshal([]byte(data), &compResp); err != nil {
				continue
			}
			ch <- compResp
		}
	}()

	return ch, nil
}

// Embed creates embeddings for the given input
func (p *OpenAIProvider) Embed(ctx context.Context, model string, input []string) (*openai.EmbeddingResponse, error) {
	req := openai.EmbeddingRequest{
		Model: model,
		Input: input,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := p.buildRequest(body, "/embeddings")
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequest(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		if er := openai.ParseErrorResponse(respBody); er != nil {
			return nil, er
		}
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var embedResp openai.EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &embedResp, nil
}
