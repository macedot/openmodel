// Package server implements the HTTP server and handlers
package server

import "net/http"

// registerRoutes registers all HTTP routes
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Root endpoint
	mux.HandleFunc("/", s.handleRoot)

	// Health endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// OpenAI-compatible API endpoints
	mux.HandleFunc("/v1/models", s.handleV1Models)
	mux.HandleFunc("/v1/models/", s.handleV1Model)
	mux.HandleFunc("/v1/chat/completions", s.handleV1ChatCompletions)
	mux.HandleFunc("/v1/completions", s.handleV1Completions)
	mux.HandleFunc("/v1/embeddings", s.handleV1Embeddings)
	mux.HandleFunc("/v1/moderations", s.handleV1Moderations)

	// Claude Messages API endpoint
	mux.HandleFunc("/v1/messages", s.handleV1Messages)
}

// RegisterRoutes registers all HTTP routes on the given mux (exported for testing)
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	s.registerRoutes(mux)
}
