package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// ContradictionType represents the type of contradiction detected
type ContradictionType string

const (
	// ContradictionTypeConflictingRelationship indicates conflicting single-valued relationships
	// Example: entity married_to A AND entity married_to B
	ContradictionTypeConflictingRelationship ContradictionType = "conflicting_relationship"

	// ContradictionTypeSupersededActive indicates a superseded memory still has active relationships
	// Example: Memory A supersedes B, but B still has relationships referencing it
	ContradictionTypeSupersededActive ContradictionType = "superseded_active"

	// ContradictionTypeTemporalImpossibility indicates temporal ordering violations
	// Example: Event A is claimed to happen before Event B, but Event A's timestamp is later
	ContradictionTypeTemporalImpossibility ContradictionType = "temporal_impossibility"
)

// Contradiction represents a detected structural contradiction in the memory graph
type Contradiction struct {
	// Type categorizes the contradiction
	Type ContradictionType `json:"type"`

	// MemoryIDs lists memory IDs involved in this contradiction
	MemoryIDs []string `json:"memory_ids"`

	// Description is a human-readable explanation of the contradiction
	Description string `json:"description"`

	// Confidence is the confidence score (0.0-1.0) in the contradiction detection
	Confidence float64 `json:"confidence"`
}

// ContradictionDetector uses deterministic graph algorithms to detect structural contradictions
type ContradictionDetector struct {
	store storage.MemoryStore
	mu    sync.RWMutex

	// For testing: alternative relationship storage
	relationshipStore map[string][]*RelationshipEntry
}

// NewContradictionDetector creates a new contradiction detector
func NewContradictionDetector(store storage.MemoryStore) *ContradictionDetector {
	return &ContradictionDetector{
		store:              store,
		relationshipStore: make(map[string][]*RelationshipEntry),
	}
}

// AddRelationshipForTesting adds a relationship for test scenarios
func (cd *ContradictionDetector) AddRelationshipForTesting(rel *RelationshipEntry) {
	key := rel.FromID + ":" + rel.Type
	cd.relationshipStore[key] = append(cd.relationshipStore[key], rel)
}

// DetectContradictions analyzes the memory graph for structural contradictions.
// If memory_id is provided, only contradictions involving that memory are returned.
// If memory_id is empty, all contradictions in the graph are detected.
//
// Contradiction types detected:
// 1. Conflicting Relationships: Same entity with multiple values for single-valued relationships
// 2. Superseded Memories Still Referenced: Memory marked as superseded has active relationships
// 3. Temporal Impossibilities: Temporal ordering violations in relationship chains
func (cd *ContradictionDetector) DetectContradictions(ctx context.Context, memoryID string) ([]Contradiction, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	var contradictions []Contradiction

	// Fetch all memories from store
	memories, err := cd.getAllMemories(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch memories: %w", err)
	}

	// Build relationship index from memory entities or use test store
	var relationshipIndex map[string][]*RelationshipEntry
	if len(cd.relationshipStore) > 0 {
		relationshipIndex = cd.relationshipStore
	} else {
		relationshipIndex = cd.buildRelationshipIndex(memories)
	}

	// 1. Detect conflicting relationships
	conflicting := cd.detectConflictingRelationships(relationshipIndex, memoryID)
	contradictions = append(contradictions, conflicting...)

	// 2. Detect superseded memories still referenced
	superseded := cd.detectSupersededActive(memories, relationshipIndex, memoryID)
	contradictions = append(contradictions, superseded...)

	// 3. Detect temporal impossibilities
	temporal := cd.detectTemporalImpossibilities(memories, relationshipIndex, memoryID)
	contradictions = append(contradictions, temporal...)

	return contradictions, nil
}

