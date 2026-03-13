package provider

import "context"

type requestContextKey string

const (
	requestIDContextKey   requestContextKey = "request_id"
	originalURLContextKey requestContextKey = "original_url"
)

// WithRequestMetadata stores request-scoped metadata using typed keys.
func WithRequestMetadata(ctx context.Context, requestID, originalURL string) context.Context {
	ctx = context.WithValue(ctx, requestIDContextKey, requestID)
	return context.WithValue(ctx, originalURLContextKey, originalURL)
}

// RequestIDFromContext extracts the request ID from context.
func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey).(string)
	return requestID
}

// OriginalURLFromContext extracts the original request URL from context.
func OriginalURLFromContext(ctx context.Context) string {
	originalURL, _ := ctx.Value(originalURLContextKey).(string)
	return originalURL
}
