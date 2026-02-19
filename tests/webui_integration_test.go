// Package tests provides comprehensive integration tests for the Memento Web UI.
//
// This file contains end-to-end tests that verify:
// - Full workflow: server startup, API operations, WebSocket connections
// - Security: authentication middleware, rate limiting, origin validation
// - CRUD operations: create, read, update, delete memories
// - Asset serving: vendor files (Alpine.js, Tailwind CSS) are accessible
//
// Tests use ephemeral servers with random ports (port 0) to avoid conflicts.
package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/server"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

func TestWebUI_FullWorkflow(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0, // random port
		},
		Storage: config.StorageConfig{
			DataPath: tmpDir,
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	store, err := sqlite.NewMemoryStore(tmpDir + "/test.db")
	require.NoError(t, err)
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	serverReady := make(chan string, 1)
	go func() {
		addr := server.Start(ctx, cfg, store)
		serverReady <- addr
	}()

	var baseURL string
	select {
	case addr := <-serverReady:
		baseURL = "http://" + addr
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start in time")
	}

	// Give server a moment to be ready
	time.Sleep(100 * time.Millisecond)

	// Test 1: Index page loads
	resp, err := http.Get(baseURL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	// Test 2: WebSocket connects
	// Note: We don't send an Origin header in tests since the WebSocket handler
	// has hardcoded allowed origins (localhost:6363, 127.0.0.1:6363).
	// In real usage, the browser will send the correct origin.
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/ws"
	wsCtx, wsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer wsCancel()

	conn, _, err := websocket.Dial(wsCtx, wsURL, nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "test complete")

	// Test 3: Create memory via API
	memoryJSON := `{"content":"Test memory for integration test","source":"integration_test"}`
	resp, err = http.Post(
		baseURL+"/api/memories",
		"application/json",
		strings.NewReader(memoryJSON),
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var created map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&created)
	require.NoError(t, err)
	assert.NotEmpty(t, created["id"])

	// Test 4: List memories
	resp, err = http.Get(baseURL + "/api/memories")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result storage.PaginatedResult[map[string]interface{}]
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
}

func TestWebUI_SecurityValidation(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: tmpDir,
		},
		Security: config.SecurityConfig{
			SecurityMode: "production",
			APIToken:     "test-secret-token-12345",
		},
	}

	store, err := sqlite.NewMemoryStore(tmpDir + "/test.db")
	require.NoError(t, err)
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverReady := make(chan string, 1)
	go func() {
		addr := server.Start(ctx, cfg, store)
		serverReady <- addr
	}()

	var baseURL string
	select {
	case addr := <-serverReady:
		baseURL = "http://" + addr
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}

	time.Sleep(100 * time.Millisecond)

	// Test 1: Unauthorized request fails
	resp, err := http.Get(baseURL + "/api/memories")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Test 2: Authorized request succeeds
	req, err := http.NewRequest("GET", baseURL+"/api/memories", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-secret-token-12345")

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test 3: WebSocket with invalid origin fails
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/ws"
	wsCtx, wsCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer wsCancel()

	_, resp, err = websocket.Dial(wsCtx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{"http://evil.com:9999"},
		},
	})
	// Should fail with 403 Forbidden
	assert.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		resp.Body.Close()
	}
}

func TestWebUI_RateLimiting(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: tmpDir,
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	store, err := sqlite.NewMemoryStore(tmpDir + "/test.db")
	require.NoError(t, err)
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverReady := make(chan string, 1)
	go func() {
		addr := server.Start(ctx, cfg, store)
		serverReady <- addr
	}()

	var baseURL string
	select {
	case addr := <-serverReady:
		baseURL = "http://" + addr
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}

	time.Sleep(100 * time.Millisecond)

	// Send 25 rapid requests (rate limit is 10 req/s with burst of 20)
	rateLimited := false
	for i := 0; i < 25; i++ {
		resp, err := http.Get(baseURL + "/api/memories")
		require.NoError(t, err)

		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimited = true
			resp.Body.Close()
			break
		}
		resp.Body.Close()
	}

	assert.True(t, rateLimited, "Expected to hit rate limit after 20 requests")
}

