package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/postgres"
	"github.com/scrypster/memento/pkg/types"
)

// postgresTestDSN returns the DSN for the test database.
// If POSTGRES_TEST_DSN is not set, tests are skipped.
func postgresTestDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set; skipping PostgreSQL integration tests")
	}
	return dsn
}

// newTestStore creates a fresh MemoryStore connected to the test database.
// It applies the schema and runs migrations, then registers cleanup.
func newTestStore(t *testing.T) *postgres.MemoryStore {
	t.Helper()

	dsn := postgresTestDSN(t)

	store, err := postgres.NewMemoryStore(dsn)
	require.NoError(t, err, "NewMemoryStore should succeed")

	// Clean up after the test
	t.Cleanup(func() {
		store.Close()
	})

	return store
}

// truncateMemories removes all rows from the memories table between tests.
func truncateMemories(t *testing.T, store *postgres.MemoryStore) {
	t.Helper()
	err := store.TruncateForTest(context.Background())
	require.NoError(t, err, "truncate memories")
}

// newTestMemory builds a minimal valid Memory for use in tests.
func newTestMemory(id string) *types.Memory {
	return &types.Memory{
		ID:      id,
		Content: "Test memory content for " + id,
		Source:  "test",
	}
}

// ---- Store tests ----

func TestStore_NilMemory(t *testing.T) {
	store := newTestStore(t)
	err := store.Store(context.Background(), nil)
	assert.ErrorIs(t, err, storage.ErrInvalidInput)
}

func TestStore_EmptyID(t *testing.T) {
	store := newTestStore(t)
	err := store.Store(context.Background(), &types.Memory{Content: "hello", Source: "test"})
	assert.ErrorIs(t, err, storage.ErrInvalidInput)
}

func TestStore_EmptyContent(t *testing.T) {
	store := newTestStore(t)
	err := store.Store(context.Background(), &types.Memory{ID: "mem:test:no-content", Source: "test"})
	assert.ErrorIs(t, err, storage.ErrInvalidInput)
}

func TestStore_BasicMemory(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:basic")
	require.NoError(t, store.Store(context.Background(), mem))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, mem.ID, got.ID)
	assert.Equal(t, mem.Content, got.Content)
	assert.Equal(t, mem.Source, got.Source)
}

func TestStore_UpsertUpdatesExisting(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:upsert")
	require.NoError(t, store.Store(context.Background(), mem))

	mem.Content = "Updated content"
	require.NoError(t, store.Store(context.Background(), mem))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated content", got.Content)
}

// ---- Provenance field tests ----

func TestStore_ProvenanceFields(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:provenance")
	mem.CreatedBy = "agent-alpha"
	mem.SessionID = "session-42"
	mem.SourceContext = map[string]interface{}{
		"tool":    "memento-cli",
		"version": "1.0.0",
	}

	require.NoError(t, store.Store(context.Background(), mem))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, "agent-alpha", got.CreatedBy)
	assert.Equal(t, "session-42", got.SessionID)
	require.NotNil(t, got.SourceContext)
	assert.Equal(t, "memento-cli", got.SourceContext["tool"])
	assert.Equal(t, "1.0.0", got.SourceContext["version"])
}

func TestStore_ProvenanceFieldsEmpty(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:provenance-empty")
	// Do not set CreatedBy, SessionID, SourceContext

	require.NoError(t, store.Store(context.Background(), mem))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, "", got.CreatedBy)
	assert.Equal(t, "", got.SessionID)
	assert.Nil(t, got.SourceContext)
}

func TestStore_SourceContextRoundtrip(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:source-context-roundtrip")
	mem.SourceContext = map[string]interface{}{
		"nested": map[string]interface{}{
			"key": "value",
		},
		"number": float64(42),
		"bool":   true,
	}

	require.NoError(t, store.Store(context.Background(), mem))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)

	// Re-marshal both to compare
	origJSON, err := json.Marshal(mem.SourceContext)
	require.NoError(t, err)
	gotJSON, err := json.Marshal(got.SourceContext)
	require.NoError(t, err)
	assert.JSONEq(t, string(origJSON), string(gotJSON))
}

func TestStore_SourceContextExceeds4KB(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	// Build a source_context that exceeds 4 KB when marshalled
	large := make(map[string]interface{})
	for i := 0; i < 200; i++ {
		large[fmt.Sprintf("key_%04d", i)] = strings.Repeat("x", 30)
	}

	mem := newTestMemory("mem:test:source-context-large")
	mem.SourceContext = large

	err := store.Store(context.Background(), mem)
	require.Error(t, err, "should reject source_context exceeding 4KB")
	assert.ErrorIs(t, err, storage.ErrInvalidInput)
}

