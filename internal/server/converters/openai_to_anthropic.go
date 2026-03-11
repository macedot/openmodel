// Package converters provides API format converters
package converters

import (
	"encoding/json"
	"fmt"

	"github.com/macedot/openmodel/internal/api/anthropic"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/endpoints"
)

// OpenAIToAnthropicConverter converts OpenAI format to Anthropic format
type OpenAIToAnthropicConverter struct{}

// NewOpenAIToAnthropicConverter creates a new OpenAI to Anthropic converter
func NewOpenAIToAnthropicConverter() *OpenAIToAnthropicConverter {
	return &OpenAIToAnthropicConverter{}
}

// ConvertRequest converts OpenAI chat completion request to Anthropic messages request
func (c *OpenAIToAnthropicConverter) ConvertRequest(body []byte) ([]byte, error) {
	openaiReq, err := openai.ParseChatCompletionRequest(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI request: %w", err)
	}
	anthropicReq := anthropic.OpenAIToAnthropicRequest(openaiReq)
	result, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Anthropic request: %w", err)
	}
	return result, nil
}

// ConvertResponse converts Anthropic messages response to OpenAI chat completion response
func (c *OpenAIToAnthropicConverter) ConvertResponse(body []byte) ([]byte, error) {
	var anthropicResp anthropic.MessagesResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic response: %w", err)
	}
	openaiResp := anthropic.AnthropicToOpenAIResponse(&anthropicResp)
	result, err := json.Marshal(openaiResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI response: %w", err)
	}
	return result, nil
}

// ConvertStreamLine converts Anthropic SSE stream to OpenAI format
func (c *OpenAIToAnthropicConverter) ConvertStreamLine(line, model, id string, state *StreamState) string {
	converted := anthropic.ConvertAnthropicStreamToOpenAI(line, model, id)
	if converted == "" {
		return "" // Skip events that have no OpenAI equivalent
	}
	return converted
}

// GetEndpoint returns the Anthropic endpoint
func (c *OpenAIToAnthropicConverter) GetEndpoint(original string) string {
	return endpoints.V1Messages
}

// GetHeaders returns Anthropic-specific headers
func (c *OpenAIToAnthropicConverter) GetHeaders() map[string]string {
	return map[string]string{
		HeaderAnthropicVersion: AnthropicAPIVersion,
	}
}

// Interface assertion
var _ StreamConverter = (*OpenAIToAnthropicConverter)(nil)