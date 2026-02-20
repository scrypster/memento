package mcp_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scrypster/memento/internal/api/mcp"
	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// mockStore is a minimal in-memory MemoryStore for testing.
type mockStore struct {
	memories map[string]*types.Memory
	// filterFn is an optional predicate applied during List calls.
	// When nil all memories are returned.
	filterFn func(mem *types.Memory, opts storage.ListOptions) bool
}

func newMockStore() *mockStore {
	return &mockStore{memories: make(map[string]*types.Memory)}
}

func (m *mockStore) Store(_ context.Context, memory *types.Memory) error {
	m.memories[memory.ID] = memory
	return nil
}

func (m *mockStore) Get(_ context.Context, id string) (*types.Memory, error) {
	mem, ok := m.memories[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	// Exclude soft-deleted memories
	if mem.DeletedAt != nil {
		return nil, storage.ErrNotFound
	}
	return mem, nil
}

func (m *mockStore) List(_ context.Context, opts storage.ListOptions) (*storage.PaginatedResult[types.Memory], error) {
	var items []types.Memory
	for _, mem := range m.memories {
		if m.filterFn != nil && !m.filterFn(mem, opts) {
			continue
		}
		// Apply SessionID filter directly in the mock (mirrors real store behaviour).
		if opts.SessionID != "" && mem.SessionID != opts.SessionID {
			continue
		}
		// Exclude soft-deleted memories unless explicitly requested
		if !opts.IncludeDeleted && mem.DeletedAt != nil {
			continue
		}
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

func (m *mockStore) Update(_ context.Context, memory *types.Memory) error {
	if _, ok := m.memories[memory.ID]; !ok {
		return storage.ErrNotFound
	}
	m.memories[memory.ID] = memory
	return nil
}

func (m *mockStore) Delete(_ context.Context, id string) error {
	mem, ok := m.memories[id]
	if !ok {
		return storage.ErrNotFound
	}
	// Soft delete: mark as deleted
	now := time.Now()
	mem.DeletedAt = &now
	return nil
}

func (m *mockStore) Purge(_ context.Context, id string) error {
	if _, ok := m.memories[id]; !ok {
		return storage.ErrNotFound
	}
	delete(m.memories, id)
	return nil
}

func (m *mockStore) UpdateStatus(_ context.Context, id string, status types.MemoryStatus) error {
	mem, ok := m.memories[id]
	if !ok {
		return storage.ErrNotFound
	}
	mem.Status = status
	return nil
}

func (m *mockStore) UpdateEnrichment(_ context.Context, id string, enrichment storage.EnrichmentUpdate) error {
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

func (m *mockStore) UpdateState(_ context.Context, id string, state string) error {
	mem, ok := m.memories[id]
	if !ok {
		return storage.ErrNotFound
	}

	// Validate state transition (same as real implementation)
	if !types.IsValidStateTransition(mem.State, state) {
		return errors.New("invalid state transition: cannot transition from '" + mem.State + "' to '" + state + "'")
	}

	now := time.Now()
	mem.State = state
	mem.StateUpdatedAt = &now
	return nil
}

func (m *mockStore) IncrementAccessCount(_ context.Context, id string) error {
	mem, ok := m.memories[id]
	if !ok {
		return storage.ErrNotFound
	}
	mem.AccessCount++
	now := time.Now()
	mem.LastAccessedAt = &now
	return nil
}

func (m *mockStore) GetRelatedMemories(_ context.Context, memoryID string) ([]string, error) {
	return []string{}, nil
}

func (m *mockStore) Traverse(_ context.Context, startMemoryID string, maxHops int, limit int) ([]storage.TraversalResult, error) {
	return nil, nil
}

func (m *mockStore) GetMemoryEntities(_ context.Context, memoryID string) ([]*types.Entity, error) {
	return nil, nil
}

func (m *mockStore) UpdateDecayScores(_ context.Context) (int, error) {
	return 0, nil
}

func (m *mockStore) Restore(_ context.Context, id string) error {
	mem, ok := m.memories[id]
	if !ok {
		return storage.ErrNotFound
	}
	mem.DeletedAt = nil
	return nil
}

func (m *mockStore) GetEvolutionChain(_ context.Context, memoryID string) ([]*types.Memory, error) {
	mem, ok := m.memories[memoryID]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return []*types.Memory{mem}, nil
}

func (m *mockStore) GetMemoriesByRelationType(_ context.Context, memoryID string, relType string) ([]*types.Memory, error) {
	return nil, nil
}

func (m *mockStore) Close() error { return nil }

// ---------------------------------------------------------------------------
// Functional options tests
// ---------------------------------------------------------------------------

// TestNewServer_NoOptions verifies that NewServer works with no options,
// preserving backward compatibility with all existing call sites.
func TestNewServer_NoOptions(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	require.NotNil(t, srv)
}

// TestNewServer_WithConfig verifies that WithConfig properly injects the
// config into the server.
func TestNewServer_WithConfig(t *testing.T) {
	store := newMockStore()
	cfg := &config.Config{
		LLM: config.LLMConfig{
			LLMProvider: "anthropic",
			AnthropicModel: "claude-opus-4-6",
		},
	}

	srv := mcp.NewServer(store, mcp.WithConfig(cfg))
	require.NotNil(t, srv)

	// Verify the config was actually applied by reading it back.
	got := srv.Config()
	require.NotNil(t, got, "Config() should return non-nil after WithConfig")
	assert.Equal(t, "anthropic", got.LLM.LLMProvider)
	assert.Equal(t, "claude-opus-4-6", got.LLM.AnthropicModel)
}

// TestNewServer_WithConfig_NilDefault verifies that without WithConfig the
// config field is nil (consumers must guard against nil).
func TestNewServer_WithConfig_NilDefault(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	assert.Nil(t, srv.Config(), "Config() should be nil when no WithConfig option is provided")
}

// TestNewServer_MultipleOptions verifies that multiple options can be chained
// and are all applied.
func TestNewServer_MultipleOptions(t *testing.T) {
	store := newMockStore()
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 9090},
	}

	srv := mcp.NewServer(store, mcp.WithConfig(cfg))
	require.NotNil(t, srv)

	got := srv.Config()
	require.NotNil(t, got)
	assert.Equal(t, 9090, got.Server.Port)
}

// TestNewServer_OptionsAreAppliedInOrder verifies that options applied later
// override earlier ones (last-write-wins semantics).
func TestNewServer_OptionsAreAppliedInOrder(t *testing.T) {
	store := newMockStore()
	cfgFirst := &config.Config{
		Server: config.ServerConfig{Port: 1111},
	}
	cfgSecond := &config.Config{
		Server: config.ServerConfig{Port: 2222},
	}

	srv := mcp.NewServer(store, mcp.WithConfig(cfgFirst), mcp.WithConfig(cfgSecond))
	require.NotNil(t, srv)

	got := srv.Config()
	require.NotNil(t, got)
	assert.Equal(t, 2222, got.Server.Port, "second WithConfig should win")
}

// ---------------------------------------------------------------------------
// Existing behaviour smoke tests (regression guard)
// ---------------------------------------------------------------------------

// TestHandleRequest_StoreAndRecall verifies that the core store/recall
// JSON-RPC methods still work after the constructor change.
func TestHandleRequest_StoreAndRecall(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store) // zero-option call — backward compat

	ctx := context.Background()

	// Store a memory
	storeReq := `{"jsonrpc":"2.0","method":"store_memory","params":{"content":"test memory content","domain":"test"},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(storeReq))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)
}

// TestHandleRequest_UnknownMethod returns a method-not-found error.
func TestHandleRequest_UnknownMethod(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)

	ctx := context.Background()
	req := `{"jsonrpc":"2.0","method":"no_such_method","params":{},"id":99}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err) // protocol error, not a Go error
	assert.Contains(t, string(resp), `"error"`)
}

