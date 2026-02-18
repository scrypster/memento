package tests

import (
	"context"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// TestSQLiteMemoryStore tests basic CRUD operations on the SQLite memory store.
func TestSQLiteMemoryStore(t *testing.T) {
	// Create an in-memory SQLite store for testing
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Test data
	memory := &types.Memory{
		ID:        "mem:test:example-1",
		Content:   "This is a test memory",
		Source:    "manual",
		Domain:    "test",
		Timestamp: time.Now(),
		Tags:      []string{"test", "example"},
		Metadata: map[string]interface{}{
			"author": "test-user",
			"priority": "high",
		},
	}

	// Test Store (create)
	t.Run("Store_Create", func(t *testing.T) {
		err := store.Store(ctx, memory)
		if err != nil {
			t.Fatalf("Failed to store memory: %v", err)
		}
	})

	// Test Get
	t.Run("Get", func(t *testing.T) {
		retrieved, err := store.Get(ctx, memory.ID)
		if err != nil {
			t.Fatalf("Failed to get memory: %v", err)
		}

		if retrieved.ID != memory.ID {
			t.Errorf("Expected ID %s, got %s", memory.ID, retrieved.ID)
		}

		if retrieved.Content != memory.Content {
			t.Errorf("Expected content %s, got %s", memory.Content, retrieved.Content)
		}

		if retrieved.Source != memory.Source {
			t.Errorf("Expected source %s, got %s", memory.Source, retrieved.Source)
		}

		// Check default status was set
		if retrieved.Status != types.StatusPending {
			t.Errorf("Expected status %s, got %s", types.StatusPending, retrieved.Status)
		}

		if retrieved.EntityStatus != types.EnrichmentPending {
			t.Errorf("Expected entity_status %s, got %s", types.EnrichmentPending, retrieved.EntityStatus)
		}

		// Check tags
		if len(retrieved.Tags) != len(memory.Tags) {
			t.Errorf("Expected %d tags, got %d", len(memory.Tags), len(retrieved.Tags))
		}

		// Check metadata
		if retrieved.Metadata == nil {
			t.Error("Expected metadata to be non-nil")
		} else {
			if author, ok := retrieved.Metadata["author"].(string); !ok || author != "test-user" {
				t.Errorf("Expected author metadata 'test-user', got %v", retrieved.Metadata["author"])
			}
		}
	})

	// Test Store (update/upsert)
	t.Run("Store_Update", func(t *testing.T) {
		memory.Content = "Updated content"
		memory.Tags = []string{"test", "updated"}

		err := store.Store(ctx, memory)
		if err != nil {
			t.Fatalf("Failed to update memory: %v", err)
		}

		retrieved, err := store.Get(ctx, memory.ID)
		if err != nil {
			t.Fatalf("Failed to get updated memory: %v", err)
		}

		if retrieved.Content != "Updated content" {
			t.Errorf("Expected updated content, got %s", retrieved.Content)
		}
	})

	// Test Update
	t.Run("Update", func(t *testing.T) {
		memory.Content = "Updated via Update method"
		err := store.Update(ctx, memory)
		if err != nil {
			t.Fatalf("Failed to update memory: %v", err)
		}

		retrieved, err := store.Get(ctx, memory.ID)
		if err != nil {
			t.Fatalf("Failed to get updated memory: %v", err)
		}

		if retrieved.Content != "Updated via Update method" {
			t.Errorf("Expected updated content, got %s", retrieved.Content)
		}
	})

	// Test UpdateStatus
	t.Run("UpdateStatus", func(t *testing.T) {
		err := store.UpdateStatus(ctx, memory.ID, types.StatusProcessing)
		if err != nil {
			t.Fatalf("Failed to update status: %v", err)
		}

		retrieved, err := store.Get(ctx, memory.ID)
		if err != nil {
			t.Fatalf("Failed to get memory after status update: %v", err)
		}

		if retrieved.Status != types.StatusProcessing {
			t.Errorf("Expected status %s, got %s", types.StatusProcessing, retrieved.Status)
		}
	})

	// Test UpdateEnrichment
	t.Run("UpdateEnrichment", func(t *testing.T) {
		now := time.Now()
		enrichment := storage.EnrichmentUpdate{
			EntityStatus:       types.EnrichmentCompleted,
			RelationshipStatus: types.EnrichmentCompleted,
			EmbeddingStatus:    types.EnrichmentCompleted,
			EnrichmentAttempts: 1,
			EnrichmentError:    "",
			EnrichedAt:         &now,
		}

		err := store.UpdateEnrichment(ctx, memory.ID, enrichment)
		if err != nil {
			t.Fatalf("Failed to update enrichment: %v", err)
		}

		retrieved, err := store.Get(ctx, memory.ID)
		if err != nil {
			t.Fatalf("Failed to get memory after enrichment update: %v", err)
		}

		if retrieved.EntityStatus != types.EnrichmentCompleted {
			t.Errorf("Expected entity_status %s, got %s", types.EnrichmentCompleted, retrieved.EntityStatus)
		}

		if retrieved.EnrichmentAttempts != 1 {
			t.Errorf("Expected enrichment_attempts 1, got %d", retrieved.EnrichmentAttempts)
		}

		if retrieved.EnrichedAt == nil {
			t.Error("Expected enriched_at to be set")
		}
	})

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		err := store.Delete(ctx, memory.ID)
		if err != nil {
			t.Fatalf("Failed to delete memory: %v", err)
		}

		// Verify it's deleted
		_, err = store.Get(ctx, memory.ID)
		if err != storage.ErrNotFound {
			t.Errorf("Expected ErrNotFound, got %v", err)
		}
	})

	// Test error cases
	t.Run("Get_NotFound", func(t *testing.T) {
		_, err := store.Get(ctx, "mem:nonexistent:id")
		if err != storage.ErrNotFound {
			t.Errorf("Expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		nonexistent := &types.Memory{
			ID:      "mem:nonexistent:id",
			Content: "Test",
			Source:  "test",
		}
		err := store.Update(ctx, nonexistent)
		if err != storage.ErrNotFound {
			t.Errorf("Expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		err := store.Delete(ctx, "mem:nonexistent:id")
		if err != storage.ErrNotFound {
			t.Errorf("Expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Store_InvalidInput", func(t *testing.T) {
		// Nil memory
		err := store.Store(ctx, nil)
		if err != storage.ErrInvalidInput {
			t.Errorf("Expected ErrInvalidInput for nil memory, got %v", err)
		}

		// Empty ID
		invalid := &types.Memory{Content: "Test", Source: "test"}
		err = store.Store(ctx, invalid)
		if err == nil || err == storage.ErrInvalidInput {
			// Should error on missing ID
			if err == nil {
				t.Error("Expected error for missing ID")
			}
		}

		// Empty content
		invalid = &types.Memory{ID: "mem:test:id", Source: "test"}
		err = store.Store(ctx, invalid)
		if err == nil || err == storage.ErrInvalidInput {
			if err == nil {
				t.Error("Expected error for missing content")
			}
		}
	})
}

// TestMemoryStorePagination tests the List operation with pagination.
func TestMemoryStorePagination(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create 25 test memories
	for i := 1; i <= 25; i++ {
		memory := &types.Memory{
			ID:      "mem:test:pagination-" + string(rune('a'+i-1)),
			Content: "Test memory " + string(rune('0'+i)),
			Source:  "test",
			Domain:  "test",
		}
		err := store.Store(ctx, memory)
		if err != nil {
			t.Fatalf("Failed to store memory %d: %v", i, err)
		}
	}

	// Test default pagination (page 1, limit 10)
	t.Run("DefaultPagination", func(t *testing.T) {
		opts := storage.ListOptions{}
		result, err := store.List(ctx, opts)
		if err != nil {
			t.Fatalf("Failed to list memories: %v", err)
		}

		if result.Total != 25 {
			t.Errorf("Expected total 25, got %d", result.Total)
		}

		if len(result.Items) != 10 {
			t.Errorf("Expected 10 items, got %d", len(result.Items))
		}

		if result.Page != 1 {
			t.Errorf("Expected page 1, got %d", result.Page)
		}

		if result.PageSize != 10 {
			t.Errorf("Expected page size 10, got %d", result.PageSize)
		}

		if !result.HasMore {
			t.Error("Expected HasMore to be true")
		}
	})

	// Test page 2
	t.Run("Page2", func(t *testing.T) {
		opts := storage.ListOptions{Page: 2, Limit: 10}
		result, err := store.List(ctx, opts)
		if err != nil {
			t.Fatalf("Failed to list memories: %v", err)
		}

		if len(result.Items) != 10 {
			t.Errorf("Expected 10 items, got %d", len(result.Items))
		}

		if result.Page != 2 {
			t.Errorf("Expected page 2, got %d", result.Page)
		}

		if !result.HasMore {
			t.Error("Expected HasMore to be true")
		}
	})

	// Test page 3 (last page with 5 items)
	t.Run("Page3_LastPage", func(t *testing.T) {
		opts := storage.ListOptions{Page: 3, Limit: 10}
		result, err := store.List(ctx, opts)
		if err != nil {
			t.Fatalf("Failed to list memories: %v", err)
		}

		if len(result.Items) != 5 {
			t.Errorf("Expected 5 items, got %d", len(result.Items))
		}

		if result.HasMore {
			t.Error("Expected HasMore to be false on last page")
		}
	})

	// Test custom page size
	t.Run("CustomPageSize", func(t *testing.T) {
		opts := storage.ListOptions{Page: 1, Limit: 20}
		result, err := store.List(ctx, opts)
		if err != nil {
			t.Fatalf("Failed to list memories: %v", err)
		}

		if len(result.Items) != 20 {
			t.Errorf("Expected 20 items, got %d", len(result.Items))
		}

		if result.PageSize != 20 {
			t.Errorf("Expected page size 20, got %d", result.PageSize)
		}
	})

	// Test max limit enforcement
	t.Run("MaxLimit", func(t *testing.T) {
		opts := storage.ListOptions{Page: 1, Limit: 200} // Exceeds max of 100
		result, err := store.List(ctx, opts)
		if err != nil {
			t.Fatalf("Failed to list memories: %v", err)
		}

		if result.PageSize != 100 {
			t.Errorf("Expected page size to be capped at 100, got %d", result.PageSize)
		}
	})

	// Test filtering by status
	t.Run("FilterByStatus", func(t *testing.T) {
		// Update some memories to have different status
		for i := 1; i <= 5; i++ {
			id := "mem:test:pagination-" + string(rune('a'+i-1))
			err := store.UpdateStatus(ctx, id, types.StatusEnriched)
			if err != nil {
				t.Fatalf("Failed to update status: %v", err)
			}
		}

		opts := storage.ListOptions{
			Filter: map[string]interface{}{
				"status": types.StatusEnriched,
			},
		}
		result, err := store.List(ctx, opts)
		if err != nil {
			t.Fatalf("Failed to list memories with filter: %v", err)
		}

		if result.Total != 5 {
			t.Errorf("Expected 5 enriched memories, got %d", result.Total)
		}

		for _, mem := range result.Items {
			if mem.Status != types.StatusEnriched {
				t.Errorf("Expected status %s, got %s", types.StatusEnriched, mem.Status)
			}
		}
	})

	// Test sorting
	t.Run("SortByCreatedAt", func(t *testing.T) {
		opts := storage.ListOptions{
			SortBy:    "created_at",
			SortOrder: "asc",
			Limit:     5,
		}
		result, err := store.List(ctx, opts)
		if err != nil {
			t.Fatalf("Failed to list memories: %v", err)
		}

		// Verify ascending order
		for i := 1; i < len(result.Items); i++ {
			if result.Items[i].CreatedAt.Before(result.Items[i-1].CreatedAt) {
				t.Error("Expected ascending order by created_at")
				break
			}
		}
	})
}

// TestMemoryStoreIdempotency tests that Store operations are idempotent (upsert semantics).
func TestMemoryStoreIdempotency(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	memory := &types.Memory{
		ID:      "mem:test:idempotent",
		Content: "Original content",
		Source:  "test",
	}

	// First store
	err = store.Store(ctx, memory)
	if err != nil {
		t.Fatalf("Failed to store memory: %v", err)
	}

	// Get and verify
	retrieved1, err := store.Get(ctx, memory.ID)
	if err != nil {
		t.Fatalf("Failed to get memory: %v", err)
	}

	if retrieved1.Content != "Original content" {
		t.Errorf("Expected 'Original content', got %s", retrieved1.Content)
	}

	// Store again with updated content (upsert)
	memory.Content = "Updated content"
	err = store.Store(ctx, memory)
	if err != nil {
		t.Fatalf("Failed to store memory again: %v", err)
	}

	// Get and verify update
	retrieved2, err := store.Get(ctx, memory.ID)
	if err != nil {
		t.Fatalf("Failed to get memory after update: %v", err)
	}

	if retrieved2.Content != "Updated content" {
		t.Errorf("Expected 'Updated content', got %s", retrieved2.Content)
	}

	// Verify only one record exists
	opts := storage.ListOptions{}
	result, err := store.List(ctx, opts)
	if err != nil {
		t.Fatalf("Failed to list memories: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Expected exactly 1 memory, got %d", result.Total)
	}
}

// TestEnrichmentStatusUpdates tests enrichment status tracking.
func TestEnrichmentStatusUpdates(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	memory := &types.Memory{
		ID:      "mem:test:enrichment",
		Content: "Test enrichment tracking",
		Source:  "test",
	}

	// Store initial memory
	err = store.Store(ctx, memory)
	if err != nil {
		t.Fatalf("Failed to store memory: %v", err)
	}

	// Verify default enrichment statuses
	retrieved, err := store.Get(ctx, memory.ID)
	if err != nil {
		t.Fatalf("Failed to get memory: %v", err)
	}

	if retrieved.EntityStatus != types.EnrichmentPending {
		t.Errorf("Expected entity_status %s, got %s", types.EnrichmentPending, retrieved.EntityStatus)
	}

	if retrieved.RelationshipStatus != types.EnrichmentPending {
		t.Errorf("Expected relationship_status %s, got %s", types.EnrichmentPending, retrieved.RelationshipStatus)
	}

	if retrieved.EmbeddingStatus != types.EnrichmentPending {
		t.Errorf("Expected embedding_status %s, got %s", types.EnrichmentPending, retrieved.EmbeddingStatus)
	}

	// Test successful enrichment
	t.Run("SuccessfulEnrichment", func(t *testing.T) {
		now := time.Now()
		enrichment := storage.EnrichmentUpdate{
			EntityStatus:       types.EnrichmentCompleted,
			RelationshipStatus: types.EnrichmentCompleted,
			EmbeddingStatus:    types.EnrichmentCompleted,
			EnrichmentAttempts: 1,
			EnrichedAt:         &now,
		}

		err := store.UpdateEnrichment(ctx, memory.ID, enrichment)
		if err != nil {
			t.Fatalf("Failed to update enrichment: %v", err)
		}

		retrieved, err := store.Get(ctx, memory.ID)
		if err != nil {
			t.Fatalf("Failed to get memory: %v", err)
		}

		if retrieved.EntityStatus != types.EnrichmentCompleted {
			t.Errorf("Expected entity_status %s, got %s", types.EnrichmentCompleted, retrieved.EntityStatus)
		}

		if retrieved.EnrichmentAttempts != 1 {
			t.Errorf("Expected 1 enrichment attempt, got %d", retrieved.EnrichmentAttempts)
		}

		if retrieved.EnrichedAt == nil {
			t.Error("Expected enriched_at to be set")
		}
	})

	// Test failed enrichment with retry tracking
	t.Run("FailedEnrichment", func(t *testing.T) {
		enrichment := storage.EnrichmentUpdate{
			EntityStatus:       types.EnrichmentFailed,
			RelationshipStatus: types.EnrichmentPending,
			EmbeddingStatus:    types.EnrichmentPending,
			EnrichmentAttempts: 3,
			EnrichmentError:    "LLM API timeout",
		}

		err := store.UpdateEnrichment(ctx, memory.ID, enrichment)
		if err != nil {
			t.Fatalf("Failed to update enrichment: %v", err)
		}

		retrieved, err := store.Get(ctx, memory.ID)
		if err != nil {
			t.Fatalf("Failed to get memory: %v", err)
		}

		if retrieved.EntityStatus != types.EnrichmentFailed {
			t.Errorf("Expected entity_status %s, got %s", types.EnrichmentFailed, retrieved.EntityStatus)
		}

		if retrieved.EnrichmentAttempts != 3 {
			t.Errorf("Expected 3 enrichment attempts, got %d", retrieved.EnrichmentAttempts)
		}

		if retrieved.EnrichmentError != "LLM API timeout" {
			t.Errorf("Expected error message 'LLM API timeout', got %s", retrieved.EnrichmentError)
		}
	})

	// Test partial enrichment (some tasks completed, some pending)
	t.Run("PartialEnrichment", func(t *testing.T) {
		enrichment := storage.EnrichmentUpdate{
			EntityStatus:       types.EnrichmentCompleted,
			RelationshipStatus: types.EnrichmentProcessing,
			EmbeddingStatus:    types.EnrichmentPending,
			EnrichmentAttempts: 1,
		}

		err := store.UpdateEnrichment(ctx, memory.ID, enrichment)
		if err != nil {
			t.Fatalf("Failed to update enrichment: %v", err)
		}

		retrieved, err := store.Get(ctx, memory.ID)
		if err != nil {
			t.Fatalf("Failed to get memory: %v", err)
		}

		if retrieved.EntityStatus != types.EnrichmentCompleted {
			t.Errorf("Expected entity_status %s, got %s", types.EnrichmentCompleted, retrieved.EntityStatus)
		}

		if retrieved.RelationshipStatus != types.EnrichmentProcessing {
			t.Errorf("Expected relationship_status %s, got %s", types.EnrichmentProcessing, retrieved.RelationshipStatus)
		}

		if retrieved.EmbeddingStatus != types.EnrichmentPending {
			t.Errorf("Expected embedding_status %s, got %s", types.EnrichmentPending, retrieved.EmbeddingStatus)
		}
	})
}
