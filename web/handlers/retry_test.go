package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockMaintenanceEngine implements MaintenanceEngine for testing.
type MockMaintenanceEngine struct {
	mock.Mock
}

func (m *MockMaintenanceEngine) QueueEnrichmentForMemory(memoryID, content string) bool {
	args := m.Called(memoryID, content)
	return args.Bool(0)
}

func (m *MockMaintenanceEngine) QueueEmbeddingForMemory(memoryID, content string) bool {
	args := m.Called(memoryID, content)
	return args.Bool(0)
}

func (m *MockMaintenanceEngine) GetQueueSize() int {
	args := m.Called()
	return args.Int(0)
}

func TestRetryEnrichment_FailedMemory(t *testing.T) {
	mockStore := new(MockMemoryStore)
	mockEngine := new(MockMaintenanceEngine)

	failedMemory := &types.Memory{
		ID:      "mem:test:failed1",
		Content: "test content for retry",
		Status:  types.StatusFailed,
	}

	mockStore.On("Get", mock.Anything, "mem:test:failed1").Return(failedMemory, nil)
	mockStore.On("Update", mock.Anything, mock.MatchedBy(func(m *types.Memory) bool {
		return m.ID == "mem:test:failed1" && m.Status == types.StatusPending
	})).Return(nil)
	mockEngine.On("QueueEnrichmentForMemory", "mem:test:failed1", "test content for retry").Return(true)

	connMgr := connections.NewManagerWithStore(mockStore, "default")
	handler := NewMaintenanceHandler(connMgr, mockEngine)

	req := httptest.NewRequest(http.MethodPost, "/api/memories/mem:test:failed1/retry", nil)
	req.SetPathValue("id", "mem:test:failed1")
	w := httptest.NewRecorder()

	handler.RetryEnrichment(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "mem:test:failed1", resp["id"])
	assert.Equal(t, true, resp["queued"])
	assert.Equal(t, "Memory queued for enrichment retry", resp["message"])

	mockStore.AssertExpectations(t)
	mockEngine.AssertExpectations(t)
}

func TestRetryEnrichment_NotFailed(t *testing.T) {
	mockStore := new(MockMemoryStore)

	enrichedMemory := &types.Memory{
		ID:      "mem:test:enriched1",
		Content: "already enriched",
		Status:  types.StatusEnriched,
	}

	mockStore.On("Get", mock.Anything, "mem:test:enriched1").Return(enrichedMemory, nil)

	connMgr := connections.NewManagerWithStore(mockStore, "default")
	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/memories/mem:test:enriched1/retry", nil)
	req.SetPathValue("id", "mem:test:enriched1")
	w := httptest.NewRecorder()

	handler.RetryEnrichment(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, false, resp["queued"])
	assert.Contains(t, resp["message"], "not in failed state")

	mockStore.AssertExpectations(t)
}

func TestRetryEnrichment_NotFound(t *testing.T) {
	mockStore := new(MockMemoryStore)

	mockStore.On("Get", mock.Anything, "mem:test:missing").Return(nil, storage.ErrNotFound)

	connMgr := connections.NewManagerWithStore(mockStore, "default")
	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/memories/mem:test:missing/retry", nil)
	req.SetPathValue("id", "mem:test:missing")
	w := httptest.NewRecorder()

	handler.RetryEnrichment(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	mockStore.AssertExpectations(t)
}

func TestRetryEnrichment_WrongMethod(t *testing.T) {
	connMgr := connections.NewManagerWithStore(new(MockMemoryStore), "default")
	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/memories/mem:test:1/retry", nil)
	req.SetPathValue("id", "mem:test:1")
	w := httptest.NewRecorder()

	handler.RetryEnrichment(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestRetryEnrichment_MissingID(t *testing.T) {
	connMgr := connections.NewManagerWithStore(new(MockMemoryStore), "default")
	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/memories//retry", nil)
	// No path value set
	w := httptest.NewRecorder()

	handler.RetryEnrichment(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
