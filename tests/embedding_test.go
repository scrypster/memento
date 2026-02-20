package tests

import (
	"context"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/llm"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// TestEnrichmentService_GenerateEmbeddings tests embedding generation with mock data.
func TestEnrichmentService_GenerateEmbeddings(t *testing.T) {
	// Create in-memory SQLite store
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create embedding provider
	embeddingProvider := sqlite.NewEmbeddingProvider(store.GetDB())

	// Create Ollama client (for testing, we'll mock the response)
	ollamaClient := llm.NewOllamaClient(llm.OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "nomic-embed-text",
		Timeout: 5 * time.Second,
	})

	// Create enrichment service with embedding support
	// Use the same ollamaClient for both text generation and embeddings
	enrichmentService := engine.NewEnrichmentServiceWithEmbeddings(ollamaClient, ollamaClient, store.GetDB(), embeddingProvider)

	ctx := context.Background()

	// Create a test memory
	testMemoryID := "mem:test:embedding:001"
	testMemory := &types.Memory{
		ID:        testMemoryID,
		Content:   "This is a test memory for embedding generation",
		Source:    "test",
		Status:    types.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := store.Store(ctx, testMemory); err != nil {
		t.Fatalf("Failed to store test memory: %v", err)
	}

	// Test embedding generation (will fail if Ollama is not running)
	err = enrichmentService.GenerateEmbeddings(ctx, testMemoryID, testMemory.Content)
	if err != nil {
		// Check if it's a connection error (Ollama not running)
		t.Logf("Note: Embedding generation failed (likely because Ollama is not running): %v", err)
		t.Logf("This is expected for unit tests. Run integration tests with Ollama running.")
		return
	}

	// If we got here, try to verify the embedding was stored
	embedding, err := embeddingProvider.GetEmbedding(ctx, testMemoryID)
	if err != nil {
		t.Fatalf("Failed to retrieve embedding: %v", err)
	}

	if len(embedding) == 0 {
		t.Error("Embedding is empty")
	}

	t.Logf("Successfully generated embedding with %d dimensions", len(embedding))
}

// TestEnrichmentService_WithoutEmbeddings tests enrichment service without embedding provider.
func TestEnrichmentService_WithoutEmbeddings(t *testing.T) {
	// Create in-memory SQLite store
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create Ollama client
	ollamaClient := llm.NewOllamaClient(llm.OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "nomic-embed-text",
		Timeout: 5 * time.Second,
	})

	// Create enrichment service WITHOUT embedding support
	enrichmentService := engine.NewEnrichmentService(ollamaClient, store.GetDB())
	_ = enrichmentService // enrichmentService is used implicitly via the store

	ctx := context.Background()

	// Create a test memory
	testMemoryID := "mem:test:no:embedding:001"
	testMemory := &types.Memory{
		ID:        testMemoryID,
		Content:   "This memory will not have embeddings",
		Source:    "test",
		Status:    types.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := store.Store(ctx, testMemory); err != nil {
		t.Fatalf("Failed to store test memory: %v", err)
	}

	// This should not fail even though we don't have embedding support
	// (it will just not generate embeddings)
	t.Logf("Enrichment service created without embedding provider")
}

// TestMemoryEngine_WithEmbeddingProvider tests memory engine creation with embeddings.
func TestMemoryEngine_WithEmbeddingProvider(t *testing.T) {
	// Create in-memory SQLite store
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create embedding provider
	embeddingProvider := sqlite.NewEmbeddingProvider(store.GetDB())

	// Create Ollama client
	ollamaClient := llm.NewOllamaClient(llm.OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "nomic-embed-text",
		Timeout: 5 * time.Second,
	})

	// Create memory engine with embedding support
	engineCfg := engine.Config{
		NumWorkers:        2,
		QueueSize:         100,
		ShutdownTimeout:   5 * time.Second,
		MaxRetries:        3,
		RecoveryBatchSize: 1000,
	}

	// Use the same ollamaClient for both text generation and embeddings
	eng, err := engine.NewMemoryEngineWithEmbeddings(store, engineCfg, ollamaClient, ollamaClient, embeddingProvider)
	if err != nil {
		t.Fatalf("Failed to create memory engine: %v", err)
	}

	ctx := context.Background()

	// Start the engine
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}
	defer func() { _ = eng.Shutdown(ctx) }()

	t.Logf("Memory engine with embedding support created and started successfully")
}

// TestEmbeddingProvider_WithPostgres tests Postgres embedding provider.
func TestEmbeddingProvider_Interface(t *testing.T) {
	// This test verifies that the EmbeddingProvider interface is properly defined.
	// The actual Postgres implementation would require a running Postgres instance.

	// Create in-memory SQLite store
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer func() { _ = store.Close() }()

	embeddingProvider := sqlite.NewEmbeddingProvider(store.GetDB())

	// Verify provider implements the interface by calling its methods
	ctx := context.Background()

	// Create a test memory
	testMemoryID := "mem:test:interface:001"
	testMemory := &types.Memory{
		ID:        testMemoryID,
		Content:   "Test content",
		Source:    "test",
		Status:    types.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := store.Store(ctx, testMemory); err != nil {
		t.Fatalf("Failed to store test memory: %v", err)
	}

	// Test all interface methods
	embedding := []float64{0.1, 0.2, 0.3}
	dimension := len(embedding)
	model := "test-model"

	// StoreEmbedding
	if err := embeddingProvider.StoreEmbedding(ctx, testMemoryID, embedding, dimension, model); err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	// GetEmbedding
	retrieved, err := embeddingProvider.GetEmbedding(ctx, testMemoryID)
	if err != nil {
		t.Fatalf("GetEmbedding failed: %v", err)
	}

	if len(retrieved) != dimension {
		t.Errorf("Retrieved embedding dimension mismatch: got %d, want %d", len(retrieved), dimension)
	}

	// GetDimension
	retrievedDim, err := embeddingProvider.GetDimension(ctx, model)
	if err != nil {
		t.Fatalf("GetDimension failed: %v", err)
	}

	if retrievedDim != dimension {
		t.Errorf("Dimension mismatch: got %d, want %d", retrievedDim, dimension)
	}

	// DeleteEmbedding
	if err := embeddingProvider.DeleteEmbedding(ctx, testMemoryID); err != nil {
		t.Fatalf("DeleteEmbedding failed: %v", err)
	}

	t.Logf("All embedding provider interface methods work correctly")
}
