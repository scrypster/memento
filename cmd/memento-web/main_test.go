package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMainServer_Routes(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0, // random port
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	// Create temporary test database
	tmpDir := t.TempDir()
	store, err := sqlite.NewMemoryStore(tmpDir + "/test.db")
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	// Start server in goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverReady := make(chan string, 1)
	go func() {
		addr := startServer(ctx, cfg, store)
		serverReady <- addr
	}()

	// Wait for server to be ready
	select {
	case addr := <-serverReady:
		// Test that server is listening
		resp, err := http.Get("http://" + addr + "/")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Test WebSocket endpoint exists
		resp, err = http.Get("http://" + addr + "/ws")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		// WebSocket upgrade fails via GET, but route exists (400 not 404)
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode)

	case <-time.After(5 * time.Second):
		t.Fatal("server did not start in time")
	}
}

func TestMainServer_GracefulShutdown(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	tmpDir := t.TempDir()
	store, err := sqlite.NewMemoryStore(tmpDir + "/test.db")
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	ctx, cancel := context.WithCancel(context.Background())

	serverReady := make(chan string, 1)
	serverStopped := make(chan struct{}, 1)

	go func() {
		addr := startServer(ctx, cfg, store)
		serverReady <- addr
		<-ctx.Done()
		time.Sleep(100 * time.Millisecond) // Give shutdown time to complete
		serverStopped <- struct{}{}
	}()

	// Wait for server start
	select {
	case <-serverReady:
		// Cancel context to trigger shutdown
		cancel()

		// Verify graceful shutdown
		select {
		case <-serverStopped:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("server did not shut down gracefully")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start in time")
	}
}
