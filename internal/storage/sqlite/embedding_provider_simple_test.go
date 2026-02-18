package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

func TestEmbeddingProvider_StoreAndGet(t *testing.T) {
	// Create in-memory database
	store, err := NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	provider := NewEmbeddingProvider(store.db)
	ctx := context.Background()

	// Create test memory
	memoryID := "mem:test:001"
	testMemory := &types.Memory{
		ID:        memoryID,
		Content:   "Test content for embedding",
		Source:    "test",
		Status:    types.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.Store(ctx, testMemory); err != nil {
		t.Fatalf("Failed to create test memory: %v", err)
	}

	// Create and store embedding
	embedding := []float64{0.1, 0.2, 0.3, 0.4, 0.5}
	dimension := len(embedding)
	model := "nomic-embed-text"

	err = provider.StoreEmbedding(ctx, memoryID, embedding, dimension, model)
	if err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := provider.GetEmbedding(ctx, memoryID)
	if err != nil {
		t.Fatalf("GetEmbedding failed: %v", err)
	}

	if len(retrieved) != len(embedding) {
		t.Errorf("Length mismatch: got %d, want %d", len(retrieved), len(embedding))
	}

	for i, v := range retrieved {
		if abs(v-embedding[i]) > 1e-10 {
			t.Errorf("Value mismatch at index %d: got %f, want %f", i, v, embedding[i])
		}
	}
}

func TestEmbeddingProvider_Delete(t *testing.T) {
	store, err := NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	provider := NewEmbeddingProvider(store.db)
	ctx := context.Background()

	// Create test memory
	memoryID := "mem:test:002"
	testMemory := &types.Memory{
		ID:        memoryID,
		Content:   "Test content",
		Source:    "test",
		Status:    types.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.Store(ctx, testMemory); err != nil {
		t.Fatalf("Failed to create test memory: %v", err)
	}

	// Store embedding
	embedding := []float64{0.1, 0.2}
	err = provider.StoreEmbedding(ctx, memoryID, embedding, len(embedding), "test-model")
	if err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	// Delete embedding
	err = provider.DeleteEmbedding(ctx, memoryID)
	if err != nil {
		t.Fatalf("DeleteEmbedding failed: %v", err)
	}

	// Verify deletion
	_, err = provider.GetEmbedding(ctx, memoryID)
	if err != storage.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestEmbeddingProvider_GetDimension(t *testing.T) {
	store, err := NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	provider := NewEmbeddingProvider(store.db)
	ctx := context.Background()

	// Create test memory
	memoryID := "mem:test:003"
	testMemory := &types.Memory{
		ID:        memoryID,
		Content:   "Test content",
		Source:    "test",
		Status:    types.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.Store(ctx, testMemory); err != nil {
		t.Fatalf("Failed to create test memory: %v", err)
	}

	// Store embedding with specific dimension
	dimension := 384
	embedding := make([]float64, dimension)
	for i := 0; i < dimension; i++ {
		embedding[i] = float64(i) * 0.001
	}

	model := "nomic-embed-text"
	err = provider.StoreEmbedding(ctx, memoryID, embedding, dimension, model)
	if err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	// Get dimension for model
	retrievedDim, err := provider.GetDimension(ctx, model)
	if err != nil {
		t.Fatalf("GetDimension failed: %v", err)
	}

	if retrievedDim != dimension {
		t.Errorf("Dimension mismatch: got %d, want %d", retrievedDim, dimension)
	}
}

func TestEmbeddingProvider_NotFound(t *testing.T) {
	store, err := NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	provider := NewEmbeddingProvider(store.db)
	ctx := context.Background()

	// Try to get non-existent embedding
	_, err = provider.GetEmbedding(ctx, "mem:nonexistent:999")
	if err != storage.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}

	// Try to delete non-existent embedding
	err = provider.DeleteEmbedding(ctx, "mem:nonexistent:999")
	if err != storage.ErrNotFound {
		t.Errorf("Expected ErrNotFound on delete, got %v", err)
	}

	// Try to get dimension for non-existent model
	_, err = provider.GetDimension(ctx, "nonexistent-model")
	if err != storage.ErrNotFound {
		t.Errorf("Expected ErrNotFound on GetDimension, got %v", err)
	}
}

func TestEmbeddingProvider_InvalidInput(t *testing.T) {
	store, err := NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	provider := NewEmbeddingProvider(store.db)
	ctx := context.Background()

	testCases := []struct {
		name        string
		memoryID    string
		embedding   []float64
		dimension   int
		model       string
		expectedErr bool
	}{
		{
			name:        "Empty memory ID",
			memoryID:    "",
			embedding:   []float64{0.1},
			dimension:   1,
			model:       "test",
			expectedErr: true,
		},
		{
			name:        "Empty embedding",
			memoryID:    "mem:test",
			embedding:   []float64{},
			dimension:   0,
			model:       "test",
			expectedErr: true,
		},
		{
			name:        "Negative dimension",
			memoryID:    "mem:test",
			embedding:   []float64{0.1},
			dimension:   -1,
			model:       "test",
			expectedErr: true,
		},
		{
			name:        "Empty model",
			memoryID:    "mem:test",
			embedding:   []float64{0.1},
			dimension:   1,
			model:       "",
			expectedErr: true,
		},
		{
			name:        "Dimension mismatch",
			memoryID:    "mem:test",
			embedding:   []float64{0.1, 0.2},
			dimension:   3,
			model:       "test",
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := provider.StoreEmbedding(ctx, tc.memoryID, tc.embedding, tc.dimension, tc.model)
			if tc.expectedErr && err == nil {
				t.Errorf("Expected error but got nil")
			}
			if !tc.expectedErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestEmbeddingProvider_Precision(t *testing.T) {
	store, err := NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	provider := NewEmbeddingProvider(store.db)
	ctx := context.Background()

	// Create test memory
	memoryID := "mem:test:precision"
	testMemory := &types.Memory{
		ID:        memoryID,
		Content:   "Precision test",
		Source:    "test",
		Status:    types.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.Store(ctx, testMemory); err != nil {
		t.Fatalf("Failed to create test memory: %v", err)
	}

	// Test edge case values
	embedding := []float64{
		0.0,
		1.0,
		-1.0,
		1e-10,
		1e10,
		0.12345678901234567,
		-0.98765432109876543,
	}

	err = provider.StoreEmbedding(ctx, memoryID, embedding, len(embedding), "precision-model")
	if err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	retrieved, err := provider.GetEmbedding(ctx, memoryID)
	if err != nil {
		t.Fatalf("GetEmbedding failed: %v", err)
	}

	for i, v := range retrieved {
		if abs(v-embedding[i]) > 1e-10 {
			t.Errorf("Precision loss at index %d: got %f, want %f (diff: %e)",
				i, v, embedding[i], abs(v-embedding[i]))
		}
	}
}

func TestEmbeddingProvider_MultipleMemories(t *testing.T) {
	store, err := NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	provider := NewEmbeddingProvider(store.db)
	ctx := context.Background()

	numMemories := 5
	for i := 0; i < numMemories; i++ {
		// Create test memory
		memoryID := "mem:test:" + string(rune(i))
		testMemory := &types.Memory{
			ID:        memoryID,
			Content:   "Test content " + string(rune(i)),
			Source:    "test",
			Status:    types.StatusPending,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := store.Store(ctx, testMemory); err != nil {
			t.Fatalf("Failed to create test memory: %v", err)
		}

		// Store embedding
		embedding := []float64{float64(i) * 0.1, float64(i) * 0.2, float64(i) * 0.3}
		err = provider.StoreEmbedding(ctx, memoryID, embedding, len(embedding), "test-model")
		if err != nil {
			t.Fatalf("Failed to store embedding for memory %d: %v", i, err)
		}
	}

	// Verify all embeddings are retrievable
	for i := 0; i < numMemories; i++ {
		memoryID := "mem:test:" + string(rune(i))
		retrieved, err := provider.GetEmbedding(ctx, memoryID)
		if err != nil {
			t.Fatalf("Failed to get embedding for memory %d: %v", i, err)
		}

		if len(retrieved) != 3 {
			t.Errorf("Memory %d: expected 3 values, got %d", i, len(retrieved))
		}
	}
}

// Helper function for floating point comparison
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// BenchmarkStoreEmbedding benchmarks embedding storage
func BenchmarkStoreEmbedding(b *testing.B) {
	store, err := NewMemoryStore(":memory:")
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	provider := NewEmbeddingProvider(store.db)
	ctx := context.Background()

	dimension := 384
	embedding := make([]float64, dimension)
	for i := range embedding {
		embedding[i] = float64(i) * 0.001
	}

	// Pre-create test memories
	for i := 0; i < b.N; i++ {
		memoryID := "mem:bench:" + string(rune(i % 100))
		testMemory := &types.Memory{
			ID:        memoryID,
			Content:   "Benchmark test",
			Source:    "test",
			Status:    types.StatusPending,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		store.Store(context.Background(), testMemory)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		memoryID := "mem:bench:" + string(rune(i % 100))
		_ = provider.StoreEmbedding(ctx, memoryID, embedding, dimension, "bench-model")
	}
}

// BenchmarkGetEmbedding benchmarks embedding retrieval
func BenchmarkGetEmbedding(b *testing.B) {
	store, err := NewMemoryStore(":memory:")
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	provider := NewEmbeddingProvider(store.db)
	ctx := context.Background()

	memoryID := "mem:bench:001"
	testMemory := &types.Memory{
		ID:        memoryID,
		Content:   "Benchmark test",
		Source:    "test",
		Status:    types.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.Store(ctx, testMemory)

	dimension := 384
	embedding := make([]float64, dimension)
	for i := range embedding {
		embedding[i] = float64(i) * 0.001
	}

	provider.StoreEmbedding(ctx, memoryID, embedding, dimension, "bench-model")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = provider.GetEmbedding(ctx, memoryID)
	}
}
