// Package server provides tests for HTTP handlers
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/endpoints"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/server/converters"
	"github.com/macedot/openmodel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubProvider struct {
	name          string
	apiMode       string
	closeFn       func() error
	doRequestFn   func(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error)
	doStreamReqFn func(ctx context.Context, endpoint string, body []byte, headers map[string]string) (<-chan []byte, error)
}

func (p *stubProvider) Name() string { return p.name }

func (p *stubProvider) BaseURL() string { return "" }

func (p *stubProvider) APIMode() string {
	if p.apiMode == "" {
		return "openai"
	}
	return p.apiMode
}

func (p *stubProvider) ListModels(ctx context.Context) (*openai.ModelList, error) {
	return nil, nil
}

func (p *stubProvider) Chat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	return nil, nil
}

func (p *stubProvider) StreamChat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan openai.ChatCompletionResponse, error) {
	return nil, nil
}

func (p *stubProvider) StreamChatRaw(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan []byte, error) {
	return nil, nil
}

func (p *stubProvider) Complete(ctx context.Context, model string, req *openai.CompletionRequest) (*openai.CompletionResponse, error) {
	return nil, nil
}

func (p *stubProvider) StreamComplete(ctx context.Context, model string, req *openai.CompletionRequest) (<-chan openai.CompletionResponse, error) {
	return nil, nil
}

func (p *stubProvider) Embed(ctx context.Context, model string, input []string) (*openai.EmbeddingResponse, error) {
	return nil, nil
}

func (p *stubProvider) Moderate(ctx context.Context, input string) (*openai.ModerationResponse, error) {
	return nil, nil
}

func (p *stubProvider) DoRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error) {
	if p.doRequestFn != nil {
		return p.doRequestFn(ctx, endpoint, body, headers)
	}
	return nil, nil
}

func (p *stubProvider) DoStreamRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) (<-chan []byte, error) {
	if p.doStreamReqFn != nil {
		return p.doStreamReqFn(ctx, endpoint, body, headers)
	}
	return nil, nil
}

func (p *stubProvider) Close() error {
	if p.closeFn != nil {
		return p.closeFn()
	}
	return nil
}

// TestHandleError tests the error response helper
func TestHandleError(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return handleError(c, "test error message", fiber.StatusBadRequest)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result map[string]string
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)
	assert.Equal(t, "test error message", result["error"])
}

// TestExtractModelFromRequestBody tests model extraction from JSON body
func TestExtractModelFromRequestBody(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected string
	}{
		{
			name:     "valid model",
			body:     []byte(`{"model": "gpt-4", "messages": []}`),
			expected: "gpt-4",
		},
		{
			name:     "empty model",
			body:     []byte(`{"model": "", "messages": []}`),
			expected: "",
		},
		{
			name:     "missing model field",
			body:     []byte(`{"messages": []}`),
			expected: "",
		},
		{
			name:     "empty body",
			body:     []byte{},
			expected: "",
		},
		{
			name:     "invalid JSON",
			body:     []byte(`{invalid}`),
			expected: "",
		},
		{
			name:     "model with whitespace",
			body:     []byte(`{"model": "  gpt-4  ", "messages": []}`),
			expected: "  gpt-4  ",
		},
		{
			name:     "nested model",
			body:     []byte(`{"model": "provider/model-name", "messages": []}`),
			expected: "provider/model-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractModelFromRequestBody(tt.body)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestReplaceModelInBody tests model replacement in JSON body
func TestReplaceModelInBody(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		newModel string
		expected string
	}{
		{
			name:     "simple replacement",
			body:     []byte(`{"model": "gpt-4", "messages": []}`),
			newModel: "gpt-3.5-turbo",
			expected: "gpt-3.5-turbo",
		},
		{
			name:     "empty body",
			body:     []byte{},
			newModel: "new-model",
			expected: "", // Returns empty body
		},
		{
			name:     "empty new model",
			body:     []byte(`{"model": "gpt-4"}`),
			newModel: "",
			expected: "gpt-4", // No change
		},
		{
			name:     "preserves other fields",
			body:     []byte(`{"model": "gpt-4", "temperature": 0.7, "messages": [{"role": "user", "content": "hello"}]}`),
			newModel: "gpt-3.5-turbo",
			expected: "gpt-3.5-turbo",
		},
		{
			name:     "invalid JSON returns original",
			body:     []byte(`{invalid}`),
			newModel: "new-model",
			expected: "", // Invalid JSON returns original
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceModelInBody(tt.body, tt.newModel)

			if tt.name == "preserves other fields" {
				// Verify other fields are preserved
				var req map[string]interface{}
				err := json.Unmarshal(result, &req)
				require.NoError(t, err)
				assert.Equal(t, tt.newModel, req["model"])
				assert.Equal(t, 0.7, req["temperature"])
			} else if tt.expected != "" {
				var req map[string]interface{}
				err := json.Unmarshal(result, &req)
				require.NoError(t, err)
				assert.Equal(t, tt.expected, req["model"])
			}
		})
	}
}

