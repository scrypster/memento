# Memory Engine - Intelligence Layer

The Memory Engine provides intelligent search, inference, decay management, and confidence scoring for the Memento memory system.

## Architecture

The intelligence layer consists of four core components:

### 1. SearchOrchestrator
Coordinates intelligent memory search with multi-factor relevance scoring.

**Features:**
- Full-text search across content, tags, and entities
- Multi-factor relevance scoring
- Pagination support
- Similarity search
- Domain filtering

**Relevance Scoring:**
- Text Match (40%): Exact phrase and partial word matching
- Recency (20%): Temporal relevance based on creation time
- Importance (30%): User-defined or system-calculated importance
- Confidence (10%): Enrichment quality and source reliability

**Usage:**
```go
orchestrator := engine.NewSearchOrchestrator(store)

opts := engine.SearchOptions{
    Query:      "Go programming",
    Limit:      10,
    Offset:     0,
    MinScore:   0.5,
}

results, err := orchestrator.Search(ctx, opts)
for _, result := range results {
    fmt.Printf("Score: %.2f, Reason: %s\n", result.Score, result.Reason)
    fmt.Printf("Components: %+v\n", result.Components)
}
```

### 2. InferenceEngine
Discovers implicit connections and patterns across memories through graph traversal.

**Features:**
- Direct connections via shared entities/tags
- Transitive connections (multi-hop)
- Pattern discovery (clusters, sequences)
- Bounded graph traversal
- Confidence-based filtering

**Connection Types:**
- **Direct**: Memories sharing entities or tags
- **Transitive**: Multi-hop connections through intermediaries
- **Pattern**: Recurring clusters or sequences

**Usage:**
```go
inference := engine.NewInferenceEngine(store)

opts := engine.InferenceOptions{
    MaxDepth:      3,
    MaxResults:    10,
    ConfidenceMin: 0.5,
    IncludeReason: true,
}

connections, err := inference.InferConnections(ctx, memoryID, opts)
for _, conn := range connections {
    fmt.Printf("Type: %s, Confidence: %.2f\n", conn.Type, conn.Confidence)
    fmt.Printf("Path: %v\n", conn.Path)
    if len(conn.Reasoning) > 0 {
        fmt.Printf("Reasoning: %v\n", conn.Reasoning)
    }
}

// Find patterns
patterns, err := inference.FindPatterns(ctx, "work")
for _, pattern := range patterns {
    fmt.Printf("%s: %s (%d memories)\n", pattern.Type, pattern.Description, pattern.Frequency)
}
```

### 3. DecayManager
Manages memory importance evolution using exponential decay with usage-based boosting.

**Features:**
- Exponential decay over time
- Configurable half-life
- Minimum importance floor
- Usage-based boosting (spaced repetition)
- Batch decay application

**Decay Formula:**
```
importance(t) = max(minImportance, baseImportance × e^(-λt))
where λ = ln(2) / halfLife
```

**Default Configuration:**
- Half-life: 30 days
- Minimum importance: 0.1
- Usage boost: 10%

**Usage:**
```go
config := engine.DecayConfig{
    HalfLife:      30 * 24 * time.Hour,
    MinImportance: 0.1,
    UsageBoost:    0.1,
    DecayEnabled:  true,
}

manager := engine.NewDecayManager(store, config)

// Apply decay to all memories
count, err := manager.ApplyDecay(ctx)
fmt.Printf("Decayed %d memories\n", count)

// Boost a memory on access
err = manager.BoostMemory(ctx, memoryID)

// Calculate theoretical decay
memory, _ := store.Get(ctx, memoryID)
decayedImportance := manager.CalculateDecay(memory)
```

### 4. ConfidenceScorer
Calculates multi-factor confidence scores for memories and relationships.

**Confidence Factors:**
- **Entity Score (30%)**: Entity extraction quality
  - Failed: 0.2
  - Completed with entities: 0.7-1.0
- **Relationship Score (20%)**: Relationship extraction quality
  - Failed: 0.2
  - Completed: 0.7-0.9
- **Source Score (30%)**: Source reliability
  - Manual: 1.0
  - Email/Document: 0.8-0.85
  - Auto/Imported: 0.6-0.7
  - AI Summary: 0.5
