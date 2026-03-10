package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/state"
)

type mockProvider struct {
	nameVal              string
	chatResult           *openai.ChatCompletionResponse
	chatStreamResult     []openai.ChatCompletionResponse
	completeResult       *openai.CompletionResponse
	completeStreamResult []openai.CompletionResponse
	embedResult          *openai.EmbeddingResponse
	moderateResult       *openai.ModerationResponse
	chatErr              error
	streamChatErr        error
	completeErr          error
	streamCompleteErr    error
	embedErr             error
	moderateErr          error
}

func (m *mockProvider) Name() string { return m.nameVal }

func (m *mockProvider) ListModels(ctx context.Context) (*openai.ModelList, error) { return nil, nil }

func (m *mockProvider) Chat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	if m.chatErr != nil {
		return nil, m.chatErr
	}
	return m.chatResult, nil
}

func (m *mockProvider) StreamChat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan openai.ChatCompletionResponse, error) {
	if m.streamChatErr != nil {
		return nil, m.streamChatErr
	}
	ch := make(chan openai.ChatCompletionResponse, len(m.chatStreamResult))
	for _, r := range m.chatStreamResult {
		ch <- r
	}
	close(ch)
	return ch, nil
}

func (m *mockProvider) StreamChatRaw(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan []byte, error) {
	if m.streamChatErr != nil {
		return nil, m.streamChatErr
	}
	ch := make(chan []byte, 10)
	// Send mock raw SSE data (with "data: " prefix like real providers)
	go func() {
		defer close(ch)
		// First chunk
		ch <- []byte(`data: {"choices":[{"delta":{"role":"assistant","content":"Hello"},"index":0,"finish_reason":null}]}`)
		// Second chunk
		ch <- []byte(`data: {"choices":[{"delta":{"content":" there!","role":"assistant"},"index":0,"finish_reason":"stop"}]}`)
		// Done
		ch <- []byte(`data: [DONE]`)
	}()
	return ch, nil
}

func (m *mockProvider) Complete(ctx context.Context, model string, req *openai.CompletionRequest) (*openai.CompletionResponse, error) {
	if m.completeErr != nil {
		return nil, m.completeErr
	}
	return m.completeResult, nil
}

func (m *mockProvider) StreamComplete(ctx context.Context, model string, req *openai.CompletionRequest) (<-chan openai.CompletionResponse, error) {
	if m.streamCompleteErr != nil {
		return nil, m.streamCompleteErr
	}
	ch := make(chan openai.CompletionResponse, len(m.completeStreamResult))
	for _, r := range m.completeStreamResult {
		ch <- r
	}
	close(ch)
	return ch, nil
}

func (m *mockProvider) Embed(ctx context.Context, model string, input []string) (*openai.EmbeddingResponse, error) {
	if m.embedErr != nil {
		return nil, m.embedErr
	}
	return m.embedResult, nil
}

func (m *mockProvider) Moderate(ctx context.Context, input string) (*openai.ModerationResponse, error) {
	if m.moderateErr != nil {
		return nil, m.moderateErr
	}
	return m.moderateResult, nil
}

func (m *mockProvider) DoRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (m *mockProvider) DoStreamRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) (<-chan []byte, error) {
	return nil, errors.New("not implemented")
}

func newTestServer(t *testing.T, mockProv *mockProvider) *Server {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Models = map[string]config.ModelConfig{
		"test-model":      {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "mock", Model: "test-model"}}},
		"test-model-2":    {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "mock", Model: "test-model-2"}}},
		"embedding-model": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "mock", Model: "embedding-model"}}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	providers := map[string]provider.Provider{}
	if mockProv != nil {
		providers["mock"] = mockProv
	}
	return New(cfg, providers, stateMgr)
}

func TestV1ModelsEndpoint(t *testing.T) {
	srv := newTestServer(t, nil)

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

	if resp.Object != "list" {
		t.Errorf("expected object 'list', got %q", resp.Object)
	}

	if len(resp.Data) != 3 {
		t.Errorf("expected 3 models, got %d", len(resp.Data))
	}

	modelIDs := make(map[string]bool)
	for _, m := range resp.Data {
		modelIDs[m.ID] = true
	}

	if !modelIDs["test-model"] {
		t.Error("expected model 'test-model' in list")
	}
	if !modelIDs["test-model-2"] {
		t.Error("expected model 'test-model-2' in list")
	}
	if !modelIDs["embedding-model"] {
		t.Error("expected model 'embedding-model' in list")
	}
}

