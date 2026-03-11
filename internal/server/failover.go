// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/macedot/openmodel/internal/config"
	applogger "github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
)

// providerResult holds a provider with its metadata
type providerResult struct {
	provider      provider.Provider
	providerKey   string
	providerModel string
}

// handleAllProvidersFailedFiber handles when all providers have failed
func (s *Server) handleAllProvidersFailedFiber(c *fiber.Ctx, lastErr error) {
	errMsg := "all providers failed"
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	requestID, _ := c.Locals("request_id").(string)
	applogger.Error("all_providers_failed", "request_id", requestID, "error", errMsg)

	timeout := s.state.GetProgressiveTimeout()
	s.state.IncrementTimeout(s.config.Thresholds.MaxTimeout)

	c.Set("Retry-After", fmt.Sprintf("%d", timeout/1000))
	handleError(c, errMsg, fiber.StatusServiceUnavailable)
}

// handleProviderError handles a provider error by recording failure
func (s *Server) handleProviderError(providerKey string, err error, threshold int) {
	applogger.Warn("provider_failed", "provider", providerKey, "error", err.Error())
	s.state.RecordFailure(providerKey, threshold)
}

// findProviderWithFailover finds an available provider for a model
func (s *Server) findProviderWithFailover(model string, providerName string) (provider.Provider, string, string, error) {
	modelConfig, exists := s.config.Models[model]
	if !exists {
		return nil, "", "", fmt.Errorf("model %q not found", model)
	}

	providers := modelConfig.Providers
	strategy := modelConfig.Strategy
	if strategy == "" {
		strategy = config.StrategyFallback
	}

	threshold := s.config.GetThresholds(providerName).FailuresBeforeSwitch

	// Find all available providers
	available := s.findAvailableProvidersForModel(providers, threshold)
	if len(available) == 0 {
		return nil, "", "", fmt.Errorf("no available providers for model %q", model)
	}

	// Select based on strategy
	switch strategy {
	case config.StrategyRoundRobin:
		idx := s.state.NextRoundRobin(model, len(available))
		p := available[idx]
		return p.provider, p.providerKey, p.providerModel, nil

	case config.StrategyRandom:
		idx := s.state.GetRandomIndex(len(available))
		p := available[idx]
		return p.provider, p.providerKey, p.providerModel, nil

	case config.StrategyFallback:
		fallthrough
	default:
		p := available[0]
		return p.provider, p.providerKey, p.providerModel, nil
	}
}

// findAvailableProvidersForModel returns available providers for a model
func (s *Server) findAvailableProvidersForModel(providers []config.ModelProvider, threshold int) []providerResult {
	var results []providerResult
	for _, p := range providers {
		providerKey := formatProviderKey(p)

		if !s.state.IsAvailable(providerKey, threshold) {
			continue
		}

		prov, exists := s.providers[p.Provider]
		if !exists {
			continue
		}

		results = append(results, providerResult{
			provider:      prov,
			providerKey:   providerKey,
			providerModel: p.Model,
		})
	}
	return results
}

// executeWithFailoverFiber handles non-streaming requests with failover
func (s *Server) executeWithFailoverFiber(ctx context.Context, model string, body []byte, headers map[string]string, endpoint string) (any, string, error) {
	var triedProviders []string

	for {
		prov, providerKey, providerModel, err := s.findProviderWithFailover(model, "")
		if err != nil {
			requestID, _ := ctx.Value("request_id").(string)
			applogger.Error("all_providers_failed",
				"request_id", requestID,
				"model", model,
				"providers_tried", triedProviders,
				"error", err.Error())
			return nil, "", fmt.Errorf("model %q temporarily unavailable: all providers failed", model)
		}

		triedProviders = append(triedProviders, providerKey)
		threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch

		// Log request processing
		requestID, _ := ctx.Value("request_id").(string)
		applogger.Debug("PROCESSING", "request_id", requestID, "provider", providerKey, "model", model)

		// Replace model name in body
		provBody := replaceModelInBody(body, providerModel)

		resp, err := prov.DoRequest(ctx, endpoint, provBody, headers)
		if err != nil {
			s.handleProviderError(providerKey, err, threshold)
			continue
		}

		return resp, providerKey, nil
	}
}