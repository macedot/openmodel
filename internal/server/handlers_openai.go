// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/server/converters"
)

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
	targetFormat := converters.APIFormatOpenAI // Default: no conversion
	if hasModelConfig && modelConfig.ApiMode != "" {
		if modelConfig.ApiMode == "anthropic" {
			targetFormat = converters.APIFormatAnthropic
		}
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

	// Convert request if needed
	forwardBody := body
	forwardEndpoint := "/v1/chat/completions"

	if targetFormat == converters.APIFormatAnthropic {
		// Get converter
		converter, ok := converters.GetConverter(converters.APIFormatOpenAI, converters.APIFormatAnthropic)
		if !ok {
			return handleError(c, "no converter available for OpenAI to Anthropic", fiber.StatusInternalServerError)
		}

		var err error
		forwardBody, err = converter.ConvertRequest(body)
		if err != nil {
			return handleError(c, "failed to convert request: "+err.Error(), fiber.StatusBadRequest)
		}
		forwardEndpoint = converter.GetEndpoint("/v1/chat/completions")
		for k, v := range converter.GetHeaders() {
			forwardHeaders[k] = v
		}
	}

	if isStreaming {
		// For streaming, use unified streaming handler
		return s.streamWithFailover(c, model, forwardBody, forwardHeaders, ctx, converters.APIFormatOpenAI, targetFormat)
	}

	// Non-streaming: forward request
	resp, providerKey, err := s.executeWithFailoverFiber(ctx, model, forwardBody, forwardHeaders, forwardEndpoint)
	if err != nil {
		s.handleAllProvidersFailedFiber(c, err)
		return nil
	}

	// Convert response if needed
	var finalResp []byte
	if targetFormat == converters.APIFormatAnthropic {
		// Get converter
		converter, ok := converters.GetConverter(converters.APIFormatOpenAI, converters.APIFormatAnthropic)
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

	// Return response
	s.state.ResetModel(providerKey)
	c.Set("Content-Type", "application/json")
	return c.Send(finalResp)
}