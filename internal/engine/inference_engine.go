package engine

import (
	"context"
	"fmt"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// InferenceEngine performs reasoning and pattern discovery across memories.
// It discovers implicit connections through entity relationships and
// identifies patterns in memory networks.
type InferenceEngine struct {
	memoryStore storage.MemoryStore
}

// NewInferenceEngine creates a new inference engine.
func NewInferenceEngine(store storage.MemoryStore) *InferenceEngine {
	return &InferenceEngine{
		memoryStore: store,
	}
}

// InferenceOptions configures inference behavior.
type InferenceOptions struct {
	// MaxDepth is the maximum traversal depth for graph searches.
	MaxDepth int

	// MaxResults is the maximum number of results to return.
	MaxResults int

	// ConfidenceMin is the minimum confidence threshold (0.0 to 1.0).
	ConfidenceMin float64

	// IncludeReason includes step-by-step reasoning in results.
	IncludeReason bool
}

// InferenceResult represents an inferred connection between memories.
type InferenceResult struct {
	// Path is the sequence of memory IDs forming the connection.
	Path []string

	// Confidence is the confidence in this connection (0.0 to 1.0).
	Confidence float64

	// Reasoning provides step-by-step explanation of the connection.
	Reasoning []string

	// Type describes the connection type.
	Type string // "direct", "transitive", "pattern"
}

// Pattern represents a discovered pattern across memories.
type Pattern struct {
	// Type describes the pattern type.
	Type string // "sequence", "cluster", "causation"

	// Memories are the memory IDs involved in this pattern.
	Memories []string

	// Frequency is how often this pattern appears.
	Frequency int

	// Confidence is the confidence in this pattern (0.0 to 1.0).
	Confidence float64

	// Description is a human-readable description of the pattern.
	Description string
}

// InferConnections discovers implicit connections between memories.
// It performs bounded graph traversal to find related memories through
// shared entities, tags, and content similarity.
func (i *InferenceEngine) InferConnections(ctx context.Context, memoryID string, opts InferenceOptions) ([]InferenceResult, error) {
	// Normalize options
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 3
	}
	if opts.MaxDepth > 10 {
		opts.MaxDepth = 10
	}
	if opts.MaxResults <= 0 {
		opts.MaxResults = 10
	}
	if opts.ConfidenceMin < 0 {
		opts.ConfidenceMin = 0.0
	}

	// Get source memory
	sourceMemory, err := i.memoryStore.Get(ctx, memoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to get source memory: %w", err)
	}

	// Find connections
	var results []InferenceResult

	// Direct connections via shared entities
	directConnections, err := i.findDirectConnections(ctx, sourceMemory, opts)
	if err != nil {
		return nil, err
	}
	results = append(results, directConnections...)

	// Transitive connections (if depth > 1)
	if opts.MaxDepth > 1 {
		transitiveConnections, err := i.findTransitiveConnections(ctx, sourceMemory, directConnections, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, transitiveConnections...)
	}

	// Filter by confidence and limit
	var filtered []InferenceResult
	for _, result := range results {
		if result.Confidence >= opts.ConfidenceMin {
			filtered = append(filtered, result)
		}
	}

	// Sort by confidence (descending)
	for i := 0; i < len(filtered); i++ {
		for j := i + 1; j < len(filtered); j++ {
			if filtered[j].Confidence > filtered[i].Confidence {
				filtered[i], filtered[j] = filtered[j], filtered[i]
			}
		}
	}

	// Limit results
	if len(filtered) > opts.MaxResults {
		filtered = filtered[:opts.MaxResults]
	}

	return filtered, nil
}

// findDirectConnections finds memories directly connected through shared entities.
func (i *InferenceEngine) findDirectConnections(ctx context.Context, source *types.Memory, opts InferenceOptions) ([]InferenceResult, error) {
	var results []InferenceResult

	// If no entities, no direct connections
	if len(source.Entities) == 0 {
		return results, nil
	}

	// List all memories to find overlaps
	// TODO: Optimize with entity-based index
	listOpts := storage.ListOptions{
		Page:      1,
		Limit:     100,
		SortBy:    "created_at",
		SortOrder: "desc",
	}

	memResult, err := i.memoryStore.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	// Find memories with shared entities
	for idx := range memResult.Items {
		memory := &memResult.Items[idx]

		// Skip self
		if memory.ID == source.ID {
			continue
		}

		// Calculate entity overlap
		sharedEntities := i.findSharedEntities(source.Entities, memory.Entities)
		if len(sharedEntities) == 0 {
			continue
		}

		// Calculate confidence based on overlap
		confidence := i.calculateEntityOverlapConfidence(source.Entities, memory.Entities, sharedEntities)

		// Build reasoning
		var reasoning []string
		if opts.IncludeReason {
			reasoning = append(reasoning, fmt.Sprintf("Shares %d entities: %v", len(sharedEntities), sharedEntities))
			reasoning = append(reasoning, fmt.Sprintf("Confidence: %.2f", confidence))
		}

		result := InferenceResult{
			Path:       []string{source.ID, memory.ID},
			Confidence: confidence,
			Reasoning:  reasoning,
			Type:       "direct",
		}

		results = append(results, result)
	}

	return results, nil
}

