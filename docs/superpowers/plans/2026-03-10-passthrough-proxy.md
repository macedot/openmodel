# Passthrough Proxy Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert OpenModel to a pure transparent passthrough proxy that forwards requests directly to providers without conversion.

**Architecture:** Remove all format conversion code. Handlers forward request bodies as-is to provider endpoints, return responses as-is. Only model name resolution and failover logic remain.

**Tech Stack:** Go, standard library net/http, existing provider interface pattern

---

## Chunk 1: Simplify Provider Interface

### Task 1: Add Raw Forwarding Methods to Provider Interface

**Files:**
- Modify: `internal/provider/provider.go:36-65`

- [ ] **Step 1: Add new raw forwarding methods to Provider interface**

Add these methods after the existing interface methods:

```go
// DoRequest forwards a raw request body to an endpoint and returns the response.
// endpoint is the path like "/v1/chat/completions" or "/v1/messages".
// body is the raw JSON request body.
// headers are additional headers to include (e.g., anthropic-version).
// Returns the raw response body from the provider.
DoRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error)

// DoStreamRequest forwards a raw streaming request and returns the SSE channel.
// Same parameters as DoRequest, but returns a channel of raw SSE lines.
DoStreamRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) (<-chan []byte, error)
```

- [ ] **Step 2: Implement DoRequest and DoStreamRequest in OpenAIProvider**

Add to `internal/provider/provider.go` after the `Moderate` method (around line 606):

```go
// DoRequest forwards a raw request body to the provider endpoint
func (p *OpenAIProvider) DoRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error) {
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

	if err := p.handleHTTPResponse(resp, true); err != nil {
		return nil, err
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return respBody, nil
}

// DoStreamRequest forwards a raw streaming request and returns SSE channel
func (p *OpenAIProvider) DoStreamRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) (<-chan []byte, error) {
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

	if err := p.handleHTTPResponse(resp, false); err != nil {
		return nil, err
	}

	// Return raw SSE channel
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
			select {
			case ch <- []byte(line):
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}
```

- [ ] **Step 3: Run existing tests to verify no regression**

Run: `make test`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add internal/provider/provider.go
git commit -m "feat: add raw forwarding methods to Provider interface

Add DoRequest and DoStreamRequest methods that forward request bodies
as-is to provider endpoints. These will enable transparent passthrough
proxying without format conversion.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Chunk 2: Add Helper to Replace Model in Request Body

### Task 2: Create Model Replacement Utility

**Files:**
- Modify: `internal/server/handlers_helpers.go`

- [ ] **Step 1: Add replaceModelInBody function**

Add after `extractModelFromRequest` function (around line 146):

```go
// replaceModelInBody replaces the model field in a JSON request body.
// This is used to resolve model aliases to actual provider model names.
func replaceModelInBody(body []byte, newModel string) []byte {
	if len(body) == 0 || newModel == "" {
		return body
	}

	// Parse as generic map to preserve all fields
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}

	// Replace model field
	req["model"] = newModel

	// Re-encode
	result, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return result
}

// extractModelFromRequestBody extracts model from raw JSON body
func extractModelFromRequestBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err == nil {
		return req.Model
	}
	return ""
}
```

- [ ] **Step 2: Run tests**

Run: `make test`
Expected: All tests pass

- [ ] **Step 3: Commit**