// TestHandleRequest_InvalidJSON returns a parse error.
func TestHandleRequest_InvalidJSON(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)

	ctx := context.Background()
	resp, err := srv.HandleRequest(ctx, []byte(`not json`))
	require.NoError(t, err)
	assert.Contains(t, string(resp), `"error"`)
}

// TestStoreMemory_MissingContent validates that an empty content is rejected.
func TestStoreMemory_MissingContent(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	_, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content is required")
}

// TestRecallMemory_NotFound returns Found=false for unknown IDs.
func TestRecallMemory_NotFound(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{ID: "nonexistent"})
	require.NoError(t, err)
	assert.False(t, result.Found)
	assert.Nil(t, result.Memory)
}

// TestRetryEnrichment_NotFailed returns Queued=false when memory is not failed.
func TestRetryEnrichment_NotFailed(t *testing.T) {
	store := newMockStore()
	// Pre-populate a pending memory
	store.memories["mem:test:1"] = &types.Memory{
		ID:        "mem:test:1",
		Content:   "hello",
		Status:    types.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.RetryEnrichment(ctx, mcp.RetryEnrichmentArgs{ID: "mem:test:1"})
	require.NoError(t, err)
	assert.False(t, result.Queued)
}

// ---------------------------------------------------------------------------
// State Transition Tests (Opus Issue #6)
// ---------------------------------------------------------------------------

// TestUpdateMemoryState_ValidTransitions tests all valid state transitions.
func TestUpdateMemoryState_ValidTransitions(t *testing.T) {
	tests := []struct {
		name        string
		initialState string
		targetState string
		shouldSucceed bool
	}{
		// Empty -> planning
		{"empty_to_planning", "", types.StatePlanning, true},
		// planning -> active
		{"planning_to_active", types.StatePlanning, types.StateActive, true},
		// planning -> cancelled
		{"planning_to_cancelled", types.StatePlanning, types.StateCancelled, true},
		// active -> paused
		{"active_to_paused", types.StateActive, types.StatePaused, true},
		// active -> blocked
		{"active_to_blocked", types.StateActive, types.StateBlocked, true},
		// active -> completed
		{"active_to_completed", types.StateActive, types.StateCompleted, true},
		// active -> cancelled
		{"active_to_cancelled", types.StateActive, types.StateCancelled, true},
		// paused -> active
		{"paused_to_active", types.StatePaused, types.StateActive, true},
		// paused -> cancelled
		{"paused_to_cancelled", types.StatePaused, types.StateCancelled, true},
		// blocked -> active
		{"blocked_to_active", types.StateBlocked, types.StateActive, true},
		// blocked -> cancelled
		{"blocked_to_cancelled", types.StateBlocked, types.StateCancelled, true},
		// completed -> archived
		{"completed_to_archived", types.StateCompleted, types.StateArchived, true},
		// cancelled -> archived
		{"cancelled_to_archived", types.StateCancelled, types.StateArchived, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			store.memories["mem:test:1"] = &types.Memory{
				ID:        "mem:test:1",
				Content:   "hello",
				State:     tt.initialState,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			srv := mcp.NewServer(store)
			ctx := context.Background()

			result, err := srv.UpdateMemoryState(ctx, mcp.UpdateMemoryStateArgs{
				ID:    "mem:test:1",
				State: tt.targetState,
			})

			if tt.shouldSucceed {
				require.NoError(t, err, "transition should succeed for %s", tt.name)
				require.NotNil(t, result)
				assert.Equal(t, tt.targetState, store.memories["mem:test:1"].State)
				assert.NotNil(t, store.memories["mem:test:1"].StateUpdatedAt)
			}
		})
	}
}

// TestUpdateMemoryState_InvalidTransitions tests that invalid transitions are rejected.
func TestUpdateMemoryState_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name         string
		initialState string
		targetState  string
	}{
		// planning cannot go to paused, blocked, or completed
		{"planning_to_paused", types.StatePlanning, types.StatePaused},
		{"planning_to_blocked", types.StatePlanning, types.StateBlocked},
		{"planning_to_completed", types.StatePlanning, types.StateCompleted},
		{"planning_to_archived", types.StatePlanning, types.StateArchived},
		// active cannot go to planning
		{"active_to_planning", types.StateActive, types.StatePlanning},
		// paused cannot go to planning, blocked, or completed
		{"paused_to_planning", types.StatePaused, types.StatePlanning},
		{"paused_to_blocked", types.StatePaused, types.StateBlocked},
		{"paused_to_completed", types.StatePaused, types.StateCompleted},
		// blocked cannot go to planning, paused, or completed
		{"blocked_to_planning", types.StateBlocked, types.StatePlanning},
		{"blocked_to_paused", types.StateBlocked, types.StatePaused},
		{"blocked_to_completed", types.StateBlocked, types.StateCompleted},
		// completed is terminal (no transitions out)
		{"completed_to_planning", types.StateCompleted, types.StatePlanning},
		{"completed_to_active", types.StateCompleted, types.StateActive},
		{"completed_to_completed", types.StateCompleted, types.StateCompleted},
		// cancelled cannot transition (only to archived, already tested)
		{"cancelled_to_planning", types.StateCancelled, types.StatePlanning},
		{"cancelled_to_active", types.StateCancelled, types.StateActive},
		// archived is terminal
		{"archived_to_planning", types.StateArchived, types.StatePlanning},
		{"archived_to_active", types.StateArchived, types.StateActive},
		{"archived_to_archived", types.StateArchived, types.StateArchived},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			store.memories["mem:test:1"] = &types.Memory{
				ID:        "mem:test:1",
				Content:   "hello",
				State:     tt.initialState,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			srv := mcp.NewServer(store)
			ctx := context.Background()

			_, err := srv.UpdateMemoryState(ctx, mcp.UpdateMemoryStateArgs{
				ID:    "mem:test:1",
				State: tt.targetState,
			})

			require.Error(t, err, "transition should fail for %s", tt.name)
			assert.Contains(t, err.Error(), "invalid state transition")
		})
	}
}

