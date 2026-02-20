package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/api/mcp"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// TestMCPServer_StoreMemory verifies that store_memory returns immediately with pending status
func TestMCPServer_StoreMemory(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	args := mcp.StoreMemoryArgs{
		Content: "Test memory content",
		Source:  "test",
		Domain:  "testing",
		Tags:    []string{"test", "mcp"},
		Metadata: map[string]interface{}{
			"test_key": "test_value",
		},
	}

	start := time.Now()
	result, err := server.StoreMemory(ctx, args)
	elapsed := time.Since(start)

	// Should complete quickly (< 100ms for v2.0)
	if elapsed > 100*time.Millisecond {
		t.Errorf("StoreMemory took too long: %v (expected < 100ms)", elapsed)
	}

	if err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	if result.ID == "" {
		t.Error("Expected non-empty ID")
	}

	if result.Status != types.StatusPending {
		t.Errorf("Expected status 'pending', got '%s'", result.Status)
	}

	if result.Message == "" {
		t.Error("Expected non-empty message")
	}

	// Verify memory was stored
	memory, err := store.Get(ctx, result.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve stored memory: %v", err)
	}

	if memory.Content != args.Content {
		t.Errorf("Expected content '%s', got '%s'", args.Content, memory.Content)
	}

	if memory.Source != args.Source {
		t.Errorf("Expected source '%s', got '%s'", args.Source, memory.Source)
	}

	if memory.Domain != args.Domain {
		t.Errorf("Expected domain '%s', got '%s'", args.Domain, memory.Domain)
	}

	if memory.Status != types.StatusPending {
		t.Errorf("Expected status 'pending', got '%s'", memory.Status)
	}
}

