package tests

import (
	"context"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// Helper function to create a test memory store
func createTestMemoryStore(t *testing.T) storage.MemoryStore {
	t.Helper()

	// Use in-memory SQLite for testing
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test memory store: %v", err)
	}

	return store
}

// Helper function to create test memories
func createTestMemory(t *testing.T, store storage.MemoryStore, content string, importance float64, tags []string) *types.Memory {
	t.Helper()

	memory := &types.Memory{
		ID:         engine.GenerateMemoryID("test", ""),
		Content:    content,
		Status:     types.StatusEnriched,
		Importance: importance,
		Tags:       tags,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := store.Store(context.Background(), memory); err != nil {
		t.Fatalf("Failed to store test memory: %v", err)
	}

	return memory
}

// TestSearchOrchestrator_BasicSearch tests basic search functionality
func TestSearchOrchestrator_BasicSearch(t *testing.T) {
	store := createTestMemoryStore(t)
	defer store.Close()

	orchestrator := engine.NewSearchOrchestrator(store)

	// Create test memories.
	// NOTE: FTS5 searches memory content only, not tags. Test content must
	// include the search term directly so FTS5 can find the document.
	mem1 := createTestMemory(t, store, "Go programming language tutorial", 0.8, []string{"programming", "go"})
	mem2 := createTestMemory(t, store, "Python programming data science guide", 0.6, []string{"programming", "python"})
	mem3 := createTestMemory(t, store, "Machine learning with Go", 0.9, []string{"ml", "go"})

	tests := []struct {
		name           string
		query          string
		expectedIDs    []string
		minExpectedLen int
	}{
		{
			name:           "Search for Go",
			query:          "Go",
			expectedIDs:    []string{mem1.ID, mem3.ID},
			minExpectedLen: 2,
		},
		{
			name:           "Search for programming",
			query:          "programming",
			expectedIDs:    []string{mem1.ID, mem2.ID},
			minExpectedLen: 2,
		},
		{
			name:           "Search with no results",
			query:          "nonexistent",
			expectedIDs:    []string{},
			minExpectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := engine.SearchOptions{
				Query:  tt.query,
				Limit:  10,
				Offset: 0,
			}

			results, err := orchestrator.Search(context.Background(), opts)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			if len(results) < tt.minExpectedLen {
				t.Errorf("Expected at least %d results, got %d", tt.minExpectedLen, len(results))
			}

			// Verify results contain expected IDs
			for _, expectedID := range tt.expectedIDs {
				found := false
				for _, result := range results {
					if result.Memory.ID == expectedID {
						found = true
						break
					}
				}
				if !found && tt.minExpectedLen > 0 {
					t.Errorf("Expected to find memory %s in results", expectedID)
				}
			}
		})
	}
}

// TestSearchOrchestrator_Pagination tests pagination functionality
func TestSearchOrchestrator_Pagination(t *testing.T) {
	store := createTestMemoryStore(t)
	defer store.Close()

	orchestrator := engine.NewSearchOrchestrator(store)

	// Create 10 test memories
	for i := 0; i < 10; i++ {
		createTestMemory(t, store, "Test memory with programming content", 0.5, []string{"test"})
	}

	// Test first page
	opts := engine.SearchOptions{
		Query:  "programming",
		Limit:  3,
		Offset: 0,
	}

	results, err := orchestrator.Search(context.Background(), opts)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results on first page, got %d", len(results))
	}

	// Test second page
	opts.Offset = 3
	results, err = orchestrator.Search(context.Background(), opts)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results on second page, got %d", len(results))
	}
}

// TestSearchOrchestrator_ScoreComponents tests score breakdown
func TestSearchOrchestrator_ScoreComponents(t *testing.T) {
	store := createTestMemoryStore(t)
	defer store.Close()

	orchestrator := engine.NewSearchOrchestrator(store)

	// Create test memory with high importance
	mem := createTestMemory(t, store, "Important Go programming guide", 0.9, []string{"go"})

	opts := engine.SearchOptions{
		Query:  "Go programming",
		Limit:  10,
		Offset: 0,
	}

	results, err := orchestrator.Search(context.Background(), opts)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected at least one result")
	}

	// Find our test memory in results
	var testResult *engine.SearchResult
	for i := range results {
		if results[i].Memory.ID == mem.ID {
			testResult = &results[i]
			break
		}
	}

	if testResult == nil {
		t.Fatal("Test memory not found in results")
	}

	// Verify score components are calculated
	if testResult.Score <= 0 {
		t.Errorf("Expected positive score, got %f", testResult.Score)
	}

	// Verify score components exist
	if testResult.Components.Importance == 0 {
		t.Error("Expected non-zero importance component")
	}

	if testResult.Reason == "" {
		t.Error("Expected non-empty reason")
	}
}

