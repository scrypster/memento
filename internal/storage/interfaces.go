// Package storage provides composable storage interfaces for the Memento system.
//
// The storage layer is designed with small, focused interfaces that can be
// implemented independently and composed as needed. This follows the Interface
// Segregation Principle and allows for flexible backend implementations.
package storage

import (
	"context"
	"time"

	"github.com/scrypster/memento/pkg/types"
)

// MemoryStore provides CRUD operations and pagination for memories.
// This is the core storage interface for memory lifecycle management.
type MemoryStore interface {
	// Store creates or updates a memory (upsert semantics).
	// If a memory with the same ID exists, it is updated; otherwise, a new one is created.
	Store(ctx context.Context, memory *types.Memory) error

	// Get retrieves a memory by ID.
	// Returns ErrNotFound if the memory doesn't exist.
	Get(ctx context.Context, id string) (*types.Memory, error)

	// List retrieves memories with pagination and filtering.
	List(ctx context.Context, opts ListOptions) (*PaginatedResult[types.Memory], error)

	// Update modifies an existing memory.
	// Returns ErrNotFound if the memory doesn't exist.
	Update(ctx context.Context, memory *types.Memory) error

	// Delete soft-deletes a memory by ID (sets deleted_at timestamp).
	// Returns ErrNotFound if the memory doesn't exist.
	Delete(ctx context.Context, id string) error

	// Purge hard-deletes a memory by ID (permanent removal).
	// Returns ErrNotFound if the memory doesn't exist.
	Purge(ctx context.Context, id string) error

	// Restore un-deletes a soft-deleted memory by clearing its deleted_at timestamp.
	// Returns ErrNotFound if the memory doesn't exist or was not soft-deleted.
	Restore(ctx context.Context, id string) error

	// GetEvolutionChain returns the full version history for a memory,
	// ordered oldest → newest (original at index 0, latest at last).
	// It walks backward via supersedes_id links and forward via reverse lookups.
	// Capped at 50 versions to prevent infinite loops.
	GetEvolutionChain(ctx context.Context, memoryID string) ([]*types.Memory, error)

	// GetMemoriesByRelationType returns memories connected to memoryID via
	// relationships of the given type (e.g. "CONTAINS").
	// Used by get_project_tree to walk project hierarchies.
	GetMemoriesByRelationType(ctx context.Context, memoryID string, relType string) ([]*types.Memory, error)

	// UpdateStatus updates the processing status of a memory.
	UpdateStatus(ctx context.Context, id string, status types.MemoryStatus) error

	// UpdateEnrichment updates enrichment metadata for a memory.
	UpdateEnrichment(ctx context.Context, id string, enrichment EnrichmentUpdate) error

	// IncrementAccessCount atomically increments the access_count and updates
	// last_accessed_at for the given memory ID. It returns ErrNotFound if the
	// memory does not exist.
	IncrementAccessCount(ctx context.Context, id string) error

	// UpdateState updates the lifecycle state of a memory (Opus Issue #6).
	// Validates state transitions according to the state machine in design doc Appendix B.
	// Returns ErrNotFound if the memory doesn't exist.
	UpdateState(ctx context.Context, id string, state string) error

	// GetRelatedMemories returns the IDs of memories that share entities with
	// the given memory. This supports 1-hop graph traversal for the
	// GraphTraversal engine. Implementations may return an empty slice when
	// relationship data is not yet populated.
	GetRelatedMemories(ctx context.Context, memoryID string) ([]string, error)

	// Traverse finds memories connected through the entity relationship graph.
	// Starting from startMemoryID, it follows entity→relationship→entity→memory
	// links up to maxHops times and returns up to limit results, sorted by
	// hop distance ascending then by memory importance descending.
	// Returns an empty slice (not an error) when no connected memories exist.
	Traverse(ctx context.Context, startMemoryID string, maxHops int, limit int) ([]TraversalResult, error)

	// GetMemoryEntities returns the entities associated with a specific memory.
	// Returns an empty slice (not an error) when the memory has no entities.
	GetMemoryEntities(ctx context.Context, memoryID string) ([]*types.Entity, error)

	// UpdateDecayScores applies time-based decay to all active memories.
	// This should be called periodically (e.g., daily). Returns count of updated rows.
	UpdateDecayScores(ctx context.Context) (int, error)

	// Close releases any resources held by the store.
	Close() error
}

