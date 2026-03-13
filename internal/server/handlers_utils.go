// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/server/converters"
)

// formatProviderKey creates a unique key for a provider/model combination
func formatProviderKey(p config.ModelProvider) string {
	return fmt.Sprintf("%s/%s", p.Provider, p.Model)
}

// handleError writes an error response
func handleError(c *fiber.Ctx, message string, statusCode int) error {
	return c.Status(statusCode).JSON(fiber.Map{"error": message})
}

type routingPlan struct {
	forwardEndpoint string
	targetFormat    converters.APIFormat
	converter       converters.StreamConverter
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

func buildRequestContext(c *fiber.Ctx) (context.Context, string) {
	requestID, _ := c.Locals("request_id").(string)
	return provider.WithRequestMetadata(c.UserContext(), requestID, c.OriginalURL()), requestID
}

func isStreamingRequest(body []byte) bool {
	var reqMap map[string]any
	if err := json.Unmarshal(body, &reqMap); err != nil {
		return false
	}
	stream, _ := reqMap["stream"].(bool)
	return stream
}

func copyHeaders(headers map[string]string) map[string]string {
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

func buildRoutingPlan(sourceFormat converters.APIFormat, receivedEndpoint, apiMode string) (routingPlan, error) {
	plan := routingPlan{
		forwardEndpoint: receivedEndpoint,
		targetFormat:    sourceFormat,
	}

	switch apiMode {
	case "", string(sourceFormat):
		return plan, nil
	}

	var ok bool
	switch apiMode {
	case string(converters.APIFormatOpenAI):
		plan.targetFormat = converters.APIFormatOpenAI
	case string(converters.APIFormatAnthropic):
		plan.targetFormat = converters.APIFormatAnthropic
	default:
		return routingPlan{}, fmt.Errorf("unsupported api_mode %q", apiMode)
	}

	plan.converter, ok = converters.GetConverter(sourceFormat, plan.targetFormat)
	if !ok {
		return routingPlan{}, fmt.Errorf("no converter available for %s to %s", sourceFormat, plan.targetFormat)
	}
	plan.forwardEndpoint = plan.converter.GetEndpoint(receivedEndpoint)
	return plan, nil
}

func prepareForwardRequest(body []byte, headers map[string]string, providerModel string, plan routingPlan) ([]byte, map[string]string, error) {
	forwardBody := body
	forwardHeaders := copyHeaders(headers)

	if plan.converter != nil {
		var err error
		forwardBody, err = plan.converter.ConvertRequest(body)
		if err != nil {
			return nil, nil, err
		}
		for key, value := range plan.converter.GetHeaders() {
			forwardHeaders[key] = value
		}
	}

	return replaceModelInBody(forwardBody, providerModel), forwardHeaders, nil
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
