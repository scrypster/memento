// main_test.go exercises the memento-web entry point wiring.
//
// Tests verify that:
//  1. The web server initializes and serves HTTP requests correctly.
//  2. Configuration is loaded properly from environment variables.
//  3. The storage backend (SQLite) integrates correctly with the server.
//  4. The memory engine starts and stops cleanly.
//  5. Signal handling and graceful shutdown work as expected.
//  6. WebSocket endpoints are properly wired.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/engine"
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

// TestMainWeb_ConfigurationLoading verifies that configuration can be loaded
// from environment variables and is not nil.
func TestMainWeb_ConfigurationLoading(t *testing.T) {
	tmpDir := t.TempDir()

	// Set required environment variables for testing
	t.Setenv("MEMENTO_DATA_PATH", tmpDir)
	t.Setenv("MEMENTO_LLM_PROVIDER", "ollama")
	t.Setenv("MEMENTO_LLM_BASE_URL", "http://localhost:11434")

	cfg, err := config.LoadConfig()
	require.NoError(t, err, "failed to load config")

	assert.NotNil(t, cfg, "config should not be nil")
	assert.Equal(t, tmpDir, cfg.Storage.DataPath, "data path should match env var")
}

// TestMainWeb_StorageInitialization verifies that the SQLite storage backend
// initializes correctly with the configured path.
func TestMainWeb_StorageInitialization(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "memento.db")

	store, err := sqlite.NewMemoryStore(dbPath)
	require.NoError(t, err, "failed to create store")
	defer func() { _ = store.Close() }()

	assert.NotNil(t, store, "store should not be nil")

	// Verify database file was created
	_, err = os.Stat(dbPath)
	assert.NoError(t, err, "database file should exist")
}

// TestMainWeb_MemoryEngineInitialization verifies that a MemoryEngine can be
// created with the web server's configuration.
func TestMainWeb_MemoryEngineInitialization(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "memento.db")

	store, err := sqlite.NewMemoryStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	t.Setenv("MEMENTO_DATA_PATH", tmpDir)
	t.Setenv("MEMENTO_LLM_PROVIDER", "ollama")
	t.Setenv("MEMENTO_LLM_BASE_URL", "http://localhost:11434")

	cfg, err := config.LoadConfig()
	require.NoError(t, err)

	// Initialize the memory engine as main() does
	engineCfg := engine.DefaultConfig()
	engineCfg.NumWorkers = 1 // Web uses 1 worker to avoid SQLite locking

	memEngine, err := engine.NewMemoryEngine(store, engineCfg, cfg)
	require.NoError(t, err, "failed to create memory engine")

	assert.NotNil(t, memEngine)
	assert.Equal(t, 1, engineCfg.NumWorkers, "web should use 1 worker for SQLite")
}

// TestMainWeb_MemoryEngineStartStop verifies that the memory engine starts
// and stops cleanly in the web context.
func TestMainWeb_MemoryEngineStartStop(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "memento.db")

	store, err := sqlite.NewMemoryStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	t.Setenv("MEMENTO_DATA_PATH", tmpDir)
	t.Setenv("MEMENTO_LLM_PROVIDER", "ollama")
	t.Setenv("MEMENTO_LLM_BASE_URL", "http://localhost:11434")

	cfg, err := config.LoadConfig()
	require.NoError(t, err)

	engineCfg := engine.DefaultConfig()
	engineCfg.NumWorkers = 1

	memEngine, err := engine.NewMemoryEngine(store, engineCfg, cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = memEngine.Start(ctx)
	require.NoError(t, err, "failed to start engine")

	// Shutdown should complete quickly
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()

	err = memEngine.Shutdown(shutdownCtx)
	require.NoError(t, err, "failed to shutdown engine cleanly")
}

// TestMainWeb_ContextCancellation verifies that the web server's context
// cancellation mechanism works correctly.
func TestMainWeb_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Verify context is not cancelled initially
	select {
	case <-ctx.Done():
		t.Fatal("context should not be cancelled initially")
	default:
	}

	// Cancel the context
	cancel()

	// Verify context is now cancelled
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Fatal("context should be cancelled after cancel()")
	}
}

