// Package claude defines types for the Anthropic Claude Messages API
package claude

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContentBlock_JSON(t *testing.T) {
	tests := []struct {
		name     string
		block    ContentBlock
		expected string
	}{
		{
			name: "text block",
			block: ContentBlock{Type: "text", Text: "Hello"},
			expected: `{"type":"text","text":"Hello"}`,
		},
		{
			name: "image block",
			block: ContentBlock{
				Type: "image",
				Source: &Source{
					Type:      "base64",
					MediaType: "image/png",
					Data:      "abc123",
				},
			},
			expected: `{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc123"}}`,
		},
		{
			name: "tool_use block",
			block: ContentBlock{
				Type:  "tool_use",
				ID:    "tool_123",
				Name:  "get_weather",
				Input: map[string]any{"location": "SF"},
			},
			expected: `{"type":"tool_use","id":"tool_123","name":"get_weather","input":{"location":"SF"}}`,
		},
		{
			name: "tool_result block",
			block: ContentBlock{
				Type:      "tool_result",
				ToolUseID: "tool_123",
				Content:   "Sunny, 72°F",
			},
			expected: `{"type":"tool_result","tool_use_id":"tool_123","content":"Sunny, 72°F"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.block)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestSource_JSON(t *testing.T) {
	tests := []struct {
		name     string
		source   Source
		expected string
	}{
		{
			name:     "base64 source",
			source:   Source{Type: "base64", MediaType: "image/jpeg", Data: "xyz"},
			expected: `{"type":"base64","media_type":"image/jpeg","data":"xyz"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.source)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestMessage_JSON(t *testing.T) {
	tests := []struct {
		name     string
		message  Message
		expected string
	}{
		{
			name:     "user with string content",
			message:  Message{Role: "user", Content: "Hello"},
			expected: `{"role":"user","content":"Hello"}`,
		},
		{
			name: "user with array content",
			message: Message{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "Hello"},
				},
			},
			expected: `{"role":"user","content":[{"text":"Hello","type":"text"}]}`,
		},
		{
			name:     "assistant message",
			message:  Message{Role: "assistant", Content: "Hi there!"},
			expected: `{"role":"assistant","content":"Hi there!"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.message)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestMessage_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Message
	}{
		{
			name:     "string content",
			input:   `{"role":"user","content":"Hello"}`,
			expected: Message{Role: "user", Content: "Hello"},
		},
		{
			name:  "array content",
			input: `{"role":"user","content":[{"type":"text","text":"Hello"}]}`,
			expected: Message{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "Hello"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg Message
			err := json.Unmarshal([]byte(tt.input), &msg)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected.Role, msg.Role)

			// Compare content type
			switch expected := tt.expected.Content.(type) {
			case string:
				actual, ok := msg.Content.(string)
				assert.True(t, ok)
				assert.Equal(t, expected, actual)
			case []any:
				actual, ok := msg.Content.([]any)
				assert.True(t, ok)
				assert.Len(t, actual, len(expected))
			}
		})
	}
}

func TestRequest_JSON(t *testing.T) {
	temp := 0.7
	topP := 0.9

	tests := []struct {
		name     string
		request  Request
		expected string
	}{
		{
			name: "basic request",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
			},
			expected: `{"model":"claude-3-opus","messages":[{"role":"user","content":"Hello"}],"max_tokens":100}`,
		},
		{
			name: "request with optional fields",
			request: Request{
				Model:       "claude-3-sonnet",
				Messages:    []Message{{Role: "user", Content: "Hi"}},
				MaxTokens:   200,
				Temperature: &temp,
				TopP:        &topP,
				Stream:      true,
			},
			expected: `{"model":"claude-3-sonnet","messages":[{"role":"user","content":"Hi"}],"max_tokens":200,"temperature":0.7,"top_p":0.9,"stream":true}`,
		},
		{
			name: "request with system string",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
				System:    "You are helpful.",
			},
			expected: `{"model":"claude-3-opus","messages":[{"role":"user","content":"Hello"}],"max_tokens":100,"system":"You are helpful."}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.request)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestRequest_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Request
	}{
		{
			name:  "basic request",
			input: `{"model":"claude-3-opus","messages":[{"role":"user","content":"Hello"}],"max_tokens":100}`,
			expected: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
			},
		},
		{
			name:  "request with system string",
			input: `{"model":"claude-3-opus","messages":[{"role":"user","content":"Hello"}],"max_tokens":100,"system":"Be helpful"}`,
			expected: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
				System:    "Be helpful",
			},
		},
		{
			name:  "request with system array",
			input: `{"model":"claude-3-opus","messages":[{"role":"user","content":"Hello"}],"max_tokens":100,"system":[{"type":"text","text":"Be helpful"}]}`,
			expected: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
				System:    []any{map[string]any{"type": "text", "text": "Be helpful"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req Request
			err := json.Unmarshal([]byte(tt.input), &req)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected.Model, req.Model)
			assert.Equal(t, tt.expected.MaxTokens, req.MaxTokens)
			assert.Len(t, req.Messages, len(tt.expected.Messages))

			// Compare system
			switch expected := tt.expected.System.(type) {
			case string:
				actual, ok := req.System.(string)
				assert.True(t, ok)
				assert.Equal(t, expected, actual)
			case []any:
				actual, ok := req.System.([]any)
				assert.True(t, ok)
				assert.NotNil(t, actual)
			case nil:
				assert.Nil(t, req.System)
			}
		})
	}
}

