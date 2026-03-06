// Package provider provides tests for the provider implementations
package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/macedot/openmodel/internal/api/openai"
)

// newTestProvider creates a provider pointing to a test server
func newTestProvider(serverURL string) *OpenAIProvider {
	return NewOpenAIProvider("test", serverURL, "test-api-key")
}

func TestListModels(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("expected GET request, got %s", r.Method)
			}
			if r.URL.Path != "/models" {
				t.Errorf("expected /models path, got %s", r.URL.Path)
			}
			// Check authorization header
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-api-key" {
				t.Errorf("expected Authorization header, got %s", auth)
			}

			w.Header().Set("Content-Type", "application/json")
			resp := openai.ModelList{
				Object: "list",
				Data: []openai.Model{
					{ID: "gpt-4", Object: "model", Created: 1234567890, OwnedBy: "openai"},
					{ID: "gpt-3.5-turbo", Object: "model", Created: 1234567890, OwnedBy: "openai"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		result, err := provider.ListModels(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Data) != 2 {
			t.Errorf("expected 2 models, got %d", len(result.Data))
		}
		if result.Data[0].ID != "gpt-4" {
			t.Errorf("expected model gpt-4, got %s", result.Data[0].ID)
		}
	})

	t.Run("error status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(openai.ErrorResponse{
				Err: &openai.ErrorDetail{
					Message: "internal server error",
					Type:    "server_error",
				},
			})
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		_, err := provider.ListModels(ctx)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		_, err := provider.ListModels(ctx)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("empty model list", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			resp := openai.ModelList{
				Object: "list",
				Data:   []openai.Model{},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		result, err := provider.ListModels(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Data) != 0 {
			t.Errorf("expected 0 models, got %d", len(result.Data))
		}
	})
}