func TestWebUI_AssetVerification(t *testing.T) {
	// Verify vendor assets exist (resolve relative to project root)
	projectRoot := filepath.Dir(mustGetwd(t))
	alpinePath := filepath.Join(projectRoot, "web/static/vendor/alpine-3.14.9.min.js")
	tailwindPath := filepath.Join(projectRoot, "web/static/dist/assets/main.css")

	// Check Alpine.js
	alpineInfo, err := os.Stat(alpinePath)
	require.NoError(t, err, "Alpine.js not found - run 'make vendor-assets'")
	assert.Greater(t, alpineInfo.Size(), int64(10000), "Alpine.js file too small")

	// Check Tailwind CSS
	tailwindInfo, err := os.Stat(tailwindPath)
	require.NoError(t, err, "CSS bundle not found - run 'make assets'")
	assert.Greater(t, tailwindInfo.Size(), int64(1000), "CSS bundle file too small")

	// Verify they're accessible via HTTP
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: tmpDir,
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	store, err := sqlite.NewMemoryStore(tmpDir + "/test.db")
	require.NoError(t, err)
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverReady := make(chan string, 1)
	go func() {
		addr := server.Start(ctx, cfg, store)
		serverReady <- addr
	}()

	var baseURL string
	select {
	case addr := <-serverReady:
		baseURL = "http://" + addr
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}

	time.Sleep(100 * time.Millisecond)

	// Test Alpine.js HTTP access
	resp, err := http.Get(baseURL + "/static/vendor/alpine-3.14.9.min.js")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "javascript")

	// Test CSS bundle HTTP access
	resp, err = http.Get(baseURL + "/static/dist/assets/main.css")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "css")
}

func TestWebUI_CRUD_Operations(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: tmpDir,
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	store, err := sqlite.NewMemoryStore(tmpDir + "/test.db")
	require.NoError(t, err)
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverReady := make(chan string, 1)
	go func() {
		addr := server.Start(ctx, cfg, store)
		serverReady <- addr
	}()

	var baseURL string
	select {
	case addr := <-serverReady:
		baseURL = "http://" + addr
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start in time")
	}

	time.Sleep(100 * time.Millisecond)

	// Test 1: Create a memory
	memoryJSON := `{"content":"Test CRUD memory","source":"test","domain":"test"}`
	resp, err := http.Post(
		baseURL+"/api/memories",
		"application/json",
		strings.NewReader(memoryJSON),
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&created)
	require.NoError(t, err)
	memoryID := created["id"].(string)
	assert.NotEmpty(t, memoryID)

	// Test 2: Get the memory
	resp, err = http.Get(baseURL + "/api/memories/" + memoryID)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var retrieved map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&retrieved)
	require.NoError(t, err)
	assert.Equal(t, memoryID, retrieved["id"])
	assert.Equal(t, "Test CRUD memory", retrieved["content"])

	// Test 3: Update the memory
	updateJSON := `{"content":"Updated CRUD memory"}`
	req, err := http.NewRequest("PATCH", baseURL+"/api/memories/"+memoryID, strings.NewReader(updateJSON))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		t.Logf("Update error response (status %d): %+v", resp.StatusCode, errResp)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var updated map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&updated)
	require.NoError(t, err)
	assert.Equal(t, "Updated CRUD memory", updated["content"])

	// Test 4: List memories (should have 1)
	resp, err = http.Get(baseURL + "/api/memories")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result storage.PaginatedResult[map[string]interface{}]
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)

	// Test 5: Delete the memory
	req, err = http.NewRequest("DELETE", baseURL+"/api/memories/"+memoryID, nil)
	require.NoError(t, err)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Test 6: Verify deletion (should be 404)
	resp, err = http.Get(baseURL + "/api/memories/" + memoryID)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Test 7: List memories (should be empty)
	resp, err = http.Get(baseURL + "/api/memories")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	return wd
}
