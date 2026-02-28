// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/state"
)

// Server represents the HTTP server
type Server struct {
	config     *config.Config
	providers  map[string]provider.Provider
	state      *state.State
	httpServer *http.Server
}

// New creates a new server with the given configuration, providers, and state
func New(cfg *config.Config, providers map[string]provider.Provider, stateMgr *state.State) *Server {
	return &Server{
		config:    cfg,
		providers: providers,
		state:     stateMgr,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return s.httpServer.ListenAndServe()
}

// Stop gracefully shuts down the server
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}
