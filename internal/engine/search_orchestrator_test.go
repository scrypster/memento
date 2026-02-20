package engine

import (
	"context"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// mockListStore implements only storage.MemoryStore interface
type mockListStore struct {
	memories map[string]*types.Memory
}

func (m *mockListStore) Store(ctx context.Context, memory *types.Memory) error {
	m.memories[memory.ID] = memory
	return nil
}

func (m *mockListStore) Get(ctx context.Context, id string) (*types.Memory, error) {
	if mem, ok := m.memories[id]; ok {
		return mem, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockListStore) List(ctx context.Context, opts storage.ListOptions) (*storage.PaginatedResult[types.Memory], error) {
	opts.Normalize()

	var items []types.Memory
	for _, mem := range m.memories {
		// Apply domain filter if specified
		if opts.Filter != nil {
			if domain, ok := opts.Filter["domain"].(string); ok && domain != "" {
				if mem.Domain != domain {
					continue
				}
			}
		}
		items = append(items, *mem)
	}

	// Sort by created_at descending (most recent first)
	if opts.SortBy == "created_at" && opts.SortOrder == "desc" {
		for i := 0; i < len(items)-1; i++ {
			for j := i + 1; j < len(items); j++ {
				if items[j].CreatedAt.After(items[i].CreatedAt) {
					items[i], items[j] = items[j], items[i]
				}
			}
		}
	}

	// Calculate pagination
	offset := opts.Offset()
	limit := opts.Limit
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}

	pageItems := items
	if offset < len(items) {
		pageItems = items[offset:end]
	} else {
		pageItems = []types.Memory{}
	}

	return &storage.PaginatedResult[types.Memory]{
		Items:    pageItems,
		Total:    len(items),
		Page:     opts.Page,
		PageSize: opts.Limit,
		HasMore:  end < len(items),
	}, nil
}

func (m *mockListStore) Update(ctx context.Context, memory *types.Memory) error {
	panic("not implemented")
}

func (m *mockListStore) Delete(ctx context.Context, id string) error {
	panic("not implemented")
}

func (m *mockListStore) Purge(ctx context.Context, id string) error {
	panic("not implemented")
}

func (m *mockListStore) Restore(ctx context.Context, id string) error {
	panic("not implemented")
}

func (m *mockListStore) GetEvolutionChain(ctx context.Context, memoryID string) ([]*types.Memory, error) {
	panic("not implemented")
}

func (m *mockListStore) GetMemoriesByRelationType(ctx context.Context, memoryID string, relType string) ([]*types.Memory, error) {
	panic("not implemented")
}

func (m *mockListStore) UpdateStatus(ctx context.Context, id string, status types.MemoryStatus) error {
	panic("not implemented")
}

func (m *mockListStore) UpdateEnrichment(ctx context.Context, id string, enrichment storage.EnrichmentUpdate) error {
	panic("not implemented")
}

func (m *mockListStore) IncrementAccessCount(ctx context.Context, id string) error {
	panic("not implemented")
}

func (m *mockListStore) UpdateState(ctx context.Context, id string, state string) error {
	panic("not implemented")
}

func (m *mockListStore) GetRelatedMemories(ctx context.Context, memoryID string) ([]string, error) {
	panic("not implemented")
}

func (m *mockListStore) Traverse(ctx context.Context, startMemoryID string, maxHops int, limit int) ([]storage.TraversalResult, error) {
	panic("not implemented")
}

func (m *mockListStore) GetMemoryEntities(ctx context.Context, memoryID string) ([]*types.Entity, error) {
	panic("not implemented")
}

func (m *mockListStore) UpdateDecayScores(ctx context.Context) (int, error) {
	panic("not implemented")
}

func (m *mockListStore) Close() error {
	panic("not implemented")
}

// mockSearchStore implements both storage.MemoryStore AND storage.SearchProvider
type mockSearchStore struct {
	*mockListStore
	ftsResults map[string]*storage.PaginatedResult[types.Memory]
}

func (m *mockSearchStore) FullTextSearch(ctx context.Context, opts storage.SearchOptions) (*storage.PaginatedResult[types.Memory], error) {
	if result, ok := m.ftsResults[opts.Query]; ok {
		return result, nil
	}
	return &storage.PaginatedResult[types.Memory]{
		Items: []types.Memory{},
		Total: 0,
	}, nil
}

func (m *mockSearchStore) VectorSearch(ctx context.Context, query []float64, opts storage.SearchOptions) (*storage.PaginatedResult[types.Memory], error) {
	panic("not implemented")
}

func (m *mockSearchStore) HybridSearch(ctx context.Context, text string, vector []float64, opts storage.SearchOptions) (*storage.PaginatedResult[types.Memory], error) {
	panic("not implemented")
}

// Helper function to create test memories
func newTestMemory(id, content, domain string) *types.Memory {
	now := time.Now()
	return &types.Memory{
		ID:        id,
		Content:   content,
		Domain:    domain,
		CreatedAt: now,
		UpdatedAt: now,
		Status:    types.StatusEnriched,
		Tags:      []string{},
		Entities:  []string{},
	}
}

// Helper function to create test memories with custom fields
func newTestMemoryWithFields(id, content, domain string, importance float64, status types.MemoryStatus, tags []string) *types.Memory {
	now := time.Now()
	return &types.Memory{
		ID:         id,
		Content:    content,
		Domain:     domain,
		CreatedAt:  now,
		UpdatedAt:  now,
		Status:     status,
		Importance: importance,
		Tags:       tags,
		Entities:   []string{},
	}
}

// Test: NewSearchOrchestrator with MemoryStore only (no SearchProvider)
func TestNewSearchOrchestrator_NoSearchProvider(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}
	orchestrator := NewSearchOrchestrator(store)

	if orchestrator.memoryStore == nil {
		t.Error("memoryStore should not be nil")
	}
	if orchestrator.searchProvider != nil {
		t.Error("searchProvider should be nil when store doesn't implement SearchProvider")
	}
}

