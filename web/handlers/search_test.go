package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
	"github.com/stretchr/testify/assert"
)

// newTestSQLiteStore creates an in-memory SQLite store for handler tests that
// require direct database access (e.g., SearchHandler).
func newTestSQLiteStore(t *testing.T) *sqlite.MemoryStore {
	t.Helper()
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// insertTestMemory inserts a memory directly via the store for search test setup.
func insertTestMemory(t *testing.T, store *sqlite.MemoryStore, id, content, source string) {
	t.Helper()
	mem := &types.Memory{
		ID:        id,
		Content:   content,
		Source:    source,
		Status:    types.StatusEnriched,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.Store(context.Background(), mem); err != nil {
		t.Fatalf("failed to insert test memory %s: %v", id, err)
	}
}

// TestSearchHandler_Search tests the Search endpoint using a real SQLite store.
func TestSearchHandler_Search(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(store *sqlite.MemoryStore)
		queryParams    string
		expectedStatus int
		validateBody   func(*testing.T, []byte)
	}{
		{
			name: "successful search with matches",
			setup: func(store *sqlite.MemoryStore) {
				insertTestMemory(t, store, "mem:test:1", "This is a test memory", "manual")
				insertTestMemory(t, store, "mem:test:2", "Another test case", "api")
				insertTestMemory(t, store, "mem:test:3", "No match here for xyz", "manual")
			},
			queryParams:    "?q=test",
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]interface{}
				assert.NoError(t, json.Unmarshal(body, &result))
				assert.Equal(t, "test", result["query"])

				results, ok := result["results"].([]interface{})
				assert.True(t, ok)
				// Both "test memory" and "test case" should match
				assert.GreaterOrEqual(t, len(results), 2)
			},
		},
		{
			name: "search with no matches",
			setup: func(store *sqlite.MemoryStore) {
				insertTestMemory(t, store, "mem:test:1", "Hello world", "manual")
			},
			queryParams:    "?q=nonexistentzzzq",
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]interface{}
				assert.NoError(t, json.Unmarshal(body, &result))
				assert.Equal(t, "nonexistentzzzq", result["query"])
				assert.Equal(t, float64(0), result["total"])

				results, ok := result["results"].([]interface{})
				assert.True(t, ok)
				assert.Len(t, results, 0)
			},
		},
		{
			name: "missing query parameter returns all memories",
			setup: func(store *sqlite.MemoryStore) {
				insertTestMemory(t, store, "mem:test:1", "First memory", "manual")
				insertTestMemory(t, store, "mem:test:2", "Second memory", "api")
			},
			queryParams:    "?page=1&page_size=20",
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]interface{}
				assert.NoError(t, json.Unmarshal(body, &result))
				assert.Equal(t, "", result["query"])
				assert.Equal(t, float64(2), result["total"])
				assert.Equal(t, float64(1), result["page"])
				assert.Equal(t, float64(20), result["page_size"])

				results, ok := result["results"].([]interface{})
				assert.True(t, ok)
				assert.Equal(t, 2, len(results))
			},
		},
		{
			name: "case-insensitive matching",
			setup: func(store *sqlite.MemoryStore) {
				insertTestMemory(t, store, "mem:test:1", "this is a test memory", "manual")
				insertTestMemory(t, store, "mem:test:2", "THIS IS A TEST MEMORY", "manual")
			},
			queryParams:    "?q=TeSt",
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]interface{}
				assert.NoError(t, json.Unmarshal(body, &result))
				assert.Equal(t, "TeSt", result["query"])
				// Both memories should match case-insensitively
				total, ok := result["total"].(float64)
				assert.True(t, ok)
				assert.GreaterOrEqual(t, int(total), 2)
			},
		},
		{
			name:           "empty result set",
			setup:          func(store *sqlite.MemoryStore) {},
			queryParams:    "?q=anything",
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]interface{}
				assert.NoError(t, json.Unmarshal(body, &result))
				assert.Equal(t, "anything", result["query"])
				assert.Equal(t, float64(0), result["total"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestSQLiteStore(t)
			tt.setup(store)

			connMgr := connections.NewManagerWithStore(store, "test-connection")
			handler := NewSearchHandler(store, connMgr)

			req := httptest.NewRequest(http.MethodGet, "/api/search"+tt.queryParams, nil)
			rec := httptest.NewRecorder()

			handler.Search(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, rec.Body.Bytes())
			}
		})
	}
}
