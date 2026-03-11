// Package server implements the HTTP server and handlers
package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/macedot/openmodel/internal/api/anthropic"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	applogger "github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
)

// formatProviderKey creates a unique key for a provider/model combination
func formatProviderKey(p config.ModelProvider) string {
	return fmt.Sprintf("%s/%s", p.Provider, p.Model)
}

// handleError writes an error response
func handleError(c *fiber.Ctx, message string, statusCode int) error {
	return c.Status(statusCode).JSON(fiber.Map{"error": message})
}

// handleV1ChatCompletions handles POST /v1/chat/completions
func (s *Server) handleV1ChatCompletions(c *fiber.Ctx) error {
	// Read raw request body
	body := c.Body()

	// Validate request
	if err := openai.ValidateChatCompletionRequest(body); err != nil {
		return handleError(c, err.Error(), fiber.StatusBadRequest)
	}

	// Extract model from request for routing
	model := extractModelFromRequestBody(body)
	if model == "" {
		return handleError(c, "model is required", fiber.StatusBadRequest)
	}

	// Check if model exists in config
	if err := s.validateModel(model); err != nil {
		return handleError(c, err.Error(), fiber.StatusNotFound)
	}

	// Check if model has api_mode configured
	modelConfig, hasModelConfig := s.config.Models[model]
	apiMode := ""
	if hasModelConfig && modelConfig.ApiMode != "" {
		apiMode = modelConfig.ApiMode
	}

	// Set original URL in context for tracing
	ctx := context.WithValue(c.UserContext(), "original_url", c.OriginalURL())
	ctx = context.WithValue(ctx, "request_id", c.Locals("request_id"))

	// Extract headers to forward
	forwardHeaders := extractForwardHeaders(c)

	// Check if streaming request
	isStreaming := false
	var reqMap map[string]any
	if err := json.Unmarshal(body, &reqMap); err == nil {
		if stream, ok := reqMap["stream"].(bool); ok {
			isStreaming = stream
		}
	}

	// Convert request if api_mode is set
	forwardBody := body
	forwardEndpoint := "/v1/chat/completions"

	if apiMode == "anthropic" {
		// Convert OpenAI request to Anthropic format
		openaiReq, err := openai.ParseChatCompletionRequest(body)
		if err != nil {
			return handleError(c, "failed to parse request: "+err.Error(), fiber.StatusBadRequest)
		}
		anthropicReq := anthropic.OpenAIToAnthropicRequest(openaiReq)
		forwardBody, err = json.Marshal(anthropicReq)
		if err != nil {
			return handleError(c, "failed to convert request", fiber.StatusInternalServerError)
		}
		forwardEndpoint = "/v1/messages"
		// Add anthropic-version header
		forwardHeaders["anthropic-version"] = "2023-06-01"
	}

	if isStreaming {
		// For streaming, use streamWithFailover for automatic retry
		return s.streamWithFailoverFiber(c, model, forwardBody, forwardHeaders, ctx, apiMode)
	}

	// Non-streaming: forward request
	resp, providerKey, err := s.executeWithFailoverFiber(ctx, model, forwardBody, forwardHeaders, forwardEndpoint)
	if err != nil {
		s.handleAllProvidersFailedFiber(c, err)
		return nil
	}

	// Convert response if needed
	var finalResp []byte
	if apiMode == "anthropic" {
		// Convert Anthropic response back to OpenAI format
		var anthropicResp anthropic.MessagesResponse
		if err := json.Unmarshal(resp.([]byte), &anthropicResp); err != nil {
			return handleError(c, "failed to parse response", fiber.StatusInternalServerError)
		}
		openaiResp := anthropic.AnthropicToOpenAIResponse(&anthropicResp)
		finalResp, err = json.Marshal(openaiResp)
		if err != nil {
			return handleError(c, "failed to convert response", fiber.StatusInternalServerError)
		}
	} else {
		finalResp = resp.([]byte)
	}

	// Return response
	s.state.ResetModel(providerKey)
	c.Set("Content-Type", "application/json")
	return c.Send(finalResp)
}

