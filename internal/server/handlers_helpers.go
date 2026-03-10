// Package server implements the HTTP server and handlers
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
)

// providerResult holds a provider with its metadata
type providerResult struct {
	provider      provider.Provider
	providerKey   string
	providerModel string
}

// findProviderWithFailover finds an available provider for a model based on selection strategy.
// This is the core provider lookup logic extracted to reduce duplication.
// Returns (nil, "", "", err) where err indicates why no provider was found.
func (s *Server) findProviderWithFailover(model string, providerName string) (provider.Provider, string, string, error) {
	modelConfig, exists := s.config.Models[model]
	if !exists {
		return nil, "", "", fmt.Errorf("model %q not found", model)
	}

	providers := modelConfig.Providers
	strategy := modelConfig.Strategy
	if strategy == "" {
		strategy = config.StrategyFallback
	}

	// Get thresholds for this provider (provider-specific or global)
	thresholds := s.config.GetThresholds(providerName)
	threshold := thresholds.FailuresBeforeSwitch

	// Find all available providers first
	available := s.findAvailableProvidersForModel(providers, threshold)
	if len(available) == 0 {
		return nil, "", "", fmt.Errorf("no available providers for model %q", model)
	}

	// Select based on strategy
	switch strategy {
	case config.StrategyRoundRobin:
		idx := s.state.NextRoundRobin(model, len(available))
		p := available[idx]
		return p.provider, p.providerKey, p.providerModel, nil

	case config.StrategyRandom:
		idx := s.state.GetRandomIndex(len(available))
		p := available[idx]
		return p.provider, p.providerKey, p.providerModel, nil

	case config.StrategyFallback:
		fallthrough
	default:
		// Fallback: return first available
		p := available[0]
		return p.provider, p.providerKey, p.providerModel, nil
	}
}

// findAvailableProvidersForModel returns available providers for a model based on threshold
func (s *Server) findAvailableProvidersForModel(providers []config.ModelProvider, threshold int) []providerResult {
	var results []providerResult
	for _, p := range providers {
		providerKey := formatProviderKey(p)

		if !s.state.IsAvailable(providerKey, threshold) {
			continue
		}

		prov, exists := s.providers[p.Provider]
		if !exists {
			continue
		}

		results = append(results, providerResult{
			provider:      prov,
			providerKey:   providerKey,
			providerModel: p.Model,
		})
	}
	return results
}

// findAllAvailableProviders returns all available providers for a model using global threshold.
// This is a convenience wrapper around findAvailableProvidersForModel.
func (s *Server) findAllAvailableProviders(model string) []providerResult {
	modelConfig, exists := s.config.Models[model]
	if !exists {
		return nil
	}
	return s.findAvailableProvidersForModel(modelConfig.Providers, s.config.Thresholds.FailuresBeforeSwitch)
}

// limitRequestBody limits the request body size to prevent memory exhaustion.
// Call at the start of POST handlers.
func limitRequestBody(w http.ResponseWriter, r *http.Request, maxBytes int64) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
}

// requireMethod validates HTTP method and returns error if not allowed
func requireMethod(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if r.Method != allowed {
		handleError(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// modelNotFoundError returns a formatted "model not found" error
func modelNotFoundError(model string) error {
	return fmt.Errorf("model %q not found", model)
}

// validateModel checks if a model exists in the configuration.
// Returns nil if valid, error if model not found.
func (s *Server) validateModel(model string) error {
	if _, exists := s.config.Models[model]; !exists {
		return modelNotFoundError(model)
	}
	return nil
}

// encodeJSON encodes v as JSON with proper error handling.
// Sets Content-Type header automatically.
// Returns the error if encoding fails.
func encodeJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.Error("Failed to encode response", "error", err)
		return err
	}
	return nil
}

// handleProviderError handles a provider error by recording failure and continuing to next provider.
// Returns true if should continue to next provider, false if should stop.
func (s *Server) handleProviderError(providerKey string, err error, threshold int) bool {
	logger.Warn("Provider request failed, trying next provider", "provider", providerKey, "error", err)
	s.state.RecordFailure(providerKey, threshold)
	return true
}

// handleProviderSuccess handles a successful provider response by resetting state and encoding.
func (s *Server) handleProviderSuccess(w http.ResponseWriter, providerKey string, response any) {
	s.state.ResetModel(providerKey)
	if err := encodeJSON(w, response); err != nil {
		logger.Error("Failed to encode response", "error", err)
	}
}

// isModelNotFoundError checks if the error indicates the model was not found in config.
func isModelNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "model") && strings.Contains(err.Error(), "not found")
}

