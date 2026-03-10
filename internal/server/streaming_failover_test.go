package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/state"
)

// mockStreamProvider is a provider that can be configured to fail or succeed on streaming
type mockStreamProvider struct {
	nameVal               string
	streamErr             error        // Error to return when starting stream
	streamData            []string     // Data to stream (for raw streaming)
	midStreamErr          error        // Error to send mid-stream
	streamCloseAfter      int          // Close stream after N messages (for mid-stream errors)
	streamCallCount       int          // Track how many times StreamChat was called
	streamChatResult      []openai.ChatCompletionResponse
	streamCompleteResult  []openai.CompletionResponse
}

func (m *mockStreamProvider) Name() string { return m.nameVal }

func (m *mockStreamProvider) ListModels(ctx context.Context) (*openai.ModelList, error) {
	return nil, nil
}

func (m *mockStreamProvider) Chat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockStreamProvider) StreamChat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan openai.ChatCompletionResponse, error) {
	m.streamCallCount++
	if m.streamErr != nil {
		return nil, m.streamErr
	}

	ch := make(chan openai.ChatCompletionResponse, len(m.streamChatResult))
	for i, r := range m.streamChatResult {
		if m.midStreamErr != nil && i == m.streamCloseAfter {
			// Don't send the rest - simulate mid-stream failure
			close(ch)
			return ch, nil
		}
		ch <- r
	}
	close(ch)
	return ch, nil
}

func (m *mockStreamProvider) StreamChatRaw(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan []byte, error) {
	m.streamCallCount++
	if m.streamErr != nil {
		return nil, m.streamErr
	}

	ch := make(chan []byte, len(m.streamData))
	for _, data := range m.streamData {
		ch <- []byte(data)
	}
	close(ch)
	return ch, nil
}

func (m *mockStreamProvider) Complete(ctx context.Context, model string, req *openai.CompletionRequest) (*openai.CompletionResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockStreamProvider) StreamComplete(ctx context.Context, model string, req *openai.CompletionRequest) (<-chan openai.CompletionResponse, error) {
	m.streamCallCount++
	if m.streamErr != nil {
		return nil, m.streamErr
	}

	ch := make(chan openai.CompletionResponse, len(m.streamCompleteResult))
	for _, r := range m.streamCompleteResult {
		ch <- r
	}
	close(ch)
	return ch, nil
}

func (m *mockStreamProvider) Embed(ctx context.Context, model string, input []string) (*openai.EmbeddingResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockStreamProvider) Moderate(ctx context.Context, input string) (*openai.ModerationResponse, error) {
	return nil, errors.New("not implemented")
}

