package benchmarks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/llm"
	storageTypes "github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// NewBenchEngine creates a memory engine for benchmarking.
func NewBenchEngine(b *testing.B) (*engine.MemoryEngine, func()) {
	b.Helper()

	// Create temp directory
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench.db")

	// Create memory store
	store, err := sqlite.NewMemoryStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}

	// Run migrations
	migrationsDir := filepath.Join("..", "..", "migrations")
	if err := store.RunMigrations(migrationsDir); err != nil {
		b.Fatalf("Failed to run migrations: %v", err)
	}

	// Create engine
	config := engine.Config{
		NumWorkers: 4,
		QueueSize:  1000,
	}

	eng, err := engine.NewMemoryEngine(store, config, nil)
	if err != nil {
		b.Fatalf("Failed to create engine: %v", err)
	}

	// Start engine
	if err := eng.Start(context.Background()); err != nil {
		b.Fatalf("Failed to start engine: %v", err)
	}

	cleanup := func() {
		eng.Shutdown(context.Background())
		store.Close()
	}

	return eng, cleanup
}

// BenchmarkStore measures Store() operation latency.
func BenchmarkStore(b *testing.B) {
	eng, cleanup := NewBenchEngine(b)
	defer cleanup()

	ctx := context.Background()
	content := "Test memory content for benchmarking"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := eng.Store(ctx, content)
		if err != nil {
			b.Fatalf("Store failed: %v", err)
		}
	}
}

// BenchmarkStore_Parallel measures parallel Store() throughput.
func BenchmarkStore_Parallel(b *testing.B) {
	eng, cleanup := NewBenchEngine(b)
	defer cleanup()

	ctx := context.Background()
	content := "Test memory content"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := eng.Store(ctx, content)
			if err != nil {
				b.Fatalf("Store failed: %v", err)
			}
		}
	})
}

// BenchmarkGet measures Get() operation latency.
func BenchmarkGet(b *testing.B) {
	eng, cleanup := NewBenchEngine(b)
	defer cleanup()

	ctx := context.Background()

	// Store a memory first
	memory, err := eng.Store(ctx, "Benchmark memory")
	if err != nil {
		b.Fatalf("Store failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := eng.Get(ctx, memory.ID)
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
	}
}

// BenchmarkGet_Parallel measures parallel Get() throughput.
func BenchmarkGet_Parallel(b *testing.B) {
	eng, cleanup := NewBenchEngine(b)
	defer cleanup()

	ctx := context.Background()

	// Store a memory first
	memory, err := eng.Store(ctx, "Benchmark memory")
	if err != nil {
		b.Fatalf("Store failed: %v", err)
	}

	// Give time for write
	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := eng.Get(ctx, memory.ID)
			if err != nil {
				b.Fatalf("Get failed: %v", err)
			}
		}
	})
}

// BenchmarkUpdate measures Update() operation latency.
func BenchmarkUpdate(b *testing.B) {
	// Skip - Update not exposed in engine API
	b.Skip("Update benchmark requires direct store access")
}

