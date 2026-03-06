package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/stretchr/testify/assert"
)

func TestValidateChatCompletionRequest_Integration(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "valid request",
			body:       `{"model":"test-model","messages":[{"role":"user","content":"Hello"}]}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid request with extra fields",
			body:       `{"model":"test-model","messages":[{"role":"user","content":"Hello"}],"enable_thinking":true,"think":"high"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing model",
			body:       `{"messages":[{"role":"user","content":"Hello"}]}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "model",
		},
		{
			name:       "missing messages",
			body:       `{"model":"test-model"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "messages",
		},
		{
			name:       "empty messages",
			body:       `{"model":"test-model","messages":[]}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "messages",
		},
		{
			name:       "invalid role",
			body:       `{"model":"test-model","messages":[{"role":"invalid","content":"test"}]}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "role",
		},
		{
			name:       "stream_options support",
			body:       `{"model":"test-model","messages":[{"role":"user","content":"Hi"}],"stream":true,"stream_options":{"include_usage":true}}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockProvider{
				nameVal: "mock",
				chatResult: &openai.ChatCompletionResponse{
					ID:      "chatcmpl-test",
					Object:  "chat.completion",
					Created: time.Now().Unix(),
					Model:   "test-model",
					Choices: []openai.ChatCompletionChoice{
						{
							Index:        0,
							Message:      &openai.ChatCompletionMessage{Role: "assistant", Content: "Hello!"},
							FinishReason: "stop",
						},
					},
				},
			}
			srv := newTestServer(t, mock)

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.handleV1ChatCompletions(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code, "body: %s", rec.Body.String())
			if tt.wantError != "" {
				assert.Contains(t, rec.Body.String(), tt.wantError)
			}
		})
	}
}

func TestValidateEmbeddingRequest_Integration(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "valid string input",
			body:       `{"model":"embedding-model","input":"Hello world"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid array input",
			body:       `{"model":"embedding-model","input":["Hello","World"]}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing model - validation catches before lookup",
			body:       `{"input":"Hello"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "model",
		},
		{
			name:       "missing input",
			body:       `{"model":"embedding-model"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockProvider{
				nameVal: "mock",
				embedResult: &openai.EmbeddingResponse{
					Object: "list",
					Data: []openai.EmbeddingData{
						{Object: "embedding", Index: 0, Embedding: []float64{0.1}},
					},
					Model: "embedding-model",
				},
			}
			srv := newTestServer(t, mock)

			req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.handleV1Embeddings(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code, "body: %s", rec.Body.String())
			if tt.wantError != "" {
				assert.Contains(t, rec.Body.String(), tt.wantError)
			}
		})
	}
}

func TestValidateModerationRequest_Integration(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "valid request",
			body:       `{"input":"Hello world"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing input",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockProvider{
				nameVal: "mock",
				moderateResult: &openai.ModerationResponse{
					ID:      "modr-123",
					Model:   "text-moderation-latest",
					Results: []openai.ModerationResult{{Flagged: false}},
				},
			}
			srv := newTestServer(t, mock)

			req := httptest.NewRequest(http.MethodPost, "/v1/moderations", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.handleV1Moderations(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code, "body: %s", rec.Body.String())
			if tt.wantError != "" {
				assert.Contains(t, rec.Body.String(), tt.wantError)
			}
		})
	}
}
