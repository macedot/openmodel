// Package server implements the HTTP server and handlers
package server

import (
	"net"
	"strings"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter per IP address
type RateLimiter struct {
	mu             sync.RWMutex
	buckets        map[string]*tokenBucket
	rate           int           // requests per second
	burst          int           // max burst size
	cleanup        time.Duration // cleanup interval
	lastCleanup    time.Time
	trustedProxies []*net.IPNet // CIDR networks that are trusted to send X-Forwarded-For headers
}

// tokenBucket represents a token bucket for rate limiting
type tokenBucket struct {
	tokens   float64
	lastSeen time.Time
}

// NewRateLimiter creates a new rate limiter with the given rate and burst
func NewRateLimiter(rate, burst int, cleanup time.Duration) *RateLimiter {
	return &RateLimiter{
		buckets:        make(map[string]*tokenBucket),
		rate:           rate,
		burst:          burst,
		cleanup:        cleanup,
		lastCleanup:    time.Now(),
		trustedProxies: nil,
	}
}

// NewRateLimiterWithTrustedProxies creates a new rate limiter with trusted proxy support
func NewRateLimiterWithTrustedProxies(rate, burst int, cleanup time.Duration, trustedProxies []string) *RateLimiter {
	rl := NewRateLimiter(rate, burst, cleanup)
	rl.trustedProxies = parseTrustedProxies(trustedProxies)
	return rl
}

// parseTrustedProxies converts CIDR strings to IPNet slices
func parseTrustedProxies(proxies []string) []*net.IPNet {
	if len(proxies) == 0 {
		return nil
	}

	nets := make([]*net.IPNet, 0, len(proxies))
	for _, proxy := range proxies {
		// Handle single IPs by converting to CIDR
		if !strings.Contains(proxy, "/") {
			// IPv4
			if strings.Contains(proxy, ":") {
				proxy += "/128" // IPv6 single host
			} else {
				proxy += "/32" // IPv4 single host
			}
		}

		_, ipNet, err := net.ParseCIDR(proxy)
		if err == nil {
			nets = append(nets, ipNet)
		}
	}
	return nets
}

// isTrustedProxy checks if an IP is in the trusted proxy list
func (rl *RateLimiter) isTrustedProxy(ip string) bool {
	if len(rl.trustedProxies) == 0 {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for _, trustedNet := range rl.trustedProxies {
		if trustedNet.Contains(parsedIP) {
			return true
		}
	}
	return false
}

// NewDefaultRateLimiter creates a rate limiter with sensible defaults
func NewDefaultRateLimiter() *RateLimiter {
	return NewRateLimiter(DefaultRequestsPerSecond, DefaultBurst, DefaultCleanupInterval)
}

// Allow checks if a request from the given IP is allowed
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Cleanup old buckets periodically
	if time.Since(rl.lastCleanup) > rl.cleanup {
		rl.cleanupOldBuckets()
	}

	now := time.Now()
	bucket, exists := rl.buckets[ip]
	if !exists {
		// Create new bucket
		bucket = &tokenBucket{
			tokens:   float64(rl.burst - 1), // Start with burst-1 tokens (allowing current request)
			lastSeen: now,
		}
		rl.buckets[ip] = bucket
		return true
	}

	// Refill tokens based on time elapsed
	elapsed := now.Sub(bucket.lastSeen).Seconds()
	tokensToAdd := elapsed * float64(rl.rate)
	bucket.tokens = minf(bucket.tokens+tokensToAdd, float64(rl.burst))
	bucket.lastSeen = now

	// Check if we have tokens
	if bucket.tokens >= 1 {
		bucket.tokens -= 1
		return true
	}

	return false
}

// cleanupOldBuckets removes buckets that haven't been used recently
func (rl *RateLimiter) cleanupOldBuckets() {
	threshold := time.Now().Add(-rl.cleanup * 2)
	for ip, bucket := range rl.buckets {
		if bucket.lastSeen.Before(threshold) {
			delete(rl.buckets, ip)
		}
	}
	rl.lastCleanup = time.Now()
}

// min returns the minimum of two floats
func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// GetStats returns current rate limiter statistics
func (rl *RateLimiter) GetStats() (int, int) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return len(rl.buckets), rl.rate
}

// GetClientIP extracts the real client IP from the request.
// If trusted proxies are configured and the direct connection IP is in the trusted list,
// it will extract the client IP from X-Forwarded-For or X-Real-IP headers.
// Otherwise, it returns the direct connection IP.
func (rl *RateLimiter) GetClientIP(directIP string, xff string, xri string) string {
	// If no trusted proxies configured, always use direct IP
	if len(rl.trustedProxies) == 0 {
		return directIP
	}

	// If direct IP is not a trusted proxy, use it directly
	if !rl.isTrustedProxy(directIP) {
		return directIP
	}

	// Direct IP is a trusted proxy - check headers for real client IP
	// Check X-Forwarded-For header (common for proxies)
	if xff != "" {
		// Take the first IP (original client)
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	if xri != "" {
		return xri
	}

	// Headers not available or empty, fall back to direct IP
	return directIP
}

// getClientIPFiber extracts the client IP from Fiber context.
// This function uses the rate limiter's trusted proxy configuration.
// Deprecated: Use RateLimiter.GetClientIP instead for better control.
func (rl *RateLimiter) getClientIPFiber(c interface {
	IP() string
	Get(string) string
}) string {
	return rl.GetClientIP(c.IP(), c.Get("X-Forwarded-For"), c.Get("X-Real-IP"))
}
