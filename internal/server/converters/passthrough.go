// Package converters provides API format converters
package converters

// PassthroughConverter is a no-op converter that passes data through unchanged
type PassthroughConverter struct{}

// NewPassthroughConverter creates a new passthrough converter
func NewPassthroughConverter() *PassthroughConverter {
	return &PassthroughConverter{}
}

// ConvertRequest returns the body unchanged
func (c *PassthroughConverter) ConvertRequest(body []byte) ([]byte, error) {
	return body, nil
}

// ConvertResponse returns the body unchanged
func (c *PassthroughConverter) ConvertResponse(body []byte) ([]byte, error) {
	return body, nil
}

// ConvertStreamLine returns the line unchanged
func (c *PassthroughConverter) ConvertStreamLine(line, model, id string, state *StreamState) string {
	return line
}

// GetEndpoint returns the original endpoint unchanged
func (c *PassthroughConverter) GetEndpoint(original string) string {
	return original
}

// GetHeaders returns no additional headers
func (c *PassthroughConverter) GetHeaders() map[string]string {
	return nil
}