// Test: NewSearchOrchestrator with both MemoryStore and SearchProvider
func TestNewSearchOrchestrator_WithSearchProvider(t *testing.T) {
	store := &mockSearchStore{
		mockListStore: &mockListStore{memories: make(map[string]*types.Memory)},
		ftsResults:    make(map[string]*storage.PaginatedResult[types.Memory]),
	}
	orchestrator := NewSearchOrchestrator(store)

	if orchestrator.memoryStore == nil {
		t.Error("memoryStore should not be nil")
	}
	if orchestrator.searchProvider == nil {
		t.Error("searchProvider should not be nil when store implements SearchProvider")
	}
}

// Test: Search with empty query uses List fallback
func TestSearch_EmptyQuery_UsesFallback(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}
	mem1 := newTestMemory("mem:test:1", "test content about golang", "test")
	mem2 := newTestMemory("mem:test:2", "more golang information", "test")
	store.memories[mem1.ID] = mem1
	store.memories[mem2.ID] = mem2

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "",
		Limit:  10,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("empty query should return all memories")
	}

	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

// Test: Search routes to SearchProvider.FullTextSearch when available
func TestSearch_WithQuery_UsesSearchProvider(t *testing.T) {
	store := &mockSearchStore{
		mockListStore: &mockListStore{memories: make(map[string]*types.Memory)},
		ftsResults:    make(map[string]*storage.PaginatedResult[types.Memory]),
	}

	mem1 := newTestMemory("mem:test:1", "golang concurrency patterns", "test")
	store.mockListStore.memories[mem1.ID] = mem1

	// Pre-configure FTS result for "golang" query
	store.ftsResults["golang"] = &storage.PaginatedResult[types.Memory]{
		Items:    []types.Memory{*mem1},
		Total:    1,
		Page:     1,
		PageSize: 10,
		HasMore:  false,
	}

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "golang",
		Limit:  10,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result from SearchProvider, got %d", len(results))
	}

	if results[0].Memory.ID != "mem:test:1" {
		t.Errorf("expected memory ID mem:test:1, got %s", results[0].Memory.ID)
	}
}

