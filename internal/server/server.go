// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/macedot/openmodel/internal/config"
	applogger "github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
	_ "github.com/macedot/openmodel/internal/server/converters"
	"github.com/macedot/openmodel/internal/state"
	"github.com/sixafter/nanoid"
)

// Server represents the Fiber HTTP server
type Server struct {
	config    *config.Config
	providers providerMap
	state     *state.State
	app       *fiber.App
	// providersMu protects runtime state swapped during hot reload.
	providersMu sync.RWMutex
	limiter     *RateLimiter
	version     string
}

// New creates a new server with the given configuration, providers, and state
func New(cfg *config.Config, providers map[string]provider.Provider, stateMgr *state.State, version string) *Server {
	srv := &Server{
		config:    cfg,
		providers: asProviderMap(providers),
		state:     stateMgr,
		version:   version,
	}

	// Initialize rate limiter if enabled
	if cfg.RateLimit != nil && cfg.RateLimit.Enabled {
		cleanupInterval := 60 * time.Second
		if cfg.RateLimit.CleanupIntervalMs > 0 {
			cleanupInterval = time.Duration(cfg.RateLimit.CleanupIntervalMs) * time.Millisecond
		}
		srv.limiter = NewRateLimiterWithTrustedProxies(
			cfg.RateLimit.RequestsPerSecond,
			cfg.RateLimit.Burst,
			cleanupInterval,
			cfg.RateLimit.TrustedProxies,
		)
	}

	return srv
}

// generateRequestID generates a short random request ID (alphanumeric only)
func generateRequestID() string {
	generator, err := nanoid.NewGenerator(
		nanoid.WithAlphabet("abcdefghijklmnopqrstuvwxyz0123456789"),
		nanoid.WithLengthHint(10),
	)
	if err != nil {
		return ""
	}
	id, err := generator.New()
	if err != nil {
		return ""
	}
	return string(id)
}

// Start starts the Fiber server
func (s *Server) Start() error {
	s.app = fiber.New(fiber.Config{
		ReadTimeout:   DefaultReadTimeout,
		WriteTimeout:  DefaultWriteTimeout,
		IdleTimeout:   DefaultIdleTimeout,
		StrictRouting: true,
		CaseSensitive: true,
		BodyLimit:     DefaultMaxRequestBody,
	})

	// Recovery middleware
	s.app.Use(recover.New())

	// Request logging middleware - logs received and completed
	s.app.Use(func(c *fiber.Ctx) error {
		start := time.Now()

		// Generate or get request ID
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		c.Set("X-Request-ID", requestID)
		c.Locals("request_id", requestID)

		// Log request received
		applogger.Info("REQUEST",
			"request_id", requestID,
			"ip", c.IP(),
			"method", c.Method(),
			"path", c.Path(),
			"size", len(c.Body()),
		)

		// Process request
		err := c.Next()

		// Log request completed
		applogger.Info("RESPONSE",
			"request_id", requestID,
			"ip", c.IP(),
			"method", c.Method(),
			"path", c.Path(),
			"status", c.Response().StatusCode(),
			"latency", time.Since(start).String(),
			"req_size", len(c.Body()),
			"res_size", len(c.Response().Body()),
		)

		return err
	})

	// Rate limiting middleware
	if s.getLimiter() != nil {
		s.app.Use(s.rateLimitMiddleware())
	}

	// Register routes
	s.registerRoutes(s.app)

	cfg := s.GetConfig()
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	applogger.Info("server_starting", "addr", addr, "version", s.version)
	return s.app.Listen(addr)
}

// Stop gracefully shuts down the server
func (s *Server) Stop(ctx context.Context) error {
	if s.app == nil {
		return nil
	}
	applogger.Info("server_shutting_down")
	return s.app.Shutdown()
}

// rateLimitMiddleware rate limits requests by IP
func (s *Server) rateLimitMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		limiter := s.getLimiter()
		if limiter == nil {
			return c.Next()
		}

		// Get client IP with trusted proxy support
		ip := limiter.GetClientIP(c.IP(), c.Get("X-Forwarded-For"), c.Get("X-Real-IP"))
		if !limiter.Allow(ip) {
			requestID, _ := c.Locals("request_id").(string)
			applogger.Warn("rate_limit_exceeded", "request_id", requestID, "ip", ip)
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded",
			})
		}
		return c.Next()
	}
}

