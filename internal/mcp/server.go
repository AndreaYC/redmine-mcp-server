package mcp

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/ycho/redmine-mcp-server/internal/redmine"
)

const (
	ServerName    = "redmine-mcp-server"
	ServerVersion = "1.0.0"
)

// Config holds MCP server configuration
type Config struct {
	RedmineURL    string
	RedmineAPIKey string
	Port          int
	SSEMode       bool
}

// Server wraps the MCP server
type Server struct {
	config  Config
	mcp     *server.MCPServer
	handler *ToolHandlers
}

// NewServer creates a new MCP server
func NewServer(config Config) *Server {
	return &Server{
		config: config,
	}
}

// Run starts the MCP server
func (s *Server) Run() error {
	// Create MCP server
	s.mcp = server.NewMCPServer(
		ServerName,
		ServerVersion,
		server.WithToolCapabilities(false),
	)

	if s.config.SSEMode {
		// SSE mode - create client per request from header
		return s.runSSE()
	}

	// Stdio mode - use env var for API key
	client := redmine.NewClient(s.config.RedmineURL, s.config.RedmineAPIKey)
	s.handler = NewToolHandlers(client)
	s.handler.RegisterTools(s.mcp)

	slog.Info("Starting MCP server in stdio mode",
		"redmine_url", s.config.RedmineURL,
	)

	return server.ServeStdio(s.mcp)
}

// runSSE starts the server in SSE mode
func (s *Server) runSSE() error {
	addr := fmt.Sprintf(":%d", s.config.Port)

	slog.Info("Starting MCP server in SSE mode",
		"address", addr,
		"redmine_url", s.config.RedmineURL,
	)

	// Create a custom SSE handler that extracts API key from header
	sseHandler := &sseHandler{
		redmineURL: s.config.RedmineURL,
	}

	// Rate limiter: 100 requests per minute per IP
	rateLimiter := newSimpleRateLimiter(100, time.Minute)

	mux := http.NewServeMux()
	mux.Handle("/sse", sseHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Apply middleware chain
	handler := securityHeadersMiddleware(rateLimiter.middleware(mux))

	return http.ListenAndServe(addr, handler)
}

// sseHandler handles SSE connections with per-request API key
type sseHandler struct {
	redmineURL string
}

func (h *sseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get API key from header
	apiKey := r.Header.Get("X-Redmine-API-Key")
	if apiKey == "" {
		http.Error(w, "Missing X-Redmine-API-Key header", http.StatusUnauthorized)
		return
	}

	// Create client for this request
	client := redmine.NewClient(h.redmineURL, apiKey)

	// Create MCP server for this connection
	mcpServer := server.NewMCPServer(
		ServerName,
		ServerVersion,
		server.WithToolCapabilities(false),
	)

	// Register tools
	handler := NewToolHandlers(client)
	handler.RegisterTools(mcpServer)

	// Create SSE server and handle the connection
	sseServer := server.NewSSEServer(mcpServer)
	sseServer.ServeHTTP(w, r)
}

// GetEnvConfig gets configuration from environment variables
func GetEnvConfig() Config {
	config := Config{
		RedmineURL:    os.Getenv("REDMINE_URL"),
		RedmineAPIKey: os.Getenv("REDMINE_API_KEY"),
		Port:          8080,
	}

	if port := os.Getenv("PORT"); port != "" {
		_, _ = fmt.Sscanf(port, "%d", &config.Port)
	}

	return config
}

// securityHeaders middleware adds security headers
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// simpleRateLimiter for SSE mode
type simpleRateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func newSimpleRateLimiter(limit int, window time.Duration) *simpleRateLimiter {
	return &simpleRateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (rl *simpleRateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// Filter old requests
	var recent []time.Time
	for _, t := range rl.requests[key] {
		if t.After(windowStart) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= rl.limit {
		rl.requests[key] = recent
		return false
	}

	rl.requests[key] = append(recent, now)
	return true
}

func (rl *simpleRateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.RemoteAddr
		if !rl.allow(key) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