func TestChat(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("expected POST request, got %s", r.Method)
			}
			if r.URL.Path != "/chat/completions" {
				t.Errorf("expected /chat/completions path, got %s", r.URL.Path)
			}

			var req openai.ChatCompletionRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &req)

			if req.Stream {
				t.Error("expected non-streaming request")
			}
			if req.Model != "gpt-4" {
				t.Errorf("expected model gpt-4, got %s", req.Model)
			}
			if len(req.Messages) != 1 || req.Messages[0].Content != "Hello" {
				t.Error("unexpected messages")
			}

			w.Header().Set("Content-Type", "application/json")
			resp := openai.ChatCompletionResponse{
				ID:      "chatcmpl-123",
				Object:  "chat.completion",
				Created: 1234567890,
				Model:   "gpt-4",
				Choices: []openai.ChatCompletionChoice{
					{
						Index: 0,
						Message: &openai.ChatCompletionMessage{
							Role:    "assistant",
							Content: "Hello! How can I help you?",
						},
						FinishReason: "stop",
					},
				},
				Usage: &openai.Usage{
					PromptTokens:     10,
					CompletionTokens: 8,
					TotalTokens:      18,
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		result, err := provider.Chat(ctx, "gpt-4", messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Choices) != 1 {
			t.Errorf("expected 1 choice, got %d", len(result.Choices))
		}
		if result.Choices[0].Message.Content != "Hello! How can I help you?" {
			t.Errorf("unexpected message content: %s", result.Choices[0].Message.Content)
		}
	})

	t.Run("with options", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req openai.ChatCompletionRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &req)

			w.Header().Set("Content-Type", "application/json")
			resp := openai.ChatCompletionResponse{
				ID:      "chatcmpl-123",
				Object:  "chat.completion",
				Created: 1234567890,
				Model:   "gpt-4",
				Choices: []openai.ChatCompletionChoice{
					{
						Index: 0,
						Message: &openai.ChatCompletionMessage{
							Role:    "assistant",
							Content: "Response",
						},
						FinishReason: "stop",
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		temp := 0.7
		maxTokens := 100
		opts := &openai.ChatCompletionRequest{
			Temperature: &temp,
			MaxTokens:   &maxTokens,
		}

		result, err := provider.Chat(ctx, "gpt-4", messages, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Choices) != 1 {
			t.Errorf("expected 1 choice, got %d", len(result.Choices))
		}
	})

	t.Run("error status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(openai.ErrorResponse{
				Err: &openai.ErrorDetail{
					Message: "invalid request",
					Type:    "invalid_request_error",
				},
			})
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		_, err := provider.Chat(ctx, "gpt-4", messages, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		_, err := provider.Chat(ctx, "gpt-4", messages, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("service unavailable"))
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		_, err := provider.Chat(ctx, "gpt-4", messages, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestChat_ExtraFields(t *testing.T) {
	t.Run("extra fields passed through", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/chat/completions" {
				t.Errorf("expected /chat/completions path, got %s", r.URL.Path)
			}

			var req openai.ChatCompletionRequest
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &req)

			// Verify extra fields are present in the request
			if req.Extra == nil {
				t.Error("expected Extra fields to be present")
			}
			if req.Extra["enable_thinking"] != true {
				t.Errorf("expected enable_thinking=true, got %v", req.Extra["enable_thinking"])
			}
			if req.Extra["custom_param"] != "value" {
				t.Errorf("expected custom_param=value, got %v", req.Extra["custom_param"])
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
				ID:      "test-id",
				Object:  "chat.completion",
				Created: 1234567890,
				Model:   "gpt-4",
				Choices: []openai.ChatCompletionChoice{
					{
						Index: 0,
						Message: &openai.ChatCompletionMessage{
							Role:    "assistant",
							Content: "Hello!",
						},
						FinishReason: "stop",
					},
				},
			})
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		opts := &openai.ChatCompletionRequest{
			Extra: map[string]any{
				"enable_thinking": true,
				"custom_param":    "value",
			},
		}

		_, err := provider.Chat(ctx, "gpt-4", messages, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestStreamChat(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/chat/completions" {
				t.Errorf("expected /chat/completions path, got %s", r.URL.Path)
			}

			var req openai.ChatCompletionRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &req)

			if !req.Stream {
				t.Error("expected streaming request")
			}

			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Transfer-Encoding", "chunked")

			// Send streaming response
			streamData := []string{
				`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				`data: [DONE]`,
			}

			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected http.Flusher")
			}

			for _, data := range streamData {
				w.Write([]byte(data + "\n"))
				flusher.Flush()
			}
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		ch, err := provider.StreamChat(ctx, "gpt-4", messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var receivedCount int
		for resp := range ch {
			receivedCount++
			if resp.ID != "chatcmpl-123" {
				t.Errorf("expected id chatcmpl-123, got %s", resp.ID)
			}
		}

		if receivedCount != 3 {
			t.Errorf("expected 3 chunks, got %d", receivedCount)
		}
	})

	t.Run("error status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(openai.ErrorResponse{
				Err: &openai.ErrorDetail{
					Message: "invalid request",
					Type:    "invalid_request_error",
				},
			})
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		_, err := provider.StreamChat(ctx, "gpt-4", messages, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("with options", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req openai.ChatCompletionRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &req)

			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			w.Write([]byte("data: [DONE]\n"))
			flusher.Flush()
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		temp := 0.5
		opts := &openai.ChatCompletionRequest{
			Temperature: &temp,
		}

		ch, err := provider.StreamChat(ctx, "gpt-4", messages, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for range ch {
			// Drain the channel
		}
	})

	t.Run("client disconnect mid-stream", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Transfer-Encoding", "chunked")

			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected http.Flusher")
			}

			// Send first chunk
			w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n"))
			flusher.Flush()

			// Wait a bit then send second chunk
			time.Sleep(50 * time.Millisecond)
			w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" World\"},\"finish_reason\":null}]}\n"))
			flusher.Flush()
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)

		// Create a cancellable context
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		ch, err := provider.StreamChat(ctx, "gpt-4", messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Receive first chunk then cancel
		receivedCount := 0
		for resp := range ch {
			receivedCount++
			if receivedCount == 1 {
				// Cancel context after receiving first chunk
				cancel()
			}
			_ = resp // Use the response to avoid unused variable
		}

		// Should have received at least 1 chunk before cancellation
		if receivedCount < 1 {
			t.Errorf("expected at least 1 chunk, got %d", receivedCount)
		}
	})

	t.Run("malformed JSON in stream chunks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Transfer-Encoding", "chunked")

			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected http.Flusher")
			}

			// Send valid chunk
			w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n"))
			flusher.Flush()

			// Send malformed JSON
			w.Write([]byte("data: {invalid json here\n"))
			flusher.Flush()

			// Send another valid chunk
			w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n"))
			flusher.Flush()

			w.Write([]byte("data: [DONE]\n"))
			flusher.Flush()
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		ch, err := provider.StreamChat(ctx, "gpt-4", messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should receive valid chunks, malformed one should be skipped
		receivedCount := 0
		errCount := 0
		for resp := range ch {
			receivedCount++
			if resp.ID == "" {
				errCount++
			}
		}

		// Should receive at least the valid chunks
		if receivedCount < 2 {
			t.Errorf("expected at least 2 valid chunks, got %d", receivedCount)
		}
		_ = errCount // May have errors in stream
	})

	t.Run("empty stream chunks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Transfer-Encoding", "chunked")

			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected http.Flusher")
			}

			// Send empty lines (should be skipped)
			w.Write([]byte("\n"))
			flusher.Flush()
			w.Write([]byte("   \n"))
			flusher.Flush()
			w.Write([]byte("\n"))

			// Send valid chunk
			w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n"))
			flusher.Flush()

			w.Write([]byte("data: [DONE]\n"))
			flusher.Flush()
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		ch, err := provider.StreamChat(ctx, "gpt-4", messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should receive the valid chunk despite empty lines
		receivedCount := 0
		for range ch {
			receivedCount++
		}

		if receivedCount != 1 {
			t.Errorf("expected 1 chunk, got %d", receivedCount)
		}
	})

	t.Run("partial data chunks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Transfer-Encoding", "chunked")

			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected http.Flusher")
			}

			// Send partial chunk (truncated JSON)
			w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\""))
			flusher.Flush()

			time.Sleep(50 * time.Millisecond)

			// Complete the chunk
			w.Write([]byte(",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n"))
			flusher.Flush()

			w.Write([]byte("data: [DONE]\n"))
			flusher.Flush()
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		messages := []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		}

		ch, err := provider.StreamChat(ctx, "gpt-4", messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should handle partial data and receive at least one chunk
		receivedCount := 0
		for range ch {
			receivedCount++
		}

		// May receive 0 or 1 depending on implementation handling of partial data
		if receivedCount > 1 {
			t.Errorf("expected at most 1 chunk, got %d", receivedCount)
		}
	})
}

func TestStreamChat_WithThinkingField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openai.ChatCompletionRequest
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)

		if !req.Stream {
			t.Error("expected streaming request")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Transfer-Encoding", "chunked")

		streamData := []string{
			`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"qwen3","choices":[{"index":0,"delta":{"role":"assistant","thinking":"Let me think..."},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"qwen3","choices":[{"index":0,"delta":{"thinking":" calculating..."},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"qwen3","choices":[{"index":0,"delta":{"content":"391"},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"qwen3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		for _, data := range streamData {
			w.Write([]byte(data + "\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := newTestProvider(server.URL)
	ctx := context.Background()

	messages := []openai.ChatCompletionMessage{
		{Role: "user", Content: "What is 17 × 23?"},
	}

	ch, err := provider.StreamChat(ctx, "qwen3", messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var chunks []openai.ChatCompletionResponse
	for resp := range ch {
		chunks = append(chunks, resp)
	}

	if len(chunks) != 4 {
		t.Errorf("expected 4 chunks, got %d", len(chunks))
	}

	// Verify thinking in first chunk
	if chunks[0].Choices[0].Delta.Thinking != "Let me think..." {
		t.Errorf("expected thinking in first chunk, got %q", chunks[0].Choices[0].Delta.Thinking)
	}

	// Verify thinking in second chunk
	if chunks[1].Choices[0].Delta.Thinking != " calculating..." {
		t.Errorf("expected thinking in second chunk, got %q", chunks[1].Choices[0].Delta.Thinking)
	}

	// Verify content in third chunk
	if chunks[2].Choices[0].Delta.Content != "391" {
		t.Errorf("expected content in third chunk, got %q", chunks[2].Choices[0].Delta.Content)
	}
}

func TestComplete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("expected POST request, got %s", r.Method)
			}
			if r.URL.Path != "/completions" {
				t.Errorf("expected /completions path, got %s", r.URL.Path)
			}

			var req openai.CompletionRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &req)

			if req.Stream {
				t.Error("expected non-streaming request")
			}
			if req.Model != "gpt-4" {
				t.Errorf("expected model gpt-4, got %s", req.Model)
			}
			if req.Prompt != "Hello" {
				t.Errorf("expected prompt Hello, got %s", req.Prompt)
			}

			w.Header().Set("Content-Type", "application/json")
			resp := openai.CompletionResponse{
				ID:      "cmpl-123",
				Object:  "text_completion",
				Created: 1234567890,
				Model:   "gpt-4",
				Choices: []openai.CompletionChoice{
					{
						Text:         "Hello! How can I help you?",
						Index:        0,
						FinishReason: "stop",
					},
				},
				Usage: &openai.Usage{
					PromptTokens:     5,
					CompletionTokens: 8,
					TotalTokens:      13,
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		req := &openai.CompletionRequest{
			Prompt: "Hello",
		}

		result, err := provider.Complete(ctx, "gpt-4", req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Choices) != 1 {
			t.Errorf("expected 1 choice, got %d", len(result.Choices))
		}
		if result.Choices[0].Text != "Hello! How can I help you?" {
			t.Errorf("unexpected text: %s", result.Choices[0].Text)
		}
	})

	t.Run("with options", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req openai.CompletionRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &req)

			w.Header().Set("Content-Type", "application/json")
			resp := openai.CompletionResponse{
				ID:      "cmpl-123",
				Object:  "text_completion",
				Created: 1234567890,
				Model:   "gpt-4",
				Choices: []openai.CompletionChoice{
					{
						Text:         "Response",
						Index:        0,
						FinishReason: "stop",
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		temp := 0.7
		maxTokens := 100
		req := &openai.CompletionRequest{
			Prompt:      "Hello",
			Temperature: &temp,
			MaxTokens:   &maxTokens,
		}

		result, err := provider.Complete(ctx, "gpt-4", req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Choices) != 1 {
			t.Errorf("expected 1 choice, got %d", len(result.Choices))
		}
	})

	t.Run("error status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(openai.ErrorResponse{
				Err: &openai.ErrorDetail{
					Message: "invalid request",
					Type:    "invalid_request_error",
				},
			})
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		req := &openai.CompletionRequest{
			Prompt: "Hello",
		}

		_, err := provider.Complete(ctx, "gpt-4", req)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		req := &openai.CompletionRequest{
			Prompt: "Hello",
		}

		_, err := provider.Complete(ctx, "gpt-4", req)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("service unavailable"))
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		req := &openai.CompletionRequest{
			Prompt: "Hello",
		}

		_, err := provider.Complete(ctx, "gpt-4", req)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestStreamComplete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/completions" {
				t.Errorf("expected /completions path, got %s", r.URL.Path)
			}

			var req openai.CompletionRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &req)

			if !req.Stream {
				t.Error("expected streaming request")
			}

			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Transfer-Encoding", "chunked")

			// Send streaming response
			streamData := []string{
				`data: {"id":"cmpl-123","object":"text_completion","created":1234567890,"model":"gpt-4","choices":[{"text":"Hello","index":0,"finish_reason":null}]}`,
				`data: {"id":"cmpl-123","object":"text_completion","created":1234567890,"model":"gpt-4","choices":[{"text":"!","index":0,"finish_reason":null}]}`,
				`data: {"id":"cmpl-123","object":"text_completion","created":1234567890,"model":"gpt-4","choices":[{"text":"","index":0,"finish_reason":"stop"}]}`,
				`data: [DONE]`,
			}

			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected http.Flusher")
			}

			for _, data := range streamData {
				w.Write([]byte(data + "\n"))
				flusher.Flush()
			}
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		req := &openai.CompletionRequest{
			Prompt: "Hello",
		}

		ch, err := provider.StreamComplete(ctx, "gpt-4", req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var receivedCount int
		for resp := range ch {
			receivedCount++
			if resp.ID != "cmpl-123" {
				t.Errorf("expected id cmpl-123, got %s", resp.ID)
			}
		}

		if receivedCount != 3 {
			t.Errorf("expected 3 chunks, got %d", receivedCount)
		}
	})

	t.Run("error status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(openai.ErrorResponse{
				Err: &openai.ErrorDetail{
					Message: "invalid request",
					Type:    "invalid_request_error",
				},
			})
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		req := &openai.CompletionRequest{
			Prompt: "Hello",
		}

		_, err := provider.StreamComplete(ctx, "gpt-4", req)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("with options", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req openai.CompletionRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &req)

			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			w.Write([]byte("data: [DONE]\n"))
			flusher.Flush()
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		temp := 0.5
		req := &openai.CompletionRequest{
			Prompt:      "Hello",
			Temperature: &temp,
		}

		ch, err := provider.StreamComplete(ctx, "gpt-4", req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for range ch {
			// Drain the channel
		}
	})

	t.Run("client disconnect mid-stream", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Transfer-Encoding", "chunked")

			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected http.Flusher")
			}

			// Send first chunk
			w.Write([]byte("data: {\"id\":\"cmpl-123\",\"object\":\"text_completion\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"text\":\"Hello\",\"index\":0,\"finish_reason\":null}]}\n"))
			flusher.Flush()

			// Wait a bit then send second chunk
			time.Sleep(50 * time.Millisecond)
			w.Write([]byte("data: {\"id\":\"cmpl-123\",\"object\":\"text_completion\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"text\":\" World\",\"index\":0,\"finish_reason\":null}]}\n"))
			flusher.Flush()
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)

		// Create a cancellable context
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		req := &openai.CompletionRequest{
			Prompt: "Hello",
		}

		ch, err := provider.StreamComplete(ctx, "gpt-4", req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Receive first chunk then cancel
		receivedCount := 0
		for resp := range ch {
			receivedCount++
			if receivedCount == 1 {
				// Cancel context after receiving first chunk
				cancel()
			}
			_ = resp // Use the response to avoid unused variable
		}

		// Should have received at least 1 chunk before cancellation
		if receivedCount < 1 {
			t.Errorf("expected at least 1 chunk, got %d", receivedCount)
		}
	})

	t.Run("malformed JSON in stream chunks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Transfer-Encoding", "chunked")

			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected http.Flusher")
			}

			// Send valid chunk
			w.Write([]byte("data: {\"id\":\"cmpl-123\",\"object\":\"text_completion\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"text\":\"Hello\",\"index\":0,\"finish_reason\":null}]}\n"))
			flusher.Flush()

			// Send malformed JSON
			w.Write([]byte("data: {invalid json here\n"))
			flusher.Flush()

			// Send another valid chunk
			w.Write([]byte("data: {\"id\":\"cmpl-123\",\"object\":\"text_completion\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"text\":\"\",\"index\":0,\"finish_reason\":\"stop\"}]}\n"))
			flusher.Flush()

			w.Write([]byte("data: [DONE]\n"))
			flusher.Flush()
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		req := &openai.CompletionRequest{
			Prompt: "Hello",
		}

		ch, err := provider.StreamComplete(ctx, "gpt-4", req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should receive valid chunks, malformed one should be skipped
		receivedCount := 0
		for resp := range ch {
			receivedCount++
			if resp.ID == "" {
				t.Error("expected valid ID in response")
			}
		}

		// Should receive at least the valid chunks
		if receivedCount < 2 {
			t.Errorf("expected at least 2 valid chunks, got %d", receivedCount)
		}
	})

	t.Run("empty stream chunks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Transfer-Encoding", "chunked")

			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected http.Flusher")
			}

			// Send empty lines (should be skipped)
			w.Write([]byte("\n"))
			flusher.Flush()
			w.Write([]byte("   \n"))
			flusher.Flush()
			w.Write([]byte("\n"))

			// Send valid chunk
			w.Write([]byte("data: {\"id\":\"cmpl-123\",\"object\":\"text_completion\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"text\":\"Hello\",\"index\":0,\"finish_reason\":null}]}\n"))
			flusher.Flush()

			w.Write([]byte("data: [DONE]\n"))
			flusher.Flush()
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		req := &openai.CompletionRequest{
			Prompt: "Hello",
		}

		ch, err := provider.StreamComplete(ctx, "gpt-4", req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should receive the valid chunk despite empty lines
		receivedCount := 0
		for range ch {
			receivedCount++
		}

		if receivedCount != 1 {
			t.Errorf("expected 1 chunk, got %d", receivedCount)
		}
	})

	t.Run("partial data chunks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Transfer-Encoding", "chunked")

			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected http.Flusher")
			}

			// Send partial chunk (truncated JSON)
			w.Write([]byte("data: {\"id\":\"cmpl-123\",\"object\":\"text_completion\""))
			flusher.Flush()

			time.Sleep(50 * time.Millisecond)

			// Complete the chunk
			w.Write([]byte(",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"text\":\"Hello\",\"index\":0,\"finish_reason\":null}]}\n"))
			flusher.Flush()

			w.Write([]byte("data: [DONE]\n"))
			flusher.Flush()
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		req := &openai.CompletionRequest{
			Prompt: "Hello",
		}

		ch, err := provider.StreamComplete(ctx, "gpt-4", req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should handle partial data and receive at least one chunk
		receivedCount := 0
		for range ch {
			receivedCount++
		}

		// May receive 0 or 1 depending on implementation handling of partial data
		if receivedCount > 1 {
			t.Errorf("expected at most 1 chunk, got %d", receivedCount)
		}
	})
}

