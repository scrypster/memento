package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/web/handlers"
	"github.com/stretchr/testify/assert"
)

func TestRequireAuth_SkipInDevelopmentMode(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			SecurityMode: "development",
			APIToken:     "secret",
		},
	}

	handler := handlers.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), cfg)

	req := httptest.NewRequest("GET", "/api/memories", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireAuth_RejectMissingToken(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			SecurityMode: "production",
			APIToken:     "secret",
		},
	}

	handler := handlers.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), cfg)

	req := httptest.NewRequest("GET", "/api/memories", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "unauthorized")
}

func TestRequireAuth_AcceptValidToken(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			SecurityMode: "production",
			APIToken:     "secret-token",
		},
	}

	handler := handlers.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), cfg)

	req := httptest.NewRequest("GET", "/api/memories", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRateLimitMiddleware_AllowsNormalRate(t *testing.T) {
	limiter := handlers.NewRateLimiter(10, 20) // 10 req/s, burst 20
	handler := handlers.RateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), limiter)

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/api/search", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
}

func TestRateLimitMiddleware_RejectsExcessiveRate(t *testing.T) {
	limiter := handlers.NewRateLimiter(1, 2) // 1 req/s, burst 2
	handler := handlers.RateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), limiter)

	// First 2 should succeed (burst)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/search", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Third should be rate limited
	req := httptest.NewRequest("GET", "/api/search", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}
