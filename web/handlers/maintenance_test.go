package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SimpleMaintenanceEngine is a simple (non-mock) engine for maintenance tests.
type SimpleMaintenanceEngine struct {
	enrichmentQueue int
	embeddingQueue  int
	queueSize       int
	failEnrichment  bool
	failEmbedding   bool
}

func (m *SimpleMaintenanceEngine) QueueEnrichmentForMemory(memoryID, content string) bool {
	if m.failEnrichment {
		return false
	}
	m.enrichmentQueue++
	m.queueSize++
	return true
}

func (m *SimpleMaintenanceEngine) QueueEmbeddingForMemory(memoryID, content string) bool {
	if m.failEmbedding {
		return false
	}
	m.embeddingQueue++
	m.queueSize++
	return true
}

func (m *SimpleMaintenanceEngine) GetQueueSize() int {
	return m.queueSize
}

// setupMaintenanceTest creates a temporary SQLite database and connection manager.
func setupMaintenanceTest(t *testing.T) (*connections.Manager, *sqlite.MemoryStore) {
	tmpDir := t.TempDir()
	store, err := sqlite.NewMemoryStore(tmpDir + "/test.db")
	require.NoError(t, err)

	// Create connection manager
	connMgr := connections.NewManagerWithStore(store, "default")

	return connMgr, store
}

// TestGetStatus_Success tests successful retrieval of maintenance status.
func TestGetStatus_Success(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	// Add a memory with pending status
	ctx := context.Background()
	mem := &types.Memory{
		ID:      "mem:test:123",
		Content: "test content",
		Status:  types.StatusPending,
	}
	err := store.Store(ctx, mem)
	require.NoError(t, err)

	// Create request with path parameter
	req := httptest.NewRequest(http.MethodGet, "/api/connections/default/maintenance", nil)
	req.SetPathValue("name", "default")
	w := httptest.NewRecorder()

	// Execute
	handler.GetStatus(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response MaintenanceStatus
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, 1, response.NeedsEnrichment)
	assert.Equal(t, 0, response.MissingEmbeddings)
	assert.IsType(t, []ModelCount{}, response.StoredModels)
}

// TestGetStatus_MissingConnectionName tests GET status without connection name.
func TestGetStatus_MissingConnectionName(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/connections//maintenance", nil)
	req.SetPathValue("name", "") // Empty path value
	w := httptest.NewRecorder()

	handler.GetStatus(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "connection name is required")
}

// TestGetStatus_ConnectionNotFound tests GET status with non-existent connection.
func TestGetStatus_ConnectionNotFound(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/connections/nonexistent/maintenance", nil)
	req.SetPathValue("name", "nonexistent")
	w := httptest.NewRecorder()

	handler.GetStatus(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "connection not found")
}

// TestGetStatus_WithEnrichedMemoriesAndMissingEmbeddings tests status with enriched memories.
func TestGetStatus_WithEnrichedMemoriesAndMissingEmbeddings(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	// Add enriched memories
	ctx := context.Background()
	mem1 := &types.Memory{
		ID:      "mem:test:e1",
		Content: "enriched 1",
		Status:  types.StatusEnriched,
	}
	mem2 := &types.Memory{
		ID:      "mem:test:e2",
		Content: "enriched 2",
		Status:  types.StatusEnriched,
	}
	err := store.Store(ctx, mem1)
	require.NoError(t, err)
	err = store.Store(ctx, mem2)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/connections/default/maintenance", nil)
	req.SetPathValue("name", "default")
	w := httptest.NewRecorder()

	handler.GetStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response MaintenanceStatus
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, 0, response.NeedsEnrichment)
	assert.Equal(t, 2, response.MissingEmbeddings) // Both enriched memories have no embeddings
}

// TestRunBackfill_Enrichment tests backfill for enrichment type.
func TestRunBackfill_Enrichment(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	engine := &SimpleMaintenanceEngine{}
	handler := NewMaintenanceHandler(connMgr, engine)

	// Add pending memories
	ctx := context.Background()
	mem1 := &types.Memory{
		ID:      "mem:test:p1",
		Content: "pending 1",
		Status:  types.StatusPending,
	}
	mem2 := &types.Memory{
		ID:      "mem:test:f1",
		Content: "failed 1",
		Status:  types.StatusFailed,
	}
	err := store.Store(ctx, mem1)
	require.NoError(t, err)
	err = store.Store(ctx, mem2)
	require.NoError(t, err)

	// Create request body
	reqBody := backfillRequest{Type: "enrichment"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/connections/default/maintenance/backfill",
		bytes.NewReader(body))
	req.SetPathValue("name", "default")
	w := httptest.NewRecorder()

	handler.RunBackfill(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response backfillResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, 2, response.Queued)
	assert.Equal(t, "queued for enrichment", response.Message)
}

