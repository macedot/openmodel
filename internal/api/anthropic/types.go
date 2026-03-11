// Package anthropic defines types for the Anthropic/Claude API
package anthropic

import "encoding/json"

// Message represents a message in the messages array
type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []ContentBlock
}

// ContentBlock represents a block of content (text, image, etc.)
type ContentBlock struct {
	Type   string         `json:"type"`             // "text", "image", etc.
	Text   string         `json:"text,omitempty"`   // For text content
	Source *ContentSource `json:"source,omitempty"` // For image content
}

// ContentSource for image content
type ContentSource struct {
	Type      string `json:"type,omitempty"`       // "base64", "url"
	MediaType string `json:"media_type,omitempty"` // "image/jpeg", etc.
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// MessagesRequest is sent to /v1/messages
type MessagesRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	TopK        *int      `json:"top_k,omitempty"`
	System      string    `json:"system,omitempty"`
	Stop        []string  `json:"stop_sequences,omitempty"`
}

// MessagesResponse is returned from /v1/messages
type MessagesResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"` // "message"
	Role         string         `json:"role"` // "assistant"
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason,omitempty"`
	StopSequence string         `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

// Usage contains token usage information
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Streaming event types
type MessageStartEvent struct {
	Type    string          `json:"type"` // "message_start"
	Message MessagesResponse `json:"message"`
}

type ContentBlockStartEvent struct {
	Type         string       `json:"type"` // "content_block_start"
	Index        int          `json:"index"`
	ContentBlock ContentBlock `json:"content_block"`
}

type ContentBlockDeltaEvent struct {
	Type  string    `json:"type"` // "content_block_delta"
	Index int       `json:"index"`
	Delta DeltaText `json:"delta"`
}

type DeltaText struct {
	Type string `json:"type"` // "text_delta"
	Text string `json:"text"`
}

type ContentBlockStopEvent struct {
	Type  string `json:"type"` // "content_block_stop"
	Index int    `json:"index"`
}

type MessageDeltaEvent struct {
	Type  string    `json:"type"` // "message_delta"
	Delta DeltaStop `json:"delta"`
	Usage Usage     `json:"usage"`
}

type DeltaStop struct {
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
}

type MessageStopEvent struct {
	Type string `json:"type"` // "message_stop"
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Type        string     `json:"type"` // "error"
	ErrorDetail ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e *ErrorResponse) Error() string {
	return e.ErrorDetail.Message
}

// ParseMessagesRequest parses raw JSON into MessagesRequest
func ParseMessagesRequest(data []byte) (*MessagesRequest, error) {
	var req MessagesRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}