```bash
git add internal/server/handlers_helpers.go
git commit -m "feat: add model replacement utility for passthrough proxy

replaceModelInBody replaces the model field in JSON request bodies,
enabling model alias resolution without full request parsing.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Chunk 3: Simplify Claude Handler

### Task 3: Rewrite Claude Handler as Passthrough

**Files:**
- Modify: `internal/server/claude_handlers.go`
- Test: `internal/server/integration_test.go`

- [ ] **Step 1: Rewrite handleV1Messages for passthrough**

Replace the entire `handleV1Messages` function with the passthrough version:

```go
// handleV1Messages handles POST /v1/messages (Claude API)
func (s *Server) handleV1Messages(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// Read request body with size limit
	const maxBodySize = 50 * 1024 * 1024
	limitRequestBody(w, r, maxBodySize)
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		handleError(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Extract model name for routing
	model := extractModelFromRequestBody(body)
	if model == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"type": "error",
			"error": map[string]string{
				"type":    "invalid_request_error",
				"message": "model is required",
			},
		})
		return
	}

	// Validate model exists in config
	if err := s.validateModel(model); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"type": "error",
			"error": map[string]string{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("model %q not found", model),
			},
		})
		return
	}

	// Check if streaming is requested
	var req struct {
		Stream bool `json:"stream"`
	}
	json.Unmarshal(body, &req)

	if req.Stream {
		s.handleV1MessagesStreamPassthrough(w, r, body, model)
		return
	}

	// Handle non-streaming request with failover
	resp, providerKey, err := s.executeWithFailover(r, model, "",
		func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			// Replace model name in body
			forwardBody := replaceModelInBody(body, providerModel)

			// Build headers for Anthropic API
			headers := map[string]string{
				"anthropic-version": r.Header.Get("anthropic-version"),
			}
			if apiKey := r.Header.Get("x-api-key"); apiKey != "" {
				headers["x-api-key"] = apiKey
			}

			return prov.DoRequest(ctx, "/v1/messages", forwardBody, headers)
		},
	)
	if err != nil {
		s.handleAllProvidersFailed(w, err)
		return
	}

	// Forward response as-is
	respBody, ok := resp.([]byte)
	if !ok {
		handleError(w, "invalid response from provider", http.StatusInternalServerError)
		return
	}

	s.state.ResetModel(providerKey)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}

// handleV1MessagesStreamPassthrough handles streaming POST /v1/messages
func (s *Server) handleV1MessagesStreamPassthrough(w http.ResponseWriter, r *http.Request, body []byte, model string) {
	err := s.streamWithFailover(w, r, model, "",
		func(ctx context.Context, prov provider.Provider, providerModel string) (<-chan []byte, error) {
			// Replace model name in body
			forwardBody := replaceModelInBody(body, providerModel)

			// Build headers for Anthropic API
			headers := map[string]string{
				"anthropic-version": r.Header.Get("anthropic-version"),
			}
			if apiKey := r.Header.Get("x-api-key"); apiKey != "" {
				headers["x-api-key"] = apiKey
			}

			return prov.DoStreamRequest(ctx, "/v1/messages", forwardBody, headers)
		},
		func(w http.ResponseWriter, r *http.Request, stream <-chan []byte, providerKey string) error {
			return writeRawStreamNoDone(w, r, stream, providerKey)
		},
	)
	if err != nil {
		s.handleAllProvidersFailed(w, err)
	}
}

// writeRawStreamNoDone writes raw SSE lines without adding [DONE] marker
// (Claude API doesn't use [DONE], it uses message_stop event)
func writeRawStreamNoDone(w http.ResponseWriter, r *http.Request, stream <-chan []byte, providerKey string) error {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}
	flusher.Flush()

	for line := range stream {
		if checkClientDisconnect(r) {
			logger.Info("Client disconnected, closing stream", "provider", providerKey)
			return fmt.Errorf("client disconnected")
		}

		// Write raw line as-is (already in SSE format from provider)
		if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
			return fmt.Errorf("failed to write stream chunk: %w", err)
		}
		flusher.Flush()
	}

	return nil
}
```

- [ ] **Step 2: Add necessary imports to claude_handlers.go**

Ensure imports include:
```go
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/macedot/openmodel/internal/provider"
)
```

- [ ] **Step 3: Run tests**

Run: `make test`
Expected: Tests may fail due to removed conversion - update tests in next step

- [ ] **Step 4: Commit**

```bash
git add internal/server/claude_handlers.go
git commit -m "feat: rewrite Claude handler as passthrough proxy

Remove format conversion from /v1/messages endpoint. Now forwards
requests directly to provider's /v1/messages endpoint with only
model name resolution.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Chunk 4: Simplify OpenAI Handler

### Task 4: Rewrite OpenAI Handler as Passthrough

**Files:**
- Modify: `internal/server/handlers.go`