// TestRunBackfill_Embeddings tests backfill for embeddings type.
func TestRunBackfill_Embeddings(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	engine := &SimpleMaintenanceEngine{}
	handler := NewMaintenanceHandler(connMgr, engine)

	// Add enriched memories
	ctx := context.Background()
	mem := &types.Memory{
		ID:      "mem:test:e1",
		Content: "enriched",
		Status:  types.StatusEnriched,
	}
	err := store.Store(ctx, mem)
	require.NoError(t, err)

	reqBody := backfillRequest{Type: "embeddings"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/connections/default/maintenance/backfill",
		bytes.NewReader(body))
	req.SetPathValue("name", "default")
	w := httptest.NewRecorder()

	handler.RunBackfill(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response backfillResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, 1, response.Queued)
	assert.Equal(t, "queued for embedding generation", response.Message)
}

// TestRunBackfill_ReEmbedAll tests backfill for re-embed-all type.
func TestRunBackfill_ReEmbedAll(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	engine := &SimpleMaintenanceEngine{}
	handler := NewMaintenanceHandler(connMgr, engine)

	// Add enriched memories
	ctx := context.Background()
	var err error
	for i := 0; i < 3; i++ {
		mem := &types.Memory{
			ID:      "mem:test:e" + string(rune(i)),
			Content: "enriched " + string(rune(i)),
			Status:  types.StatusEnriched,
		}
		err = store.Store(ctx, mem)
		require.NoError(t, err)
	}

	reqBody := backfillRequest{Type: "re-embed-all"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/connections/default/maintenance/backfill",
		bytes.NewReader(body))
	req.SetPathValue("name", "default")
	w := httptest.NewRecorder()

	handler.RunBackfill(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response backfillResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, 3, response.Queued)
	assert.Contains(t, response.Message, "deleted all embeddings")
}

// TestRunBackfill_InvalidType tests backfill with invalid type.
func TestRunBackfill_InvalidType(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	reqBody := backfillRequest{Type: "invalid-type"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/connections/default/maintenance/backfill",
		bytes.NewReader(body))
	req.SetPathValue("name", "default")
	w := httptest.NewRecorder()

	handler.RunBackfill(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid type")
}

// TestRunBackfill_MissingConnectionName tests backfill without connection name.
func TestRunBackfill_MissingConnectionName(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	reqBody := backfillRequest{Type: "enrichment"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/connections//maintenance/backfill",
		bytes.NewReader(body))
	req.SetPathValue("name", "")
	w := httptest.NewRecorder()

	handler.RunBackfill(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "connection name is required")
}

// TestRunBackfill_BadJSON tests backfill with invalid JSON body.
func TestRunBackfill_BadJSON(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/connections/default/maintenance/backfill",
		bytes.NewReader([]byte("invalid json")))
	req.SetPathValue("name", "default")
	w := httptest.NewRecorder()

	handler.RunBackfill(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid request body")
}

// TestRunBackfill_NoEngine tests backfill when engine is nil.
func TestRunBackfill_NoEngine(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil) // No engine

	ctx := context.Background()
	mem := &types.Memory{
		ID:      "mem:test:p1",
		Content: "pending",
		Status:  types.StatusPending,
	}
	err := store.Store(ctx, mem)
	require.NoError(t, err)

	reqBody := backfillRequest{Type: "enrichment"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/connections/default/maintenance/backfill",
		bytes.NewReader(body))
	req.SetPathValue("name", "default")
	w := httptest.NewRecorder()

	handler.RunBackfill(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response backfillResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, 1, response.Queued)
	assert.Contains(t, response.Message, "engine not running")
}