// findTransitiveConnections finds connections through intermediate memories.
func (i *InferenceEngine) findTransitiveConnections(ctx context.Context, source *types.Memory, directConnections []InferenceResult, opts InferenceOptions) ([]InferenceResult, error) {
	var results []InferenceResult

	// For each direct connection, explore one more hop
	visited := make(map[string]bool)
	visited[source.ID] = true

	for _, direct := range directConnections {
		if len(direct.Path) < 2 {
			continue
		}

		// Get the intermediate memory
		intermediateID := direct.Path[1]
		if visited[intermediateID] {
			continue
		}
		visited[intermediateID] = true

		intermediate, err := i.memoryStore.Get(ctx, intermediateID)
		if err != nil {
			continue
		}

		// Find connections from intermediate
		nextHop, err := i.findDirectConnections(ctx, intermediate, opts)
		if err != nil {
			continue
		}

		// Build transitive paths
		for _, hop := range nextHop {
			if len(hop.Path) < 2 {
				continue
			}

			targetID := hop.Path[1]

			// Skip if already visited or is source
			if visited[targetID] || targetID == source.ID {
				continue
			}

			// Build transitive path
			path := []string{source.ID, intermediateID, targetID}

			// Calculate transitive confidence (decay with distance)
			confidence := direct.Confidence * hop.Confidence * 0.7 // 30% decay for transitivity

			// Build reasoning
			var reasoning []string
			if opts.IncludeReason {
				reasoning = append(reasoning, fmt.Sprintf("Path: %s -> %s -> %s", source.ID, intermediateID, targetID))
				reasoning = append(reasoning, fmt.Sprintf("Transitive confidence: %.2f", confidence))
			}

			result := InferenceResult{
				Path:       path,
				Confidence: confidence,
				Reasoning:  reasoning,
				Type:       "transitive",
			}

			results = append(results, result)
		}
	}

	return results, nil
}

// FindPatterns identifies recurring patterns across memories.
// It looks for clusters of related memories, sequences, and causal relationships.
func (i *InferenceEngine) FindPatterns(ctx context.Context, domain string) ([]Pattern, error) {
	// Build filter
	filter := make(map[string]interface{})
	if domain != "" {
		filter["domain"] = domain
	}

	// List memories in domain
	listOpts := storage.ListOptions{
		Page:      1,
		Limit:     100,
		SortBy:    "created_at",
		SortOrder: "desc",
		Filter:    filter,
	}

	result, err := i.memoryStore.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	var patterns []Pattern

	// Find entity clusters (memories sharing multiple entities)
	entityClusters := i.findEntityClusters(result.Items)
	patterns = append(patterns, entityClusters...)

	// Find tag-based patterns
	tagPatterns := i.findTagPatterns(result.Items)
	patterns = append(patterns, tagPatterns...)

	return patterns, nil
}

// findEntityClusters finds clusters of memories sharing entities.
func (i *InferenceEngine) findEntityClusters(memories []types.Memory) []Pattern {
	var patterns []Pattern

	// Group memories by entity
	entityGroups := make(map[string][]string) // entity -> memory IDs

	for idx := range memories {
		memory := &memories[idx]
		for _, entity := range memory.Entities {
			entityGroups[entity] = append(entityGroups[entity], memory.ID)
		}
	}

	// Find groups with multiple memories (clusters)
	for entity, memoryIDs := range entityGroups {
		if len(memoryIDs) < 3 {
			continue // Need at least 3 memories to form a pattern
		}

		pattern := Pattern{
			Type:        "cluster",
			Memories:    memoryIDs,
			Frequency:   len(memoryIDs),
			Confidence:  min(1.0, float64(len(memoryIDs))/10.0), // Confidence grows with cluster size
			Description: fmt.Sprintf("Cluster of %d memories related to entity %s", len(memoryIDs), entity),
		}

		patterns = append(patterns, pattern)
	}

	return patterns
}

// findTagPatterns finds patterns in tag usage.
func (i *InferenceEngine) findTagPatterns(memories []types.Memory) []Pattern {
	var patterns []Pattern

	// Group memories by tag combinations
	tagGroups := make(map[string][]string) // tag -> memory IDs

	for idx := range memories {
		memory := &memories[idx]
		for _, tag := range memory.Tags {
			tagGroups[tag] = append(tagGroups[tag], memory.ID)
		}
	}

	// Find frequently used tags
	for tag, memoryIDs := range tagGroups {
		if len(memoryIDs) < 3 {
			continue
		}

		pattern := Pattern{
			Type:        "cluster",
			Memories:    memoryIDs,
			Frequency:   len(memoryIDs),
			Confidence:  min(1.0, float64(len(memoryIDs))/10.0),
			Description: fmt.Sprintf("Cluster of %d memories tagged with '%s'", len(memoryIDs), tag),
		}

		patterns = append(patterns, pattern)
	}

	return patterns
}

// findSharedEntities finds entities present in both lists.
func (i *InferenceEngine) findSharedEntities(entities1, entities2 []string) []string {
	entityMap := make(map[string]bool)
	for _, e := range entities1 {
		entityMap[e] = true
	}

	var shared []string
	for _, e := range entities2 {
		if entityMap[e] {
			shared = append(shared, e)
		}
	}

	return shared
}

// calculateEntityOverlapConfidence calculates confidence based on entity overlap.
func (i *InferenceEngine) calculateEntityOverlapConfidence(entities1, entities2, shared []string) float64 {
	if len(entities1) == 0 && len(entities2) == 0 {
		return 0.0
	}

	// Use Jaccard similarity: intersection / union
	union := len(entities1) + len(entities2) - len(shared)
	if union == 0 {
		return 0.0
	}

	return float64(len(shared)) / float64(union)
}