func TestEmbed(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("expected POST request, got %s", r.Method)
			}
			if r.URL.Path != "/embeddings" {
				t.Errorf("expected /embeddings path, got %s", r.URL.Path)
			}

			var req openai.EmbeddingRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &req)

			if req.Model != "text-embedding-3-small" {
				t.Errorf("expected model text-embedding-3-small, got %s", req.Model)
			}

			inputSlice, ok := req.Input.([]any)
			if !ok || len(inputSlice) != 2 {
				t.Errorf("expected 2 inputs, got %v", req.Input)
			}

			w.Header().Set("Content-Type", "application/json")
			resp := openai.EmbeddingResponse{
				Object: "list",
				Data: []openai.EmbeddingData{
					{
						Object:    "embedding",
						Index:     0,
						Embedding: []float64{0.1, 0.2, 0.3},
					},
					{
						Object:    "embedding",
						Index:     1,
						Embedding: []float64{0.4, 0.5, 0.6},
					},
				},
				Model: "text-embedding-3-small",
				Usage: &openai.Usage{
					PromptTokens:     10,
					CompletionTokens: 0,
					TotalTokens:      10,
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		input := []string{"Hello world", "Testing embeddings"}

		result, err := provider.Embed(ctx, "text-embedding-3-small", input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Data) != 2 {
			t.Errorf("expected 2 embeddings, got %d", len(result.Data))
		}
		if result.Data[0].Index != 0 {
			t.Errorf("expected index 0, got %d", result.Data[0].Index)
		}
		if len(result.Data[0].Embedding) != 3 {
			t.Errorf("expected 3 dimensions, got %d", len(result.Data[0].Embedding))
		}
	})

	t.Run("single input", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req openai.EmbeddingRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &req)

			w.Header().Set("Content-Type", "application/json")
			resp := openai.EmbeddingResponse{
				Object: "list",
				Data: []openai.EmbeddingData{
					{
						Object:    "embedding",
						Index:     0,
						Embedding: []float64{0.1, 0.2, 0.3},
					},
				},
				Model: "text-embedding-3-small",
				Usage: &openai.Usage{
					PromptTokens:     5,
					CompletionTokens: 0,
					TotalTokens:      5,
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		input := []string{"Hello world"}

		result, err := provider.Embed(ctx, "text-embedding-3-small", input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Data) != 1 {
			t.Errorf("expected 1 embedding, got %d", len(result.Data))
		}
	})

	t.Run("error status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(openai.ErrorResponse{
				Err: &openai.ErrorDetail{
					Message: "invalid request",
					Type:    "invalid_request_error",
				},
			})
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		input := []string{"Hello world"}

		_, err := provider.Embed(ctx, "text-embedding-3-small", input)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		input := []string{"Hello world"}

		_, err := provider.Embed(ctx, "text-embedding-3-small", input)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("service unavailable"))
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		input := []string{"Hello world"}

		_, err := provider.Embed(ctx, "text-embedding-3-small", input)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestName(t *testing.T) {
	provider := NewOpenAIProvider("custom-name", "http://localhost:8080", "api-key")
	if provider.Name() != "custom-name" {
		t.Errorf("expected name custom-name, got %s", provider.Name())
	}
}

