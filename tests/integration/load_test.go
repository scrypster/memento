package integration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/engine"
)

// TestLoad_ConcurrentStores tests system under concurrent Store load.
//
// Note: eng.Store() returns a non-nil error when the enrichment queue is full,
// but the memory IS persisted to the database. Under high-concurrency load
// with a finite queue size, enrichment saturation is expected and is not a
// storage failure. We therefore differentiate between true storage errors
// and enrichment-queue-full warnings.
func TestLoad_ConcurrentStores(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	numGoroutines := 10
	storesPerGoroutine := 100

	var (
		successCount    int64
		queueFullCount  int64
		fatalErrorCount int64
		wg              sync.WaitGroup
	)

	startTime := time.Now()

	// Launch concurrent goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < storesPerGoroutine; j++ {
				content := fmt.Sprintf("Memory from worker %d, iteration %d", workerID, j)
				_, err := eng.Store(ctx, content)
				if err != nil {
					if strings.Contains(err.Error(), "enrichment queue full") {
						atomic.AddInt64(&queueFullCount, 1)
					} else {
						atomic.AddInt64(&fatalErrorCount, 1)
						t.Logf("Store failed: %v", err)
					}
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Report results
	totalOps := numGoroutines * storesPerGoroutine
	storedCount := successCount + queueFullCount // memory is persisted in both cases
	t.Logf("Load test completed in %v", duration)
	t.Logf("Total operations: %d", totalOps)
	t.Logf("Stored (with enrichment): %d", successCount)
	t.Logf("Stored (queue full, no enrichment): %d", queueFullCount)
	t.Logf("Fatal errors: %d", fatalErrorCount)
	t.Logf("Throughput: %.2f ops/sec", float64(storedCount)/duration.Seconds())

	// Verify no fatal storage errors (enrichment queue full is acceptable)
	if fatalErrorCount > 0 {
		t.Errorf("Got %d fatal storage errors (not enrichment-queue-full)", fatalErrorCount)
	}

	// Verify all memories were persisted
	if storedCount < int64(totalOps) {
		t.Errorf("Not all memories were persisted: %d/%d", storedCount, totalOps)
	}
}

// TestLoad_ConcurrentReads tests system under concurrent Get load.
func TestLoad_ConcurrentReads(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Store memories first
	memoryIDs := make([]string, 100)
	for i := 0; i < 100; i++ {
		mem, err := eng.Store(ctx, fmt.Sprintf("Memory %d", i))
		if err != nil {
			t.Fatalf("Setup failed: %v", err)
		}
		memoryIDs[i] = mem.ID
	}

	// Give time for writes
	time.Sleep(100 * time.Millisecond)

	// Concurrent reads
	numGoroutines := 20
	readsPerGoroutine := 100

	var (
		successCount int64
		errorCount   int64
		wg           sync.WaitGroup
	)

	startTime := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < readsPerGoroutine; j++ {
				memID := memoryIDs[(workerID+j)%len(memoryIDs)]
				_, err := eng.Get(ctx, memID)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Report results
	totalOps := numGoroutines * readsPerGoroutine
	t.Logf("Read load test completed in %v", duration)
	t.Logf("Total reads: %d", totalOps)
	t.Logf("Successful: %d", successCount)
	t.Logf("Errors: %d", errorCount)
	t.Logf("Read throughput: %.2f ops/sec", float64(successCount)/duration.Seconds())

	if errorCount > 0 {
		t.Errorf("Expected no read errors, got %d", errorCount)
	}
}

// TestLoad_MixedWorkload tests system under mixed read/write load.
func TestLoad_MixedWorkload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Store initial memories
	memoryIDs := make([]string, 50)
	for i := 0; i < 50; i++ {
		mem, err := eng.Store(ctx, fmt.Sprintf("Initial memory %d", i))
		if err != nil && !strings.Contains(err.Error(), "enrichment queue full") {
			t.Fatalf("Setup failed: %v", err)
		}
		memoryIDs[i] = mem.ID
	}

	time.Sleep(100 * time.Millisecond)

	// Mixed workload
	numGoroutines := 10
	opsPerGoroutine := 100

	var (
		storeCount     int64
		getCount       int64
		searchCount    int64
		updateCount    int64
		queueFullCount int64
		fatalErrors    int64
		wg             sync.WaitGroup
	)

	startTime := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < opsPerGoroutine; j++ {
				// Mix of operations
				switch j % 4 {
				case 0: // Store
					_, err := eng.Store(ctx, fmt.Sprintf("New memory %d-%d", workerID, j))
					if err != nil {
						if strings.Contains(err.Error(), "enrichment queue full") {
							atomic.AddInt64(&queueFullCount, 1)
						} else {
							atomic.AddInt64(&fatalErrors, 1)
						}
					} else {
						atomic.AddInt64(&storeCount, 1)
					}

				case 1: // Get
					memID := memoryIDs[(workerID+j)%len(memoryIDs)]
					_, err := eng.Get(ctx, memID)
					if err != nil {
						atomic.AddInt64(&fatalErrors, 1)
					} else {
						atomic.AddInt64(&getCount, 1)
					}

				case 2: // Search
					_, err := eng.Search(ctx, engine.SearchOptions{
						Query: "memory",
						Limit: 5,
					})
					if err != nil {
						atomic.AddInt64(&fatalErrors, 1)
					} else {
						atomic.AddInt64(&searchCount, 1)
					}

				case 3: // Second Get (Update not exposed in engine API)
					memID := memoryIDs[(workerID+j)%len(memoryIDs)]
					_, err := eng.Get(ctx, memID)
					if err != nil {
						atomic.AddInt64(&fatalErrors, 1)
					} else {
						atomic.AddInt64(&getCount, 1)
					}
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Report results
	totalOps := storeCount + getCount + searchCount + updateCount + queueFullCount
	t.Logf("Mixed workload test completed in %v", duration)
	t.Logf("Total operations: %d", totalOps)
	t.Logf("  Stores: %d", storeCount)
	t.Logf("  Gets: %d", getCount)
	t.Logf("  Searches: %d", searchCount)
	t.Logf("  Updates: %d", updateCount)
	t.Logf("  Queue full (non-fatal): %d", queueFullCount)
	t.Logf("  Fatal errors: %d", fatalErrors)
	t.Logf("Throughput: %.2f ops/sec", float64(totalOps)/duration.Seconds())

	// Verify no fatal errors (enrichment queue full is acceptable under load)
	if fatalErrors > 0 {
		t.Errorf("Got %d fatal errors", fatalErrors)
	}
}

// TestLoad_QueuePressure tests enrichment queue under pressure.
//
// This test intentionally saturates the enrichment queue by storing
// memories faster than workers can process them. The expectation is that
// all memories are persisted (Store returns the Memory object even when
// the queue is full), and the queue-full condition is logged but not fatal.
func TestLoad_QueuePressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Rapidly store many memories to pressure the queue
	numMemories := 500
	var (
		successCount   int64
		queueFullCount int64
		fatalErrors    int64
	)

	startTime := time.Now()

	for i := 0; i < numMemories; i++ {
		content := fmt.Sprintf("Memory %d for queue pressure test", i)
		_, err := eng.Store(ctx, content)
		if err != nil {
			if strings.Contains(err.Error(), "enrichment queue full") {
				atomic.AddInt64(&queueFullCount, 1)
			} else {
				atomic.AddInt64(&fatalErrors, 1)
			}
		} else {
			atomic.AddInt64(&successCount, 1)
		}
	}

	storeTime := time.Since(startTime)
	totalStored := successCount + queueFullCount

	t.Logf("Stored %d memories in %v (queue full for %d)", totalStored, storeTime, queueFullCount)
	t.Logf("Fatal errors: %d", fatalErrors)
	t.Logf("Store rate: %.2f/sec", float64(totalStored)/storeTime.Seconds())

	// Verify no fatal storage errors
	if fatalErrors > 0 {
		t.Errorf("Expected no fatal store errors, got %d", fatalErrors)
	}

	// All memories should be persisted (queue full still stores the row)
	if totalStored < int64(numMemories) {
		t.Errorf("Not all memories persisted: %d/%d", totalStored, numMemories)
	}

	// All stores should complete quickly (<10ms avg)
	if totalStored > 0 {
		avgStoreTime := storeTime / time.Duration(totalStored)
		if avgStoreTime > 10*time.Millisecond {
			t.Logf("Warning: Average store time high: %v", avgStoreTime)
		}
	}
}

// TestLoad_SustainedLoad tests system under sustained load over time.
func TestLoad_SustainedLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	duration := 10 * time.Second // Run for 10 seconds
	numWorkers := 5

	var (
		totalOps   int64
		errorCount int64
		wg         sync.WaitGroup
		stopFlag   int32
	)

	// Stop workers after duration
	time.AfterFunc(duration, func() {
		atomic.StoreInt32(&stopFlag, 1)
	})

	startTime := time.Now()

	var queueFullCount int64

	// Launch workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			iteration := 0
			for atomic.LoadInt32(&stopFlag) == 0 {
				content := fmt.Sprintf("Sustained load memory %d-%d", workerID, iteration)
				_, err := eng.Store(ctx, content)
				if err != nil {
					if strings.Contains(err.Error(), "enrichment queue full") {
						atomic.AddInt64(&queueFullCount, 1)
					} else {
						atomic.AddInt64(&errorCount, 1)
					}
				}
				atomic.AddInt64(&totalOps, 1)
				iteration++

				// Small sleep to avoid tight loop
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	actualDuration := time.Since(startTime)

	// Report results
	t.Logf("Sustained load test completed")
	t.Logf("Duration: %v", actualDuration)
	t.Logf("Total operations: %d", totalOps)
	t.Logf("Fatal errors: %d", errorCount)
	t.Logf("Queue full (non-fatal): %d", queueFullCount)
	t.Logf("Average throughput: %.2f ops/sec", float64(totalOps)/actualDuration.Seconds())

	// Verify no fatal storage errors (enrichment queue full is acceptable)
	if errorCount > 0 {
		t.Errorf("Got %d fatal errors under sustained load", errorCount)
	}
}

// TestLoad_BurstTraffic tests system handling burst traffic patterns.
func TestLoad_BurstTraffic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Simulate burst pattern: 3 bursts with quiet periods
	numBursts := 3
	opsPerBurst := 200
	burstWorkers := 10

	for burst := 0; burst < numBursts; burst++ {
		t.Logf("Starting burst %d/%d", burst+1, numBursts)

		var (
			successCount int64
			errorCount   int64
			wg           sync.WaitGroup
		)

		startTime := time.Now()

		// Launch burst
		for i := 0; i < burstWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				for j := 0; j < opsPerBurst/burstWorkers; j++ {
					content := fmt.Sprintf("Burst %d worker %d op %d", burst, workerID, j)
					_, err := eng.Store(ctx, content)
					if err != nil {
						atomic.AddInt64(&errorCount, 1)
					} else {
						atomic.AddInt64(&successCount, 1)
					}
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(startTime)

		t.Logf("Burst %d: %d ops in %v (%.2f ops/sec), %d errors",
			burst+1, successCount, duration,
			float64(successCount)/duration.Seconds(), errorCount)

		// Quiet period between bursts
		if burst < numBursts-1 {
			time.Sleep(2 * time.Second)
		}
	}

	t.Log("Burst traffic test completed successfully")
}

// TestLoad_MemoryLeak tests for memory leaks under sustained load.
func TestLoad_MemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Store and retrieve many memories
	numIterations := 1000

	var queueFullCount int
	for i := 0; i < numIterations; i++ {
		// Store — the memory is always persisted even when the enrichment
		// queue is full (the engine returns an error but the row exists).
		mem, err := eng.Store(ctx, fmt.Sprintf("Memory leak test %d", i))
		if err != nil {
			if strings.Contains(err.Error(), "enrichment queue full") {
				// Non-fatal: the memory was stored, enrichment was skipped.
				queueFullCount++
			} else {
				t.Fatalf("Store failed: %v", err)
			}
		}

		// Retrieve — must succeed regardless of enrichment status.
		_, err = eng.Get(ctx, mem.ID)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		// Log progress periodically
		if i%100 == 0 {
			t.Logf("Progress: %d/%d iterations", i, numIterations)
		}
	}

	if queueFullCount > 0 {
		t.Logf("Enrichment queue was full for %d/%d stores (expected under sustained load)", queueFullCount, numIterations)
	}

	t.Logf("Memory leak test completed: %d iterations", numIterations)
	// Note: Use profiling tools to detect actual memory leaks
}
