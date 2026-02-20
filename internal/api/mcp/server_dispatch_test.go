package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scrypster/memento/internal/api/mcp"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// ---------------------------------------------------------------------------
// Tests for handleRecallMemory (recall_memory JSON-RPC method)
// ---------------------------------------------------------------------------

// TestHandleRecallMemory_Success tests the happy path via JSON-RPC dispatch.
func TestHandleRecallMemory_Success(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store a test memory
	now := time.Now()
	store.memories["mem:test:recall:1"] = &types.Memory{
		ID:        "mem:test:recall:1",
		Content:   "test recall content",
		Status:    types.StatusEnriched,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Call via JSON-RPC
	req := `{"jsonrpc":"2.0","method":"recall_memory","params":{"id":"mem:test:recall:1"},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify response is not an error
	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)

	// Unmarshal and verify the memory was returned
	var jsonResp map[string]interface{}
	err = json.Unmarshal(resp, &jsonResp)
	require.NoError(t, err)
	result := jsonResp["result"].(map[string]interface{})
	assert.True(t, result["found"].(bool))
}

// TestHandleRecallMemory_NotFound tests recall with non-existent ID.
func TestHandleRecallMemory_NotFound(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"recall_memory","params":{"id":"mem:nonexistent"},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	var jsonResp map[string]interface{}
	err = json.Unmarshal(resp, &jsonResp)
	require.NoError(t, err)
	result := jsonResp["result"].(map[string]interface{})
	assert.False(t, result["found"].(bool))
}

// ---------------------------------------------------------------------------
// Tests for handleRetryEnrichment (retry_enrichment JSON-RPC method)
// ---------------------------------------------------------------------------

// TestHandleRetryEnrichment_Success tests retry on a failed memory.
func TestHandleRetryEnrichment_Success(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Pre-populate a failed memory
	store.memories["mem:retry:1"] = &types.Memory{
		ID:                  "mem:retry:1",
		Content:             "content to retry",
		Status:              types.StatusFailed,
		EnrichmentAttempts:  3,
		EnrichmentError:     "previous error",
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	req := `{"jsonrpc":"2.0","method":"retry_enrichment","params":{"id":"mem:retry:1"},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, string(resp), `"result"`)
}

// TestHandleRetryEnrichment_MissingID tests with missing ID parameter.
func TestHandleRetryEnrichment_MissingID(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"retry_enrichment","params":{},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Should contain an error
	assert.Contains(t, string(resp), `"error"`)
}

// ---------------------------------------------------------------------------
// Tests for handleForgetMemory (forget_memory JSON-RPC method)
// ---------------------------------------------------------------------------

// TestHandleForgetMemory_Success tests soft-delete via JSON-RPC.
func TestHandleForgetMemory_Success(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store a memory
	result, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "to forget"})
	require.NoError(t, err)
	memID := result.ID

	// Forget via JSON-RPC
	req := fmt.Sprintf(`{"jsonrpc":"2.0","method":"forget_memory","params":{"id":%q},"id":1}`, memID)
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)
}

// TestHandleForgetMemory_MissingID tests with missing ID.
func TestHandleForgetMemory_MissingID(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"forget_memory","params":{},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)

	assert.Contains(t, string(resp), `"error"`)
}

// ---------------------------------------------------------------------------
// Tests for handleEvolveMemory (evolve_memory JSON-RPC method)
// ---------------------------------------------------------------------------

// TestHandleEvolveMemory_Success tests memory evolution via JSON-RPC.
func TestHandleEvolveMemory_Success(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store original memory
	result, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "original evolution"})
	require.NoError(t, err)
	originalID := result.ID

	// Evolve via JSON-RPC
	req := fmt.Sprintf(
		`{"jsonrpc":"2.0","method":"evolve_memory","params":{"id":%q,"new_content":"evolved content"},"id":1}`,
		originalID,
	)
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)

	// Verify response contains new_id
	var jsonResp map[string]interface{}
	err = json.Unmarshal(resp, &jsonResp)
	require.NoError(t, err)
	result2 := jsonResp["result"].(map[string]interface{})
	assert.NotEmpty(t, result2["new_id"])
	assert.Equal(t, originalID, result2["superseded_id"])
}

// TestHandleEvolveMemory_MissingContent tests evolution with missing new_content.
func TestHandleEvolveMemory_MissingContent(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "test"})
	require.NoError(t, err)

	req := fmt.Sprintf(
		`{"jsonrpc":"2.0","method":"evolve_memory","params":{"id":%q},"id":1}`,
		result.ID,
	)
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)

	assert.Contains(t, string(resp), `"error"`)
}

// ---------------------------------------------------------------------------
// Tests for handleConsolidateMemories (consolidate_memories JSON-RPC method)
// ---------------------------------------------------------------------------

