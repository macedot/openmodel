// Package server implements the HTTP server and handlers
package server

import "github.com/macedot/openmodel/internal/server/converters"

// APIFormat represents an API format type (alias to converters.APIFormat)
type APIFormat = converters.APIFormat

// StreamConverter handles streaming line-by-line conversion (alias to converters.StreamConverter)
type StreamConverter = converters.StreamConverter

// StreamState holds state for stream conversion (alias to converters.StreamState)
type StreamState = converters.StreamState
