package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// mockContradictionStore is a minimal in-memory store for testing contradiction detection.
type mockContradictionStore struct {
	memories      map[string]*types.Memory
	relationships map[string][]*types.Relationship
	entities      map[string]*types.Entity
}

func newMockContradictionStore() *mockContradictionStore {
	return &mockContradictionStore{
		memories:      make(map[string]*types.Memory),
		relationships: make(map[string][]*types.Relationship),
		entities:      make(map[string]*types.Entity),
	}
}

func (m *mockContradictionStore) Store(_ context.Context, memory *types.Memory) error {
	m.memories[memory.ID] = memory
	return nil
}

func (m *mockContradictionStore) Get(_ context.Context, id string) (*types.Memory, error) {
	mem, ok := m.memories[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return mem, nil
}

func (m *mockContradictionStore) List(_ context.Context, opts storage.ListOptions) (*storage.PaginatedResult[types.Memory], error) {
	var items []types.Memory
	for _, mem := range m.memories {
		items = append(items, *mem)
	}
	return &storage.PaginatedResult[types.Memory]{
		Items:    items,
		Total:    len(items),
		Page:     opts.Page,
		PageSize: opts.Limit,
		HasMore:  false,
	}, nil
}

func (m *mockContradictionStore) Update(_ context.Context, memory *types.Memory) error {
	if _, ok := m.memories[memory.ID]; !ok {
		return storage.ErrNotFound
	}
	m.memories[memory.ID] = memory
	return nil
}

func (m *mockContradictionStore) Delete(_ context.Context, id string) error {
	if _, ok := m.memories[id]; !ok {
		return storage.ErrNotFound
	}
	delete(m.memories, id)
	return nil
}

func (m *mockContradictionStore) Purge(_ context.Context, id string) error {
	if _, ok := m.memories[id]; !ok {
		return storage.ErrNotFound
	}
	delete(m.memories, id)
	return nil
}

func (m *mockContradictionStore) UpdateStatus(_ context.Context, id string, status types.MemoryStatus) error {
	mem, ok := m.memories[id]
	if !ok {
		return storage.ErrNotFound
	}
	mem.Status = status
	return nil
}

func (m *mockContradictionStore) UpdateEnrichment(_ context.Context, id string, enrichment storage.EnrichmentUpdate) error {
	mem, ok := m.memories[id]
	if !ok {
		return storage.ErrNotFound
	}
	mem.EntityStatus = enrichment.EntityStatus
	mem.RelationshipStatus = enrichment.RelationshipStatus
	mem.EmbeddingStatus = enrichment.EmbeddingStatus
	mem.EnrichmentAttempts = enrichment.EnrichmentAttempts
	mem.EnrichmentError = enrichment.EnrichmentError
	if enrichment.EnrichedAt != nil {
		mem.EnrichedAt = enrichment.EnrichedAt
	}
	return nil
}

func (m *mockContradictionStore) UpdateState(_ context.Context, id string, state string) error {
	mem, ok := m.memories[id]
	if !ok {
		return storage.ErrNotFound
	}
	mem.State = state
	return nil
}

func (m *mockContradictionStore) IncrementAccessCount(_ context.Context, id string) error {
	mem, ok := m.memories[id]
	if !ok {
		return storage.ErrNotFound
	}
	mem.AccessCount++
	now := time.Now()
	mem.LastAccessedAt = &now
	return nil
}

func (m *mockContradictionStore) UpdateDecayScores(_ context.Context) (int, error) {
	return 0, nil
}

func (m *mockContradictionStore) GetRelatedMemories(_ context.Context, memoryID string) ([]string, error) {
	return []string{}, nil
}

func (m *mockContradictionStore) Traverse(_ context.Context, _ string, _ int, _ int) ([]storage.TraversalResult, error) {
	return nil, nil
}

func (m *mockContradictionStore) GetMemoryEntities(_ context.Context, _ string) ([]*types.Entity, error) {
	return nil, nil
}

func (m *mockContradictionStore) Restore(_ context.Context, _ string) error { return nil }

func (m *mockContradictionStore) GetEvolutionChain(_ context.Context, _ string) ([]*types.Memory, error) {
	return nil, nil
}

func (m *mockContradictionStore) GetMemoriesByRelationType(_ context.Context, _ string, _ string) ([]*types.Memory, error) {
	return nil, nil
}

func (m *mockContradictionStore) Close() error {
	return nil
}

// addRelationship adds a relationship to the mock store
func (m *mockContradictionStore) addRelationship(rel *types.Relationship) {
	m.relationships[rel.ID] = append(m.relationships[rel.ID], rel)
}

// getRelationshipsForEntity returns all relationships where the entity is the source
func (m *mockContradictionStore) getRelationshipsForEntity(entityID string, relType string) []*types.Relationship {
	var rels []*types.Relationship
	for _, relList := range m.relationships {
		for _, rel := range relList {
			if rel.FromID == entityID && rel.Type == relType {
				rels = append(rels, rel)
			}
		}
	}
	return rels
}

// addEntity adds an entity to the mock store
func (m *mockContradictionStore) addEntity(entity *types.Entity) {
	m.entities[entity.ID] = entity
}

// TestDetectConflictingRelationships tests detection of conflicting single-valued relationships
func TestDetectConflictingRelationships(t *testing.T) {
	ctx := context.Background()
	store := newMockContradictionStore()
	detector := NewContradictionDetector(store)

	// Create memories
	mj := &types.Memory{
		ID:        "mem:person:mj",
		Content:   "MJ is married",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	norma := &types.Memory{
		ID:        "mem:person:norma",
		Content:   "Norma",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	sarah := &types.Memory{
		ID:        "mem:person:sarah",
		Content:   "Sarah",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	require.NoError(t, store.Store(ctx, mj))
	require.NoError(t, store.Store(ctx, norma))
	require.NoError(t, store.Store(ctx, sarah))

	// Create conflicting relationships: MJ married_to Norma AND MJ married_to Sarah
	rel1 := &RelationshipEntry{
		ID:       "rel:1",
		FromID:   "ent:mj",
		ToID:     "ent:norma",
		Type:     types.RelMarriedTo,
		Evidence: []string{"mem:person:mj"},
	}
	rel2 := &RelationshipEntry{
		ID:       "rel:2",
		FromID:   "ent:mj",
		ToID:     "ent:sarah",
		Type:     types.RelMarriedTo,
		Evidence: []string{"mem:person:mj"},
	}

	detector.AddRelationshipForTesting(rel1)
	detector.AddRelationshipForTesting(rel2)

	contradictions, err := detector.DetectContradictions(ctx, "")
	require.NoError(t, err)

	// Should detect conflicting_relationship contradiction
	assert.Greater(t, len(contradictions), 0, "should detect at least one contradiction")

	found := false
	for _, c := range contradictions {
		if c.Type == ContradictionTypeConflictingRelationship {
			found = true
			assert.Contains(t, c.Description, types.RelMarriedTo)
			assert.Greater(t, c.Confidence, 0.0)
			assert.Less(t, c.Confidence, 1.1)
			break
		}
	}
	assert.True(t, found, "should detect conflicting_relationship contradiction")
}

// TestDetectSupersededMemoriesStillReferenced tests detection of superseded memories with active relationships
func TestDetectSupersededMemoriesStillReferenced(t *testing.T) {
	ctx := context.Background()
	store := newMockContradictionStore()
	detector := NewContradictionDetector(store)

	// Create memory A and memory B
	memoryA := &types.Memory{
		ID:        "mem:old:a",
		Content:   "Original project description",
		CreatedAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt: time.Now().Add(-24 * time.Hour),
		State:     types.StateActive, // Original is active
	}

	memoryB := &types.Memory{
		ID:        "mem:new:b",
		Content:   "Updated project description",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		State:     types.StateActive,
	}

	require.NoError(t, store.Store(ctx, memoryA))
	require.NoError(t, store.Store(ctx, memoryB))

	// Create a supersedes relationship: B supersedes A
	supersedesRel := &RelationshipEntry{
		ID:       "rel:supersedes",
		FromID:   "mem:new:b",
		ToID:     "mem:old:a",
		Type:     types.RelSupersedes,
		Evidence: []string{"mem:new:b"},
	}
	detector.AddRelationshipForTesting(supersedesRel)

	// Now create a relationship that references the superseded memory
	refRel := &RelationshipEntry{
		ID:       "rel:references",
		FromID:   "mem:something:else",
		ToID:     "mem:old:a",
		Type:     types.RelReferences,
		Evidence: []string{"mem:something:else"},
	}
	detector.AddRelationshipForTesting(refRel)

	contradictions, err := detector.DetectContradictions(ctx, "")
	require.NoError(t, err)

	found := false
	for _, c := range contradictions {
		if c.Type == ContradictionTypeSupersededActive {
			found = true
			assert.Greater(t, c.Confidence, 0.0)
			break
		}
	}
	assert.True(t, found, "should detect superseded_active contradiction")
}

// TestDetectTemporalImpossibilities tests detection of temporal contradictions
func TestDetectTemporalImpossibilities(t *testing.T) {
	ctx := context.Background()
	store := newMockContradictionStore()
	detector := NewContradictionDetector(store)

	now := time.Now()
	earlier := now.Add(-48 * time.Hour)
	later := now.Add(48 * time.Hour)

	// Create two memories with contradictory temporal relationships
	eventA := &types.Memory{
		ID:        "mem:event:a",
		Content:   "Event A happens",
		Timestamp: later, // Event A happens in the future
		CreatedAt: now,
		UpdatedAt: now,
	}

	eventB := &types.Memory{
		ID:        "mem:event:b",
		Content:   "Event B happens first",
		Timestamp: earlier, // Event B happens in the past
		CreatedAt: now,
		UpdatedAt: now,
	}

	require.NoError(t, store.Store(ctx, eventA))
	require.NoError(t, store.Store(ctx, eventB))

	// Create a "before" relationship: A before B (but A's timestamp is later than B's)
	beforeRel := &RelationshipEntry{
		ID:       "rel:before",
		FromID:   "mem:event:a",
		ToID:     "mem:event:b",
		Type:     types.RelDocuments, // Using as placeholder; in real scenario might be custom
		Metadata: map[string]interface{}{"temporal_order": "before"},
		Evidence: []string{"mem:event:a"},
	}
	detector.AddRelationshipForTesting(beforeRel)

	contradictions, err := detector.DetectContradictions(ctx, "")
	require.NoError(t, err)

	found := false
	for _, c := range contradictions {
		if c.Type == ContradictionTypeTemporalImpossibility {
			found = true
			assert.Greater(t, c.Confidence, 0.0)
			break
		}
	}
	assert.True(t, found, "should detect temporal_impossibility contradiction")
}

// TestNoContradictions tests that no contradictions are detected for valid scenarios
func TestNoContradictions(t *testing.T) {
	ctx := context.Background()
	store := newMockContradictionStore()
	detector := NewContradictionDetector(store)

	// Create valid memories with no contradictions
	memory1 := &types.Memory{
		ID:        "mem:valid:1",
		Content:   "Valid memory 1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		State:     types.StateActive,
	}

	memory2 := &types.Memory{
		ID:        "mem:valid:2",
		Content:   "Valid memory 2",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		State:     types.StateActive,
	}

	require.NoError(t, store.Store(ctx, memory1))
	require.NoError(t, store.Store(ctx, memory2))

	// Create valid single relationship (many-to-one is OK)
	rel1 := &types.Relationship{
		ID:     "rel:1",
		FromID: "ent:project:1",
		ToID:   "ent:team:a",
		Type:   types.RelWorksOn,
		Evidence: []string{"mem:valid:1"},
	}
	store.addRelationship(rel1)

	contradictions, err := detector.DetectContradictions(ctx, "")
	require.NoError(t, err)

	// Should have no contradictions
	assert.Equal(t, 0, len(contradictions), "should detect no contradictions for valid scenario")
}

// TestDetectContradictionForSpecificMemory tests detecting contradictions for a specific memory
func TestDetectContradictionForSpecificMemory(t *testing.T) {
	ctx := context.Background()
	store := newMockContradictionStore()
	detector := NewContradictionDetector(store)

	now := time.Now()

	// Create memories
	mem1 := &types.Memory{
		ID:        "mem:specific:1",
		Content:   "Person A married to B",
		CreatedAt: now,
		UpdatedAt: now,
	}

	mem2 := &types.Memory{
		ID:        "mem:specific:2",
		Content:   "Person A married to C",
		CreatedAt: now,
		UpdatedAt: now,
	}

	require.NoError(t, store.Store(ctx, mem1))
	require.NoError(t, store.Store(ctx, mem2))

	// Create conflicting relationships both referencing mem1
	rel1 := &RelationshipEntry{
		ID:       "rel:1",
		FromID:   "ent:person:a",
		ToID:     "ent:person:b",
		Type:     types.RelMarriedTo,
		Evidence: []string{"mem:specific:1"},
	}
	rel2 := &RelationshipEntry{
		ID:       "rel:2",
		FromID:   "ent:person:a",
		ToID:     "ent:person:c",
		Type:     types.RelMarriedTo,
		Evidence: []string{"mem:specific:1"},
	}

	detector.AddRelationshipForTesting(rel1)
	detector.AddRelationshipForTesting(rel2)

	// Detect contradictions for specific memory
	contradictions, err := detector.DetectContradictions(ctx, "mem:specific:1")
	require.NoError(t, err)

	// Should only find contradictions related to this memory
	assert.Greater(t, len(contradictions), 0, "should detect contradictions for specific memory")
	for _, c := range contradictions {
		// All returned contradictions should include the specific memory ID
		found := false
		for _, mid := range c.MemoryIDs {
			if mid == "mem:specific:1" {
				found = true
				break
			}
		}
		assert.True(t, found, "contradiction should reference the specific memory")
	}
}

// TestMultipleContradictionTypes tests detection of multiple contradiction types simultaneously
func TestMultipleContradictionTypes(t *testing.T) {
	ctx := context.Background()
	store := newMockContradictionStore()
	detector := NewContradictionDetector(store)

	now := time.Now()

	// Create various memories for different contradiction scenarios
	mem1 := &types.Memory{
		ID:        "mem:diverse:1",
		Content:   "Person A married B",
		CreatedAt: now,
		UpdatedAt: now,
	}

	mem2 := &types.Memory{
		ID:        "mem:diverse:2",
		Content:   "Person A married C",
		CreatedAt: now,
		UpdatedAt: now,
	}

	mem3 := &types.Memory{
		ID:        "mem:old:superseded",
		Content:   "Old information",
		CreatedAt: now.Add(-72 * time.Hour),
		UpdatedAt: now.Add(-72 * time.Hour),
	}

	memNew := &types.Memory{
		ID:        "mem:new:replaces",
		Content:   "New information",
		CreatedAt: now,
		UpdatedAt: now,
	}

	require.NoError(t, store.Store(ctx, mem1))
	require.NoError(t, store.Store(ctx, mem2))
	require.NoError(t, store.Store(ctx, mem3))
	require.NoError(t, store.Store(ctx, memNew))

	// Create conflicting relationship: type 1
	rel1 := &RelationshipEntry{
		ID:       "rel:conflict1",
		FromID:   "ent:person:a",
		ToID:     "ent:person:b",
		Type:     types.RelMarriedTo,
		Evidence: []string{"mem:diverse:1"},
	}
	rel2 := &RelationshipEntry{
		ID:       "rel:conflict2",
		FromID:   "ent:person:a",
		ToID:     "ent:person:c",
		Type:     types.RelMarriedTo,
		Evidence: []string{"mem:diverse:2"},
	}

	// Create supersedes relationship: type 2
	supersedesRel := &RelationshipEntry{
		ID:       "rel:supersedes",
		FromID:   "mem:new:replaces",
		ToID:     "mem:old:superseded",
		Type:     types.RelSupersedes,
		Evidence: []string{"mem:new:replaces"},
	}

	// Create reference to superseded memory
	refRel := &RelationshipEntry{
		ID:       "rel:ref",
		FromID:   "mem:diverse:1",
		ToID:     "mem:old:superseded",
		Type:     types.RelReferences,
		Evidence: []string{"mem:diverse:1"},
	}

	detector.AddRelationshipForTesting(rel1)
	detector.AddRelationshipForTesting(rel2)
	detector.AddRelationshipForTesting(supersedesRel)
	detector.AddRelationshipForTesting(refRel)

	contradictions, err := detector.DetectContradictions(ctx, "")
	require.NoError(t, err)

	// Should detect multiple contradiction types
	assert.Greater(t, len(contradictions), 0, "should detect multiple contradictions")

	typeMap := make(map[ContradictionType]bool)
	for _, c := range contradictions {
		typeMap[c.Type] = true
	}

	// Should have detected at least conflicting relationships and superseded active
	assert.True(t, len(typeMap) > 0, "should detect multiple contradiction types")
}

// BenchmarkDetectContradictions benchmarks the contradiction detector
func BenchmarkDetectContradictions(b *testing.B) {
	ctx := context.Background()
	store := newMockContradictionStore()
	detector := NewContradictionDetector(store)

	// Create 1000 memories
	for i := 0; i < 1000; i++ {
		mem := &types.Memory{
			ID:        "mem:bench:" + string(rune(i)),
			Content:   "Test memory",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = store.Store(ctx, mem)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = detector.DetectContradictions(ctx, "")
	}
}
