// Package openai defines types for the OpenAI-compatible API
package openai

import (
	"encoding/json"
	"fmt"
	"time"
)

// Model represents a model object
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelList is returned by /v1/models
type ModelList struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// ChatCompletionMessage represents a message in a chat completion
type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// ChatCompletionRequest is sent to /v1/chat/completions
type ChatCompletionRequest struct {
	Model            string                  `json:"model"`
	Messages         []ChatCompletionMessage `json:"messages"`
	Temperature      *float64                `json:"temperature,omitempty"`
	TopP             *float64                `json:"top_p,omitempty"`
	N                *int                    `json:"n,omitempty"`
	Stream           bool                    `json:"stream,omitempty"`
	Stop             []string                `json:"stop,omitempty"`
	MaxTokens        *int                    `json:"max_tokens,omitempty"`
	PresencePenalty  *float64                `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64                `json:"frequency_penalty,omitempty"`
	LogitBias        map[string]float64      `json:"logit_bias,omitempty"`
	User             string                  `json:"user,omitempty"`
	ResponseFormat   *ResponseFormat         `json:"response_format,omitempty"`
	Seed             *int                    `json:"seed,omitempty"`
	Tools            []Tool                  `json:"tools,omitempty"`
	ToolChoice       any                     `json:"tool_choice,omitempty"`
}

// ResponseFormat specifies the format of the response
type ResponseFormat struct {
	Type       string `json:"type"` // "text" or "json_object"
	JSONSchema any    `json:"json_schema,omitempty"`
}

// Tool represents a tool that can be called
type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction defines a function tool
type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// ChatCompletionChoice represents a completion choice
type ChatCompletionChoice struct {
	Index        int                    `json:"index"`
	Message      *ChatCompletionMessage `json:"message"`
	Delta        *ChatCompletionDelta   `json:"delta,omitempty"`
	FinishReason string                 `json:"finish_reason"`
	Logprobs     any                    `json:"logprobs,omitempty"`
}

// ChatCompletionDelta is used for streaming responses
type ChatCompletionDelta struct {
	Role      string              `json:"role,omitempty"`
	Content   string              `json:"content,omitempty"`
	ToolCalls []ChatToolCallDelta `json:"tool_calls,omitempty"`
}

// ChatToolCallDelta represents a tool call in a streaming response
type ChatToolCallDelta struct {
	Index    int                    `json:"index"`
	ID       string                 `json:"id,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Function *ToolCallFunctionDelta `json:"function,omitempty"`
}

// ToolCallFunctionDelta represents function call data in streaming
type ToolCallFunctionDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ChatCompletionResponse is returned from /v1/chat/completions
type ChatCompletionResponse struct {
	ID                string                 `json:"id"`
	Object            string                 `json:"object"`
	Created           int64                  `json:"created"`
	Model             string                 `json:"model"`
	Choices           []ChatCompletionChoice `json:"choices"`
	Usage             *Usage                 `json:"usage,omitempty"`
	SystemFingerprint string                 `json:"system_fingerprint,omitempty"`
}

// Usage contains token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Err *ErrorDetail `json:"error"`
}

// ErrorDetail contains error details
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param,omitempty"`
	Code    string `json:"code,omitempty"`
}

// Error implements the error interface
func (e *ErrorResponse) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s", e.Err.Type, e.Err.Message)
	}
	return "unknown error"
}

// ParseErrorResponse attempts to parse an error response body
func ParseErrorResponse(body []byte) *ErrorResponse {
	var er ErrorResponse
	if err := json.Unmarshal(body, &er); err != nil {
		return nil
	}
	if er.Err == nil {
		return nil
	}
	return &er
}

// CompletionRequest is sent to /v1/completions (legacy)
type CompletionRequest struct {
	Model            string             `json:"model"`
	Prompt           any                `json:"prompt"` // string or []string
	Suffix           string             `json:"suffix,omitempty"`
	MaxTokens        *int               `json:"max_tokens,omitempty"`
	Temperature      *float64           `json:"temperature,omitempty"`
	TopP             *float64           `json:"top_p,omitempty"`
	N                *int               `json:"n,omitempty"`
	Stream           bool               `json:"stream,omitempty"`
	Logprobs         *int               `json:"logprobs,omitempty"`
	Echo             bool               `json:"echo,omitempty"`
	Stop             []string           `json:"stop,omitempty"`
	PresencePenalty  *float64           `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64           `json:"frequency_penalty,omitempty"`
	BestOf           *int               `json:"best_of,omitempty"`
	LogitBias        map[string]float64 `json:"logit_bias,omitempty"`
	User             string             `json:"user,omitempty"`
}

// CompletionResponse is returned from /v1/completions
type CompletionResponse struct {
	ID                string             `json:"id"`
	Object            string             `json:"object"`
	Created           int64              `json:"created"`
	Model             string             `json:"model"`
	Choices           []CompletionChoice `json:"choices"`
	Usage             *Usage             `json:"usage,omitempty"`
	SystemFingerprint string             `json:"system_fingerprint,omitempty"`
}

// CompletionChoice represents a legacy completion choice
type CompletionChoice struct {
	Text         string    `json:"text"`
	Index        int       `json:"index"`
	Logprobs     *Logprobs `json:"logprobs,omitempty"`
	FinishReason string    `json:"finish_reason"`
}

// Logprobs contains log probability information
type Logprobs struct {
	Tokens        []string             `json:"tokens,omitempty"`
	TokenLogprobs []float64            `json:"token_logprobs,omitempty"`
	TopLogprobs   []map[string]float64 `json:"top_logprobs,omitempty"`
	TextOffset    []int                `json:"text_offset,omitempty"`
}

// EmbeddingRequest is sent to /v1/embeddings
type EmbeddingRequest struct {
	Model          string `json:"model"`
	Input          any    `json:"input"` // string, []string, or [][]int
	EncodingFormat string `json:"encoding_format,omitempty"`
	Dimensions     int    `json:"dimensions,omitempty"`
	User           string `json:"user,omitempty"`
}

// EmbeddingResponse is returned from /v1/embeddings
type EmbeddingResponse struct {
	Object string          `json:"object"`
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  *Usage          `json:"usage"`
}

// EmbeddingData represents a single embedding
type EmbeddingData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

// ChatCompletionChunk is used for streaming chat completions
type ChatCompletionChunk struct {
	ID                string                      `json:"id"`
	Object            string                      `json:"object"`
	Created           int64                       `json:"created"`
	Model             string                      `json:"model"`
	Choices           []ChatCompletionChunkChoice `json:"choices"`
	SystemFingerprint string                      `json:"system_fingerprint,omitempty"`
}

// ChatCompletionChunkChoice represents a choice in a streaming chunk
type ChatCompletionChunkChoice struct {
	Index        int                 `json:"index"`
	Delta        ChatCompletionDelta `json:"delta"`
	FinishReason *string             `json:"finish_reason"`
	Logprobs     any                 `json:"logprobs,omitempty"`
}

// StreamResponseToChunk converts a streaming line to a ChatCompletionChunk
func StreamResponseToChunk(data []byte) (*ChatCompletionChunk, error) {
	var chunk ChatCompletionChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, err
	}
	return &chunk, nil
}

// IsStreamDone checks if the stream is finished (chunk has finish_reason set or is [DONE])
func IsStreamDone(line string) bool {
	return line == "[DONE]" || line == "data: [DONE]"
}

// NewModel creates a Model with sensible defaults
func NewModel(id, ownedBy string) Model {
	return Model{
		ID:      id,
		Object:  "model",
		Created: time.Now().Unix(),
		OwnedBy: ownedBy,
	}
}