// TestInferenceEngine_DirectConnections tests direct connection inference
func TestInferenceEngine_DirectConnections(t *testing.T) {
	t.Skip("Skipping until storage layer supports Entities field persistence")

	store := createTestMemoryStore(t)
	defer store.Close()

	inference := engine.NewInferenceEngine(store)

	// Create memories with explicit entities
	mem1 := createTestMemory(t, store, "Alice works on Project X", 0.8, []string{"alice", "project-x"})
	_ = createTestMemory(t, store, "Bob also works on Project X", 0.7, []string{"bob", "project-x"})

	opts := engine.InferenceOptions{
		MaxDepth:      2,
		MaxResults:    10,
		ConfidenceMin: 0.1,
		IncludeReason: true,
	}

	// Find connections from mem1
	results, err := inference.InferConnections(context.Background(), mem1.ID, opts)
	if err != nil {
		t.Fatalf("InferConnections failed: %v", err)
	}

	// Should find connection to mem2 via shared tag
	if len(results) == 0 {
		t.Error("Expected at least one inferred connection")
	}

	// Verify reasoning is included
	for _, result := range results {
		if result.Confidence <= 0 {
			t.Errorf("Expected positive confidence, got %f", result.Confidence)
		}
		if opts.IncludeReason && len(result.Reasoning) == 0 {
			t.Error("Expected reasoning to be included")
		}
	}
}

// TestInferenceEngine_TransitiveConnections tests multi-hop inference
func TestInferenceEngine_TransitiveConnections(t *testing.T) {
	t.Skip("Skipping until storage layer supports Entities field persistence")

	store := createTestMemoryStore(t)
	defer store.Close()

	inference := engine.NewInferenceEngine(store)

	// Create chain: A -> B -> C
	mem1 := createTestMemory(t, store, "Alice mentors Bob", 0.8, []string{"alice", "bob"})
	_ = createTestMemory(t, store, "Bob mentors Charlie", 0.7, []string{"bob", "charlie"})

	opts := engine.InferenceOptions{
		MaxDepth:      3,
		MaxResults:    10,
		ConfidenceMin: 0.1,
		IncludeReason: true,
	}

	// Find transitive connections from mem1
	results, err := inference.InferConnections(context.Background(), mem1.ID, opts)
	if err != nil {
		t.Fatalf("InferConnections failed: %v", err)
	}

	// Should find connection via shared tag "bob"
	if len(results) == 0 {
		t.Error("Expected at least one inferred connection")
	}

	// Verify path structure
	for _, result := range results {
		if len(result.Path) < 2 {
			t.Errorf("Expected path with at least 2 nodes, got %d", len(result.Path))
		}
		if result.Type == "" {
			t.Error("Expected non-empty connection type")
		}
	}
}

// TestDecayManager_ExponentialDecay tests exponential decay calculation
func TestDecayManager_ExponentialDecay(t *testing.T) {
	store := createTestMemoryStore(t)
	defer store.Close()

	// DecayManager now takes only halfLifeHours in hours (30 days = 720 hours)
	manager := engine.NewDecayManagerWithHalfLife(720) // 30 days

	// Create old memory
	now := time.Now()
	mem := &types.Memory{
		ID:         engine.GenerateMemoryID("test", ""),
		Content:    "Old memory",
		Importance: 0.8,
		CreatedAt:  now.Add(-60 * 24 * time.Hour), // 60 days ago
		UpdatedAt:  now.Add(-60 * 24 * time.Hour),
		Status:     types.StatusEnriched,
	}

	// Calculate decay with current time
	decayFactor := manager.CalculateDecay(mem, now)

	// After 60 days (2 half-lives), decay factor should be ~0.25 (0.5^2)
	expectedDecayFactor := 0.25
	tolerance := 0.05

	if decayFactor < expectedDecayFactor-tolerance || decayFactor > expectedDecayFactor+tolerance {
		t.Errorf("Expected decay factor ~%f, got %f", expectedDecayFactor, decayFactor)
	}
}

