// Package server provides tests for the rate limiter
package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10, 20, time.Minute)
	assert.NotNil(t, rl)
	assert.Equal(t, 10, rl.rate)
	assert.Equal(t, 20, rl.burst)
	assert.NotNil(t, rl.buckets)
}

func TestNewRateLimiterWithTrustedProxies(t *testing.T) {
	tests := []struct {
		name           string
		trustedProxies []string
		expectCount    int
	}{
		{
			name:           "no trusted proxies",
			trustedProxies: nil,
			expectCount:    0,
		},
		{
			name:           "empty trusted proxies",
			trustedProxies: []string{},
			expectCount:    0,
		},
		{
			name:           "single IPv4",
			trustedProxies: []string{"192.168.1.1"},
			expectCount:    1,
		},
		{
			name:           "IPv4 CIDR",
			trustedProxies: []string{"192.168.1.0/24"},
			expectCount:    1,
		},
		{
			name:           "multiple proxies",
			trustedProxies: []string{"192.168.1.0/24", "10.0.0.1", "172.16.0.0/16"},
			expectCount:    3,
		},
		{
			name:           "IPv6 address",
			trustedProxies: []string{"::1"},
			expectCount:    1,
		},
		{
			name:           "IPv6 CIDR",
			trustedProxies: []string{"2001:db8::/32"},
			expectCount:    1,
		},
		{
			name:           "invalid proxy ignored",
			trustedProxies: []string{"not-a-valid-ip", "192.168.1.0/24"},
			expectCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiterWithTrustedProxies(10, 20, time.Minute, tt.trustedProxies)
			assert.NotNil(t, rl)
			assert.Len(t, rl.trustedProxies, tt.expectCount)
		})
	}
}

func TestParseTrustedProxies(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected int // number of parsed networks
	}{
		{"nil input", nil, 0},
		{"empty input", []string{}, 0},
		{"single IPv4", []string{"192.168.1.1"}, 1},
		{"IPv4 CIDR", []string{"192.168.1.0/24"}, 1},
		{"IPv6 single", []string{"::1"}, 1},
		{"IPv6 CIDR", []string{"2001:db8::/32"}, 1},
		{"multiple valid", []string{"192.168.1.0/24", "10.0.0.1"}, 2},
		{"invalid ignored", []string{"invalid", "192.168.1.0/24"}, 1},
		{"all invalid", []string{"invalid", "also-invalid"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTrustedProxies(tt.input)
			assert.Len(t, result, tt.expected)
		})
	}
}

func TestIsTrustedProxy(t *testing.T) {
	rl := NewRateLimiterWithTrustedProxies(10, 20, time.Minute, []string{"192.168.1.0/24", "10.0.0.1"})

	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"in CIDR range", "192.168.1.100", true},
		{"exact match", "10.0.0.1", true},
		{"outside CIDR range", "192.168.2.1", false},
		{"different subnet", "172.16.0.1", false},
		{"invalid IP", "not-an-ip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rl.isTrustedProxy(tt.ip)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Test with no trusted proxies
	t.Run("no trusted proxies configured", func(t *testing.T) {
		rlNoProxies := NewRateLimiter(10, 20, time.Minute)
		assert.False(t, rlNoProxies.isTrustedProxy("192.168.1.1"))
	})
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name           string
		trustedProxies []string
		directIP       string
		xff            string
		xri            string
		expected       string
	}{
		{
			name:           "no trusted proxies - use direct IP",
			trustedProxies: nil,
			directIP:       "192.168.1.1",
			xff:            "1.2.3.4",
			expected:       "192.168.1.1",
		},
		{
			name:           "direct IP not trusted - use direct IP",
			trustedProxies: []string{"10.0.0.1"},
			directIP:       "192.168.1.1",
			xff:            "1.2.3.4",
			expected:       "192.168.1.1",
		},
		{
			name:           "direct IP trusted - use XFF",
			trustedProxies: []string{"192.168.1.0/24"},
			directIP:       "192.168.1.100",
			xff:            "1.2.3.4, 192.168.1.100",
			expected:       "1.2.3.4",
		},
		{
			name:           "direct IP trusted - use X-Real-IP",
			trustedProxies: []string{"192.168.1.0/24"},
			directIP:       "192.168.1.100",
			xff:            "",
			xri:            "5.6.7.8",
			expected:       "5.6.7.8",
		},
		{
			name:           "direct IP trusted - prefer XFF over X-Real-IP",
			trustedProxies: []string{"192.168.1.0/24"},
			directIP:       "192.168.1.100",
			xff:            "1.2.3.4",
			xri:            "5.6.7.8",
			expected:       "1.2.3.4",
		},
		{
			name:           "direct IP trusted - empty headers fall back to direct",
			trustedProxies: []string{"192.168.1.0/24"},
			directIP:       "192.168.1.100",
			xff:            "",
			xri:            "",
			expected:       "192.168.1.100",
		},
		{
			name:           "XFF with multiple IPs - take first",
			trustedProxies: []string{"192.168.1.0/24"},
			directIP:       "192.168.1.100",
			xff:            "1.2.3.4, 192.168.1.100, 10.0.0.1",
			expected:       "1.2.3.4",
		},
		{
			name:           "XFF with spaces",
			trustedProxies: []string{"192.168.1.0/24"},
			directIP:       "192.168.1.100",
			xff:            "  1.2.3.4  , 192.168.1.100",
			expected:       "1.2.3.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiterWithTrustedProxies(10, 20, time.Minute, tt.trustedProxies)
			result := rl.GetClientIP(tt.directIP, tt.xff, tt.xri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(2, 5, time.Minute) // 2 requests/sec, burst of 5

	// First request should always be allowed
	assert.True(t, rl.Allow("192.168.1.1"))

	// Exhaust burst
	for i := 0; i < 4; i++ {
		assert.True(t, rl.Allow("192.168.1.1"), "request %d should be allowed", i+2)
	}

	// Should be rate limited now
	assert.False(t, rl.Allow("192.168.1.1"))

	// Different IP should still be allowed
	assert.True(t, rl.Allow("192.168.1.2"))
}

func TestRateLimiterCleanup(t *testing.T) {
	rl := NewRateLimiter(1, 10, 10*time.Millisecond) // Very short cleanup for testing

	// Add some buckets
	rl.Allow("192.168.1.1")
	rl.Allow("192.168.1.2")

	// Wait for cleanup
	time.Sleep(30 * time.Millisecond)

	// Force cleanup by making a new request
	rl.Allow("192.168.1.3")

	rl.mu.RLock()
	bucketCount := len(rl.buckets)
	rl.mu.RUnlock()

	// Old buckets should be cleaned up
	assert.LessOrEqual(t, bucketCount, 1, "old buckets should be cleaned up")
}

func TestRateLimiterConcurrent(t *testing.T) {
	rl := NewRateLimiter(100, 1000, time.Minute)

	// Test concurrent access
	done := make(chan bool)

	for i := 0; i < 100; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				rl.Allow("192.168.1.1")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Should not panic or race
	assert.True(t, true)
}

func TestRateLimiterStats(t *testing.T) {
	rl := NewRateLimiter(10, 20, time.Minute)

	// No buckets initially
	count, rate := rl.GetStats()
	assert.Equal(t, 0, count)
	assert.Equal(t, 10, rate)

	// Add some buckets
	rl.Allow("192.168.1.1")
	rl.Allow("192.168.1.2")

	count, rate = rl.GetStats()
	assert.Equal(t, 2, count)
	assert.Equal(t, 10, rate)
}