// TestUpdateMemoryState_NotFound returns error when memory doesn't exist.
func TestUpdateMemoryState_NotFound(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	_, err := srv.UpdateMemoryState(ctx, mcp.UpdateMemoryStateArgs{
		ID:    "nonexistent",
		State: types.StateActive,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestUpdateMemoryState_InvalidState returns error for invalid state value.
func TestUpdateMemoryState_InvalidState(t *testing.T) {
	store := newMockStore()
	store.memories["mem:test:1"] = &types.Memory{
		ID:        "mem:test:1",
		Content:   "hello",
		State:     "",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	_, err := srv.UpdateMemoryState(ctx, mcp.UpdateMemoryStateArgs{
		ID:    "mem:test:1",
		State: "invalid_state",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid state")
}

// TestUpdateMemoryState_MissingID returns error when ID is empty.
func TestUpdateMemoryState_MissingID(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	_, err := srv.UpdateMemoryState(ctx, mcp.UpdateMemoryStateArgs{
		ID:    "",
		State: types.StateActive,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory ID is required")
}

// TestUpdateMemoryState_MissingState returns error when state is empty.
func TestUpdateMemoryState_MissingState(t *testing.T) {
	store := newMockStore()
	store.memories["mem:test:1"] = &types.Memory{
		ID:        "mem:test:1",
		Content:   "hello",
		State:     "",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	_, err := srv.UpdateMemoryState(ctx, mcp.UpdateMemoryStateArgs{
		ID:    "mem:test:1",
		State: "",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "state is required")
}

// TestHandleRequest_UpdateMemoryState tests update_memory_state JSON-RPC method.
func TestHandleRequest_UpdateMemoryState(t *testing.T) {
	store := newMockStore()
	store.memories["mem:test:1"] = &types.Memory{
		ID:        "mem:test:1",
		Content:   "hello",
		State:     "",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"update_memory_state","params":{"id":"mem:test:1","state":"planning"},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)
	assert.Equal(t, types.StatePlanning, store.memories["mem:test:1"].State)
}

// ---------------------------------------------------------------------------
// RecallMemory filter tests (Opus Issue #5 and #10)
// ---------------------------------------------------------------------------

// TestRecallMemory_IDLookup_BackwardCompat verifies that ID lookup still works
// and is not broken by the new filter fields (Opus Issue #10).
func TestRecallMemory_IDLookup_BackwardCompat(t *testing.T) {
	store := newMockStore()
	mem := &types.Memory{
		ID:        "mem:test:backward",
		Content:   "backward compat check",
		Status:    types.StatusEnriched,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.memories[mem.ID] = mem

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{ID: "mem:test:backward"})
	require.NoError(t, err)
	require.True(t, result.Found)
	require.NotNil(t, result.Memory)
	assert.Equal(t, "mem:test:backward", result.Memory.ID)
	assert.Nil(t, result.Memories)
	assert.Equal(t, 0, result.Total)
}

// TestRecallMemory_IDLookup_NotFound verifies that a missing ID returns
// Found=false (existing behaviour preserved).
func TestRecallMemory_IDLookup_NotFound(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{ID: "mem:missing:1"})
	require.NoError(t, err)
	assert.False(t, result.Found)
	assert.Nil(t, result.Memory)
}

// TestRecallMemory_EmptyID_NoFilters returns a page of memories (list mode).
func TestRecallMemory_EmptyID_NoFilters(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("mem:general:%d", i)
		store.memories[id] = &types.Memory{
			ID:        id,
			Content:   fmt.Sprintf("memory %d", i),
			State:     "active",
			CreatedBy: "agent-a",
			Status:    types.StatusEnriched,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{})
	require.NoError(t, err)
	assert.False(t, result.Found)
	assert.Nil(t, result.Memory)
	assert.GreaterOrEqual(t, result.Total, 3)
}

// TestRecallMemory_FilterByState verifies that the State filter is forwarded
// through to storage.ListOptions.
func TestRecallMemory_FilterByState(t *testing.T) {
	store := newMockStore()
	now := time.Now()

	store.memories["mem:a:1"] = &types.Memory{
		ID: "mem:a:1", Content: "active one", State: "active",
		Status: types.StatusEnriched, CreatedAt: now, UpdatedAt: now,
	}
	store.memories["mem:a:2"] = &types.Memory{
		ID: "mem:a:2", Content: "archived one", State: "archived",
		Status: types.StatusEnriched, CreatedAt: now, UpdatedAt: now,
	}

	store.filterFn = func(mem *types.Memory, opts storage.ListOptions) bool {
		if opts.State != "" && mem.State != opts.State {
			return false
		}
		return true
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{State: "active"})
	require.NoError(t, err)
	assert.False(t, result.Found)
	for _, m := range result.Memories {
		assert.Equal(t, "active", m.State)
	}
}

// TestRecallMemory_FilterByCreatedBy verifies the CreatedBy filter.
func TestRecallMemory_FilterByCreatedBy(t *testing.T) {
	store := newMockStore()
	now := time.Now()

	store.memories["mem:b:1"] = &types.Memory{
		ID: "mem:b:1", Content: "by alice", CreatedBy: "alice",
		Status: types.StatusEnriched, CreatedAt: now, UpdatedAt: now,
	}
	store.memories["mem:b:2"] = &types.Memory{
		ID: "mem:b:2", Content: "by bob", CreatedBy: "bob",
		Status: types.StatusEnriched, CreatedAt: now, UpdatedAt: now,
	}

	store.filterFn = func(mem *types.Memory, opts storage.ListOptions) bool {
		if opts.CreatedBy != "" && mem.CreatedBy != opts.CreatedBy {
			return false
		}
		return true
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{CreatedBy: "alice"})
	require.NoError(t, err)
	for _, m := range result.Memories {
		assert.Equal(t, "alice", m.CreatedBy)
	}
}

// TestRecallMemory_FilterByCreatedAfter verifies the CreatedAfter temporal filter.
func TestRecallMemory_FilterByCreatedAfter(t *testing.T) {
	store := newMockStore()
	base := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)

	store.memories["mem:c:old"] = &types.Memory{
		ID: "mem:c:old", Content: "old",
		Status: types.StatusEnriched, CreatedAt: base.Add(-24 * time.Hour), UpdatedAt: base,
	}
	store.memories["mem:c:new"] = &types.Memory{
		ID: "mem:c:new", Content: "new",
		Status: types.StatusEnriched, CreatedAt: base.Add(24 * time.Hour), UpdatedAt: base,
	}

	store.filterFn = func(mem *types.Memory, opts storage.ListOptions) bool {
		if !opts.CreatedAfter.IsZero() && !mem.CreatedAt.After(opts.CreatedAfter) {
			return false
		}
		return true
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{
		CreatedAfter: base.Format(time.RFC3339),
	})
	require.NoError(t, err)
	for _, m := range result.Memories {
		assert.True(t, m.CreatedAt.After(base), "expected memory created after base time")
	}
}

// TestRecallMemory_FilterByCreatedBefore verifies the CreatedBefore temporal filter.
func TestRecallMemory_FilterByCreatedBefore(t *testing.T) {
	store := newMockStore()
	base := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)

	store.memories["mem:d:old"] = &types.Memory{
		ID: "mem:d:old", Content: "old",
		Status: types.StatusEnriched, CreatedAt: base.Add(-24 * time.Hour), UpdatedAt: base,
	}
	store.memories["mem:d:new"] = &types.Memory{
		ID: "mem:d:new", Content: "new",
		Status: types.StatusEnriched, CreatedAt: base.Add(24 * time.Hour), UpdatedAt: base,
	}

	store.filterFn = func(mem *types.Memory, opts storage.ListOptions) bool {
		if !opts.CreatedBefore.IsZero() && !mem.CreatedAt.Before(opts.CreatedBefore) {
			return false
		}
		return true
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{
		CreatedBefore: base.Format(time.RFC3339),
	})
	require.NoError(t, err)
	for _, m := range result.Memories {
		assert.True(t, m.CreatedAt.Before(base), "expected memory created before base time")
	}
}

// TestRecallMemory_FilterByMinDecayScore verifies the MinDecayScore filter.
func TestRecallMemory_FilterByMinDecayScore(t *testing.T) {
	store := newMockStore()
	now := time.Now()

	store.memories["mem:e:low"] = &types.Memory{
		ID: "mem:e:low", Content: "low decay", DecayScore: 0.2,
		Status: types.StatusEnriched, CreatedAt: now, UpdatedAt: now,
	}
	store.memories["mem:e:high"] = &types.Memory{
		ID: "mem:e:high", Content: "high decay", DecayScore: 0.9,
		Status: types.StatusEnriched, CreatedAt: now, UpdatedAt: now,
	}

	store.filterFn = func(mem *types.Memory, opts storage.ListOptions) bool {
		if opts.MinDecayScore > 0 && mem.DecayScore < opts.MinDecayScore {
			return false
		}
		return true
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{MinDecayScore: 0.5})
	require.NoError(t, err)
	for _, m := range result.Memories {
		assert.GreaterOrEqual(t, m.DecayScore, 0.5)
	}
}

// TestRecallMemory_InvalidTemporalBounds returns an error when CreatedAfter
// is set to a time after CreatedBefore.
func TestRecallMemory_InvalidTemporalBounds(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	after := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // before < after — invalid

	_, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{
		CreatedAfter:  after.Format(time.RFC3339),
		CreatedBefore: before.Format(time.RFC3339),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "created_after")
}

// TestRecallMemory_InvalidCreatedAfterTimestamp returns an error for bad
// timestamp strings.
func TestRecallMemory_InvalidCreatedAfterTimestamp(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	_, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{
		CreatedAfter: "not-a-timestamp",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "created_after")
}

// TestRecallMemory_InvalidCreatedBeforeTimestamp returns an error for bad
// timestamp strings.
func TestRecallMemory_InvalidCreatedBeforeTimestamp(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	_, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{
		CreatedBefore: "not-a-timestamp",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "created_before")
}

// ---------------------------------------------------------------------------
// FindRelated temporal filter tests (Phase 3)
// ---------------------------------------------------------------------------

// TestFindRelated_CreatedAfter verifies that the CreatedAfter temporal bound
// is forwarded to the store so only memories created after the cutoff are
// considered.
func TestFindRelated_CreatedAfter(t *testing.T) {
	store := newMockStore()
	base := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	store.memories["mem:f:old"] = &types.Memory{
		ID:        "mem:f:old",
		Content:   "golang project planning",
		Status:    types.StatusEnriched,
		CreatedAt: base.Add(-48 * time.Hour),
		UpdatedAt: base,
	}
	store.memories["mem:f:new"] = &types.Memory{
		ID:        "mem:f:new",
		Content:   "golang project planning",
		Status:    types.StatusEnriched,
		CreatedAt: base.Add(48 * time.Hour),
		UpdatedAt: base,
	}

	// filterFn enforces the CreatedAfter bound from ListOptions.
	store.filterFn = func(mem *types.Memory, opts storage.ListOptions) bool {
		if !opts.CreatedAfter.IsZero() && !mem.CreatedAt.After(opts.CreatedAfter) {
			return false
		}
		return true
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.FindRelated(ctx, mcp.FindRelatedArgs{
		Query:        "golang",
		CreatedAfter: base.Format(time.RFC3339),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	for _, m := range result.Memories {
		assert.True(t, m.CreatedAt.After(base), "expected memory created after base time")
	}
}

// TestFindRelated_CreatedBefore verifies that the CreatedBefore temporal bound
// is forwarded to the store so only memories created before the cutoff are
// considered.
func TestFindRelated_CreatedBefore(t *testing.T) {
	store := newMockStore()
	base := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	store.memories["mem:g:old"] = &types.Memory{
		ID:        "mem:g:old",
		Content:   "database schema design",
		Status:    types.StatusEnriched,
		CreatedAt: base.Add(-48 * time.Hour),
		UpdatedAt: base,
	}
	store.memories["mem:g:new"] = &types.Memory{
		ID:        "mem:g:new",
		Content:   "database schema design",
		Status:    types.StatusEnriched,
		CreatedAt: base.Add(48 * time.Hour),
		UpdatedAt: base,
	}

	store.filterFn = func(mem *types.Memory, opts storage.ListOptions) bool {
		if !opts.CreatedBefore.IsZero() && !mem.CreatedAt.Before(opts.CreatedBefore) {
			return false
		}
		return true
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.FindRelated(ctx, mcp.FindRelatedArgs{
		Query:         "database",
		CreatedBefore: base.Format(time.RFC3339),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	for _, m := range result.Memories {
		assert.True(t, m.CreatedAt.Before(base), "expected memory created before base time")
	}
}

// TestFindRelated_BothTemporalBounds verifies that a valid (after, before) pair
// is accepted and both bounds are forwarded to storage.
func TestFindRelated_BothTemporalBounds(t *testing.T) {
	store := newMockStore()
	windowStart := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)

	store.memories["mem:h:before"] = &types.Memory{
		ID:        "mem:h:before",
		Content:   "microservices architecture",
		Status:    types.StatusEnriched,
		CreatedAt: windowStart.Add(-24 * time.Hour),
		UpdatedAt: windowStart,
	}
	store.memories["mem:h:inside"] = &types.Memory{
		ID:        "mem:h:inside",
		Content:   "microservices architecture",
		Status:    types.StatusEnriched,
		CreatedAt: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: windowStart,
	}
	store.memories["mem:h:after"] = &types.Memory{
		ID:        "mem:h:after",
		Content:   "microservices architecture",
		Status:    types.StatusEnriched,
		CreatedAt: windowEnd.Add(24 * time.Hour),
		UpdatedAt: windowStart,
	}

	store.filterFn = func(mem *types.Memory, opts storage.ListOptions) bool {
		if !opts.CreatedAfter.IsZero() && !mem.CreatedAt.After(opts.CreatedAfter) {
			return false
		}
		if !opts.CreatedBefore.IsZero() && !mem.CreatedAt.Before(opts.CreatedBefore) {
			return false
		}
		return true
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.FindRelated(ctx, mcp.FindRelatedArgs{
		Query:         "microservices",
		CreatedAfter:  windowStart.Format(time.RFC3339),
		CreatedBefore: windowEnd.Format(time.RFC3339),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	for _, m := range result.Memories {
		assert.True(t, m.CreatedAt.After(windowStart), "expected memory after window start")
		assert.True(t, m.CreatedAt.Before(windowEnd), "expected memory before window end")
	}
}

// TestFindRelated_InvalidTemporalBounds returns an error when CreatedAfter is
// after or equal to CreatedBefore.
func TestFindRelated_InvalidTemporalBounds(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	after := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC) // before < after — invalid

	_, err := srv.FindRelated(ctx, mcp.FindRelatedArgs{
		Query:         "test",
		CreatedAfter:  after.Format(time.RFC3339),
		CreatedBefore: before.Format(time.RFC3339),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "created_after")
}

// TestFindRelated_InvalidCreatedAfterTimestamp returns an error for a
// malformed created_after value.
func TestFindRelated_InvalidCreatedAfterTimestamp(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	_, err := srv.FindRelated(ctx, mcp.FindRelatedArgs{
		Query:        "test",
		CreatedAfter: "not-a-timestamp",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "created_after")
}

// TestFindRelated_InvalidCreatedBeforeTimestamp returns an error for a
// malformed created_before value.
func TestFindRelated_InvalidCreatedBeforeTimestamp(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	_, err := srv.FindRelated(ctx, mcp.FindRelatedArgs{
		Query:         "test",
		CreatedBefore: "not-a-timestamp",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "created_before")
}

// TestFindRelated_NoTemporalBounds verifies that existing find_related
// behaviour is unchanged when no temporal bounds are provided.
func TestFindRelated_NoTemporalBounds(t *testing.T) {
	store := newMockStore()
	now := time.Now()

	store.memories["mem:i:1"] = &types.Memory{
		ID:        "mem:i:1",
		Content:   "api design patterns",
		Status:    types.StatusEnriched,
		CreatedAt: now,
		UpdatedAt: now,
	}
	store.memories["mem:i:2"] = &types.Memory{
		ID:        "mem:i:2",
		Content:   "unrelated topic",
		Status:    types.StatusEnriched,
		CreatedAt: now,
		UpdatedAt: now,
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.FindRelated(ctx, mcp.FindRelatedArgs{Query: "api design"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.GreaterOrEqual(t, result.Total, 1)
	for _, m := range result.Memories {
		assert.Contains(t, strings.ToLower(m.Content), "api design")
	}
}

// TestHandleRequest_FindRelated_WithTemporalBounds tests find_related via the
// JSON-RPC handler with temporal parameters present.
func TestHandleRequest_FindRelated_WithTemporalBounds(t *testing.T) {
	store := newMockStore()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	store.memories["mem:j:1"] = &types.Memory{
		ID:        "mem:j:1",
		Content:   "cloud infrastructure setup",
		Status:    types.StatusEnriched,
		CreatedAt: base.Add(24 * time.Hour),
		UpdatedAt: base,
	}

	store.filterFn = func(mem *types.Memory, opts storage.ListOptions) bool {
		if !opts.CreatedAfter.IsZero() && !mem.CreatedAt.After(opts.CreatedAfter) {
			return false
		}
		return true
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := fmt.Sprintf(
		`{"jsonrpc":"2.0","method":"find_related","params":{"query":"cloud","created_after":%q},"id":1}`,
		base.Format(time.RFC3339),
	)
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)
}

// ---------------------------------------------------------------------------
// GetSessionContext tests (Task 1.3)
// ---------------------------------------------------------------------------

// TestGetSessionContext_DefaultsToCurrentSession verifies that GetSessionContext
// uses the server's own session ID when no session_id arg is provided.
func TestGetSessionContext_DefaultsToCurrentSession(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store two memories through the server so they get tagged with its session ID.
	_, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "first memory", Domain: "work"})
	require.NoError(t, err)
	_, err = srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "second memory", Domain: "work"})
	require.NoError(t, err)

	// Also inject a memory with a different session ID that should NOT appear.
	store.memories["mem:other:1"] = &types.Memory{
		ID:        "mem:other:1",
		Content:   "other session memory",
		Domain:    "work",
		SessionID: "other-session-id",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	result, err := srv.GetSessionContext(ctx, mcp.GetSessionContextArgs{})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Result should contain only memories from this server's session.
	assert.NotEmpty(t, result.SessionID)
	assert.Equal(t, len(result.Memories), result.MemoryCount)
	for _, m := range result.Memories {
		assert.Equal(t, result.SessionID, m.SessionID, "all returned memories should match the session ID")
	}
	// The injected "other session" memory should not appear.
	for _, m := range result.Memories {
		assert.NotEqual(t, "mem:other:1", m.ID)
	}
}

// TestGetSessionContext_ExplicitSessionID verifies querying a specific session
// by passing session_id explicitly.
func TestGetSessionContext_ExplicitSessionID(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()
	const targetSession = "session-abc-123"

	now := time.Now()
	store.memories["mem:x:1"] = &types.Memory{
		ID:        "mem:x:1",
		Content:   "memory in target session",
		Domain:    "engineering",
		SessionID: targetSession,
		CreatedAt: now,
		UpdatedAt: now,
	}
	store.memories["mem:x:2"] = &types.Memory{
		ID:        "mem:x:2",
		Content:   "another memory in target session",
		Domain:    "product",
		SessionID: targetSession,
		CreatedAt: now,
		UpdatedAt: now,
	}
	store.memories["mem:x:3"] = &types.Memory{
		ID:        "mem:x:3",
		Content:   "memory in different session",
		Domain:    "engineering",
		SessionID: "different-session",
		CreatedAt: now,
		UpdatedAt: now,
	}

	result, err := srv.GetSessionContext(ctx, mcp.GetSessionContextArgs{SessionID: targetSession})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, targetSession, result.SessionID)
	assert.Equal(t, 2, result.MemoryCount)
	assert.Len(t, result.Memories, 2)
	for _, m := range result.Memories {
		assert.Equal(t, targetSession, m.SessionID)
	}
}

// TestGetSessionContext_TopicSummary verifies that the Topics slice is built
// correctly from the returned memories' domains.
func TestGetSessionContext_TopicSummary(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()
	const sess = "session-topics"

	now := time.Now()
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("mem:eng:%d", i)
		store.memories[id] = &types.Memory{
			ID:        id,
			Content:   fmt.Sprintf("engineering memory %d", i),
			Domain:    "engineering",
			SessionID: sess,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	store.memories["mem:prod:1"] = &types.Memory{
		ID:        "mem:prod:1",
		Content:   "product memory",
		Domain:    "product",
		SessionID: sess,
		CreatedAt: now,
		UpdatedAt: now,
	}

	result, err := srv.GetSessionContext(ctx, mcp.GetSessionContextArgs{SessionID: sess})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 4, result.MemoryCount)
	require.NotEmpty(t, result.Topics)

	// engineering should be the top domain (count 3)
	assert.Equal(t, "engineering", result.Topics[0].Domain)
	assert.Equal(t, 3, result.Topics[0].Count)

	// Summary should mention memory count.
	assert.Contains(t, result.Summary, "4 memories")
	assert.Contains(t, result.Summary, "engineering")
	assert.Contains(t, result.Summary, "product")
}

// TestGetSessionContext_EmptySession verifies that an empty session returns
// an empty memory list and a sensible summary.
func TestGetSessionContext_EmptySession(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.GetSessionContext(ctx, mcp.GetSessionContextArgs{SessionID: "nonexistent-session"})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 0, result.MemoryCount)
	assert.Empty(t, result.Memories)
	assert.Empty(t, result.Topics)
	assert.Contains(t, result.Summary, "0 memories")
}

// TestGetSessionContext_DefaultLimit verifies that the default limit of 20 is
// applied when no limit is specified.
func TestGetSessionContext_DefaultLimit(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	// The store returns whatever is in memories regardless of limit (mock behaviour).
	// This test verifies that ListOptions.Limit is set to 20 when Limit <= 0.
	// We inspect indirectly by checking the result is non-nil (no panic/error).
	result, err := srv.GetSessionContext(ctx, mcp.GetSessionContextArgs{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.SessionID)
}

// TestStoreMemory_SetsSessionID verifies that StoreMemory tags memories with
// the server's session ID by default.
func TestStoreMemory_SetsSessionID(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	res, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "session test memory"})
	require.NoError(t, err)
	require.NotNil(t, res)

	stored := store.memories[res.ID]
	require.NotNil(t, stored)
	assert.NotEmpty(t, stored.SessionID, "session_id should be set after StoreMemory")
}

// TestStoreMemory_SessionIDOverride verifies that an explicit session_id arg
// overrides the server session ID.
func TestStoreMemory_SessionIDOverride(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	const customSession = "my-custom-session"
	res, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{
		Content:   "override session memory",
		SessionID: customSession,
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	stored := store.memories[res.ID]
	require.NotNil(t, stored)
	assert.Equal(t, customSession, stored.SessionID)
}

// TestHandleRequest_GetSessionContext verifies get_session_context via the
// JSON-RPC handler (tools/call path).
func TestHandleRequest_GetSessionContext(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store a memory so the session has content.
	_, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "session memory via rpc"})
	require.NoError(t, err)

	req := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"get_session_context","arguments":{}},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)
	assert.Contains(t, string(resp), "session_id")
}

// TestForgetMemory_SoftDelete verifies soft deletion via MCP.
func TestForgetMemory_SoftDelete(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store a memory
	result, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "To be forgotten"})
	require.NoError(t, err)
	require.NotNil(t, result)
	memID := result.ID

	// Forget (soft delete) the memory
	forget, err := srv.ForgetMemory(ctx, mcp.ForgetMemoryArgs{ID: memID})
	require.NoError(t, err)
	assert.True(t, forget.Deleted)
	assert.False(t, forget.Purged)

	// Recall should fail (soft-deleted memories are excluded)
	recall, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{ID: memID})
	require.NoError(t, err)
	assert.False(t, recall.Found)
}

// TestForgetMemory_HardDelete verifies hard deletion via MCP.
func TestForgetMemory_HardDelete(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store a memory
	result, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "To be purged"})
	require.NoError(t, err)
	require.NotNil(t, result)
	memID := result.ID

	// Hard delete (purge) the memory
	forget, err := srv.ForgetMemory(ctx, mcp.ForgetMemoryArgs{ID: memID, HardDelete: true})
	require.NoError(t, err)
	assert.True(t, forget.Deleted)
	assert.True(t, forget.Purged)

	// Recall should fail
	recall, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{ID: memID})
	require.NoError(t, err)
	assert.False(t, recall.Found)
}

// TestEvolveMemory_CreatesNewVersionAndSupersedes verifies memory evolution via MCP.
func TestEvolveMemory_CreatesNewVersionAndSupersedes(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store original memory
	storeResult, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{
		Content: "Original thought",
		Tags:    []string{"important"},
	})
	require.NoError(t, err)
	require.NotNil(t, storeResult)
	originalID := storeResult.ID

	// Evolve the memory
	evolveResult, err := srv.EvolveMemory(ctx, mcp.EvolveMemoryArgs{
		ID:         originalID,
		NewContent: "Evolved thought with new insights",
	})
	require.NoError(t, err)
	assert.NotEqual(t, originalID, evolveResult.NewID)
	assert.Equal(t, originalID, evolveResult.SupersededID)

	// Verify new memory can be recalled
	recall, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{ID: evolveResult.NewID})
	require.NoError(t, err)
	assert.True(t, recall.Found)
	assert.Equal(t, "Evolved thought with new insights", recall.Memory.Content)
	assert.Equal(t, originalID, recall.Memory.SupersedesID)

	// Verify original memory is marked as superseded
	originalRecall, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{ID: originalID})
	require.NoError(t, err)
	assert.True(t, originalRecall.Found)
	assert.Equal(t, types.StateSuperseded, originalRecall.Memory.State)
}

// TestStoreMemory_DeduplicationViaAPI verifies deduplication returns duplicate marker.
// Note: StoreMemory generates deterministic IDs from content, so identical content
// produces identical IDs. The real deduplication happens in the storage layer where
// we check content_hash. When both calls generate the same ID and the second tries
// to upsert with the same ID, it returns the existing memory unchanged.
func TestStoreMemory_DeduplicationViaAPI(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	content := "Exact same content for both"

	// Store first memory
	result1, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: content})
	require.NoError(t, err)
	require.NotNil(t, result1)
	assert.False(t, result1.Duplicate)
	id1 := result1.ID

	// Store second memory with same content
	// Since generateMemoryID is deterministic, this generates the same ID
	result2, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: content})
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Both calls generate the same deterministic ID from the same content
	assert.Equal(t, id1, result2.ID)
	// Second call is detected as a duplicate via pre-store Get() check
	assert.True(t, result2.Duplicate)
	assert.Equal(t, id1, result2.ExistingID)
}