- **Age Score (20%)**: Temporal relevance
  - < 1 day: 1.0
  - < 1 week: 0.9
  - < 1 month: 0.8
  - < 3 months: 0.7
  - < 6 months: 0.6
  - < 1 year: 0.5
  - > 1 year: 0.4

**Usage:**
```go
scorer := engine.NewConfidenceScorer(store)

// Calculate confidence
confidence, err := scorer.CalculateMemoryConfidence(ctx, memory)
fmt.Printf("Overall: %.2f\n", confidence.Overall)
fmt.Printf("Entity: %.2f, Relationship: %.2f\n",
    confidence.EntityScore, confidence.RelScore)
fmt.Printf("Source: %.2f, Age: %.2f\n",
    confidence.SourceScore, confidence.AgeScore)

// Store confidence in metadata
err = scorer.UpdateConfidence(ctx, memoryID)

// Batch update
count, err := scorer.BatchUpdateConfidence(ctx, memoryIDs)

// Calculate relationship confidence
relConfidence := scorer.CalculateRelationshipConfidence(relationship)
```

## Integration with MemoryEngine

The intelligence layer is integrated into the MemoryEngine:

```go
engine, err := engine.NewMemoryEngine(store, engine.DefaultConfig())
if err != nil {
    return err
}

if err := engine.Start(ctx); err != nil {
    return err
}
defer engine.Shutdown(ctx)

// Search
results, err := engine.Search(ctx, engine.SearchOptions{
    Query: "project deadline",
    Limit: 10,
})

// Infer connections
connections, err := engine.InferConnections(ctx, memoryID, engine.InferenceOptions{
    MaxDepth:      3,
    ConfidenceMin: 0.5,
})

// Apply decay
count, err := engine.ApplyDecay(ctx)

// Boost on access
err = engine.BoostMemory(ctx, memoryID)

// Update confidence
err = engine.UpdateConfidence(ctx, memoryID)
```

## Design Principles

1. **Composability**: Each component is independent and can be used standalone
2. **Transparency**: Score breakdowns explain why results were ranked/matched
3. **Configurability**: Behavior can be customized via configuration structs
4. **Performance**: Bounded operations prevent combinatorial explosion
5. **Idiomatic Go**: Clean APIs, proper error handling, context support

## Performance Considerations

### SearchOrchestrator
- Currently fetches all memories and filters in-memory
- TODO: Implement full-text search at storage layer
- Recommended: Add caching for frequently-used queries

### InferenceEngine
- Bounded graph traversal (max depth, nodes, edges)
- Default limits prevent performance issues
- TODO: Implement graph index for faster traversal

### DecayManager
- Batch processing for efficiency
- Only updates memories with significant change (>1%)
- TODO: Add background scheduler for automatic decay

### ConfidenceScorer
- Lightweight calculations (no LLM calls)
- Batch update support for efficiency
- Confidence stored in metadata for quick access

## Future Enhancements

1. **SearchOrchestrator**
   - Vector similarity search
   - Hybrid search (text + vector)
   - Query expansion
   - Result caching

2. **InferenceEngine**
   - LLM-based reasoning
   - Causal inference
   - Temporal pattern detection
   - Community detection

3. **DecayManager**
   - Adaptive decay rates per domain
   - Importance prediction
   - Automatic scheduling
   - Decay visualization

4. **ConfidenceScorer**
   - Machine learning-based scoring
   - User feedback integration
   - Cross-validation
   - Uncertainty quantification

## Testing

Comprehensive tests are provided in `tests/engine_test.go`:

```bash
# Run all tests
go test -v ./tests/engine_test.go

# Run specific component tests
go test -v ./tests/engine_test.go -run TestSearchOrchestrator
go test -v ./tests/engine_test.go -run TestInferenceEngine
go test -v ./tests/engine_test.go -run TestDecayManager
go test -v ./tests/engine_test.go -run TestConfidenceScorer
```

## Dependencies

- **Storage Layer**: `internal/storage/interfaces.go`
- **Type Definitions**: `pkg/types/`
- **No external dependencies**: Pure Go implementation

## Thread Safety

All components are thread-safe when used through the MemoryEngine, which provides proper synchronization. When using components standalone, ensure proper synchronization in concurrent contexts.