- [ ] **Step 1: Rewrite handleV1ChatCompletions for passthrough**

Replace the POST handling in `handleV1ChatCompletions`:

```go
// handleV1ChatCompletions handles GET and POST /v1/chat/completions
func (s *Server) handleV1ChatCompletions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleV1ChatCompletionsList(w, r)
		return
	case http.MethodPost:
		// Read request body with size limit
		const maxBodySize = 50 * 1024 * 1024
		limitRequestBody(w, r, maxBodySize)
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
		if err != nil {
			handleError(w, "failed to read request body", http.StatusBadRequest)
			return
		}

		// Extract model name for routing
		model := extractModelFromRequestBody(body)
		if model == "" {
			handleError(w, "model is required", http.StatusBadRequest)
			return
		}

		// Validate model exists in config
		if err := s.validateModel(model); err != nil {
			handleError(w, err.Error(), http.StatusNotFound)
			return
		}

		// Check if streaming is requested
		var req struct {
			Stream bool `json:"stream"`
		}
		json.Unmarshal(body, &req)

		if req.Stream {
			// Handle streaming with failover
			err := s.streamWithFailover(w, r, model, "",
				func(ctx context.Context, prov provider.Provider, providerModel string) (<-chan []byte, error) {
					forwardBody := replaceModelInBody(body, providerModel)
					headers := extractForwardHeaders(r)
					return prov.DoStreamRequest(ctx, "/v1/chat/completions", forwardBody, headers)
				},
				func(w http.ResponseWriter, r *http.Request, stream <-chan []byte, providerKey string) error {
					return writeRawStream(w, r, stream, providerKey)
				},
			)
			if err != nil {
				s.handleAllProvidersFailed(w, err)
			}
			return
		}

		// Handle non-streaming with failover
		resp, providerKey, err := s.executeWithFailover(r, model, "",
			func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
				forwardBody := replaceModelInBody(body, providerModel)
				headers := extractForwardHeaders(r)
				return prov.DoRequest(ctx, "/v1/chat/completions", forwardBody, headers)
			},
		)
		if err != nil {
			s.handleAllProvidersFailed(w, err)
			return
		}

		respBody, ok := resp.([]byte)
		if !ok {
			handleError(w, "invalid response from provider", http.StatusInternalServerError)
			return
		}

		s.state.ResetModel(providerKey)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(respBody)
		return
	default:
		handleError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// extractForwardHeaders extracts headers that should be forwarded to provider
func extractForwardHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string)

	// Forward authorization if present
	if auth := r.Header.Get("Authorization"); auth != "" {
		headers["Authorization"] = auth
	}

	// Forward request ID for tracing
	if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
		headers["X-Request-ID"] = requestID
	}

	return headers
}
```

- [ ] **Step 2: Add io import to handlers.go if needed**

- [ ] **Step 3: Update handleV1Completions similarly**

Update the POST handling in `handleV1Completions`:

```go
// handleV1Completions handles POST /v1/completions
func (s *Server) handleV1Completions(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// Read request body with size limit
	const maxBodySize = 50 * 1024 * 1024
	limitRequestBody(w, r, maxBodySize)
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		handleError(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Extract model name for routing
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handleError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Model == "" {
		handleError(w, "model is required", http.StatusBadRequest)
		return
	}

	// Validate model exists in config
	if err := s.validateModel(req.Model); err != nil {
		handleError(w, err.Error(), http.StatusNotFound)
		return
	}

	// Forward request with failover
	resp, providerKey, err := s.executeWithFailover(r, req.Model, "",
		func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			forwardBody := replaceModelInBody(body, providerModel)
			headers := extractForwardHeaders(r)
			return prov.DoRequest(ctx, "/v1/completions", forwardBody, headers)
		},
	)
	if err != nil {
		s.handleAllProvidersFailed(w, err)
		return
	}

	respBody, ok := resp.([]byte)
	if !ok {
		handleError(w, "invalid response from provider", http.StatusInternalServerError)
		return
	}

	s.state.ResetModel(providerKey)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}
```

- [ ] **Step 4: Update handleV1Embeddings similarly**

