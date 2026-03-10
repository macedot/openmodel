// Package claude defines types for the Anthropic Claude Messages API
package claude

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateMessageRequest(t *testing.T) {
	tests := []struct {
		name       string
		request    Request
		wantErr    bool
		errContain string
	}{
		{
			name: "valid request",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
			},
			wantErr: false,
		},
		{
			name: "missing model",
			request: Request{
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
			},
			wantErr:    true,
			errContain: ErrModelRequired.Error(),
		},
		{
			name: "missing messages",
			request: Request{
				Model:     "claude-3-opus",
				MaxTokens: 100,
			},
			wantErr:    true,
			errContain: ErrMessagesRequired.Error(),
		},
		{
			name: "empty messages",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{},
				MaxTokens: 100,
			},
			wantErr:    true,
			errContain: ErrMessagesRequired.Error(),
		},
		{
			name: "invalid max_tokens zero",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 0,
			},
			wantErr:    true,
			errContain: ErrMaxTokensInvalid.Error(),
		},
		{
			name: "invalid max_tokens negative",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: -10,
			},
			wantErr:    true,
			errContain: ErrMaxTokensInvalid.Error(),
		},
		{
			name: "invalid role",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "system", Content: "Hello"}},
				MaxTokens: 100,
			},
			wantErr:    true,
			errContain: ErrInvalidRole.Error(),
		},
		{
			name: "empty content string for user",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: ""}},
				MaxTokens: 100,
			},
			wantErr:    true,
			errContain: ErrEmptyContent.Error(),
		},
		{
			name: "empty content string for assistant allowed",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "assistant", Content: ""}},
				MaxTokens: 100,
			},
			wantErr: false,
		},
		{
			name: "nil content for user",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: nil}},
				MaxTokens: 100,
			},
			wantErr:    true,
			errContain: ErrEmptyContent.Error(),
		},
		{
			name: "empty array content for user",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: []any{}}},
				MaxTokens: 100,
			},
			wantErr:    true,
			errContain: ErrEmptyContent.Error(),
		},
		{
			name: "valid array content",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: []any{map[string]any{"type": "text", "text": "Hello"}}}},
				MaxTokens: 100,
			},
			wantErr: false,
		},
		{
			name: "valid with system string",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
				System:    "You are helpful.",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMessageRequest(&tt.request)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContain != "" {
					assert.Contains(t, err.Error(), tt.errContain)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMessage(t *testing.T) {
	tests := []struct {
		name    string
		message Message
		wantErr error
	}{
		{
			name:    "valid user message with string",
			message: Message{Role: "user", Content: "Hello"},
			wantErr: nil,
		},
		{
			name:    "valid assistant message",
			message: Message{Role: "assistant", Content: "Hi!"},
			wantErr: nil,
		},
		{
			name:    "invalid role",
			message: Message{Role: "system", Content: "You are helpful"},
			wantErr: ErrInvalidRole,
		},
		{
			name:    "empty string content for user",
			message: Message{Role: "user", Content: ""},
			wantErr: ErrEmptyContent,
		},
		{
			name:    "empty string content for assistant (allowed)",
			message: Message{Role: "assistant", Content: ""},
			wantErr: nil,
		},
		{
			name:    "nil content",
			message: Message{Role: "user", Content: nil},
			wantErr: ErrEmptyContent,
		},
		{
			name:    "empty array content",
			message: Message{Role: "user", Content: []any{}},
			wantErr: ErrEmptyContent,
		},
		{
			name:    "valid array content",
			message: Message{Role: "user", Content: []any{map[string]any{"type": "text", "text": "Hello"}}},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMessage(&tt.message)
			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateContentBlock(t *testing.T) {
	tests := []struct {
		name    string
		block   ContentBlock
		wantErr bool
	}{
		{
			name:    "valid text block",
			block:   ContentBlock{Type: "text", Text: "Hello"},
			wantErr: false,
		},
		{
			name: "valid image block",
			block: ContentBlock{
				Type: "image",
				Source: &Source{
					Type:      "base64",
					MediaType: "image/png",
					Data:      "abc123",
				},
			},
			wantErr: false,
		},
		{
			name:    "image block missing source",
			block:   ContentBlock{Type: "image"},
			wantErr: true,
		},
		{
			name: "image block missing media_type",
			block: ContentBlock{
				Type:   "image",
				Source: &Source{Type: "base64", Data: "abc123"},
			},
			wantErr: true,
		},
		{
			name:    "valid tool_use block",
			block:   ContentBlock{Type: "tool_use", Name: "get_weather", Input: map[string]any{}},
			wantErr: false,
		},
		{
			name:    "tool_use missing name",
			block:   ContentBlock{Type: "tool_use", Input: map[string]any{}},
			wantErr: true,
		},
		{
			name:    "tool_use missing input",
			block:   ContentBlock{Type: "tool_use", Name: "get_weather"},
			wantErr: true,
		},
		{
			name:    "valid tool_result block",
			block:   ContentBlock{Type: "tool_result", ToolUseID: "tool_123", Content: "result"},
			wantErr: false,
		},
		{
			name:    "tool_result missing both",
			block:   ContentBlock{Type: "tool_result"},
			wantErr: true,
		},
		{
			name:    "unknown block type",
			block:   ContentBlock{Type: "unknown"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContentBlock(&tt.block)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateStreamRequest(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		wantErr bool
	}{
		{
			name: "valid streaming request",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
				Stream:    true,
			},
			wantErr: false,
		},
		{
			name: "stream flag not set",
			request: Request{
				Model:     "claude-3-opus",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
				Stream:    false,
			},
			wantErr: true,
		},
		{
			name: "invalid base request",
			request: Request{
				Model:     "",
				Messages:  []Message{{Role: "user", Content: "Hello"}},
				MaxTokens: 100,
				Stream:    true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStreamRequest(&tt.request)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRequestBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errContains string
	}{
		{
			name:    "valid request",
			input:   `{"model":"claude-3-opus","messages":[{"role":"user","content":"Hello"}],"max_tokens":100}`,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   `{invalid json}`,
			wantErr: true,
			errContains: "invalid request body",
		},
		{
			name:    "missing model",
			input:   `{"messages":[{"role":"user","content":"Hello"}],"max_tokens":100}`,
			wantErr: true,
			errContains: ErrModelRequired.Error(),
		},
		{
			name:    "missing messages",
			input:   `{"model":"claude-3-opus","max_tokens":100}`,
			wantErr: true,
			errContains: ErrMessagesRequired.Error(),
		},
		{
			name:    "empty messages",
			input:   `{"model":"claude-3-opus","messages":[],"max_tokens":100}`,
			wantErr: true,
			errContains: ErrMessagesRequired.Error(),
		},
		{
			name:    "invalid max_tokens",
			input:   `{"model":"claude-3-opus","messages":[{"role":"user","content":"Hello"}],"max_tokens":0}`,
			wantErr: true,
			errContains: ErrMaxTokensInvalid.Error(),
		},
		{
			name:    "negative max_tokens",
			input:   `{"model":"claude-3-opus","messages":[{"role":"user","content":"Hello"}],"max_tokens":-10}`,
			wantErr: true,
			errContains: ErrMaxTokensInvalid.Error(),
		},
		{
			name:    "invalid role",
			input:   `{"model":"claude-3-opus","messages":[{"role":"system","content":"Hello"}],"max_tokens":100}`,
			wantErr: true,
			errContains: ErrInvalidRole.Error(),
		},
		{
			name:    "nil content",
			input:   `{"model":"claude-3-opus","messages":[{"role":"user"}],"max_tokens":100}`,
			wantErr: true,
			errContains: ErrEmptyContent.Error(),
		},
		{
			name:    "valid array content",
			input:   `{"model":"claude-3-opus","messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}],"max_tokens":100}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequestBytes([]byte(tt.input))
			if tt.wantErr {
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

func TestGetErrorResponse(t *testing.T) {
	err := ErrModelRequired
	statusCode, errResp := GetErrorResponse(err)

	assert.Equal(t, 400, statusCode)
	assert.Equal(t, "error", errResp.Type)
	assert.Equal(t, ErrModelRequired.Error(), errResp.Message)
}