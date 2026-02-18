package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// TestE2E_StoreAndRecall tests the basic store and recall workflow.
// This validates that memories can be stored and retrieved successfully.
func TestE2E_StoreAndRecall(t *testing.T) {
	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Store a memory
	content := "John Doe works at Acme Corp on the Phoenix project"
	memory, err := eng.Store(ctx, content)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Verify immediate response
	if memory.ID == "" {
		t.Error("Expected non-empty memory ID")
	}

	if memory.Content != content {
		t.Errorf("Content mismatch. Expected %q, got %q", content, memory.Content)
	}

	if memory.Status != types.StatusPending {
		t.Errorf("Expected status %s, got %s", types.StatusPending, memory.Status)
	}

	// Retrieve the memory immediately
	retrieved, err := eng.Get(ctx, memory.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Verify retrieved content matches
	if retrieved.Content != content {
		t.Errorf("Retrieved content mismatch. Expected %q, got %q", content, retrieved.Content)
	}
}

// TestE2E_SearchWorkflow tests the search functionality with multiple memories.
func TestE2E_SearchWorkflow(t *testing.T) {
	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Store multiple memories
	memories := []string{
		"Machine learning basics and introduction",
		"Deep learning with neural networks",
		"Natural language processing fundamentals",
		"Computer vision and image recognition",
	}

	for _, content := range memories {
		_, err := eng.Store(ctx, content)
		if err != nil {
			t.Fatalf("Store failed: %v", err)
		}
	}

	// Wait for memories to be written with polling instead of fixed sleep
	var results []engine.SearchResult
	var searchErr error
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	foundResults := false
	for !foundResults {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for search results")
		case <-ticker.C:
			// Search for AI/ML related content
			results, searchErr = eng.Search(ctx, engine.SearchOptions{
				Query:  "machine learning neural networks",
				Limit:  10,
				Offset: 0,
			})

			if searchErr != nil {
				t.Fatalf("Search failed: %v", searchErr)
			}

			// Should find at least some results
			if len(results) > 0 {
				foundResults = true
			}
		}
	}

	if len(results) == 0 {
		t.Error("Expected at least one search result")
	}

	// Verify results contain relevant memories
	foundRelevant := false
	for _, result := range results {
		if result.Memory.Content == "Machine learning basics and introduction" ||
			result.Memory.Content == "Deep learning with neural networks" {
			foundRelevant = true
			break
		}
	}

	if !foundRelevant {
		t.Error("Expected to find relevant memories in search results")
	}
}

// TestE2E_EnrichmentPipeline tests the full enrichment pipeline.
//
// When no LLM provider is configured (the default in test environments),
// the enrichment worker skips LLM enrichment and marks the memory as
// enriched without populating entities. Entity extraction requires an
// actual LLM backend (Ollama). This test verifies the enrichment
// lifecycle transitions correctly regardless of LLM availability.
func TestE2E_EnrichmentPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Store a memory with clear entity and relationship content
	content := `John Doe is a software engineer at Acme Corporation.
He works on the Phoenix project with Jane Smith.
They use Go and Python for backend development.`

	memory, err := eng.Store(ctx, content)
	if err != nil && !strings.Contains(err.Error(), "enrichment queue full") {
		t.Fatalf("Store failed: %v", err)
	}

	// Wait for enrichment with timeout and polling
	timeout := time.After(15 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	enriched := false
	for !enriched {
		select {
		case <-timeout:
			t.Fatal("Enrichment timeout - memory not enriched within deadline")
		case <-ticker.C:
			retrieved, err := eng.Get(ctx, memory.ID)
			if err != nil {
				t.Fatalf("Get failed: %v", err)
			}

			if retrieved.Status == types.StatusEnriched {
				enriched = true

				// Log enrichment results for debugging.
				// Note: Without an LLM provider, entities will be empty.
				// That is expected -- entity extraction requires Ollama.
				t.Logf("Enrichment completed. Entities: %v", retrieved.Entities)
			}

			if retrieved.Status == types.StatusFailed {
				t.Fatalf("Enrichment failed for memory %s", memory.ID)
			}
		}
	}

	if !enriched {
		t.Error("Enrichment timeout - memory not enriched within deadline")
	}
}

// TestE2E_ConcurrentStores tests concurrent Store operations.
func TestE2E_ConcurrentStores(t *testing.T) {
	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	numGoroutines := 5
	storesPerGoroutine := 10

	errors := make(chan error, numGoroutines)
	memoryIDs := make(chan string, numGoroutines*storesPerGoroutine)

	// Store memories concurrently
	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			for j := 0; j < storesPerGoroutine; j++ {
				content := "Test memory from concurrent worker"
				memory, err := eng.Store(ctx, content)
				if err != nil {
					// Enrichment queue full is non-fatal -- memory is still persisted.
					if strings.Contains(err.Error(), "enrichment queue full") {
						memoryIDs <- memory.ID
						continue
					}
					errors <- err
					return
				}
				memoryIDs <- memory.ID
			}
			errors <- nil
		}(i)
	}

	// Check for errors
	for i := 0; i < numGoroutines; i++ {
		if err := <-errors; err != nil {
			t.Fatalf("Concurrent store failed: %v", err)
		}
	}

	close(memoryIDs)

	// Verify all memories were stored
	count := 0
	for range memoryIDs {
		count++
	}

	expectedCount := numGoroutines * storesPerGoroutine
	if count != expectedCount {
		t.Errorf("Expected %d memories stored, got %d", expectedCount, count)
	}
}

// TestE2E_UpdateMemory tests updating an existing memory.
func TestE2E_UpdateMemory(t *testing.T) {
	// Skip - engine doesn't expose Update directly
	// Updates would typically be done through Store (upsert semantics)
	t.Skip("Update test requires direct store access - will add when API is exposed")
}

// TestE2E_DeleteMemory tests deleting a memory.
func TestE2E_DeleteMemory(t *testing.T) {
	// Skip - engine doesn't expose Delete directly
	// Deletion would typically be done through store layer
	t.Skip("Delete test requires direct store access - will add when API is exposed")
}

// TestE2E_ListMemories tests listing all memories with pagination.
func TestE2E_ListMemories(t *testing.T) {
	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Store multiple memories
	numMemories := 15
	for i := 0; i < numMemories; i++ {
		_, err := eng.Store(ctx, "Test memory")
		if err != nil && !strings.Contains(err.Error(), "enrichment queue full") {
			t.Fatalf("Store failed: %v", err)
		}
	}

	// Give time for writes
	time.Sleep(100 * time.Millisecond)

	// List first page
	result, err := eng.List(ctx, storage.ListOptions{
		Page:  1,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(result.Items) != 10 {
		t.Errorf("Expected 10 memories on first page, got %d", len(result.Items))
	}

	// List second page
	result, err = eng.List(ctx, storage.ListOptions{
		Page:  2,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(result.Items) != 5 {
		t.Errorf("Expected 5 memories on second page, got %d", len(result.Items))
	}
}
