// Package converters provides API format converters
package converters

import "sync"

// APIFormat represents an API format type
type APIFormat string

const (
	// APIFormatOpenAI represents OpenAI API format
	APIFormatOpenAI APIFormat = "openai"
	// APIFormatAnthropic represents Anthropic API format
	APIFormatAnthropic APIFormat = "anthropic"
	// APIFormatPassthrough represents no conversion (passthrough)
	APIFormatPassthrough APIFormat = ""
)

// Anthropic API constants
const (
	HeaderAnthropicVersion = "anthropic-version"
	AnthropicAPIVersion   = "2023-06-01"
)

// StreamState holds state for stream conversion
type StreamState struct {
	IsFirst  *bool
	BlockIdx *int
}

// StreamConverter handles streaming line-by-line conversion
type StreamConverter interface {
	// ConvertRequest transforms the request body from source to target format
	ConvertRequest(body []byte) ([]byte, error)

	// ConvertResponse transforms the response body from target to source format
	ConvertResponse(body []byte) ([]byte, error)

	// ConvertStreamLine transforms a single SSE line during streaming
	// Returns empty string to skip the line
	ConvertStreamLine(line, model, id string, state *StreamState) string

	// GetEndpoint returns the target endpoint for the request
	GetEndpoint(original string) string

	// GetHeaders returns additional headers to include in the request
	GetHeaders() map[string]string
}

// Registry manages API format converters
type Registry struct {
	mu         sync.RWMutex
	converters map[string]StreamConverter // key: "source:target"
}

// NewRegistry creates a new converter registry
func NewRegistry() *Registry {
	return &Registry{
		converters: make(map[string]StreamConverter),
	}
}

// Register adds a converter for a source/target format pair
func (r *Registry) Register(source, target APIFormat, converter StreamConverter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := formatKey(source, target)
	r.converters[key] = converter
}

// Get retrieves a converter for a source/target format pair
func (r *Registry) Get(source, target APIFormat) (StreamConverter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := formatKey(source, target)
	converter, ok := r.converters[key]
	return converter, ok
}

// formatKey creates a registry key from source and target formats
func formatKey(source, target APIFormat) string {
	return string(source) + ":" + string(target)
}

// Global registry instance
var globalRegistry = NewRegistry()

// RegisterConverter adds a converter to the global registry
func RegisterConverter(source, target APIFormat, converter StreamConverter) {
	globalRegistry.Register(source, target, converter)
}

// GetConverter retrieves a converter from the global registry
func GetConverter(source, target APIFormat) (StreamConverter, bool) {
	return globalRegistry.Get(source, target)
}