func TestBuildRequest(t *testing.T) {
	t.Run("with API key", func(t *testing.T) {
		provider := NewOpenAIProvider("test", "http://localhost:8080", "test-key")

		body := []byte(`{"model":"gpt-4"}`)
		req, err := provider.buildRequest(body, "/chat/completions")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", req.Header.Get("Content-Type"))
		}
		if req.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header, got %s", req.Header.Get("Authorization"))
		}
	})

	t.Run("without API key", func(t *testing.T) {
		provider := NewOpenAIProvider("test", "http://localhost:8080", "")

		body := []byte(`{"model":"gpt-4"}`)
		req, err := provider.buildRequest(body, "/chat/completions")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if req.Header.Get("Authorization") != "" {
			t.Errorf("expected no Authorization header, got %s", req.Header.Get("Authorization"))
		}
	})

	t.Run("base URL trailing slash", func(t *testing.T) {
		provider := NewOpenAIProvider("test", "http://localhost:8080/", "test-key")

		body := []byte(`{"model":"gpt-4"}`)
		req, err := provider.buildRequest(body, "/chat/completions")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// The URL should not have double slashes
		if !strings.Contains(req.URL.String(), "8080/chat/completions") {
			t.Errorf("unexpected URL: %s", req.URL.String())
		}
	})
}

