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
	applogger "github.com/macedot/openmodel/internal/logger"
)

// DoRequest forwards a raw request body to the provider endpoint
func (p *OpenAIProvider) DoRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error) {
	// Get request ID and original URL from context
	requestID := "unknown"
	if id, ok := ctx.Value("request_id").(string); ok && id != "" {
		requestID = id
	}
	originalURL, _ := ctx.Value("original_url").(string)

	// Create per-request trace file
	traceFile := createTraceFileForRequest(p.name, requestID)
	if traceFile != nil {
		defer traceFile.Close()
		redactedHeaders := applogger.RedactHeaders(headers)
		headersJSON, _ := json.Marshal(redactedHeaders)
		fmt.Fprintf(traceFile, "{\"type\":\"request\",\"provider\":\"%s\",\"original_url\":\"%s\",\"endpoint\":\"%s\",\"url\":\"%s\",\"headers\":%s,\"body\":%s}\n",
			p.name, originalURL, endpoint, p.baseURL+endpoint, headersJSON, body)
	}

	req, err := p.buildRequest(ctx, body, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add additional headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := p.doRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Write response to trace file
	if traceFile != nil {
		fmt.Fprintf(traceFile, "{\"type\":\"response\",\"provider\":\"%s\",\"original_url\":\"%s\",\"endpoint\":\"%s\",\"url\":\"%s\",\"status\":%d,\"body\":%s}\n",
			p.name, originalURL, endpoint, p.baseURL+endpoint, resp.StatusCode, respBody)
	}

	if resp.StatusCode != http.StatusOK {
		if er := openai.ParseErrorResponse(respBody); er != nil {
			return nil, er
		}
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// DoStreamRequest forwards a raw streaming request and returns SSE channel
func (p *OpenAIProvider) DoStreamRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) (<-chan []byte, error) {
	// Get request ID and original URL from context
	requestID := "unknown"
	if id, ok := ctx.Value("request_id").(string); ok && id != "" {
		requestID = id
	}
	originalURL, _ := ctx.Value("original_url").(string)

	// Create per-request trace file
	traceFile := createTraceFileForRequest(p.name, requestID)
	if traceFile != nil {
		redactedHeaders := applogger.RedactHeaders(headers)
		headersJSON, _ := json.Marshal(redactedHeaders)
		fmt.Fprintf(traceFile, "{\"type\":\"request\",\"provider\":\"%s\",\"original_url\":\"%s\",\"endpoint\":\"%s\",\"url\":\"%s\",\"headers\":%s,\"body\":%s}\n",
			p.name, originalURL, endpoint, p.baseURL+endpoint, headersJSON, body)
	}

	req, err := p.buildRequest(ctx, body, endpoint)
	if err != nil {
		if traceFile != nil {
			traceFile.Close()
		}
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add additional headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Use streaming client (no timeout) for streaming requests
	resp, err := p.streamingClient().Do(req.WithContext(ctx))
	if err != nil {
		if traceFile != nil {
			traceFile.Close()
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Read error body
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
		if traceFile != nil {
			fmt.Fprintf(traceFile, "{\"type\":\"error\",\"provider\":\"%s\",\"original_url\":\"%s\",\"endpoint\":\"%s\",\"url\":\"%s\",\"status\":%d,\"body\":%s}\n",
				p.name, originalURL, endpoint, p.baseURL+endpoint, resp.StatusCode, respBody)
			traceFile.Close()
		}
		resp.Body.Close()
		if er := openai.ParseErrorResponse(respBody); er != nil {
			return nil, er
		}
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Return raw SSE channel
	ch := make(chan []byte, 10)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		if traceFile != nil {
			defer traceFile.Close()
		}

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

			// Dump to trace file in real-time
			if traceFile != nil {
				fmt.Fprintf(traceFile, "{\"type\":\"stream\",\"provider\":\"%s\",\"line\":%q}\n", p.name, line)
			}

			select {
			case ch <- []byte(line):
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}