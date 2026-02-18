package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockMemoryStore is a mock implementation of storage.MemoryStore for testing.
type MockMemoryStore struct {
	mock.Mock
}

func (m *MockMemoryStore) Store(ctx context.Context, memory *types.Memory) error {
	args := m.Called(ctx, memory)
	return args.Error(0)
}

func (m *MockMemoryStore) Get(ctx context.Context, id string) (*types.Memory, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Memory), args.Error(1)
}

func (m *MockMemoryStore) List(ctx context.Context, opts storage.ListOptions) (*storage.PaginatedResult[types.Memory], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.PaginatedResult[types.Memory]), args.Error(1)
}

func (m *MockMemoryStore) Update(ctx context.Context, memory *types.Memory) error {
	args := m.Called(ctx, memory)
	return args.Error(0)
}

func (m *MockMemoryStore) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockMemoryStore) Purge(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockMemoryStore) UpdateStatus(ctx context.Context, id string, status types.MemoryStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockMemoryStore) UpdateEnrichment(ctx context.Context, id string, enrichment storage.EnrichmentUpdate) error {
	args := m.Called(ctx, id, enrichment)
	return args.Error(0)
}

func (m *MockMemoryStore) UpdateState(ctx context.Context, id string, state string) error {
	args := m.Called(ctx, id, state)
	return args.Error(0)
}