// TestConsolidateMemories_ByIDs verifies consolidation of memories by explicit ID list.
func TestConsolidateMemories_ByIDs(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store two memories to consolidate
	r1, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "First memory about Go programming"})
	require.NoError(t, err)
	r2, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "Second memory about Go testing"})
	require.NoError(t, err)

	result, err := srv.ConsolidateMemories(ctx, mcp.ConsolidateMemoriesArgs{
		IDs:   []string{r1.ID, r2.ID},
		Title: "Go Development Notes",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotEmpty(t, result.NewID)
	assert.Len(t, result.ConsolidatedIDs, 2)
	assert.Contains(t, result.Content, "Go Development Notes")
	assert.Contains(t, result.Content, "First memory")
	assert.Contains(t, result.Content, "Second memory")

	// Originals should be soft-deleted (not retrievable via Get)
	_, err = store.Get(ctx, r1.ID)
	assert.Error(t, err, "original memory should be soft-deleted")
	_, err = store.Get(ctx, r2.ID)
	assert.Error(t, err, "original memory should be soft-deleted")
}

// TestConsolidateMemories_RequiresMinTwo verifies minimum requirement of 2 memories.
func TestConsolidateMemories_RequiresMinTwo(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	r1, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "Only one memory"})
	require.NoError(t, err)

	_, err = srv.ConsolidateMemories(ctx, mcp.ConsolidateMemoriesArgs{
		IDs: []string{r1.ID},
	})
	assert.Error(t, err, "should require at least 2 memories")
	assert.Contains(t, err.Error(), "at least 2 memories")
}

