// Package server implements the HTTP server and handlers
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
)

// notImplemented is a helper that returns 501 Not Implemented
func notImplemented(w http.ResponseWriter, endpoint string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{
		"error":  "not implemented",
		"detail": endpoint + " endpoint is not yet implemented",
	})
}

// handleError writes an error response
func handleError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// handleRoot handles GET /
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"name":    "openmodel",
		"version": "0.1.0",
		"status":  "running",
	})
}

// handleV1Models handles GET /v1/models
func (s *Server) handleV1Models(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var models []openai.Model
	for modelName := range s.config.Models {
		models = append(models, openai.NewModel(modelName, "openmodel"))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(openai.ModelList{
		Object: "list",
		Data:   models,
	})
}

// handleV1Model handles GET /v1/models/{model}
func (s *Server) handleV1Model(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	modelName := r.URL.Path[len("/v1/models/"):]
	if modelName == "" {
		http.Error(w, "model name required", http.StatusBadRequest)
		return
	}

	// Check if model exists
	if _, exists := s.config.Models[modelName]; !exists {
		http.Error(w, "model not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(openai.NewModel(modelName, "openmodel"))
}

// handleV1ChatCompletions handles POST /v1/chat/completions
func (s *Server) handleV1ChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handleError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	backends, exists := s.config.Models[req.Model]
	if !exists {
		handleError(w, fmt.Sprintf("model %q not found", req.Model), http.StatusNotFound)
		return
	}

	threshold := s.config.Thresholds.FailuresBeforeSwitch
	var lastErr error

	for _, backend := range backends {
		backendKey := fmt.Sprintf("%s/%s", backend.Backend, backend.Model)

		if !s.state.IsAvailable(backendKey, threshold) {
			continue
		}

		prov, exists := s.backends[backend.Backend]
		if !exists {
			continue
		}

		if req.Stream {
			s.streamV1ChatCompletions(w, r, prov, backend.Model, backendKey, req.Model, req.Messages, &req, threshold)
			return
		}

		resp, err := prov.Chat(r.Context(), backend.Model, req.Messages, &req)
		if err != nil {
			logger.Error("Chat failed", "backend", backendKey, "error", err)
			lastErr = err
			s.state.RecordFailure(backendKey, threshold)
			continue
		}

		s.state.ResetModel(backendKey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	s.handleAllProvidersFailed(w, lastErr)
}

// streamV1ChatCompletions streams chat completions in OpenAI SSE format
func (s *Server) streamV1ChatCompletions(w http.ResponseWriter, r *http.Request, prov provider.Provider, backendModel, backendKey, requestModel string, messages []openai.ChatCompletionMessage, req *openai.ChatCompletionRequest, threshold int) {
	stream, err := prov.StreamChat(r.Context(), backendModel, messages, req)
	if err != nil {
		logger.Error("StreamChat failed", "backend", backendKey, "error", err)
		s.state.RecordFailure(backendKey, threshold)
		handleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	flusher, _ := w.(http.Flusher)

	completionID := "chatcmpl-" + uuid.New().String()[:8]
	created := time.Now().Unix()

	for resp := range stream {
		choices := make([]openai.ChatCompletionChunkChoice, len(resp.Choices))
		for i, c := range resp.Choices {
			choices[i] = openai.ChatCompletionChunkChoice{
				Index: c.Index,
				Delta: openai.ChatCompletionDelta{
					Role:    c.Message.Role,
					Content: c.Message.Content,
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
			continue
		}
		w.Write([]byte("data: "))
		w.Write(data)
		w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}

	w.Write([]byte("data: [DONE]\n\n"))
	if flusher != nil {
		flusher.Flush()
	}

	s.state.ResetModel(backendKey)
}

// handleV1Completions handles POST /v1/completions
func (s *Server) handleV1Completions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req openai.CompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handleError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	backends, exists := s.config.Models[req.Model]
	if !exists {
		handleError(w, fmt.Sprintf("model %q not found", req.Model), http.StatusNotFound)
		return
	}

	threshold := s.config.Thresholds.FailuresBeforeSwitch
	var lastErr error

	for _, backend := range backends {
		backendKey := fmt.Sprintf("%s/%s", backend.Backend, backend.Model)

		if !s.state.IsAvailable(backendKey, threshold) {
			continue
		}

		prov, exists := s.backends[backend.Backend]
		if !exists {
			continue
		}

		if req.Stream {
			s.streamV1Completions(w, r, prov, backend.Model, backendKey, req.Model, &req, threshold)
			return
		}

		resp, err := prov.Complete(r.Context(), backend.Model, &req)
		if err != nil {
			logger.Error("Complete failed", "backend", backendKey, "error", err)
			lastErr = err
			s.state.RecordFailure(backendKey, threshold)
			continue
		}

		s.state.ResetModel(backendKey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	s.handleAllProvidersFailed(w, lastErr)
}

// streamV1Completions streams completions in SSE format
func (s *Server) streamV1Completions(w http.ResponseWriter, r *http.Request, prov provider.Provider, backendModel, backendKey, requestModel string, req *openai.CompletionRequest, threshold int) {
	stream, err := prov.StreamComplete(r.Context(), backendModel, req)
	if err != nil {
		logger.Error("StreamComplete failed", "backend", backendKey, "error", err)
		s.state.RecordFailure(backendKey, threshold)
		handleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	flusher, _ := w.(http.Flusher)

	completionID := "cmpl-" + uuid.New().String()[:8]
	created := time.Now().Unix()

	for resp := range stream {
		resp.ID = completionID
		resp.Created = created
		resp.Model = requestModel

		data, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		w.Write([]byte("data: "))
		w.Write(data)
		w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}

	w.Write([]byte("data: [DONE]\n\n"))
	if flusher != nil {
		flusher.Flush()
	}

	s.state.ResetModel(backendKey)
}

// handleV1Embeddings handles POST /v1/embeddings
func (s *Server) handleV1Embeddings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req openai.EmbeddingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handleError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	backends, exists := s.config.Models[req.Model]
	if !exists {
		handleError(w, fmt.Sprintf("model %q not found", req.Model), http.StatusNotFound)
		return
	}

	threshold := s.config.Thresholds.FailuresBeforeSwitch
	var lastErr error

	// Convert input to string slice
	input := convertInputToSlice(req.Input)

	for _, backend := range backends {
		backendKey := fmt.Sprintf("%s/%s", backend.Backend, backend.Model)

		if !s.state.IsAvailable(backendKey, threshold) {
			continue
		}

		prov, exists := s.backends[backend.Backend]
		if !exists {
			continue
		}

		resp, err := prov.Embed(r.Context(), backend.Model, input)
		if err != nil {
			logger.Error("Embed failed", "backend", backendKey, "error", err)
			lastErr = err
			s.state.RecordFailure(backendKey, threshold)
			continue
		}

		s.state.ResetModel(backendKey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	s.handleAllProvidersFailed(w, lastErr)
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

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", fmt.Sprintf("%d", timeout/1000))
	w.WriteHeader(http.StatusServiceUnavailable)

	errMsg := "all providers failed"
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
}
