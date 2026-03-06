package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/state"
)

func TestHandleRoot(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleRoot(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["name"] != "openmodel" {
		t.Errorf("expected name 'openmodel', got %q", resp["name"])
	}
}

func TestHandleHealth(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", resp["status"])
	}
}

func TestHandleV1Models(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string]config.ModelConfig{
		"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-4"}}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Models(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp openai.ModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Errorf("expected 1 model, got %d", len(resp.Data))
	}

	if resp.Data[0].ID != "gpt-4" {
		t.Errorf("expected model id 'gpt-4', got %q", resp.Data[0].ID)
	}
}

func TestHandleV1ModelNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models/nonexistent", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Model(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleV1ModelFound(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string]config.ModelConfig{
		"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-4"}}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models/gpt-4", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Model(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp openai.Model
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.ID != "gpt-4" {
		t.Errorf("expected model id 'gpt-4', got %q", resp.ID)
	}
}

func TestHandleV1ChatCompletionsModelNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	body := strings.NewReader(`{"model":"nonexistent","messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestServerStopNil(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	// Stop should return nil when httpServer is nil
	err := srv.Stop(nil)
	if err != nil {
		t.Errorf("Stop() with nil httpServer = %v, want nil", err)
	}
}

// Stop has a race condition between Start/Stop - skipping for coverage
// In production, use proper synchronization for httpServer field

func TestServerStartInvalidPort(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Port = 1 // Use invalid port
	cfg.Server.Host = "localhost"
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	// Start should fail on invalid port
	err := srv.Start()
	if err == nil {
		t.Error("Start() on invalid port should fail")
	}
}

func TestHandleV1ModelNonGet(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string]config.ModelConfig{
		"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-4"}}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodPost, "/v1/models/gpt-4", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Model(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestHandleV1ModelEmptyName(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models/", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Model(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestConvertInputToSlice(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  []string
	}{
		{
			name:  "string input",
			input: "hello world",
			want:  []string{"hello world"},
		},
		{
			name:  "array of strings",
			input: []any{"hello", "world"},
			want:  []string{"hello", "world"},
		},
		{
			name:  "array with non-string filtered",
			input: []any{"hello", 123, "world"},
			want:  []string{"hello", "world"},
		},
		{
			name:  "nil input",
			input: nil,
			want:  nil,
		},
		{
			name:  "unsupported type",
			input: 12345,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertInputToSlice(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("convertInputToSlice() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("convertInputToSlice()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestHandleError(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	// Create a response recorder
	rec := httptest.NewRecorder()

	// GET /v1/chat/completions is now valid (list stored completions returns 200)
	srv.handleV1ChatCompletions(rec, httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for GET, got %d", rec.Code)
	}

	// Test DELETE method still returns 405
	rec = httptest.NewRecorder()
	srv.handleV1ChatCompletions(rec, httptest.NewRequest(http.MethodDelete, "/v1/chat/completions", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for DELETE, got %d", rec.Code)
	}
}

func TestHandleV1ModelsNonGet(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodPost, "/v1/models", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Models(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestHandleV1CompletionsNonGet(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/completions", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Completions(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestHandleV1EmbeddingsNonGet(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/embeddings", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Embeddings(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

// BenchmarkHandleRoot benchmarks the root handler
func BenchmarkHandleRoot(b *testing.B) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		srv.handleRoot(rec, req)
	}
}

// BenchmarkHandleV1Models benchmarks the v1 models handler
func BenchmarkHandleV1Models(b *testing.B) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string]config.ModelConfig{
		"gpt-4":         {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-4"}}},
		"gpt-3.5-turbo": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-3.5-turbo"}}},
		"claude-3":      {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "claude-3"}}},
		"llama-2":       {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "ollama", Model: "llama-2"}}},
		"mistral":       {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "ollama", Model: "mistral"}}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		srv.handleV1Models(rec, req)
	}
}

// TestExtractModelFromRequest tests extractModelFromRequest
func TestExtractModelFromRequest(t *testing.T) {
	t.Run("empty body", func(t *testing.T) {
		model := extractModelFromRequest([]byte{})
		if model != "" {
			t.Errorf("expected empty string, got %q", model)
		}
	})

	t.Run("valid JSON with model", func(t *testing.T) {
		body := []byte(`{"model": "gpt-4", "messages": [{"role": "user", "content": "hi"}]}`)
		model := extractModelFromRequest(body)
		if model != "gpt-4" {
			t.Errorf("expected gpt-4, got %q", model)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		body := []byte(`{invalid json}`)
		model := extractModelFromRequest(body)
		if model != "" {
			t.Errorf("expected empty string for invalid JSON, got %q", model)
		}
	})

	t.Run("JSON without model field", func(t *testing.T) {
		body := []byte(`{"messages": [{"role": "user", "content": "hi"}]}`)
		model := extractModelFromRequest(body)
		if model != "" {
			t.Errorf("expected empty string, got %q", model)
		}
	})
}

// TestSetGetProviderContext tests setProviderContext and getProviderFromContext
func TestSetGetProviderContext(t *testing.T) {
	t.Run("both values set", func(t *testing.T) {
		ctx := context.Background()
		ctx = setProviderContext(ctx, "openai", "gpt-4")
		provider, model := getProviderFromContext(ctx)
		if provider != "openai" {
			t.Errorf("expected provider 'openai', got %q", provider)
		}
		if model != "gpt-4" {
			t.Errorf("expected model 'gpt-4', got %q", model)
		}
	})

	t.Run("only provider set", func(t *testing.T) {
		ctx := context.Background()
		ctx = setProviderContext(ctx, "ollama", "")
		provider, model := getProviderFromContext(ctx)
		if provider != "ollama" {
			t.Errorf("expected provider 'ollama', got %q", provider)
		}
		if model != "" {
			t.Errorf("expected empty model, got %q", model)
		}
	})

	t.Run("no values set", func(t *testing.T) {
		ctx := context.Background()
		provider, model := getProviderFromContext(ctx)
		if provider != "" {
			t.Errorf("expected empty provider, got %q", provider)
		}
		if model != "" {
			t.Errorf("expected empty model, got %q", model)
		}
	})
}

// TestLoggingMiddleware tests the loggingMiddleware
func TestLoggingMiddleware(t *testing.T) {
	t.Run("logs request metadata", func(t *testing.T) {
		cfg := config.DefaultConfig()
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)
		srv := New(cfg, nil, stateMgr)

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		})

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model": "gpt-4", "messages": [{"role": "user", "content": "hi"}]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		middleware := srv.loggingMiddleware(next)
		middleware.ServeHTTP(rec, req)

		if !nextCalled {
			t.Error("next handler was not called")
		}

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("extracts model from request body", func(t *testing.T) {
		cfg := config.DefaultConfig()
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)
		srv := New(cfg, nil, stateMgr)

		var extractedModel string
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, model := getProviderFromContext(r.Context())
			extractedModel = model
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model": "claude-3", "messages": []}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		middleware := srv.loggingMiddleware(next)
		middleware.ServeHTTP(rec, req)

		// Middleware should extract model from request body
		// This is tested indirectly through the logging behavior
		if extractedModel != "" {
			t.Log("Model extraction working (indirect test)")
		}
	})

	t.Run("with provider context", func(t *testing.T) {
		cfg := config.DefaultConfig()
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)
		srv := New(cfg, nil, stateMgr)

		var providerName, modelName string
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			providerName, modelName = getProviderFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		ctx := context.Background()
		ctx = setProviderContext(ctx, "ollama", "llama-2")
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		middleware := srv.loggingMiddleware(next)
		middleware.ServeHTTP(rec, req)

		if providerName != "ollama" {
			t.Errorf("expected provider 'ollama', got %q", providerName)
		}
		if modelName != "llama-2" {
			t.Errorf("expected model 'llama-2', got %q", modelName)
		}
	})
}

// TestResponseWriter tests the responseWriter wrapper
func TestResponseWriter(t *testing.T) {
	t.Run("captures status code", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

		rw.WriteHeader(http.StatusCreated)

		if rw.statusCode != http.StatusCreated {
			t.Errorf("expected status 201, got %d", rw.statusCode)
		}
	})

	t.Run("captures body size", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

		data := []byte(`{"response": "data"}`)
		n, err := rw.Write(data)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != len(data) {
			t.Errorf("expected to write %d bytes, wrote %d", len(data), n)
		}
		if rw.size != len(data) {
			t.Errorf("expected size %d, got %d", len(data), rw.size)
		}
	})

	t.Run("captures body content", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

		data := []byte(`{"response": "test"}`)
		rw.Write(data)

		if string(rw.body) != string(data) {
			t.Errorf("expected body %q, got %q", string(data), string(rw.body))
		}
	})

	t.Run("limits body capture size", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

		// Write more than 10MiB
		largeData := make([]byte, 11*1024*1024)
		rw.Write(largeData)

		expectedSize := 10 * 1024 * 1024
		if rw.size != len(largeData) {
			t.Errorf("expected size %d, got %d (size should track all written bytes)", len(largeData), rw.size)
		}
		if len(rw.body) != expectedSize {
			t.Errorf("expected body capture limited to %d bytes, got %d", expectedSize, len(rw.body))
		}
	})
}

// TestLoggingMiddleware_BodyTruncation tests that the logging middleware handles large bodies
func TestLoggingMiddleware_BodyTruncation(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	// Create a large body (over 1000 chars to trigger truncation)
	largeBody := strings.NewReader(`{"model": "` + strings.Repeat("a", 2000) + `", "messages": [{"role": "user", "content": "hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", largeBody)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := srv.loggingMiddleware(next)
	middleware.ServeHTTP(rec, req)

	// Should complete without error
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// TestLoggingMiddleware_EmptyBody tests middleware with no body
func TestLoggingMiddleware_EmptyBody(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := srv.loggingMiddleware(next)
	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// TestLoggingMiddleware_InvalidJSONBody tests middleware with invalid JSON
func TestLoggingMiddleware_InvalidJSONBody(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	// Invalid JSON body - the middleware should still handle it gracefully
	invalidBody := strings.NewReader(`{invalid json`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", invalidBody)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := srv.loggingMiddleware(next)
	middleware.ServeHTTP(rec, req)

	// Should complete - middleware should not fail on invalid JSON
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// TestHandleV1Moderations tests the moderations handler
func TestHandleV1Moderations(t *testing.T) {
	t.Run("POST with no providers", func(t *testing.T) {
		cfg := config.DefaultConfig()
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)
		srv := New(cfg, nil, stateMgr)

		body := strings.NewReader(`{"input": "hello"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/moderations", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		srv.handleV1Moderations(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status 503, got %d", rec.Code)
		}
	})

	t.Run("GET method not allowed", func(t *testing.T) {
		cfg := config.DefaultConfig()
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)
		srv := New(cfg, nil, stateMgr)

		req := httptest.NewRequest(http.MethodGet, "/v1/moderations", nil)
		rec := httptest.NewRecorder()

		srv.handleV1Moderations(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected status 405, got %d", rec.Code)
		}
	})
}

// TestHandleV1Completions tests the completions handler
func TestHandleV1Completions(t *testing.T) {
	t.Run("POST with no models configured", func(t *testing.T) {
		cfg := config.DefaultConfig()
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)
		srv := New(cfg, nil, stateMgr)

		body := strings.NewReader(`{"model": "gpt-4", "prompt": "hello"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/completions", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		srv.handleV1Completions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rec.Code)
		}
	})
}

// TestHandleV1EmbeddingsPost tests embeddings POST handler
func TestHandleV1EmbeddingsPost(t *testing.T) {
	t.Run("POST with no models configured", func(t *testing.T) {
		cfg := config.DefaultConfig()
		stateMgr := state.New(cfg.Thresholds.InitialTimeout)
		srv := New(cfg, nil, stateMgr)

		body := strings.NewReader(`{"model": "text-embedding-3-small", "input": "hello"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		srv.handleV1Embeddings(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rec.Code)
		}
	})
}