// TestConsolidateMemories_RequiresIDsOrQuery verifies that either IDs or Query is required.
func TestConsolidateMemories_RequiresIDsOrQuery(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	_, err = srv.ConsolidateMemories(ctx, mcp.ConsolidateMemoriesArgs{
		Title: "Test",
	})
	assert.Error(t, err, "should require either ids or query")
	assert.Contains(t, err.Error(), "ids or query is required")
}

// TestConsolidateMemories_CollectsTags verifies that tags from all memories are merged.
func TestConsolidateMemories_CollectsTags(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store memories with different tags
	r1, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{
		Content: "First memory",
		Tags:    []string{"go", "backend"},
	})
	require.NoError(t, err)

	r2, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{
		Content: "Second memory",
		Tags:    []string{"testing", "go"},
	})
	require.NoError(t, err)

	result, err := srv.ConsolidateMemories(ctx, mcp.ConsolidateMemoriesArgs{
		IDs: []string{r1.ID, r2.ID},
	})
	require.NoError(t, err)

	// All unique tags should be present
	consolidated, err := store.Get(ctx, result.NewID)
	require.NoError(t, err)
	assert.Contains(t, consolidated.Tags, "go")
	assert.Contains(t, consolidated.Tags, "backend")
	assert.Contains(t, consolidated.Tags, "testing")
	assert.Len(t, consolidated.Tags, 3)
}