// TestStreamFailover tests the streaming failover logic for chat completions
func TestStreamFailover(t *testing.T) {
	t.Run("successful stream on first provider", func(t *testing.T) {
		mock := &mockStreamProvider{
			nameVal: "mock",
			streamData: []string{
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
				`data: {"choices":[{"delta":{"content":"!"},"finish_reason":"stop"}]}`,
			},
		}

		cfg := config.DefaultConfig()
		cfg.Models = map[string]config.ModelConfig{
			"test-model": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "mock", Model: "test-model"}}},
		}
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)
		providers := map[string]provider.Provider{"mock": mock}
		srv := New(cfg, providers, stateMgr)

		body := `{"model":"test-model","messages":[{"role":"user","content":"hi"}],"stream":true}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		srv.handleV1ChatCompletions(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		if rec.Header().Get("Content-Type") != "text/event-stream" {
			t.Errorf("expected Content-Type 'text/event-stream', got %q", rec.Header().Get("Content-Type"))
		}

		bodyStr := rec.Body.String()
		if !strings.Contains(bodyStr, "data:") {
			t.Error("expected SSE data in response")
		}
		// Check for content in stream (raw data is passed through)
		if !strings.Contains(bodyStr, "Hello") {
			t.Errorf("expected 'Hello' in stream content, got: %s", bodyStr)
		}

		// Provider should only be called once
		if mock.streamCallCount != 1 {
			t.Errorf("expected 1 stream call, got %d", mock.streamCallCount)
		}
	})

	t.Run("failover on connection error", func(t *testing.T) {
		firstProvider := &mockStreamProvider{
			nameVal:   "first",
			streamErr: errors.New("connection refused"),
		}
		secondProvider := &mockStreamProvider{
			nameVal: "second",
			streamData: []string{
				`data: {"choices":[{"delta":{"content":"Fallback response"}}]}`,
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			},
		}

		cfg := config.DefaultConfig()
		cfg.Thresholds.FailuresBeforeSwitch = 1 // Failover immediately after 1 failure
		cfg.Models = map[string]config.ModelConfig{
			"failover-model": {
				Strategy: "fallback",
				Providers: []config.ModelProvider{
					{Provider: "first", Model: "first-model"},
					{Provider: "second", Model: "fallback-model"},
				},
			},
		}
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)
		providers := map[string]provider.Provider{
			"first":  firstProvider,
			"second": secondProvider,
		}
		srv := New(cfg, providers, stateMgr)

		body := `{"model":"failover-model","messages":[{"role":"user","content":"test"}],"stream":true}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		srv.handleV1ChatCompletions(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d, body: %s", rec.Code, rec.Body.String())
		}

		bodyStr := rec.Body.String()
		if !strings.Contains(bodyStr, "Fallback response") {
			t.Error("expected fallback response in stream")
		}

		// First provider should have been called and failed
		if firstProvider.streamCallCount != 1 {
			t.Errorf("expected first provider to be called once, got %d", firstProvider.streamCallCount)
		}

		// Second provider should have been called and succeeded
		if secondProvider.streamCallCount != 1 {
			t.Errorf("expected second provider to be called once, got %d", secondProvider.streamCallCount)
		}

		// First provider should be marked as failed
		providerKey := "first/first-model"
		if stateMgr.IsAvailable(providerKey, cfg.Thresholds.FailuresBeforeSwitch) {
			t.Error("expected first provider to be marked as failed")
		}
	})

	t.Run("all providers fail returns 503", func(t *testing.T) {
		firstProvider := &mockStreamProvider{
			nameVal:   "first",
			streamErr: errors.New("connection refused"),
		}
		secondProvider := &mockStreamProvider{
			nameVal:   "second",
			streamErr: errors.New("timeout"),
		}

		cfg := config.DefaultConfig()
		cfg.Thresholds.FailuresBeforeSwitch = 1 // Failover immediately after 1 failure
		cfg.Models = map[string]config.ModelConfig{
			"failover-model": {
				Strategy: "fallback",
				Providers: []config.ModelProvider{
					{Provider: "first", Model: "first-model"},
					{Provider: "second", Model: "second-model"},
				},
			},
		}
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)
		providers := map[string]provider.Provider{
			"first":  firstProvider,
			"second": secondProvider,
		}
		srv := New(cfg, providers, stateMgr)

		body := `{"model":"failover-model","messages":[{"role":"user","content":"test"}],"stream":true}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		srv.handleV1ChatCompletions(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status 503, got %d", rec.Code)
		}
	})
}

// TestStreamWithFailoverHelper tests the streamWithFailover helper function directly
func TestStreamWithFailoverHelper(t *testing.T) {
	t.Run("processChunk succeeds on first try", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Models = map[string]config.ModelConfig{
			"test-model": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "mock", Model: "test-model"}}},
		}
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)

		mock := &mockStreamProvider{
			nameVal:    "mock",
			streamData: []string{
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
				`data: [DONE]`,
			},
		}
		providers := map[string]provider.Provider{"mock": mock}
		srv := New(cfg, providers, stateMgr)

		var streamCalls int
		streamFn := func(ctx context.Context, prov provider.Provider, providerModel string) (<-chan []byte, error) {
			streamCalls++
			return prov.StreamChatRaw(ctx, providerModel, nil, nil)
		}

		processChunk := func(w http.ResponseWriter, r *http.Request, stream <-chan []byte, providerKey string) error {
			// Read all chunks to simulate processing
			for range stream {
			}
			return nil
		}

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test-model","stream":true}`))
		rec := httptest.NewRecorder()

		err := srv.streamWithFailover(rec, req, "test-model", "", streamFn, processChunk)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if streamCalls != 1 {
			t.Errorf("expected 1 stream call, got %d", streamCalls)
		}
	})

	t.Run("failover on stream error", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Models = map[string]config.ModelConfig{
			"test-model": {
				Strategy: "fallback",
				Providers: []config.ModelProvider{
					{Provider: "first", Model: "first-model"},
					{Provider: "second", Model: "second-model"},
				},
			},
		}
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)

		callCount := 0
		streamFn := func(ctx context.Context, prov provider.Provider, providerModel string) (<-chan []byte, error) {
			callCount++
			if callCount == 1 {
				// First call fails
				return nil, errors.New("connection failed")
			}
			// Second call succeeds
			ch := make(chan []byte, 1)
			ch <- []byte("data: success")
			close(ch)
			return ch, nil
		}

		processChunk := func(w http.ResponseWriter, r *http.Request, stream <-chan []byte, providerKey string) error {
			for range stream {
			}
			return nil
		}

		firstProvider := &mockStreamProvider{nameVal: "first"}
		secondProvider := &mockStreamProvider{nameVal: "second"}
		providers := map[string]provider.Provider{
			"first":  firstProvider,
			"second": secondProvider,
		}
		srv := New(cfg, providers, stateMgr)

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		rec := httptest.NewRecorder()

		err := srv.streamWithFailover(rec, req, "test-model", "", streamFn, processChunk)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if callCount != 2 {
			t.Errorf("expected 2 stream calls (failover), got %d", callCount)
		}
	})
}