// handleV1Messages handles POST /v1/messages (Claude API)
func (s *Server) handleV1Messages(c *fiber.Ctx) error {
	// Validate required headers (anthropic-version is required)
	anthropicVersion := c.Get("anthropic-version")
	if anthropicVersion == "" {
		return handleError(c, "anthropic-version header is required", fiber.StatusBadRequest)
	}

	// Read request body
	body := c.Body()

	// Extract model name from request body
	model := extractModelFromRequestBody(body)
	if model == "" {
		return handleError(c, "model is required", fiber.StatusBadRequest)
	}

	// Check if model exists in config
	if err := s.validateModel(model); err != nil {
		c.Set("Content-Type", "application/json")
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"type": "error",
			"error": fiber.Map{
				"type":    "invalid_request_error",
				"message": "model not found",
			},
		})
	}

	// Check if model has api_mode configured
	modelConfig, hasModelConfig := s.config.Models[model]
	apiMode := ""
	if hasModelConfig && modelConfig.ApiMode != "" {
		apiMode = modelConfig.ApiMode
	}

	// Set original URL in context for tracing
	ctx := context.WithValue(c.UserContext(), "original_url", c.OriginalURL())
	ctx = context.WithValue(ctx, "request_id", c.Locals("request_id"))

	// Check if streaming is requested
	var isStreaming bool
	var reqMap map[string]any
	if err := json.Unmarshal(body, &reqMap); err == nil {
		if stream, ok := reqMap["stream"].(bool); ok {
			isStreaming = stream
		}
	}

	// Build headers for Claude API
	forwardHeaders := map[string]string{
		"anthropic-version": anthropicVersion,
	}
	// Add optional headers
	if requestID, ok := c.Locals("request_id").(string); ok && requestID != "" {
		forwardHeaders["X-Request-ID"] = requestID
	}

	// Convert request if api_mode is set
	forwardBody := body
	forwardEndpoint := "/v1/messages"

	if apiMode == "openai" {
		// Convert Anthropic request to OpenAI format
		anthropicReq, err := anthropic.ParseMessagesRequest(body)
		if err != nil {
			return handleError(c, "failed to parse request: "+err.Error(), fiber.StatusBadRequest)
		}
		openaiReq := anthropic.AnthropicToOpenAIRequest(anthropicReq)
		forwardBody, err = json.Marshal(openaiReq)
		if err != nil {
			return handleError(c, "failed to convert request", fiber.StatusInternalServerError)
		}
		forwardEndpoint = "/v1/chat/completions"
		// Remove anthropic-version header for OpenAI endpoint
		delete(forwardHeaders, "anthropic-version")
	}

	if isStreaming {
		return s.streamWithFailoverFiberClaude(c, model, forwardBody, forwardHeaders, ctx, apiMode)
	}

	// Non-streaming request
	resp, providerKey, err := s.executeWithFailoverFiber(ctx, model, forwardBody, forwardHeaders, forwardEndpoint)
	if err != nil {
		s.handleAllProvidersFailedFiber(c, err)
		return nil
	}

	// Convert response if needed
	var finalResp []byte
	if apiMode == "openai" {
		// Convert OpenAI response back to Anthropic format
		var openaiResp openai.ChatCompletionResponse
		if err := json.Unmarshal(resp.([]byte), &openaiResp); err != nil {
			return handleError(c, "failed to parse response", fiber.StatusInternalServerError)
		}
		anthropicResp := anthropic.OpenAIToAnthropicResponse(&openaiResp)
		finalResp, err = json.Marshal(anthropicResp)
		if err != nil {
			return handleError(c, "failed to convert response", fiber.StatusInternalServerError)
		}
	} else {
		finalResp = resp.([]byte)
	}

	// Response is in Claude format
	s.state.ResetModel(providerKey)
	c.Set("Content-Type", "application/json")
	return c.Send(finalResp)
}

// extractForwardHeaders extracts headers that should be forwarded to providers
func extractForwardHeaders(c *fiber.Ctx) map[string]string {
	headers := make(map[string]string)

	// Forward Authorization header
	if auth := c.Get("Authorization"); auth != "" {
		headers["Authorization"] = auth
	}

	// Forward X-Request-ID header
	if requestID := c.Get("X-Request-ID"); requestID != "" {
		headers["X-Request-ID"] = requestID
	}

	return headers
}

// extractModelFromRequestBody extracts model from raw JSON body
func extractModelFromRequestBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err == nil {
		return req.Model
	}
	return ""
}