// TestRetryEnrichment_Success tests successful enrichment retry.
func TestRetryEnrichment_Success(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	engine := &SimpleMaintenanceEngine{}
	handler := NewMaintenanceHandler(connMgr, engine)

	// Add a failed memory
	ctx := context.Background()
	mem := &types.Memory{
		ID:      "mem:test:fail1",
		Content: "failed content",
		Status:  types.StatusFailed,
	}
	err := store.Store(ctx, mem)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/memories/mem:test:fail1/retry", nil)
	req.SetPathValue("id", "mem:test:fail1")
	req.Header.Set("X-Connection", "default")
	w := httptest.NewRecorder()

	handler.RetryEnrichment(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "mem:test:fail1", response["id"])
	assert.Equal(t, true, response["queued"])
	assert.Contains(t, response["message"], "enrichment retry")

	// Verify memory status was updated
	updated, err := store.Get(ctx, "mem:test:fail1")
	require.NoError(t, err)
	assert.Equal(t, types.StatusPending, updated.Status)
}

// TestRetryEnrichment_MissingMemoryID tests retry without memory ID.
func TestRetryEnrichment_MissingMemoryID(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/memories//retry", nil)
	req.SetPathValue("id", "")
	w := httptest.NewRecorder()

	handler.RetryEnrichment(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "memory ID is required")
}

// TestRetryEnrichment_MemoryNotFound tests retry with non-existent memory.
func TestRetryEnrichment_MemoryNotFound(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/memories/nonexistent/retry", nil)
	req.SetPathValue("id", "nonexistent")
	req.Header.Set("X-Connection", "default")
	w := httptest.NewRecorder()

	handler.RetryEnrichment(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "memory not found")
}

// TestRetryEnrichment_NotInFailedState tests retry on non-failed memory.
func TestRetryEnrichment_NotInFailedState(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	// Add a pending memory (not failed)
	ctx := context.Background()
	mem := &types.Memory{
		ID:      "mem:test:pending1",
		Content: "pending content",
		Status:  types.StatusPending,
	}
	err := store.Store(ctx, mem)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/memories/mem:test:pending1/retry", nil)
	req.SetPathValue("id", "mem:test:pending1")
	req.Header.Set("X-Connection", "default")
	w := httptest.NewRecorder()

	handler.RetryEnrichment(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, false, response["queued"])
	assert.Contains(t, response["message"], "not in failed state")
}

// TestRetryEnrichment_MethodNotAllowed tests non-POST method.
func TestRetryEnrichment_MethodNotAllowed(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/memories/mem:test:123/retry", nil)
	req.SetPathValue("id", "mem:test:123")
	w := httptest.NewRecorder()

	handler.RetryEnrichment(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Contains(t, w.Body.String(), "Method not allowed")
}

// TestRetryEnrichment_NoEngine tests retry when engine is nil.
func TestRetryEnrichment_NoEngine(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil) // No engine

	ctx := context.Background()
	mem := &types.Memory{
		ID:      "mem:test:fail2",
		Content: "failed content",
		Status:  types.StatusFailed,
	}
	err := store.Store(ctx, mem)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/memories/mem:test:fail2/retry", nil)
	req.SetPathValue("id", "mem:test:fail2")
	req.Header.Set("X-Connection", "default")
	w := httptest.NewRecorder()

	handler.RetryEnrichment(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, false, response["queued"]) // Engine is nil, so queued=false
}

// TestGetUnknownTypes_Success tests successful retrieval of unknown types.
func TestGetUnknownTypes_Success(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/connections/default/maintenance/unknown-types", nil)
	req.SetPathValue("name", "default")
	w := httptest.NewRecorder()

	handler.GetUnknownTypes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response UnknownTypesResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.IsType(t, []UnknownTypeStat{}, response.Entities)
	assert.IsType(t, []UnknownTypeStat{}, response.Relationships)
}

// TestGetUnknownTypes_MissingConnectionName tests GET unknown-types without connection name.
func TestGetUnknownTypes_MissingConnectionName(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/connections//maintenance/unknown-types", nil)
	req.SetPathValue("name", "")
	w := httptest.NewRecorder()

	handler.GetUnknownTypes(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "connection name is required")
}

// TestGetUnknownTypes_ConnectionNotFound tests GET unknown-types with invalid connection.
func TestGetUnknownTypes_ConnectionNotFound(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/connections/invalid/maintenance/unknown-types", nil)
	req.SetPathValue("name", "invalid")
	w := httptest.NewRecorder()

	handler.GetUnknownTypes(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "connection not found")
}