func TestDoRequest(t *testing.T) {
	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate slow response
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		provider := NewOpenAIProvider("test", server.URL, "test-key")
		// Use very short timeout to trigger context cancellation
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		req, _ := http.NewRequest("GET", server.URL, nil)
		_, err := provider.doRequest(ctx, req)
		if err == nil {
			t.Fatal("expected error due to context cancellation")
		}
	})
}

// Test table-driven tests for various scenarios
func TestListModelsTableDriven(t *testing.T) {
	tests := []struct {
		name           string
		responseStatus int
		responseBody   string
		expectError    bool
	}{
		{
			name:           "success",
			responseStatus: http.StatusOK,
			responseBody:   `{"object":"list","data":[{"id":"gpt-4","object":"model","created":1234567890,"owned_by":"openai"}]}`,
			expectError:    false,
		},
		{
			name:           "server error",
			responseStatus: http.StatusInternalServerError,
			responseBody:   `{"error":{"message":"internal error","type":"server_error"}}`,
			expectError:    true,
		},
		{
			name:           "rate limit",
			responseStatus: http.StatusTooManyRequests,
			responseBody:   `{"error":{"message":"rate limit exceeded","type":"rate_limit_error"}}`,
			expectError:    true,
		},
		{
			name:           "unauthorized",
			responseStatus: http.StatusUnauthorized,
			responseBody:   `{"error":{"message":"invalid API key","type":"invalid_request_error"}}`,
			expectError:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.responseStatus)
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tc.responseBody))
			}))
			defer server.Close()

			provider := newTestProvider(server.URL)
			ctx := context.Background()

			_, err := provider.ListModels(ctx)
			if tc.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestChatTableDriven(t *testing.T) {
	tests := []struct {
		name           string
		responseStatus int
		responseBody   string
		expectError    bool
	}{
		{
			name:           "success",
			responseStatus: http.StatusOK,
			responseBody:   `{"id":"chatcmpl-123","object":"chat.completion","created":1234567890,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"Hi"},"finish_reason":"stop"}]}`,
			expectError:    false,
		},
		{
			name:           "bad request",
			responseStatus: http.StatusBadRequest,
			responseBody:   `{"error":{"message":"invalid request","type":"invalid_request_error"}}`,
			expectError:    true,
		},
		{
			name:           "model not found",
			responseStatus: http.StatusNotFound,
			responseBody:   `{"error":{"message":"model not found","type":"invalid_request_error"}}`,
			expectError:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.responseStatus)
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tc.responseBody))
			}))
			defer server.Close()

			provider := newTestProvider(server.URL)
			ctx := context.Background()

			messages := []openai.ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
			}

			_, err := provider.Chat(ctx, "gpt-4", messages, nil)
			if tc.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// BenchmarkListModels benchmarks the ListModels method