// replaceModelInBody replaces the model field in a JSON request body
func replaceModelInBody(body []byte, newModel string) []byte {
	if len(body) == 0 || newModel == "" {
		return body
	}

	// Parse as generic map to preserve all fields
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}

	// Replace model field
	req["model"] = newModel

	// Re-encode
	result, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return result
}

// validateModel checks if a model exists in the configuration
func (s *Server) validateModel(model string) error {
	if _, exists := s.config.Models[model]; !exists {
		return fmt.Errorf("model %q not found", model)
	}
	return nil
}

// handleAllProvidersFailedFiber handles when all providers have failed
func (s *Server) handleAllProvidersFailedFiber(c *fiber.Ctx, lastErr error) {
	errMsg := "all providers failed"
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	requestID, _ := c.Locals("request_id").(string)
	applogger.Error("all_providers_failed", "request_id", requestID, "error", errMsg)

	timeout := s.state.GetProgressiveTimeout()
	s.state.IncrementTimeout(s.config.Thresholds.MaxTimeout)

	c.Set("Retry-After", fmt.Sprintf("%d", timeout/1000))
	handleError(c, errMsg, fiber.StatusServiceUnavailable)
}

// executeWithFailoverFiber handles non-streaming requests with failover
func (s *Server) executeWithFailoverFiber(ctx context.Context, model string, body []byte, headers map[string]string, endpoint string) (any, string, error) {
	var triedProviders []string

	for {
		prov, providerKey, providerModel, err := s.findProviderWithFailover(model, "")
		if err != nil {
			requestID, _ := ctx.Value("request_id").(string)
			applogger.Error("all_providers_failed",
				"request_id", requestID,
				"model", model,
				"providers_tried", triedProviders,
				"error", err.Error())
			return nil, "", fmt.Errorf("model %q temporarily unavailable: all providers failed", model)
		}

		triedProviders = append(triedProviders, providerKey)
		threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch

		// Log request processing
		requestID, _ := ctx.Value("request_id").(string)
		applogger.Debug("PROCESSING", "request_id", requestID, "provider", providerKey, "model", model)

		// Replace model name in body
		provBody := replaceModelInBody(body, providerModel)

		resp, err := prov.DoRequest(ctx, endpoint, provBody, headers)
		if err != nil {
			s.handleProviderError(providerKey, err, threshold)
			continue
		}

		return resp, providerKey, nil
	}
}

// streamWithFailoverFiber handles streaming requests with failover for OpenAI format
func (s *Server) streamWithFailoverFiber(c *fiber.Ctx, model string, body []byte, headers map[string]string, ctx context.Context, apiMode string) error {
	var triedProviders []string
	requestID, _ := c.Locals("request_id").(string)

	for {
		prov, providerKey, providerModel, err := s.findProviderWithFailover(model, "")
		if err != nil {
			applogger.Error("all_providers_failed",
				"request_id", requestID,
				"model", model,
				"providers_tried", triedProviders,
				"error", err.Error())
			s.handleAllProvidersFailedFiber(c, fmt.Errorf("model %q temporarily unavailable: all providers failed", model))
			return nil
		}

		triedProviders = append(triedProviders, providerKey)
		threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch

		// Log request processing
		applogger.Debug("PROCESSING", "request_id", requestID, "provider", providerKey, "model", model)

		// Store provider in context for logging
		c.Locals("provider", providerKey)
		c.Locals("model", model)

		// Replace model name in body
		provBody := replaceModelInBody(body, providerModel)

		// Set streaming headers
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no")
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			defer w.Flush()

			stream, err := prov.DoStreamRequest(ctx, "/v1/chat/completions", provBody, headers)
			if err != nil {
				applogger.Warn("provider_stream_failed",
					"request_id", requestID,
					"provider", providerKey,
					"error", err.Error())
				s.state.RecordFailure(providerKey, threshold)
				return
			}

			// Track state for stream conversion if needed
			streamID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())

			for line := range stream {
				lineStr := string(line)
				// Convert stream format if apiMode is set
				if apiMode == "anthropic" {
					// Provider sends Anthropic format, convert to OpenAI for client
					converted := anthropic.ConvertAnthropicStreamToOpenAI(lineStr, model, streamID)
					if converted == "" {
						continue // Skip events that have no OpenAI equivalent
					}
					if _, err := fmt.Fprintf(w, "%s\n", converted); err != nil {
						applogger.Info("client_disconnected", "request_id", requestID, "provider", providerKey)
						return
					}
				} else {
					// Passthrough - write line as-is
					if _, err := fmt.Fprintf(w, "%s\n", lineStr); err != nil {
						applogger.Info("client_disconnected", "request_id", requestID, "provider", providerKey)
						return
					}
				}
				w.Flush()
			}

			// Write [DONE] marker for OpenAI format
			if apiMode != "anthropic" {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				w.Flush()
			}

			s.state.ResetModel(providerKey)
		})
		return nil
	}
}

