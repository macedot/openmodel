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
	name       string
	baseURL    string
	apiKey     string
	httpClient *http.Client
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
func NewOpenAIProvider(name, baseURL, apiKey string) *OpenAIProvider {
	return NewOpenAIProviderWithConfig(name, baseURL, apiKey, DefaultHTTPConfig())
}

// NewOpenAIProviderWithConfig creates a new OpenAI-compatible provider with custom HTTP config
func NewOpenAIProviderWithConfig(name, baseURL, apiKey string, httpConfig HTTPConfig) *OpenAIProvider {
	return &OpenAIProvider{
		name:    name,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: time.Duration(httpConfig.TimeoutSeconds) * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        httpConfig.MaxIdleConns,
				MaxIdleConnsPerHost: httpConfig.MaxIdleConnsPerHost,
				IdleConnTimeout:     time.Duration(httpConfig.IdleConnTimeoutSeconds) * time.Second,
				DialContext: (&net.Dialer{
					Timeout: time.Duration(httpConfig.DialTimeoutSeconds) * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   time.Duration(httpConfig.TLSHandshakeTimeoutSeconds) * time.Second,
				ResponseHeaderTimeout: time.Duration(httpConfig.ResponseHeaderTimeoutSeconds) * time.Second,
			},
		},
	}
}

// Name returns the provider name
func (p *OpenAIProvider) Name() string {
	return p.name
}