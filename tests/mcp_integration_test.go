package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/scrypster/memento/internal/api/mcp"
	"github.com/scrypster/memento/pkg/types"
)

// TestMCPIntegration_FullWorkflow tests the complete MCP workflow
func TestMCPIntegration_FullWorkflow(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Step 1: Store a memory
	storeRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "store_memory",
		"params": map[string]interface{}{
			"content": "Go is a statically typed programming language",
			"source":  "test",
			"domain":  "tech",
			"tags":    []string{"programming", "go"},
		},
		"id": 1,
	}

	storeJSON, err := json.Marshal(storeRequest)
	if err != nil {
		t.Fatalf("Failed to marshal store request: %v", err)
	}

	storeResponse, err := server.HandleRequest(ctx, storeJSON)
	if err != nil {
		t.Fatalf("Store request failed: %v", err)
	}

	var storeResult map[string]interface{}
	if err := json.Unmarshal(storeResponse, &storeResult); err != nil {
		t.Fatalf("Failed to unmarshal store response: %v", err)
	}

	// Extract memory ID from result
	result, ok := storeResult["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("Invalid store result: %v", storeResult)
	}

	memoryID, ok := result["id"].(string)
	if !ok || memoryID == "" {
		t.Fatalf("Invalid memory ID: %v", result["id"])
	}

	t.Logf("Stored memory with ID: %s", memoryID)

	// Step 2: Recall the memory
	recallRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "recall_memory",
		"params": map[string]interface{}{
			"id": memoryID,
		},
		"id": 2,
	}

	recallJSON, err := json.Marshal(recallRequest)
	if err != nil {
		t.Fatalf("Failed to marshal recall request: %v", err)
	}

	recallResponse, err := server.HandleRequest(ctx, recallJSON)
	if err != nil {
		t.Fatalf("Recall request failed: %v", err)
	}

	var recallResult map[string]interface{}
	if err := json.Unmarshal(recallResponse, &recallResult); err != nil {
		t.Fatalf("Failed to unmarshal recall response: %v", err)
	}

	recallData, ok := recallResult["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("Invalid recall result: %v", recallResult)
	}

	found, ok := recallData["found"].(bool)
	if !ok || !found {
		t.Fatalf("Memory not found: %v", recallData)
	}

	t.Logf("Successfully recalled memory")

	// Step 3: Find related memories
	findRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "find_related",
		"params": map[string]interface{}{
			"query":  "programming",
			"limit":  10,
			"domain": "tech",
		},
		"id": 3,
	}

	findJSON, err := json.Marshal(findRequest)
	if err != nil {
		t.Fatalf("Failed to marshal find request: %v", err)
	}

	findResponse, err := server.HandleRequest(ctx, findJSON)
	if err != nil {
		t.Fatalf("Find request failed: %v", err)
	}

	var findResult map[string]interface{}
	if err := json.Unmarshal(findResponse, &findResult); err != nil {
		t.Fatalf("Failed to unmarshal find response: %v", err)
	}

	findData, ok := findResult["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("Invalid find result: %v", findResult)
	}

	total, ok := findData["total"].(float64)
	if !ok {
		t.Fatalf("Invalid total: %v", findData["total"])
	}

	if total < 1 {
		t.Errorf("Expected at least 1 related memory, got %v", total)
	}

	t.Logf("Found %d related memories", int(total))

	// Step 4: Explain reasoning
	explainRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "explain_reasoning",
		"params": map[string]interface{}{
			"query":      "programming",
			"memory_ids": []string{memoryID},
		},
		"id": 4,
	}

	explainJSON, err := json.Marshal(explainRequest)
	if err != nil {
		t.Fatalf("Failed to marshal explain request: %v", err)
	}

	explainResponse, err := server.HandleRequest(ctx, explainJSON)
	if err != nil {
		t.Fatalf("Explain request failed: %v", err)
	}

	var explainResult map[string]interface{}
	if err := json.Unmarshal(explainResponse, &explainResult); err != nil {
		t.Fatalf("Failed to unmarshal explain response: %v", err)
	}

	explainData, ok := explainResult["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("Invalid explain result: %v", explainResult)
	}

	explanation, ok := explainData["explanation"].(string)
	if !ok || explanation == "" {
		t.Errorf("Invalid explanation: %v", explainData["explanation"])
	}

	t.Logf("Explanation: %s", explanation)
}