// ---- Quality signal field tests ----

func TestStore_QualitySignalFields(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	now := time.Now().UTC().Truncate(time.Millisecond)
	mem := newTestMemory("mem:test:quality")
	mem.AccessCount = 7
	mem.LastAccessedAt = &now
	mem.DecayScore = 0.75
	mem.DecayUpdatedAt = &now

	require.NoError(t, store.Store(context.Background(), mem))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, 7, got.AccessCount)
	require.NotNil(t, got.LastAccessedAt)
	assert.WithinDuration(t, now, *got.LastAccessedAt, time.Second)
	assert.InDelta(t, 0.75, got.DecayScore, 0.0001)
	require.NotNil(t, got.DecayUpdatedAt)
	assert.WithinDuration(t, now, *got.DecayUpdatedAt, time.Second)
}

func TestStore_QualitySignalDefaults(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:quality-defaults")
	// Do not set quality signal fields; they get database defaults

	require.NoError(t, store.Store(context.Background(), mem))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, got.AccessCount)
	assert.Nil(t, got.LastAccessedAt)
	// decay_score defaults to 0.0 (Go zero value) when not explicitly set
	assert.InDelta(t, 0.0, got.DecayScore, 0.0001)
	assert.Nil(t, got.DecayUpdatedAt)
}

// ---- Lifecycle state field tests ----

func TestStore_LifecycleStateFields(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	now := time.Now().UTC().Truncate(time.Millisecond)
	mem := newTestMemory("mem:test:lifecycle")
	mem.State = "active"
	mem.StateUpdatedAt = &now

	require.NoError(t, store.Store(context.Background(), mem))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, "active", got.State)
	require.NotNil(t, got.StateUpdatedAt)
	assert.WithinDuration(t, now, *got.StateUpdatedAt, time.Second)
}

func TestStore_LifecycleStateEmpty(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:lifecycle-empty")
	// Do not set State or StateUpdatedAt

	require.NoError(t, store.Store(context.Background(), mem))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, "", got.State)
	assert.Nil(t, got.StateUpdatedAt)
}

// ---- Get tests ----

func TestGet_NotFound(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	_, err := store.Get(context.Background(), "mem:test:does-not-exist")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestGet_EmptyID(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Get(context.Background(), "")
	assert.ErrorIs(t, err, storage.ErrInvalidInput)
}

func TestGet_AllNewFields(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	now := time.Now().UTC().Truncate(time.Millisecond)

	mem := newTestMemory("mem:test:all-new-fields")
	mem.State = "completed"
	mem.StateUpdatedAt = &now
	mem.CreatedBy = "test-agent"
	mem.SessionID = "ses-xyz"
	mem.SourceContext = map[string]interface{}{"env": "test"}
	mem.AccessCount = 3
	mem.LastAccessedAt = &now
	mem.DecayScore = 0.5
	mem.DecayUpdatedAt = &now

	require.NoError(t, store.Store(context.Background(), mem))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)

	assert.Equal(t, "completed", got.State)
	require.NotNil(t, got.StateUpdatedAt)
	assert.Equal(t, "test-agent", got.CreatedBy)
	assert.Equal(t, "ses-xyz", got.SessionID)
	require.NotNil(t, got.SourceContext)
	assert.Equal(t, "test", got.SourceContext["env"])
	assert.Equal(t, 3, got.AccessCount)
	require.NotNil(t, got.LastAccessedAt)
	assert.InDelta(t, 0.5, got.DecayScore, 0.0001)
	require.NotNil(t, got.DecayUpdatedAt)
}

// ---- Delete tests ----

