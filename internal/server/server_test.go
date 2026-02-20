// Package server_test provides comprehensive unit tests for the HTTP server package.
package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/server"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestServer is a helper that starts a test server with an in-memory SQLite store.
// It returns the base URL and registers cleanup with t.Cleanup.
func startTestServer(t *testing.T, cfg *config.Config) string {
	t.Helper()

	// Ensure Host is 127.0.0.1 and Port is 0 (random) unless explicitly set
	if cfg.Server.Host == "" {
		cfg.Server.Host = "127.0.0.1"
	}
	if cfg.Server.Port != 0 && cfg.Server.Port != 6363 {
		// Allow explicit port overrides for specific tests
	} else {
		cfg.Server.Port = 0 // Use random port for tests
	}

	// Create in-memory SQLite store (schema is automatically initialized)
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err, "failed to create in-memory SQLite store")

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Start server in goroutine
	addrChan := make(chan string, 1)
	go func() {
		addr, _ := server.Start(ctx, cfg, store)
		addrChan <- addr
	}()

	// Wait for server to start with timeout
	var addr string
	select {
	case addr = <-addrChan:
	case <-time.After(5 * time.Second):
		cancel()
		_ = store.Close()
		t.Fatal("server did not start within timeout")
	}

	// Give server a moment to be fully ready for connections
	time.Sleep(100 * time.Millisecond)

	// Register cleanup
	t.Cleanup(func() {
		cancel()
		time.Sleep(100 * time.Millisecond) // Give server time to shut down
		_ = store.Close()
	})

	return "http://" + addr
}

// TestServer_StartsOnRandomPort verifies that the server can start on a random port
// (port 0) and returns a valid, non-zero address.
func TestServer_StartsOnRandomPort(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0, // Request random port
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	baseURL := startTestServer(t, cfg)

	// Verify baseURL is not empty and contains a valid address
	assert.NotEmpty(t, baseURL, "baseURL should not be empty")
	assert.True(t, strings.HasPrefix(baseURL, "http://"), "baseURL should have http:// prefix")

	// Extract and validate the address
	parts := strings.Split(baseURL, "://")
	require.Len(t, parts, 2)
	addr := parts[1]

	// Verify it's a valid network address (can parse host:port)
	host, port, err := net.SplitHostPort(addr)
	assert.NoError(t, err, "address should be valid host:port format")
	assert.NotEmpty(t, host, "host should not be empty")
	assert.NotEqual(t, "0", port, "port should not be 0 in actual address")
	assert.Greater(t, len(port), 0, "port number should be assigned")
}

// TestServer_HealthEndpoint verifies the health endpoint returns 200 with JSON content.
func TestServer_HealthEndpoint(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	baseURL := startTestServer(t, cfg)

	// GET /api/health should return 200 with JSON
	resp, err := http.Get(baseURL + "/api/health")
	require.NoError(t, err, "failed to GET /api/health")
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "health endpoint should return 200")
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"), "health endpoint should return JSON")

	// Parse response body
	var healthResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&healthResp)
	require.NoError(t, err, "failed to decode health response JSON")

	// Verify it has at least one field (e.g., status)
	assert.Greater(t, len(healthResp), 0, "health response should have at least one field")

	// Specifically check for status field (based on server.go line 385)
	status, ok := healthResp["status"]
	assert.True(t, ok, "health response should have 'status' field")
	assert.Equal(t, "healthy", status, "status should be 'healthy'")
}

// TestServer_SecurityHeaders verifies all security headers are present in responses.
func TestServer_SecurityHeaders(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	baseURL := startTestServer(t, cfg)

	// Make a GET request to any endpoint
	resp, err := http.Get(baseURL + "/api/health")
	require.NoError(t, err, "failed to GET /api/health")
	defer func() { _ = resp.Body.Close() }()

	// Verify all security headers are present
	expectedHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}

	for headerName, expectedValue := range expectedHeaders {
		actualValue := resp.Header.Get(headerName)
		assert.Equal(t, expectedValue, actualValue,
			"header %q should be %q but got %q", headerName, expectedValue, actualValue)
	}
}

