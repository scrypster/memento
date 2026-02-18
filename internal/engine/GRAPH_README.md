# Graph Traversal Algorithms - Bounded Graph Operations

This package implements bounded graph traversal algorithms for the Memento memory graph. All algorithms enforce strict resource limits to prevent combinatorial explosion during graph operations.

## Overview

The graph traversal system provides:

- **Bounded Breadth-First Search (BFS)**: Explore the graph level by level
- **Bounded Depth-First Search (DFS)**: Find paths between memories
- **Related Memory Discovery**: Find all memories connected to a source
- **Resource Bounds Enforcement**: Prevent runaway queries with limits

## Key Components

### GraphTraversal

The main traversal engine that provides bounded graph algorithms.

```go
store := sqlite.NewMemoryStore("memento.db")
traversal := engine.NewGraphTraversal(store)
```

**Methods:**

- `BreadthFirstSearch()` - Level-by-level graph exploration
- `FindPathsBounded()` - Find all paths between two memories
- `FindRelatedBounded()` - Discover connected memories

### BoundsChecker

Tracks and enforces resource limits during traversal.

```go
bounds := storage.GraphBounds{
    MaxHops:  3,           // Maximum depth
    MaxNodes: 100,         // Maximum nodes to visit
    MaxEdges: 500,         // Maximum edges to traverse
    Timeout:  5 * time.Second,
}

checker := engine.NewBoundsChecker(bounds)
```

**Methods:**

- `CanVisitNode()` - Check if we can visit another node
- `CanTraverseEdge()` - Check if we can traverse another edge
- `CanContinue()` - Check all bounds (nodes, edges, depth, timeout)
- `RecordNode()` - Increment node counter
- `RecordEdge()` - Increment edge counter
- `Stats()` - Get current traversal statistics

## GraphBounds

Resource limits to prevent combinatorial explosion:

```go
type GraphBounds struct {
    MaxHops   int           // Max depth (default: 3, max: 10)
    MaxNodes  int           // Max nodes visited (default: 100, max: 1000)
    MaxEdges  int           // Max edges traversed (default: 500, max: 5000)
    Timeout   time.Duration // Max time (default: 30s, max: 5m)
}
```

The `Normalize()` method sets defaults and enforces maximums automatically.

## Usage Examples

### 1. Breadth-First Search

Explore the graph level by level with custom visitor logic:

```go
bounds := storage.GraphBounds{
    MaxHops:  3,
    MaxNodes: 100,
    MaxEdges: 500,
    Timeout:  5 * time.Second,
}

visited := []string{}
err := traversal.BreadthFirstSearch(
    ctx,
    "mem:project:start",
    bounds,
    func(memoryID string, depth int) bool {
        visited = append(visited, memoryID)
        fmt.Printf("Visited %s at depth %d\n", memoryID, depth)
        return true // Continue traversal
    },
)
```

**Visitor Return Values:**
- Return `true` to continue traversal
- Return `false` to stop traversal early

### 2. Find Paths Between Memories

Find all paths from source to target:

```go
paths, err := traversal.FindPathsBounded(
    ctx,
    "mem:project:source",
    "mem:project:target",
    bounds,
)

if err != nil {
    log.Printf("Error: %v", err)
}

for _, path := range paths {
    fmt.Printf("Path (distance=%d, confidence=%.2f): %v\n",
        path.Distance,
        path.Confidence,
        path.Path,
    )

    if path.Truncated {
        fmt.Println("  (search was truncated due to bounds)")
    }
}
```

**PathResult Fields:**
- `Path` - Sequence of memory IDs from source to target
- `Distance` - Number of hops (path length - 1)
- `Confidence` - Path confidence (1.0 / (distance + 1))
- `Truncated` - True if bounds were exceeded during search

### 3. Find Related Memories

Discover all memories connected to a source:

