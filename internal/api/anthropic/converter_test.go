package anthropic

import (
	"testing"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/stretchr/testify/assert"
)

func TestOpenAIToAnthropicRequest(t *testing.T) {
	temp := float64(0.7)
	maxTokens := 100

	openaiReq := &openai.ChatCompletionRequest{
		Model:       "gpt-4",
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		Stream:      false,
		Messages: []openai.ChatCompletionMessage{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
		},
	}

	anthropicReq := OpenAIToAnthropicRequest(openaiReq)

	assert.Equal(t, "gpt-4", anthropicReq.Model)
	assert.Equal(t, 100, anthropicReq.MaxTokens)
	assert.Equal(t, &temp, anthropicReq.Temperature)
	assert.Equal(t, "You are helpful.\n", anthropicReq.System)
	assert.Len(t, anthropicReq.Messages, 2)
	assert.Equal(t, "user", anthropicReq.Messages[0].Role)
	assert.Equal(t, "Hello", anthropicReq.Messages[0].Content)
}

func TestAnthropicToOpenAIRequest(t *testing.T) {
	temp := float64(0.5)

	anthropicReq := &MessagesRequest{
		Model:       "claude-3-opus",
		MaxTokens:   200,
		Temperature: &temp,
		System:      "Be helpful",
		Messages: []Message{
			{Role: "user", Content: "Hi"},
		},
	}

	openaiReq := AnthropicToOpenAIRequest(anthropicReq)

	assert.Equal(t, "claude-3-opus", openaiReq.Model)
	assert.Equal(t, 200, *openaiReq.MaxTokens)
	assert.Equal(t, &temp, openaiReq.Temperature)
	assert.Len(t, openaiReq.Messages, 2)
	assert.Equal(t, "system", openaiReq.Messages[0].Role)
	assert.Equal(t, "Be helpful", openaiReq.Messages[0].Content)
	assert.Equal(t, "user", openaiReq.Messages[1].Role)
}

