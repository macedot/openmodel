// Package ollama defines types for the Ollama native API
package ollama

import (
	"encoding/json"
	"fmt"
	"time"
)

// VersionResponse is returned by /api/version
type VersionResponse struct {
	Version string `json:"version"`
}

// ListResponse is returned by /api/tags
type ListResponse struct {
	Models []ListModelResponse `json:"models"`
}

// ListModelResponse represents a model in the list
type ListModelResponse struct {
	Name       string       `json:"name"`
	Model      string       `json:"model"`
	ModifiedAt time.Time    `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
}

// ModelDetails contains model metadata
type ModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// Message represents a chat message
type Message struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"` // Base64 encoded images
}

// ChatRequest is sent to /api/chat
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   *bool     `json:"stream,omitempty"`
	Format   string    `json:"format,omitempty"` // "json" for structured output
	Options  *Options  `json:"options,omitempty"`
}

// ChatResponse is returned from /api/chat
type ChatResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Message   *Message  `json:"message"`
	Done      bool      `json:"done"`
	Metrics   *Metrics  `json:",omitempty"`
}

// GenerateRequest is sent to /api/generate
type GenerateRequest struct {
	Model   string   `json:"model"`
	Prompt  string   `json:"prompt"`
	Stream  *bool    `json:"stream,omitempty"`
	Raw     bool     `json:"raw,omitempty"`
	Format  string   `json:"format,omitempty"`
	Images  []string `json:"images,omitempty"`
	Options *Options `json:"options,omitempty"`
}

// GenerateResponse is returned from /api/generate
type GenerateResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Response  string    `json:"response"`
	Done      bool      `json:"done"`
	Metrics   *Metrics  `json:",omitempty"`
}

// EmbedRequest is sent to /api/embed
type EmbedRequest struct {
	Model   string   `json:"model"`
	Input   []string `json:"input"`
	Options *Options `json:"options,omitempty"`
}

// EmbedResponse is returned from /api/embed
type EmbedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float64 `json:"embeddings"`
}

// Metrics contains performance metrics returned when done is true
type Metrics struct {
	PromptEvalCount       int     `json:"prompt_eval_count"`
	PromptEvalDuration    float64 `json:"prompt_eval_duration"`
	EvalCount             int     `json:"eval_count"`
	EvalDuration          float64 `json:"eval_duration"`
	LoadDuration          float64 `json:"load_duration"`
	TotalDuration         float64 `json:"total_duration"`
	SampleCount           int     `json:"sample_count,omitempty"`
	SampleDuration        float64 `json:"sample_duration,omitempty"`
	PromptEvalTokenCounts []int   `json:"prompt_eval_token_counts,omitempty"`
}

// Options contains model generation options
type Options struct {
	// Sampling options
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	TopK        int     `json:"top_k,omitempty"`
	Seed        int     `json:"seed,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
	NumKeep     int     `json:"num_keep,omitempty"`

	// Stop sequences
	Stop []string `json:"stop,omitempty"`

	// Advanced options
	RepeatLastN      int     `json:"repeat_last_n,omitempty"`
	RepeatPenalty    float64 `json:"repeat_penalty,omitempty"`
	FrequencyPenalty float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64 `json:"presence_penalty,omitempty"`
	Mirostat         int     `json:"mirostat,omitempty"`
	MirostatTau      float64 `json:"mirostat_tau,omitempty"`
	MirostatEta      float64 `json:"mirostat_eta,omitempty"`

	// Performance options
	NumCtx    int  `json:"num_ctx,omitempty"`
	NumBatch  int  `json:"num_batch,omitempty"`
	NumGPU    int  `json:"num_gpu,omitempty"`
	MainGPU   int  `json:"main_gpu,omitempty"`
	LowVRAM   bool `json:"low_vram,omitempty"`
	F16KV     bool `json:"f16_kv,omitempty"`
	LogitsAll bool `json:"logits_all,omitempty"`
	VocabOnly bool `json:"vocab_only,omitempty"`
	UseMMap   bool `json:"use_mmap,omitempty"`
	UseMLock  bool `json:"use_mlock,omitempty"`
	NumThread int  `json:"num_thread,omitempty"`
}

// StatusError represents an error response from Ollama
type StatusError struct {
	StatusCode   int    `json:"status_code"`
	ErrorMessage string `json:"error"`
}

// Error implements the error interface
func (e *StatusError) Error() string {
	return fmt.Sprintf("ollama error (status %d): %s", e.StatusCode, e.ErrorMessage)
}

// ParseStatusError attempts to parse an error response body as StatusError
func ParseStatusError(body []byte) *StatusError {
	var se StatusError
	if err := json.Unmarshal(body, &se); err != nil {
		return nil
	}
	if se.ErrorMessage == "" {
		return nil
	}
	return &se
}

// PullRequest is sent to /api/pull
type PullRequest struct {
	Name     string `json:"name"`
	Insecure bool   `json:"insecure,omitempty"`
	Stream   bool   `json:"stream,omitempty"`
}

// PullResponse is returned from /api/pull (streaming)
type PullResponse struct {
	Status    string `json:"status"`
	Digest    string `json:"digest,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Completed int64  `json:"completed,omitempty"`
}

// ShowRequest is sent to /api/show
type ShowRequest struct {
	Name    string `json:"name"`
	Verbose bool   `json:"verbose,omitempty"`
}

// ShowResponse is returned from /api/show
type ShowResponse struct {
	License    string         `json:"license,omitempty"`
	Modelfile  string         `json:"modelfile,omitempty"`
	Parameters string         `json:"parameters,omitempty"`
	Template   string         `json:"template,omitempty"`
	Details    *ModelDetails  `json:"details,omitempty"`
	ModelInfo  map[string]any `json:"model_info,omitempty"`
}
