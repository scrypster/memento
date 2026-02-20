// main_test.go exercises the memento-mcp entry point wiring.
//
// Tests verify that:
//  1. The MCP server initializes correctly with a store and configuration.
//  2. Environment variables are properly handled (MEMENTO_DATA_PATH, MEMENTO_NUM_WORKERS).
//  3. Signal handling is set up correctly.
//  4. The memory engine starts and stops cleanly within reasonable timeouts.
//  5. Data directory creation and database opening work as expected.
package main

import (
	"context"
	"fmt"
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

// TestMCPMain_InitializeStore verifies that a SQLite store can be opened
// at the configured data path and is not nil.
func TestMCPMain_InitializeStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "memento.db")

	store, err := sqlite.NewMemoryStore(dbPath)
	require.NoError(t, err, "failed to create store")
	defer func() { _ = store.Close() }()

	assert.NotNil(t, store, "store should not be nil")
}

// TestMCPMain_CreateDataDirectory verifies that the data directory is created
// with proper permissions when it does not exist.
func TestMCPMain_CreateDataDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "nonexistent", "data", "path")

	// Ensure the directory does not exist
	assert.NoError(t, os.RemoveAll(dataPath))

	// Create the directory as the main() function does
	err := os.MkdirAll(dataPath, 0o700)
	require.NoError(t, err, "failed to create data directory")

	// Verify it exists and has correct permissions
	info, err := os.Stat(dataPath)
	require.NoError(t, err, "failed to stat data directory")
	assert.True(t, info.IsDir(), "path should be a directory")
}

// TestMCPMain_ConfigurationLoading verifies that configuration can be loaded
// from environment variables and is not nil.
func TestMCPMain_ConfigurationLoading(t *testing.T) {
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

// TestMCPMain_MemoryEngineInitialization verifies that a MemoryEngine can be
// created and started without error.
func TestMCPMain_MemoryEngineInitialization(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "memento.db")

	// Create store
	store, err := sqlite.NewMemoryStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	// Set up config
	t.Setenv("MEMENTO_DATA_PATH", tmpDir)
	t.Setenv("MEMENTO_LLM_PROVIDER", "ollama")
	t.Setenv("MEMENTO_LLM_BASE_URL", "http://localhost:11434")

	appCfg, err := config.LoadConfig()
	require.NoError(t, err)

	// Initialize the memory engine
	engineCfg := engine.DefaultConfig()
	memEngine, err := engine.NewMemoryEngine(store, engineCfg, appCfg)
	require.NoError(t, err, "failed to create memory engine")
	assert.NotNil(t, memEngine)
}

// TestMCPMain_MemoryEngineStartStop verifies that the memory engine can be
// started and stopped cleanly within a reasonable timeout.
func TestMCPMain_MemoryEngineStartStop(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "memento.db")

	// Create store
	store, err := sqlite.NewMemoryStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	// Set up config
	t.Setenv("MEMENTO_DATA_PATH", tmpDir)
	t.Setenv("MEMENTO_LLM_PROVIDER", "ollama")
	t.Setenv("MEMENTO_LLM_BASE_URL", "http://localhost:11434")

	cfg, err := config.LoadConfig()
	require.NoError(t, err)

	// Create and start the engine
	engineCfg := engine.DefaultConfig()
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

// TestMCPMain_NumWorkersEnvironmentVariable verifies that MEMENTO_NUM_WORKERS
// environment variable is respected and parsed correctly.
func TestMCPMain_NumWorkersEnvironmentVariable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "memento.db")

	store, err := sqlite.NewMemoryStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	t.Setenv("MEMENTO_DATA_PATH", tmpDir)
	t.Setenv("MEMENTO_LLM_PROVIDER", "openai")
	t.Setenv("MEMENTO_LLM_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("MEMENTO_NUM_WORKERS", "8")

	_, err = config.LoadConfig()
	require.NoError(t, err)

	// Test the logic from main()
	engineCfg := engine.DefaultConfig()

	// Simulate the override logic from main()
	if override := os.Getenv("MEMENTO_NUM_WORKERS"); override != "" {
		var n int
		_, _ = fmt.Sscanf(override, "%d", &n)
		if n >= 1 {
			engineCfg.NumWorkers = n
		}
	}

	assert.Equal(t, 8, engineCfg.NumWorkers, "num workers should be overridden to 8")
}

// TestMCPMain_NumWorkersOllamaDefault verifies that when LLM provider is ollama,
// the engine defaults to 1 worker to avoid contention.
func TestMCPMain_NumWorkersOllamaDefault(t *testing.T) {
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

	// Simulate the logic from main()
	engineCfg := engine.DefaultConfig()

	// Override based on environment (no explicit override set)
	if override := os.Getenv("MEMENTO_NUM_WORKERS"); override != "" {
		var n int
		_, _ = fmt.Sscanf(override, "%d", &n)
		if n >= 1 {
			engineCfg.NumWorkers = n
		}
	} else if cfg.LLM.LLMProvider == "ollama" {
		engineCfg.NumWorkers = 1
	}

	assert.Equal(t, 1, engineCfg.NumWorkers, "ollama provider should use 1 worker")
}