```go
// handleV1Embeddings handles POST /v1/embeddings
func (s *Server) handleV1Embeddings(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// Read request body with size limit
	const maxBodySize = 50 * 1024 * 1024
	limitRequestBody(w, r, maxBodySize)
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		handleError(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Extract model name for routing
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handleError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Model == "" {
		handleError(w, "model is required", http.StatusBadRequest)
		return
	}

	// Validate model exists in config
	if err := s.validateModel(req.Model); err != nil {
		handleError(w, err.Error(), http.StatusNotFound)
		return
	}

	// Forward request with failover
	resp, providerKey, err := s.executeWithFailover(r, req.Model, "",
		func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			forwardBody := replaceModelInBody(body, providerModel)
			headers := extractForwardHeaders(r)
			return prov.DoRequest(ctx, "/v1/embeddings", forwardBody, headers)
		},
	)
	if err != nil {
		s.handleAllProvidersFailed(w, err)
		return
	}

	respBody, ok := resp.([]byte)
	if !ok {
		handleError(w, "invalid response from provider", http.StatusInternalServerError)
		return
	}

	s.state.ResetModel(providerKey)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}
```

- [ ] **Step 5: Update handleV1Moderations similarly**

Update in `handlers_moderations.go`:

```go
// handleV1Moderations handles POST /v1/moderations
func (s *Server) handleV1Moderations(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// Read request body with size limit
	const maxBodySize = 50 * 1024 * 1024
	limitRequestBody(w, r, maxBodySize)
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		handleError(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Forward request - moderations endpoint doesn't use model routing
	// but we still need a provider to forward to
	// Use first available model's provider
	if len(s.config.Models) == 0 {
		handleError(w, "no models configured", http.StatusServiceUnavailable)
		return
	}

	// Get first model's provider chain
	var firstModel string
	for model := range s.config.Models {
		firstModel = model
		break
	}

	// Forward request with failover
	resp, providerKey, err := s.executeWithFailover(r, firstModel, "",
		func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			headers := extractForwardHeaders(r)
			return prov.DoRequest(ctx, "/v1/moderations", body, headers)
		},
	)
	if err != nil {
		s.handleAllProvidersFailed(w, err)
		return
	}

	respBody, ok := resp.([]byte)
	if !ok {
		handleError(w, "invalid response from provider", http.StatusInternalServerError)
		return
	}

	s.state.ResetModel(providerKey)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}
```

- [ ] **Step 6: Run tests**

Run: `make test`
Expected: Some tests may fail due to interface changes

- [ ] **Step 7: Commit**