```go
related, err := traversal.FindRelatedBounded(
    ctx,
    "mem:project:root",
    bounds,
)

if err != nil {
    log.Printf("Error: %v", err)
}

fmt.Printf("Found %d related memories\n", len(related))
for _, memoryID := range related {
    fmt.Printf("  - %s\n", memoryID)
}
```

## Error Handling

### Bounds Exceeded

When bounds are exceeded, `storage.ErrGraphBoundsExceeded` is returned:

```go
err := traversal.BreadthFirstSearch(ctx, startID, bounds, visitor)
if errors.Is(err, storage.ErrGraphBoundsExceeded) {
    if strings.Contains(err.Error(), "max nodes") {
        // Node limit hit
    } else if strings.Contains(err.Error(), "max edges") {
        // Edge limit hit
    } else if strings.Contains(err.Error(), "max hops") {
        // Depth limit hit
    } else if strings.Contains(err.Error(), "timeout") {
        // Timeout exceeded
    }
}
```

### Context Cancellation

All algorithms respect context cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := traversal.BreadthFirstSearch(ctx, startID, bounds, visitor)
if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
    // Context was cancelled
}
```

### Partial Results

`FindPathsBounded()` and `FindRelatedBounded()` return partial results when bounds are hit:

```go
paths, err := traversal.FindPathsBounded(ctx, sourceID, targetID, bounds)
// paths contains all paths found before bounds were exceeded
// Check path.Truncated to see if search was incomplete
```

## Algorithm Details

### Breadth-First Search (BFS)

**Algorithm:** Level-order traversal using a queue

**Time Complexity:** O(V + E) where V = nodes, E = edges

**Space Complexity:** O(V) for visited set and queue

**Best For:**
- Finding shortest paths
- Level-by-level exploration
- Discovering nearby memories

**Implementation:**
1. Start with source node at depth 0
2. Process nodes level by level
3. Mark visited to avoid cycles
4. Stop when MaxHops depth reached
5. Check bounds before each operation

### Depth-First Search (DFS) for Paths

**Algorithm:** Recursive path exploration with backtracking

**Time Complexity:** O(V^MaxHops) worst case (exponential)

**Space Complexity:** O(MaxHops) for recursion stack

**Best For:**
- Finding all paths (not just shortest)
- Path enumeration
- Connectivity analysis

**Implementation:**
1. Start DFS from source
2. Track current path
3. When target found, record path
4. Backtrack to explore other paths
5. Unmark visited nodes to allow multiple paths

## Resource Management

### Why Bounds Matter

Without bounds, graph traversal can explode:

- **No MaxHops**: Could traverse entire graph
- **No MaxNodes**: Could visit millions of nodes
- **No MaxEdges**: Could follow billions of edges
- **No Timeout**: Could run indefinitely

### Setting Appropriate Bounds

**For Discovery (BFS):**
```go
bounds := storage.GraphBounds{
    MaxHops:  2,              // Nearby memories only
    MaxNodes: 50,             // Limited exploration
    MaxEdges: 200,            // Few connections
    Timeout:  2 * time.Second,
}
```

**For Path Finding (DFS):**
```go
bounds := storage.GraphBounds{
    MaxHops:  4,              // Allow longer paths
    MaxNodes: 200,            // More exploration
    MaxEdges: 1000,           // More connections
    Timeout:  10 * time.Second,
}
```

**For Deep Analysis:**
```go
bounds := storage.GraphBounds{
    MaxHops:  6,              // Deep traversal
    MaxNodes: 500,            // Large exploration
    MaxEdges: 3000,           // Many connections
    Timeout:  30 * time.Second,
}
```

### Monitoring Bounds

Use `BoundsChecker.Stats()` to monitor progress:

```go
checker := engine.NewBoundsChecker(bounds)

// ... perform traversal ...

