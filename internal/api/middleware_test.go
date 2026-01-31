package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSecurityHeaders(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	tests := []struct {
		header string
		want   string
	}{
		{"X-Frame-Options", "DENY"},
		{"X-Content-Type-Options", "nosniff"},
		{"X-XSS-Protection", "1; mode=block"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'"},
		{"Permissions-Policy", "geolocation=(), microphone=(), camera=()"},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := w.Header().Get(tt.header)
			if got != tt.want {
				t.Errorf("%s = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(5, time.Second, 5)

	// Should allow first 5 requests
	for i := 0; i < 5; i++ {
		if !rl.Allow("test-key") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied
	if rl.Allow("test-key") {
		t.Error("6th request should be denied")
	}

	// Different key should be allowed
	if !rl.Allow("other-key") {
		t.Error("Different key should be allowed")
	}
}

func TestRateLimiter_Middleware(t *testing.T) {
	rl := NewRateLimiter(2, time.Second, 2)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: expected %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("3rd request: expected %d, got %d", http.StatusTooManyRequests, w.Code)
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := NewRateLimiter(10, time.Millisecond, 10)

	// Add some entries
	rl.Allow("key1")
	rl.Allow("key2")

	// Wait for entries to become stale
	time.Sleep(10 * time.Millisecond)

	// Cleanup should remove stale entries
	rl.Cleanup(5 * time.Millisecond)

	rl.mu.Lock()
	count := len(rl.tokens)
	rl.mu.Unlock()

	if count != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", count)
	}
}