// SearchProvider provides full-text and vector search capabilities.
// This interface will be implemented in a later phase.
type SearchProvider interface {
	// FullTextSearch performs full-text search across memory content.
	// TODO: Implement in Phase 3
	FullTextSearch(ctx context.Context, opts SearchOptions) (*PaginatedResult[types.Memory], error)

	// VectorSearch performs semantic search using embeddings.
	// TODO: Implement in Phase 3
	VectorSearch(ctx context.Context, query []float64, opts SearchOptions) (*PaginatedResult[types.Memory], error)

	// HybridSearch combines full-text and vector search.
	// TODO: Implement in Phase 3
	HybridSearch(ctx context.Context, text string, vector []float64, opts SearchOptions) (*PaginatedResult[types.Memory], error)
}

// GraphProvider provides bounded graph traversal operations.
// This interface will be implemented in a later phase.
type GraphProvider interface {
	// Traverse performs bounded graph traversal from a starting memory.
	// TODO: Implement in Phase 4
	Traverse(ctx context.Context, startID string, bounds GraphBounds) (*GraphResult, error)

	// FindPath finds the shortest path between two memories.
	// TODO: Implement in Phase 4
	FindPath(ctx context.Context, startID, endID string, bounds GraphBounds) ([]string, error)

	// GetNeighbors retrieves immediate neighbors of a memory.
	// TODO: Implement in Phase 4
	GetNeighbors(ctx context.Context, memoryID string, opts ListOptions) (*PaginatedResult[types.Memory], error)
}

// RelationshipStore manages relationships between memories and entities.
// This interface will be implemented in a later phase.
type RelationshipStore interface {
	// CreateRelationship creates a new relationship.
	// TODO: Implement in Phase 4
	CreateRelationship(ctx context.Context, rel *types.Relationship) error

	// GetRelationships retrieves relationships for a memory.
	// TODO: Implement in Phase 4
	GetRelationships(ctx context.Context, memoryID string, opts ListOptions) (*PaginatedResult[types.Relationship], error)

	// DeleteRelationship removes a relationship.
	// TODO: Implement in Phase 4
	DeleteRelationship(ctx context.Context, id string) error

	// StoreEntity creates or updates an entity.
	// TODO: Implement in Phase 4
	StoreEntity(ctx context.Context, entity *types.Entity) error

	// GetEntity retrieves an entity by ID.
	// TODO: Implement in Phase 4
	GetEntity(ctx context.Context, id string) (*types.Entity, error)
}

// EmbeddingProvider manages vector embeddings with dimension tracking.
// This interface will be implemented in a later phase.
type EmbeddingProvider interface {
	// StoreEmbedding stores a vector embedding for a memory.
	// TODO: Implement in Phase 3
	StoreEmbedding(ctx context.Context, memoryID string, embedding []float64, dimension int, model string) error

	// GetEmbedding retrieves the embedding for a memory.
	// TODO: Implement in Phase 3
	GetEmbedding(ctx context.Context, memoryID string) ([]float64, error)

	// DeleteEmbedding removes an embedding.
	// TODO: Implement in Phase 3
	DeleteEmbedding(ctx context.Context, memoryID string) error

	// GetDimension returns the embedding dimension for a model.
	// TODO: Implement in Phase 3
	GetDimension(ctx context.Context, model string) (int, error)
}

// EnrichmentUpdate contains metadata for enrichment status updates.
type EnrichmentUpdate struct {
	// EntityStatus is the status of entity extraction.
	EntityStatus types.EnrichmentStatus

	// RelationshipStatus is the status of relationship extraction.
	RelationshipStatus types.EnrichmentStatus

	// EmbeddingStatus is the status of embedding generation.
	EmbeddingStatus types.EnrichmentStatus

	// EnrichmentAttempts tracks the number of enrichment retry attempts.
	EnrichmentAttempts int

	// EnrichmentError stores the last enrichment error message.
	EnrichmentError string

	// EnrichedAt is the timestamp when enrichment completed.
	EnrichedAt *time.Time
}