// readAndValidateRequest reads body, validates, and parses.
// Returns false if error occurs (response already written), true on success.
func readAndValidateRequest(w http.ResponseWriter, r *http.Request, maxSize int64, validator func([]byte) error, target any) bool {
	limitRequestBody(w, r, maxSize)
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSize))
	if err != nil {
		handleError(w, "failed to read request body: "+err.Error(), http.StatusBadRequest)
		return false
	}
	if validator != nil {
		if err := validator(body); err != nil {
			handleError(w, err.Error(), http.StatusBadRequest)
			return false
		}
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(target); err != nil {
		handleError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// checkClientDisconnect checks if the client has disconnected.
// Returns true if disconnected, false otherwise.
func checkClientDisconnect(r *http.Request) bool {
	select {
	case <-r.Context().Done():
		return true
	default:
		return false
	}
}

// drainStream drains remaining messages from a stream channel
// to prevent goroutine leaks when client disconnects early.
// This is the generic version that works with []byte streams.
func drainStream(stream <-chan []byte) {
	for range stream {
		// Discard remaining messages
	}
}

// drainStreamTyped drains remaining messages from a typed stream channel.
// Generic version for typed streams (e.g., CompletionResponse).
func drainStreamTyped[T any](stream <-chan T) {
	for range stream {
		// Discard remaining messages
	}
}

// writeRawStream writes raw SSE lines to the response writer.
// Used for transparent proxying of streaming responses.
// Returns error if write fails or client disconnects.
func writeRawStream(w http.ResponseWriter, r *http.Request, stream <-chan []byte, providerKey string) error {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Flush headers
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}
	flusher.Flush()

	// Forward raw SSE lines to client
	for line := range stream {
		// Check for client disconnect
		if checkClientDisconnect(r) {
			logger.Info("Client disconnected, closing stream", "provider", providerKey)
			return fmt.Errorf("client disconnected")
		}

		// Write raw line as-is (already in SSE format with "data: " prefix from provider)
		if _, err := fmt.Fprintf(w, "%s\n\n", line); err != nil {
			return fmt.Errorf("failed to write stream chunk: %w", err)
		}
		flusher.Flush()
	}

	// Write [DONE] marker
	if err := writeSSEDone(w, flusher); err != nil {
		return fmt.Errorf("failed to write stream done: %w", err)
	}

	return nil
}

// writeCompletionStream writes CompletionResponse chunks to the response writer.
// Returns error if write fails or client disconnects.
func (s *Server) writeCompletionStream(w http.ResponseWriter, r *http.Request, stream <-chan openai.CompletionResponse, providerKey, requestModel string) error {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Flush headers
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}
	flusher.Flush()

	completionID := "cmpl-" + uuid.New().String()[:8]
	created := time.Now().Unix()

	for resp := range stream {
		// Check for client disconnect
		if checkClientDisconnect(r) {
			logger.Info("Client disconnected, closing stream", "provider", providerKey)
			return fmt.Errorf("client disconnected")
		}

		resp.ID = completionID
		resp.Created = created
		resp.Model = requestModel

		data, err := json.Marshal(resp)
		if err != nil {
			logger.Warn("Failed to marshal stream chunk", "provider", providerKey, "error", err)
			continue
		}

		if err := writeSSEChunk(w, flusher, data); err != nil {
			return fmt.Errorf("failed to write stream chunk: %w", err)
		}
	}

	// Write [DONE] marker
	if err := writeSSEDone(w, flusher); err != nil {
		return fmt.Errorf("failed to write stream done: %w", err)
	}

	return nil
}

// streamWithFailover handles streaming with automatic failover on connection or mid-stream errors.
// It tries providers according to strategy and only returns error when all providers exhausted.
func (s *Server) streamWithFailover(
	w http.ResponseWriter,
	r *http.Request,
	model string,
	providerName string,
	streamFn func(ctx context.Context, prov provider.Provider, providerModel string) (<-chan []byte, error),
	processChunk func(w http.ResponseWriter, r *http.Request, stream <-chan []byte, providerKey string) error,
) error {
	var triedProviders []string

	for {
		prov, providerKey, providerModel, err := s.findProviderWithFailover(model, providerName)
		if err != nil {
			// All providers exhausted - log ERROR and return
			logger.Error("All providers failed for streaming request",
				"model", model,
				"providers_tried", triedProviders,
				"error", err)
			return fmt.Errorf("model %q temporarily unavailable: all providers failed", model)
		}

		// Track which providers we've tried
		triedProviders = append(triedProviders, providerKey)

		// Get threshold for this provider
		threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch

		// Set provider/model in context for logging
		*r = *r.WithContext(setProviderContext(r.Context(), providerKey, model))

		// Try to establish stream
		stream, err := streamFn(r.Context(), prov, providerModel)
		if err != nil {
			// Connection failed - log WARN and try next provider
			logger.Warn("Provider stream connection failed, trying next provider",
				"provider", providerKey,
				"error", err)
			s.state.RecordFailure(providerKey, threshold)
			continue
		}

		// Stream established successfully - process it
		streamErr := processChunk(w, r, stream, providerKey)

		if streamErr == nil {
			// Stream completed successfully
			s.state.ResetModel(providerKey)
			return nil
		}

		// Stream failed mid-stream - drain remaining messages and try next provider
		drainStream(stream)
		logger.Warn("Provider stream failed mid-stream, trying next provider",
			"provider", providerKey,
			"error", streamErr)
		s.state.RecordFailure(providerKey, threshold)
		// Continue to next provider
	}
}

