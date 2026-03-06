// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
)

// formatProviderKey creates a unique key for a provider/model combination
func formatProviderKey(p config.ModelProvider) string {
	return fmt.Sprintf("%s/%s", p.Provider, p.Model)
}

// setupStreamHeaders sets up SSE response headers and returns a flusher
func setupStreamHeaders(w http.ResponseWriter) http.Flusher {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	flusher, _ := w.(http.Flusher)
	return flusher
}

// writeSSEChunk writes a data chunk in SSE format with flush
func writeSSEChunk(w http.ResponseWriter, flusher http.Flusher, data []byte) error {
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

// writeSSEDone writes the SSE [DONE] message with flush
func writeSSEDone(w http.ResponseWriter, flusher http.Flusher) error {
	if _, err := w.Write([]byte("data: [DONE]\n\n")); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

// handleError writes an error response
func handleError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		logger.Error("Failed to encode error response", "error", err)
	}
}

// handleRoot handles GET /
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	encodeJSON(w, map[string]string{
		"name":    "openmodel",
		"version": "0.1.0",
		"status":  "running",
	})
}

// handleHealth handles GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	w.WriteHeader(http.StatusOK)
	encodeJSON(w, map[string]string{
		"status": "ok",
	})
}

// handleV1Models handles GET /v1/models
func (s *Server) handleV1Models(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	// Preallocate slice for efficiency
	models := make([]openai.Model, 0, len(s.config.Models))
	for modelName := range s.config.Models {
		models = append(models, openai.NewModel(modelName, "openmodel"))
	}

	encodeJSON(w, openai.ModelList{
		Object: "list",
		Data:   models,
	})
}