// getAllMemories retrieves all memories from the store with pagination
func (cd *ContradictionDetector) getAllMemories(ctx context.Context) (map[string]*types.Memory, error) {
	memories := make(map[string]*types.Memory)
	pageSize := 1000
	page := 1

	for {
		opts := storage.ListOptions{
			Limit: pageSize,
			Page:  page,
		}

		result, err := cd.store.List(ctx, opts)
		if err != nil {
			return nil, err
		}

		for i := range result.Items {
			memories[result.Items[i].ID] = &result.Items[i]
		}

		if !result.HasMore {
			break
		}
		page++
	}

	return memories, nil
}

// RelationshipEntry represents a relationship extracted from memory entities
type RelationshipEntry struct {
	ID       string // Relationship ID
	FromID   string // Source entity ID
	ToID     string // Target entity ID
	Type     string // Relationship type
	Evidence []string // Memory IDs supporting this relationship
	Metadata map[string]interface{}
}

// buildRelationshipIndex extracts all relationships from memory Entities field
func (cd *ContradictionDetector) buildRelationshipIndex(memories map[string]*types.Memory) map[string][]*RelationshipEntry {
	index := make(map[string][]*RelationshipEntry)

	for memID, memory := range memories {
		// For now, we simulate relationship data from metadata
		// In a real implementation, this would query a RelationshipStore
		if memory.Metadata == nil {
			continue
		}

		// Check for relationships in metadata (simulation)
		if rels, ok := memory.Metadata["relationships"].([]interface{}); ok {
			for _, relData := range rels {
				if relMap, ok := relData.(map[string]interface{}); ok {
					rel := &RelationshipEntry{
						FromID:   toString(relMap["from_id"]),
						ToID:     toString(relMap["to_id"]),
						Type:     toString(relMap["type"]),
						Evidence: []string{memID},
					}
					if rel.FromID != "" && rel.ToID != "" && rel.Type != "" {
						key := rel.FromID + ":" + rel.Type
						index[key] = append(index[key], rel)
					}
				}
			}
		}
	}

	return index
}

// detectConflictingRelationships finds entities with multiple values for single-valued relationship types
func (cd *ContradictionDetector) detectConflictingRelationships(
	relationshipIndex map[string][]*RelationshipEntry,
	memoryID string,
) []Contradiction {
	var contradictions []Contradiction

	// Single-valued relationship types (can only have one target)
	singleValuedRelations := map[string]bool{
		types.RelMarriedTo:   true,
		types.RelParentOf:    true,
		types.RelChildOf:     true,
		types.RelSupersedes:  true,
	}

	// Track which entity:type combinations we've seen
	for _, rels := range relationshipIndex {
		// Parse key to get entity and relation type
		if len(rels) < 2 {
			continue
		}

		relType := rels[0].Type
		if !singleValuedRelations[relType] {
			continue
		}

		// Check if all relations involve the target memory (if specified)
		if memoryID != "" {
			hasMemory := false
			for _, rel := range rels {
				for _, evid := range rel.Evidence {
					if evid == memoryID {
						hasMemory = true
						break
					}
				}
				if hasMemory {
					break
				}
			}
			if !hasMemory {
				continue
			}
		}

		// Found multiple values for a single-valued relationship
		targets := make(map[string]bool)
		allEvidence := make([]string, 0)
		for _, rel := range rels {
			if targets[rel.ToID] {
				continue // Skip duplicates
			}
			targets[rel.ToID] = true
			allEvidence = append(allEvidence, rel.Evidence...)
		}

		if len(targets) > 1 {
			contradiction := Contradiction{
				Type:        ContradictionTypeConflictingRelationship,
				MemoryIDs:   dedupSlice(allEvidence),
				Description: fmt.Sprintf("Entity %s has multiple conflicting %s relationships: %v", rels[0].FromID, relType, getKeys(targets)),
				Confidence:  0.95, // High confidence for exact duplicates
			}
			contradictions = append(contradictions, contradiction)
		}
	}

	return contradictions
}

