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

	// Create session manager for multi-tenant SSE
	sessionMgr := newSessionManager(s.config.RedmineURL)

	// Rate limiter: 100 requests per minute per IP
	rateLimiter := newSimpleRateLimiter(100, time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", sessionMgr.handleSSE)
	mux.HandleFunc("/message", sessionMgr.handleMessage)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Apply middleware chain
	handler := securityHeadersMiddleware(rateLimiter.middleware(mux))

	return http.ListenAndServe(addr, handler)
}

// sessionManager manages SSE sessions for multi-tenant access
type sessionManager struct {
	mu         sync.RWMutex
	servers    map[string]*server.SSEServer // API key -> SSE server
	redmineURL string
}

func newSessionManager(redmineURL string) *sessionManager {
	return &sessionManager{
		servers:    make(map[string]*server.SSEServer),
		redmineURL: redmineURL,
	}
}

func (m *sessionManager) getOrCreateServer(apiKey string) *server.SSEServer {
	m.mu.RLock()
	if srv, ok := m.servers[apiKey]; ok {
		m.mu.RUnlock()
		return srv
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if srv, ok := m.servers[apiKey]; ok {
		return srv
	}

	// Create client for this API key
	client := redmine.NewClient(m.redmineURL, apiKey)

	// Create MCP server
	mcpServer := server.NewMCPServer(
		ServerName,
		ServerVersion,
		server.WithToolCapabilities(false),
	)

	// Register tools
	handler := NewToolHandlers(client)
	handler.RegisterTools(mcpServer)

	// Create SSE server
	sseServer := server.NewSSEServer(mcpServer, server.WithBaseURL(""))
	m.servers[apiKey] = sseServer

	slog.Info("Created new SSE server for API key", "key_prefix", apiKey[:8]+"...")

	return sseServer
}

func (m *sessionManager) handleSSE(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("X-Redmine-API-Key")
	if apiKey == "" {
		http.Error(w, "Missing X-Redmine-API-Key header", http.StatusUnauthorized)
		return
	}

	sseServer := m.getOrCreateServer(apiKey)
	sseServer.ServeHTTP(w, r)
}

func (m *sessionManager) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	apiKey := r.Header.Get("X-Redmine-API-Key")
	if apiKey == "" {
		http.Error(w, "Missing X-Redmine-API-Key header", http.StatusUnauthorized)
		return
	}

	sseServer := m.getOrCreateServer(apiKey)
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
