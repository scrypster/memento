package types

import "time"

// Entity represents a named entity extracted from memories.
// Entities can be people, organizations, projects, locations, concepts, etc.
// The system supports 20+ entity types for comprehensive knowledge representation.
type Entity struct {
	// Core identification fields
	ID          string    `json:"id"`                     // Unique identifier (format: ent:type:slug)
	Name        string    `json:"name"`                   // Display name
	Type        string    `json:"type"`                   // Entity type (see EntityType constants)
	Description string    `json:"description,omitempty"`  // Human-readable description
	Aliases     []string  `json:"aliases,omitempty"`      // Alternative names/aliases
	CreatedAt   time.Time `json:"created_at"`             // Creation timestamp
	UpdatedAt   time.Time `json:"updated_at"`             // Last update timestamp

	// Classification and metadata
	Tags     []string               `json:"tags,omitempty"`     // User-defined tags
	Metadata map[string]interface{} `json:"metadata,omitempty"` // Type-specific metadata

	// Embedding for entity similarity
	Embedding          []float32 `json:"embedding,omitempty"`           // Vector embedding
	EmbeddingModel     string    `json:"embedding_model,omitempty"`     // Embedding model used
	EmbeddingDimension int       `json:"embedding_dimension,omitempty"` // Embedding dimension

	// Statistics and provenance
	MemoryCount int      `json:"memory_count,omitempty"` // Number of memories referencing this entity
	FirstSeen   time.Time `json:"first_seen,omitempty"`   // First occurrence in memories
	LastSeen    time.Time `json:"last_seen,omitempty"`    // Most recent occurrence
}
