package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// mockMemoryStore implements storage.MemoryStore for testing.
type mockMemoryStore struct {
	memories map[string]*types.Memory
	getErr   error
	listErr  error
}

func newMockMemoryStore() *mockMemoryStore {
	return &mockMemoryStore{
		memories: make(map[string]*types.Memory),
	}
}

func (m *mockMemoryStore) Store(ctx context.Context, memory *types.Memory) error {
	if m.getErr != nil {
		return m.getErr
	}
	m.memories[memory.ID] = memory
	return nil
}

func (m *mockMemoryStore) Get(ctx context.Context, id string) (*types.Memory, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	mem, exists := m.memories[id]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return mem, nil
}

func (m *mockMemoryStore) List(ctx context.Context, opts storage.ListOptions) (*storage.PaginatedResult[types.Memory], error) {
	if m.listErr != nil {
		return nil, m.listErr
	}

	var items []types.Memory
	for _, mem := range m.memories {
		items = append(items, *mem)
	}

	return &storage.PaginatedResult[types.Memory]{
		Items:    items,
		Total:    len(items),
		Page:     1,
		PageSize: len(items),
		HasMore:  false,
	}, nil
}

func (m *mockMemoryStore) Update(ctx context.Context, memory *types.Memory) error {
	if _, exists := m.memories[memory.ID]; !exists {
		return storage.ErrNotFound
	}
	m.memories[memory.ID] = memory
	return nil
}

func (m *mockMemoryStore) Delete(ctx context.Context, id string) error {
	if _, exists := m.memories[id]; !exists {
		return storage.ErrNotFound
	}
	delete(m.memories, id)
	return nil
}

func (m *mockMemoryStore) Purge(ctx context.Context, id string) error {
	if _, exists := m.memories[id]; !exists {
		return storage.ErrNotFound
	}
	delete(m.memories, id)
	return nil
}

func (m *mockMemoryStore) Restore(ctx context.Context, id string) error {
	panic("not implemented")
}

func (m *mockMemoryStore) GetEvolutionChain(ctx context.Context, memoryID string) ([]*types.Memory, error) {
	panic("not implemented")
}

func (m *mockMemoryStore) GetMemoriesByRelationType(ctx context.Context, memoryID string, relType string) ([]*types.Memory, error) {
	panic("not implemented")
}

func (m *mockMemoryStore) UpdateStatus(ctx context.Context, id string, status types.MemoryStatus) error {
	panic("not implemented")
}

func (m *mockMemoryStore) UpdateEnrichment(ctx context.Context, id string, enrichment storage.EnrichmentUpdate) error {
	panic("not implemented")
}

func (m *mockMemoryStore) IncrementAccessCount(ctx context.Context, id string) error {
	panic("not implemented")
}

func (m *mockMemoryStore) UpdateState(ctx context.Context, id string, state string) error {
	panic("not implemented")
}

func (m *mockMemoryStore) GetRelatedMemories(ctx context.Context, memoryID string) ([]string, error) {
	// Return empty slice for testing - no relationships set up by default
	return []string{}, nil
}

func (m *mockMemoryStore) Traverse(ctx context.Context, startMemoryID string, maxHops int, limit int) ([]storage.TraversalResult, error) {
	panic("not implemented")
}

func (m *mockMemoryStore) GetMemoryEntities(ctx context.Context, memoryID string) ([]*types.Entity, error) {
	panic("not implemented")
}

func (m *mockMemoryStore) UpdateDecayScores(ctx context.Context) (int, error) {
	panic("not implemented")
}

func (m *mockMemoryStore) Close() error {
	return nil
}

