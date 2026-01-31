package api

import (
	"net/http"
	"sync"
	"time"
)

// securityHeaders adds security headers to all responses
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// XSS protection
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Referrer policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy for API
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

		// Permissions Policy
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		next.ServeHTTP(w, r)
	})
}

// RateLimiter implements a simple token bucket rate limiter
type RateLimiter struct {
	mu        sync.Mutex
	tokens    map[string]*bucket
	rate      int           // tokens per interval
	interval  time.Duration // refill interval
	maxTokens int           // max burst size
}

type bucket struct {
	tokens     int
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter
// rate: number of requests allowed per interval
// interval: time period for rate limiting
// maxTokens: maximum burst size
func NewRateLimiter(rate int, interval time.Duration, maxTokens int) *RateLimiter {
	return &RateLimiter{
		tokens:    make(map[string]*bucket),
		rate:      rate,
		interval:  interval,
		maxTokens: maxTokens,
	}
}

// Allow checks if a request from the given key is allowed
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.tokens[key]

	if !exists {
		rl.tokens[key] = &bucket{
			tokens:     rl.maxTokens - 1,
			lastRefill: now,
		}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastRefill)
	refillCount := int(elapsed / rl.interval) * rl.rate
	if refillCount > 0 {
		b.tokens += refillCount
		if b.tokens > rl.maxTokens {
			b.tokens = rl.maxTokens
		}
		b.lastRefill = now
	}

	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}

// Middleware returns an HTTP middleware for rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use IP + API key as the rate limit key
		key := r.RemoteAddr
		if apiKey := r.Header.Get("X-Redmine-API-Key"); apiKey != "" {
			// Use first 8 chars of API key to avoid storing full key
			if len(apiKey) > 8 {
				key = apiKey[:8]
			} else {
				key = apiKey
			}
		}

		if !rl.Allow(key) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, `{"error": "Rate limit exceeded. Please slow down."}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Cleanup removes stale entries from the rate limiter
// Call this periodically to prevent memory leaks
func (rl *RateLimiter) Cleanup(maxAge time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, b := range rl.tokens {
		if now.Sub(b.lastRefill) > maxAge {
			delete(rl.tokens, key)
		}
	}
}