// Test: Search respects Limit
func TestSearch_RespectLimit(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	// Create 15 memories
	for i := 0; i < 15; i++ {
		id := "mem:test:" + string(rune(i+48))
		mem := newTestMemory(id, "test content", "test")
		store.memories[mem.ID] = mem
	}

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "",
		Limit:  5,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("expected 5 results with Limit=5, got %d", len(results))
	}
}

// Test: Search respects Offset (pagination)
func TestSearch_RespectOffset(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	// Create 10 memories with different creation times
	for i := 0; i < 10; i++ {
		id := "mem:test:" + string(rune(48+i))
		mem := newTestMemory(id, "test content", "test")
		// Stagger creation times
		mem.CreatedAt = time.Now().Add(-time.Duration(10-i) * time.Second)
		store.memories[mem.ID] = mem
	}

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	// Get first page (offset 0, limit 3)
	results1, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "",
		Limit:  3,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Get second page (offset 3, limit 3)
	results2, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "",
		Limit:  3,
		Offset: 3,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results1) != 3 || len(results2) != 3 {
		t.Errorf("expected 3 results per page, got %d and %d", len(results1), len(results2))
	}

	// Verify no overlap
	if results1[0].Memory.ID == results2[0].Memory.ID {
		t.Error("pagination should return different results")
	}
}

// Test: Search with MinScore filters low-scoring results
func TestSearch_MinScoreFilter(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	// Memory with exact match (high score)
	mem1 := newTestMemoryWithFields(
		"mem:test:1",
		"golang language programming",
		"test",
		0.9,
		types.StatusEnriched,
		[]string{},
	)

	// Memory with low relevance
	mem2 := newTestMemoryWithFields(
		"mem:test:2",
		"python and java programming",
		"test",
		0.1,
		types.StatusEnriched,
		[]string{},
	)

	store.memories[mem1.ID] = mem1
	store.memories[mem2.ID] = mem2

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	// Search with high MinScore threshold
	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:    "golang",
		Limit:    10,
		Offset:   0,
		MinScore: 0.5,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Should only get high-scoring result
	if len(results) == 0 {
		t.Error("should have returned results with high importance memory")
	}

	for _, result := range results {
		if result.Score < 0.5 {
			t.Errorf("result score %f is below MinScore threshold 0.5", result.Score)
		}
	}
}

// Test: Recent memories (StatusEnriched) score higher
func TestSearch_RecencyScoring(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	// Fresh enriched memory
	mem1 := newTestMemoryWithFields(
		"mem:test:1",
		"test topic",
		"test",
		0.5,
		types.StatusEnriched,
		[]string{},
	)

	// Old pending memory
	mem2 := newTestMemoryWithFields(
		"mem:test:2",
		"test topic",
		"test",
		0.5,
		types.StatusPending,
		[]string{},
	)
	mem2.CreatedAt = time.Now().Add(-30 * 24 * time.Hour)

	store.memories[mem1.ID] = mem1
	store.memories[mem2.ID] = mem2

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "test",
		Limit:  10,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 1 {
		t.Error("should have returned at least 1 result")
		return
	}

	// Fresh enriched should score higher than old pending
	if len(results) >= 2 {
		if results[0].Memory.ID != "mem:test:1" {
			t.Error("fresh enriched memory should rank higher")
		}
	}
}