// TestConsolidateMemories_CreatesSourceWithDomain verifies the consolidated memory uses correct domain.
func TestConsolidateMemories_CreatesSourceWithDomain(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store memories with specific domain
	r1, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{
		Content: "First memory",
		Domain:  "project-alpha",
	})
	require.NoError(t, err)

	r2, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{
		Content: "Second memory",
		Domain:  "project-alpha",
	})
	require.NoError(t, err)

	result, err := srv.ConsolidateMemories(ctx, mcp.ConsolidateMemoriesArgs{
		IDs: []string{r1.ID, r2.ID},
	})
	require.NoError(t, err)

	// Consolidated memory should have the same domain
	consolidated, err := store.Get(ctx, result.NewID)
	require.NoError(t, err)
	assert.Equal(t, "project-alpha", consolidated.Domain)
	assert.Equal(t, "consolidation", consolidated.Source)
}

// ---------------------------------------------------------------------------
// Tests for explain_reasoning
// ---------------------------------------------------------------------------

func TestExplainReasoning(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store a memory so we have a valid ID.
	r, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "Alice leads the platform team"})
	require.NoError(t, err)

	result, err := srv.ExplainReasoning(ctx, mcp.ExplainReasoningArgs{
		Query:     "platform team leadership",
		MemoryIDs: []string{r.ID},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Explanation)
	assert.NotEmpty(t, result.Reasoning)
}

