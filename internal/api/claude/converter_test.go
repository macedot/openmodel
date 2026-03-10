// Package claude defines types for the Anthropic Claude Messages API
package claude

import (
	"testing"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/stretchr/testify/assert"
)

func TestToOpenAIRequest(t *testing.T) {
	tests := []struct {
		name     string
		input    *Request
		expected *openai.ChatCompletionRequest
	}{
		{
			name: "basic conversion",
			input: &Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
			},
			expected: &openai.ChatCompletionRequest{
				Model:     "claude-3-opus",
				Messages:  []openai.ChatCompletionMessage{{Role: "user", Content: "Hello"}},
				MaxTokens: intPtr(100),
				Stream:    false,
			},
		},
		{
			name: "with temperature and top_p",
			input: &Request{
				Model:       "claude-3-sonnet",
				Messages:    []Message{{Role: "user", Content: "Hi"}},
				MaxTokens:   200,
				Temperature: floatPtr(0.7),
				TopP:        floatPtr(0.9),
			},
			expected: &openai.ChatCompletionRequest{
				Model:       "claude-3-sonnet",
				Messages:    []openai.ChatCompletionMessage{{Role: "user", Content: "Hi"}},
				MaxTokens:   intPtr(200),
				Temperature: floatPtr(0.7),
				TopP:        floatPtr(0.9),
				Stream:      false,
			},
		},
		{
			name: "with system message",
			input: &Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
				System:    "You are helpful.",
			},
			expected: &openai.ChatCompletionRequest{
				Model:     "claude-3-opus",
				Messages:  []openai.ChatCompletionMessage{{Role: "system", Content: "You are helpful."}, {Role: "user", Content: "Hello"}},
				MaxTokens: intPtr(100),
				Stream:    false,
			},
		},
		{
			name: "with tools",
			input: &Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "What's the weather?"}},
				MaxTokens: 100,
				Tools: []Tool{
					{Name: "get_weather", Description: "Get weather", InputSchema: map[string]any{"type": "object"}},
				},
			},
			expected: &openai.ChatCompletionRequest{
				Model:     "claude-3-opus",
				Messages:  []openai.ChatCompletionMessage{{Role: "user", Content: "What's the weather?"}},
				MaxTokens: intPtr(100),
				Stream:    false,
				Tools: []openai.Tool{
					{Type: "function", Function: openai.ToolFunction{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"}}},
				},
			},
		},
		{
			name: "assistant message",
			input: &Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "assistant", Content: "Hello!"}},
				MaxTokens: 100,
			},
			expected: &openai.ChatCompletionRequest{
				Model:     "claude-3-opus",
				Messages:  []openai.ChatCompletionMessage{{Role: "assistant", Content: "Hello!"}},
				MaxTokens: intPtr(100),
				Stream:    false,
			},
		},
		{
			name: "array content flattening",
			input: &Request{
				Model: "claude-3-opus",
				Messages: []Message{
					{
						Role: "user",
						Content: []any{
							map[string]any{"type": "text", "text": "Hello"},
							map[string]any{"type": "text", "text": " World"},
						},
					},
				},
				MaxTokens: 100,
			},
			expected: &openai.ChatCompletionRequest{
				Model:     "claude-3-opus",
				Messages:  []openai.ChatCompletionMessage{{Role: "user", Content: "Hello World"}},
				MaxTokens: intPtr(100),
				Stream:    false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToOpenAIRequest(tt.input)
			assert.Equal(t, tt.expected.Model, result.Model)
			assert.Equal(t, tt.expected.MaxTokens, result.MaxTokens)
			assert.Equal(t, tt.expected.Temperature, result.Temperature)
			assert.Equal(t, tt.expected.TopP, result.TopP)
			assert.Equal(t, tt.expected.Stream, result.Stream)
			assert.Len(t, result.Messages, len(tt.expected.Messages))
			for i, msg := range result.Messages {
				assert.Equal(t, tt.expected.Messages[i].Role, msg.Role)
				assert.Equal(t, tt.expected.Messages[i].Content, msg.Content)
			}
			if len(tt.expected.Tools) > 0 {
				assert.Len(t, result.Tools, len(tt.expected.Tools))
			}
		})
	}
}