// TestDecayManager_UsageBoost tests decay calculation with updated access time
func TestDecayManager_UsageBoost(t *testing.T) {
	store := createTestMemoryStore(t)
	defer store.Close()

	manager := engine.NewDecayManagerWithHalfLife(720) // 30 days

	// Create memory
	now := time.Now()
	mem := createTestMemory(t, store, "Test memory", 0.5, []string{})

	// Recently accessed memory should have higher decay factor
	recentTime := now.Add(-1 * time.Hour)
	mem.LastAccessedAt = &recentTime

	// Calculate decay for recently accessed memory
	recentDecay := manager.CalculateDecay(mem, now)

	// Create another memory accessed long ago
	oldTime := now.Add(-100 * 24 * time.Hour)
	mem2 := createTestMemory(t, store, "Old memory", 0.5, []string{})
	mem2.LastAccessedAt = &oldTime

	oldDecay := manager.CalculateDecay(mem2, now)

	// Recently accessed should have higher decay factor
	if recentDecay <= oldDecay {
		t.Errorf("Expected recent decay (%f) > old decay (%f)", recentDecay, oldDecay)
	}
}

// TestDecayManager_VeryOldMemory tests decay with very old memory
func TestDecayManager_VeryOldMemory(t *testing.T) {
	store := createTestMemoryStore(t)
	defer store.Close()

	// Very short half-life for testing (1 hour)
	manager := engine.NewDecayManagerWithHalfLife(1.0)

	// Create old memory (not too old to avoid underflow to 0)
	now := time.Now()
	mem := &types.Memory{
		ID:         engine.GenerateMemoryID("test", ""),
		Content:    "Old memory",
		Importance: 0.8,
		CreatedAt:  now.Add(-100 * time.Hour), // 100 hours ago
		UpdatedAt:  now.Add(-100 * time.Hour),
		Status:     types.StatusEnriched,
	}

	decayFactor := manager.CalculateDecay(mem, now)

	// After 100 hours with 1-hour half-life, decay should be very close to 0
	// But still positive due to exponential decay: exp(-ln(2) * 100) ≈ 7.89e-31
	// However, with floating point limits, this may appear as 0
	// So we just verify it's non-negative and less than 0.1
	if decayFactor < 0 {
		t.Errorf("Expected non-negative decay factor, got %f", decayFactor)
	}

	if decayFactor >= 0.1 {
		t.Errorf("Expected very small decay factor for old memory, got %f", decayFactor)
	}
}

// TestDecayManager_ApplyDecay tests applying decay to a memory
func TestDecayManager_ApplyDecay(t *testing.T) {
	store := createTestMemoryStore(t)
	defer store.Close()

	manager := engine.NewDecayManagerWithHalfLife(30 * 24) // 30 days in hours

	now := time.Now()

	// Create old memory
	mem := &types.Memory{
		ID:         engine.GenerateMemoryID("test", ""),
		Content:    "Test memory",
		Importance: 0.8,
		CreatedAt:  now.Add(-60 * 24 * time.Hour), // 60 days ago
		UpdatedAt:  now.Add(-60 * 24 * time.Hour),
		Status:     types.StatusEnriched,
		DecayScore: 0.5, // Initial score
	}

	// Apply decay
	manager.ApplyDecay(mem, now)

	// Verify decay score was updated
	if mem.DecayScore <= 0 {
		t.Errorf("Expected positive decay score after ApplyDecay, got %f", mem.DecayScore)
	}

	// For 60 days (2 half-lives) with importance 0.8:
	// decayFactor = 0.5^2 = 0.25
	// score = (0.8 + 0.25) / 2.0 = 0.525
	expectedScore := (0.8 + 0.25) / 2.0
	tolerance := 0.01

	if mem.DecayScore < expectedScore-tolerance || mem.DecayScore > expectedScore+tolerance {
		t.Errorf("Expected decay score ~%f, got %f", expectedScore, mem.DecayScore)
	}

	// Verify DecayUpdatedAt was set
	if mem.DecayUpdatedAt == nil || mem.DecayUpdatedAt.IsZero() {
		t.Error("Expected DecayUpdatedAt to be set")
	}
}