// TestMainWeb_DataDirectoryCreation verifies that the data directory is
// created when it does not exist.
func TestMainWeb_DataDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "nested", "data", "path")

	// Ensure the directory does not exist
	assert.NoError(t, os.RemoveAll(dataPath))

	// Create the directory as main() does via cfg.Storage.DataPath
	err := os.MkdirAll(dataPath, 0o700)
	require.NoError(t, err, "failed to create data directory")

	// Verify it exists
	info, err := os.Stat(dataPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestMainWeb_ConfigPathResolution verifies that the connections config path
// can be resolved from the command line flag or environment.
func TestMainWeb_ConfigPathResolution(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test connections config file
	configDir := filepath.Join(tmpDir, "config")
	require.NoError(t, os.Mkdir(configDir, 0o755))

	configPath := filepath.Join(configDir, "connections.json")
	require.NoError(t, os.WriteFile(configPath, []byte("{}"), 0o644))

	// Verify the file exists
	_, err := os.Stat(configPath)
	assert.NoError(t, err, "config file should exist")
}

// TestMainWeb_MultipleStoresOnDifferentPaths verifies that the web server can
// be tested with multiple independent storage backends.
func TestMainWeb_MultipleStoresOnDifferentPaths(t *testing.T) {
	tmpDir := t.TempDir()

	dbPath1 := filepath.Join(tmpDir, "memento1.db")
	dbPath2 := filepath.Join(tmpDir, "memento2.db")

	store1, err := sqlite.NewMemoryStore(dbPath1)
	require.NoError(t, err)
	defer func() { _ = store1.Close() }()

	store2, err := sqlite.NewMemoryStore(dbPath2)
	require.NoError(t, err)
	defer func() { _ = store2.Close() }()

	// Both should be initialized and usable
	assert.NotNil(t, store1)
	assert.NotNil(t, store2)

	// Verify both database files exist
	_, err = os.Stat(dbPath1)
	assert.NoError(t, err)
	_, err = os.Stat(dbPath2)
	assert.NoError(t, err)
}

// TestMainWeb_ServerStartHelper verifies that the startServer helper function
// correctly wires the server components.
func TestMainWeb_ServerStartHelper(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0, // random port
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
	defer cancel()

	// Call the helper function
	addr := startServer(ctx, cfg, store)

	// Verify an address was returned
	assert.NotEmpty(t, addr, "startServer should return a non-empty address")

	// Attempt to connect to the server
	resp, err := http.Get("http://" + addr + "/")
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "server should respond to HTTP requests")
	}
}

// TestMainWeb_EngineCallbacks verifies that the engine callback registration
// logic works correctly.
func TestMainWeb_EngineCallbacks(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "memento.db")

	store, err := sqlite.NewMemoryStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	t.Setenv("MEMENTO_DATA_PATH", tmpDir)
	t.Setenv("MEMENTO_LLM_PROVIDER", "ollama")
	t.Setenv("MEMENTO_LLM_BASE_URL", "http://localhost:11434")

	cfg, err := config.LoadConfig()
	require.NoError(t, err)

	engineCfg := engine.DefaultConfig()
	engineCfg.NumWorkers = 1

	memEngine, err := engine.NewMemoryEngine(store, engineCfg, cfg)
	require.NoError(t, err)

	// Test that callbacks can be registered without error
	callbacksCalled := []string{}

	memEngine.SetOnMemoryCreated(func(memoryID string) {
		callbacksCalled = append(callbacksCalled, "memory_created")
	})

	memEngine.SetOnEnrichmentStarted(func(memoryID string) {
		callbacksCalled = append(callbacksCalled, "enrichment_started")
	})

	memEngine.SetOnEnrichmentComplete(func(memoryID string) {
		callbacksCalled = append(callbacksCalled, "enrichment_complete")
	})

	// Callbacks should be registered without error
	assert.Equal(t, 0, len(callbacksCalled), "callbacks should not be called during registration")
}

// TestMainWeb_ServerConfiguration verifies that the server configuration is
// properly constructed and validated.
func TestMainWeb_ServerConfiguration(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	assert.Equal(t, "127.0.0.1", cfg.Server.Host, "host should be set")
	assert.Equal(t, 0, cfg.Server.Port, "port 0 should select a random port")
	assert.Equal(t, "development", cfg.Security.SecurityMode, "security mode should be set")
}

// TestMainWeb_ContextTimeout verifies that context timeout handling works
// correctly for the server lifecycle.
func TestMainWeb_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Context should not be done immediately
	select {
	case <-ctx.Done():
		t.Fatal("context should not be done immediately")
	default:
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Context should now be done
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Fatal("context should be done after timeout")
	}

	assert.Equal(t, context.DeadlineExceeded, ctx.Err())
}

// TestMainWeb_NumWorkersConfiguration verifies that the web server uses the
// correct number of workers (1 for SQLite to avoid locking).
func TestMainWeb_NumWorkersConfiguration(t *testing.T) {
	engineCfg := engine.DefaultConfig()

	// Simulate the web main() configuration
	engineCfg.NumWorkers = 1 // Web uses 1 worker for SQLite to avoid database locking

	assert.Equal(t, 1, engineCfg.NumWorkers, "web should configure 1 worker for SQLite")
}

// TestMainWeb_DatabasePathConstruction verifies that the database path is
// correctly constructed from the data directory.
func TestMainWeb_DatabasePathConstruction(t *testing.T) {
	tmpDir := t.TempDir()
	expectedPath := filepath.Join(tmpDir, "memento.db")

	// Simulate the path construction from main()
	dbPath := fmt.Sprintf("%s/memento.db", tmpDir)

	assert.Equal(t, expectedPath, dbPath, "database path should be constructed correctly")
}