// TestStreamWithFailoverTyped tests the typed stream failover function
func TestStreamWithFailoverTyped(t *testing.T) {
	t.Run("typed stream failover", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Models = map[string]config.ModelConfig{
			"test-model": {
				Strategy: "fallback",
				Providers: []config.ModelProvider{
					{Provider: "first", Model: "first-model"},
					{Provider: "second", Model: "second-model"},
				},
			},
		}
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)

		callCount := 0
		streamFn := func(ctx context.Context, prov provider.Provider, providerModel string) (<-chan openai.ChatCompletionResponse, error) {
			callCount++
			if callCount == 1 {
				// First call fails
				return nil, errors.New("connection failed")
			}
			// Second call succeeds
			ch := make(chan openai.ChatCompletionResponse, 1)
			ch <- openai.ChatCompletionResponse{
				ID:      "test",
				Model:   "test-model",
				Choices: []openai.ChatCompletionChoice{{Delta: &openai.ChatCompletionDelta{Content: "success"}}},
			}
			close(ch)
			return ch, nil
		}

		processChunk := func(w http.ResponseWriter, r *http.Request, stream <-chan openai.ChatCompletionResponse, providerKey string) error {
			for range stream {
			}
			return nil
		}

		firstProvider := &mockStreamProvider{nameVal: "first"}
		secondProvider := &mockStreamProvider{nameVal: "second"}
		providers := map[string]provider.Provider{
			"first":  firstProvider,
			"second": secondProvider,
		}
		srv := New(cfg, providers, stateMgr)

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		rec := httptest.NewRecorder()

		err := streamWithFailoverTyped(srv, rec, req, "test-model", "", streamFn, processChunk)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if callCount != 2 {
			t.Errorf("expected 2 stream calls (failover), got %d", callCount)
		}
	})
}

// TestDrainStream tests that drainStream properly drains channels
func TestDrainStream(t *testing.T) {
	t.Run("drains byte stream", func(t *testing.T) {
		ch := make(chan []byte, 5)
		ch <- []byte("msg1")
		ch <- []byte("msg2")
		ch <- []byte("msg3")
		close(ch)

		// Should not block
		drainStream(ch)
	})

	t.Run("drains typed stream", func(t *testing.T) {
		ch := make(chan openai.ChatCompletionResponse, 5)
		ch <- openai.ChatCompletionResponse{ID: "1"}
		ch <- openai.ChatCompletionResponse{ID: "2"}
		ch <- openai.ChatCompletionResponse{ID: "3"}
		close(ch)

		// Should not block
		drainStreamTyped(ch)
	})

	t.Run("handles empty stream", func(t *testing.T) {
		ch := make(chan []byte)
		close(ch)

		drainStream(ch)
	})
}

