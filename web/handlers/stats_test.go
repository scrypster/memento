package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
	"github.com/stretchr/testify/mock"
)

// mockMemoryStore implements storage.MemoryStore for testing.
type mockMemoryStoreForStats struct {
	memories []*types.Memory
	listErr  error
}

func (m *mockMemoryStoreForStats) Store(ctx context.Context, memory *types.Memory) error {
	return nil
}

func (m *mockMemoryStoreForStats) Get(ctx context.Context, id string) (*types.Memory, error) {
	return nil, storage.ErrNotFound
}

func (m *mockMemoryStoreForStats) List(ctx context.Context, opts storage.ListOptions) (*storage.PaginatedResult[types.Memory], error) {
	if m.listErr != nil {
		return nil, m.listErr
	}

	// Convert []*types.Memory to []types.Memory
	items := make([]types.Memory, len(m.memories))
	for i, mem := range m.memories {
		items[i] = *mem
	}

	return &storage.PaginatedResult[types.Memory]{
		Items:    items,
		Total:    len(m.memories),
		Page:     opts.Page,
		PageSize: opts.Limit,
		HasMore:  false,
	}, nil
}

func (m *mockMemoryStoreForStats) Update(ctx context.Context, memory *types.Memory) error {
	return nil
}

func (m *mockMemoryStoreForStats) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockMemoryStoreForStats) Purge(ctx context.Context, id string) error {
	return nil
}

func (m *mockMemoryStoreForStats) UpdateStatus(ctx context.Context, id string, status types.MemoryStatus) error {
	return nil
}

func (m *mockMemoryStoreForStats) UpdateEnrichment(ctx context.Context, id string, enrichment storage.EnrichmentUpdate) error {
	return nil
}

func (m *mockMemoryStoreForStats) UpdateState(ctx context.Context, id string, state string) error {
	return nil
}

func (m *mockMemoryStoreForStats) IncrementAccessCount(ctx context.Context, id string) error {
	return nil
}

func (m *mockMemoryStoreForStats) UpdateDecayScores(ctx context.Context) (int, error) {
	return 0, nil
}

func (m *mockMemoryStoreForStats) GetRelatedMemories(ctx context.Context, memoryID string) ([]string, error) {
	return nil, nil
}

func (m *mockMemoryStoreForStats) Traverse(_ context.Context, _ string, _ int, _ int) ([]storage.TraversalResult, error) {
	return nil, nil
}

func (m *mockMemoryStoreForStats) GetMemoryEntities(_ context.Context, _ string) ([]*types.Entity, error) {
	return nil, nil
}

func (m *mockMemoryStoreForStats) Restore(_ context.Context, _ string) error { return nil }

func (m *mockMemoryStoreForStats) GetEvolutionChain(_ context.Context, _ string) ([]*types.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForStats) GetMemoriesByRelationType(_ context.Context, _ string, _ string) ([]*types.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForStats) Close() error {
	return nil
}

// MockQueueSizeGetter mocks the QueueSizeGetter interface
type MockQueueSizeGetter struct {
	mock.Mock
}

func (m *MockQueueSizeGetter) GetQueueSize() int {
	args := m.Called()
	return args.Int(0)
}

func TestGetStats_Success(t *testing.T) {
	// Create mock store with 5 memories
	mockStore := &mockMemoryStoreForStats{
		memories: []*types.Memory{
			{ID: "mem:test:1", Content: "test 1", CreatedAt: time.Now()},
			{ID: "mem:test:2", Content: "test 2", CreatedAt: time.Now()},
			{ID: "mem:test:3", Content: "test 3", CreatedAt: time.Now()},
			{ID: "mem:test:4", Content: "test 4", CreatedAt: time.Now()},
			{ID: "mem:test:5", Content: "test 5", CreatedAt: time.Now()},
		},
	}

	// Create mocks
	connMgr := connections.NewManagerWithStore(mockStore, "test-connection")
	mockQueueGetter := new(MockQueueSizeGetter)
	mockQueueGetter.On("GetQueueSize").Return(0)

	// Create handler
	handler := NewStatsHandler(mockStore, connMgr, mockQueueGetter)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()

	// Execute
	handler.GetStats(w, req)

	// Assert response code
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Assert response body
	var response StatsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Memories != 5 {
		t.Errorf("Expected 5 memories, got %d", response.Memories)
	}

	// Entities, relationships, and queue size should be 0 for now
	if response.Entities != 0 {
		t.Errorf("Expected 0 entities, got %d", response.Entities)
	}
	if response.Relationships != 0 {
		t.Errorf("Expected 0 relationships, got %d", response.Relationships)
	}
	if response.QueueSize != 0 {
		t.Errorf("Expected 0 queue size, got %d", response.QueueSize)
	}
}

func TestGetStats_EmptyDatabase(t *testing.T) {
	// Create mock store with no memories
	mockStore := &mockMemoryStoreForStats{
		memories: []*types.Memory{},
	}

	// Create mocks
	connMgr := connections.NewManagerWithStore(mockStore, "test-connection")
	mockQueueGetter := new(MockQueueSizeGetter)
	mockQueueGetter.On("GetQueueSize").Return(0)

	// Create handler
	handler := NewStatsHandler(mockStore, connMgr, mockQueueGetter)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()

	// Execute
	handler.GetStats(w, req)

	// Assert response code
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Assert response body
	var response StatsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Memories != 0 {
		t.Errorf("Expected 0 memories, got %d", response.Memories)
	}
}

func TestGetStats_StoreError(t *testing.T) {
	// Create mock store that returns error
	mockStore := &mockMemoryStoreForStats{
		listErr: storage.ErrInvalidInput,
	}

	// Create mocks
	connMgr := connections.NewManagerWithStore(mockStore, "test-connection")
	mockQueueGetter := new(MockQueueSizeGetter)
	mockQueueGetter.On("GetQueueSize").Return(0)

	// Create handler
	handler := NewStatsHandler(mockStore, connMgr, mockQueueGetter)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()

	// Execute
	handler.GetStats(w, req)

	// Assert response code
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	// Assert error response
	var response ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if response.Error != "failed to count memories" {
		t.Errorf("Expected error message 'failed to count memories', got '%s'", response.Error)
	}
}