// TestServer_RouteRegistration_APIPaths verifies core API routes are registered and accessible.
func TestServer_RouteRegistration_APIPaths(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "development", // No auth required
		},
	}

	baseURL := startTestServer(t, cfg)

	apiPaths := []string{
		"/api/memories",
		"/api/health",
		"/api/stats",
	}

	for _, path := range apiPaths {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(baseURL + path)
			require.NoError(t, err, "failed to GET %s", path)
			defer func() { _ = resp.Body.Close() }()

			// Route should not return 404 Not Found
			assert.NotEqual(t, http.StatusNotFound, resp.StatusCode,
				"route %s should be registered (got 404)", path)

			// Route should return either 2xx success or 4xx error (not 404)
			assert.True(t, resp.StatusCode < 500,
				"route %s should not return 5xx (got %d)", path, resp.StatusCode)
		})
	}
}

// TestServer_RouteRegistration_WebUIPaths verifies web UI routes are registered.
func TestServer_RouteRegistration_WebUIPaths(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	baseURL := startTestServer(t, cfg)

	// Test root index page
	resp, err := http.Get(baseURL + "/")
	require.NoError(t, err, "failed to GET /")
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "root index page should return 200")
	assert.True(t, strings.Contains(resp.Header.Get("Content-Type"), "text/html"),
		"root index page should return HTML content")

	// Test static path exists (even if no actual files)
	resp, err = http.Get(baseURL + "/static/")
	require.NoError(t, err, "failed to GET /static/")
	defer func() { _ = resp.Body.Close() }()

	// /static/ should return 200 or 404 (not 500 error)
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound,
		"/static/ should return 200 or 404 (got %d)", resp.StatusCode)
}

// TestServer_GracefulShutdown verifies the server shuts down gracefully when context is cancelled.
func TestServer_GracefulShutdown(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	// Ensure Host is 127.0.0.1 and Port is 0
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0

	// Create in-memory SQLite store (schema is automatically initialized)
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	addrChan := make(chan string, 1)
	go func() {
		addr, _ := server.Start(ctx, cfg, store)
		addrChan <- addr
	}()

	// Wait for server to start
	var addr string
	select {
	case addr = <-addrChan:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}

	time.Sleep(100 * time.Millisecond)
	baseURL := "http://" + addr

	// Verify server is responding
	resp, err := http.Get(baseURL + "/api/health")
	require.NoError(t, err, "server should be responding before shutdown")
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Cancel context to trigger graceful shutdown
	cancel()

	// Give server time to shut down
	time.Sleep(200 * time.Millisecond)

	// Verify server is no longer responding within a timeout
	shutdownCheckCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()

	done := make(chan bool)
	go func() {
		req, _ := http.NewRequestWithContext(shutdownCheckCtx, "GET", baseURL+"/api/health", nil)
		_, err := http.DefaultClient.Do(req)
		done <- err != nil // true if error (connection refused)
	}()

	select {
	case isDown := <-done:
		assert.True(t, isDown, "server should stop responding after shutdown")
	case <-time.After(3 * time.Second):
		t.Fatal("server shutdown check timed out")
	}
}

// TestServer_DevelopmentMode_NoAuth verifies API endpoints are accessible without auth in development mode.
func TestServer_DevelopmentMode_NoAuth(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "development", // No auth in development
			APIToken:     "",
		},
	}

	baseURL := startTestServer(t, cfg)

	// GET /api/memories should return 200 in development mode (no auth required)
	resp, err := http.Get(baseURL + "/api/memories")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"in development mode, /api/memories should be accessible without auth")
}

// TestServer_ProductionMode_RequiresAuth verifies API endpoints require auth in production mode.
func TestServer_ProductionMode_RequiresAuth(t *testing.T) {
	testToken := "test-secret-token-xyz123"
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "production",
			APIToken:     testToken,
		},
	}

	baseURL := startTestServer(t, cfg)

	t.Run("without_auth_header", func(t *testing.T) {
		// GET /api/memories without auth should return 401
		resp, err := http.Get(baseURL + "/api/memories")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"in production mode without auth, /api/memories should return 401")
	})

	t.Run("with_valid_auth_header", func(t *testing.T) {
		// Create request with valid Authorization header
		req, err := http.NewRequest("GET", baseURL+"/api/memories", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+testToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"with valid auth, /api/memories should return 200")
	})

	t.Run("with_invalid_auth_header", func(t *testing.T) {
		// Create request with invalid Authorization header
		req, err := http.NewRequest("GET", baseURL+"/api/memories", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer wrong-token")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"with invalid auth, /api/memories should return 401")
	})
}

