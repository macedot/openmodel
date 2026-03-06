// Package state provides failure tracking and sanitization caching for providers.
// This enables automatic fallback when a provider/model combination requires sanitization.
package state

import (
	"sync"
)

// SanitizationCache tracks which provider/model combinations need sanitization.
// This is used to cache the decision to sanitize messages after encountering
// prefill errors, avoiding repeated failed attempts.
type SanitizationCache struct {
	mu    sync.RWMutex
	cache map[string]bool // key: "provider/model", value: needs sanitization
}

// Global sanitization cache instance
var sanitizationCache = &SanitizationCache{
	cache: make(map[string]bool),
}

// GetKey generates a cache key for provider/model combination
func GetKey(provider, model string) string {
	return provider + "/" + model
}

// NeedsSanitization checks if a provider/model combination needs sanitization
func (s *SanitizationCache) NeedsSanitization(provider, model string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[GetKey(provider, model)]
}

// MarkNeedsSanitization marks a provider/model combination as needing sanitization
func (s *SanitizationCache) MarkNeedsSanitization(provider, model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[GetKey(provider, model)] = true
}

// Clear removes all entries from the cache
func (s *SanitizationCache) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[string]bool)
}

// GetGlobal returns the global sanitization cache instance
func GetGlobal() *SanitizationCache {
	return sanitizationCache
}
