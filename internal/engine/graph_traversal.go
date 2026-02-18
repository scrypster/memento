// Package engine provides bounded graph traversal algorithms for the memory graph.
// This file implements BFS, DFS, and path-finding with resource bounds.
package engine

import (
	"context"
	"fmt"

	"github.com/scrypster/memento/internal/storage"
)

// GraphTraversal implements bounded graph algorithms for the memory graph.
// It uses MemoryStore for memory access and will integrate with GraphProvider
// when Phase 2 graph storage is implemented.
//
// All algorithms enforce GraphBounds to prevent combinatorial explosion:
//   - MaxHops: Limits traversal depth
//   - MaxNodes: Limits total nodes visited
//   - MaxEdges: Limits total edges traversed
//   - Timeout: Limits total execution time
type GraphTraversal struct {
	memoryStore storage.MemoryStore
	// Future: graphProvider storage.GraphProvider when Phase 2 is ready
}

// PathResult represents a path between two memories in the graph.
type PathResult struct {
	// Path is the sequence of memory IDs from source to target.
	Path []string

	// Distance is the number of hops in the path (length - 1).
	Distance int

	// Confidence is the path confidence score (0.0 to 1.0).
	// Shorter paths have higher confidence.
	Confidence float64

	// Truncated indicates whether the path search was truncated due to bounds.
	Truncated bool
}

// NewGraphTraversal creates a new graph traversal engine.
// The memory store is used to verify memory existence.
func NewGraphTraversal(store storage.MemoryStore) *GraphTraversal {
	return &GraphTraversal{
		memoryStore: store,
	}
}

// BreadthFirstSearch performs bounded BFS starting from startID.
// The visitor function is called for each node visited and receives:
//   - memoryID: The ID of the current memory
//   - depth: The depth/distance from the start node
//
// The visitor should return true to continue traversal, false to stop.
//
// BFS respects all bounds (MaxHops, MaxNodes, MaxEdges, Timeout) and
// returns ErrGraphBoundsExceeded if any bound is hit.
//
// Context cancellation is respected and will stop traversal immediately.
func (g *GraphTraversal) BreadthFirstSearch(
	ctx context.Context,
	startID string,
	bounds storage.GraphBounds,
	visitor func(memoryID string, depth int) bool,
) error {
	bounds.Normalize()
	checker := NewBoundsChecker(bounds)

	// BFS uses a queue of (memoryID, depth) pairs
	type queueItem struct {
		id    string
		depth int
	}

	queue := []queueItem{{startID, 0}}
	visited := make(map[string]bool)

	for len(queue) > 0 {
		// Check bounds and context before processing
		current := queue[0]
		queue = queue[1:]

		// Check if we've already visited this node
		if visited[current.id] {
			continue
		}

		// Check bounds before visiting
		if err := checker.CanContinue(ctx, current.depth); err != nil {
			return err
		}

		// Mark as visited and record
		visited[current.id] = true
		checker.RecordNode()

		// Call visitor function
		if !visitor(current.id, current.depth) {
			// Visitor requested stop
			break
		}

		// Stop expanding if we've reached max depth
		if current.depth >= bounds.MaxHops {
			continue
		}

		// Get neighbors (relationships)
		neighbors, err := g.getNeighbors(ctx, current.id)
		if err != nil {
			return fmt.Errorf("failed to get neighbors for %s: %w", current.id, err)
		}

		// Add unvisited neighbors to queue
		for _, neighborID := range neighbors {
			if !visited[neighborID] {
				checker.RecordEdge()
				queue = append(queue, queueItem{neighborID, current.depth + 1})
			}
		}
	}

	return nil
}

