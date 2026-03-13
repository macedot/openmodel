// Package anthropic provides validation tests for Anthropic API requests
package anthropic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateMessagesRequest(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		expectError bool
		errContains string
	}{
		{
			name: "valid request",
			data: `{
				"model": "claude-3-opus",
				"messages": [{"role": "user", "content": "Hello"}],
				"max_tokens": 1024
			}`,
			expectError: false,
		},
		{
			name: "missing model",
			data: `{
				"messages": [{"role": "user", "content": "Hello"}]
			}`,
			expectError: true,
			errContains: "model is required",
		},
		{
			name: "empty model",
			data: `{
				"model": "",
				"messages": [{"role": "user", "content": "Hello"}]
			}`,
			expectError: true,
			errContains: "model is required",
		},
		{
			name: "missing messages",
			data: `{
				"model": "claude-3-opus"
			}`,
			expectError: true,
			errContains: "messages is required",
		},
		{
			name: "empty messages",
			data: `{
				"model": "claude-3-opus",
				"messages": []
			}`,
			expectError: true,
			errContains: "messages is required",
		},
		{
			name:        "invalid JSON",
			data:        `{invalid}`,
			expectError: true,
			errContains: "invalid JSON",
		},
		{
			name: "message without role",
			data: `{
				"model": "claude-3-opus",
				"messages": [{"content": "Hello"}]
			}`,
			expectError: true,
			errContains: "role is required",
		},
		{
			name: "message with invalid role",
			data: `{
				"model": "claude-3-opus",
				"messages": [{"role": "system", "content": "Hello"}]
			}`,
			expectError: true,
			errContains: "role must be 'user' or 'assistant'",
		},
		{
			name: "message without content",
			data: `{
				"model": "claude-3-opus",
				"messages": [{"role": "user"}]
			}`,
			expectError: true,
			errContains: "content is required",
		},
		{
			name: "negative max_tokens",
			data: `{
				"model": "claude-3-opus",
				"messages": [{"role": "user", "content": "Hello"}],
				"max_tokens": -1
			}`,
			expectError: true,
			errContains: "max_tokens must be positive",
		},
		{
			name: "zero max_tokens",
			data: `{
				"model": "claude-3-opus",
				"messages": [{"role": "user", "content": "Hello"}],
				"max_tokens": 0
			}`,
			expectError: true,
			errContains: "max_tokens must be positive",
		},
		{
			name: "message as array instead of object",
			data: `{
				"model": "claude-3-opus",
				"messages": ["not an object"]
			}`,
			expectError: true,
			errContains: "must be an object",
		},
		{
			name: "assistant role",
			data: `{
				"model": "claude-3-opus",
				"messages": [{"role": "assistant", "content": "Hello"}]
			}`,
			expectError: false,
		},
		{
			name: "multiple messages",
			data: `{
				"model": "claude-3-opus",
				"messages": [
					{"role": "user", "content": "Hello"},
					{"role": "assistant", "content": "Hi there"},
					{"role": "user", "content": "How are you?"}
				]
			}`,
			expectError: false,
		},
		{
			name: "empty string content",
			data: `{
				"model": "claude-3-opus",
				"messages": [{"role": "user", "content": ""}]
			}`,
			expectError: false, // Empty content is allowed
		},
		{
			name: "content as array",
			data: `{
				"model": "claude-3-opus",
				"messages": [{"role": "user", "content": [{"type": "text", "text": "Hello"}]}]
			}`,
			expectError: false, // Content can be array for multimodal
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMessagesRequest([]byte(tt.data))
			if tt.expectError {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMessage(t *testing.T) {
	tests := []struct {
		name        string
		message     interface{}
		index       int
		expectError bool
		errContains string
	}{
		{
			name: "valid user message",
			message: map[string]interface{}{
				"role":    "user",
				"content": "Hello",
			},
			index:       0,
			expectError: false,
		},
		{
			name: "valid assistant message",
			message: map[string]interface{}{
				"role":    "assistant",
				"content": "Hi there",
			},
			index:       0,
			expectError: false,
		},
		{
			name:        "non-object message",
			message:     "not an object",
			index:       0,
			expectError: true,
			errContains: "must be an object",
		},
		{
			name: "missing role",
			message: map[string]interface{}{
				"content": "Hello",
			},
			index:       0,
			expectError: true,
			errContains: "role is required",
		},
		{
			name: "invalid role",
			message: map[string]interface{}{
				"role":    "system",
				"content": "Hello",
			},
			index:       0,
			expectError: true,
			errContains: "role must be 'user' or 'assistant'",
		},
		{
			name: "role as number",
			message: map[string]interface{}{
				"role":    123,
				"content": "Hello",
			},
			index:       0,
			expectError: true,
			errContains: "role is required",
		},
		{
			name: "missing content",
			message: map[string]interface{}{
				"role": "user",
			},
			index:       0,
			expectError: true,
			errContains: "content is required",
		},
		{
			name: "nil content",
			message: map[string]interface{}{
				"role":    "user",
				"content": nil,
			},
			index:       0,
			expectError: true,
			errContains: "content is required",
		},
		{
			name: "error includes message index",
			message: map[string]interface{}{
				"content": "Hello",
			},
			index:       5,
			expectError: true,
			errContains: "messages[5]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMessage(tt.message, tt.index)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