// TestMaintenanceHandlerConstruction tests creation of MaintenanceHandler.
func TestMaintenanceHandlerConstruction(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	// With engine
	engine := &SimpleMaintenanceEngine{}
	handler := NewMaintenanceHandler(connMgr, engine)
	assert.NotNil(t, handler)

	// Without engine (nil is valid)
	handler2 := NewMaintenanceHandler(connMgr, nil)
	assert.NotNil(t, handler2)
}

// TestRunBackfill_ConnectionNotFound tests backfill with non-existent connection.
func TestRunBackfill_ConnectionNotFound(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	reqBody := backfillRequest{Type: "enrichment"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/connections/invalid/maintenance/backfill",
		bytes.NewReader(body))
	req.SetPathValue("name", "invalid")
	w := httptest.NewRecorder()

	handler.RunBackfill(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "connection not found")
}

// TestBackfillResponse tests backfill response structure.
func TestBackfillResponse_JSON(t *testing.T) {
	resp := backfillResponse{
		Queued:  5,
		Message: "test message",
	}

	body, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded backfillResponse
	err = json.Unmarshal(body, &decoded)
	require.NoError(t, err)
	assert.Equal(t, 5, decoded.Queued)
	assert.Equal(t, "test message", decoded.Message)
}

// TestMaintenanceStatus_JSON tests MaintenanceStatus response structure.
func TestMaintenanceStatus_JSON(t *testing.T) {
	status := MaintenanceStatus{
		NeedsEnrichment: 3,
		MissingEmbeddings: 2,
		ModelMismatches:   1,
		CurrentModel:      "test-model",
		StoredModels: []ModelCount{
			{Model: "model-1", Count: 10},
			{Model: "model-2", Count: 5},
		},
	}

	body, err := json.Marshal(status)
	require.NoError(t, err)

	var decoded MaintenanceStatus
	err = json.Unmarshal(body, &decoded)
	require.NoError(t, err)
	assert.Equal(t, 3, decoded.NeedsEnrichment)
	assert.Equal(t, 2, decoded.MissingEmbeddings)
	assert.Equal(t, "test-model", decoded.CurrentModel)
	assert.Len(t, decoded.StoredModels, 2)
}

// TestRetryEnrichment_DefaultConnectionHeader tests retry with default connection from header.
func TestRetryEnrichment_DefaultConnectionHeader(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	engine := &SimpleMaintenanceEngine{}
	handler := NewMaintenanceHandler(connMgr, engine)

	ctx := context.Background()
	mem := &types.Memory{
		ID:      "mem:test:fail3",
		Content: "failed content",
		Status:  types.StatusFailed,
	}
	err := store.Store(ctx, mem)
	require.NoError(t, err)

	// No X-Connection header, should default to "default"
	req := httptest.NewRequest(http.MethodPost, "/api/memories/mem:test:fail3/retry", nil)
	req.SetPathValue("id", "mem:test:fail3")
	// Don't set X-Connection header
	w := httptest.NewRecorder()

	handler.RetryEnrichment(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, true, response["queued"])
}

// TestRunBackfill_EmptyQueryResults tests backfill with no matching memories.
func TestRunBackfill_EmptyQueryResults(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	engine := &SimpleMaintenanceEngine{}
	handler := NewMaintenanceHandler(connMgr, engine)

	// No memories in database
	reqBody := backfillRequest{Type: "enrichment"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/connections/default/maintenance/backfill",
		bytes.NewReader(body))
	req.SetPathValue("name", "default")
	w := httptest.NewRecorder()

	handler.RunBackfill(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response backfillResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, 0, response.Queued)
	assert.Equal(t, "queued for enrichment", response.Message)
}

// TestGetStatus_EmptyDatabase tests status with empty database.
func TestGetStatus_EmptyDatabase(t *testing.T) {
	connMgr, store := setupMaintenanceTest(t)
	defer func() { _ = store.Close() }()

	handler := NewMaintenanceHandler(connMgr, nil)

	// No memories added
	req := httptest.NewRequest(http.MethodGet, "/api/connections/default/maintenance", nil)
	req.SetPathValue("name", "default")
	w := httptest.NewRecorder()

	handler.GetStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response MaintenanceStatus
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, 0, response.NeedsEnrichment)
	assert.Equal(t, 0, response.MissingEmbeddings)
	assert.Len(t, response.StoredModels, 0)
}
