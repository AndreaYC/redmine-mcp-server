package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthCheck(t *testing.T) {
	server := NewServer(Config{
		RedmineURL: "http://localhost",
		Port:       8080,
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "OK" {
		t.Errorf("expected body 'OK', got '%s'", w.Body.String())
	}
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	server := NewServer(Config{
		RedmineURL: "http://localhost",
		Port:       8080,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAuthMiddleware_WithHeader(t *testing.T) {
	// Mock Redmine server
	mockRedmine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/users/current.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"user": {"id": 1, "login": "test", "firstname": "Test", "lastname": "User"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockRedmine.Close()

	server := NewServer(Config{
		RedmineURL: mockRedmine.URL,
		Port:       8080,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("X-Redmine-API-Key", "test-key")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestSwaggerDocs(t *testing.T) {
	server := NewServer(Config{
		RedmineURL: "http://localhost",
		Port:       8080,
	})

	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/yaml" {
		t.Errorf("expected Content-Type 'application/yaml', got '%s'", contentType)
	}
}
