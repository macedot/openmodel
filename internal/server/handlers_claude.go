// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/macedot/openmodel/internal/server/converters"
)

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
	targetFormat := converters.APIFormatPassthrough // Default: no conversion (Claude passthrough)
	if hasModelConfig && modelConfig.ApiMode != "" {
		if modelConfig.ApiMode == "openai" {
			targetFormat = converters.APIFormatOpenAI
		}
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

	// Convert request if needed
	forwardBody := body
	forwardEndpoint := "/v1/messages"

	if targetFormat == converters.APIFormatOpenAI {
		// Get converter
		converter, ok := converters.GetConverter(converters.APIFormatAnthropic, converters.APIFormatOpenAI)
		if !ok {
			return handleError(c, "no converter available for Anthropic to OpenAI", fiber.StatusInternalServerError)
		}

		var err error
		forwardBody, err = converter.ConvertRequest(body)
		if err != nil {
			return handleError(c, "failed to convert request: "+err.Error(), fiber.StatusBadRequest)
		}
		forwardEndpoint = converter.GetEndpoint("/v1/messages")
		// Remove anthropic-version header for OpenAI endpoint
		delete(forwardHeaders, "anthropic-version")
	}

	if isStreaming {
		return s.streamWithFailover(c, model, forwardBody, forwardHeaders, ctx, converters.APIFormatAnthropic, targetFormat)
	}

	// Non-streaming request
	resp, providerKey, err := s.executeWithFailoverFiber(ctx, model, forwardBody, forwardHeaders, forwardEndpoint)
	if err != nil {
		s.handleAllProvidersFailedFiber(c, err)
		return nil
	}

	// Convert response if needed
	var finalResp []byte
	if targetFormat == converters.APIFormatOpenAI {
		// Get converter
		converter, ok := converters.GetConverter(converters.APIFormatAnthropic, converters.APIFormatOpenAI)
		if !ok {
			return handleError(c, "no converter available", fiber.StatusInternalServerError)
		}

		var err error
		finalResp, err = converter.ConvertResponse(resp.([]byte))
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

// validateModel checks if a model exists in the configuration
func (s *Server) validateModel(model string) error {
	if _, exists := s.config.Models[model]; !exists {
		return fmt.Errorf("model %q not found", model)
	}
	return nil
}