func TestResponse_JSON(t *testing.T) {
	tests := []struct {
		name     string
		response Response
		expected string
	}{
		{
			name: "basic response",
			response: Response{
				ID:      "msg_123",
				Type:    "message",
				Role:    "assistant",
				Content: []ContentBlock{{Type: "text", Text: "Hello!"}},
				Model:   "claude-3-opus",
				Usage:   Usage{InputTokens: 10, OutputTokens: 5},
			},
			expected: `{"id":"msg_123","type":"message","role":"assistant","content":[{"type":"text","text":"Hello!"}],"model":"claude-3-opus","stop_reason":"","usage":{"input_tokens":10,"output_tokens":5}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.response)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestStreamEvent_JSON(t *testing.T) {
	tests := []struct {
		name     string
		event    StreamEvent
		expected string
	}{
		{
			name:     "message_start event",
			event:    StreamEvent{Type: "message_start"},
			expected: `{"type":"message_start"}`,
		},
		{
			name: "content_block_delta event with index",
			event: StreamEvent{
				Type:  "content_block_delta",
				Index: 1,
				ContentBlock: &ContentBlock{Type: "text", Text: "Hello"},
			},
			expected: `{"type":"content_block_delta","index":1,"content_block":{"type":"text","text":"Hello"}}`,
		},
		{
			name: "content_block_delta event index 0 omitted",
			event: StreamEvent{
				Type:  "content_block_delta",
				Index: 0,
				ContentBlock: &ContentBlock{Type: "text", Text: "Hello"},
			},
			// Index 0 is omitted due to omitempty
			expected: `{"type":"content_block_delta","content_block":{"type":"text","text":"Hello"}}`,
		},
		{
			name: "message_delta event",
			event: StreamEvent{
				Type: "message_delta",
				Usage: &Usage{OutputTokens: 10},
			},
			expected: `{"type":"message_delta","usage":{"input_tokens":0,"output_tokens":10}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.event)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestTool_JSON(t *testing.T) {
	tool := Tool{
		Name:        "get_weather",
		Description: "Get weather info",
		InputSchema: map[string]any{"type": "object"},
	}

	data, err := json.Marshal(tool)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"name":"get_weather","description":"Get weather info","input_schema":{"type":"object"}}`, string(data))
}

func TestThinking_JSON(t *testing.T) {
	tests := []struct {
		name     string
		thinking Thinking
		expected string
	}{
		{
			name:     "enabled thinking",
			thinking: Thinking{Type: "enabled", BudgetTokens: 1000},
			expected: `{"type":"enabled","budget_tokens":1000}`,
		},
		{
			name:     "auto thinking",
			thinking: Thinking{Type: "auto"},
			expected: `{"type":"auto"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.thinking)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestUsage_JSON(t *testing.T) {
	usage := Usage{
		InputTokens:              100,
		OutputTokens:             50,
		CacheCreationInputTokens: 10,
		CacheReadInputTokens:     5,
	}

	data, err := json.Marshal(usage)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":5}`, string(data))
}

func TestNewResponse(t *testing.T) {
	resp := NewResponse("claude-3-opus")
	assert.Equal(t, "message", resp.Type)
	assert.Equal(t, "assistant", resp.Role)
	assert.Equal(t, "claude-3-opus", resp.Model)
	assert.NotEmpty(t, resp.ID)
	assert.True(t, len(resp.ID) > 0)
}

func TestNewStreamEvent(t *testing.T) {
	event := NewStreamEvent("message_start")
	assert.Equal(t, "message_start", event.Type)
}

func TestErrorResponse_JSON(t *testing.T) {
	errResp := ErrorResponse{
		Type:    "error",
		Message: "Invalid request",
	}

	data, err := json.Marshal(errResp)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"type":"error","message":"Invalid request"}`, string(data))
}