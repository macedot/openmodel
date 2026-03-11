// Package server implements the HTTP server and handlers
package server

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/macedot/openmodel/internal/config"
)

// formatProviderKey creates a unique key for a provider/model combination
func formatProviderKey(p config.ModelProvider) string {
	return fmt.Sprintf("%s/%s", p.Provider, p.Model)
}

// handleError writes an error response
func handleError(c *fiber.Ctx, message string, statusCode int) error {
	return c.Status(statusCode).JSON(fiber.Map{"error": message})
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