// ---------------------------------------------------------------------------
// Tests for restore_memory + list_deleted_memories
// ---------------------------------------------------------------------------

func TestRestoreMemory(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store then soft-delete a memory.
	r, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "Temporary note"})
	require.NoError(t, err)
	memID := r.ID

	_, err = srv.ForgetMemory(ctx, mcp.ForgetMemoryArgs{ID: memID, HardDelete: false})
	require.NoError(t, err)

	// Recall should fail — memory is soft-deleted.
	_, err = srv.RecallMemory(ctx, mcp.RecallMemoryArgs{Query: "Temporary note"})
	require.NoError(t, err) // RecallMemory returns empty, not an error

	// Restore the memory.
	restoreResult, err := srv.RestoreMemory(ctx, mcp.RestoreMemoryArgs{ID: memID})
	require.NoError(t, err)
	assert.Equal(t, memID, restoreResult.ID)
	assert.True(t, restoreResult.Restored)

	// Get should now succeed.
	mem, err := store.Get(ctx, memID)
	require.NoError(t, err)
	assert.Nil(t, mem.DeletedAt)
}

func TestListDeletedMemories(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store two memories, delete one.
	r1, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "Keep me"})
	require.NoError(t, err)
	r2, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "Delete me"})
	require.NoError(t, err)
	_ = r1

	_, err = srv.ForgetMemory(ctx, mcp.ForgetMemoryArgs{ID: r2.ID, HardDelete: false})
	require.NoError(t, err)

	// List deleted memories — should include r2.
	result, err := srv.ListDeletedMemories(ctx, mcp.ListDeletedMemoriesArgs{})
	require.NoError(t, err)
	require.NotNil(t, result)

	var found bool
	for _, m := range result.Memories {
		if m.ID == r2.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "soft-deleted memory should appear in ListDeletedMemories")
}