```bash
git add internal/server/handlers.go internal/server/handlers_moderations.go
git commit -m "feat: rewrite OpenAI handlers as passthrough proxy

Remove format validation and conversion from all OpenAI endpoints.
Now forwards requests directly to provider endpoints with only
model name resolution.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Chunk 5: Remove Conversion Code

### Task 5: Delete Claude Conversion Package

**Files:**
- Delete: `internal/api/claude/converter.go`
- Delete: `internal/api/claude/converter_test.go`
- Delete: `internal/api/claude/validation.go`
- Delete: `internal/api/claude/validation_test.go`
- Keep: `internal/api/claude/types.go` (still needed for type definitions if any)
- Keep: `internal/api/claude/types_test.go`

- [ ] **Step 1: Remove converter.go and its tests**

```bash
rm internal/api/claude/converter.go
rm internal/api/claude/converter_test.go
```

- [ ] **Step 2: Remove validation.go and its tests**

```bash
rm internal/api/claude/validation.go
rm internal/api/claude/validation_test.go
```

- [ ] **Step 3: Check if types.go is still needed**

Read `internal/api/claude/types.go` - if no longer referenced, remove it too.

Run: `grep -r "claude\." internal/`
Check if any references remain. If not, remove the entire package.

- [ ] **Step 4: Remove unused imports from handlers**

Update imports in:
- `internal/server/claude_handlers.go` - remove claude package import
- `internal/server/handlers.go` - remove openai validation imports if unused

- [ ] **Step 5: Run tests and fix compilation errors**

Run: `make build`
Fix any compilation errors.

Run: `make test`
Fix any failing tests.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: remove Claude conversion and validation code

Remove converter.go, validation.go and their tests. The proxy now
operates as a pure passthrough without format conversion.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Chunk 6: Update Tests

### Task 6: Update Integration Tests

**Files:**
- Modify: `internal/server/integration_test.go`
- Modify: `internal/server/handlers_test.go`
- Modify: `internal/provider/provider_test.go`

- [ ] **Step 1: Update provider tests for new interface**

Update `internal/provider/provider_test.go` to test `DoRequest` and `DoStreamRequest` methods:

```go
func TestOpenAIProvider_DoRequest(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Echo back the request
		body, _ := io.ReadAll(r.Body)
		w.Write(body)
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test", server.URL, "test-key")

	body := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`)
	resp, err := provider.DoRequest(context.Background(), "/v1/chat/completions", body, nil)
	require.NoError(t, err)

	// Verify response
	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, "gpt-4", result["model"])
}

func TestOpenAIProvider_DoStreamRequest(t *testing.T) {
	// Setup mock SSE server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"content\":\"test\"}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test", server.URL, "test-key")

	body := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	stream, err := provider.DoStreamRequest(context.Background(), "/v1/chat/completions", body, nil)
	require.NoError(t, err)

	// Read from stream
	var lines []string
	for line := range stream {
		lines = append(lines, string(line))
	}

	assert.GreaterOrEqual(t, len(lines), 1)
}
```

- [ ] **Step 2: Run tests to verify**

Run: `make test`

- [ ] **Step 3: Commit**

```bash
git add internal/provider/provider_test.go
git commit -m "test: add tests for raw forwarding methods

Add tests for DoRequest and DoStreamRequest methods to verify
passthrough functionality.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Chunk 7: Final Verification

### Task 7: Run Full Test Suite and Integration Tests

- [ ] **Step 1: Run all tests**

Run: `make test`

- [ ] **Step 2: Run linter**

Run: `make lint` (if available) or `go vet ./...`

- [ ] **Step 3: Build the binary**

Run: `make build`

- [ ] **Step 4: Manual integration test**

Start the server with a test config and verify:
1. `/v1/chat/completions` forwards to provider correctly
2. `/v1/messages` forwards to provider correctly
3. Model resolution works
4. Failover works

- [ ] **Step 5: Final commit and summary**

```bash
git add -A
git commit -m "feat: complete passthrough proxy implementation

OpenModel now operates as a pure transparent proxy:
- Requests are forwarded as-is to provider endpoints
- Only model name is resolved and replaced
- No format conversion between OpenAI and Claude APIs
- Responses returned directly from provider

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Files Changed Summary

| File | Action | Description |
|------|--------|-------------|
| `internal/provider/provider.go` | Modify | Add `DoRequest`, `DoStreamRequest` methods |
| `internal/server/handlers.go` | Modify | Rewrite as passthrough |
| `internal/server/handlers_moderations.go` | Modify | Rewrite as passthrough |
| `internal/server/claude_handlers.go` | Modify | Rewrite as passthrough |
| `internal/server/handlers_helpers.go` | Modify | Add `replaceModelInBody`, `extractModelFromRequestBody` |
| `internal/api/claude/converter.go` | Delete | No longer needed |
| `internal/api/claude/converter_test.go` | Delete | No longer needed |
| `internal/api/claude/validation.go` | Delete | No longer needed |
| `internal/api/claude/validation_test.go` | Delete | No longer needed |
| `internal/provider/provider_test.go` | Modify | Add tests for new methods |
| `docs/superpowers/specs/2026-03-10-passthrough-proxy-design.md` | Create | Design document |

---

## Testing Checklist

- [ ] Unit tests pass: `make test`
- [ ] Build succeeds: `make build`
- [ ] No lint errors: `make lint` or `go vet ./...`
- [ ] Manual test: OpenAI endpoint works
- [ ] Manual test: Claude endpoint works
- [ ] Manual test: Failover works
- [ ] Manual test: Model resolution works