// Test: Important memories score higher
func TestSearch_ImportanceScoring(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	// High importance
	mem1 := newTestMemoryWithFields(
		"mem:test:1",
		"test topic",
		"test",
		0.9,
		types.StatusEnriched,
		[]string{},
	)

	// Low importance
	mem2 := newTestMemoryWithFields(
		"mem:test:2",
		"test topic",
		"test",
		0.1,
		types.StatusEnriched,
		[]string{},
	)

	store.memories[mem1.ID] = mem1
	store.memories[mem2.ID] = mem2

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "test",
		Limit:  10,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 2 {
		t.Error("should have returned 2 results")
		return
	}

	// High importance should rank first
	if results[0].Memory.ID != "mem:test:1" {
		t.Error("high importance memory should rank first")
	}
	if results[1].Memory.ID != "mem:test:2" {
		t.Error("low importance memory should rank second")
	}

	// High importance should have higher score
	if results[0].Score <= results[1].Score {
		t.Errorf("high importance score %f should be > low importance score %f",
			results[0].Score, results[1].Score)
	}
}

// Test: Empty store returns empty results (not error)
func TestSearch_EmptyStore(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}
	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "anything",
		Limit:  10,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search should not error on empty store, got: %v", err)
	}

	if results == nil {
		t.Error("Search should return empty slice, not nil")
	}

	if len(results) != 0 {
		t.Errorf("empty store should return 0 results, got %d", len(results))
	}
}

// Test: Domain filter is passed through (fallback path)
func TestSearch_DomainFilter_Fallback(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	mem1 := newTestMemory("mem:test:1", "test content", "nps-aid")
	mem2 := newTestMemory("mem:test:2", "test content", "ops")
	mem3 := newTestMemory("mem:test:3", "test content", "nps-aid")

	store.memories[mem1.ID] = mem1
	store.memories[mem2.ID] = mem2
	store.memories[mem3.ID] = mem3

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "",
		Domain: "nps-aid",
		Limit:  10,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("domain filter should return 2 nps-aid memories, got %d", len(results))
	}

	for _, result := range results {
		if result.Memory.Domain != "nps-aid" {
			t.Errorf("domain filter returned wrong domain: %s", result.Memory.Domain)
		}
	}
}

// Test: Domain filter is applied in SearchProvider path
func TestSearch_DomainFilter_SearchProvider(t *testing.T) {
	store := &mockSearchStore{
		mockListStore: &mockListStore{memories: make(map[string]*types.Memory)},
		ftsResults:    make(map[string]*storage.PaginatedResult[types.Memory]),
	}

	mem1 := newTestMemory("mem:test:1", "golang language", "nps-aid")
	mem2 := newTestMemory("mem:test:2", "golang programming", "ops")
	mem3 := newTestMemory("mem:test:3", "golang guide", "nps-aid")

	store.mockListStore.memories[mem1.ID] = mem1
	store.mockListStore.memories[mem2.ID] = mem2
	store.mockListStore.memories[mem3.ID] = mem3

	// FTS returns all golang matches
	store.ftsResults["golang"] = &storage.PaginatedResult[types.Memory]{
		Items:    []types.Memory{*mem1, *mem2, *mem3},
		Total:    3,
		Page:     1,
		PageSize: 10,
		HasMore:  false,
	}

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "golang",
		Domain: "nps-aid",
		Limit:  10,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("domain filter should return 2 nps-aid memories, got %d", len(results))
	}

	for _, result := range results {
		if result.Memory.Domain != "nps-aid" {
			t.Errorf("domain filter returned wrong domain: %s", result.Memory.Domain)
		}
	}
}

// Test: Limit defaults to 10 if not specified
func TestSearch_DefaultLimit(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	// Create 20 memories
	for i := 0; i < 20; i++ {
		id := "mem:test:" + string(rune(48+i%10))
		mem := newTestMemory(id+string(rune(48+i/10)), "test content", "test")
		store.memories[mem.ID] = mem
	}

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "",
		Limit:  0,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 10 {
		t.Errorf("default limit should be 10, got %d results", len(results))
	}
}