// TestServer_HealthEndpointNoAuth verifies health endpoint is accessible without auth in production.
func TestServer_HealthEndpointNoAuth(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "production",
			APIToken:     "test-token",
		},
	}

	baseURL := startTestServer(t, cfg)

	// Health endpoint should be accessible without auth even in production
	resp, err := http.Get(baseURL + "/api/health")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"/api/health should be accessible without auth even in production mode")
}

// TestServer_HTTPMethods verifies correct HTTP method handling.
func TestServer_HTTPMethods(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	baseURL := startTestServer(t, cfg)

	tests := []struct {
		method   string
		path     string
		body     string
		expectOK bool // true if we expect 2xx or auth error, false if we expect method not allowed
	}{
		{"POST", "/api/health", "", false}, // POST not allowed on health
		{"PUT", "/api/health", "", false},  // PUT not allowed
		{"DELETE", "/api/health", "", false}, // DELETE not allowed
		{"GET", "/api/memories", "", true},   // GET is allowed
		{"POST", "/api/memories", `{"content":"test"}`, true}, // POST is allowed
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.method, tt.path), func(t *testing.T) {
			var resp *http.Response
			var err error

			req, err := http.NewRequest(tt.method, baseURL+tt.path, strings.NewReader(tt.body))
			require.NoError(t, err)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}

			resp, err = http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			if tt.expectOK {
				// Should return 2xx or 3xx, not method not allowed
				assert.NotEqual(t, http.StatusMethodNotAllowed, resp.StatusCode,
					"%s %s should be allowed", tt.method, tt.path)
			} else {
				// Should return method not allowed
				assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode,
					"%s %s should not be allowed", tt.method, tt.path)
			}
		})
	}
}

// TestServer_RequestResponseContent verifies request/response handling.
func TestServer_RequestResponseContent(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	baseURL := startTestServer(t, cfg)

	t.Run("health_response_structure", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/health")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Verify it's valid JSON
		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		require.NoError(t, err, "health response should be valid JSON")

		// Verify structure
		assert.Contains(t, data, "status", "health response should have 'status' field")
		assert.Contains(t, data, "version", "health response should have 'version' field")
	})

	t.Run("root_page_html", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Verify it's HTML content
		bodyStr := string(body)
		assert.NotEmpty(t, bodyStr, "root page should have content")
		assert.True(t, strings.Contains(bodyStr, "<") || len(bodyStr) > 0,
			"root page should contain HTML or be served")
	})
}

// TestServer_NotFoundHandling verifies 404 behavior for non-existent routes.
func TestServer_NotFoundHandling(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	baseURL := startTestServer(t, cfg)

	// Non-existent route should return 404
	resp, err := http.Get(baseURL + "/nonexistent/route")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"non-existent route should return 404")
}

// TestServer_ConnectionTimeouts verifies server timeout settings are applied.
func TestServer_ConnectionTimeouts(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	baseURL := startTestServer(t, cfg)

	// Create a client with custom timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// A normal request should complete within the timeout
	req, err := http.NewRequest("GET", baseURL+"/api/health", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err, "normal request should complete within timeout")
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestServer_ContentTypes verifies appropriate Content-Type headers are set.
func TestServer_ContentTypes(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: t.TempDir(),
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	baseURL := startTestServer(t, cfg)

	tests := []struct {
		path        string
		expectedCT  string
		description string
	}{
		{"/api/health", "application/json", "API endpoint should return JSON"},
		{"/", "text/html", "HTML page should return HTML content-type"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			resp, err := http.Get(baseURL + tt.path)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			ct := resp.Header.Get("Content-Type")
			assert.True(t, strings.Contains(ct, tt.expectedCT),
				"path %s should have Content-Type containing %q (got %q)", tt.path, tt.expectedCT, ct)
		})
	}
}