func (m *MockMemoryStore) IncrementAccessCount(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockMemoryStore) UpdateDecayScores(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

func (m *MockMemoryStore) GetRelatedMemories(ctx context.Context, memoryID string) ([]string, error) {
	args := m.Called(ctx, memoryID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockMemoryStore) Traverse(ctx context.Context, startMemoryID string, maxHops int, limit int) ([]storage.TraversalResult, error) {
	args := m.Called(ctx, startMemoryID, maxHops, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.TraversalResult), args.Error(1)
}

func (m *MockMemoryStore) GetMemoryEntities(ctx context.Context, memoryID string) ([]*types.Entity, error) {
	args := m.Called(ctx, memoryID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Entity), args.Error(1)
}

func (m *MockMemoryStore) Restore(_ context.Context, _ string) error { return nil }

func (m *MockMemoryStore) GetEvolutionChain(_ context.Context, _ string) ([]*types.Memory, error) {
	return nil, nil
}

func (m *MockMemoryStore) GetMemoriesByRelationType(_ context.Context, _ string, _ string) ([]*types.Memory, error) {
	return nil, nil
}

func (m *MockMemoryStore) Close() error {
	args := m.Called()
	return args.Error(0)
}

// TestAPIHandlers_ListMemories tests the ListMemories endpoint with pagination and filtering.
func TestAPIHandlers_ListMemories(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(*MockMemoryStore)
		expectedStatus int
		validateBody   func(*testing.T, []byte)
	}{
		{
			name:        "successful list with defaults",
			queryParams: "",
			mockSetup: func(m *MockMemoryStore) {
				m.On("List", mock.Anything, mock.MatchedBy(func(opts storage.ListOptions) bool {
					return opts.Page == 1 && opts.Limit == 10
				})).Return(&storage.PaginatedResult[types.Memory]{
					Items:    []types.Memory{{ID: "mem:test:1", Content: "Test memory"}},
					Total:    1,
					Page:     1,
					PageSize: 10,
					HasMore:  false,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var result storage.PaginatedResult[types.Memory]
				assert.NoError(t, json.Unmarshal(body, &result))
				assert.Equal(t, 1, result.Total)
				assert.Len(t, result.Items, 1)
				assert.Equal(t, "mem:test:1", result.Items[0].ID)
			},
		},
		{
			name:        "successful list with pagination",
			queryParams: "?page=2&limit=20",
			mockSetup: func(m *MockMemoryStore) {
				m.On("List", mock.Anything, mock.MatchedBy(func(opts storage.ListOptions) bool {
					return opts.Page == 2 && opts.Limit == 20
				})).Return(&storage.PaginatedResult[types.Memory]{
					Items:    []types.Memory{{ID: "mem:test:2", Content: "Page 2"}},
					Total:    25,
					Page:     2,
					PageSize: 20,
					HasMore:  true,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var result storage.PaginatedResult[types.Memory]
				assert.NoError(t, json.Unmarshal(body, &result))
				assert.Equal(t, 25, result.Total)
				assert.True(t, result.HasMore)
			},
		},
		{
			name:        "filter by status",
			queryParams: "?status=enriched",
			mockSetup: func(m *MockMemoryStore) {
				m.On("List", mock.Anything, mock.MatchedBy(func(opts storage.ListOptions) bool {
					status, ok := opts.Filter["status"]
					return ok && status == "enriched"
				})).Return(&storage.PaginatedResult[types.Memory]{
					Items:    []types.Memory{{ID: "mem:test:3", Status: types.StatusEnriched}},
					Total:    1,
					Page:     1,
					PageSize: 10,
					HasMore:  false,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var result storage.PaginatedResult[types.Memory]
				assert.NoError(t, json.Unmarshal(body, &result))
				assert.Equal(t, types.StatusEnriched, result.Items[0].Status)
			},
		},
		{
			name:        "filter by domain",
			queryParams: "?domain=work",
			mockSetup: func(m *MockMemoryStore) {
				m.On("List", mock.Anything, mock.MatchedBy(func(opts storage.ListOptions) bool {
					domain, ok := opts.Filter["domain"]
					return ok && domain == "work"
				})).Return(&storage.PaginatedResult[types.Memory]{
					Items:    []types.Memory{{ID: "mem:work:1", Domain: "work"}},
					Total:    1,
					Page:     1,
					PageSize: 10,
					HasMore:  false,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var result storage.PaginatedResult[types.Memory]
				assert.NoError(t, json.Unmarshal(body, &result))
				assert.Equal(t, "work", result.Items[0].Domain)
			},
		},
		{
			name:        "invalid page parameter",
			queryParams: "?page=invalid",
			mockSetup: func(m *MockMemoryStore) {
				// Should still work with default page=1
				m.On("List", mock.Anything, mock.Anything).Return(&storage.PaginatedResult[types.Memory]{
					Items:    []types.Memory{},
					Total:    0,
					Page:     1,
					PageSize: 10,
					HasMore:  false,
				}, nil)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockMemoryStore)
			tt.mockSetup(mockStore)

			cfg := &config.Config{}
			handlers := NewAPIHandlers(mockStore, cfg)

			req := httptest.NewRequest(http.MethodGet, "/api/memories"+tt.queryParams, nil)
			rec := httptest.NewRecorder()

			handlers.ListMemories(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, rec.Body.Bytes())
			}
			mockStore.AssertExpectations(t)
		})
	}
}

// TestAPIHandlers_GetMemory tests the GetMemory endpoint.
func TestAPIHandlers_GetMemory(t *testing.T) {
	now := time.Now()
	testMemory := &types.Memory{
		ID:        "mem:test:123",
		Content:   "Test content",
		Source:    "manual",
		Domain:    "test",
		Status:    types.StatusEnriched,
		CreatedAt: now,
		UpdatedAt: now,
	}

	tests := []struct {
		name           string
		memoryID       string
		mockSetup      func(*MockMemoryStore)
		expectedStatus int
		validateBody   func(*testing.T, []byte)
	}{
		{
			name:     "successful get",
			memoryID: "mem:test:123",
			mockSetup: func(m *MockMemoryStore) {
				m.On("Get", mock.Anything, "mem:test:123").Return(testMemory, nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var memory types.Memory
				assert.NoError(t, json.Unmarshal(body, &memory))
				assert.Equal(t, "mem:test:123", memory.ID)
				assert.Equal(t, "Test content", memory.Content)
			},
		},
		{
			name:     "memory not found",
			memoryID: "mem:test:999",
			mockSetup: func(m *MockMemoryStore) {
				m.On("Get", mock.Anything, "mem:test:999").Return(nil, storage.ErrNotFound)
			},
			expectedStatus: http.StatusNotFound,
			validateBody: func(t *testing.T, body []byte) {
				var err ErrorResponse
				assert.NoError(t, json.Unmarshal(body, &err))
				assert.Contains(t, err.Error, "not found")
			},
		},
		{
			name:     "missing memory ID",
			memoryID: "",
			mockSetup: func(m *MockMemoryStore) {
				// No mock setup needed - should fail validation
			},
			expectedStatus: http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var err ErrorResponse
				assert.NoError(t, json.Unmarshal(body, &err))
				assert.Contains(t, err.Error, "memory ID")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockMemoryStore)
			tt.mockSetup(mockStore)

			cfg := &config.Config{}
			handlers := NewAPIHandlers(mockStore, cfg)

			req := httptest.NewRequest(http.MethodGet, "/api/memories/"+tt.memoryID, nil)
			req.SetPathValue("id", tt.memoryID)
			rec := httptest.NewRecorder()

			handlers.GetMemory(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, rec.Body.Bytes())
			}
			mockStore.AssertExpectations(t)
		})
	}
}

// TestAPIHandlers_CreateMemory tests the CreateMemory endpoint.
func TestAPIHandlers_CreateMemory(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    interface{}
		mockSetup      func(*MockMemoryStore)
		expectedStatus int
		validateBody   func(*testing.T, []byte)
	}{
		{
			name: "successful create",
			requestBody: map[string]interface{}{
				"content": "New memory content",
				"source":  "manual",
				"domain":  "test",
				"tags":    []string{"test", "new"},
			},
			mockSetup: func(m *MockMemoryStore) {
				m.On("Store", mock.Anything, mock.MatchedBy(func(mem *types.Memory) bool {
					return mem.Content == "New memory content" &&
						mem.Status == types.StatusPending &&
						mem.EntityStatus == types.EnrichmentPending
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body []byte) {
				var memory types.Memory
				assert.NoError(t, json.Unmarshal(body, &memory))
				assert.Equal(t, "New memory content", memory.Content)
				assert.Equal(t, types.StatusPending, memory.Status)
				assert.NotEmpty(t, memory.ID)
			},
		},
		{
			name: "create with metadata",
			requestBody: map[string]interface{}{
				"content": "Memory with metadata",
				"source":  "api",
				"metadata": map[string]interface{}{
					"key1": "value1",
					"key2": 123,
				},
			},
			mockSetup: func(m *MockMemoryStore) {
				m.On("Store", mock.Anything, mock.MatchedBy(func(mem *types.Memory) bool {
					return mem.Metadata != nil && mem.Metadata["key1"] == "value1"
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body []byte) {
				var memory types.Memory
				assert.NoError(t, json.Unmarshal(body, &memory))
				assert.NotNil(t, memory.Metadata)
				assert.Equal(t, "value1", memory.Metadata["key1"])
			},
		},
		{
			name:        "missing content",
			requestBody: map[string]interface{}{"source": "manual"},
			mockSetup: func(m *MockMemoryStore) {
				// No mock setup - should fail validation
			},
			expectedStatus: http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var err ErrorResponse
				assert.NoError(t, json.Unmarshal(body, &err))
				assert.Contains(t, err.Error, "content")
			},
		},
		{
			name:        "invalid JSON",
			requestBody: "invalid json",
			mockSetup: func(m *MockMemoryStore) {
				// No mock setup - should fail parsing
			},
			expectedStatus: http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var err ErrorResponse
				assert.NoError(t, json.Unmarshal(body, &err))
				assert.Contains(t, err.Error, "parse")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockMemoryStore)
			tt.mockSetup(mockStore)

			cfg := &config.Config{}
			handlers := NewAPIHandlers(mockStore, cfg)

			var body []byte
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, _ = json.Marshal(tt.requestBody)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/memories", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handlers.CreateMemory(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, rec.Body.Bytes())
			}
			mockStore.AssertExpectations(t)
		})
	}
}

// TestAPIHandlers_UpdateMemory tests the UpdateMemory endpoint.
func TestAPIHandlers_UpdateMemory(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		memoryID       string
		requestBody    interface{}
		mockSetup      func(*MockMemoryStore)
		expectedStatus int
		validateBody   func(*testing.T, []byte)
	}{
		{
			name:     "successful full update",
			memoryID: "mem:test:123",
			requestBody: map[string]interface{}{
				"content": "Updated content",
				"tags":    []string{"updated"},
			},
			mockSetup: func(m *MockMemoryStore) {
				existingMemory := &types.Memory{
					ID:        "mem:test:123",
					Content:   "Original content",
					Source:    "manual",
					Status:    types.StatusEnriched,
					CreatedAt: now.Add(-1 * time.Hour),
					UpdatedAt: now.Add(-1 * time.Hour),
				}
				m.On("Get", mock.Anything, "mem:test:123").Return(existingMemory, nil)
				m.On("Update", mock.Anything, mock.MatchedBy(func(mem *types.Memory) bool {
					return mem.ID == "mem:test:123" && mem.Content == "Updated content"
				})).Return(nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var memory types.Memory
				assert.NoError(t, json.Unmarshal(body, &memory))
				assert.Equal(t, "Updated content", memory.Content)
			},
		},
		{
			name:     "partial update - tags only",
			memoryID: "mem:test:123",
			requestBody: map[string]interface{}{
				"tags": []string{"new-tag"},
			},
			mockSetup: func(m *MockMemoryStore) {
				existingMemory := &types.Memory{
					ID:        "mem:test:123",
					Content:   "Original content",
					Source:    "manual",
					Status:    types.StatusEnriched,
					CreatedAt: now.Add(-1 * time.Hour),
					UpdatedAt: now.Add(-1 * time.Hour),
				}
				m.On("Get", mock.Anything, "mem:test:123").Return(existingMemory, nil)
				m.On("Update", mock.Anything, mock.MatchedBy(func(mem *types.Memory) bool {
					return mem.ID == "mem:test:123" &&
						mem.Content == "Original content" && // Content unchanged
						len(mem.Tags) == 1 && mem.Tags[0] == "new-tag"
				})).Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "memory not found",
			memoryID: "mem:test:999",
			requestBody: map[string]interface{}{
				"content": "Updated content",
			},
			mockSetup: func(m *MockMemoryStore) {
				m.On("Get", mock.Anything, "mem:test:999").Return(nil, storage.ErrNotFound)
			},
			expectedStatus: http.StatusNotFound,
			validateBody: func(t *testing.T, body []byte) {
				var err ErrorResponse
				assert.NoError(t, json.Unmarshal(body, &err))
				assert.Contains(t, err.Error, "not found")
			},
		},
		{
			name:        "missing memory ID",
			memoryID:    "",
			requestBody: map[string]interface{}{"content": "Updated"},
			mockSetup: func(m *MockMemoryStore) {
				// No mock setup - should fail validation
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "invalid JSON",
			memoryID:    "mem:test:123",
			requestBody: "invalid",
			mockSetup: func(m *MockMemoryStore) {
				// No mock setup - should fail parsing
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockMemoryStore)
			tt.mockSetup(mockStore)

			cfg := &config.Config{}
			handlers := NewAPIHandlers(mockStore, cfg)

			var body []byte
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, _ = json.Marshal(tt.requestBody)
			}

			req := httptest.NewRequest(http.MethodPut, "/api/memories/"+tt.memoryID, bytes.NewReader(body))
			req.SetPathValue("id", tt.memoryID)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handlers.UpdateMemory(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, rec.Body.Bytes())
			}
			mockStore.AssertExpectations(t)
		})
	}
}

// TestAPIHandlers_DeleteMemory tests the DeleteMemory endpoint.
func TestAPIHandlers_DeleteMemory(t *testing.T) {
	tests := []struct {
		name           string
		memoryID       string
		mockSetup      func(*MockMemoryStore)
		expectedStatus int
		validateBody   func(*testing.T, []byte)
	}{
		{
			name:     "successful delete",
			memoryID: "mem:test:123",
			mockSetup: func(m *MockMemoryStore) {
				m.On("Delete", mock.Anything, "mem:test:123").Return(nil)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:     "memory not found",
			memoryID: "mem:test:999",
			mockSetup: func(m *MockMemoryStore) {
				m.On("Delete", mock.Anything, "mem:test:999").Return(storage.ErrNotFound)
			},
			expectedStatus: http.StatusNotFound,
			validateBody: func(t *testing.T, body []byte) {
				var err ErrorResponse
				assert.NoError(t, json.Unmarshal(body, &err))
				assert.Contains(t, err.Error, "not found")
			},
		},
		{
			name:     "missing memory ID",
			memoryID: "",
			mockSetup: func(m *MockMemoryStore) {
				// No mock setup - should fail validation
			},
			expectedStatus: http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var err ErrorResponse
				assert.NoError(t, json.Unmarshal(body, &err))
				assert.Contains(t, err.Error, "memory ID")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockMemoryStore)
			tt.mockSetup(mockStore)

			cfg := &config.Config{}
			handlers := NewAPIHandlers(mockStore, cfg)

			req := httptest.NewRequest(http.MethodDelete, "/api/memories/"+tt.memoryID, nil)
			req.SetPathValue("id", tt.memoryID)
			rec := httptest.NewRecorder()

			handlers.DeleteMemory(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, rec.Body.Bytes())
			}
			mockStore.AssertExpectations(t)
		})
	}
}