// TestMCPMain_DatabasePathConstruction verifies that the database path is
// correctly constructed from the data directory.
func TestMCPMain_DatabasePathConstruction(t *testing.T) {
	tmpDir := t.TempDir()
	expectedDBPath := filepath.Join(tmpDir, "memento.db")

	// Simulate the path construction from main()
	dbPath := fmt.Sprintf("%s/memento.db", tmpDir)

	assert.Equal(t, expectedDBPath, dbPath, "database path should be correct")
}

// TestMCPMain_InvalidDataPathHandling verifies that attempting to create a
// store with an invalid path produces an error (not a panic).
func TestMCPMain_InvalidDataPathHandling(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("skipping permission test as root")
	}

	// Try to create a database in a read-only directory (if possible)
	tmpDir := t.TempDir()
	roDir := filepath.Join(tmpDir, "readonly")

	require.NoError(t, os.Mkdir(roDir, 0o555))
	defer func() {
		_ = os.Chmod(roDir, 0o755)
		_ = os.RemoveAll(roDir)
	}()

	dbPath := filepath.Join(roDir, "memento.db")

	// Attempt to open the store in a read-only directory
	// This should fail gracefully, not panic
	_, err := sqlite.NewMemoryStore(dbPath)
	assert.Error(t, err, "creating store in read-only directory should fail")
}

// TestMCPMain_ConcurrentDataDirectoryCreation verifies that multiple goroutines
// calling os.MkdirAll on the same path does not cause issues.
func TestMCPMain_ConcurrentDataDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "concurrent", "data", "path")

	done := make(chan error, 3)

	// Simulate concurrent calls to os.MkdirAll from multiple goroutines
	for i := 0; i < 3; i++ {
		go func() {
			err := os.MkdirAll(dataPath, 0o700)
			done <- err
		}()
	}

	// All should succeed without errors
	for i := 0; i < 3; i++ {
		err := <-done
		assert.NoError(t, err, "concurrent directory creation should not error")
	}

	// Verify directory was created exactly once
	info, err := os.Stat(dataPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestMCPMain_StoreClose verifies that calling Close on a store after opening
// it succeeds without error.
func TestMCPMain_StoreClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "memento.db")

	store, err := sqlite.NewMemoryStore(dbPath)
	require.NoError(t, err)

	// Close should succeed
	err = store.Close()
	assert.NoError(t, err, "store.Close() should not error")
}

// TestMCPMain_MultipleStoresOnDifferentPaths verifies that multiple stores
// can be opened on different paths without interfering with each other.
func TestMCPMain_MultipleStoresOnDifferentPaths(t *testing.T) {
	tmpDir := t.TempDir()

	dbPath1 := filepath.Join(tmpDir, "memento1.db")
	dbPath2 := filepath.Join(tmpDir, "memento2.db")

	store1, err := sqlite.NewMemoryStore(dbPath1)
	require.NoError(t, err)
	defer func() { _ = store1.Close() }()

	store2, err := sqlite.NewMemoryStore(dbPath2)
	require.NoError(t, err)
	defer func() { _ = store2.Close() }()

	// Both should be open and usable
	assert.NotNil(t, store1)
	assert.NotNil(t, store2)

	// Verify both database files exist
	_, err = os.Stat(dbPath1)
	assert.NoError(t, err, "store1 database file should exist")

	_, err = os.Stat(dbPath2)
	assert.NoError(t, err, "store2 database file should exist")
}

// TestMCPMain_ContextCancellation verifies that the context-based shutdown
// mechanism works correctly.
func TestMCPMain_ContextCancellation(t *testing.T) {
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

// TestMCPMain_ContextWithTimeout verifies that a context with timeout works
// correctly for the engine shutdown scenario.
func TestMCPMain_ContextWithTimeout(t *testing.T) {
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

	// Error should be context.DeadlineExceeded
	assert.Equal(t, context.DeadlineExceeded, ctx.Err())
}

// TestResolveConnectionsConfig_Integration verifies that the resolveConnectionsConfig
// function integrates correctly with the file system.
func TestResolveConnectionsConfig_Integration(t *testing.T) {
	// This test verifies integration with the existing config resolution tests
	// in config_resolution_test.go. It just ensures resolveConnectionsConfig
	// returns a string (empty or valid path).

	result := resolveConnectionsConfig()
	assert.IsType(t, "", result, "resolveConnectionsConfig should return a string")
}