func TestToOpenAIRequestWithStream(t *testing.T) {
	req := &Request{
		Model:     "claude-3-opus",
		Messages:  []Message{{Role: "user", Content: "Hello"}},
		MaxTokens: 100,
		Stream:    true,
	}

	result := ToOpenAIRequestWithStream(req)
	assert.Equal(t, true, result.Stream)
}

func TestToClaudeResponse(t *testing.T) {
	tests := []struct {
		name         string
		openaiResp   *openai.ChatCompletionResponse
		model        string
		expectedType string
		expectedRole string
		expectedStop string
	}{
		{
			name: "basic response",
			openaiResp: &openai.ChatCompletionResponse{
				ID: "chatcmpl-123",
				Choices: []openai.ChatCompletionChoice{
					{
						Message: &openai.ChatCompletionMessage{
							Role:    "assistant",
							Content: "Hello!",
						},
						FinishReason: "stop",
					},
				},
			},
			model:        "claude-3-opus",
			expectedType: "message",
			expectedRole: "assistant",
			expectedStop: "end_turn",
		},
		{
			name: "response with usage",
			openaiResp: &openai.ChatCompletionResponse{
				ID: "chatcmpl-123",
				Choices: []openai.ChatCompletionChoice{
					{
						Message: &openai.ChatCompletionMessage{
							Role:    "assistant",
							Content: "Hello!",
						},
						FinishReason: "length",
					},
				},
				Usage: &openai.Usage{
					PromptTokens:     100,
					CompletionTokens: 50,
				},
			},
			model:        "claude-3-sonnet",
			expectedType: "message",
			expectedRole: "assistant",
			expectedStop: "max_tokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToClaudeResponse(tt.openaiResp, tt.model)

			assert.Equal(t, tt.expectedType, result.Type)
			assert.Equal(t, tt.expectedRole, result.Role)
			assert.Equal(t, tt.model, result.Model)
			assert.Equal(t, tt.expectedStop, result.StopReason)

			if tt.openaiResp.Usage != nil {
				assert.Equal(t, tt.openaiResp.Usage.PromptTokens, result.Usage.InputTokens)
				assert.Equal(t, tt.openaiResp.Usage.CompletionTokens, result.Usage.OutputTokens)
			}

			if len(tt.openaiResp.Choices) > 0 && tt.openaiResp.Choices[0].Message != nil {
				assert.Len(t, result.Content, 1)
				assert.Equal(t, "text", result.Content[0].Type)
				assert.Equal(t, tt.openaiResp.Choices[0].Message.Content, result.Content[0].Text)
			}
		})
	}
}

