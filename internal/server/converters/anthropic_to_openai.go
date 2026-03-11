// Package converters provides API format converters
package converters

import (
	"encoding/json"
	"fmt"

	"github.com/macedot/openmodel/internal/api/anthropic"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/endpoints"
)

// AnthropicToOpenAIConverter converts Anthropic format to OpenAI format
type AnthropicToOpenAIConverter struct{}

// NewAnthropicToOpenAIConverter creates a new Anthropic to OpenAI converter
func NewAnthropicToOpenAIConverter() *AnthropicToOpenAIConverter {
	return &AnthropicToOpenAIConverter{}
}

// ConvertRequest converts Anthropic messages request to OpenAI chat completion request
func (c *AnthropicToOpenAIConverter) ConvertRequest(body []byte) ([]byte, error) {
	anthropicReq, err := anthropic.ParseMessagesRequest(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic request: %w", err)
	}
	openaiReq := anthropic.AnthropicToOpenAIRequest(anthropicReq)
	result, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI request: %w", err)
	}
	return result, nil
}

// ConvertResponse converts OpenAI chat completion response to Anthropic messages response
func (c *AnthropicToOpenAIConverter) ConvertResponse(body []byte) ([]byte, error) {
	var openaiResp openai.ChatCompletionResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}
	anthropicResp := anthropic.OpenAIToAnthropicResponse(&openaiResp)
	result, err := json.Marshal(anthropicResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Anthropic response: %w", err)
	}
	return result, nil
}

// ConvertStreamLine converts OpenAI SSE stream to Anthropic format
func (c *AnthropicToOpenAIConverter) ConvertStreamLine(line, model, id string, state *StreamState) string {
	converted := anthropic.ConvertOpenAIStreamToAnthropic(line, model, id, state.IsFirst, state.BlockIdx)
	if converted == "" {
		return "" // Skip events that have no Anthropic equivalent
	}
	return converted
}

// GetEndpoint returns the OpenAI endpoint
func (c *AnthropicToOpenAIConverter) GetEndpoint(original string) string {
	return endpoints.V1ChatCompletions
}

// GetHeaders returns headers to remove (not add) - removes anthropic-version
func (c *AnthropicToOpenAIConverter) GetHeaders() map[string]string {
	return nil // No additional headers needed for OpenAI
}

// Interface assertion
var _ StreamConverter = (*AnthropicToOpenAIConverter)(nil)