// handleV1Model handles GET and DELETE /v1/models/{model}
func (s *Server) handleV1Model(w http.ResponseWriter, r *http.Request) {
	prefix := "/v1/models/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		handleError(w, "invalid path", http.StatusBadRequest)
		return
	}
	modelName := r.URL.Path[len(prefix):]
	if modelName == "" {
		handleError(w, "model name required", http.StatusBadRequest)
		return
	}

	// Check if model exists
	if _, exists := s.config.Models[modelName]; !exists {
		handleError(w, modelNotFoundError(modelName).Error(), http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		encodeJSON(w, openai.NewModel(modelName, "openmodel"))
	case http.MethodDelete:
		// DELETE /v1/models/{model} - for fine-tuned model deletion
		// For proxy, return success (model deletion is proxy-level operation)
		w.WriteHeader(http.StatusOK)
		encodeJSON(w, map[string]interface{}{
			"id":      modelName,
			"object":  "model",
			"deleted": true,
		})
	default:
		handleError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleV1ChatCompletions handles GET and POST /v1/chat/completions
func (s *Server) handleV1ChatCompletions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleV1ChatCompletionsList(w, r)
		return
	case http.MethodPost:
		// POST /v1/chat/completions - create completion
		var req openai.ChatCompletionRequest
		if !readAndValidateRequest(w, r, 50*1024*1024, openai.ValidateChatCompletionRequest, &req) {
			return
		}

		// Check if model exists before trying providers
		if _, exists := s.config.Models[req.Model]; !exists {
			handleError(w, modelNotFoundError(req.Model).Error(), http.StatusNotFound)
			return
		}

		if req.Stream {
			// For streaming, find provider and delegate to streaming handler
			prov, providerKey, providerModel, err := s.findProviderWithFailover(req.Model, "")
			if err != nil {
				s.handleAllProvidersFailed(w, err)
				return
			}
			threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch
			*r = *r.WithContext(setProviderContext(r.Context(), providerKey, req.Model))
			s.streamV1ChatCompletions(w, r, prov, providerModel, providerKey, req.Model, req.Messages, &req, threshold)
			return
		}

		resp, providerKey, err := s.executeWithFailover(r, req.Model, "",
			func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
				return prov.Chat(ctx, providerModel, req.Messages, &req)
			},
		)
		if err != nil {
			s.handleAllProvidersFailed(w, err)
			return
		}

		s.handleProviderSuccess(w, providerKey, resp)
		return
	default:
		handleError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleV1ChatCompletionsList handles GET /v1/chat/completions
// Returns an empty list for proxy since we don't store completions
func (s *Server) handleV1ChatCompletionsList(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	encodeJSON(w, map[string]interface{}{
		"object": "list",
		"data":   []interface{}{},
	})
}

// streamV1ChatCompletions streams chat completions in OpenAI SSE format
func (s *Server) streamV1ChatCompletions(w http.ResponseWriter, r *http.Request, prov provider.Provider, providerModel, providerKey, requestModel string, messages []openai.ChatCompletionMessage, req *openai.ChatCompletionRequest, threshold int) {
	stream, err := prov.StreamChat(r.Context(), providerModel, messages, req)
	if err != nil {
		logger.Error("StreamChat failed", "provider", providerKey, "error", err)
		s.state.RecordFailure(providerKey, threshold)
		handleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	completionID := "chatcmpl-" + uuid.New().String()[:8]
	created := time.Now().Unix()

	s.streamCommon(w, r, providerKey, requestModel, threshold, completionID, func(flusher http.Flusher) bool {
		for resp := range stream {
			// Check if client disconnected
			if checkClientDisconnect(r) {
				logger.Info("Client disconnected, closing stream", "provider", providerKey)
				s.state.RecordFailure(providerKey, threshold)
				return false
			}

			choices := make([]openai.ChatCompletionChunkChoice, len(resp.Choices))
			for i, c := range resp.Choices {
				var role, content string
				if c.Delta != nil {
					role = c.Delta.Role
					content = c.Delta.Content
				}
				choices[i] = openai.ChatCompletionChunkChoice{
					Index: c.Index,
					Delta: openai.ChatCompletionDelta{
						Role:    role,
						Content: content,
					},
					FinishReason: func() *string { s := c.FinishReason; return &s }(),
				}
			}
			chunk := openai.ChatCompletionChunk{
				ID:      completionID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   requestModel,
				Choices: choices,
			}

			data, err := json.Marshal(chunk)
			if err != nil {
				logger.Error("Failed to marshal stream chunk", "provider", providerKey, "error", err)
				continue
			}
			if err := writeSSEChunk(w, flusher, data); err != nil {
				logger.Error("Failed to write stream chunk", "provider", providerKey, "error", err)
				s.state.RecordFailure(providerKey, threshold)
				return false
			}
		}
		return true
	})

	// Drain remaining messages to unblock provider goroutine
	defer drainStream(stream)
}

// handleV1Completions handles POST /v1/completions
func (s *Server) handleV1Completions(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// Note: CompletionRequest uses ChatCompletionRequest structure under the hood
	// We validate by decoding and checking required fields manually
	// since we don't have a raw-bytes validator for completions
	var req openai.CompletionRequest
	if !readAndValidateRequest(w, r, 50*1024*1024, nil, &req) {
		return
	}

	// Manual validation for completion request
	if req.Model == "" {
		handleError(w, "model is required", http.StatusBadRequest)
		return
	}
	if req.Prompt == nil {
		handleError(w, "prompt is required", http.StatusBadRequest)
		return
	}
	// Check if prompt is empty string or empty array
	switch p := req.Prompt.(type) {
	case string:
		if p == "" {
			handleError(w, "prompt cannot be empty", http.StatusBadRequest)
			return
		}
	case []any:
		if len(p) == 0 {
			handleError(w, "prompt array cannot be empty", http.StatusBadRequest)
			return
		}
	}

	// Check if model exists before trying providers
	if _, exists := s.config.Models[req.Model]; !exists {
		handleError(w, modelNotFoundError(req.Model).Error(), http.StatusNotFound)
		return
	}

	if req.Stream {
		// For streaming, find provider and delegate to streaming handler
		prov, providerKey, providerModel, err := s.findProviderWithFailover(req.Model, "")
		if err != nil {
			s.handleAllProvidersFailed(w, err)
			return
		}
		threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch
		*r = *r.WithContext(setProviderContext(r.Context(), providerKey, req.Model))
		s.streamV1Completions(w, r, prov, providerModel, providerKey, req.Model, &req, threshold)
		return
	}

	resp, providerKey, err := s.executeWithFailover(r, req.Model, "",
		func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			return prov.Complete(ctx, providerModel, &req)
		},
	)
	if err != nil {
		s.handleAllProvidersFailed(w, err)
		return
	}

	s.handleProviderSuccess(w, providerKey, resp)
}

// streamV1Completions streams completions in SSE format
func (s *Server) streamV1Completions(w http.ResponseWriter, r *http.Request, prov provider.Provider, providerModel, providerKey, requestModel string, req *openai.CompletionRequest, threshold int) {
	stream, err := prov.StreamComplete(r.Context(), providerModel, req)
	if err != nil {
		logger.Error("StreamComplete failed", "provider", providerKey, "error", err)
		s.state.RecordFailure(providerKey, threshold)
		handleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	completionID := "cmpl-" + uuid.New().String()[:8]
	created := time.Now().Unix()

	s.streamCommon(w, r, providerKey, requestModel, threshold, completionID, func(flusher http.Flusher) bool {
		for resp := range stream {
			// Check if client disconnected
			if checkClientDisconnect(r) {
				logger.Info("Client disconnected, closing stream", "provider", providerKey)
				s.state.RecordFailure(providerKey, threshold)
				return false
			}

			resp.ID = completionID
			resp.Created = created
			resp.Model = requestModel

			data, err := json.Marshal(resp)
			if err != nil {
				logger.Error("Failed to marshal stream chunk", "provider", providerKey, "error", err)
				continue
			}
			if err := writeSSEChunk(w, flusher, data); err != nil {
				logger.Error("Failed to write stream chunk", "provider", providerKey, "error", err)
				s.state.RecordFailure(providerKey, threshold)
				return false
			}
		}
		return true
	})

	// Drain remaining messages to unblock provider goroutine
	defer drainStream(stream)
}

// handleV1Embeddings handles POST /v1/embeddings
func (s *Server) handleV1Embeddings(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req openai.EmbeddingRequest
	if !readAndValidateRequest(w, r, 50*1024*1024, openai.ValidateEmbeddingRequest, &req) {
		return
	}

	// Additional validation: check for empty input (not covered by spec validator)
	if req.Input == "" {
		handleError(w, "input cannot be empty", http.StatusBadRequest)
		return
	}
	if arr, ok := req.Input.([]any); ok && len(arr) == 0 {
		handleError(w, "input array cannot be empty", http.StatusBadRequest)
		return
	}

	// Check if model exists before trying providers
	if _, exists := s.config.Models[req.Model]; !exists {
		handleError(w, modelNotFoundError(req.Model).Error(), http.StatusNotFound)
		return
	}

	// Convert input to string slice
	input := convertInputToSlice(req.Input)

	resp, providerKey, err := s.executeWithFailover(r, req.Model, "",
		func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			return prov.Embed(ctx, providerModel, input)
		},
	)
	if err != nil {
		s.handleAllProvidersFailed(w, err)
		return
	}

	s.handleProviderSuccess(w, providerKey, resp)
}

// convertInputToSlice converts embedding input to string slice
func convertInputToSlice(input any) []string {
	switch v := input.(type) {
	case string:
		return []string{v}
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// handleAllProvidersFailed handles when all providers have failed
func (s *Server) handleAllProvidersFailed(w http.ResponseWriter, lastErr error) {
	timeout := s.state.GetProgressiveTimeout()
	s.state.IncrementTimeout(s.config.Thresholds.MaxTimeout)

	w.Header().Set("Retry-After", fmt.Sprintf("%d", timeout/1000))
	w.WriteHeader(http.StatusServiceUnavailable)

	errMsg := "all providers failed"
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	encodeJSON(w, map[string]string{"error": errMsg})
}