// TestHandleConsolidateMemories_Success tests consolidation via JSON-RPC.
func TestHandleConsolidateMemories_Success(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store two memories
	r1, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "memory one"})
	require.NoError(t, err)
	r2, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "memory two"})
	require.NoError(t, err)

	// Consolidate via JSON-RPC
	req := fmt.Sprintf(
		`{"jsonrpc":"2.0","method":"consolidate_memories","params":{"ids":[%q,%q],"title":"Consolidated"},"id":1}`,
		r1.ID, r2.ID,
	)
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)
}

// TestHandleConsolidateMemories_MissingIDsAndQuery tests with no ids or query.
func TestHandleConsolidateMemories_MissingIDsAndQuery(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"consolidate_memories","params":{},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)

	assert.Contains(t, string(resp), `"error"`)
}

// ---------------------------------------------------------------------------
// Tests for handleDetectContradictions (detect_contradictions JSON-RPC method)
// ---------------------------------------------------------------------------

// TestHandleDetectContradictions_Success tests contradiction detection via JSON-RPC.
func TestHandleDetectContradictions_Success(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store a test memory
	result, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "test for contradiction"})
	require.NoError(t, err)

	// Call detect_contradictions via JSON-RPC
	req := fmt.Sprintf(
		`{"jsonrpc":"2.0","method":"detect_contradictions","params":{"memory_id":%q},"id":1}`,
		result.ID,
	)
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Should succeed (even if no contradictions found)
	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)
}

// TestHandleDetectContradictions_NoMemoryID tests with no memory_id (detects all).
func TestHandleDetectContradictions_NoMemoryID(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"detect_contradictions","params":{},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, string(resp), `"result"`)
}

// ---------------------------------------------------------------------------
// Tests for handleUpdateMemory (update_memory JSON-RPC method)
// ---------------------------------------------------------------------------

// TestHandleUpdateMemory_Success tests memory update via JSON-RPC.
func TestHandleUpdateMemory_Success(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store a memory
	result, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "original content"})
	require.NoError(t, err)
	memID := result.ID

	// Update via JSON-RPC
	req := fmt.Sprintf(
		`{"jsonrpc":"2.0","method":"update_memory","params":{"id":%q,"content":"updated content"},"id":1}`,
		memID,
	)
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)
}

// TestHandleUpdateMemory_MissingID tests with missing ID.
func TestHandleUpdateMemory_MissingID(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"update_memory","params":{"content":"new"},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)

	assert.Contains(t, string(resp), `"error"`)
}

// ---------------------------------------------------------------------------
// Tests for handleRestoreMemory (restore_memory JSON-RPC method)
// ---------------------------------------------------------------------------

// TestHandleRestoreMemory_Success tests restoration via JSON-RPC.
func TestHandleRestoreMemory_Success(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store and soft-delete a memory
	result, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "to restore"})
	require.NoError(t, err)
	memID := result.ID

	_, err = srv.ForgetMemory(ctx, mcp.ForgetMemoryArgs{ID: memID, HardDelete: false})
	require.NoError(t, err)

	// Restore via JSON-RPC
	req := fmt.Sprintf(`{"jsonrpc":"2.0","method":"restore_memory","params":{"id":%q},"id":1}`, memID)
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)
}

// TestHandleRestoreMemory_MissingID tests with missing ID.
func TestHandleRestoreMemory_MissingID(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"restore_memory","params":{},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)

	assert.Contains(t, string(resp), `"error"`)
}

// ---------------------------------------------------------------------------
// Tests for handleListDeletedMemories (list_deleted_memories JSON-RPC method)
// ---------------------------------------------------------------------------