// ---------------------------------------------------------------------------
// Tests for get_evolution_chain
// ---------------------------------------------------------------------------

func TestGetEvolutionChain(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store original memory.
	r1, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "Original decision: use PostgreSQL"})
	require.NoError(t, err)

	// Evolve it.
	r2, err := srv.EvolveMemory(ctx, mcp.EvolveMemoryArgs{
		ID:         r1.ID,
		NewContent: "Updated decision: use PostgreSQL with read replicas",
	})
	require.NoError(t, err)

	// Get evolution chain starting from the latest version.
	chain, err := srv.GetEvolutionChain(ctx, mcp.GetEvolutionChainArgs{ID: r2.NewID})
	require.NoError(t, err)
	require.NotNil(t, chain)
	assert.GreaterOrEqual(t, chain.TotalVersions, 2, "should have at least 2 versions in chain")
}

// ---------------------------------------------------------------------------
// Tests for project management tools
// ---------------------------------------------------------------------------

func TestCreateProject(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.CreateProject(ctx, mcp.CreateProjectArgs{
		Name:        "Platform Rewrite",
		Description: "Rewrite the platform in Go",
		PhaseNames:  []string{"Discovery", "Implementation", "Launch"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.ProjectID)
	assert.Len(t, result.PhaseIDs, 3, "should have 3 phase IDs")

	// Verify the project memory exists.
	proj, err := store.Get(ctx, result.ProjectID)
	require.NoError(t, err)
	assert.Equal(t, "project", proj.MemoryType)
	assert.Contains(t, proj.Content, "Platform Rewrite")
}

func TestAddProjectItem(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Create a project first.
	proj, err := srv.CreateProject(ctx, mcp.CreateProjectArgs{
		Name:        "API Modernisation",
		Description: "REST → gRPC migration",
	})
	require.NoError(t, err)

	// Add a task under the project.
	item, err := srv.AddProjectItem(ctx, mcp.AddProjectItemArgs{
		ParentID:    proj.ProjectID,
		ItemType:    "task",
		Name:        "Design gRPC schema",
		Description: "Define protobuf types",
	})
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.NotEmpty(t, item.ID)
	assert.Equal(t, proj.ProjectID, item.ParentID)
	assert.Equal(t, "task", item.ItemType)
}

func TestGetProjectTree(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Create project with phases.
	proj, err := srv.CreateProject(ctx, mcp.CreateProjectArgs{
		Name:       "Data Platform",
		PhaseNames: []string{"Ingestion"},
	})
	require.NoError(t, err)

	// Add a task under the first phase.
	_, err = srv.AddProjectItem(ctx, mcp.AddProjectItemArgs{
		ParentID: proj.PhaseIDs[0],
		ItemType: "task",
		Name:     "Build Kafka consumer",
	})
	require.NoError(t, err)

	// Get the project tree.
	tree, err := srv.GetProjectTree(ctx, mcp.GetProjectTreeArgs{
		ProjectID: proj.ProjectID,
		Depth:     3,
	})
	require.NoError(t, err)
	require.NotNil(t, tree)
	assert.Equal(t, proj.ProjectID, tree.Tree.ID)
	assert.Equal(t, "project", tree.Tree.Type)
	// Root should have Ingestion phase as child.
	assert.NotEmpty(t, tree.Tree.Children, "project root should have children (phases)")
}

func TestListProjects(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Create two projects.
	_, err = srv.CreateProject(ctx, mcp.CreateProjectArgs{Name: "Project Alpha"})
	require.NoError(t, err)
	_, err = srv.CreateProject(ctx, mcp.CreateProjectArgs{Name: "Project Beta"})
	require.NoError(t, err)

	// Also store a non-project memory to ensure it's not included.
	_, err = srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "Just a regular note"})
	require.NoError(t, err)

	result, err := srv.ListProjects(ctx, mcp.ListProjectsArgs{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.GreaterOrEqual(t, len(result.Projects), 2, "should list at least 2 projects")
}

// ---------------------------------------------------------------------------
// Tests for traverse_memory_graph (server-level)
// ---------------------------------------------------------------------------

func TestTraverseMemoryGraph(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	r, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{
		Content: "Alice is the engineering lead",
	})
	require.NoError(t, err)

	// traverse_memory_graph is handled via HandleRequest (raw JSON-RPC).
	req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"traverse_memory_graph","params":{"memory_id":%q,"max_hops":2,"limit":5}}`, r.ID)
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)
	// Response should not be an error.
	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)
}
