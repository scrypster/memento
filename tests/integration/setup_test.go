package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// TestMain runs before all tests in this package
func TestMain(m *testing.M) {
	// Setup code here if needed
	code := m.Run()
	// Teardown code here if needed
	os.Exit(code)
}

// NewTestEngine creates a memory engine configured for integration testing.
// It uses a temporary SQLite database. The full schema is applied by
// NewMemoryStore, so no separate migration step is needed.
// The returned cleanup function should be called when the test completes.
func NewTestEngine(t *testing.T) (*engine.MemoryEngine, func()) {
	t.Helper()

	// Create temp directory for test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Create memory store (applies full schema internally)
	store, err := sqlite.NewMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test memory store: %v", err)
	}

	// Create engine with test-friendly configuration
	config := engine.Config{
		NumWorkers:        2,
		QueueSize:         100,
		RecoveryBatchSize: 1000,
		MaxRetries:        3,
		ShutdownTimeout:   5 * time.Second,
	}

	eng, err := engine.NewMemoryEngine(store, config, nil)
	if err != nil {
		t.Fatalf("Failed to create memory engine: %v", err)
	}

	// Start the engine
	ctx := context.Background()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Failed to start memory engine: %v", err)
	}

	// Return cleanup function
	cleanup := func() {
		shutdownCtx := context.Background()
		if err := eng.Shutdown(shutdownCtx); err != nil {
			t.Logf("Warning: Engine shutdown error: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Logf("Warning: Store close error: %v", err)
		}
	}

	return eng, cleanup
}

// NewTestStore creates a standalone memory store for testing storage operations.
// It uses a temporary SQLite database. The full schema is applied by
// NewMemoryStore, so no separate migration step is needed.
// The returned cleanup function should be called when the test completes.
func NewTestStore(t *testing.T) (storage.MemoryStore, func()) {
	t.Helper()

	// Create temp directory for test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Create memory store (applies full schema internally)
	store, err := sqlite.NewMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test memory store: %v", err)
	}

	// Return cleanup function
	cleanup := func() {
		if err := store.Close(); err != nil {
			t.Logf("Warning: Store close error: %v", err)
		}
	}

	return store, cleanup
}

// newTestMemory creates a Memory struct with a generated ID for direct store tests.
func newTestMemory(content string, tags ...string) *types.Memory {
	return &types.Memory{
		ID:      "mem:" + uuid.New().String(),
		Content: content,
		Tags:    tags,
		Status:  types.StatusPending,
	}
}
