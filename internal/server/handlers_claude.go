// Package server implements the HTTP server and handlers
package server

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	applogger "github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/server/converters"
)

// handleV1Messages handles POST /v1/messages (Claude API)
func (s *Server) handleV1Messages(c *fiber.Ctx) error {
	// Validate required headers (anthropic-version is required)
	anthropicVersion := c.Get(HeaderAnthropicVersion)
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

	ctx, requestID := buildRequestContext(c)

	isStreaming := isStreamingRequest(body)

	attemptedProviders := 0
	for {
		// Find provider first to determine api_mode
		prov, providerKey, providerModel, err := s.findProviderWithFailover(model, "")
		if err != nil {
			if attemptedProviders > 0 {
				s.handleAllProvidersFailedFiber(c, fmt.Errorf("model %q temporarily unavailable: all providers failed", model))
				return nil
			}
			return handleError(c, err.Error(), fiber.StatusNotFound)
		}
		attemptedProviders++

		// Log provider selection
		applogger.Debug("ROUTING", "request_id", requestID, "provider", providerKey, "model", providerModel, "api_mode", prov.APIMode())

		plan, err := buildRoutingPlan(converters.APIFormatAnthropic, EndpointV1Messages, prov.APIMode())
		if err != nil {
			return handleError(c, err.Error(), fiber.StatusInternalServerError)
		}

		// Build headers for Claude API
		forwardHeaders := map[string]string{}
		if plan.converter == nil {
			forwardHeaders[HeaderAnthropicVersion] = anthropicVersion
		}
		if requestID != "" {
			forwardHeaders["X-Request-ID"] = requestID
		}

		forwardBody, attemptHeaders, err := prepareForwardRequest(body, forwardHeaders, providerModel, plan)
		if err != nil {
			return handleError(c, "failed to convert request: "+err.Error(), fiber.StatusBadRequest)
		}

		if isStreaming {
			return s.streamWithFailover(c, model, forwardBody, attemptHeaders, ctx, converters.APIFormatAnthropic, plan.targetFormat)
		}

		resp, err := prov.DoRequest(ctx, plan.forwardEndpoint, forwardBody, attemptHeaders)
		if err != nil {
			threshold := s.GetConfig().GetThresholds(providerKey).FailuresBeforeSwitch
			s.handleProviderError(providerKey, err, threshold)
			continue
		}

		var finalResp []byte
		if plan.converter != nil {
			finalResp, err = plan.converter.ConvertResponse(resp)
			if err != nil {
				return handleError(c, "failed to convert response", fiber.StatusInternalServerError)
			}
		} else {
			finalResp = resp
		}

		// Response is in Claude format
		s.state.ResetModel(providerKey)
		c.Set("Content-Type", "application/json")
		return c.Send(finalResp)
	}
}

// validateModel checks if a model exists in the configuration
func (s *Server) validateModel(model string) error {
	if _, exists := s.GetConfig().Models[model]; !exists {
		return fmt.Errorf("model %q not found", model)
	}
	return nil
}
