package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestMetadataContext(t *testing.T) {
	ctx := WithRequestMetadata(context.Background(), "req-123", "/v1/chat/completions")

	assert.Equal(t, "req-123", RequestIDFromContext(ctx))
	assert.Equal(t, "/v1/chat/completions", OriginalURLFromContext(ctx))
}

func TestRequestMetadataContext_MissingValues(t *testing.T) {
	ctx := context.Background()

	assert.Empty(t, RequestIDFromContext(ctx))
	assert.Empty(t, OriginalURLFromContext(ctx))
}