func TestFlattenContent(t *testing.T) {
	tests := []struct {
		name     string
		content  interface{}
		expected string
	}{
		{
			name:     "string content",
			content:  "Hello, World!",
			expected: "Hello, World!",
		},
		{
			name:     "empty string",
			content:  "",
			expected: "",
		},
		{
			name:     "nil content",
			content:  nil,
			expected: "",
		},
		{
			name: "array with text blocks",
			content: []any{
				map[string]any{"type": "text", "text": "Hello"},
				map[string]any{"type": "text", "text": " "},
				map[string]any{"type": "text", "text": "World"},
			},
			expected: "Hello World",
		},
		{
			name: "array with mixed blocks",
			content: []any{
				map[string]any{"type": "text", "text": "Hello "},
				map[string]any{"type": "image", "source": map[string]any{"type": "base64"}},
				map[string]any{"type": "text", "text": "World"},
			},
			expected: "Hello World",
		},
		{
			name:     "empty array",
			content:  []any{},
			expected: "",
		},
		{
			name:     "invalid type",
			content:  123,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := flattenContent(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFlattenContentBlocks(t *testing.T) {
	tests := []struct {
		name     string
		blocks   []ContentBlock
		expected string
	}{
		{
			name: "text blocks only",
			blocks: []ContentBlock{
				{Type: "text", Text: "Hello"},
				{Type: "text", Text: " "},
				{Type: "text", Text: "World"},
			},
			expected: "Hello World",
		},
		{
			name: "mixed blocks",
			blocks: []ContentBlock{
				{Type: "text", Text: "Hello "},
				{Type: "tool_use", Name: "get_weather"},
				{Type: "text", Text: " World"},
			},
			expected: "Hello [tool_use: get_weather] World",
		},
		{
			name: "tool_result block",
			blocks: []ContentBlock{
				{Type: "tool_result", Content: "result"},
			},
			expected: "[tool_result]",
		},
		{
			name:     "empty blocks",
			blocks:   []ContentBlock{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := flattenContentBlocks(tt.blocks)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertTools(t *testing.T) {
	tools := []Tool{
		{
			Name:        "get_weather",
			Description: "Get the current weather",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"location": map[string]any{"type": "string"}}},
		},
	}

	result := convertTools(tools)
	assert.Len(t, result, 1)
	assert.Equal(t, "function", result[0].Type)
	assert.Equal(t, "get_weather", result[0].Function.Name)
	assert.Equal(t, "Get the current weather", result[0].Function.Description)
}

func TestConvertMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		expected []openai.ChatCompletionMessage
	}{
		{
			name: "user and assistant",
			messages: []Message{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
			},
			expected: []openai.ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
			},
		},
		{
			name: "with array content",
			messages: []Message{
				{Role: "user", Content: []any{map[string]any{"type": "text", "text": "Hello"}}},
			},
			expected: []openai.ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertMessages(tt.messages)
			assert.Len(t, result, len(tt.expected))
			for i, msg := range result {
				assert.Equal(t, tt.expected[i].Role, msg.Role)
				assert.Equal(t, tt.expected[i].Content, msg.Content)
			}
		})
	}
}

func TestToClaudeStreamEvent(t *testing.T) {
	finishReason := "stop"
	tests := []struct {
		name           string
		chunk          *openai.ChatCompletionChunk
		index          int
		expectedType   string
		expectedText   string
		expectedIndex  int
		expectFinished bool
	}{
		{
			name: "content delta",
			chunk: &openai.ChatCompletionChunk{
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Delta: openai.ChatCompletionDelta{Content: "Hello"},
					},
				},
			},
			index:         0,
			expectedType:  "content_block_delta",
			expectedText:  "Hello",
			expectedIndex: 0,
		},
		{
			name: "with thinking",
			chunk: &openai.ChatCompletionChunk{
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Delta: openai.ChatCompletionDelta{Thinking: "Let me think..."},
					},
				},
			},
			index:         0,
			expectedType:  "content_block_delta",
			expectedIndex: 0,
		},
		{
			name: "finish reason with content returns message_delta",
			chunk: &openai.ChatCompletionChunk{
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Delta:         openai.ChatCompletionDelta{Content: "done"},
						FinishReason:  &finishReason,
					},
				},
			},
			index:          0,
			expectedType:   "message_delta",
			expectFinished: true,
		},
		{
			name: "empty delta no finish returns content_block_delta",
			chunk: &openai.ChatCompletionChunk{
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Delta: openai.ChatCompletionDelta{},
					},
				},
			},
			index:         0,
			expectedType:  "content_block_delta",
			expectedIndex: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToClaudeStreamEvent(tt.chunk, tt.index)
			assert.Equal(t, tt.expectedType, result.Type)

			if !tt.expectFinished {
				assert.Equal(t, tt.expectedIndex, result.Index)
				if tt.expectedText != "" {
					assert.Equal(t, tt.expectedText, result.ContentBlock.Text)
				}
			} else {
				assert.NotNil(t, result.Usage)
			}
		})
	}
}

func TestToClaudeStreamStart(t *testing.T) {
	result := ToClaudeStreamStart("claude-3-opus")
	assert.Equal(t, "message_start", result.Type)
	assert.Equal(t, "message", result.Message.Type)
	assert.Equal(t, "assistant", result.Message.Role)
	assert.Equal(t, "claude-3-opus", result.Message.Model)
}

func TestToClaudeContentBlockStart(t *testing.T) {
	result := ToClaudeContentBlockStart(0)
	assert.Equal(t, "content_block_start", result.Type)
	assert.Equal(t, 0, result.Index)
	assert.Equal(t, "text", result.ContentBlock.Type)
}

func TestToClaudeContentBlockStop(t *testing.T) {
	result := ToClaudeContentBlockStop(1)
	assert.Equal(t, "content_block_stop", result.Type)
	assert.Equal(t, 1, result.Index)
}

func TestToClaudeMessageStop(t *testing.T) {
	result := ToClaudeMessageStop()
	assert.Equal(t, "message_stop", result.Type)
}

// Helper functions
func floatPtr(f float64) *float64 {
	return &f
}