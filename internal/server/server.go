// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"
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
	config      *config.Config
	providers   map[string]provider.Provider
	state       *state.State
	app         *fiber.App
	providersMu sync.RWMutex
	limiter     *RateLimiter
	version     string
}

// New creates a new server with the given configuration, providers, and state
func New(cfg *config.Config, providers map[string]provider.Provider, stateMgr *state.State, version string) *Server {
	srv := &Server{
		config:    cfg,
		providers: providers,
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
		ReadTimeout:    DefaultReadTimeout,
		WriteTimeout:   DefaultWriteTimeout,
		IdleTimeout:    DefaultIdleTimeout,
		StrictRouting:  true,
		CaseSensitive:  true,
		BodyLimit:      DefaultMaxRequestBody,
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
	if s.limiter != nil {
		s.app.Use(s.rateLimitMiddleware())
	}

	// Register routes
	s.registerRoutes(s.app)

	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
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
		if s.limiter == nil {
			return c.Next()
		}

		// Get client IP with trusted proxy support
		ip := s.limiter.GetClientIP(c.IP(), c.Get("X-Forwarded-For"), c.Get("X-Real-IP"))
		if !s.limiter.Allow(ip) {
			requestID, _ := c.Locals("request_id").(string)
			applogger.Warn("rate_limit_exceeded", "request_id", requestID, "ip", ip)
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded",
			})
		}
		return c.Next()
	}
}

// extractModelFromRequest extracts the model name from request body
func extractModelFromRequest(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err == nil && req.Model != "" {
		return req.Model
	}
	return ""
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
	return s.config
}

// GetProviders returns the current providers map
func (s *Server) GetProviders() map[string]provider.Provider {
	s.providersMu.RLock()
	defer s.providersMu.RUnlock()
	
	// Return a copy to prevent external modification
	result := make(map[string]provider.Provider, len(s.providers))
	for k, v := range s.providers {
		result[k] = v
	}
	return result
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
	newProviders := make(map[string]provider.Provider)
	for name, pc := range cfg.Providers {
		newProviders[name] = provider.NewOpenAIProviderWithConfig(name, pc.URL, pc.APIKey, pc.ApiMode, httpConfig)
	}

	// Close old providers to release resources
	for _, p := range s.providers {
		if closer, ok := p.(interface{ Close() error }); ok {
			closer.Close()
		}
	}

	s.config = cfg
	s.providers = newProviders

	// Recreate rate limiter if settings changed
	if cfg.RateLimit != nil && cfg.RateLimit.Enabled {
		cleanupInterval := 60 * time.Second
		if cfg.RateLimit.CleanupIntervalMs > 0 {
			cleanupInterval = time.Duration(cfg.RateLimit.CleanupIntervalMs) * time.Millisecond
		}
		s.limiter = NewRateLimiterWithTrustedProxies(
			cfg.RateLimit.RequestsPerSecond,
			cfg.RateLimit.Burst,
			cleanupInterval,
			cfg.RateLimit.TrustedProxies,
		)
	} else {
		s.limiter = nil
	}

	// Write trace file if trace level is enabled
	applogger.TraceFile("config-reload-"+time.Now().Format("20060102-150405"), map[string]any{
		"config_path":   cfg.GetConfigPath(),
		"providers":      len(newProviders),
		"models":        len(cfg.Models),
		"rate_limit":    cfg.RateLimit != nil && cfg.RateLimit.Enabled,
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