// streamWithFailoverTyped handles streaming with automatic failover for typed streams.
// This is the generic standalone function for typed stream channels (e.g., CompletionResponse).
func streamWithFailoverTyped[T any](
	s *Server,
	w http.ResponseWriter,
	r *http.Request,
	model string,
	providerName string,
	streamFn func(ctx context.Context, prov provider.Provider, providerModel string) (<-chan T, error),
	processChunk func(w http.ResponseWriter, r *http.Request, stream <-chan T, providerKey string) error,
) error {
	var triedProviders []string

	for {
		prov, providerKey, providerModel, err := s.findProviderWithFailover(model, providerName)
		if err != nil {
			// All providers exhausted - log ERROR and return
			logger.Error("All providers failed for streaming request",
				"model", model,
				"providers_tried", triedProviders,
				"error", err)
			return fmt.Errorf("model %q temporarily unavailable: all providers failed", model)
		}

		// Track which providers we've tried
		triedProviders = append(triedProviders, providerKey)

		// Get threshold for this provider
		threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch

		// Set provider/model in context for logging
		*r = *r.WithContext(setProviderContext(r.Context(), providerKey, model))

		// Try to establish stream
		stream, err := streamFn(r.Context(), prov, providerModel)
		if err != nil {
			// Connection failed - log WARN and try next provider
			logger.Warn("Provider stream connection failed, trying next provider",
				"provider", providerKey,
				"error", err)
			s.state.RecordFailure(providerKey, threshold)
			continue
		}

		// Stream established successfully - process it
		streamErr := processChunk(w, r, stream, providerKey)

		if streamErr == nil {
			// Stream completed successfully
			s.state.ResetModel(providerKey)
			return nil
		}

		// Stream failed mid-stream - drain remaining messages and try next provider
		drainStreamTyped(stream)
		logger.Warn("Provider stream failed mid-stream, trying next provider",
			"provider", providerKey,
			"error", streamErr)
		s.state.RecordFailure(providerKey, threshold)
		// Continue to next provider
	}
}

// executeWithFailover handles provider failover logic for non-streaming requests.
// It loops through available providers, executes the provided function,
// and handles failover on errors.
// Returns response (to encode), provider key (for logging), error (if all failed).
func (s *Server) executeWithFailover(
	r *http.Request,
	model string,
	providerName string,
	execute func(ctx context.Context, prov provider.Provider, providerModel string) (any, error),
) (any, string, error) {
	var triedProviders []string

	for {
		prov, providerKey, providerModel, err := s.findProviderWithFailover(model, providerName)
		if err != nil {
			// All providers exhausted - log ERROR and return
			logger.Error("All providers failed for model",
				"model", model,
				"providers_tried", triedProviders,
				"error", err)
			return nil, "", fmt.Errorf("model %q temporarily unavailable: all providers failed", model)
		}

		// Track which providers we've tried
		triedProviders = append(triedProviders, providerKey)

		// Get threshold for this provider (provider-specific or global)
		threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch

		// Set provider/model in context for logging
		*r = *r.WithContext(setProviderContext(r.Context(), providerKey, model))

		resp, err := execute(r.Context(), prov, providerModel)
		if err != nil {
			s.handleProviderError(providerKey, err, threshold)
			continue
		}

		return resp, providerKey, nil
	}
}

// readRequestBody reads and restores request body with size limit.
// Returns the body bytes and any error encountered.
func readRequestBody(r *http.Request, maxSize int64) ([]byte, error) {
	if r.Body == nil || r.ContentLength <= 0 {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSize))
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

// prettyPrintJSON formats JSON with indentation.
// Returns the formatted string, or a truncated raw string if JSON parsing fails.
func prettyPrintJSON(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err == nil {
		return buf.String()
	}
	// Fallback to raw string
	str := string(data)
	if len(str) > 1000 {
		return str[:1000] + "..."
	}
	return str
}

// logRequest logs HTTP request metadata at DEBUG level.
func logRequest(r *http.Request, contentLength int64, requestID, model string) {
	logArgs := []any{
		"method", r.Method,
		"path", r.URL.Path,
		"remote_addr", r.RemoteAddr,
		"content_length", contentLength,
		"request_id", requestID,
	}
	if model != "" {
		logArgs = append(logArgs, "model", model)
	}
	logger.Debug("HTTP request", logArgs...)
}

// logResponse logs HTTP response metadata at DEBUG level.
func logResponse(r *http.Request, statusCode, size int, latency time.Duration, requestID, provider, model string) {
	logArgs := []any{
		"method", r.Method,
		"path", r.URL.Path,
		"status", statusCode,
		"latency", latency.String(),
		"response_size", size,
		"request_id", requestID,
	}
	if provider != "" {
		logArgs = append(logArgs, "provider", provider)
	}
	if model != "" {
		logArgs = append(logArgs, "model", model)
	}
	logger.Debug("HTTP response", logArgs...)
}