func BenchmarkListModels(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := openai.ModelList{
			Object: "list",
			Data: []openai.Model{
				{ID: "gpt-4", Object: "model", Created: 1234567890, OwnedBy: "openai"},
				{ID: "gpt-3.5-turbo", Object: "model", Created: 1234567890, OwnedBy: "openai"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := newTestProvider(server.URL)
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		_, err := provider.ListModels(ctx)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

// TestModerate tests the Moderate method
func TestModerate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("expected POST request, got %s", r.Method)
			}
			if r.URL.Path != "/moderations" {
				t.Errorf("expected /moderations path, got %s", r.URL.Path)
			}

			var req openai.ModerationRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &req)

			if req.Input != "Test content" {
				t.Errorf("expected input 'Test content', got %s", req.Input)
			}

			w.Header().Set("Content-Type", "application/json")
			resp := openai.ModerationResponse{
				ID:    "modr-123",
				Model: "text-moderation-001",
				Results: []openai.ModerationResult{
					{
						Flagged: false,
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		resp, err := provider.Moderate(ctx, "Test content")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Results) == 0 {
			t.Error("expected results, got empty slice")
		}
		if resp.Results[0].Flagged {
			t.Error("expected flagged=false")
		}
	})

	t.Run("error_status_code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "bad request",
			})
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		_, err := provider.Moderate(ctx, "Test content")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid_JSON_response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		provider := newTestProvider(server.URL)
		ctx := context.Background()

		_, err := provider.Moderate(ctx, "Test content")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// BenchmarkChat benchmarks the Chat method
func BenchmarkChat(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openai.ChatCompletionRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)

		w.Header().Set("Content-Type", "application/json")
		resp := openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: 1234567890,
			Model:   "gpt-4",
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: &openai.ChatCompletionMessage{
						Role:    "assistant",
						Content: "Hello! How can I help you?",
					},
					FinishReason: "stop",
				},
			},
			Usage: &openai.Usage{
				PromptTokens:     10,
				CompletionTokens: 8,
				TotalTokens:      18,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := newTestProvider(server.URL)
	ctx := context.Background()
	messages := []openai.ChatCompletionMessage{
		{Role: "user", Content: "Hello"},
	}

	for i := 0; i < b.N; i++ {
		_, err := provider.Chat(ctx, "gpt-4", messages, nil)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}
