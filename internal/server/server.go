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
	app.Get("/", s.handleRoot)
	app.Get("/health", s.handleHealth)

	// OpenAI endpoints
	app.Post("/v1/chat/completions", s.handleV1ChatCompletions)

	// Anthropic endpoints
	app.Post("/v1/messages", s.handleV1Messages)
}

// handleRoot handles GET /
func (s *Server) handleRoot(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"name":    "openmodel",
		"version": s.version,
		"status":  "running",
	})
}

// handleHealth handles GET /health
func (s *Server) handleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
	})
}