// Test: Limit caps at 100
func TestSearch_LimitCap(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	// Create 5 memories
	for i := 0; i < 5; i++ {
		id := "mem:test:" + string(rune(48+i))
		mem := newTestMemory(id, "test content", "test")
		store.memories[mem.ID] = mem
	}

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "",
		Limit:  200,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Should only get 5 (all available)
	if len(results) != 5 {
		t.Errorf("expected 5 results (all available), got %d", len(results))
	}
}

// Test: Offset defaults to 0 if negative
func TestSearch_NegativeOffsetDefault(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	mem := newTestMemory("mem:test:1", "test content", "test")
	store.memories[mem.ID] = mem

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "",
		Limit:  10,
		Offset: -5,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Error("negative offset should default to 0")
	}
}

// Test: Text match scoring
func TestSearch_TextMatchScoring(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	// Exact phrase match
	mem1 := newTestMemory("mem:test:1", "golang concurrency patterns are important", "test")

	// Partial word match
	mem2 := newTestMemory("mem:test:2", "python and java patterns", "test")

	// No match
	mem3 := newTestMemory("mem:test:3", "unrelated content here", "test")

	store.memories[mem1.ID] = mem1
	store.memories[mem2.ID] = mem2
	store.memories[mem3.ID] = mem3

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "concurrency patterns",
		Limit:  10,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 1 {
		t.Error("should find memory with matching content")
		return
	}

	// Results should be sorted by score (highest first)
	if results[0].Components.TextMatch == 0 {
		t.Error("text match score should be > 0 for matching content")
	}
}

// Test: Tag matching boosts score
func TestSearch_TagMatchScoring(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	mem := newTestMemoryWithFields(
		"mem:test:1",
		"some other content",
		"test",
		0.5,
		types.StatusEnriched,
		[]string{"golang", "architecture"},
	)

	store.memories[mem.ID] = mem

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "golang",
		Limit:  10,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Error("should find memory with matching tag")
		return
	}

	if results[0].Components.TextMatch == 0 && results[0].Score > 0 {
		t.Error("tag match should boost score when content doesn't match")
	}
}