// TestExtractForwardHeaders tests header extraction
func TestExtractForwardHeaders(t *testing.T) {
	tests := []struct {
		name          string
		headers       map[string]string
		expectedAuth  string
		expectedReqID string
	}{
		{
			name:          "no headers",
			headers:       map[string]string{},
			expectedAuth:  "",
			expectedReqID: "",
		},
		{
			name:          "authorization only",
			headers:       map[string]string{"Authorization": "Bearer token123"},
			expectedAuth:  "Bearer token123",
			expectedReqID: "",
		},
		{
			name:          "request ID only",
			headers:       map[string]string{"X-Request-ID": "req-123"},
			expectedAuth:  "",
			expectedReqID: "req-123",
		},
		{
			name:          "both headers",
			headers:       map[string]string{"Authorization": "Bearer token123", "X-Request-ID": "req-123"},
			expectedAuth:  "Bearer token123",
			expectedReqID: "req-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				for k, v := range tt.headers {
					c.Request().Header.Set(k, v)
				}
				result := extractForwardHeaders(c)
				assert.Equal(t, tt.expectedAuth, result["Authorization"])
				assert.Equal(t, tt.expectedReqID, result["X-Request-ID"])
				return c.SendStatus(200)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			_, err := app.Test(req)
			require.NoError(t, err)
		})
	}
}

func TestIsStreamingRequest(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected bool
	}{
		{name: "stream true", body: []byte(`{"stream":true}`), expected: true},
		{name: "stream false", body: []byte(`{"stream":false}`), expected: false},
		{name: "missing stream", body: []byte(`{"model":"gpt-4"}`), expected: false},
		{name: "invalid json", body: []byte(`{invalid}`), expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isStreamingRequest(tt.body))
		})
	}
}

func TestBuildRoutingPlan(t *testing.T) {
	t.Run("passthrough openai", func(t *testing.T) {
		plan, err := buildRoutingPlan(converters.APIFormatOpenAI, EndpointV1ChatCompletions, "openai")
		require.NoError(t, err)
		assert.Equal(t, EndpointV1ChatCompletions, plan.forwardEndpoint)
		assert.Equal(t, converters.APIFormatOpenAI, plan.targetFormat)
		assert.Nil(t, plan.converter)
	})

	t.Run("convert openai to anthropic", func(t *testing.T) {
		plan, err := buildRoutingPlan(converters.APIFormatOpenAI, EndpointV1ChatCompletions, "anthropic")
		require.NoError(t, err)
		assert.Equal(t, converters.APIFormatAnthropic, plan.targetFormat)
		assert.NotNil(t, plan.converter)
	})

	t.Run("unsupported mode", func(t *testing.T) {
		_, err := buildRoutingPlan(converters.APIFormatOpenAI, EndpointV1ChatCompletions, "custom")
		require.Error(t, err)
	})
}