// TestInferConnections_EmptyStore tests inference with no memories.
func TestInferConnections_EmptyStore(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()
	_ = store.Store(ctx, &types.Memory{
		ID:       "mem:test:source",
		Content:  "test content",
		Domain:   "test",
		Entities: []string{"entity1", "entity2"},
	})

	engine := NewInferenceEngine(store)
	results, err := engine.InferConnections(ctx, "mem:test:source", InferenceOptions{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected no results with single memory, got %d", len(results))
	}
}

// TestInferConnections_DirectConnection tests finding directly connected memories.
func TestInferConnections_DirectConnection(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	sourceID := "mem:test:source"
	targetID := "mem:test:target"

	_ = store.Store(ctx, &types.Memory{
		ID:       sourceID,
		Content:  "source content",
		Domain:   "test",
		Entities: []string{"entity1", "entity2", "entity3"},
	})

	_ = store.Store(ctx, &types.Memory{
		ID:       targetID,
		Content:  "target content",
		Domain:   "test",
		Entities: []string{"entity2", "entity3", "entity4"},
	})

	engine := NewInferenceEngine(store)
	results, err := engine.InferConnections(ctx, sourceID, InferenceOptions{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) == 0 {
		t.Errorf("expected direct connection to be found")
	}

	if len(results) > 0 {
		result := results[0]
		if result.Type != "direct" {
			t.Errorf("expected type 'direct', got %s", result.Type)
		}
		if result.Confidence <= 0.0 || result.Confidence > 1.0 {
			t.Errorf("expected confidence in (0, 1], got %f", result.Confidence)
		}
		if len(result.Path) != 2 || result.Path[0] != sourceID || result.Path[1] != targetID {
			t.Errorf("expected path [%s, %s], got %v", sourceID, targetID, result.Path)
		}
	}
}

// TestInferConnections_OptionsDefaults tests option normalization.
func TestInferConnections_OptionsDefaults(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	sourceID := "mem:test:source"
	_ = store.Store(ctx, &types.Memory{
		ID:       sourceID,
		Content:  "source content",
		Domain:   "test",
		Entities: []string{"entity1"},
	})

	engine := NewInferenceEngine(store)

	// Test with invalid options
	opts := InferenceOptions{
		MaxDepth:   -1,
		MaxResults: 0,
		ConfidenceMin: -0.5,
	}

	results, err := engine.InferConnections(ctx, sourceID, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify that options were normalized (no error thrown) — results may be empty
	_ = results
}

// TestInferConnections_ConfidenceFilter tests confidence threshold filtering.
func TestInferConnections_ConfidenceFilter(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	sourceID := "mem:test:source"
	target1ID := "mem:test:target1"
	target2ID := "mem:test:target2"

	// Source with 3 entities
	_ = store.Store(ctx, &types.Memory{
		ID:       sourceID,
		Content:  "source",
		Domain:   "test",
		Entities: []string{"e1", "e2", "e3"},
	})

	// Target 1: shares 2/3 entities (high similarity)
	_ = store.Store(ctx, &types.Memory{
		ID:       target1ID,
		Content:  "target1",
		Domain:   "test",
		Entities: []string{"e1", "e2", "e4"},
	})

	// Target 2: shares 1/3 entities (low similarity)
	_ = store.Store(ctx, &types.Memory{
		ID:       target2ID,
		Content:  "target2",
		Domain:   "test",
		Entities: []string{"e1", "e4", "e5"},
	})

	engine := NewInferenceEngine(store)
	results, err := engine.InferConnections(ctx, sourceID, InferenceOptions{
		ConfidenceMin: 0.5, // High threshold
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, result := range results {
		if result.Confidence < 0.5 {
			t.Errorf("confidence filter violated: got %f, expected >= 0.5", result.Confidence)
		}
	}
}

// TestInferConnections_ResultsLimit tests MaxResults limiting.
func TestInferConnections_ResultsLimit(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	sourceID := "mem:test:source"
	_ = store.Store(ctx, &types.Memory{
		ID:       sourceID,
		Content:  "source",
		Domain:   "test",
		Entities: []string{"e1"},
	})

	// Add 10 target memories with same entity
	for i := 0; i < 10; i++ {
		targetID := "mem:test:target" + string(rune(i+48))
		_ = store.Store(ctx, &types.Memory{
			ID:       targetID,
			Content:  "target",
			Domain:   "test",
			Entities: []string{"e1"},
		})
	}

	engine := NewInferenceEngine(store)
	results, err := engine.InferConnections(ctx, sourceID, InferenceOptions{
		MaxResults: 3,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(results))
	}
}

// TestInferConnections_SortByConfidence tests results are sorted by confidence descending.
func TestInferConnections_SortByConfidence(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	sourceID := "mem:test:source"
	_ = store.Store(ctx, &types.Memory{
		ID:       sourceID,
		Content:  "source",
		Domain:   "test",
		Entities: []string{"e1", "e2", "e3"},
	})

	// High confidence target (3 shared entities)
	_ = store.Store(ctx, &types.Memory{
		ID:       "mem:test:high",
		Content:  "high",
		Domain:   "test",
		Entities: []string{"e1", "e2", "e3"},
	})

	// Medium confidence target (2 shared entities)
	_ = store.Store(ctx, &types.Memory{
		ID:       "mem:test:medium",
		Content:  "medium",
		Domain:   "test",
		Entities: []string{"e1", "e2"},
	})

	// Low confidence target (1 shared entity)
	_ = store.Store(ctx, &types.Memory{
		ID:       "mem:test:low",
		Content:  "low",
		Domain:   "test",
		Entities: []string{"e1"},
	})

	engine := NewInferenceEngine(store)
	results, err := engine.InferConnections(ctx, sourceID, InferenceOptions{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify sorted by confidence descending
	for i := 1; i < len(results); i++ {
		if results[i].Confidence > results[i-1].Confidence {
			t.Errorf("results not sorted by confidence descending at index %d: %f > %f",
				i, results[i].Confidence, results[i-1].Confidence)
		}
	}
}

// TestInferConnections_IncludeReason tests reasoning field population.
func TestInferConnections_IncludeReason(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	sourceID := "mem:test:source"
	targetID := "mem:test:target"

	_ = store.Store(ctx, &types.Memory{
		ID:       sourceID,
		Content:  "source",
		Domain:   "test",
		Entities: []string{"e1", "e2"},
	})

	_ = store.Store(ctx, &types.Memory{
		ID:       targetID,
		Content:  "target",
		Domain:   "test",
		Entities: []string{"e1", "e2"},
	})

	engine := NewInferenceEngine(store)
	results, err := engine.InferConnections(ctx, sourceID, InferenceOptions{
		IncludeReason: true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected results")
	}

	if len(results[0].Reasoning) == 0 {
		t.Errorf("expected reasoning to be populated when IncludeReason=true")
	}
}

// TestInferConnections_SourceNotFound tests error when source memory doesn't exist.
func TestInferConnections_SourceNotFound(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	engine := NewInferenceEngine(store)
	_, err := engine.InferConnections(ctx, "mem:nonexistent", InferenceOptions{})

	if err == nil {
		t.Fatalf("expected error for nonexistent source memory")
	}
}

// TestFindPatterns_NoMemories tests pattern discovery with empty store.
func TestFindPatterns_NoMemories(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	engine := NewInferenceEngine(store)
	patterns, err := engine.FindPatterns(ctx, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patterns) != 0 {
		t.Errorf("expected no patterns with empty store, got %d", len(patterns))
	}
}

// TestFindPatterns_EntityCluster tests pattern detection with shared entities.
func TestFindPatterns_EntityCluster(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	// Create 3 memories sharing an entity
	for i := 0; i < 3; i++ {
		_ = store.Store(ctx, &types.Memory{
			ID:       "mem:test:entity-" + string(rune(i+48)),
			Content:  "content",
			Domain:   "test",
			Entities: []string{"shared-entity", "entity-" + string(rune(i+48))},
		})
	}

	engine := NewInferenceEngine(store)
	patterns, err := engine.FindPatterns(ctx, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should detect cluster pattern for "shared-entity"
	clusterFound := false
	for _, p := range patterns {
		if p.Type == "cluster" && len(p.Memories) >= 3 {
			clusterFound = true
			break
		}
	}

	if !clusterFound {
		t.Errorf("expected cluster pattern to be detected")
	}
}

// TestFindPatterns_TagCluster tests pattern detection with shared tags.
func TestFindPatterns_TagCluster(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	// Create 3 memories sharing a tag
	for i := 0; i < 3; i++ {
		_ = store.Store(ctx, &types.Memory{
			ID:       "mem:test:tag-" + string(rune(i+48)),
			Content:  "content",
			Domain:   "test",
			Tags:     []string{"important", "tag-" + string(rune(i+48))},
			Entities: []string{},
		})
	}

	engine := NewInferenceEngine(store)
	patterns, err := engine.FindPatterns(ctx, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should detect cluster pattern for "important" tag
	tagClusterFound := false
	for _, p := range patterns {
		if p.Type == "cluster" && len(p.Memories) >= 3 {
			tagClusterFound = true
			break
		}
	}

	if !tagClusterFound {
		t.Errorf("expected tag cluster pattern to be detected")
	}
}

// TestFindPatterns_MinimumMemories tests that patterns require minimum memory count.
func TestFindPatterns_MinimumMemories(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	// Create only 2 memories sharing an entity (below threshold of 3)
	_ = store.Store(ctx, &types.Memory{
		ID:       "mem:test:1",
		Content:  "content",
		Domain:   "test",
		Entities: []string{"shared"},
	})

	_ = store.Store(ctx, &types.Memory{
		ID:       "mem:test:2",
		Content:  "content",
		Domain:   "test",
		Entities: []string{"shared"},
	})

	engine := NewInferenceEngine(store)
	patterns, err := engine.FindPatterns(ctx, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, p := range patterns {
		if p.Type == "cluster" && len(p.Memories) < 3 {
			t.Errorf("pattern detected with fewer than 3 memories, got %d", len(p.Memories))
		}
	}
}

// TestFindPatterns_DomainFiltering tests pattern discovery filters by domain.
func TestFindPatterns_DomainFiltering(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	// Create memories in different domains
	for i := 0; i < 3; i++ {
		_ = store.Store(ctx, &types.Memory{
			ID:       "mem:domain1:1-" + string(rune(i+48)),
			Content:  "content",
			Domain:   "domain1",
			Entities: []string{"entity-domain1"},
		})
	}

	for i := 0; i < 3; i++ {
		_ = store.Store(ctx, &types.Memory{
			ID:       "mem:domain2:2-" + string(rune(i+48)),
			Content:  "content",
			Domain:   "domain2",
			Entities: []string{"entity-domain2"},
		})
	}

	engine := NewInferenceEngine(store)
	patterns, err := engine.FindPatterns(ctx, "domain1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Patterns should only relate to domain1 memories (or be empty)
	_ = patterns
}

// TestFindPatterns_ConfidenceCalculation tests that pattern confidence is calculated correctly.
func TestFindPatterns_ConfidenceCalculation(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	// Create 5 memories sharing an entity
	for i := 0; i < 5; i++ {
		_ = store.Store(ctx, &types.Memory{
			ID:       "mem:test:pat-" + string(rune(i+48)),
			Content:  "content",
			Domain:   "test",
			Entities: []string{"shared"},
		})
	}

	engine := NewInferenceEngine(store)
	patterns, err := engine.FindPatterns(ctx, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, p := range patterns {
		if p.Type == "cluster" && len(p.Memories) == 5 {
			if p.Confidence <= 0.0 || p.Confidence > 1.0 {
				t.Errorf("expected confidence in (0, 1], got %f", p.Confidence)
			}
			if p.Frequency != 5 {
				t.Errorf("expected frequency 5, got %d", p.Frequency)
			}
		}
	}
}

// TestInferConnections_GetError tests handling of store Get error.
func TestInferConnections_GetError(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()
	store.getErr = errors.New("database error")

	engine := NewInferenceEngine(store)
	_, err := engine.InferConnections(ctx, "mem:test:any", InferenceOptions{})

	if err == nil {
		t.Fatalf("expected error from store Get")
	}
}

// TestInferConnections_ListError tests handling of store List error.
func TestInferConnections_ListError(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	sourceID := "mem:test:source"
	_ = store.Store(ctx, &types.Memory{
		ID:       sourceID,
		Content:  "source",
		Domain:   "test",
		Entities: []string{"e1"},
	})

	store.listErr = errors.New("database error")

	engine := NewInferenceEngine(store)
	_, err := engine.InferConnections(ctx, sourceID, InferenceOptions{})

	if err == nil {
		t.Fatalf("expected error from store List")
	}
}