// TestCheckClientDisconnect tests client disconnect detection
func TestCheckClientDisconnect(t *testing.T) {
	t.Run("no disconnect", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		if checkClientDisconnect(req) {
			t.Error("expected no disconnect for fresh request")
		}
	})

	t.Run("with canceled context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		ctx, cancel := context.WithCancel(req.Context())
		cancel() // Cancel immediately
		req = req.WithContext(ctx)

		if !checkClientDisconnect(req) {
			t.Error("expected disconnect for canceled context")
		}
	})
}

// TestWriteRawStream tests raw SSE stream writing
func TestWriteRawStream(t *testing.T) {
	t.Run("writes stream data", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)

		ch := make(chan []byte, 2)
		ch <- []byte(`data: {"test": "message"}`)
		ch <- []byte(`data: [DONE]`)
		close(ch)

		err := writeRawStream(rec, req, ch, "test-provider")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "data: {\"test\": \"message\"}") {
			t.Errorf("expected stream data in body, got: %s", body)
		}
		if !strings.Contains(body, "data: [DONE]") {
			t.Error("expected [DONE] marker in body")
		}
	})

	t.Run("handles client disconnect", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		ctx, cancel := context.WithCancel(req.Context())
		cancel()
		req = req.WithContext(ctx)

		// Empty channel - should still complete without error since there's nothing to iterate
		ch := make(chan []byte)
		close(ch)

		err := writeRawStream(rec, req, ch, "test-provider")
		// Empty channel should complete without error (client disconnect only matters during iteration)
		if err != nil {
			t.Errorf("unexpected error for empty stream: %v", err)
		}
	})
}

// TestWriteCompletionStream tests completion stream writing
func TestWriteCompletionStream(t *testing.T) {
	t.Run("writes completion stream", func(t *testing.T) {
		cfg := config.DefaultConfig()
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)
		srv := New(cfg, nil, stateMgr)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)

		ch := make(chan openai.CompletionResponse, 2)
		ch <- openai.CompletionResponse{
			ID:      "test-id",
			Model:   "test-model",
			Choices: []openai.CompletionChoice{{Text: "Hello"}},
		}
		ch <- openai.CompletionResponse{
			ID:      "test-id",
			Model:   "test-model",
			Choices: []openai.CompletionChoice{{Text: " world", FinishReason: "stop"}},
		}
		close(ch)

		err := srv.writeCompletionStream(rec, req, ch, "test-provider", "test-model")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "data:") {
			t.Error("expected SSE data prefix")
		}
		if !strings.Contains(body, "Hello") {
			t.Error("expected content in stream")
		}
	})
}