func (s *Server) getLimiter() *RateLimiter {
	s.providersMu.RLock()
	defer s.providersMu.RUnlock()
	return s.limiter
}

// registerRoutes registers all API routes
func (s *Server) registerRoutes(app *fiber.App) {
	// Health endpoints
	app.Get(EndpointRoot, s.handleRoot)
	app.Get(EndpointHealth, s.handleHealth)

	// OpenAI endpoints
	app.Post(EndpointV1ChatCompletions, s.handleV1ChatCompletions)

	// Anthropic endpoints
	app.Post(EndpointV1Messages, s.handleV1Messages)
}

// handleRoot handles GET /
func (s *Server) handleRoot(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"name":    "openmodel",
		"version": s.version,
		"status":  "running",
	})
}

// GetConfig returns the current configuration
func (s *Server) GetConfig() *config.Config {
	s.providersMu.RLock()
	defer s.providersMu.RUnlock()
	return s.config
}

// GetProviders returns a copy of the current server-facing providers map.
func (s *Server) GetProviders() providerMap {
	s.providersMu.RLock()
	defer s.providersMu.RUnlock()
	return cloneProviderMap(s.providers)
}

// ReloadConfig atomically reloads the configuration and providers
// Returns an error if the new config is invalid (config remains unchanged)
func (s *Server) ReloadConfig(cfg *config.Config) error {
	// Convert config.HTTP to provider.HTTPConfig
	httpConfig := provider.HTTPConfig{
		TimeoutSeconds:               cfg.HTTP.TimeoutSeconds,
		MaxIdleConns:                 cfg.HTTP.MaxIdleConns,
		MaxIdleConnsPerHost:          cfg.HTTP.MaxIdleConnsPerHost,
		IdleConnTimeoutSeconds:       cfg.HTTP.IdleConnTimeoutSeconds,
		DialTimeoutSeconds:           cfg.HTTP.DialTimeoutSeconds,
		TLSHandshakeTimeoutSeconds:   cfg.HTTP.TLSHandshakeTimeoutSeconds,
		ResponseHeaderTimeoutSeconds: cfg.HTTP.ResponseHeaderTimeoutSeconds,
	}

	// Create new providers from the config
	newProviders := make(providerMap)
	for name, pc := range cfg.Providers {
		newProviders[name] = provider.NewOpenAIProviderWithConfig(name, pc.URL, pc.APIKey, pc.ApiMode, httpConfig)
	}

	var newLimiter *RateLimiter
	if cfg.RateLimit != nil && cfg.RateLimit.Enabled {
		cleanupInterval := 60 * time.Second
		if cfg.RateLimit.CleanupIntervalMs > 0 {
			cleanupInterval = time.Duration(cfg.RateLimit.CleanupIntervalMs) * time.Millisecond
		}
		newLimiter = NewRateLimiterWithTrustedProxies(
			cfg.RateLimit.RequestsPerSecond,
			cfg.RateLimit.Burst,
			cleanupInterval,
			cfg.RateLimit.TrustedProxies,
		)
	}

	s.providersMu.Lock()
	oldProviders := s.providers
	s.config = cfg
	s.providers = newProviders
	s.limiter = newLimiter
	s.providersMu.Unlock()

	// Close old providers to release resources after the swap.
	for _, p := range oldProviders {
		if err := p.Close(); err != nil {
			applogger.Warn("provider_close_failed", "provider", p.Name(), "error", err)
		}
	}

	// Write trace file if trace level is enabled
	applogger.TraceFile("config-reload-"+time.Now().Format("20060102-150405"), map[string]any{
		"config_path": cfg.GetConfigPath(),
		"providers":   len(newProviders),
		"models":      len(cfg.Models),
		"rate_limit":  cfg.RateLimit != nil && cfg.RateLimit.Enabled,
	})

	applogger.Info("config_reloaded",
		"config_path", cfg.GetConfigPath(),
		"providers", len(newProviders),
		"models", len(cfg.Models))

	return nil
}

// handleHealth handles GET /health
func (s *Server) handleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
	})
}
