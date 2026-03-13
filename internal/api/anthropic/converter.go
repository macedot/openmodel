// Package anthropic provides conversion between OpenAI and Anthropic formats
package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/macedot/openmodel/internal/api/openai"
)

// OpenAIToAnthropicRequest converts OpenAI chat completion request to Anthropic messages request
func OpenAIToAnthropicRequest(openaiReq *openai.ChatCompletionRequest) *MessagesRequest {
	anthropicReq := &MessagesRequest{
		Model:       openaiReq.Model,
		MaxTokens:   4096, // Default
		Stream:      openaiReq.Stream,
		Temperature: openaiReq.Temperature,
		TopP:        openaiReq.TopP,
	}

	if openaiReq.MaxTokens != nil {
		anthropicReq.MaxTokens = *openaiReq.MaxTokens
	}

	if len(openaiReq.Stop) > 0 {
		anthropicReq.Stop = openaiReq.Stop
	}

	// Convert messages
	var systemPrompt string
	anthropicReq.Messages = make([]Message, 0, len(openaiReq.Messages))

	for _, msg := range openaiReq.Messages {
		// System messages go to top-level system field
		if msg.Role == "system" {
			systemPrompt += msg.Content + "\n"
			continue
		}

		anthropicReq.Messages = append(anthropicReq.Messages, Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	anthropicReq.System = systemPrompt

	return anthropicReq
}

// AnthropicToOpenAIRequest converts Anthropic messages request to OpenAI chat completion request
func AnthropicToOpenAIRequest(anthropicReq *MessagesRequest) *openai.ChatCompletionRequest {
	openaiReq := &openai.ChatCompletionRequest{
		Model:       anthropicReq.Model,
		Stream:      anthropicReq.Stream,
		Temperature: anthropicReq.Temperature,
		TopP:        anthropicReq.TopP,
	}

	if anthropicReq.MaxTokens > 0 {
		openaiReq.MaxTokens = &anthropicReq.MaxTokens
	}

	if len(anthropicReq.Stop) > 0 {
		openaiReq.Stop = anthropicReq.Stop
	}

	// Convert messages
	openaiReq.Messages = make([]openai.ChatCompletionMessage, 0, len(anthropicReq.Messages)+1)

	// Add system message if present
	if anthropicReq.System != "" {
		openaiReq.Messages = append(openaiReq.Messages, openai.ChatCompletionMessage{
			Role:    "system",
			Content: anthropicReq.System,
		})
	}

	for _, msg := range anthropicReq.Messages {
		openaiReq.Messages = append(openaiReq.Messages, openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: extractTextContent(msg.Content),
		})
	}

	return openaiReq
}

// OpenAIToAnthropicResponse converts OpenAI chat completion response to Anthropic messages response
func OpenAIToAnthropicResponse(openaiResp *openai.ChatCompletionResponse) *MessagesResponse {
	if openaiResp == nil || len(openaiResp.Choices) == 0 {
		return &MessagesResponse{
			ID:      "",
			Type:    "message",
			Role:    "assistant",
			Content: []ContentBlock{},
			Model:   "",
		}
	}

	choice := openaiResp.Choices[0]
	content := []ContentBlock{}

	if choice.Message != nil && choice.Message.Content != "" {
		content = append(content, ContentBlock{
			Type: "text",
			Text: choice.Message.Content,
		})
	}

	stopReason := ""
	if choice.FinishReason == "stop" {
		stopReason = "end_turn"
	} else if choice.FinishReason != "" {
		stopReason = choice.FinishReason
	}

	usage := Usage{}
	if openaiResp.Usage != nil {
		usage.InputTokens = openaiResp.Usage.PromptTokens
		usage.OutputTokens = openaiResp.Usage.CompletionTokens
	}

	return &MessagesResponse{
		ID:         openaiResp.ID,
		Type:       "message",
		Role:       "assistant",
		Content:    content,
		Model:      openaiResp.Model,
		StopReason: stopReason,
		Usage:      usage,
	}
}

// AnthropicToOpenAIResponse converts Anthropic messages response to OpenAI chat completion response
func AnthropicToOpenAIResponse(anthropicResp *MessagesResponse) *openai.ChatCompletionResponse {
	content := ""
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	finishReason := "stop"
	if anthropicResp.StopReason != "" && anthropicResp.StopReason != "end_turn" {
		finishReason = anthropicResp.StopReason
	}

	return &openai.ChatCompletionResponse{
		ID:      anthropicResp.ID,
		Object:  "chat.completion",
		Created: 0,
		Model:   anthropicResp.Model,
		Choices: []openai.ChatCompletionChoice{
			{
				Index: 0,
				Message: &openai.ChatCompletionMessage{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: finishReason,
			},
		},
		Usage: &openai.Usage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		},
	}
}

// Helper function to extract text content from various content formats
func extractTextContent(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []interface{}:
		var text string
		for _, block := range c {
			if m, ok := block.(map[string]interface{}); ok {
				if t, ok := m["text"].(string); ok {
					text += t
				}
			}
		}
		return text
	default:
		return fmt.Sprintf("%v", content)
	}
}

