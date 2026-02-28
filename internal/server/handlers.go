// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/macedot/openmodel/internal/api/ollama"
	"github.com/macedot/openmodel/internal/api/openai"
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

// handleVersion handles GET /api/version
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ollama.VersionResponse{Version: "0.1.0"})
}

// handleTags handles GET /api/tags
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var models []ollama.ListModelResponse
	for modelName := range s.config.Models {
		models = append(models, ollama.ListModelResponse{
			Name:       modelName,
			Model:      modelName,
			ModifiedAt: time.Now(),
			Digest:     "openmodel-virtual",
			Details:    ollama.ModelDetails{Family: "openmodel"},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ollama.ListResponse{Models: models})
}

// handlePS handles GET /api/ps
func (s *Server) handlePS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"models": []interface{}{}})
}

// handleChat handles POST /api/chat
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ollama.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handleError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get the model chain
	backends, exists := s.config.Models[req.Model]
	if !exists {
		handleError(w, fmt.Sprintf("model %q not found", req.Model), http.StatusNotFound)
		return
	}

	// Default to streaming
	stream := true
	if req.Stream != nil {
		stream = *req.Stream
	}

	threshold := s.config.Thresholds.FailuresBeforeSwitch
	var lastErr error

	for _, backend := range backends {
		backendKey := fmt.Sprintf("%s/%s", backend.Provider, backend.Model)

		if !s.state.IsAvailable(backendKey, threshold) {
			continue
		}

		prov, exists := s.providers[backend.Provider]
		if !exists {
			continue
		}

		if stream {
			s.streamChatResponse(w, r, prov, backend.Model, backendKey, req.Messages, req.Options, threshold)
			return
		}

		resp, err := prov.Chat(r.Context(), backend.Model, req.Messages, req.Options)
		if err != nil {
			log.Printf("Chat failed for %s: %v", backendKey, err)
			lastErr = err
			s.state.RecordFailure(backendKey, threshold)
			continue
		}

		s.state.ResetModel(backendKey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// All providers failed
	s.handleAllProvidersFailed(w, lastErr)
}

// streamChatResponse streams chat responses in NDJSON format
func (s *Server) streamChatResponse(w http.ResponseWriter, r *http.Request, prov interface {
	StreamChat(ctx context.Context, model string, messages []ollama.Message, opts *ollama.Options) (<-chan ollama.ChatResponse, error)
}, model, backendKey string, messages []ollama.Message, opts *ollama.Options, threshold int) {

	stream, err := prov.StreamChat(r.Context(), model, messages, opts)
	if err != nil {
		log.Printf("StreamChat failed for %s: %v", backendKey, err)
		s.state.RecordFailure(backendKey, threshold)
		handleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, _ := w.(http.Flusher)

	for resp := range stream {
		data, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		w.Write(data)
		w.Write([]byte("\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}

	s.state.ResetModel(backendKey)
}

// handleGenerate handles POST /api/generate
func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ollama.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handleError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	backends, exists := s.config.Models[req.Model]
	if !exists {
		handleError(w, fmt.Sprintf("model %q not found", req.Model), http.StatusNotFound)
		return
	}

	stream := true
	if req.Stream != nil {
		stream = *req.Stream
	}

	threshold := s.config.Thresholds.FailuresBeforeSwitch
	var lastErr error

	for _, backend := range backends {
		backendKey := fmt.Sprintf("%s/%s", backend.Provider, backend.Model)

		if !s.state.IsAvailable(backendKey, threshold) {
			continue
		}

		prov, exists := s.providers[backend.Provider]
		if !exists {
			continue
		}

		if stream {
			s.streamGenerateResponse(w, r, prov, backend.Model, backendKey, req.Prompt, req.Options, threshold)
			return
		}

		resp, err := prov.Generate(r.Context(), backend.Model, req.Prompt, req.Options)
		if err != nil {
			log.Printf("Generate failed for %s: %v", backendKey, err)
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

// streamGenerateResponse streams generate responses in NDJSON format
func (s *Server) streamGenerateResponse(w http.ResponseWriter, r *http.Request, prov interface {
	StreamGenerate(ctx context.Context, model string, prompt string, opts *ollama.Options) (<-chan ollama.GenerateResponse, error)
}, model, backendKey, prompt string, opts *ollama.Options, threshold int) {

	stream, err := prov.StreamGenerate(r.Context(), model, prompt, opts)
	if err != nil {
		log.Printf("StreamGenerate failed for %s: %v", backendKey, err)
		s.state.RecordFailure(backendKey, threshold)
		handleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, _ := w.(http.Flusher)

	for resp := range stream {
		data, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		w.Write(data)
		w.Write([]byte("\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}

	s.state.ResetModel(backendKey)
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

// handleEmbed handles POST /api/embed
func (s *Server) handleEmbed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ollama.EmbedRequest
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
		backendKey := fmt.Sprintf("%s/%s", backend.Provider, backend.Model)

		if !s.state.IsAvailable(backendKey, threshold) {
			continue
		}

		prov, exists := s.providers[backend.Provider]
		if !exists {
			continue
		}

		resp, err := prov.Embed(r.Context(), backend.Model, req.Input)
		if err != nil {
			log.Printf("Embed failed for %s: %v", backendKey, err)
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

// handleEmbeddings handles POST /api/embeddings
func (s *Server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	s.handleEmbed(w, r)
}

// handleShow handles POST /api/show
func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	notImplemented(w, "/api/show")
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

	// Get the model chain
	backends, exists := s.config.Models[req.Model]
	if !exists {
		handleError(w, fmt.Sprintf("model %q not found", req.Model), http.StatusNotFound)
		return
	}

	// Convert OpenAI messages to Ollama format
	ollamaMessages := make([]ollama.Message, len(req.Messages))
	for i, msg := range req.Messages {
		ollamaMessages[i] = ollama.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Convert OpenAI options to Ollama options
	opts := openaiRequestToOllamaOptions(&req)

	threshold := s.config.Thresholds.FailuresBeforeSwitch
	var lastErr error

	for _, backend := range backends {
		backendKey := fmt.Sprintf("%s/%s", backend.Provider, backend.Model)

		if !s.state.IsAvailable(backendKey, threshold) {
			continue
		}

		prov, exists := s.providers[backend.Provider]
		if !exists {
			continue
		}

		if req.Stream {
			s.streamV1ChatCompletions(w, r, prov, backend.Model, backendKey, req.Model, ollamaMessages, opts, threshold)
			return
		}

		resp, err := prov.Chat(r.Context(), backend.Model, ollamaMessages, opts)
		if err != nil {
			log.Printf("Chat failed for %s: %v", backendKey, err)
			lastErr = err
			s.state.RecordFailure(backendKey, threshold)
			continue
		}

		s.state.ResetModel(backendKey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaChatToOpenAIResponse(resp, req.Model))
		return
	}

	// All providers failed
	s.handleAllProvidersFailed(w, lastErr)
}

// openaiRequestToOllamaOptions converts OpenAI request options to Ollama options
func openaiRequestToOllamaOptions(req *openai.ChatCompletionRequest) *ollama.Options {
	opts := &ollama.Options{}
	if req.Temperature != nil {
		opts.Temperature = *req.Temperature
	}
	if req.TopP != nil {
		opts.TopP = *req.TopP
	}
	if req.Stop != nil {
		opts.Stop = req.Stop
	}
	return opts
}

// ollamaChatToOpenAIResponse converts an Ollama chat response to OpenAI format
func ollamaChatToOpenAIResponse(resp *ollama.ChatResponse, model string) *openai.ChatCompletionResponse {
	var promptTokens, completionTokens int
	if resp.Metrics != nil {
		promptTokens = resp.Metrics.PromptEvalCount
		completionTokens = resp.Metrics.EvalCount
	}

	return &openai.ChatCompletionResponse{
		ID:      "chatcmpl-" + uuid.New().String()[:8],
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []openai.ChatCompletionChoice{
			{
				Index: 0,
				Message: &openai.ChatCompletionMessage{
					Role:    "assistant",
					Content: resp.Message.Content,
				},
				FinishReason: "stop",
			},
		},
		Usage: &openai.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}
}

// streamV1ChatCompletions streams chat completions in OpenAI SSE format
func (s *Server) streamV1ChatCompletions(w http.ResponseWriter, r *http.Request, prov interface {
	StreamChat(ctx context.Context, model string, messages []ollama.Message, opts *ollama.Options) (<-chan ollama.ChatResponse, error)
}, backendModel, backendKey, requestModel string, messages []ollama.Message, opts *ollama.Options, threshold int) {

	stream, err := prov.StreamChat(r.Context(), backendModel, messages, opts)
	if err != nil {
		log.Printf("StreamChat failed for %s: %v", backendKey, err)
		s.state.RecordFailure(backendKey, threshold)
		handleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	completionID := "chatcmpl-" + uuid.New().String()[:8]
	created := time.Now().Unix()

	for resp := range stream {
		chunk := openai.ChatCompletionChunk{
			ID:      completionID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   requestModel,
			Choices: []openai.ChatCompletionChunkChoice{
				{
					Index: 0,
					Delta: openai.ChatCompletionDelta{
						Role:    "assistant",
						Content: resp.Message.Content,
					},
				},
			},
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

	// Send final chunk with finish_reason
	finalChunk := openai.ChatCompletionChunk{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   requestModel,
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Index: 0,
				Delta: openai.ChatCompletionDelta{},
				FinishReason: func() *string { s := "stop"; return &s }(),
			},
		},
	}
	finalData, _ := json.Marshal(finalChunk)
	w.Write([]byte("data: "))
	w.Write(finalData)
	w.Write([]byte("\n\n"))
	w.Write([]byte("data: [DONE]\n\n"))
	if flusher != nil {
		flusher.Flush()
	}

	s.state.ResetModel(backendKey)
}