func TestOpenAIToAnthropicResponse(t *testing.T) {
	openaiResp := &openai.ChatCompletionResponse{
		ID:    "chatcmpl-123",
		Model: "gpt-4",
		Choices: []openai.ChatCompletionChoice{
			{
				Index:        0,
				Message:      &openai.ChatCompletionMessage{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			},
		},
		Usage: &openai.Usage{PromptTokens: 10, CompletionTokens: 5},
	}

	anthropicResp := OpenAIToAnthropicResponse(openaiResp)

	assert.Equal(t, "chatcmpl-123", anthropicResp.ID)
	assert.Equal(t, "message", anthropicResp.Type)
	assert.Equal(t, "assistant", anthropicResp.Role)
	assert.Len(t, anthropicResp.Content, 1)
	assert.Equal(t, "text", anthropicResp.Content[0].Type)
	assert.Equal(t, "Hello!", anthropicResp.Content[0].Text)
	assert.Equal(t, "end_turn", anthropicResp.StopReason)
}

func TestAnthropicToOpenAIResponse(t *testing.T) {
	anthropicResp := &MessagesResponse{
		ID:      "msg-123",
		Model:   "claude-3-opus",
		Role:    "assistant",
		Content: []ContentBlock{{Type: "text", Text: "Hi there!"}},
		Usage:   Usage{InputTokens: 15, OutputTokens: 10},
	}

	openaiResp := AnthropicToOpenAIResponse(anthropicResp)

	assert.Equal(t, "msg-123", openaiResp.ID)
	assert.Equal(t, "chat.completion", openaiResp.Object)
	assert.Equal(t, "claude-3-opus", openaiResp.Model)
	assert.Len(t, openaiResp.Choices, 1)
	assert.Equal(t, "assistant", openaiResp.Choices[0].Message.Role)
	assert.Equal(t, "Hi there!", openaiResp.Choices[0].Message.Content)
	assert.Equal(t, 15, openaiResp.Usage.PromptTokens)
	assert.Equal(t, 10, openaiResp.Usage.CompletionTokens)
}

func TestOpenAIToAnthropicResponseEmpty(t *testing.T) {
	openaiResp := &openai.ChatCompletionResponse{
		ID:      "chatcmpl-empty",
		Model:   "gpt-4",
		Choices: []openai.ChatCompletionChoice{},
	}

	anthropicResp := OpenAIToAnthropicResponse(openaiResp)

	// When choices are empty, the function returns empty defaults
	assert.Equal(t, "", anthropicResp.ID)
	assert.Equal(t, "message", anthropicResp.Type)
	assert.Equal(t, "assistant", anthropicResp.Role)
	assert.Len(t, anthropicResp.Content, 0)
}

func TestAnthropicToOpenAIResponseWithStopReason(t *testing.T) {
	anthropicResp := &MessagesResponse{
		ID:         "msg-456",
		Model:      "claude-3-sonnet",
		Role:       "assistant",
		Content:    []ContentBlock{{Type: "text", Text: "Done"}},
		StopReason: "max_tokens",
		Usage:      Usage{InputTokens: 10, OutputTokens: 20},
	}

	openaiResp := AnthropicToOpenAIResponse(anthropicResp)

	assert.Equal(t, "max_tokens", openaiResp.Choices[0].FinishReason)
}

func TestExtractTextContent(t *testing.T) {
	// Test string content
	assert.Equal(t, "hello", extractTextContent("hello"))

	// Test array content
	arrContent := []interface{}{
		map[string]interface{}{"type": "text", "text": "Hello "},
		map[string]interface{}{"type": "text", "text": "World"},
	}
	assert.Equal(t, "Hello World", extractTextContent(arrContent))

	// Test other types
	assert.Equal(t, "123", extractTextContent(123))
}

func TestConvertAnthropicStreamToOpenAI(t *testing.T) {
	// Test message_start event - use data: format
	line := `data: {"type":"message_start","message":{"id":"msg-123","type":"message","role":"assistant","content":[],"model":"claude-3-opus"}}`

	result := ConvertAnthropicStreamToOpenAI(line, "gpt-4", "test-id")
	assert.Contains(t, result, `"role":"assistant"`)
	assert.Contains(t, result, `"model":"gpt-4"`)

	// Test content_block_delta event
	deltaLine := `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`

	result = ConvertAnthropicStreamToOpenAI(deltaLine, "gpt-4", "test-id")
	assert.Contains(t, result, `"content":"Hello"`)

	// Test message_stop event
	stopLine := `data: {"type":"message_stop"}`

	result = ConvertAnthropicStreamToOpenAI(stopLine, "gpt-4", "test-id")
	assert.Contains(t, result, `"finish_reason":"stop"`)
	assert.Contains(t, result, "[DONE]")
}

func TestConvertOpenAIStreamToAnthropic(t *testing.T) {
	isFirst := true
	blockIdx := 0

	// First chunk - should emit message_start and content_block_start
	firstChunk := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":0,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`

	result := ConvertOpenAIStreamToAnthropic(firstChunk, "claude-3-opus", "msg-123", &isFirst, &blockIdx)
	assert.Contains(t, result, `"type":"message_start"`)
	assert.Contains(t, result, `"type":"content_block_start"`)
	assert.False(t, isFirst) // isFirst should be false now

	// Content delta
	deltaChunk := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":0,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`

	result = ConvertOpenAIStreamToAnthropic(deltaChunk, "claude-3-opus", "msg-123", &isFirst, &blockIdx)
	assert.Contains(t, result, `"type":"content_block_delta"`)
	assert.Contains(t, result, `"text":"Hello"`)

	// Done marker
	doneLine := "data: [DONE]"
	result = ConvertOpenAIStreamToAnthropic(doneLine, "claude-3-opus", "msg-123", &isFirst, &blockIdx)
	assert.Contains(t, result, `"type":"message_stop"`)
}

func TestIsAnthropicSSE(t *testing.T) {
	assert.True(t, isAnthropicSSE("event: message_start"))
	assert.True(t, isAnthropicSSE("data: {\"type\":\"message_start\"}"))
	assert.False(t, isAnthropicSSE(""))
	assert.False(t, isAnthropicSSE("invalid"))
}

func TestExtractSSEData(t *testing.T) {
	// extractSSEData strips "data:" (5 chars check) and returns line[6:] (6 chars stripped)
	// So "data: {\"test\":true}" -> "{\"test\":true}"
	assert.Equal(t, "{\"test\":true}", extractSSEData("data: {\"test\":true}"))
	assert.Equal(t, "test", extractSSEData("data: test"))
	// Non-data lines are returned unchanged
	assert.Equal(t, "event: test", extractSSEData("event: test"))
}