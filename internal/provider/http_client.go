// Package provider defines the provider interface and implementations
package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/macedot/openmodel/internal/api/openai"
)

// maxResponseBodySize defines the maximum size of response body to read for error handling
const maxResponseBodySize = 1024 * 1024 // 1MB

// buildRequest creates an HTTP request with proper headers
func (p *OpenAIProvider) buildRequest(ctx context.Context, body []byte, path string) (*http.Request, error) {
	// Combine baseURL with path, avoiding /v1/v1 duplication
	fullURL := p.baseURL
	// If baseURL ends with /v1 and path starts with /v1, remove one /v1
	if strings.HasSuffix(p.baseURL, "/v1") && strings.HasPrefix(path, "/v1") {
		fullURL = p.baseURL + path[3:] // Remove /v1 from path
	} else if strings.HasPrefix(path, "/") {
		fullURL = p.baseURL + path
	} else {
		fullURL = p.baseURL + "/" + path
	}
	req, err := http.NewRequest("POST", fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	// Propagate request ID for distributed tracing
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	return req.WithContext(ctx), nil
}

// doRequest executes an HTTP request
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