func TestV1ChatCompletionsNonStreaming(t *testing.T) {
	mock := &mockProvider{
		nameVal: "mock",
		chatResult: &openai.ChatCompletionResponse{
			ID:      "chatcmpl-test123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "test-model",
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
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		},
	}
	srv := newTestServer(t, mock)

	body := `{"model":"test-model","messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var resp openai.ChatCompletionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Object != "chat.completion" {
		t.Errorf("expected object 'chat.completion', got %q", resp.Object)
	}

	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}

	if resp.Choices[0].Message.Content != "Hello! How can I help you?" {
		t.Errorf("expected response content, got %q", resp.Choices[0].Message.Content)
	}

	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", resp.Choices[0].FinishReason)
	}
}

func TestV1ChatCompletionsStreaming(t *testing.T) {
	mock := &mockProvider{
		nameVal: "mock",
		chatStreamResult: []openai.ChatCompletionResponse{
			{
				ID:      "chatcmpl-test123",
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   "test-model",
				Choices: []openai.ChatCompletionChoice{
					{
						Index: 0,
						Delta: &openai.ChatCompletionDelta{
							Role:    "assistant",
							Content: "Hello",
						},
					},
				},
			},
			{
				ID:      "chatcmpl-test123",
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   "test-model",
				Choices: []openai.ChatCompletionChoice{
					{
						Index: 0,
						Delta: &openai.ChatCompletionDelta{
							Role:    "assistant",
							Content: " there!",
						},
						FinishReason: "stop",
					},
				},
			},
		},
	}
	srv := newTestServer(t, mock)

	body := `{"model":"test-model","messages":[{"role":"user","content":"Hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("expected Content-Type 'text/event-stream', got %q", contentType)
	}

	bodyBytes, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	bodyStr := string(bodyBytes)
	if !strings.Contains(bodyStr, "data: ") {
		t.Error("expected SSE data prefix")
	}
	if !strings.Contains(bodyStr, "data: [DONE]") {
		t.Error("expected stream done marker")
	}
	if !strings.Contains(bodyStr, "Hello") {
		t.Error("expected content 'Hello' in stream")
	}
}

func TestV1CompletionsEndpoint(t *testing.T) {
	mock := &mockProvider{
		nameVal: "mock",
		completeResult: &openai.CompletionResponse{
			ID:      "cmpl-test123",
			Object:  "text_completion",
			Created: time.Now().Unix(),
			Model:   "test-model",
			Choices: []openai.CompletionChoice{
				{
					Text:         "This is a test completion.",
					Index:        0,
					FinishReason: "stop",
				},
			},
			Usage: &openai.Usage{
				PromptTokens:     5,
				CompletionTokens: 6,
				TotalTokens:      11,
			},
		},
	}
	srv := newTestServer(t, mock)

	body := `{"model":"test-model","prompt":"Test prompt"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1Completions(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var resp openai.CompletionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Object != "text_completion" {
		t.Errorf("expected object 'text_completion', got %q", resp.Object)
	}

	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}

	if resp.Choices[0].Text != "This is a test completion." {
		t.Errorf("expected completion text, got %q", resp.Choices[0].Text)
	}
}

func TestV1CompletionsStreaming(t *testing.T) {
	mock := &mockProvider{
		nameVal: "mock",
		completeStreamResult: []openai.CompletionResponse{
			{
				ID:      "cmpl-test123",
				Object:  "text_completion",
				Created: time.Now().Unix(),
				Model:   "test-model",
				Choices: []openai.CompletionChoice{
					{
						Text:  "Hello",
						Index: 0,
					},
				},
			},
			{
				ID:      "cmpl-test123",
				Object:  "text_completion",
				Created: time.Now().Unix(),
				Model:   "test-model",
				Choices: []openai.CompletionChoice{
					{
						Text:         " world!",
						Index:        0,
						FinishReason: "stop",
					},
				},
			},
		},
	}
	srv := newTestServer(t, mock)

	body := `{"model":"test-model","prompt":"Hi","stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1Completions(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("expected Content-Type 'text/event-stream', got %q", contentType)
	}

	bodyBytes, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	bodyStr := string(bodyBytes)
	if !strings.Contains(bodyStr, "data: ") {
		t.Error("expected SSE data prefix")
	}
	if !strings.Contains(bodyStr, "data: [DONE]") {
		t.Error("expected stream done marker")
	}
}

func TestV1EmbeddingsEndpoint(t *testing.T) {
	mock := &mockProvider{
		nameVal: "mock",
		embedResult: &openai.EmbeddingResponse{
			Object: "list",
			Data: []openai.EmbeddingData{
				{
					Object:    "embedding",
					Index:     0,
					Embedding: []float64{0.1, 0.2, 0.3},
				},
			},
			Model: "embedding-model",
			Usage: &openai.Usage{
				PromptTokens:     8,
				CompletionTokens: 0,
				TotalTokens:      8,
			},
		},
	}
	srv := newTestServer(t, mock)

	body := `{"model":"embedding-model","input":"Hello world"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1Embeddings(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var resp openai.EmbeddingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("expected object 'list', got %q", resp.Object)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(resp.Data))
	}

	if len(resp.Data[0].Embedding) != 3 {
		t.Errorf("expected embedding size 3, got %d", len(resp.Data[0].Embedding))
	}

	if resp.Data[0].Embedding[0] != 0.1 {
		t.Errorf("expected first embedding value 0.1, got %f", resp.Data[0].Embedding[0])
	}
}

func TestV1EmbeddingsWithArrayInput(t *testing.T) {
	mock := &mockProvider{
		nameVal: "mock",
		embedResult: &openai.EmbeddingResponse{
			Object: "list",
			Data: []openai.EmbeddingData{
				{Object: "embedding", Index: 0, Embedding: []float64{0.1, 0.2}},
				{Object: "embedding", Index: 1, Embedding: []float64{0.3, 0.4}},
			},
			Model: "embedding-model",
			Usage: &openai.Usage{
				PromptTokens:     16,
				CompletionTokens: 0,
				TotalTokens:      16,
			},
		},
	}
	srv := newTestServer(t, mock)

	body := `{"model":"embedding-model","input":["Hello","world"]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1Embeddings(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp openai.EmbeddingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(resp.Data))
	}
}

func TestErrorInvalidJSON(t *testing.T) {
	srv := newTestServer(t, nil)

	body := `{"model":"test-model","messages":[{"role`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var errResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if errResp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestErrorModelNotFound(t *testing.T) {
	srv := newTestServer(t, nil)

	body := `{"model":"nonexistent-model","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	var errResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "not found") {
		t.Errorf("expected 'not found' in error, got %q", errResp["error"])
	}
}

func TestErrorModelNotFoundCompletions(t *testing.T) {
	srv := newTestServer(t, nil)

	body := `{"model":"nonexistent-model","prompt":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1Completions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestErrorModelNotFoundEmbeddings(t *testing.T) {
	srv := newTestServer(t, nil)

	body := `{"model":"nonexistent-model","input":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1Embeddings(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestErrorAllProvidersFailed(t *testing.T) {
	mock := &mockProvider{
		nameVal: "mock",
		chatErr: fmt.Errorf("provider error"),
	}
	srv := newTestServer(t, mock)

	body := `{"model":"test-model","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t, nil)

	// GET /v1/chat/completions is now valid (list stored completions)
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for GET /v1/chat/completions, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/models", nil)
	rec = httptest.NewRecorder()

	srv.handleV1Models(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestIntegrationFullFlow(t *testing.T) {
	mock := &mockProvider{
		nameVal: "mock",
		chatResult: &openai.ChatCompletionResponse{
			ID:      "chatcmpl-integration",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "test-model",
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: &openai.ChatCompletionMessage{
						Role:    "assistant",
						Content: "Integration test response",
					},
					FinishReason: "stop",
				},
			},
		},
	}
	srv := newTestServer(t, mock)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/models")
	if err != nil {
		t.Fatalf("failed to get models: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for /v1/models, got %d", resp.StatusCode)
	}

	var models openai.ModelList
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		t.Fatalf("failed to decode models: %v", err)
	}
	resp.Body.Close()

	if len(models.Data) != 3 {
		t.Errorf("expected 3 models, got %d", len(models.Data))
	}

	resp, err = http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"test"}]}`))
	if err != nil {
		t.Fatalf("failed to post chat: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for /v1/chat/completions, got %d", resp.StatusCode)
	}

	var chatResp openai.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		t.Fatalf("failed to decode chat response: %v", err)
	}
	resp.Body.Close()

	if chatResp.Choices[0].Message.Content != "Integration test response" {
		t.Errorf("unexpected response: %q", chatResp.Choices[0].Message.Content)
	}
}

func TestValidationChatEmptyMessages(t *testing.T) {
	srv := newTestServer(t, nil)

	body := `{"model":"test-model","messages":[]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var errResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "messages") {
		t.Errorf("expected 'messages' in error, got %q", errResp["error"])
	}
}

func TestValidationChatInvalidRole(t *testing.T) {
	srv := newTestServer(t, nil)

	body := `{"model":"test-model","messages":[{"role":"invalid","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var errResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if !strings.Contains(errResp["error"], "invalid") {
		t.Errorf("expected 'invalid' in error, got %q", errResp["error"])
	}
}

func TestValidationCompletionEmptyPrompt(t *testing.T) {
	srv := newTestServer(t, nil)

	body := `{"model":"test-model","prompt":""}`
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1Completions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestValidationEmbeddingsEmptyInput(t *testing.T) {
	srv := newTestServer(t, nil)

	body := `{"model":"embedding-model","input":""}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1Embeddings(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestHandleRootNonGet(t *testing.T) {
	srv := newTestServer(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleRoot(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestValidationChatEmptyModel(t *testing.T) {
	srv := newTestServer(t, nil)

	body := `{"model":"","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestValidationCompletionNilPrompt(t *testing.T) {
	srv := newTestServer(t, nil)

	body := `{"model":"test-model"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1Completions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestValidationEmbeddingsNilInput(t *testing.T) {
	srv := newTestServer(t, nil)

	body := `{"model":"embedding-model"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1Embeddings(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestProviderFailover(t *testing.T) {
	// First provider fails, second provider succeeds
	firstProvider := &mockProvider{
		nameVal: "first",
		chatErr: fmt.Errorf("first provider failed"),
	}
	secondProvider := &mockProvider{
		nameVal: "second",
		chatResult: &openai.ChatCompletionResponse{
			ID:      "chatcmpl-failover",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "fallback-model",
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: &openai.ChatCompletionMessage{
						Role:    "assistant",
						Content: "Fallback response",
					},
					FinishReason: "stop",
				},
			},
		},
	}

	cfg := config.DefaultConfig()
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

	body := `{"model":"failover-model","messages":[{"role":"user","content":"test"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var resp openai.ChatCompletionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Choices[0].Message.Content != "Fallback response" {
		t.Errorf("expected fallback response, got %q", resp.Choices[0].Message.Content)
	}
}

func TestStateResetAfterSuccess(t *testing.T) {
	// This test verifies that successful responses reset provider failure state
	mock := &mockProvider{
		nameVal: "mock",
		chatResult: &openai.ChatCompletionResponse{
			ID:      "chatcmpl-reset",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "test-model",
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: &openai.ChatCompletionMessage{
						Role:    "assistant",
						Content: "Success!",
					},
					FinishReason: "stop",
				},
			},
		},
	}

	cfg := config.DefaultConfig()
	cfg.Models = map[string]config.ModelConfig{
		"test-model": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "mock", Model: "test-model"}}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	providers := map[string]provider.Provider{"mock": mock}
	srv := New(cfg, providers, stateMgr)

	providerKey := "mock/test-model"

	// Verify provider is initially available
	if !stateMgr.IsAvailable(providerKey, 2) {
		t.Error("expected provider to be available initially")
	}

	// Make a successful request
	body := `{"model":"test-model","messages":[{"role":"user","content":"test"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// After successful response, provider should still be available
	if !stateMgr.IsAvailable(providerKey, 2) {
		t.Error("expected provider to remain available after successful request")
	}
}

func TestProviderNotAvailable(t *testing.T) {
	mock := &mockProvider{
		nameVal: "mock",
	}
	cfg := config.DefaultConfig()
	cfg.Models = map[string]config.ModelConfig{
		"test-model": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "mock", Model: "test-model"}}},
	}
	cfg.Thresholds.FailuresBeforeSwitch = 3
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	providers := map[string]provider.Provider{"mock": mock}
	srv := New(cfg, providers, stateMgr)

	// Record failures to mark provider as unavailable
	providerKey := "mock/test-model"
	for i := 0; i < 3; i++ {
		stateMgr.RecordFailure(providerKey, cfg.Thresholds.FailuresBeforeSwitch)
	}

	if stateMgr.IsAvailable(providerKey, cfg.Thresholds.FailuresBeforeSwitch) {
		t.Error("expected provider to be unavailable after 3 failures")
	}

	// Attempt request - should get 503 because all providers failed
	body := `{"model":"test-model","messages":[{"role":"user","content":"test"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}
}
