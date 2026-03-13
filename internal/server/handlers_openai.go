// Package server implements the HTTP server and handlers
package server

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/macedot/openmodel/internal/api/openai"
	applogger "github.com/macedot/openmodel/internal/logger"
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

	ctx, requestID := buildRequestContext(c)

	// Extract headers to forward
	forwardHeaders := extractForwardHeaders(c)

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

		plan, err := buildRoutingPlan(converters.APIFormatOpenAI, EndpointV1ChatCompletions, prov.APIMode())
		if err != nil {
			return handleError(c, err.Error(), fiber.StatusInternalServerError)
		}

		forwardBody, attemptHeaders, err := prepareForwardRequest(body, forwardHeaders, providerModel, plan)
		if err != nil {
			return handleError(c, "failed to convert request: "+err.Error(), fiber.StatusBadRequest)
		}

		if isStreaming {
			return s.streamWithFailover(c, model, forwardBody, attemptHeaders, ctx, converters.APIFormatOpenAI, plan.targetFormat)
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

		s.state.ResetModel(providerKey)
		c.Set("Content-Type", "application/json")
		return c.Send(finalResp)
	}
}
