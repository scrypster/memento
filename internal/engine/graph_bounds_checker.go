// Package engine provides bounded graph traversal with resource limits.
// This file implements bounds checking to prevent combinatorial explosion.
package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/scrypster/memento/internal/storage"
)

// BoundsChecker tracks and enforces graph traversal bounds to prevent
// combinatorial explosion during graph traversal operations.
//
// It monitors:
//   - Number of nodes visited
//   - Number of edges traversed
//   - Traversal depth (hops from start)
//   - Time elapsed since traversal started
//
// All checks respect context cancellation for clean shutdown.
type BoundsChecker struct {
	bounds       storage.GraphBounds
	nodesVisited int
	edgesVisited int
	startTime    time.Time
}

// BoundsStats contains statistics about graph traversal progress.
type BoundsStats struct {
	// NodesVisited is the number of nodes visited so far.
	NodesVisited int

	// EdgesVisited is the number of edges traversed so far.
	EdgesVisited int

	// Elapsed is the time elapsed since traversal started.
	Elapsed time.Duration

	// DepthReached is the maximum depth reached (set externally).
	DepthReached int
}

// NewBoundsChecker creates a new bounds checker with the given bounds.
// The bounds will be normalized to ensure valid defaults and maximums.
func NewBoundsChecker(bounds storage.GraphBounds) *BoundsChecker {
	bounds.Normalize()

	return &BoundsChecker{
		bounds:       bounds,
		nodesVisited: 0,
		edgesVisited: 0,
		startTime:    time.Now(),
	}
}

// CanVisitNode checks if we can visit another node without exceeding MaxNodes.
// Returns ErrGraphBoundsExceeded if the limit would be exceeded.
func (b *BoundsChecker) CanVisitNode() error {
	if b.nodesVisited >= b.bounds.MaxNodes {
		return fmt.Errorf("%w: max nodes (%d) exceeded", storage.ErrGraphBoundsExceeded, b.bounds.MaxNodes)
	}
	return nil
}

// CanTraverseEdge checks if we can traverse another edge without exceeding MaxEdges.
// Returns ErrGraphBoundsExceeded if the limit would be exceeded.
func (b *BoundsChecker) CanTraverseEdge() error {
	if b.edgesVisited >= b.bounds.MaxEdges {
		return fmt.Errorf("%w: max edges (%d) exceeded", storage.ErrGraphBoundsExceeded, b.bounds.MaxEdges)
	}
	return nil
}

// CanContinue checks all bounds (nodes, edges, depth, timeout, context) to determine
// if traversal can continue.
//
// Returns:
//   - nil if traversal can continue
//   - ErrGraphBoundsExceeded if any bound is exceeded
//   - context.Canceled or context.DeadlineExceeded if context is done
func (b *BoundsChecker) CanContinue(ctx context.Context, depth int) error {
	// Check context first (highest priority)
	select {
	case <-ctx.Done():
		return fmt.Errorf("context cancelled during graph traversal: %w", ctx.Err())
	default:
	}

	// Check node limit
	if b.nodesVisited >= b.bounds.MaxNodes {
		return fmt.Errorf("%w: max nodes (%d) exceeded", storage.ErrGraphBoundsExceeded, b.bounds.MaxNodes)
	}

	// Check edge limit
	if b.edgesVisited >= b.bounds.MaxEdges {
		return fmt.Errorf("%w: max edges (%d) exceeded", storage.ErrGraphBoundsExceeded, b.bounds.MaxEdges)
	}

	// Check depth limit
	if depth >= b.bounds.MaxHops {
		return fmt.Errorf("%w: max hops (%d) exceeded at depth %d", storage.ErrGraphBoundsExceeded, b.bounds.MaxHops, depth)
	}

	// Check timeout
	elapsed := time.Since(b.startTime)
	if elapsed >= b.bounds.Timeout {
		return fmt.Errorf("%w: timeout (%v) exceeded after %v", storage.ErrGraphBoundsExceeded, b.bounds.Timeout, elapsed)
	}

	return nil
}

// RecordNode increments the node counter.
// This should be called each time a node is visited.
func (b *BoundsChecker) RecordNode() {
	b.nodesVisited++
}

// RecordEdge increments the edge counter.
// This should be called each time an edge is traversed.
func (b *BoundsChecker) RecordEdge() {
	b.edgesVisited++
}

// Stats returns current traversal statistics.
// The DepthReached field must be tracked externally by the traversal algorithm.
func (b *BoundsChecker) Stats() BoundsStats {
	return BoundsStats{
		NodesVisited: b.nodesVisited,
		EdgesVisited: b.edgesVisited,
		Elapsed:      time.Since(b.startTime),
		DepthReached: 0, // Must be set by caller
	}
}