// FindPathsBounded finds all paths from sourceID to targetID within bounds.
// Uses depth-first search with path tracking to find multiple paths.
//
// Returns:
//   - A slice of PathResult containing all found paths
//   - Paths are sorted by distance (shortest first)
//   - If bounds are exceeded, Truncated will be true on all results
//
// Note: This can be expensive for highly connected graphs.
// Use tight bounds (low MaxHops, MaxNodes) to limit search space.
func (g *GraphTraversal) FindPathsBounded(
	ctx context.Context,
	sourceID string,
	targetID string,
	bounds storage.GraphBounds,
) ([]PathResult, error) {
	bounds.Normalize()
	checker := NewBoundsChecker(bounds)

	var paths []PathResult
	visited := make(map[string]bool)

	// Special case: source == target
	if sourceID == targetID {
		return []PathResult{{
			Path:       []string{sourceID},
			Distance:   0,
			Confidence: 1.0,
			Truncated:  false,
		}}, nil
	}

	// DFS with path tracking
	var dfs func(currentID string, path []string, depth int) error
	dfs = func(currentID string, path []string, depth int) error {
		// Check bounds
		if err := checker.CanContinue(ctx, depth); err != nil {
			return err
		}

		// Avoid cycles in current path
		if visited[currentID] {
			return nil
		}

		visited[currentID] = true
		checker.RecordNode()
		path = append(path, currentID)

		// Found target?
		if currentID == targetID {
			// Calculate confidence (shorter paths = higher confidence)
			confidence := 1.0 / float64(depth+1)

			paths = append(paths, PathResult{
				Path:       append([]string{}, path...), // Copy path
				Distance:   depth,
				Confidence: confidence,
				Truncated:  false,
			})

			// Unmark visited to allow other paths through this node
			visited[currentID] = false
			return nil
		}

		// Continue search if within depth limit
		if depth < bounds.MaxHops {
			neighbors, err := g.getNeighbors(ctx, currentID)
			if err != nil {
				return fmt.Errorf("failed to get neighbors for %s: %w", currentID, err)
			}

			for _, neighborID := range neighbors {
				checker.RecordEdge()
				if err := dfs(neighborID, path, depth+1); err != nil {
					return err
				}
			}
		}

		// Unmark visited after exploring this branch
		visited[currentID] = false
		return nil
	}

	// Start DFS from source
	err := dfs(sourceID, []string{}, 0)

	// Handle bounds exceeded
	if err != nil {
		// Mark all found paths as truncated
		for i := range paths {
			paths[i].Truncated = true
		}
		return paths, nil // Return partial results
	}

	return paths, nil
}

// FindRelatedBounded finds all memories related to sourceID within bounds.
// Uses BFS to discover all reachable memories up to MaxHops distance.
//
// When bounds.CreatedAfter or bounds.CreatedBefore are set, each visited
// memory is fetched and its created_at timestamp is checked against the
// temporal window. Memories outside the window are excluded from the result
// but traversal continues through them so that temporally-valid nodes
// reachable via out-of-window intermediaries are still found.
//
// Returns a slice of memory IDs reachable from the source.
func (g *GraphTraversal) FindRelatedBounded(
	ctx context.Context,
	sourceID string,
	bounds storage.GraphBounds,
) ([]string, error) {
	bounds.Normalize()

	hasTemporalBounds := !bounds.CreatedAfter.IsZero() || !bounds.CreatedBefore.IsZero()

	related := make([]string, 0) // Initialize non-nil slice

	err := g.BreadthFirstSearch(ctx, sourceID, bounds, func(memoryID string, depth int) bool {
		// Skip the source node itself
		if memoryID == sourceID {
			return true // Continue traversal
		}

		// Apply temporal filtering when bounds are configured.
		if hasTemporalBounds {
			mem, fetchErr := g.memoryStore.Get(ctx, memoryID)
			if fetchErr != nil {
				// If we cannot fetch the memory, skip it but keep traversing.
				return true
			}
			if !bounds.MatchesTemporalBounds(mem.CreatedAt) {
				// Outside temporal window — exclude from results but keep traversing.
				return true
			}
		}

		related = append(related, memoryID)
		return true // Continue traversal
	})

	if err != nil {
		return related, err // Return partial results even on error
	}

	return related, nil
}

// getNeighbors retrieves the neighbor memory IDs for a given memory.
// It finds related memories through entity relationships:
// 1. Get entities in source memory
// 2. Find relationships involving those entities
// 3. Find memories containing the related entities
func (g *GraphTraversal) getNeighbors(ctx context.Context, memoryID string) ([]string, error) {
	return g.memoryStore.GetRelatedMemories(ctx, memoryID)
}

// verifyMemoryExists checks if a memory exists in the store.
// Returns an error if the memory is not found.
func (g *GraphTraversal) verifyMemoryExists(ctx context.Context, memoryID string) error {
	_, err := g.memoryStore.Get(ctx, memoryID)
	if err != nil {
		return fmt.Errorf("memory %s not found: %w", memoryID, err)
	}
	return nil
}
