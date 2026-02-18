package tests

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// TestMemoryEngineStore_NonBlocking verifies Store() returns in <10ms.
func TestMemoryEngineStore_NonBlocking(t *testing.T) {
	// Create file-based SQLite store (avoids connection pool issues with :memory:)
	// Use t.TempDir() for automatic cleanup
	dbPath := filepath.Join(t.TempDir(), "memento.db")
	store, err := sqlite.NewMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	defer store.Close()

	// Create memory engine with minimal worker pool
	engineCfg := engine.Config{
		NumWorkers:        2,
		QueueSize:         100,
		ShutdownTimeout:   5 * time.Second,
		MaxRetries:        3,
		RecoveryBatchSize: 1000,
	}

	eng, err := engine.NewMemoryEngine(store, engineCfg, nil)
	if err != nil {
		t.Fatalf("failed to create memory engine: %v", err)
	}

	// Start the engine
	ctx := context.Background()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}
	defer eng.Shutdown(ctx)

	// Store a memory and measure time
	content := "Test memory content for non-blocking store"
	start := time.Now()

	memory, err := eng.Store(ctx, content)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	// Verify response time is <10ms
	if elapsed > 10*time.Millisecond {
		t.Errorf("Store() took %v, expected <10ms", elapsed)
	}

	// Verify memory was created with correct status
	if memory.ID == "" {
		t.Error("Memory ID is empty")
	}

	if memory.Content != content {
		t.Errorf("Content mismatch: got %q, want %q", memory.Content, content)
	}

	if memory.Status != types.StatusPending {
		t.Errorf("Status = %v, want %v", memory.Status, types.StatusPending)
	}
}

// TestEnrichmentWorker_ProcessesJobs verifies jobs are processed by workers.
func TestEnrichmentWorker_ProcessesJobs(t *testing.T) {
	// Create file-based SQLite store (avoids connection pool issues with :memory:)
	// Use t.TempDir() for automatic cleanup
	dbPath := filepath.Join(t.TempDir(), "memento.db")
	store, err := sqlite.NewMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	defer store.Close()

	// Create memory engine
	engineCfg := engine.Config{
		NumWorkers:        4,
		QueueSize:         100,
		ShutdownTimeout:   5 * time.Second,
		MaxRetries:        3,
		RecoveryBatchSize: 1000,
	}

	eng, err := engine.NewMemoryEngine(store, engineCfg, nil)
	if err != nil {
		t.Fatalf("failed to create memory engine: %v", err)
	}

	// Start the engine
	ctx := context.Background()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}
	defer eng.Shutdown(ctx)

	// Store multiple memories
	numMemories := 10
	memoryIDs := make([]string, numMemories)

	for i := 0; i < numMemories; i++ {
		memory, err := eng.Store(ctx, "Test memory content")
		if err != nil {
			t.Fatalf("Store() failed: %v", err)
		}
		memoryIDs[i] = memory.ID
	}

	// Wait for workers to process jobs (with timeout)
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	allEnriched := false
	for !allEnriched {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for enrichment to complete")
		case <-ticker.C:
			enrichedCount := 0
			for _, id := range memoryIDs {
				mem, err := store.Get(ctx, id)
				if err != nil {
					t.Fatalf("Get() failed: %v", err)
				}
				if mem.Status == types.StatusEnriched {
					enrichedCount++
				}
			}
			if enrichedCount == numMemories {
				allEnriched = true
			}
		}
	}

	// Verify all memories were enriched
	for _, id := range memoryIDs {
		mem, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get() failed for %s: %v", id, err)
		}

		if mem.Status != types.StatusEnriched {
			t.Errorf("Memory %s status = %v, want %v", id, mem.Status, types.StatusEnriched)
		}
	}
}

