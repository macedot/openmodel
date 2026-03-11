// Package provider defines the provider interface and implementations
package provider

import (
	"fmt"
	"os"
	"strings"
	"time"

	applogger "github.com/macedot/openmodel/internal/logger"
)

// createTraceFileForRequest creates a new trace file for a specific request
// Returns the file handle (caller must close it)
func createTraceFileForRequest(providerName, requestID string) *os.File {
	// Only create trace files if TRACE level is enabled
	if !applogger.IsTraceEnabled() {
		return nil
	}

	// Sanitize provider name to prevent path traversal (only allow alphanumeric and dash/underscore)
	sanitizedProvider := sanitizeProviderName(providerName)
	sanitizedRequestID := sanitizeRequestID(requestID)

	// Create file with timestamp prefix in current directory
	filename := fmt.Sprintf("%d-%s-%s.json", time.Now().UnixNano(), sanitizedProvider, sanitizedRequestID)
	f, err := os.Create(filename)
	if err != nil {
		return nil
	}
	return f
}

// sanitizeProviderName sanitizes provider names for use in trace filenames.
// Only allows alphanumeric characters, dash, and underscore.
func sanitizeProviderName(name string) string {
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		}
	}
	if result.Len() == 0 {
		return "unknown"
	}
	return result.String()
}

// sanitizeRequestID sanitizes request IDs for use in trace filenames.
// Only allows alphanumeric characters.
func sanitizeRequestID(id string) string {
	var result strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		}
	}
	if result.Len() == 0 {
		return "unknown"
	}
	return result.String()
}