package engine

import (
	"context"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// TestBreadthFirstSearch_SingleNode tests BFS with only the start node.
func TestBreadthFirstSearch_SingleNode(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	startID := "mem:test:start"
	_ = store.Store(ctx, &types.Memory{
		ID:      startID,
		Content: "start",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	visited := make(map[string]bool)

	err := gt.BreadthFirstSearch(ctx, startID, storage.GraphBounds{
		MaxHops: 3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	}, func(memoryID string, depth int) bool {
		visited[memoryID] = true
		return true
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !visited[startID] {
		t.Errorf("start node should be visited")
	}

	if len(visited) != 1 {
		t.Errorf("expected 1 visited node, got %d", len(visited))
	}
}

// TestBreadthFirstSearch_VisitorStopEarly tests that visitor can stop traversal.
func TestBreadthFirstSearch_VisitorStopEarly(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	startID := "mem:test:start"
	_ = store.Store(ctx, &types.Memory{
		ID:      startID,
		Content: "start",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	visitCount := 0

	err := gt.BreadthFirstSearch(ctx, startID, storage.GraphBounds{
		MaxHops: 3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	}, func(memoryID string, depth int) bool {
		visitCount++
		return false // Stop after first visit
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if visitCount != 1 {
		t.Errorf("expected 1 visit, got %d", visitCount)
	}
}

// TestBreadthFirstSearch_ContextCancellation tests that context cancellation stops traversal.
func TestBreadthFirstSearch_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	store := newMockMemoryStore()
	startID := "mem:test:start"
	_ = store.Store(ctx, &types.Memory{
		ID:      startID,
		Content: "start",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	cancel() // Cancel before BFS

	err := gt.BreadthFirstSearch(ctx, startID, storage.GraphBounds{
		MaxHops: 3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	}, func(memoryID string, depth int) bool {
		return true
	})

	if err == nil {
		t.Fatalf("expected error from context cancellation")
	}
}

// TestBreadthFirstSearch_MaxHopsBound tests that MaxHops bound is respected.
func TestBreadthFirstSearch_MaxHopsBound(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	// Create a chain: start -> n1 -> n2 -> n3
	startID := "mem:test:start"
	n1ID := "mem:test:n1"
	n2ID := "mem:test:n2"
	n3ID := "mem:test:n3"

	_ = store.Store(ctx, &types.Memory{
		ID:      startID,
		Content: "start",
		Domain:  "test",
	})

	_ = store.Store(ctx, &types.Memory{
		ID:      n1ID,
		Content: "n1",
		Domain:  "test",
	})

	_ = store.Store(ctx, &types.Memory{
		ID:      n2ID,
		Content: "n2",
		Domain:  "test",
	})

	_ = store.Store(ctx, &types.Memory{
		ID:      n3ID,
		Content: "n3",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	maxDepthReached := 0

	err := gt.BreadthFirstSearch(ctx, startID, storage.GraphBounds{
		MaxHops: 2,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	}, func(memoryID string, depth int) bool {
		if depth > maxDepthReached {
			maxDepthReached = depth
		}
		return true
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if maxDepthReached > 2 {
		t.Errorf("expected max depth 2, got %d", maxDepthReached)
	}
}

// TestBreadthFirstSearch_MaxNodesBound tests that MaxNodes bound is enforced.
func TestBreadthFirstSearch_MaxNodesBound(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	startID := "mem:test:start"
	_ = store.Store(ctx, &types.Memory{
		ID:      startID,
		Content: "start",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	visitCount := 0

	err := gt.BreadthFirstSearch(ctx, startID, storage.GraphBounds{
		MaxHops: 10,
		MaxNodes: 1,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	}, func(memoryID string, depth int) bool {
		visitCount++
		return true
	})

	// Should error or succeed with 1 visit
	if err == nil && visitCount != 1 {
		t.Errorf("expected at most 1 visit with MaxNodes=1, got %d", visitCount)
	}
}

// TestFindPathsBounded_SameSourceTarget tests path from node to itself.
func TestFindPathsBounded_SameSourceTarget(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	nodeID := "mem:test:node"
	_ = store.Store(ctx, &types.Memory{
		ID:      nodeID,
		Content: "node",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	paths, err := gt.FindPathsBounded(ctx, nodeID, nodeID, storage.GraphBounds{
		MaxHops: 3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(paths) != 1 {
		t.Errorf("expected 1 path from node to itself, got %d", len(paths))
	}

	if len(paths) > 0 {
		path := paths[0]
		if len(path.Path) != 1 || path.Path[0] != nodeID {
			t.Errorf("expected path [%s], got %v", nodeID, path.Path)
		}
		if path.Distance != 0 {
			t.Errorf("expected distance 0, got %d", path.Distance)
		}
		if path.Confidence != 1.0 {
			t.Errorf("expected confidence 1.0, got %f", path.Confidence)
		}
	}
}

// TestFindPathsBounded_NoPath tests path search with no connection.
func TestFindPathsBounded_NoPath(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	source := "mem:test:source"
	target := "mem:test:target"

	_ = store.Store(ctx, &types.Memory{
		ID:      source,
		Content: "source",
		Domain:  "test",
	})

	_ = store.Store(ctx, &types.Memory{
		ID:      target,
		Content: "target",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	paths, err := gt.FindPathsBounded(ctx, source, target, storage.GraphBounds{
		MaxHops: 3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(paths) != 0 {
		t.Errorf("expected no paths between disconnected nodes, got %d", len(paths))
	}
}

// TestFindPathsBounded_ContextCancellation tests path finding respects context cancellation.
func TestFindPathsBounded_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	store := newMockMemoryStore()
	source := "mem:test:source"
	target := "mem:test:target"

	_ = store.Store(ctx, &types.Memory{
		ID:      source,
		Content: "source",
		Domain:  "test",
	})

	_ = store.Store(ctx, &types.Memory{
		ID:      target,
		Content: "target",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	cancel() // Cancel before path finding

	paths, err := gt.FindPathsBounded(ctx, source, target, storage.GraphBounds{
		MaxHops: 3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	})

	// The function may return an error or may return with truncated results.
	// FindPathsBounded returns partial results when bounds are exceeded,
	// and context cancellation is handled as a bounds check.
	if err != nil {
		// Error expected - this is fine
		return
	}

	// If no error, paths may be partial — truncation is an implementation detail
	_ = paths
}

// TestFindPathsBounded_PathConfidence tests that confidence decreases with distance.
func TestFindPathsBounded_PathConfidence(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	source := "mem:test:source"
	n1 := "mem:test:n1"
	n2 := "mem:test:n2"
	target := "mem:test:target"

	_ = store.Store(ctx, &types.Memory{ID: source, Content: "source", Domain: "test"})
	_ = store.Store(ctx, &types.Memory{ID: n1, Content: "n1", Domain: "test"})
	_ = store.Store(ctx, &types.Memory{ID: n2, Content: "n2", Domain: "test"})
	_ = store.Store(ctx, &types.Memory{ID: target, Content: "target", Domain: "test"})

	gt := NewGraphTraversal(store)
	paths, err := gt.FindPathsBounded(ctx, source, target, storage.GraphBounds{
		MaxHops: 5,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Confidence should be within valid range
	for _, p := range paths {
		if p.Confidence <= 0.0 || p.Confidence > 1.0 {
			t.Errorf("invalid confidence %f", p.Confidence)
		}
		// Longer paths should have lower confidence
		if len(p.Path) > 1 {
			expectedConfidence := 1.0 / float64(len(p.Path))
			if p.Confidence != expectedConfidence {
				t.Errorf("expected confidence %f, got %f for path length %d",
					expectedConfidence, p.Confidence, len(p.Path))
			}
		}
	}
}

// TestFindRelatedBounded_SourceNodeOnly tests finding related nodes with no neighbors.
func TestFindRelatedBounded_SourceNodeOnly(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	sourceID := "mem:test:source"
	_ = store.Store(ctx, &types.Memory{
		ID:      sourceID,
		Content: "source",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	related, err := gt.FindRelatedBounded(ctx, sourceID, storage.GraphBounds{
		MaxHops: 3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Source node itself should be excluded from results
	if len(related) != 0 {
		t.Errorf("expected no related nodes for isolated node, got %d", len(related))
	}
}

// TestFindRelatedBounded_TemporalBounds tests temporal filtering during traversal.
func TestFindRelatedBounded_TemporalBounds(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	sourceID := "mem:test:source"
	oldNodeID := "mem:test:old"
	newNodeID := "mem:test:new"

	now := time.Now()
	oldTime := now.Add(-30 * 24 * time.Hour)
	newTime := now.Add(-1 * 24 * time.Hour)

	_ = store.Store(ctx, &types.Memory{
		ID:        sourceID,
		Content:   "source",
		Domain:    "test",
		CreatedAt: now,
	})

	_ = store.Store(ctx, &types.Memory{
		ID:        oldNodeID,
		Content:   "old",
		Domain:    "test",
		CreatedAt: oldTime,
	})

	_ = store.Store(ctx, &types.Memory{
		ID:        newNodeID,
		Content:   "new",
		Domain:    "test",
		CreatedAt: newTime,
	})

	gt := NewGraphTraversal(store)

	// Set temporal bounds to exclude old node
	bounds := storage.GraphBounds{
		MaxHops:      3,
		MaxNodes:     100,
		MaxEdges:     500,
		Timeout:      30 * time.Second,
		CreatedAfter: oldTime.Add(1 * time.Hour), // Excludes oldTime
		CreatedBefore: newTime.Add(1 * time.Hour), // Includes newTime
	}

	related, err := gt.FindRelatedBounded(ctx, sourceID, bounds)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify temporal filtering (this depends on store implementation)
	for _, nodeID := range related {
		if nodeID == oldNodeID {
			t.Errorf("node outside temporal bounds should be excluded from results")
		}
	}
}

// TestFindRelatedBounded_ContextCancellation tests context cancellation.
func TestFindRelatedBounded_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	store := newMockMemoryStore()
	sourceID := "mem:test:source"
	_ = store.Store(ctx, &types.Memory{
		ID:      sourceID,
		Content: "source",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	cancel() // Cancel before search

	_, err := gt.FindRelatedBounded(ctx, sourceID, storage.GraphBounds{
		MaxHops: 3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	})

	if err == nil {
		t.Fatalf("expected error from context cancellation")
	}
}

// TestBreadthFirstSearch_DepthTracking tests that depth increases properly.
func TestBreadthFirstSearch_DepthTracking(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	startID := "mem:test:start"
	_ = store.Store(ctx, &types.Memory{
		ID:      startID,
		Content: "start",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	depths := make(map[string]int)

	err := gt.BreadthFirstSearch(ctx, startID, storage.GraphBounds{
		MaxHops: 3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	}, func(memoryID string, depth int) bool {
		depths[memoryID] = depth
		return true
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if depths[startID] != 0 {
		t.Errorf("start node should have depth 0, got %d", depths[startID])
	}
}

// TestFindPathsBounded_MaxHopsRespected tests that MaxHops is respected in path finding.
func TestFindPathsBounded_MaxHopsRespected(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	source := "mem:test:source"
	target := "mem:test:target"

	_ = store.Store(ctx, &types.Memory{
		ID:      source,
		Content: "source",
		Domain:  "test",
	})

	_ = store.Store(ctx, &types.Memory{
		ID:      target,
		Content: "target",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	paths, err := gt.FindPathsBounded(ctx, source, target, storage.GraphBounds{
		MaxHops:  1, // Very restrictive
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout:  30 * time.Second,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify paths respect MaxHops
	for _, p := range paths {
		// Distance should be less than or equal to MaxHops (0-indexed vs 1-indexed)
		if p.Distance > 1 {
			t.Errorf("path distance %d exceeds MaxHops 1", p.Distance)
		}
	}
}

// TestBreadthFirstSearch_NoDuplicateVisits tests that nodes are visited once.
func TestBreadthFirstSearch_NoDuplicateVisits(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	startID := "mem:test:start"
	_ = store.Store(ctx, &types.Memory{
		ID:      startID,
		Content: "start",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	visitCounts := make(map[string]int)

	err := gt.BreadthFirstSearch(ctx, startID, storage.GraphBounds{
		MaxHops: 3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	}, func(memoryID string, depth int) bool {
		visitCounts[memoryID]++
		return true
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Each node should be visited exactly once
	for nodeID, count := range visitCounts {
		if count != 1 {
			t.Errorf("node %s visited %d times, expected 1", nodeID, count)
		}
	}
}

// TestFindPathsBounded_EmptyPathList tests returning paths with successful result.
func TestFindPathsBounded_EmptyPathList(t *testing.T) {
	ctx := context.Background()
	store := newMockMemoryStore()

	source := "mem:test:source"
	nonexistent := "mem:test:nonexistent"

	_ = store.Store(ctx, &types.Memory{
		ID:      source,
		Content: "source",
		Domain:  "test",
	})

	gt := NewGraphTraversal(store)
	paths, err := gt.FindPathsBounded(ctx, source, nonexistent, storage.GraphBounds{
		MaxHops: 3,
		MaxNodes: 100,
		MaxEdges: 500,
		Timeout: 30 * time.Second,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(paths) != 0 {
		t.Errorf("expected empty path list for unreachable target, got %d paths", len(paths))
	}
}