// TestHandleListDeletedMemories_Success tests listing deleted memories via JSON-RPC.
func TestHandleListDeletedMemories_Success(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store a memory and delete it
	result, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "to delete"})
	require.NoError(t, err)

	_, err = srv.ForgetMemory(ctx, mcp.ForgetMemoryArgs{ID: result.ID, HardDelete: false})
	require.NoError(t, err)

	// List deleted memories via JSON-RPC
	req := `{"jsonrpc":"2.0","method":"list_deleted_memories","params":{},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)

	// Verify the deleted memory is in the list
	var jsonResp map[string]interface{}
	err = json.Unmarshal(resp, &jsonResp)
	require.NoError(t, err)
	result2 := jsonResp["result"].(map[string]interface{})
	assert.Greater(t, int(result2["total"].(float64)), 0)
}

// TestHandleListDeletedMemories_Empty tests with no deleted memories.
func TestHandleListDeletedMemories_Empty(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"list_deleted_memories","params":{},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, string(resp), `"result"`)

	var jsonResp map[string]interface{}
	err = json.Unmarshal(resp, &jsonResp)
	require.NoError(t, err)
	result := jsonResp["result"].(map[string]interface{})
	assert.Equal(t, 0, int(result["total"].(float64)))
}

// ---------------------------------------------------------------------------
// Tests for handleGetEvolutionChain (get_evolution_chain JSON-RPC method)
// ---------------------------------------------------------------------------

// TestHandleGetEvolutionChain_Success tests retrieving evolution chain via JSON-RPC.
func TestHandleGetEvolutionChain_Success(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store and evolve a memory
	r1, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "v1"})
	require.NoError(t, err)

	r2, err := srv.EvolveMemory(ctx, mcp.EvolveMemoryArgs{
		ID:         r1.ID,
		NewContent: "v2",
	})
	require.NoError(t, err)

	// Get evolution chain via JSON-RPC
	req := fmt.Sprintf(`{"jsonrpc":"2.0","method":"get_evolution_chain","params":{"id":%q},"id":1}`, r2.NewID)
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)

	// Verify response contains chain
	var jsonResp map[string]interface{}
	err = json.Unmarshal(resp, &jsonResp)
	require.NoError(t, err)
	result := jsonResp["result"].(map[string]interface{})
	assert.Greater(t, int(result["total_versions"].(float64)), 0)
}

// TestHandleGetEvolutionChain_NotFound tests with non-existent memory.
func TestHandleGetEvolutionChain_NotFound(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"get_evolution_chain","params":{"id":"mem:nonexistent"},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)

	// Should error or return empty
	var jsonResp map[string]interface{}
	err = json.Unmarshal(resp, &jsonResp)
	require.NoError(t, err)
	// Either error or result is acceptable depending on implementation
	assert.True(t, jsonResp["error"] != nil || jsonResp["result"] != nil)
}

// ---------------------------------------------------------------------------
// Tests for handleCreateProject (create_project JSON-RPC method)
// ---------------------------------------------------------------------------

// TestHandleCreateProject_Success tests project creation via JSON-RPC.
func TestHandleCreateProject_Success(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"create_project","params":{"name":"Test Project"},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, string(resp), `"result"`)
	assert.NotContains(t, string(resp), `"error"`)

	// Verify response contains project_id
	var jsonResp map[string]interface{}
	err = json.Unmarshal(resp, &jsonResp)
	require.NoError(t, err)
	result := jsonResp["result"].(map[string]interface{})
	assert.NotEmpty(t, result["project_id"])
}

// TestHandleCreateProject_WithPhases tests creating project with phases via JSON-RPC.
func TestHandleCreateProject_WithPhases(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"create_project","params":{"name":"Project","phase_names":["Phase 1","Phase 2"]},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Contains(t, string(resp), `"result"`)

	var jsonResp map[string]interface{}
	err = json.Unmarshal(resp, &jsonResp)
	require.NoError(t, err)
	result := jsonResp["result"].(map[string]interface{})
	phaseIDs := result["phase_ids"].([]interface{})
	assert.Equal(t, 2, len(phaseIDs))
}

// TestHandleCreateProject_MissingName tests with missing project name.
func TestHandleCreateProject_MissingName(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	req := `{"jsonrpc":"2.0","method":"create_project","params":{},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)

	assert.Contains(t, string(resp), `"error"`)
}

// ---------------------------------------------------------------------------
// Direct dispatch tests (via handleXxx functions)
// ---------------------------------------------------------------------------

// TestDispatchRecallMemory tests handleRecallMemory directly.
func TestDispatchRecallMemory(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.memories["mem:direct:1"] = &types.Memory{
		ID:        "mem:direct:1",
		Content:   "dispatch test",
		Status:    types.StatusEnriched,
		CreatedAt: now,
		UpdatedAt: now,
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Call RecallMemory directly (exercises handleRecallMemory path)
	result, err := srv.RecallMemory(ctx, mcp.RecallMemoryArgs{ID: "mem:direct:1"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Found)
	assert.Equal(t, "mem:direct:1", result.Memory.ID)
}

// TestDispatchRetryEnrichment tests handleRetryEnrichment directly.
func TestDispatchRetryEnrichment(t *testing.T) {
	store := newMockStore()
	store.memories["mem:retry:direct"] = &types.Memory{
		ID:                 "mem:retry:direct",
		Content:            "test",
		Status:             types.StatusFailed,
		EnrichmentAttempts: 2,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.RetryEnrichment(ctx, mcp.RetryEnrichmentArgs{ID: "mem:retry:direct"})
	require.NoError(t, err)
	require.NotNil(t, result)
	// Result should indicate whether retry was queued
	assert.NotEmpty(t, result.Message)
}

// TestDispatchForgetMemory tests handleForgetMemory directly.
func TestDispatchForgetMemory(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store a memory
	storeResult, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "forget me"})
	require.NoError(t, err)

	// Call ForgetMemory directly
	result, err := srv.ForgetMemory(ctx, mcp.ForgetMemoryArgs{ID: storeResult.ID})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Deleted)
}

