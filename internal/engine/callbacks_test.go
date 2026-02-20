package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestEngine creates a MemoryEngine backed by a temp SQLite store with no LLM.
// The enrichment worker runs but skips LLM extraction (enrichmentService is nil).
func newTestEngine(t *testing.T) *MemoryEngine {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := sqlite.NewMemoryStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	cfg := DefaultConfig()
	cfg.NumWorkers = 1

	eng, err := NewMemoryEngine(store, cfg, nil)
	require.NoError(t, err)

	return eng
}

func TestOnMemoryCreated_FiresOnStore(t *testing.T) {
	eng := newTestEngine(t)

	received := make(chan string, 1)
	eng.SetOnMemoryCreated(func(memoryID string) {
		received <- memoryID
	})

	ctx := context.Background()
	require.NoError(t, eng.Start(ctx))
	defer func() { _ = eng.Shutdown(ctx) }()

	mem, err := eng.Store(ctx, "test callback content")
	require.NoError(t, err)
	require.NotNil(t, mem)

	select {
	case id := <-received:
		assert.Equal(t, mem.ID, id)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: onMemoryCreated callback never fired")
	}
}

func TestOnEnrichmentStarted_FiresOnProcessing(t *testing.T) {
	eng := newTestEngine(t)

	received := make(chan string, 1)
	eng.SetOnEnrichmentStarted(func(memoryID string) {
		received <- memoryID
	})

	ctx := context.Background()
	require.NoError(t, eng.Start(ctx))
	defer func() { _ = eng.Shutdown(ctx) }()

	mem, err := eng.Store(ctx, "test enrichment started callback")
	require.NoError(t, err)

	select {
	case id := <-received:
		assert.Equal(t, mem.ID, id)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: onEnrichmentStarted callback never fired")
	}
}

func TestOnEnrichmentComplete_FiresAfterEnrichment(t *testing.T) {
	eng := newTestEngine(t)

	received := make(chan string, 1)
	eng.SetOnEnrichmentComplete(func(memoryID string) {
		received <- memoryID
	})

	ctx := context.Background()
	require.NoError(t, eng.Start(ctx))
	defer func() { _ = eng.Shutdown(ctx) }()

	mem, err := eng.Store(ctx, "test enrichment complete callback")
	require.NoError(t, err)

	select {
	case id := <-received:
		assert.Equal(t, mem.ID, id)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: onEnrichmentComplete callback never fired")
	}
}

func TestAllCallbacks_FireInOrder(t *testing.T) {
	eng := newTestEngine(t)

	events := make(chan string, 10)
	eng.SetOnMemoryCreated(func(memoryID string) {
		events <- fmt.Sprintf("created:%s", memoryID)
	})
	eng.SetOnEnrichmentStarted(func(memoryID string) {
		events <- fmt.Sprintf("started:%s", memoryID)
	})
	eng.SetOnEnrichmentComplete(func(memoryID string) {
		events <- fmt.Sprintf("complete:%s", memoryID)
	})

	ctx := context.Background()
	require.NoError(t, eng.Start(ctx))
	defer func() { _ = eng.Shutdown(ctx) }()

	mem, err := eng.Store(ctx, "test all callbacks in order")
	require.NoError(t, err)

	var collected []string
	timeout := time.After(5 * time.Second)
	for len(collected) < 3 {
		select {
		case evt := <-events:
			collected = append(collected, evt)
		case <-timeout:
			t.Fatalf("timeout: only received %d/3 events: %v", len(collected), collected)
		}
	}

	assert.Equal(t, fmt.Sprintf("created:%s", mem.ID), collected[0])
	assert.Equal(t, fmt.Sprintf("started:%s", mem.ID), collected[1])
	assert.Equal(t, fmt.Sprintf("complete:%s", mem.ID), collected[2])
}

func TestNoCallbacks_DoesNotPanic(t *testing.T) {
	eng := newTestEngine(t)

	// Don't set any callbacks — should not panic
	ctx := context.Background()
	require.NoError(t, eng.Start(ctx))
	defer func() { _ = eng.Shutdown(ctx) }()

	mem, err := eng.Store(ctx, "no callbacks set")
	require.NoError(t, err)
	require.NotNil(t, mem)

	// Give the worker time to process
	time.Sleep(500 * time.Millisecond)

	// Verify memory was still enriched despite no callbacks
	got, err := eng.Get(ctx, mem.ID)
	require.NoError(t, err)
	// Without LLM, the worker skips enrichment but still marks as enriched
	assert.Equal(t, "enriched", string(got.Status))
}

func init() {
	// Suppress noisy log output during tests
	_ = os.Setenv("MEMENTO_DATA_PATH", os.TempDir())
}