// TestMCPServer_RecallMemory verifies memory retrieval by ID
func TestMCPServer_RecallMemory(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Store a memory first
	memory := &types.Memory{
		ID:        "mem:test:recall",
		Content:   "Test recall content",
		Source:    "test",
		Domain:    "testing",
		Status:    types.StatusEnriched,
		Timestamp: time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := store.Store(ctx, memory); err != nil {
		t.Fatalf("Failed to store test memory: %v", err)
	}

	// Recall the memory
	args := mcp.RecallMemoryArgs{
		ID: memory.ID,
	}

	result, err := server.RecallMemory(ctx, args)
	if err != nil {
		t.Fatalf("RecallMemory failed: %v", err)
	}

	if !result.Found {
		t.Error("Expected memory to be found")
	}

	if result.Memory == nil {
		t.Fatal("Expected non-nil memory")
	}

	if result.Memory.ID != memory.ID {
		t.Errorf("Expected ID '%s', got '%s'", memory.ID, result.Memory.ID)
	}

	if result.Memory.Content != memory.Content {
		t.Errorf("Expected content '%s', got '%s'", memory.Content, result.Memory.Content)
	}
}

// TestMCPServer_RecallMemory_NotFound verifies handling of non-existent memories
func TestMCPServer_RecallMemory_NotFound(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	args := mcp.RecallMemoryArgs{
		ID: "mem:test:nonexistent",
	}

	result, err := server.RecallMemory(ctx, args)
	if err != nil {
		t.Fatalf("RecallMemory failed: %v", err)
	}

	if result.Found {
		t.Error("Expected memory not to be found")
	}

	if result.Memory != nil {
		t.Error("Expected nil memory for not found case")
	}
}

// TestMCPServer_FindRelated verifies basic search functionality
func TestMCPServer_FindRelated(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Store some test memories
	memories := []*types.Memory{
		{
			ID:        "mem:test:related1",
			Content:   "Go programming language",
			Source:    "test",
			Domain:    "tech",
			Status:    types.StatusEnriched,
			Timestamp: time.Now(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:        "mem:test:related2",
			Content:   "Python programming tutorial",
			Source:    "test",
			Domain:    "tech",
			Status:    types.StatusEnriched,
			Timestamp: time.Now(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:        "mem:test:related3",
			Content:   "Cooking recipes for dinner",
			Source:    "test",
			Domain:    "food",
			Status:    types.StatusEnriched,
			Timestamp: time.Now(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	for _, mem := range memories {
		if err := store.Store(ctx, mem); err != nil {
			t.Fatalf("Failed to store test memory: %v", err)
		}
	}

	// Find related memories
	args := mcp.FindRelatedArgs{
		Query:  "programming",
		Limit:  10,
		Domain: "tech",
	}

	result, err := server.FindRelated(ctx, args)
	if err != nil {
		t.Fatalf("FindRelated failed: %v", err)
	}

	if result.Total < 1 {
		t.Error("Expected at least 1 related memory")
	}

	if len(result.Memories) == 0 {
		t.Error("Expected non-empty memories list")
	}
}

// TestMCPServer_RetryEnrichment verifies retry enrichment functionality
func TestMCPServer_RetryEnrichment(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Store a failed memory
	memory := &types.Memory{
		ID:                 "mem:test:failed",
		Content:            "Test failed enrichment",
		Source:             "test",
		Domain:             "testing",
		Status:             types.StatusFailed,
		EntityStatus:       types.EnrichmentFailed,
		RelationshipStatus: types.EnrichmentFailed,
		EmbeddingStatus:    types.EnrichmentFailed,
		EnrichmentAttempts: 3,
		EnrichmentError:    "Test error",
		Timestamp:          time.Now(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := store.Store(ctx, memory); err != nil {
		t.Fatalf("Failed to store test memory: %v", err)
	}

	// Retry enrichment
	args := mcp.RetryEnrichmentArgs{
		ID: memory.ID,
	}

	result, err := server.RetryEnrichment(ctx, args)
	if err != nil {
		t.Fatalf("RetryEnrichment failed: %v", err)
	}

	if result.ID != memory.ID {
		t.Errorf("Expected ID '%s', got '%s'", memory.ID, result.ID)
	}

	if !result.Queued {
		t.Error("Expected memory to be queued for retry")
	}

	if result.Message == "" {
		t.Error("Expected non-empty message")
	}

	// Verify memory status was updated
	updated, err := store.Get(ctx, memory.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve updated memory: %v", err)
	}

	if updated.Status != types.StatusPending {
		t.Errorf("Expected status 'pending', got '%s'", updated.Status)
	}
}

// TestMCPServer_ExplainReasoning verifies reasoning explanation
func TestMCPServer_ExplainReasoning(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	args := mcp.ExplainReasoningArgs{
		Query:     "Go programming",
		MemoryIDs: []string{"mem:test:1", "mem:test:2"},
	}

	result, err := server.ExplainReasoning(ctx, args)
	if err != nil {
		t.Fatalf("ExplainReasoning failed: %v", err)
	}

	if result.Explanation == "" {
		t.Error("Expected non-empty explanation")
	}

	if len(result.Reasoning) == 0 {
		t.Error("Expected non-empty reasoning steps")
	}

	// Verify each reasoning step has required fields
	for i, step := range result.Reasoning {
		if step.Step == "" {
			t.Errorf("Reasoning step %d missing step name", i)
		}
		if step.Description == "" {
			t.Errorf("Reasoning step %d missing description", i)
		}
	}
}

// TestMCPProtocol_Compliance verifies JSON-RPC 2.0 format compliance
func TestMCPProtocol_Compliance(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Create a JSON-RPC request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "store_memory",
		"params": map[string]interface{}{
			"content": "Test content",
			"source":  "test",
		},
		"id": 1,
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	// Process the request
	response, err := server.HandleRequest(ctx, requestJSON)
	if err != nil {
		t.Fatalf("HandleRequest failed: %v", err)
	}

	// Parse response
	var responseMap map[string]interface{}
	if err := json.Unmarshal(response, &responseMap); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify JSON-RPC 2.0 fields
	if responseMap["jsonrpc"] != "2.0" {
		t.Errorf("Expected jsonrpc '2.0', got '%v'", responseMap["jsonrpc"])
	}

	if responseMap["id"] != float64(1) {
		t.Errorf("Expected id 1, got %v", responseMap["id"])
	}

	// Should have either result or error, not both
	_, hasResult := responseMap["result"]
	_, hasError := responseMap["error"]

	if hasResult && hasError {
		t.Error("Response should not have both result and error")
	}

	if !hasResult && !hasError {
		t.Error("Response should have either result or error")
	}
}

// TestMCPServer_ErrorHandling verifies error handling with invalid inputs
func TestMCPServer_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	tests := []struct {
		name    string
		args    interface{}
		wantErr bool
	}{
		{
			name: "empty content",
			args: mcp.StoreMemoryArgs{
				Content: "",
				Source:  "test",
			},
			wantErr: true,
		},
		// NOTE: empty ID for RecallMemory is no longer an error.
		// Opus Issue #5 introduced list-filter mode: when ID is empty
		// RecallMemory lists all memories matching optional filter fields.
		{
			name: "empty query for find_related",
			args: mcp.FindRelatedArgs{
				Query: "",
			},
			wantErr: true,
		},
		{
			name: "negative limit for find_related",
			args: mcp.FindRelatedArgs{
				Query: "test",
				Limit: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error

			switch args := tt.args.(type) {
			case mcp.StoreMemoryArgs:
				_, err = server.StoreMemory(ctx, args)
			case mcp.RecallMemoryArgs:
				_, err = server.RecallMemory(ctx, args)
			case mcp.FindRelatedArgs:
				_, err = server.FindRelated(ctx, args)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("Expected error: %v, got: %v", tt.wantErr, err)
			}
		})
	}
}

// setupTestStore creates a test memory store
func setupTestStore(t *testing.T) *sqlite.MemoryStore {
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test store: %v", err)
	}
	return store
}