// Test: SearchResult includes Score and Components
func TestSearch_ResultStructure(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	mem := newTestMemory("mem:test:1", "golang programming", "test")
	store.memories[mem.ID] = mem

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.Search(ctx, SearchOptions{
		Query:  "golang",
		Limit:  10,
		Offset: 0,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Error("should have 1 result")
		return
	}

	result := results[0]

	// Check all required fields exist
	if result.Memory == nil {
		t.Error("SearchResult.Memory should not be nil")
	}
	if result.Score == 0 && result.Memory != nil {
		t.Error("SearchResult.Score should be calculated")
	}
	if result.Reason == "" {
		t.Error("SearchResult.Reason should not be empty")
	}

	// Check Components
	comp := result.Components
	if comp.TextMatch == 0 && comp.Recency == 0 && comp.Importance == 0 {
		t.Error("ScoreComponents should have non-zero values")
	}
}

// Test: SearchSimilar removes source memory from results
func TestSearchSimilar_RemovesSourceMemory(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	mem1 := newTestMemoryWithFields(
		"mem:test:1",
		"golang concurrency",
		"test",
		0.5,
		types.StatusEnriched,
		[]string{"golang", "concurrency"},
	)
	mem1.Entities = []string{"entity:golang", "entity:concurrency"}

	mem2 := newTestMemoryWithFields(
		"mem:test:2",
		"golang patterns",
		"test",
		0.5,
		types.StatusEnriched,
		[]string{"golang"},
	)
	mem2.Entities = []string{"entity:golang"}

	store.memories[mem1.ID] = mem1
	store.memories[mem2.ID] = mem2

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.SearchSimilar(ctx, "mem:test:1", 10)

	if err != nil {
		t.Fatalf("SearchSimilar failed: %v", err)
	}

	// Should not include the source memory
	for _, result := range results {
		if result.Memory.ID == "mem:test:1" {
			t.Error("SearchSimilar should not return the source memory")
		}
	}
}

// Test: SearchSimilar respects limit
func TestSearchSimilar_RespectsLimit(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	mem1 := newTestMemoryWithFields(
		"mem:test:1",
		"golang",
		"test",
		0.5,
		types.StatusEnriched,
		[]string{"golang"},
	)
	mem1.Entities = []string{"entity:golang"}

	// Create 10 similar memories
	for i := 2; i <= 11; i++ {
		id := "mem:test:" + string(rune(48+i%10))
		mem := newTestMemoryWithFields(
			id,
			"golang patterns",
			"test",
			0.5,
			types.StatusEnriched,
			[]string{"golang"},
		)
		mem.Entities = []string{"entity:golang"}
		store.memories[mem.ID] = mem
	}

	store.memories[mem1.ID] = mem1

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.SearchSimilar(ctx, "mem:test:1", 5)

	if err != nil {
		t.Fatalf("SearchSimilar failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("SearchSimilar with limit 5 should return 5 results, got %d", len(results))
	}
}

// Test: SearchSimilar defaults limit to 10
func TestSearchSimilar_DefaultLimit(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	mem1 := newTestMemoryWithFields(
		"mem:test:1",
		"golang",
		"test",
		0.5,
		types.StatusEnriched,
		[]string{"golang"},
	)
	mem1.Entities = []string{"entity:golang"}

	store.memories[mem1.ID] = mem1

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.SearchSimilar(ctx, "mem:test:1", 0)

	if err != nil {
		t.Fatalf("SearchSimilar failed: %v", err)
	}

	// Should use default limit of 10 (minus source memory)
	// Since we only have 1 memory total, should get 0
	if len(results) != 0 {
		t.Errorf("with only source memory, should get 0 similar, got %d", len(results))
	}
}

// Test: SearchSimilar caps limit at 100
func TestSearchSimilar_LimitCap(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	mem1 := newTestMemoryWithFields(
		"mem:test:1",
		"golang",
		"test",
		0.5,
		types.StatusEnriched,
		[]string{"golang"},
	)
	mem1.Entities = []string{"entity:golang"}

	store.memories[mem1.ID] = mem1

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.SearchSimilar(ctx, "mem:test:1", 500)

	if err != nil {
		t.Fatalf("SearchSimilar failed: %v", err)
	}

	// Limit should cap at 100, minus the source memory, = 99 max
	// But we only have 1 memory, so should get 0
	if len(results) != 0 {
		t.Errorf("with only source memory, should get 0 similar, got %d", len(results))
	}
}

// Test: SearchSimilar uses entity names from entities field
func TestSearchSimilar_UsesEntities(t *testing.T) {
	store := &mockListStore{memories: make(map[string]*types.Memory)}

	mem1 := newTestMemoryWithFields(
		"mem:test:1",
		"some content",
		"test",
		0.5,
		types.StatusEnriched,
		[]string{},
	)
	mem1.Entities = []string{"person:john", "org:acme", "project:memento"}

	// Memory with same entity
	mem2 := newTestMemoryWithFields(
		"mem:test:2",
		"related content",
		"test",
		0.5,
		types.StatusEnriched,
		[]string{},
	)
	mem2.Entities = []string{"person:john", "project:other"}

	store.memories[mem1.ID] = mem1
	store.memories[mem2.ID] = mem2

	orchestrator := NewSearchOrchestrator(store)
	ctx := context.Background()

	results, err := orchestrator.SearchSimilar(ctx, "mem:test:1", 10)

	if err != nil {
		t.Fatalf("SearchSimilar failed: %v", err)
	}

	// Should find mem2 because it shares entity "person:john"
	found := false
	for _, result := range results {
		if result.Memory.ID == "mem:test:2" {
			found = true
			break
		}
	}

	if len(results) > 0 && !found {
		t.Error("SearchSimilar should find related memories via entities")
	}
}