// TestValidateModel tests model validation
func TestValidateModel(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"gpt-4":    {Strategy: "fallback"},
			"gpt-3.5":  {Strategy: "fallback"},
			"claude-3": {Strategy: "fallback"},
		},
	}

	srv := &Server{config: cfg}

	tests := []struct {
		name        string
		model       string
		expectError bool
	}{
		{
			name:        "existing model",
			model:       "gpt-4",
			expectError: false,
		},
		{
			name:        "another existing model",
			model:       "claude-3",
			expectError: false,
		},
		{
			name:        "non-existent model",
			model:       "non-existent-model",
			expectError: true,
		},
		{
			name:        "empty model",
			model:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := srv.validateModel(tt.model)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not found")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestFormatProviderKey tests provider key formatting
func TestFormatProviderKey(t *testing.T) {
	tests := []struct {
		name     string
		provider config.ModelProvider
		expected string
	}{
		{
			name: "standard provider",
			provider: config.ModelProvider{
				Provider: "openai",
				Model:    "gpt-4",
			},
			expected: "openai/gpt-4",
		},
		{
			name: "provider with complex model name",
			provider: config.ModelProvider{
				Provider: "anthropic",
				Model:    "claude-3-opus-20240229",
			},
			expected: "anthropic/claude-3-opus-20240229",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatProviderKey(tt.provider)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHandleRoot tests the root endpoint
func TestHandleRoot(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 12345,
			Host: "localhost",
		},
	}
	srv := &Server{config: cfg, version: "test-version"}

	app := fiber.New()
	app.Get("/", srv.handleRoot)

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	assert.Equal(t, "openmodel", result["name"])
	assert.Equal(t, "test-version", result["version"])
	assert.Equal(t, "running", result["status"])
}

// TestHandleHealth tests the health endpoint
func TestHandleHealth(t *testing.T) {
	cfg := &config.Config{}
	srv := &Server{config: cfg}

	app := fiber.New()
	app.Get("/health", srv.handleHealth)

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	assert.Equal(t, "ok", result["status"])
}

// TestHandleV1ChatCompletions_MissingModel tests error handling for missing model
func TestHandleV1ChatCompletions_MissingModel(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-4"}}},
		},
	}
	providers := map[string]provider.Provider{}
	srv := &Server{config: cfg, providers: asProviderMap(providers)}

	app := fiber.New()
	app.Post(endpoints.V1ChatCompletions, srv.handleV1ChatCompletions)

	// Request without model
	reqBody := `{"messages": [{"role": "user", "content": "hello"}]}`
	req := httptest.NewRequest("POST", endpoints.V1ChatCompletions, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

// TestHandleV1ChatCompletions_NonExistentModel tests error handling for non-existent model
func TestHandleV1ChatCompletions_NonExistentModel(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-4"}}},
		},
	}
	providers := map[string]provider.Provider{}
	srv := &Server{config: cfg, providers: asProviderMap(providers)}

	app := fiber.New()
	app.Post(endpoints.V1ChatCompletions, srv.handleV1ChatCompletions)

	// Request with non-existent model
	reqBody := `{"model": "non-existent-model", "messages": [{"role": "user", "content": "hello"}]}`
	req := httptest.NewRequest("POST", endpoints.V1ChatCompletions, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

// TestHandleV1Messages_MissingAnthropicVersion tests error handling for missing anthropic-version header
func TestHandleV1Messages_MissingAnthropicVersion(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"claude-3": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "anthropic", Model: "claude-3"}}},
		},
	}
	providers := map[string]provider.Provider{}
	srv := &Server{config: cfg, providers: asProviderMap(providers)}

	app := fiber.New()
	app.Post(endpoints.V1Messages, srv.handleV1Messages)

	// Request without anthropic-version header
	reqBody := `{"model": "claude-3", "messages": [{"role": "user", "content": "hello"}]}`
	req := httptest.NewRequest("POST", endpoints.V1Messages, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestHandleV1ChatCompletions_FailsOverToNextProvider(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"gpt-4": {
				Strategy: "fallback",
				Providers: []config.ModelProvider{
					{Provider: "first", Model: "gpt-4-a"},
					{Provider: "second", Model: "gpt-4-b"},
				},
			},
		},
		Thresholds: config.ThresholdsConfig{
			FailuresBeforeSwitch: 1,
			InitialTimeout:       1000,
			MaxTimeout:           10000,
		},
	}

	var firstCalls, secondCalls int
	srv := &Server{
		config: cfg,
		providers: providerMap{
			"first": &stubProvider{
				name: "first",
				doRequestFn: func(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error) {
					firstCalls++
					return nil, fmt.Errorf("upstream failed")
				},
			},
			"second": &stubProvider{
				name: "second",
				doRequestFn: func(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error) {
					secondCalls++
					require.Equal(t, endpoints.V1ChatCompletions, endpoint)

					var req map[string]any
					require.NoError(t, json.Unmarshal(body, &req))
					assert.Equal(t, "gpt-4-b", req["model"])

					return []byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-4-b","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`), nil
				},
			},
		},
		state: state.New(1000),
	}

	app := fiber.New()
	app.Post(endpoints.V1ChatCompletions, srv.handleV1ChatCompletions)

	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", endpoints.V1ChatCompletions, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, 1, firstCalls)
	assert.Equal(t, 1, secondCalls)
}