// TestExecuteWithFailover tests non-streaming failover
func TestExecuteWithFailover(t *testing.T) {
	t.Run("success on first provider", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Models = map[string]config.ModelConfig{
			"test-model": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "mock", Model: "test-model"}}},
		}
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)

		mock := &mockProvider{
			nameVal: "mock",
			chatResult: &openai.ChatCompletionResponse{
				ID:     "test",
				Model:  "test-model",
				Object: "chat.completion",
				Choices: []openai.ChatCompletionChoice{
					{Message: &openai.ChatCompletionMessage{Content: "Hello"}},
				},
			},
		}
		providers := map[string]provider.Provider{"mock": mock}
		srv := New(cfg, providers, stateMgr)

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		resp, providerKey, err := srv.executeWithFailover(req, "test-model", "", func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			return prov.Chat(ctx, providerModel, nil, nil)
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if providerKey != "mock/test-model" {
			t.Errorf("expected provider key 'mock/test-model', got %q", providerKey)
		}
		if resp == nil {
			t.Error("expected response")
		}
	})

	t.Run("failover on failure", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Thresholds.FailuresBeforeSwitch = 1 // Failover immediately after 1 failure
		cfg.Models = map[string]config.ModelConfig{
			"test-model": {
				Strategy: "fallback",
				Providers: []config.ModelProvider{
					{Provider: "first", Model: "first-model"},
					{Provider: "second", Model: "second-model"},
				},
			},
		}
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)

		firstProvider := &mockProvider{
			nameVal: "first",
			chatErr: errors.New("connection failed"),
		}
		secondProvider := &mockProvider{
			nameVal: "second",
			chatResult: &openai.ChatCompletionResponse{
				ID:      "test",
				Model:   "second-model",
				Object:  "chat.completion",
				Choices: []openai.ChatCompletionChoice{{Message: &openai.ChatCompletionMessage{Content: "Success"}}},
			},
		}
		providers := map[string]provider.Provider{
			"first":  firstProvider,
			"second": secondProvider,
		}
		srv := New(cfg, providers, stateMgr)

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		resp, providerKey, err := srv.executeWithFailover(req, "test-model", "", func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			return prov.Chat(ctx, providerModel, nil, nil)
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if providerKey != "second/second-model" {
			t.Errorf("expected provider key 'second/second-model', got %q", providerKey)
		}
		if resp == nil {
			t.Error("expected response")
		}
	})

	t.Run("all providers fail", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Thresholds.FailuresBeforeSwitch = 1 // Failover immediately after 1 failure
		cfg.Models = map[string]config.ModelConfig{
			"test-model": {
				Strategy: "fallback",
				Providers: []config.ModelProvider{
					{Provider: "first", Model: "first-model"},
					{Provider: "second", Model: "second-model"},
				},
			},
		}
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)

		firstProvider := &mockProvider{
			nameVal: "first",
			chatErr: errors.New("connection failed"),
		}
		secondProvider := &mockProvider{
			nameVal: "second",
			chatErr: errors.New("timeout"),
		}
		providers := map[string]provider.Provider{
			"first":  firstProvider,
			"second": secondProvider,
		}
		srv := New(cfg, providers, stateMgr)

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		resp, providerKey, err := srv.executeWithFailover(req, "test-model", "", func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			return prov.Chat(ctx, providerModel, nil, nil)
		})

		if err == nil {
			t.Error("expected error when all providers fail")
		}
		if resp != nil {
			t.Error("expected nil response")
		}
		if providerKey != "" {
			t.Errorf("expected empty provider key, got %q", providerKey)
		}
	})
}

// TestReadRequestBody tests request body reading with size limit
func TestReadRequestBody(t *testing.T) {
	t.Run("reads body correctly", func(t *testing.T) {
		body := `{"test": "data"}`
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))

		data, err := readRequestBody(req, 1024)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if string(data) != body {
			t.Errorf("expected body %q, got %q", body, string(data))
		}
	})

	t.Run("handles empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)

		data, err := readRequestBody(req, 1024)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if data != nil {
			t.Errorf("expected nil data for empty body, got %q", string(data))
		}
	})

	t.Run("restores body for re-reading", func(t *testing.T) {
		body := `{"test": "data"}`
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))

		_, err := readRequestBody(req, 1024)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Body should be restored and readable again
		data, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("unexpected error re-reading body: %v", err)
		}
		if string(data) != body {
			t.Errorf("expected body to be restored, got %q", string(data))
		}
	})
}

// TestPrettyPrintJSON tests JSON formatting
func TestPrettyPrintJSON(t *testing.T) {
	t.Run("formats valid JSON", func(t *testing.T) {
		data := []byte(`{"key":"value"}`)
		result := prettyPrintJSON(data)
		if !strings.Contains(result, "key") || !strings.Contains(result, "value") {
			t.Errorf("expected formatted JSON, got %q", result)
		}
	})

	t.Run("handles empty data", func(t *testing.T) {
		result := prettyPrintJSON([]byte{})
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("truncates invalid JSON", func(t *testing.T) {
		largeData := []byte(strings.Repeat("x", 2000))
		result := prettyPrintJSON(largeData)
		if len(result) > 1100 {
			t.Errorf("expected truncated result, got length %d", len(result))
		}
	})
}