// ConvertAnthropicStreamToOpenAI converts Anthropic SSE stream to OpenAI format
func ConvertAnthropicStreamToOpenAI(line string, model string, id string) string {
	// Parse Anthropic SSE event
	if !isAnthropicSSE(line) {
		return line
	}

	// Extract event data
	data := extractSSEData(line)
	if data == "" {
		return line
	}

	var event map[string]interface{}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return line
	}

	eventType, _ := event["type"].(string)

	switch eventType {
	case "message_start":
		// Initial message - create first chunk with role
		return fmt.Sprintf("data: {\"id\":\"%s\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"%s\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":null}]}\n\n", id, model)

	case "content_block_start":
		// Start of content block
		return "" // No equivalent in OpenAI streaming

	case "content_block_delta":
		// Text delta
		if delta, ok := event["delta"].(map[string]interface{}); ok {
			if text, ok := delta["text"].(string); ok {
				escaped, _ := json.Marshal(text)
				return fmt.Sprintf("data: {\"id\":\"%s\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"%s\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%s},\"finish_reason\":null}]}\n\n", id, model, string(escaped))
			}
		}

	case "content_block_stop":
		return "" // No equivalent

	case "message_delta":
		// Stop reason
		return "" // Will be handled in message_stop

	case "message_stop":
		return fmt.Sprintf("data: {\"id\":\"%s\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"%s\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", id, model)

	default:
		return line
	}

	return ""
}

// ConvertOpenAIStreamToAnthropic converts OpenAI SSE stream to Anthropic format
func ConvertOpenAIStreamToAnthropic(line string, model string, id string, isFirst *bool, blockIdx *int) string {
	data := extractSSEData(line)
	if data == "" || data == "[DONE]" {
		if data == "[DONE]" {
			return fmt.Sprintf("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		}
		return line
	}

	var chunk openai.ChatCompletionChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return line
	}

	if len(chunk.Choices) == 0 {
		return ""
	}

	choice := chunk.Choices[0]
	var events []string

	// First chunk - send message_start
	if *isFirst {
		*isFirst = false
		msgStart := fmt.Sprintf(`{"type":"message_start","message":{"id":"%s","type":"message","role":"assistant","content":[],"model":"%s","usage":{"input_tokens":0,"output_tokens":0}}}`, id, model)
		events = append(events, fmt.Sprintf("event: message_start\ndata: %s\n\n", msgStart))

		// Start first content block
		events = append(events, fmt.Sprintf("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
		*blockIdx = 0
	}

	// Content delta
	if choice.Delta.Content != "" {
		escaped, _ := json.Marshal(choice.Delta.Content)
		events = append(events, fmt.Sprintf("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":%d,\"delta\":{\"type\":\"text_delta\",\"text\":%s}}\n\n", *blockIdx, string(escaped)))
	}

	// Finish reason - end message
	if choice.FinishReason != nil && *choice.FinishReason != "" {
		events = append(events, fmt.Sprintf("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", *blockIdx))
		events = append(events, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}

	return joinEvents(events)
}

func isAnthropicSSE(line string) bool {
	return len(line) > 6 && (line[:6] == "event:" || line[:5] == "data:")
}

func extractSSEData(line string) string {
	// Handle "data: {...}" format
	if len(line) > 6 && line[:5] == "data:" {
		return line[6:]
	}
	return line
}

func joinEvents(events []string) string {
	result := ""
	for _, e := range events {
		if e != "" {
			result += e
		}
	}
	return result
}
