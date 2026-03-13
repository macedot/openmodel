// Package provider defines the provider interface and implementations
package provider

import (
	"net"
	"net/http"
	"strings"
	"time"
)

// OpenAIProvider implements Provider for OpenAI-compatible APIs
type OpenAIProvider struct {
	name               string
	baseURL            string
	apiKey             string
	apiMode            string
	httpClient         *http.Client
	transport          *http.Transport // Store transport for both clients
	cachedStreamClient *http.Client    // Cached streaming client (no timeout)
}

// HTTPConfig holds HTTP client configuration
type HTTPConfig struct {
	TimeoutSeconds               int
	MaxIdleConns                 int
	MaxIdleConnsPerHost          int
	IdleConnTimeoutSeconds       int
	DialTimeoutSeconds           int
	TLSHandshakeTimeoutSeconds   int
	ResponseHeaderTimeoutSeconds int
}

// DefaultHTTPConfig returns the default HTTP configuration
func DefaultHTTPConfig() HTTPConfig {
	return HTTPConfig{
		TimeoutSeconds:               120,
		MaxIdleConns:                 100,
		MaxIdleConnsPerHost:          100,
		IdleConnTimeoutSeconds:       90,
		DialTimeoutSeconds:           10,
		TLSHandshakeTimeoutSeconds:   10,
		ResponseHeaderTimeoutSeconds: 30,
	}
}

// NewOpenAIProvider creates a new OpenAI-compatible provider
func NewOpenAIProvider(name, baseURL, apiKey, apiMode string) *OpenAIProvider {
	return NewOpenAIProviderWithConfig(name, baseURL, apiKey, apiMode, DefaultHTTPConfig())
}

// NewOpenAIProviderWithConfig creates a new OpenAI-compatible provider with custom HTTP config
func NewOpenAIProviderWithConfig(name, baseURL, apiKey, apiMode string, httpConfig HTTPConfig) *OpenAIProvider {
	transport := &http.Transport{
		MaxIdleConns:        httpConfig.MaxIdleConns,
		MaxIdleConnsPerHost: httpConfig.MaxIdleConnsPerHost,
		IdleConnTimeout:     time.Duration(httpConfig.IdleConnTimeoutSeconds) * time.Second,
		DialContext: (&net.Dialer{
			Timeout: time.Duration(httpConfig.DialTimeoutSeconds) * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   time.Duration(httpConfig.TLSHandshakeTimeoutSeconds) * time.Second,
		ResponseHeaderTimeout: time.Duration(httpConfig.ResponseHeaderTimeoutSeconds) * time.Second,
	}

	streamingClient := &http.Client{
		Transport: transport,
		// No Timeout - lets streaming continue indefinitely as long as data arrives
	}

	return &OpenAIProvider{
		name:    name,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		apiMode: apiMode,
		httpClient: &http.Client{
			Timeout:   time.Duration(httpConfig.TimeoutSeconds) * time.Second,
			Transport: transport,
		},
		transport:          transport,
		cachedStreamClient: streamingClient,
	}
}

// Name returns the provider name
func (p *OpenAIProvider) Name() string {
	return p.name
}

// BaseURL returns the provider base URL
func (p *OpenAIProvider) BaseURL() string {
	return p.baseURL
}

// APIMode returns the provider's API mode ("openai" or "anthropic")
func (p *OpenAIProvider) APIMode() string {
	return p.apiMode
}

// Close releases resources associated with the provider.
// It closes idle connections and releases transport resources.
func (p *OpenAIProvider) Close() error {
	if p.transport != nil {
		p.transport.CloseIdleConnections()
	}
	return nil
}

// getStreamingClient returns an HTTP client configured for streaming.
// For streaming, we disable the client-level timeout because it applies to the
// entire request lifecycle including reading the response body. Instead, we rely
// on Transport-level timeouts (DialTimeout, TLSHandshakeTimeout, ResponseHeaderTimeout)
// which only apply to connection establishment and header reception.
// This ensures the timeout only applies to initial connection and header reception,
// allowing the stream to continue indefinitely as long as data chunks arrive.
func (p *OpenAIProvider) getStreamingClient() *http.Client {
	return p.cachedStreamClient
}