// TestConfidenceScorer_MultiFactorScore tests multi-factor confidence calculation
func TestConfidenceScorer_MultiFactorScore(t *testing.T) {
	store := createTestMemoryStore(t)
	defer store.Close()

	scorer := engine.NewConfidenceScorer(store)

	// Create memory with various enrichment statuses
	mem := &types.Memory{
		ID:                 engine.GenerateMemoryID("test", ""),
		Content:            "Test memory content",
		Source:             "manual",
		EntityStatus:       types.EnrichmentCompleted,
		RelationshipStatus: types.EnrichmentCompleted,
		EmbeddingStatus:    types.EnrichmentCompleted,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		Status:             types.StatusEnriched,
		Entities:           []string{"ent:person:alice"},
	}

	store.Store(context.Background(), mem)

	// Calculate confidence
	confidence, err := scorer.CalculateMemoryConfidence(context.Background(), mem)
	if err != nil {
		t.Fatalf("CalculateMemoryConfidence failed: %v", err)
	}

	// Verify overall confidence is calculated
	if confidence.Overall <= 0 || confidence.Overall > 1 {
		t.Errorf("Expected overall confidence in (0,1], got %f", confidence.Overall)
	}

	// Verify individual components
	if confidence.EntityScore <= 0 {
		t.Error("Expected positive entity score")
	}

	if confidence.RelScore <= 0 {
		t.Error("Expected positive relationship score")
	}

	if confidence.SourceScore <= 0 {
		t.Error("Expected positive source score")
	}

	if confidence.AgeScore <= 0 {
		t.Error("Expected positive age score")
	}
}

// TestConfidenceScorer_SourceReliability tests source-based scoring
func TestConfidenceScorer_SourceReliability(t *testing.T) {
	store := createTestMemoryStore(t)
	defer store.Close()

	scorer := engine.NewConfidenceScorer(store)

	tests := []struct {
		name           string
		source         string
		expectedHigher bool // True if this source should score higher
	}{
		{
			name:           "Manual source",
			source:         "manual",
			expectedHigher: true,
		},
		{
			name:           "Auto source",
			source:         "auto",
			expectedHigher: false,
		},
	}

	var scores []float64

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := &types.Memory{
				ID:                 engine.GenerateMemoryID("test", ""),
				Content:            "Test memory",
				Source:             tt.source,
				EntityStatus:       types.EnrichmentCompleted,
				RelationshipStatus: types.EnrichmentCompleted,
				EmbeddingStatus:    types.EnrichmentCompleted,
				CreatedAt:          time.Now(),
				UpdatedAt:          time.Now(),
				Status:             types.StatusEnriched,
			}

			store.Store(context.Background(), mem)

			confidence, err := scorer.CalculateMemoryConfidence(context.Background(), mem)
			if err != nil {
				t.Fatalf("CalculateMemoryConfidence failed: %v", err)
			}

			scores = append(scores, confidence.SourceScore)

			if confidence.SourceScore <= 0 || confidence.SourceScore > 1 {
				t.Errorf("Expected source score in (0,1], got %f", confidence.SourceScore)
			}
		})
	}

	// Manual should score higher than auto
	if len(scores) == 2 && scores[0] <= scores[1] {
		t.Error("Expected manual source to score higher than auto")
	}
}

// TestConfidenceScorer_UpdateConfidence tests confidence update
func TestConfidenceScorer_UpdateConfidence(t *testing.T) {
	store := createTestMemoryStore(t)
	defer store.Close()

	scorer := engine.NewConfidenceScorer(store)

	mem := createTestMemory(t, store, "Test memory", 0.5, []string{})

	// Update confidence
	err := scorer.UpdateConfidence(context.Background(), mem.ID)
	if err != nil {
		t.Fatalf("UpdateConfidence failed: %v", err)
	}

	// Retrieve and verify confidence was updated
	updated, err := store.Get(context.Background(), mem.ID)
	if err != nil {
		t.Fatalf("Failed to get updated memory: %v", err)
	}

	// Verify metadata contains confidence score
	if updated.Metadata == nil {
		t.Fatal("Expected metadata to be populated")
	}

	confidence, ok := updated.Metadata["confidence"]
	if !ok {
		t.Error("Expected confidence in metadata")
	}

	if confidenceVal, ok := confidence.(float64); !ok || confidenceVal <= 0 {
		t.Errorf("Expected positive confidence value, got %v", confidence)
	}
}
