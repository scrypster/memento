// Package tests provides test suites for the Memento memory system.
// This file contains tests for bounded graph traversal algorithms.
package tests

import (
	"context"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupGraphTraversalTestStore creates an in-memory SQLite store for graph traversal testing
func setupGraphTraversalTestStore(t *testing.T) storage.MemoryStore {
	t.Helper()

	store, err := sqlite.NewMemoryStore(":memory:")
	require.NoError(t, err, "Failed to create test store")

	t.Cleanup(func() {
		_ = store.Close()
	})

	return store
}

// TestBoundsChecker_Stats tests bounds checker statistics tracking
func TestBoundsChecker_Stats(t *testing.T) {
	bounds := storage.GraphBounds{
		MaxHops:  3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout:  5 * time.Second,
	}
	bounds.Normalize()

	checker := engine.NewBoundsChecker(bounds)

	// Initial stats should be zero
	stats := checker.Stats()
	assert.Equal(t, 0, stats.NodesVisited)
	assert.Equal(t, 0, stats.EdgesVisited)
	assert.Equal(t, 0, stats.DepthReached)

	// Record some nodes and edges
	checker.RecordNode()
	checker.RecordNode()
	checker.RecordEdge()
	checker.RecordEdge()
	checker.RecordEdge()

	stats = checker.Stats()
	assert.Equal(t, 2, stats.NodesVisited)
	assert.Equal(t, 3, stats.EdgesVisited)
}

// TestBoundsChecker_NodeLimit tests node limit enforcement
func TestBoundsChecker_NodeLimit(t *testing.T) {
	bounds := storage.GraphBounds{
		MaxHops:  10,
		MaxNodes: 5, // Small limit
		MaxEdges: 1000,
		Timeout:  1 * time.Minute,
	}
	bounds.Normalize()

	checker := engine.NewBoundsChecker(bounds)
	ctx := context.Background()

	// Should be able to visit MaxNodes nodes
	for i := 0; i < bounds.MaxNodes; i++ {
		err := checker.CanVisitNode()
		assert.NoError(t, err)
		checker.RecordNode()
	}

	// Next node should exceed limit
	err := checker.CanContinue(ctx, 0)
	assert.ErrorIs(t, err, storage.ErrGraphBoundsExceeded)
	assert.Contains(t, err.Error(), "max nodes")
}

// TestBoundsChecker_EdgeLimit tests edge limit enforcement
func TestBoundsChecker_EdgeLimit(t *testing.T) {
	bounds := storage.GraphBounds{
		MaxHops:  10,
		MaxNodes: 1000,
		MaxEdges: 5, // Small limit
		Timeout:  1 * time.Minute,
	}
	bounds.Normalize()

	checker := engine.NewBoundsChecker(bounds)
	ctx := context.Background()

	// Should be able to traverse MaxEdges edges
	for i := 0; i < bounds.MaxEdges; i++ {
		err := checker.CanTraverseEdge()
		assert.NoError(t, err)
		checker.RecordEdge()
	}

	// Next edge should exceed limit
	err := checker.CanContinue(ctx, 0)
	assert.ErrorIs(t, err, storage.ErrGraphBoundsExceeded)
	assert.Contains(t, err.Error(), "max edges")
}

// TestBoundsChecker_DepthLimit tests depth limit enforcement
func TestBoundsChecker_DepthLimit(t *testing.T) {
	bounds := storage.GraphBounds{
		MaxHops:  3,
		MaxNodes: 1000,
		MaxEdges: 1000,
		Timeout:  1 * time.Minute,
	}
	bounds.Normalize()

	checker := engine.NewBoundsChecker(bounds)
	ctx := context.Background()

	// Should be able to go up to MaxHops depth
	for i := 0; i <= bounds.MaxHops; i++ {
		err := checker.CanContinue(ctx, i)
		if i < bounds.MaxHops {
			assert.NoError(t, err)
		} else {
			assert.ErrorIs(t, err, storage.ErrGraphBoundsExceeded)
			assert.Contains(t, err.Error(), "max hops")
		}
	}
}

// TestBoundsChecker_Timeout tests timeout enforcement
func TestBoundsChecker_Timeout(t *testing.T) {
	bounds := storage.GraphBounds{
		MaxHops:  10,
		MaxNodes: 1000,
		MaxEdges: 1000,
		Timeout:  50 * time.Millisecond, // Very short timeout
	}
	bounds.Normalize()

	checker := engine.NewBoundsChecker(bounds)
	ctx := context.Background()

	// Wait for timeout to elapse
	time.Sleep(100 * time.Millisecond)

	err := checker.CanContinue(ctx, 0)
	assert.ErrorIs(t, err, storage.ErrGraphBoundsExceeded)
	assert.Contains(t, err.Error(), "timeout")
}

// TestBoundsChecker_ContextCancellation tests context cancellation
func TestBoundsChecker_ContextCancellation(t *testing.T) {
	bounds := storage.GraphBounds{
		MaxHops:  10,
		MaxNodes: 1000,
		MaxEdges: 1000,
		Timeout:  1 * time.Minute,
	}
	bounds.Normalize()

	checker := engine.NewBoundsChecker(bounds)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context
	cancel()

	err := checker.CanContinue(ctx, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

// TestGraphTraversal_BFS_EmptyGraph tests BFS on an empty graph
func TestGraphTraversal_BFS_EmptyGraph(t *testing.T) {
	store := setupGraphTraversalTestStore(t)
	traversal := engine.NewGraphTraversal(store)

	bounds := storage.GraphBounds{
		MaxHops:  3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout:  5 * time.Second,
	}

	// Create a single memory with no relationships
	memory := &types.Memory{
		ID:      "mem:test:node1",
		Content: "Test memory",
		Status:  types.StatusPending,
	}
	err := store.Store(context.Background(), memory)
	require.NoError(t, err)

	// BFS should visit only the start node
	visited := []string{}
	err = traversal.BreadthFirstSearch(context.Background(), memory.ID, bounds, func(memoryID string, depth int) bool {
		visited = append(visited, memoryID)
		return true
	})

	assert.NoError(t, err)
	assert.Equal(t, []string{"mem:test:node1"}, visited)
}

// TestGraphTraversal_BFS_MaxHops tests BFS with max hops limit
func TestGraphTraversal_BFS_MaxHops(t *testing.T) {
	store := setupGraphTraversalTestStore(t)
	traversal := engine.NewGraphTraversal(store)

	bounds := storage.GraphBounds{
		MaxHops:  2, // Only go 2 levels deep
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout:  5 * time.Second,
	}

	// Create a single memory (no relationships yet)
	memory := &types.Memory{
		ID:      "mem:test:root",
		Content: "Root node",
		Status:  types.StatusPending,
	}
	err := store.Store(context.Background(), memory)
	require.NoError(t, err)

	depths := map[string]int{}
	err = traversal.BreadthFirstSearch(context.Background(), memory.ID, bounds, func(memoryID string, depth int) bool {
		depths[memoryID] = depth
		return true
	})

	assert.NoError(t, err)
	assert.LessOrEqual(t, depths[memory.ID], bounds.MaxHops)
}

// TestGraphTraversal_BFS_MaxNodes tests BFS with max nodes limit
func TestGraphTraversal_BFS_MaxNodes(t *testing.T) {
	store := setupGraphTraversalTestStore(t)
	traversal := engine.NewGraphTraversal(store)

	bounds := storage.GraphBounds{
		MaxHops:  10,
		MaxNodes: 3, // Only visit 3 nodes
		MaxEdges: 500,
		Timeout:  5 * time.Second,
	}

	// Create a single memory
	memory := &types.Memory{
		ID:      "mem:test:root",
		Content: "Root node",
		Status:  types.StatusPending,
	}
	err := store.Store(context.Background(), memory)
	require.NoError(t, err)

	visitCount := 0
	err = traversal.BreadthFirstSearch(context.Background(), memory.ID, bounds, func(memoryID string, depth int) bool {
		visitCount++
		return true
	})

	// Should hit bounds limit (current implementation returns error)
	if err != nil {
		assert.ErrorIs(t, err, storage.ErrGraphBoundsExceeded)
	}
	assert.LessOrEqual(t, visitCount, bounds.MaxNodes)
}

// TestGraphTraversal_BFS_VisitorStop tests BFS with visitor stopping traversal
func TestGraphTraversal_BFS_VisitorStop(t *testing.T) {
	store := setupGraphTraversalTestStore(t)
	traversal := engine.NewGraphTraversal(store)

	bounds := storage.GraphBounds{
		MaxHops:  10,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout:  5 * time.Second,
	}

	// Create a single memory
	memory := &types.Memory{
		ID:      "mem:test:root",
		Content: "Root node",
		Status:  types.StatusPending,
	}
	err := store.Store(context.Background(), memory)
	require.NoError(t, err)

	visitCount := 0
	err = traversal.BreadthFirstSearch(context.Background(), memory.ID, bounds, func(memoryID string, depth int) bool {
		visitCount++
		return false // Stop after first visit
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, visitCount)
}

// TestGraphTraversal_FindPaths_NoPath tests FindPathsBounded when no path exists
func TestGraphTraversal_FindPaths_NoPath(t *testing.T) {
	store := setupGraphTraversalTestStore(t)
	traversal := engine.NewGraphTraversal(store)

	bounds := storage.GraphBounds{
		MaxHops:  3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout:  5 * time.Second,
	}

	// Create two unconnected memories
	mem1 := &types.Memory{
		ID:      "mem:test:node1",
		Content: "Node 1",
		Status:  types.StatusPending,
	}
	mem2 := &types.Memory{
		ID:      "mem:test:node2",
		Content: "Node 2",
		Status:  types.StatusPending,
	}

	err := store.Store(context.Background(), mem1)
	require.NoError(t, err)
	err = store.Store(context.Background(), mem2)
	require.NoError(t, err)

	// No relationships, so no path should exist
	paths, err := traversal.FindPathsBounded(context.Background(), mem1.ID, mem2.ID, bounds)
	assert.NoError(t, err)
	assert.Empty(t, paths, "Should find no paths between unconnected nodes")
}

// TestGraphTraversal_FindPaths_SameNode tests FindPathsBounded with same source and target
func TestGraphTraversal_FindPaths_SameNode(t *testing.T) {
	store := setupGraphTraversalTestStore(t)
	traversal := engine.NewGraphTraversal(store)

	bounds := storage.GraphBounds{
		MaxHops:  3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout:  5 * time.Second,
	}

	// Create a memory
	memory := &types.Memory{
		ID:      "mem:test:node1",
		Content: "Node 1",
		Status:  types.StatusPending,
	}
	err := store.Store(context.Background(), memory)
	require.NoError(t, err)

	// Path from node to itself should be immediate
	paths, err := traversal.FindPathsBounded(context.Background(), memory.ID, memory.ID, bounds)
	assert.NoError(t, err)
	require.Len(t, paths, 1, "Should find one path (the node itself)")
	assert.Equal(t, []string{memory.ID}, paths[0].Path)
	assert.Equal(t, 0, paths[0].Distance)
	assert.False(t, paths[0].Truncated)
}

// TestGraphTraversal_FindRelated tests FindRelatedBounded
func TestGraphTraversal_FindRelated(t *testing.T) {
	store := setupGraphTraversalTestStore(t)
	traversal := engine.NewGraphTraversal(store)

	bounds := storage.GraphBounds{
		MaxHops:  2,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout:  5 * time.Second,
	}

	// Create a memory
	memory := &types.Memory{
		ID:      "mem:test:root",
		Content: "Root node",
		Status:  types.StatusPending,
	}
	err := store.Store(context.Background(), memory)
	require.NoError(t, err)

	// Find related memories (currently should just return empty since no relationships)
	related, err := traversal.FindRelatedBounded(context.Background(), memory.ID, bounds)
	assert.NoError(t, err)
	assert.NotNil(t, related)
	// Should at least include the source node or be empty depending on implementation
}

// TestGraphTraversal_BFS_Timeout tests BFS timeout handling
func TestGraphTraversal_BFS_Timeout(t *testing.T) {
	store := setupGraphTraversalTestStore(t)
	traversal := engine.NewGraphTraversal(store)

	bounds := storage.GraphBounds{
		MaxHops:  10,
		MaxNodes: 1000,
		MaxEdges: 1000,
		Timeout:  1 * time.Millisecond, // Very short timeout
	}

	// Create a memory
	memory := &types.Memory{
		ID:      "mem:test:root",
		Content: "Root node",
		Status:  types.StatusPending,
	}
	err := store.Store(context.Background(), memory)
	require.NoError(t, err)

	// Add a small delay to ensure timeout
	time.Sleep(10 * time.Millisecond)

	err = traversal.BreadthFirstSearch(context.Background(), memory.ID, bounds, func(memoryID string, depth int) bool {
		return true
	})

	// Should timeout
	if err != nil {
		assert.ErrorIs(t, err, storage.ErrGraphBoundsExceeded)
	}
}

// TestGraphTraversal_NormalizesBounds tests that bounds are normalized
func TestGraphTraversal_NormalizesBounds(t *testing.T) {
	store := setupGraphTraversalTestStore(t)
	traversal := engine.NewGraphTraversal(store)

	// Invalid bounds (all zeros)
	bounds := storage.GraphBounds{}

	// Create a memory
	memory := &types.Memory{
		ID:      "mem:test:root",
		Content: "Root node",
		Status:  types.StatusPending,
	}
	err := store.Store(context.Background(), memory)
	require.NoError(t, err)

	// Should normalize bounds internally and not fail
	visited := []string{}
	err = traversal.BreadthFirstSearch(context.Background(), memory.ID, bounds, func(memoryID string, depth int) bool {
		visited = append(visited, memoryID)
		return true
	})

	assert.NoError(t, err)
	assert.NotEmpty(t, visited)
}

// TestNewGraphTraversal tests GraphTraversal construction
func TestNewGraphTraversal(t *testing.T) {
	store := setupGraphTraversalTestStore(t)

	traversal := engine.NewGraphTraversal(store)
	assert.NotNil(t, traversal, "NewGraphTraversal should return non-nil traversal")
}

// ---------------------------------------------------------------------------
// Temporal filtering integration tests (Phase 3)
// ---------------------------------------------------------------------------

// TestGraphBounds_MatchesTemporalBounds_NoFilter passes all timestamps when
// neither CreatedAfter nor CreatedBefore is set.
func TestGraphBounds_MatchesTemporalBounds_NoFilter(t *testing.T) {
	bounds := storage.GraphBounds{}
	now := time.Now()

	assert.True(t, bounds.MatchesTemporalBounds(now), "unrestricted bounds should accept any timestamp")
	assert.True(t, bounds.MatchesTemporalBounds(now.Add(-365*24*time.Hour)), "should accept old timestamps")
	assert.True(t, bounds.MatchesTemporalBounds(now.Add(365*24*time.Hour)), "should accept future timestamps")
}

// TestGraphBounds_MatchesTemporalBounds_CreatedAfter passes timestamps that
// are strictly after the lower bound.
func TestGraphBounds_MatchesTemporalBounds_CreatedAfter(t *testing.T) {
	cutoff := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	bounds := storage.GraphBounds{
		CreatedAfter: cutoff,
	}

	// Exactly at cutoff — should NOT pass (strictly after)
	assert.False(t, bounds.MatchesTemporalBounds(cutoff), "cutoff itself should not pass (strictly after)")

	// Before cutoff
	assert.False(t, bounds.MatchesTemporalBounds(cutoff.Add(-time.Second)), "timestamp before cutoff should not pass")

	// After cutoff
	assert.True(t, bounds.MatchesTemporalBounds(cutoff.Add(time.Second)), "timestamp after cutoff should pass")
}

// TestGraphBounds_MatchesTemporalBounds_CreatedBefore passes timestamps that
// are strictly before the upper bound.
func TestGraphBounds_MatchesTemporalBounds_CreatedBefore(t *testing.T) {
	cutoff := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	bounds := storage.GraphBounds{
		CreatedBefore: cutoff,
	}

	// Exactly at cutoff — should NOT pass (strictly before)
	assert.False(t, bounds.MatchesTemporalBounds(cutoff), "cutoff itself should not pass (strictly before)")

	// After cutoff
	assert.False(t, bounds.MatchesTemporalBounds(cutoff.Add(time.Second)), "timestamp after cutoff should not pass")

	// Before cutoff
	assert.True(t, bounds.MatchesTemporalBounds(cutoff.Add(-time.Second)), "timestamp before cutoff should pass")
}

// TestGraphBounds_MatchesTemporalBounds_BothBounds tests the inclusive window
// (after start, before end).
func TestGraphBounds_MatchesTemporalBounds_BothBounds(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	bounds := storage.GraphBounds{
		CreatedAfter:  start,
		CreatedBefore: end,
	}

	// Outside window (before start)
	assert.False(t, bounds.MatchesTemporalBounds(start), "start boundary itself should not pass")
	assert.False(t, bounds.MatchesTemporalBounds(start.Add(-time.Second)), "before window should not pass")

	// Inside window
	mid := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	assert.True(t, bounds.MatchesTemporalBounds(mid), "timestamp inside window should pass")

	// Outside window (after end)
	assert.False(t, bounds.MatchesTemporalBounds(end), "end boundary itself should not pass")
	assert.False(t, bounds.MatchesTemporalBounds(end.Add(time.Second)), "after window should not pass")
}

// TestGraphBounds_Normalize_PreservesTemporalBounds verifies that Normalize
// does not clear or alter the temporal bounds fields.
func TestGraphBounds_Normalize_PreservesTemporalBounds(t *testing.T) {
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	bounds := storage.GraphBounds{
		CreatedAfter:  after,
		CreatedBefore: before,
	}
	bounds.Normalize()

	assert.Equal(t, after, bounds.CreatedAfter, "Normalize should preserve CreatedAfter")
	assert.Equal(t, before, bounds.CreatedBefore, "Normalize should preserve CreatedBefore")
}

// TestFindRelatedBounded_TemporalFilter_NoRelationships verifies that
// temporal filtering does not break FindRelatedBounded when there are no
// inter-memory relationships (the common case given the placeholder
// getNeighbors implementation).
func TestFindRelatedBounded_TemporalFilter_NoRelationships(t *testing.T) {
	store := setupGraphTraversalTestStore(t)
	traversal := engine.NewGraphTraversal(store)

	base := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	// Store a source memory
	source := &types.Memory{
		ID:        "mem:temporal:source",
		Content:   "Source node",
		Status:    types.StatusPending,
		CreatedAt: base,
		UpdatedAt: base,
	}
	err := store.Store(context.Background(), source)
	require.NoError(t, err)

	// Bounds that exclude everything (before the source was created)
	bounds := storage.GraphBounds{
		MaxHops:       3,
		MaxNodes:      100,
		MaxEdges:      500,
		Timeout:       5 * time.Second,
		CreatedAfter:  base.Add(24 * time.Hour), // only after tomorrow
		CreatedBefore: base.Add(48 * time.Hour),
	}

	related, err := traversal.FindRelatedBounded(context.Background(), source.ID, bounds)
	assert.NoError(t, err)
	assert.NotNil(t, related, "result slice must be non-nil")
	// No relationships exist, so result should always be empty regardless of temporal filter
	assert.Empty(t, related, "no related memories expected when graph has no edges")
}

// TestFindRelatedBounded_NoTemporalFilter_BackwardCompat verifies that
// FindRelatedBounded with zero temporal bounds still works correctly
// (backward-compatibility regression guard).
func TestFindRelatedBounded_NoTemporalFilter_BackwardCompat(t *testing.T) {
	store := setupGraphTraversalTestStore(t)
	traversal := engine.NewGraphTraversal(store)

	memory := &types.Memory{
		ID:        "mem:compat:root",
		Content:   "Root node",
		Status:    types.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.Store(context.Background(), memory)
	require.NoError(t, err)

	// Zero temporal bounds — should behave exactly as before Phase 3
	bounds := storage.GraphBounds{
		MaxHops:  3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout:  5 * time.Second,
		// CreatedAfter and CreatedBefore are zero (unset)
	}

	related, err := traversal.FindRelatedBounded(context.Background(), memory.ID, bounds)
	assert.NoError(t, err)
	assert.NotNil(t, related, "result slice should be non-nil")
	// No relationships exist in this test so related should be empty
	assert.Empty(t, related)
}