// BenchmarkSearch measures Search() operation latency.
func BenchmarkSearch(b *testing.B) {
	eng, cleanup := NewBenchEngine(b)
	defer cleanup()

	ctx := context.Background()

	// Store multiple memories for searching
	for i := 0; i < 100; i++ {
		content := fmt.Sprintf("Memory %d about machine learning and AI", i)
		_, err := eng.Store(ctx, content)
		if err != nil {
			b.Fatalf("Store failed: %v", err)
		}
	}

	// Give time for writes
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := eng.Search(ctx, engine.SearchOptions{
			Query:  "machine learning",
			Limit:  10,
			Offset: 0,
		})
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

// BenchmarkSearch_LargeDataset measures search performance with large dataset.
func BenchmarkSearch_LargeDataset(b *testing.B) {
	eng, cleanup := NewBenchEngine(b)
	defer cleanup()

	ctx := context.Background()

	// Store 1000 memories
	for i := 0; i < 1000; i++ {
		content := fmt.Sprintf("Memory %d with various content about technology", i)
		_, err := eng.Store(ctx, content)
		if err != nil {
			b.Fatalf("Store failed: %v", err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := eng.Search(ctx, engine.SearchOptions{
			Query:  "technology",
			Limit:  10,
			Offset: 0,
		})
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

// BenchmarkList measures List() operation latency.
func BenchmarkList(b *testing.B) {
	eng, cleanup := NewBenchEngine(b)
	defer cleanup()

	ctx := context.Background()

	// Store memories
	for i := 0; i < 50; i++ {
		_, err := eng.Store(ctx, "Test memory")
		if err != nil {
			b.Fatalf("Store failed: %v", err)
		}
	}

	time.Sleep(50 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := eng.List(ctx, storageTypes.ListOptions{
			Page:  1,
			Limit: 10,
		})
		if err != nil {
			b.Fatalf("List failed: %v", err)
		}
	}
}

// BenchmarkEntityExtraction measures entity extraction latency (requires Ollama).
func BenchmarkEntityExtraction(b *testing.B) {
	// Check if Ollama is available
	client := llm.NewOllamaClient(llm.OllamaConfig{})
	if err := client.HealthCheck(context.Background()); err != nil {
		b.Skip("Ollama not available")
	}

	ctx := context.Background()
	content := "John Doe works at Google on Kubernetes project"
	prompt := llm.EntityExtractionPrompt(content)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.Complete(ctx, prompt)
		if err != nil {
			b.Fatalf("Complete failed: %v", err)
		}
	}
}

// BenchmarkEmbeddingGeneration skipped - API not exposed yet.
func BenchmarkEmbeddingGeneration(b *testing.B) {
	b.Skip("Embedding generation benchmark - API not exposed yet")
}

// BenchmarkChunking skipped - API not exposed yet.
func BenchmarkChunking(b *testing.B) {
	b.Skip("Chunking benchmark - API not exposed yet")
}

// BenchmarkEnrichmentPipeline measures full enrichment pipeline (requires Ollama).
func BenchmarkEnrichmentPipeline(b *testing.B) {
	client := llm.NewOllamaClient(llm.OllamaConfig{})
	if err := client.HealthCheck(context.Background()); err != nil {
		b.Skip("Ollama not available")
	}

	eng, cleanup := NewBenchEngine(b)
	defer cleanup()

	ctx := context.Background()
	content := "Alice and Bob work on Project Phoenix using Go programming"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		memory, err := eng.Store(ctx, content)
		if err != nil {
			b.Fatalf("Store failed: %v", err)
		}

		// Wait for enrichment (with timeout)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			retrieved, _ := eng.Get(ctx, memory.ID)
			if retrieved.Status == types.StatusEnriched {
				break
			}
			if retrieved.Status == types.StatusFailed {
				b.Fatalf("Enrichment failed")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// BenchmarkMemoryWithMetadata measures storing memories with metadata.
func BenchmarkMemoryWithMetadata(b *testing.B) {
	// Skip - Update not exposed in engine API
	b.Skip("Metadata benchmark requires Update API")
}

// BenchmarkConcurrentOperations measures mixed read/write workload.
func BenchmarkConcurrentOperations(b *testing.B) {
	eng, cleanup := NewBenchEngine(b)
	defer cleanup()

	ctx := context.Background()

	// Store some initial memories
	memoryIDs := make([]string, 10)
	for i := 0; i < 10; i++ {
		memory, err := eng.Store(ctx, fmt.Sprintf("Memory %d", i))
		if err != nil {
			b.Fatalf("Store failed: %v", err)
		}
		memoryIDs[i] = memory.ID
	}

	time.Sleep(50 * time.Millisecond)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Mix of operations
			switch i % 3 {
			case 0: // Store
				eng.Store(ctx, "New memory")
			case 1: // Get
				eng.Get(ctx, memoryIDs[i%len(memoryIDs)])
			case 2: // Search
				eng.Search(ctx, engine.SearchOptions{
					Query: "memory",
					Limit: 5,
				})
			}
			i++
		}
	})
}

// init sets up benchmark environment
func init() {
	// Ensure we're in the right directory
	if _, err := os.Stat("../../migrations"); err != nil {
		// Try to change to repo root
		os.Chdir("../..")
	}
}
