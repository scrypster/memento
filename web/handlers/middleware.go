// Package handlers provides HTTP handlers and middleware for the Memento Web UI.
package handlers

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/scrypster/memento/internal/config"
	"golang.org/x/time/rate"
)

// RequireAuth is middleware that enforces API token authentication in production mode.
// In development mode, all requests are allowed through.
func RequireAuth(next http.Handler, cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth in development mode
		if cfg.Security.SecurityMode == "development" {
			next.ServeHTTP(w, r)
			return
		}

		// Require Bearer token in production
		auth := r.Header.Get("Authorization")
		expectedToken := cfg.Security.APIToken
		if expectedToken == "" {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"unauthorized","code":"UNAUTHORIZED"}`,
				http.StatusUnauthorized)
			return
		}

		// Extract token from Authorization header using constant-time comparison
		token := strings.TrimPrefix(auth, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"unauthorized","code":"UNAUTHORIZED"}`,
				http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RateLimiter wraps a rate.Limiter for HTTP middleware.
type RateLimiter struct {
	limiter *rate.Limiter
}

// NewRateLimiter creates a new rate limiter.
// reqPerSec is the sustained rate, burst is the maximum burst size.
func NewRateLimiter(reqPerSec float64, burst int) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Every(time.Duration(1000.0/reqPerSec)*time.Millisecond), burst),
	}
}

// RateLimitMiddleware enforces rate limiting on HTTP requests.
func RateLimitMiddleware(next http.Handler, rl *RateLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.limiter.Allow() {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"rate limit exceeded","code":"RATE_LIMITED"}`,
				http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