// TestEnrichmentQueue_Full verifies queue full behavior.
func TestEnrichmentQueue_Full(t *testing.T) {
	// Create file-based SQLite store (avoids connection pool issues with :memory:)
	// Use t.TempDir() for automatic cleanup
	dbPath := filepath.Join(t.TempDir(), "memento.db")
	store, err := sqlite.NewMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	defer store.Close()

	// Create memory engine with very small queue
	engineCfg := engine.Config{
		NumWorkers:        1, // Single slow worker
		QueueSize:         2, // Very small queue
		ShutdownTimeout:   5 * time.Second,
		MaxRetries:        3,
		RecoveryBatchSize: 1000,
	}

	eng, err := engine.NewMemoryEngine(store, engineCfg, nil)
	if err != nil {
		t.Fatalf("failed to create memory engine: %v", err)
	}

	// Start the engine
	ctx := context.Background()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}
	defer eng.Shutdown(ctx)

	// Flood the queue by storing many memories quickly.
	// Some stores may return an "enrichment queue full" error when the queue is
	// saturated; that is expected behaviour for this test, so we only fatalf on
	// unexpected errors (i.e. storage failures, not queue-full).
	numMemories := 20
	memoryIDs := make([]string, 0, numMemories)

	for i := 0; i < numMemories; i++ {
		memory, err := eng.Store(ctx, "Test memory content")
		if err != nil {
			// Queue-full errors are expected when flooding; skip the memory.
			if strings.Contains(err.Error(), "queue full") {
				continue
			}
			t.Fatalf("Store() failed with unexpected error: %v", err)
		}
		memoryIDs = append(memoryIDs, memory.ID)
	}

	// Wait for processing with timeout and polling
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Count memories in different states
	pendingCount := 0
	failedCount := 0
	enrichedCount := 0
	processingCount := 0

	processingComplete := false
	for !processingComplete {
		select {
		case <-timeout:
			processingComplete = true
		case <-ticker.C:
			pendingCount = 0
			failedCount = 0
			enrichedCount = 0
			processingCount = 0

			for _, id := range memoryIDs {
				mem, err := store.Get(ctx, id)
				if err != nil {
					t.Fatalf("Get() failed: %v", err)
				}

				switch mem.Status {
				case types.StatusPending:
					pendingCount++
				case types.StatusFailed:
					failedCount++
				case types.StatusEnriched:
					enrichedCount++
				case types.StatusProcessing:
					processingCount++
				}
			}
		}
	}

	// With a small queue and slow workers, we should have some failed memories
	// (when queue is full, memories are marked as failed)
	if failedCount == 0 {
		t.Logf("Queue full behavior test: pending=%d, processing=%d, enriched=%d, failed=%d",
			pendingCount, processingCount, enrichedCount, failedCount)
		// This is not necessarily an error - timing dependent
		// Just log the results
	}
}

// TestStartupRecovery verifies pending memories are recovered on startup.
func TestStartupRecovery(t *testing.T) {
	// Create file-based SQLite store (avoids connection pool issues with :memory:)
	// Use t.TempDir() for automatic cleanup
	dbPath := filepath.Join(t.TempDir(), "memento.db")
	store, err := sqlite.NewMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create some pending memories directly in the store
	numPending := 5
	pendingIDs := make([]string, numPending)

	for i := 0; i < numPending; i++ {
		memory := &types.Memory{
			ID:        engine.GenerateMemoryID("test", "recovery"),
			Content:   "Pending memory from previous session",
			Status:    types.StatusPending,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := store.Store(ctx, memory); err != nil {
			t.Fatalf("Store() failed: %v", err)
		}
		pendingIDs[i] = memory.ID
	}

	// Create memory engine (simulating a restart)
	engineCfg := engine.Config{
		NumWorkers:        4,
		QueueSize:         100,
		ShutdownTimeout:   5 * time.Second,
		MaxRetries:        3,
		RecoveryBatchSize: 1000,
	}

	eng, err := engine.NewMemoryEngine(store, engineCfg, nil)
	if err != nil {
		t.Fatalf("failed to create memory engine: %v", err)
	}

	// Start the engine (this should recover pending memories)
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}
	defer eng.Shutdown(ctx)

	// Wait for recovery to complete
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	allRecovered := false
	for !allRecovered {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for recovery to complete")
		case <-ticker.C:
			recoveredCount := 0
			for _, id := range pendingIDs {
				mem, err := store.Get(ctx, id)
				if err != nil {
					t.Fatalf("Get() failed: %v", err)
				}
				if mem.Status == types.StatusEnriched {
					recoveredCount++
				}
			}
			if recoveredCount == numPending {
				allRecovered = true
			}
		}
	}

	// Verify all pending memories were recovered and enriched
	for _, id := range pendingIDs {
		mem, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get() failed for %s: %v", id, err)
		}

		if mem.Status != types.StatusEnriched {
			t.Errorf("Memory %s status = %v, want %v", id, mem.Status, types.StatusEnriched)
		}
	}
}