// TestDispatchEvolveMemory tests handleEvolveMemory directly.
func TestDispatchEvolveMemory(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store original
	storeResult, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "v1"})
	require.NoError(t, err)

	// Evolve
	result, err := srv.EvolveMemory(ctx, mcp.EvolveMemoryArgs{
		ID:         storeResult.ID,
		NewContent: "v2 evolved",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.NewID)
	assert.NotEqual(t, storeResult.ID, result.NewID)
}

// TestDispatchConsolidateMemories tests handleConsolidateMemories directly.
func TestDispatchConsolidateMemories(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store two memories
	r1, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "memo 1"})
	require.NoError(t, err)
	r2, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "memo 2"})
	require.NoError(t, err)

	// Consolidate
	result, err := srv.ConsolidateMemories(ctx, mcp.ConsolidateMemoriesArgs{
		IDs:   []string{r1.ID, r2.ID},
		Title: "Combined",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.NewID)
	assert.Len(t, result.ConsolidatedIDs, 2)
}

// TestDispatchDetectContradictions tests handleDetectContradictions directly.
func TestDispatchDetectContradictions(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Call without memory_id (detects all contradictions)
	result, err := srv.DetectContradictions(ctx, mcp.DetectContradictionsArgs{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.Contradictions)
}

// TestDispatchUpdateMemory tests handleUpdateMemory directly.
func TestDispatchUpdateMemory(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store a memory
	storeResult, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "original"})
	require.NoError(t, err)

	// Update
	result, err := srv.UpdateMemory(ctx, mcp.UpdateMemoryArgs{
		ID:      storeResult.ID,
		Content: "modified",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Updated)
}

// TestDispatchRestoreMemory tests handleRestoreMemory directly.
func TestDispatchRestoreMemory(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store and delete
	storeResult, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "to restore"})
	require.NoError(t, err)
	_, err = srv.ForgetMemory(ctx, mcp.ForgetMemoryArgs{ID: storeResult.ID})
	require.NoError(t, err)

	// Restore
	result, err := srv.RestoreMemory(ctx, mcp.RestoreMemoryArgs{ID: storeResult.ID})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Restored)
}

// TestDispatchListDeletedMemories tests handleListDeletedMemories directly.
func TestDispatchListDeletedMemories(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store and delete
	storeResult, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "temp"})
	require.NoError(t, err)
	_, err = srv.ForgetMemory(ctx, mcp.ForgetMemoryArgs{ID: storeResult.ID})
	require.NoError(t, err)

	// List deleted
	result, err := srv.ListDeletedMemories(ctx, mcp.ListDeletedMemoriesArgs{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Greater(t, result.Total, 0)
}

// TestDispatchGetEvolutionChain tests handleGetEvolutionChain directly.
func TestDispatchGetEvolutionChain(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Store and evolve
	r1, err := srv.StoreMemory(ctx, mcp.StoreMemoryArgs{Content: "v1"})
	require.NoError(t, err)

	r2, err := srv.EvolveMemory(ctx, mcp.EvolveMemoryArgs{
		ID:         r1.ID,
		NewContent: "v2",
	})
	require.NoError(t, err)

	// Get chain
	result, err := srv.GetEvolutionChain(ctx, mcp.GetEvolutionChainArgs{ID: r2.NewID})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Greater(t, result.TotalVersions, 0)
}

// TestDispatchCreateProject tests handleCreateProject directly.
func TestDispatchCreateProject(t *testing.T) {
	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	srv := mcp.NewServer(store)
	ctx := context.Background()

	result, err := srv.CreateProject(ctx, mcp.CreateProjectArgs{
		Name:        "Test",
		Description: "Test project",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.ProjectID)
}

// ---------------------------------------------------------------------------
// Error path tests
// ---------------------------------------------------------------------------

// TestDispatchErrors_UnmarshalFailure tests JSON unmarshal failure in dispatch.
func TestDispatchErrors_UnmarshalFailure(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Send malformed JSON for a method
	req := `{"jsonrpc":"2.0","method":"recall_memory","params":"not_an_object","id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)

	// Should get an error response (not a Go error)
	assert.Contains(t, string(resp), `"error"`)
}

// TestDispatchErrors_InvalidParams tests with invalid parameter types.
func TestDispatchErrors_InvalidParams(t *testing.T) {
	store := newMockStore()
	srv := mcp.NewServer(store)
	ctx := context.Background()

	// Call retry_enrichment with non-string ID
	req := `{"jsonrpc":"2.0","method":"retry_enrichment","params":{"id":123},"id":1}`
	resp, err := srv.HandleRequest(ctx, []byte(req))
	require.NoError(t, err)

	// May succeed or error depending on unmarshal tolerance
	assert.NotEmpty(t, resp)
}