func TestDelete_Existing(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:delete")
	require.NoError(t, store.Store(context.Background(), mem))
	require.NoError(t, store.Delete(context.Background(), mem.ID))

	_, err := store.Get(context.Background(), mem.ID)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestDelete_NotFound(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	err := store.Delete(context.Background(), "mem:test:ghost")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

// ---- UpdateStatus tests ----

func TestUpdateStatus_Success(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:update-status")
	require.NoError(t, store.Store(context.Background(), mem))
	require.NoError(t, store.UpdateStatus(context.Background(), mem.ID, types.StatusEnriched))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, types.StatusEnriched, got.Status)
}

func TestUpdateStatus_NotFound(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	err := store.UpdateStatus(context.Background(), "mem:test:ghost", types.StatusEnriched)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

// ---- UpdateEnrichment tests ----

func TestUpdateEnrichment_Success(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:update-enrichment")
	require.NoError(t, store.Store(context.Background(), mem))

	now := time.Now()
	update := storage.EnrichmentUpdate{
		EntityStatus:       types.EnrichmentCompleted,
		RelationshipStatus: types.EnrichmentCompleted,
		EmbeddingStatus:    types.EnrichmentCompleted,
		EnrichmentAttempts: 1,
		EnrichedAt:         &now,
	}
	require.NoError(t, store.UpdateEnrichment(context.Background(), mem.ID, update))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, types.EnrichmentCompleted, got.EntityStatus)
	assert.Equal(t, types.EnrichmentCompleted, got.RelationshipStatus)
	assert.Equal(t, types.EnrichmentCompleted, got.EmbeddingStatus)
	assert.Equal(t, 1, got.EnrichmentAttempts)
	require.NotNil(t, got.EnrichedAt)
}

func TestUpdateEnrichment_NotFound(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	err := store.UpdateEnrichment(context.Background(), "mem:test:ghost", storage.EnrichmentUpdate{})
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

// ---- List tests ----

func TestList_BasicPagination(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	// Store 3 memories
	for i := 0; i < 3; i++ {
		mem := newTestMemory(fmt.Sprintf("mem:test:list-%02d", i))
		require.NoError(t, store.Store(context.Background(), mem))
	}

	result, err := store.List(context.Background(), storage.ListOptions{
		Page:  1,
		Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	assert.Len(t, result.Items, 3)
	assert.False(t, result.HasMore)
}

func TestList_StatusFilter(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	for i := 0; i < 2; i++ {
		mem := newTestMemory(fmt.Sprintf("mem:test:list-pending-%02d", i))
		require.NoError(t, store.Store(context.Background(), mem))
	}

	enriched := newTestMemory("mem:test:list-enriched")
	enriched.Status = types.StatusEnriched
	require.NoError(t, store.Store(context.Background(), enriched))

	result, err := store.List(context.Background(), storage.ListOptions{
		Page:  1,
		Limit: 10,
		Filter: map[string]interface{}{
			"status": types.StatusPending,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	assert.Len(t, result.Items, 2)
}

// ---- Update tests ----

func TestUpdate_NotFound(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:update-ghost")
	err := store.Update(context.Background(), mem)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestUpdate_Success(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:update-success")
	require.NoError(t, store.Store(context.Background(), mem))

	mem.Content = "Updated"
	mem.State = "archived"
	require.NoError(t, store.Update(context.Background(), mem))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Content)
	assert.Equal(t, "archived", got.State)
}

// ---- IncrementAccessCount tests ----

func TestIncrementAccessCount_Basic(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:inc-access")
	mem.AccessCount = 0
	require.NoError(t, store.Store(context.Background(), mem))

	// First increment
	require.NoError(t, store.IncrementAccessCount(context.Background(), mem.ID))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.AccessCount)
	assert.NotNil(t, got.LastAccessedAt)

	// Second increment
	require.NoError(t, store.IncrementAccessCount(context.Background(), mem.ID))

	got2, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, got2.AccessCount)
}

func TestIncrementAccessCount_NotFound(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	err := store.IncrementAccessCount(context.Background(), "mem:test:ghost-access")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestIncrementAccessCount_LastAccessedAtUpdated(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	before := time.Now().Add(-time.Second)

	mem := newTestMemory("mem:test:last-accessed-pg")
	require.NoError(t, store.Store(context.Background(), mem))

	require.NoError(t, store.IncrementAccessCount(context.Background(), mem.ID))

	after := time.Now().Add(time.Second)

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)

	require.NotNil(t, got.LastAccessedAt, "LastAccessedAt should be set after increment")
	assert.True(t, !got.LastAccessedAt.Before(before) && !got.LastAccessedAt.After(after),
		"LastAccessedAt %v should be within [%v, %v]", got.LastAccessedAt, before, after)
}

func TestIncrementAccessCount_StartsFromExistingCount(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:existing-count-pg")
	mem.AccessCount = 5
	require.NoError(t, store.Store(context.Background(), mem))

	require.NoError(t, store.IncrementAccessCount(context.Background(), mem.ID))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Equal(t, 6, got.AccessCount)
}

// ---- NullableTimestamp edge cases ----

func TestNullableTimestamps_AllNil(t *testing.T) {
	store := newTestStore(t)
	truncateMemories(t, store)

	mem := newTestMemory("mem:test:nullable-ts")
	// Leave EnrichedAt, StateUpdatedAt, LastAccessedAt, DecayUpdatedAt as nil

	require.NoError(t, store.Store(context.Background(), mem))

	got, err := store.Get(context.Background(), mem.ID)
	require.NoError(t, err)
	assert.Nil(t, got.EnrichedAt)
	assert.Nil(t, got.StateUpdatedAt)
	assert.Nil(t, got.LastAccessedAt)
	assert.Nil(t, got.DecayUpdatedAt)
}