// detectSupersededActive finds superseded memories that still have active relationships
func (cd *ContradictionDetector) detectSupersededActive(
	memories map[string]*types.Memory,
	relationshipIndex map[string][]*RelationshipEntry,
	memoryID string,
) []Contradiction {
	var contradictions []Contradiction

	// Find all supersedes relationships
	for _, rels := range relationshipIndex {
		for _, rel := range rels {
			if rel.Type != types.RelSupersedes {
				continue
			}

			// Check if target memory is superseded
			if memoryID != "" && rel.ToID != memoryID {
				continue
			}

			// Check if superseded memory still has other relationships
			for _, otherRels := range relationshipIndex {
				for _, otherRel := range otherRels {
					// Look for other relationships involving the superseded memory
					if otherRel.FromID == rel.ToID || otherRel.ToID == rel.ToID {
						// Make sure it's not the supersedes relationship itself
						if otherRel.Type == types.RelSupersedes {
							continue
						}

						superseededMem := memories[rel.ToID]
						if superseededMem == nil {
							continue
						}

						allEvidence := make([]string, 0)
						allEvidence = append(allEvidence, rel.Evidence...)
						allEvidence = append(allEvidence, otherRel.Evidence...)

						contradiction := Contradiction{
							Type:        ContradictionTypeSupersededActive,
							MemoryIDs:   dedupSlice(allEvidence),
							Description: fmt.Sprintf("Superseded memory %s still has active %s relationship to %s", rel.ToID, otherRel.Type, otherRel.ToID),
							Confidence:  0.85,
						}
						contradictions = append(contradictions, contradiction)
						return contradictions // Return early to avoid duplicates
					}
				}
			}
		}
	}

	return contradictions
}

// detectTemporalImpossibilities finds temporal ordering violations
func (cd *ContradictionDetector) detectTemporalImpossibilities(
	memories map[string]*types.Memory,
	relationshipIndex map[string][]*RelationshipEntry,
	memoryID string,
) []Contradiction {
	var contradictions []Contradiction

	// Look for temporal ordering violations
	// Check if relationships with temporal meaning have timestamps that contradict each other
	for _, rels := range relationshipIndex {
		for _, rel := range rels {
			// Check if relationship has temporal metadata
			temporalOrder, ok := rel.Metadata["temporal_order"].(string)
			if !ok {
				continue
			}

			fromMem := memories[rel.FromID]
			toMem := memories[rel.ToID]

			if fromMem == nil || toMem == nil {
				continue
			}

			// Check for contradictions based on temporal order
			switch temporalOrder {
			case "before":
				// FromID should happen before ToID
				// So fromMem.Timestamp should be <= toMem.Timestamp
				if fromMem.Timestamp.After(toMem.Timestamp) {
					if memoryID == "" || contains(rel.Evidence, memoryID) {
						contradiction := Contradiction{
							Type:        ContradictionTypeTemporalImpossibility,
							MemoryIDs:   dedupSlice(rel.Evidence),
							Description: fmt.Sprintf("Memory %s claims to happen before %s, but timestamp is later (%v > %v)", rel.FromID, rel.ToID, fromMem.Timestamp, toMem.Timestamp),
							Confidence:  0.9,
						}
						contradictions = append(contradictions, contradiction)
					}
				}
			case "after":
				// FromID should happen after ToID
				if fromMem.Timestamp.Before(toMem.Timestamp) {
					if memoryID == "" || contains(rel.Evidence, memoryID) {
						contradiction := Contradiction{
							Type:        ContradictionTypeTemporalImpossibility,
							MemoryIDs:   dedupSlice(rel.Evidence),
							Description: fmt.Sprintf("Memory %s claims to happen after %s, but timestamp is earlier (%v < %v)", rel.FromID, rel.ToID, fromMem.Timestamp, toMem.Timestamp),
							Confidence:  0.9,
						}
						contradictions = append(contradictions, contradiction)
					}
				}
			}
		}
	}

	return contradictions
}

// Helper functions

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func dedupSlice(s []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func getKeys(m map[string]bool) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