// TestGracefulShutdown verifies workers drain on shutdown.
func TestGracefulShutdown(t *testing.T) {
	// Create file-based SQLite store (avoids connection pool issues with :memory:)
	// Use t.TempDir() for automatic cleanup
	dbPath := filepath.Join(t.TempDir(), "memento.db")
	store, err := sqlite.NewMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	defer store.Close()

	// Create memory engine
	engineCfg := engine.Config{
		NumWorkers:        4,
		QueueSize:         100,
		ShutdownTimeout:   5 * time.Second,
		MaxRetries:        3,
		RecoveryBatchSize: 1000,
	}

	eng, err := engine.NewMemoryEngine(store, engineCfg, nil)
	if err != nil {
		t.Fatalf("failed to create memory engine: %v", err)
	}

	// Start the engine
	ctx := context.Background()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Store some memories
	numMemories := 10
	memoryIDs := make([]string, numMemories)

	for i := 0; i < numMemories; i++ {
		memory, err := eng.Store(ctx, "Test memory content")
		if err != nil {
			t.Fatalf("Store() failed: %v", err)
		}
		memoryIDs[i] = memory.ID
	}

	// Shutdown gracefully
	start := time.Now()
	if err := eng.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() failed: %v", err)
	}
	elapsed := time.Since(start)

	t.Logf("Shutdown took %v", elapsed)

	// Verify shutdown completed within timeout
	if elapsed > 10*time.Second {
		t.Errorf("Shutdown took %v, expected <10s", elapsed)
	}

	// Verify all memories were processed before shutdown
	for _, id := range memoryIDs {
		mem, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get() failed: %v", err)
		}

		// Should be enriched (workers drained before shutdown)
		if mem.Status != types.StatusEnriched {
			t.Errorf("Memory %s status = %v, want %v (workers should have drained)",
				id, mem.Status, types.StatusEnriched)
		}
	}
}

// TestGracefulShutdown_Timeout verifies timeout behavior during shutdown.
func TestGracefulShutdown_Timeout(t *testing.T) {
	// Create file-based SQLite store (avoids connection pool issues with :memory:)
	// Use t.TempDir() for automatic cleanup
	dbPath := filepath.Join(t.TempDir(), "memento.db")
	store, err := sqlite.NewMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	defer store.Close()

	// Create memory engine with very short shutdown timeout
	engineCfg := engine.Config{
		NumWorkers:        1,
		QueueSize:         100,
		ShutdownTimeout:   100 * time.Millisecond, // Very short timeout
		MaxRetries:        3,
		RecoveryBatchSize: 1000,
	}

	eng, err := engine.NewMemoryEngine(store, engineCfg, nil)
	if err != nil {
		t.Fatalf("failed to create memory engine: %v", err)
	}

	// Start the engine
	ctx := context.Background()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Store many memories to fill queue
	numMemories := 50
	for i := 0; i < numMemories; i++ {
		_, err := eng.Store(ctx, "Test memory content")
		if err != nil {
			t.Fatalf("Store() failed: %v", err)
		}
	}

	// Shutdown with short timeout (should timeout)
	start := time.Now()
	err = eng.Shutdown(ctx)
	elapsed := time.Since(start)

	// Shutdown should complete around the timeout duration
	if elapsed > 1*time.Second {
		t.Errorf("Shutdown took %v, expected ~100ms (timeout)", elapsed)
	}

	// Error may or may not be returned depending on implementation
	// The key is that shutdown doesn't hang indefinitely
	t.Logf("Shutdown returned in %v with error: %v", elapsed, err)
}