// streamWithFailoverFiberClaude handles streaming requests for Claude format
func (s *Server) streamWithFailoverFiberClaude(c *fiber.Ctx, model string, body []byte, headers map[string]string, ctx context.Context, apiMode string) error {
	var triedProviders []string
	requestID, _ := c.Locals("request_id").(string)

	// Determine endpoint based on apiMode
	endpoint := "/v1/messages"
	if apiMode == "openai" {
		endpoint = "/v1/chat/completions"
	}

	for {
		prov, providerKey, providerModel, err := s.findProviderWithFailover(model, "")
		if err != nil {
			applogger.Error("all_providers_failed",
				"request_id", requestID,
				"model", model,
				"providers_tried", triedProviders,
				"error", err.Error())
			s.handleAllProvidersFailedFiber(c, fmt.Errorf("model %q temporarily unavailable: all providers failed", model))
			return nil
		}

		triedProviders = append(triedProviders, providerKey)
		threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch

		// Log request processing
		applogger.Debug("PROCESSING", "request_id", requestID, "provider", providerKey, "model", model)

		// Store provider in context for logging
		c.Locals("provider", providerKey)
		c.Locals("model", model)

		// Replace model name in body
		provBody := replaceModelInBody(body, providerModel)

		// Set streaming headers
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no")
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			defer w.Flush()

			stream, err := prov.DoStreamRequest(ctx, endpoint, provBody, headers)
			if err != nil {
				applogger.Warn("provider_stream_failed",
					"request_id", requestID,
					"provider", providerKey,
					"error", err.Error())
				s.state.RecordFailure(providerKey, threshold)
				return
			}

			// Track state for stream conversion if needed
			var isFirst bool = true
			var blockIdx int = 0
			streamID := fmt.Sprintf("msg-%d", time.Now().UnixNano())

			for line := range stream {
				lineStr := string(line)
				// Convert stream format if apiMode is set
				if apiMode == "openai" {
					// Provider sends OpenAI format, convert to Anthropic for client
					converted := anthropic.ConvertOpenAIStreamToAnthropic(lineStr, model, streamID, &isFirst, &blockIdx)
					if converted == "" {
						continue // Skip events that have no Anthropic equivalent
					}
					if _, err := fmt.Fprintf(w, "%s\n", converted); err != nil {
						applogger.Info("client_disconnected", "request_id", requestID, "provider", providerKey)
						return
					}
				} else {
					// Passthrough - write line as-is (Claude SSE format)
					if _, err := fmt.Fprintf(w, "%s\n", lineStr); err != nil {
						applogger.Info("client_disconnected", "request_id", requestID, "provider", providerKey)
						return
					}
				}
				w.Flush()
			}

			s.state.ResetModel(providerKey)
		})
		return nil
	}
}

// handleProviderError handles a provider error by recording failure
func (s *Server) handleProviderError(providerKey string, err error, threshold int) {
	applogger.Warn("provider_failed", "provider", providerKey, "error", err.Error())
	s.state.RecordFailure(providerKey, threshold)
}

// findProviderWithFailover finds an available provider for a model
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

	threshold := s.config.GetThresholds(providerName).FailuresBeforeSwitch

	// Find all available providers
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
		p := available[0]
		return p.provider, p.providerKey, p.providerModel, nil
	}
}

// providerResult holds a provider with its metadata
type providerResult struct {
	provider      provider.Provider
	providerKey   string
	providerModel string
}

// findAvailableProvidersForModel returns available providers for a model
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