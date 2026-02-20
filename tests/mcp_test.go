package tests

import (
	"context"
	"encoding/json"
	"strings"
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

// TestUpdateMemory_Success verifies that UpdateMemory can successfully update memory content
func TestUpdateMemory_Success(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Store a memory first
	memory := &types.Memory{
		ID:        "mem:test:update",
		Content:   "Original content",
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

	// Update the memory
	args := mcp.UpdateMemoryArgs{
		ID:      memory.ID,
		Content: "Updated content",
	}

	result, err := server.UpdateMemory(ctx, args)
	if err != nil {
		t.Fatalf("UpdateMemory failed: %v", err)
	}

	if !result.Updated {
		t.Error("Expected Updated to be true")
	}

	if result.ID != memory.ID {
		t.Errorf("Expected ID '%s', got '%s'", memory.ID, result.ID)
	}

	if result.Message == "" {
		t.Error("Expected non-empty message")
	}

	// Verify the update by recalling the memory
	recallArgs := mcp.RecallMemoryArgs{
		ID: memory.ID,
	}

	recallResult, err := server.RecallMemory(ctx, recallArgs)
	if err != nil {
		t.Fatalf("RecallMemory failed: %v", err)
	}

	if !recallResult.Found {
		t.Fatal("Expected memory to be found after update")
	}

	if recallResult.Memory.Content != "Updated content" {
		t.Errorf("Expected content 'Updated content', got '%s'", recallResult.Memory.Content)
	}
}

// TestUpdateMemory_NotFound verifies error handling when updating non-existent memory
func TestUpdateMemory_NotFound(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	args := mcp.UpdateMemoryArgs{
		ID:      "mem:test:nonexistent",
		Content: "Updated content",
	}

	result, err := server.UpdateMemory(ctx, args)
	if err == nil {
		t.Fatal("Expected error for non-existent memory")
	}

	if result != nil {
		t.Error("Expected nil result on error")
	}

	// Verify error message contains "not found"
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' in error message, got: %v", err)
	}
}

// TestUpdateMemory_EmptyContent verifies error handling when all update fields are empty
func TestUpdateMemory_EmptyContent(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Store a memory first
	memory := &types.Memory{
		ID:        "mem:test:empty",
		Content:   "Original content",
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

	// Try to update with empty content, tags, and metadata
	args := mcp.UpdateMemoryArgs{
		ID:       memory.ID,
		Content:  "",
		Tags:     nil,
		Metadata: nil,
	}

	result, err := server.UpdateMemory(ctx, args)
	if err == nil {
		t.Fatal("Expected error when all fields are empty")
	}

	if result != nil {
		t.Error("Expected nil result on error")
	}
}

// TestUpdateMemory_UpdateTags verifies that tags can be updated independently
func TestUpdateMemory_UpdateTags(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Store a memory first
	memory := &types.Memory{
		ID:        "mem:test:tags",
		Content:   "Content with tags",
		Source:    "test",
		Domain:    "testing",
		Tags:      []string{"old-tag"},
		Status:    types.StatusEnriched,
		Timestamp: time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := store.Store(ctx, memory); err != nil {
		t.Fatalf("Failed to store test memory: %v", err)
	}

	// Update only tags
	args := mcp.UpdateMemoryArgs{
		ID:   memory.ID,
		Tags: []string{"new-tag", "another-tag"},
	}

	result, err := server.UpdateMemory(ctx, args)
	if err != nil {
		t.Fatalf("UpdateMemory failed: %v", err)
	}

	if !result.Updated {
		t.Error("Expected Updated to be true")
	}

	// Verify the update
	recallArgs := mcp.RecallMemoryArgs{
		ID: memory.ID,
	}

	recallResult, err := server.RecallMemory(ctx, recallArgs)
	if err != nil {
		t.Fatalf("RecallMemory failed: %v", err)
	}

	if len(recallResult.Memory.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(recallResult.Memory.Tags))
	}

	// Content should remain unchanged
	if recallResult.Memory.Content != "Content with tags" {
		t.Errorf("Expected content unchanged, got '%s'", recallResult.Memory.Content)
	}
}

// TestUpdateMemory_UpdateMetadata verifies that metadata can be updated independently
func TestUpdateMemory_UpdateMetadata(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Store a memory first
	memory := &types.Memory{
		ID:        "mem:test:metadata",
		Content:   "Content with metadata",
		Source:    "test",
		Domain:    "testing",
		Metadata:  map[string]interface{}{"old_key": "old_value"},
		Status:    types.StatusEnriched,
		Timestamp: time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := store.Store(ctx, memory); err != nil {
		t.Fatalf("Failed to store test memory: %v", err)
	}

	// Update only metadata
	args := mcp.UpdateMemoryArgs{
		ID: memory.ID,
		Metadata: map[string]interface{}{
			"new_key": "new_value",
		},
	}

	result, err := server.UpdateMemory(ctx, args)
	if err != nil {
		t.Fatalf("UpdateMemory failed: %v", err)
	}

	if !result.Updated {
		t.Error("Expected Updated to be true")
	}

	// Verify the update
	recallArgs := mcp.RecallMemoryArgs{
		ID: memory.ID,
	}

	recallResult, err := server.RecallMemory(ctx, recallArgs)
	if err != nil {
		t.Fatalf("RecallMemory failed: %v", err)
	}

	if recallResult.Memory.Metadata["new_key"] != "new_value" {
		t.Errorf("Expected metadata new_key='new_value', got '%v'", recallResult.Memory.Metadata["new_key"])
	}

	// Content should remain unchanged
	if recallResult.Memory.Content != "Content with metadata" {
		t.Errorf("Expected content unchanged, got '%s'", recallResult.Memory.Content)
	}
}

// TestConsolidateMemories_ByQuery verifies consolidation using search query
func TestConsolidateMemories_ByQuery(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Store several related memories
	memories := []*types.Memory{
		{
			ID:        "mem:test:consol1",
			Content:   "Go programming language features",
			Source:    "test",
			Domain:    "testing",
			Tags:      []string{"programming"},
			Status:    types.StatusEnriched,
			Timestamp: time.Now(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:        "mem:test:consol2",
			Content:   "Go concurrency with goroutines",
			Source:    "test",
			Domain:    "testing",
			Tags:      []string{"programming"},
			Status:    types.StatusEnriched,
			Timestamp: time.Now(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:        "mem:test:consol3",
			Content:   "Go error handling best practices",
			Source:    "test",
			Domain:    "testing",
			Tags:      []string{"programming"},
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

	// Consolidate using query
	args := mcp.ConsolidateMemoriesArgs{
		Query: "Go programming",
		Title: "Go Programming Summary",
		Limit: 10,
	}

	result, err := server.ConsolidateMemories(ctx, args)
	if err != nil {
		t.Fatalf("ConsolidateMemories failed: %v", err)
	}

	if result.NewID == "" {
		t.Error("Expected non-empty NewID")
	}

	if result.Content == "" {
		t.Error("Expected non-empty consolidated content")
	}

	if len(result.ConsolidatedIDs) < 2 {
		t.Errorf("Expected at least 2 consolidated IDs, got %d", len(result.ConsolidatedIDs))
	}

	if result.Message == "" {
		t.Error("Expected non-empty message")
	}

	// Verify the consolidated memory exists
	recallArgs := mcp.RecallMemoryArgs{
		ID: result.NewID,
	}

	recallResult, err := server.RecallMemory(ctx, recallArgs)
	if err != nil {
		t.Fatalf("RecallMemory for consolidated memory failed: %v", err)
	}

	if !recallResult.Found {
		t.Error("Expected consolidated memory to be found")
	}
}

// TestConsolidateMemories_ByExplicitIDs verifies consolidation using explicit IDs
func TestConsolidateMemories_ByExplicitIDs(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	// Store memories
	memories := []*types.Memory{
		{
			ID:        "mem:test:exp1",
			Content:   "Memory one for consolidation",
			Source:    "test",
			Domain:    "testing",
			Status:    types.StatusEnriched,
			Timestamp: time.Now(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:        "mem:test:exp2",
			Content:   "Memory two for consolidation",
			Source:    "test",
			Domain:    "testing",
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

	// Consolidate using explicit IDs
	args := mcp.ConsolidateMemoriesArgs{
		IDs:   []string{memories[0].ID, memories[1].ID},
		Title: "Consolidated Result",
	}

	result, err := server.ConsolidateMemories(ctx, args)
	if err != nil {
		t.Fatalf("ConsolidateMemories failed: %v", err)
	}

	if result.NewID == "" {
		t.Error("Expected non-empty NewID")
	}

	if len(result.ConsolidatedIDs) != 2 {
		t.Errorf("Expected 2 consolidated IDs, got %d", len(result.ConsolidatedIDs))
	}
}

// TestConsolidateMemories_Validation verifies error handling for invalid inputs
func TestConsolidateMemories_Validation(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	server := mcp.NewServer(store)

	tests := []struct {
		name    string
		args    mcp.ConsolidateMemoriesArgs
		wantErr bool
	}{
		{
			name: "neither ids nor query",
			args: mcp.ConsolidateMemoriesArgs{
				IDs:    []string{},
				Query:  "",
				Limit:  5,
			},
			wantErr: true,
		},
		{
			name: "insufficient memories",
			args: mcp.ConsolidateMemoriesArgs{
				IDs:   []string{"mem:test:single"},
				Query: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := server.ConsolidateMemories(ctx, tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Expected error: %v, got: %v", tt.wantErr, err)
			}
			if !tt.wantErr && result == nil {
				t.Error("Expected non-nil result on success")
			}
		})
	}
}

// TestUpdateMemory_WithConnectionIDInMemoryID verifies that UpdateMemory routes to the correct store
// based on the connection_id segment in the memory ID (mem:connection_id:hash format)
func TestUpdateMemory_WithConnectionIDInMemoryID(t *testing.T) {
	ctx := context.Background()

	// Create a single store for this test (connectionManager is not available for test setup)
	store := setupTestStore(t)
	defer func() { _ = store.Close() }()

	// Create an MCP server without connection manager (tests default behavior)
	server := mcp.NewServer(store)

	// Store a memory with a non-"general" domain in the memory ID
	// The format is mem:domain:hash, so mem:myapp:abc123 has domain "myapp"
	memory := &types.Memory{
		ID:        "mem:myapp:testmemory",
		Content:   "Memory with custom domain",
		Source:    "test",
		Domain:    "myapp",
		Status:    types.StatusEnriched,
		Timestamp: time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := store.Store(ctx, memory); err != nil {
		t.Fatalf("Failed to store test memory: %v", err)
	}

	// Verify recall works with the custom domain ID
	recallArgs := mcp.RecallMemoryArgs{
		ID: memory.ID,
	}

	recallResult, err := server.RecallMemory(ctx, recallArgs)
	if err != nil {
		t.Fatalf("RecallMemory failed: %v", err)
	}

	if !recallResult.Found {
		t.Error("Expected memory to be found")
	}

	// Update the memory through the MCP server
	// When no connection manager is configured, resolveStoreForID falls back to the primary store
	args := mcp.UpdateMemoryArgs{
		ID:      memory.ID,
		Content: "Updated memory with custom domain",
	}

	result, err := server.UpdateMemory(ctx, args)
	if err != nil {
		t.Fatalf("UpdateMemory failed: %v", err)
	}

	if !result.Updated {
		t.Error("Expected Updated to be true")
	}

	// Verify the update was applied
	updated, err := store.Get(ctx, memory.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve updated memory: %v", err)
	}

	if updated.Content != "Updated memory with custom domain" {
		t.Errorf("Expected updated content, got '%s'", updated.Content)
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
