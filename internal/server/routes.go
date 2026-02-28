// Package server implements the HTTP server and handlers
package server

import "net/http"

// registerRoutes registers all HTTP routes
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Root endpoint
	mux.HandleFunc("/", s.handleRoot)

	// Ollama-compatible API endpoints
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/api/tags", s.handleTags)
	mux.HandleFunc("/api/ps", s.handlePS)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/generate", s.handleGenerate)
	mux.HandleFunc("/api/embed", s.handleEmbed)
	mux.HandleFunc("/api/embeddings", s.handleEmbeddings)
	mux.HandleFunc("/api/show", s.handleShow)

	// OpenAI-compatible API endpoints
	mux.HandleFunc("/v1/models", s.handleV1Models)
	mux.HandleFunc("/v1/chat/completions", s.handleV1ChatCompletions)
}
