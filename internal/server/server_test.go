// Package server provides tests for the HTTP server
package server

import (
	"context"
	"testing"
	"time"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/state"
	"github.com/stretchr/testify/assert"
)

// TestNewServer tests server creation
func TestNewServer(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 12345,
			Host: "localhost",
		},
	}
	providers := map[string]provider.Provider{}
	stateMgr := state.New(10000) // 10 second initial timeout

	srv := New(cfg, providers, stateMgr, "test-version")

	assert.NotNil(t, srv)
	assert.Equal(t, cfg, srv.config)
	assert.Equal(t, asProviderMap(providers), srv.providers)
	assert.Equal(t, stateMgr, srv.state)
	assert.Equal(t, "test-version", srv.version)
}

// TestNewServer_WithRateLimit tests server creation with rate limiting
func TestNewServer_WithRateLimit(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 12345,
			Host: "localhost",
		},
		RateLimit: &config.RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 10,
			Burst:             20,
			CleanupIntervalMs: 60000,
		},
	}
	providers := map[string]provider.Provider{}
	stateMgr := state.New(10000)

	srv := New(cfg, providers, stateMgr, "test-version")

	assert.NotNil(t, srv)
	assert.NotNil(t, srv.limiter)
}

// TestNewServer_WithTrustedProxies tests server creation with trusted proxies
func TestNewServer_WithTrustedProxies(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 12345,
			Host: "localhost",
		},
		RateLimit: &config.RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 10,
			Burst:             20,
			CleanupIntervalMs: 60000,
			TrustedProxies:    []string{"192.168.1.0/24", "10.0.0.1"},
		},
	}
	providers := map[string]provider.Provider{}
	stateMgr := state.New(10000)

	srv := New(cfg, providers, stateMgr, "test-version")

	assert.NotNil(t, srv)
	assert.NotNil(t, srv.limiter)
	assert.Len(t, srv.limiter.trustedProxies, 2)
}

// TestNewServer_WithoutRateLimit tests server creation without rate limiting
func TestNewServer_WithoutRateLimit(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 12345,
			Host: "localhost",
		},
	}
	providers := map[string]provider.Provider{}
	stateMgr := state.New(10000)

	srv := New(cfg, providers, stateMgr, "test-version")

	assert.NotNil(t, srv)
	assert.Nil(t, srv.limiter)
}

// TestGenerateRequestID tests request ID generation
func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	id2 := generateRequestID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2, "request IDs should be unique")
	assert.Len(t, id1, 10, "request ID should be 10 characters")

	// Should only contain alphanumeric characters
	for _, r := range id1 {
		assert.True(t, (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'),
			"request ID should only contain lowercase alphanumeric characters")
	}
}

// TestDefaultRateLimiter tests default rate limiter creation
func TestDefaultRateLimiter(t *testing.T) {
	rl := NewDefaultRateLimiter()

	assert.NotNil(t, rl)
	assert.Equal(t, DefaultRequestsPerSecond, rl.rate)
	assert.Equal(t, DefaultBurst, rl.burst)
	assert.Equal(t, DefaultCleanupInterval, rl.cleanup)
}

// TestRateLimiterDifferentIPs tests that different IPs have separate rate limits
func TestRateLimiterDifferentIPs(t *testing.T) {
	rl := NewRateLimiter(1, 5, time.Minute)

	// Exhaust rate limit for IP1
	for i := 0; i < 5; i++ {
		assert.True(t, rl.Allow("192.168.1.1"))
	}
	assert.False(t, rl.Allow("192.168.1.1"))

	// IP2 should still be allowed
	assert.True(t, rl.Allow("192.168.1.2"))
}

// TestRateLimiterTokenRefill tests that tokens refill over time
func TestRateLimiterTokenRefill(t *testing.T) {
	rl := NewRateLimiter(100, 2, time.Minute) // 100 req/sec, burst of 2

	// Use all tokens
	assert.True(t, rl.Allow("192.168.1.1"))
	assert.True(t, rl.Allow("192.168.1.1"))
	assert.False(t, rl.Allow("192.168.1.1"))

	// Wait for refill (10ms should give us ~1 token at 100 req/sec)
	time.Sleep(15 * time.Millisecond)

	// Should have tokens again
	assert.True(t, rl.Allow("192.168.1.1"))
}

type closableProvider struct {
	closed bool
}

func (p *closableProvider) Name() string    { return "old" }
func (p *closableProvider) BaseURL() string { return "" }
func (p *closableProvider) APIMode() string { return "openai" }
func (p *closableProvider) ListModels(ctx context.Context) (*openai.ModelList, error) {
	return nil, nil
}
func (p *closableProvider) Chat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	return nil, nil
}
func (p *closableProvider) StreamChat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan openai.ChatCompletionResponse, error) {
	return nil, nil
}
func (p *closableProvider) StreamChatRaw(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan []byte, error) {
	return nil, nil
}
func (p *closableProvider) DoRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error) {
	return nil, nil
}
func (p *closableProvider) DoStreamRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) (<-chan []byte, error) {
	return nil, nil
}
func (p *closableProvider) Complete(ctx context.Context, model string, req *openai.CompletionRequest) (*openai.CompletionResponse, error) {
	return nil, nil
}
func (p *closableProvider) StreamComplete(ctx context.Context, model string, req *openai.CompletionRequest) (<-chan openai.CompletionResponse, error) {
	return nil, nil
}
func (p *closableProvider) Embed(ctx context.Context, model string, input []string) (*openai.EmbeddingResponse, error) {
	return nil, nil
}
func (p *closableProvider) Moderate(ctx context.Context, input string) (*openai.ModerationResponse, error) {
	return nil, nil
}
func (p *closableProvider) Close() error {
	p.closed = true
	return nil
}

func TestReloadConfig_ReplacesProvidersAndClosesOldOnes(t *testing.T) {
	oldProvider := &closableProvider{}
	srv := &Server{
		config: &config.Config{
			Providers: map[string]config.ProviderConfig{
				"old": {URL: "http://old", ApiMode: "openai"},
			},
			Models: map[string]config.ModelConfig{},
			HTTP:   config.DefaultConfig().HTTP,
		},
		providers: providerMap{"old": oldProvider},
		state:     state.New(1000),
	}

	newCfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"new": {URL: "http://new", ApiMode: "openai"},
		},
		Models: map[string]config.ModelConfig{},
		HTTP:   config.DefaultConfig().HTTP,
	}

	err := srv.ReloadConfig(newCfg)
	assert.NoError(t, err)
	assert.True(t, oldProvider.closed)
	assert.Equal(t, newCfg, srv.GetConfig())
	assert.Contains(t, srv.GetProviders(), "new")
	assert.NotContains(t, srv.GetProviders(), "old")
}
