// Package provider defines the provider interface and implementations
package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/logger"
)

// maxResponseBodySize defines the maximum size of response body to read for error handling
const maxResponseBodySize = 1024 * 1024 // 1MB

// maxTokenSize defines the maximum token size for streaming buffer
const maxTokenSize = 1024 * 1024 // 1MB

// debugDir is the directory for saving debug files
const debugDir = ".openmodel_debug"

// saveDebugFile saves request or response to a debug file
func saveDebugFile(requestID string, fileType string, content []byte) {
	// Create debug directory if it doesn't exist
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		logger.Trace("Failed to create debug directory", "error", err)
		return
	}

	// Generate filename with timestamp and request_id
	timestamp := time.Now().Format("20060102-150405.000")
	filename := fmt.Sprintf("%s_%s_%s_%s.json", fileType, timestamp, requestID, uuid.New().String()[:8])
	filepath := filepath.Join(debugDir, filename)

	// Write content to file
	if err := os.WriteFile(filepath, content, 0644); err != nil {
		logger.Trace("Failed to write debug file", "file", filename, "error", err)
		return
	}

	logger.Trace("Saved debug file", "type", fileType, "request_id", requestID, "file", filename)
}

var (
	// streamBufferPool is a pool for reusing streaming buffers (1MB each)
	streamBufferPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, maxTokenSize)
			return &buf
		},
	}
)

// getRequestID extracts or generates a request ID for debugging
func getRequestID(opts *openai.ChatCompletionRequest) string {
	if opts != nil && opts.User != "" {
		return opts.User
	}
	return uuid.New().String()
}

// Provider defines the interface for LLM providers
type Provider interface {
	// Name returns the provider name
	Name() string

	// ListModels lists available models from the provider
	ListModels(ctx context.Context) (*openai.ModelList, error)

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

	// Moderate checks content for policy violations
	Moderate(ctx context.Context, input string) (*openai.ModerationResponse, error)
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
		name:    name,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
				DialContext: (&net.Dialer{
					Timeout: 10 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
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

// handleHTTPResponse checks the HTTP response status and returns an error if not OK.
// The closeBody parameter controls whether to close the response body (use true for streaming).
func (p *OpenAIProvider) handleHTTPResponse(resp *http.Response, closeBody bool) error {
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
		if closeBody {
			resp.Body.Close()
		}
		if er := openai.ParseErrorResponse(respBody); er != nil {
			return er
		}
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

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

// streamResponse is a generic streaming helper that reads from an HTTP response
// and sends parsed responses to a channel.
// The parseFunc receives the data string (already stripped of "data: " prefix) and
// should return the parsed response and any error (errors are skipped).
// The isDoneFunc checks if the data indicates streaming is complete.
func streamResponse[T any](ctx context.Context, resp *http.Response, parseFunc func(data string) (T, error), isDoneFunc func(data string) bool) <-chan T {
	ch := make(chan T, 10)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		// Get buffer from pool for large streaming responses
		bufPtr := streamBufferPool.Get().(*[]byte)
		defer streamBufferPool.Put(bufPtr)
		scanner.Buffer(*bufPtr, maxTokenSize)

		for scanner.Scan() {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if isDoneFunc(data) {
				break
			}

			resp, err := parseFunc(data)
			if err != nil {
				continue
			}

			// Non-blocking send with context check
			select {
			case ch <- resp:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

// ListModels lists available models from the provider
func (p *OpenAIProvider) ListModels(ctx context.Context) (*openai.ModelList, error) {
	req, err := http.NewRequest("GET", p.baseURL+"/models", nil)
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

	// DEBUG: Save request to file
	requestID := getRequestID(opts)
	logger.Trace("Chat request", "request_id", requestID, "model", model, "messages_count", len(messages))
	saveDebugFile(requestID, "request", body)

	httpReq, err := p.buildRequest(body, "/chat/completions")
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

	// DEBUG: Save response to file (AS IS)
	logger.Trace("Chat response", "request_id", requestID, "status", resp.StatusCode, "size", len(respBody))
	saveDebugFile(requestID, "response", respBody)

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

	// DEBUG: Save request to file
	requestID := getRequestID(opts)
	logger.Trace("StreamChat request", "request_id", requestID, "model", model, "messages_count", len(messages))
	saveDebugFile(requestID, "request", body)

	httpReq, err := p.buildRequest(body, "/chat/completions")
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequest(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// DEBUG: Log initial response (AS IS)
	logger.Trace("StreamChat response start", "request_id", requestID, "status", resp.StatusCode)

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

	httpReq, err := p.buildRequest(body, "/completions")
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.doRequest(ctx, httpReq)
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

	httpReq, err := p.buildRequest(body, "/embeddings")
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

	httpReq, err := p.buildRequest(body, "/moderations")
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