stats := checker.Stats()
fmt.Printf("Visited %d nodes, traversed %d edges in %v\n",
    stats.NodesVisited,
    stats.EdgesVisited,
    stats.Elapsed,
)
```

## Integration with Phase 2 Storage

Currently, `getNeighbors()` returns an empty slice because Phase 2 graph storage (relationships table) is not yet implemented.

**When Phase 2 is complete, it will:**

1. Query the relationships table for edges
2. Return target memory IDs
3. Optionally filter by relationship type
4. Support bidirectional relationships

**Example future implementation:**

```go
func (g *GraphTraversal) getNeighbors(ctx context.Context, memoryID string) ([]string, error) {
    // Query relationships table
    relationships, err := g.graphProvider.GetRelationships(ctx, memoryID, storage.ListOptions{})
    if err != nil {
        return nil, err
    }

    // Extract target IDs
    neighbors := make([]string, 0, len(relationships.Items))
    for _, rel := range relationships.Items {
        neighbors = append(neighbors, rel.ToMemoryID)
    }

    return neighbors, nil
}
```

## Testing

Comprehensive test suite in `tests/graph_traversal_test.go`:

### BoundsChecker Tests
- `TestBoundsChecker_Stats` - Statistics tracking
- `TestBoundsChecker_NodeLimit` - Node limit enforcement
- `TestBoundsChecker_EdgeLimit` - Edge limit enforcement
- `TestBoundsChecker_DepthLimit` - Depth limit enforcement
- `TestBoundsChecker_Timeout` - Timeout handling
- `TestBoundsChecker_ContextCancellation` - Context handling

### GraphTraversal Tests
- `TestGraphTraversal_BFS_EmptyGraph` - BFS with no edges
- `TestGraphTraversal_BFS_MaxHops` - Depth limiting
- `TestGraphTraversal_BFS_MaxNodes` - Node limiting
- `TestGraphTraversal_BFS_VisitorStop` - Early termination
- `TestGraphTraversal_FindPaths_NoPath` - Disconnected nodes
- `TestGraphTraversal_FindPaths_SameNode` - Source == target
- `TestGraphTraversal_FindRelated` - Related discovery
- `TestGraphTraversal_BFS_Timeout` - Timeout behavior
- `TestGraphTraversal_NormalizesBounds` - Bound normalization

**Run tests:**

```bash
go test -v ./tests -run "TestBoundsChecker|TestGraphTraversal"
```

## Performance Considerations

### Memory Usage

- **Visited Set**: O(V) space, one entry per visited node
- **Queue (BFS)**: O(V) space in worst case
- **Recursion Stack (DFS)**: O(MaxHops) space

### CPU Usage

- **BFS**: Linear in visited nodes and edges
- **DFS**: Exponential in worst case, bounded by MaxHops
- **Bounds Checking**: O(1) per check

### Optimization Tips

1. **Use tight bounds** for exploratory queries
2. **Use BFS** for shortest paths (more efficient than DFS)
3. **Use DFS** only when you need all paths
4. **Monitor Stats()** to tune bounds
5. **Set appropriate timeouts** for user-facing queries

## Future Enhancements

When Phase 2 storage is implemented:

1. **Relationship Filtering**: Filter by relationship type
2. **Weighted Paths**: Use relationship weights in path scoring
3. **Bidirectional BFS**: Faster path finding
4. **Graph Caching**: Cache frequently accessed subgraphs
5. **Parallel Traversal**: Process multiple branches concurrently
6. **Graph Indexes**: Optimize neighbor lookups

## Design Principles

1. **Safety First**: Always enforce bounds to prevent runaway queries
2. **Context Awareness**: Respect cancellation for clean shutdown
3. **Partial Results**: Return what we found even if bounds hit
4. **Clear Errors**: Specific error messages for each bound type
5. **Visitor Pattern**: Flexible traversal with custom logic
6. **Idiomatic Go**: Clean, readable, well-documented code

## References

- Phase 2 Storage: `internal/storage/interfaces.go`
- GraphBounds: `internal/storage/types.go`
- Memory Types: `pkg/types/memory.go`
- Relationship Types: `pkg/types/relationship.go`
