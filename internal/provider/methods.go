// Package provider defines the provider interface and implementations
package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/endpoints"
)

// copyRequestOptions copies optional fields from src to dst.
// The stream parameter sets the Stream field on dst.
func copyRequestOptions(src *openai.ChatCompletionRequest, dst *openai.ChatCompletionRequest, stream bool) {
	dst.Stream = stream
	if src == nil {
		return
	}
	dst.Temperature = src.Temperature
	dst.TopP = src.TopP
	dst.N = src.N
	dst.Stop = src.Stop
	dst.MaxTokens = src.MaxTokens
	dst.PresencePenalty = src.PresencePenalty
	dst.FrequencyPenalty = src.FrequencyPenalty
	dst.LogitBias = src.LogitBias
	dst.User = src.User
	dst.ResponseFormat = src.ResponseFormat
	dst.Seed = src.Seed
	dst.Tools = src.Tools
	dst.ToolChoice = src.ToolChoice
	// Copy extra fields for provider-specific parameters
	if len(src.Extra) > 0 {
		dst.Extra = make(map[string]any, len(src.Extra))
		for k, v := range src.Extra {
			dst.Extra[k] = v
		}
	}
}

// hasThinkingEnabled checks if enable_thinking is set in the request options
func hasThinkingEnabled(opts *openai.ChatCompletionRequest) bool {
	if opts == nil || opts.Extra == nil {
		return false
	}
	// Check for enable_thinking field (common in llama.cpp)
	if v, ok := opts.Extra["enable_thinking"]; ok {
		if b, ok := v.(bool); ok && b {
			return true
		}
	}
	// Also check for think field (alternative naming)
	if v, ok := opts.Extra["think"]; ok {
		if b, ok := v.(bool); ok && b {
			return true
		}
	}
	return false
}

// ListModels lists available models from the provider
func (p *OpenAIProvider) ListModels(ctx context.Context) (*openai.ModelList, error) {
	req, err := http.NewRequest("GET", p.baseURL+endpoints.V1Models, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if err := p.handleHTTPResponse(resp, true); err != nil {
		return nil, err
	}

	var modelList openai.ModelList
	if err := json.NewDecoder(resp.Body).Decode(&modelList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &modelList, nil
}

// Chat sends a chat completion request
func (p *OpenAIProvider) Chat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	// Forward request AS IS, only change the model name
	req := openai.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
	}
	copyRequestOptions(opts, &req, false)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := p.buildRequest(ctx, body, endpoints.V1ChatCompletions)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequest(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if err := p.handleHTTPResponse(resp, true); err != nil {
		return nil, err
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var chatResp openai.ChatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w (raw response: %s)", err, string(respBody))
	}

	return &chatResp, nil
}

// StreamChat sends a chat request and streams the response
func (p *OpenAIProvider) StreamChat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan openai.ChatCompletionResponse, error) {
	// Forward request AS IS, only change the model name
	req := openai.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
	}
	copyRequestOptions(opts, &req, true)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := p.buildRequest(ctx, body, endpoints.V1ChatCompletions)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use streaming client (no timeout) for streaming requests - timeout applies only to
	// connection establishment and header reception, not to reading streaming body
	resp, err := p.getStreamingClient().Do(httpReq.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if err := p.handleHTTPResponse(resp, true); err != nil {
		return nil, err
	}


	// Parse function for chat streaming
	parseChat := func(data string) (openai.ChatCompletionResponse, error) {
		chunk, err := openai.StreamResponseToChunk([]byte(data))
		if err != nil {
			return openai.ChatCompletionResponse{}, err
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
		return chatResp, nil
	}

	ch := streamResponse(ctx, resp, parseChat, openai.IsStreamDone)

	return ch, nil
}

// StreamChatRaw streams chat completions as raw bytes for transparent proxying.
// It returns the raw SSE data lines without any parsing - preserves all fields exactly as received.
func (p *OpenAIProvider) StreamChatRaw(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan []byte, error) {
	// Forward request AS IS, only change the model name
	req := openai.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
	}
	copyRequestOptions(opts, &req, true)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := p.buildRequest(ctx, body, endpoints.V1ChatCompletions)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use streaming client (no timeout) for streaming requests
	resp, err := p.getStreamingClient().Do(httpReq.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if err := p.handleHTTPResponse(resp, true); err != nil {
		return nil, err
	}

	// Return raw SSE channel - no parsing, just forward bytes
	ch := make(chan []byte, 10)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		bufPtr := streamBufferPool.Get().(*[]byte)
		defer streamBufferPool.Put(bufPtr)
		scanner.Buffer(*bufPtr, maxTokenSize)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()
			// Forward raw line as-is
			select {
			case ch <- []byte(line):
			case <-ctx.Done():
				return
			}
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

	httpReq, err := p.buildRequest(ctx, body, endpoints.V1Completions)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequest(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if err := p.handleHTTPResponse(resp, true); err != nil {
		return nil, err
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

	httpReq, err := p.buildRequest(ctx, body, endpoints.V1Completions)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use streaming client (no timeout) for streaming requests
	resp, err := p.getStreamingClient().Do(httpReq.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if err := p.handleHTTPResponse(resp, true); err != nil {
		return nil, err
	}

	// Parse function for completion streaming
	parseComplete := func(data string) (openai.CompletionResponse, error) {
		var compResp openai.CompletionResponse
		if err := json.Unmarshal([]byte(data), &compResp); err != nil {
			return openai.CompletionResponse{}, err
		}
		return compResp, nil
	}

	// Done check for completion streaming
	isCompleteDone := func(data string) bool {
		return data == "[DONE]"
	}

	ch := streamResponse(ctx, resp, parseComplete, isCompleteDone)

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

	httpReq, err := p.buildRequest(ctx, body, endpoints.V1Embeddings)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequest(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if err := p.handleHTTPResponse(resp, true); err != nil {
		return nil, err
	}

	var embedResp openai.EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &embedResp, nil
}

// Moderate checks content for policy violations
func (p *OpenAIProvider) Moderate(ctx context.Context, input string) (*openai.ModerationResponse, error) {
	req := openai.ModerationRequest{
		Input: input,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := p.buildRequest(ctx, body, endpoints.V1Moderations)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequest(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if err := p.handleHTTPResponse(resp, true); err != nil {
		return nil, err
	}

	var modResp openai.ModerationResponse
	if err := json.NewDecoder(resp.Body).Decode(&modResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &modResp, nil
}