// TestMCPIntegration_RetryWorkflow tests the retry enrichment workflow
func TestMCPIntegration_RetryWorkflow(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Store a memory with failed status
	memory := &types.Memory{
		ID:                 "mem:test:retry1",
		Content:            "Test retry workflow",
		Source:             "test",
		Domain:             "testing",
		Status:             types.StatusFailed,
		EntityStatus:       types.EnrichmentFailed,
		RelationshipStatus: types.EnrichmentFailed,
		EmbeddingStatus:    types.EnrichmentFailed,
		EnrichmentAttempts: 3,
		EnrichmentError:    "Test error",
	}

	if err := store.Store(ctx, memory); err != nil {
		t.Fatalf("Failed to store test memory: %v", err)
	}

	// Retry enrichment
	retryRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "retry_enrichment",
		"params": map[string]interface{}{
			"id": memory.ID,
		},
		"id": 1,
	}

	retryJSON, err := json.Marshal(retryRequest)
	if err != nil {
		t.Fatalf("Failed to marshal retry request: %v", err)
	}

	retryResponse, err := server.HandleRequest(ctx, retryJSON)
	if err != nil {
		t.Fatalf("Retry request failed: %v", err)
	}

	var retryResult map[string]interface{}
	if err := json.Unmarshal(retryResponse, &retryResult); err != nil {
		t.Fatalf("Failed to unmarshal retry response: %v", err)
	}

	retryData, ok := retryResult["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("Invalid retry result: %v", retryResult)
	}

	queued, ok := retryData["queued"].(bool)
	if !ok || !queued {
		t.Errorf("Expected memory to be queued for retry")
	}

	// Verify memory status was updated
	updated, err := store.Get(ctx, memory.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve updated memory: %v", err)
	}

	if updated.Status != types.StatusPending {
		t.Errorf("Expected status 'pending', got '%s'", updated.Status)
	}

	if updated.EntityStatus != types.EnrichmentPending {
		t.Errorf("Expected entity status 'pending', got '%s'", updated.EntityStatus)
	}

	t.Logf("Successfully retried enrichment for memory %s", memory.ID)
}

// TestMCPIntegration_ErrorResponses tests error handling in the protocol
func TestMCPIntegration_ErrorResponses(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	tests := []struct {
		name           string
		request        map[string]interface{}
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "invalid JSON-RPC version",
			request: map[string]interface{}{
				"jsonrpc": "1.0",
				"method":  "store_memory",
				"params":  map[string]interface{}{"content": "test"},
				"id":      1,
			},
			expectError:    true,
			expectedErrMsg: "Invalid JSON-RPC version",
		},
		{
			name: "method not found",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "nonexistent_method",
				"params":  map[string]interface{}{},
				"id":      2,
			},
			expectError:    true,
			expectedErrMsg: "Method not found",
		},
		{
			name: "invalid params for store_memory",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "store_memory",
				"params":  map[string]interface{}{"content": ""},
				"id":      3,
			},
			expectError:    true,
			expectedErrMsg: "content is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestJSON, err := json.Marshal(tt.request)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			response, err := server.HandleRequest(ctx, requestJSON)
			if err != nil {
				t.Fatalf("HandleRequest failed: %v", err)
			}

			var result map[string]interface{}
			if err := json.Unmarshal(response, &result); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if tt.expectError {
				if _, hasError := result["error"]; !hasError {
					t.Errorf("Expected error in response, got: %v", result)
				}
			} else {
				if _, hasError := result["error"]; hasError {
					t.Errorf("Expected no error, got: %v", result["error"])
				}
			}
		})
	}
}
