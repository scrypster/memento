package types

import "time"

// Memory represents a single memory unit in the system (v2.0 with async enrichment).
// Memories are the atomic units of information storage, containing content,
// metadata, embeddings, and enrichment tracking.
type Memory struct {
	// Core identification fields
	ID        string    `json:"id"`                   // Unique identifier (format: mem:domain:slug)
	Content   string    `json:"content"`              // Raw memory content
	Source    string    `json:"source"`               // Source of the memory (e.g., "manual", "email", "note")
	Domain    string    `json:"domain"`               // Domain/category of the memory
	Timestamp time.Time `json:"timestamp"`            // When the memory event occurred
	CreatedAt time.Time `json:"created_at"`           // When the memory was created in system
	UpdatedAt time.Time `json:"updated_at"`           // Last update timestamp

	// Classification and organization
	MemoryType    string                 `json:"memory_type,omitempty"`    // Memory type (decision, process, concept, event, etc.)
	Category      string                 `json:"category,omitempty"`       // Primary category (Architecture, Security, etc.)
	Subcategory   string                 `json:"subcategory,omitempty"`    // Sub-category within primary
	ContextLabels []string               `json:"context_labels,omitempty"` // Context labels (Technical, Critical, etc.)
	Priority      string                 `json:"priority,omitempty"`       // Priority level (Critical, High, Medium, Low)
	Tags          []string               `json:"tags,omitempty"`           // User-defined tags
	Metadata      map[string]interface{} `json:"metadata,omitempty"`       // Arbitrary metadata

	// v2.0: Overall processing status
	Status MemoryStatus `json:"status"` // Overall memory status (pending, processing, enriched, failed)

	// v2.0: Async enrichment status tracking (per task)
	EntityStatus         EnrichmentStatus `json:"entity_status"`         // Entity extraction status
	RelationshipStatus   EnrichmentStatus `json:"relationship_status"`   // Relationship extraction status
	ClassificationStatus EnrichmentStatus `json:"classification_status"` // Classification extraction status
	SummarizationStatus  EnrichmentStatus `json:"summarization_status"`  // Summarization extraction status
	EmbeddingStatus      EnrichmentStatus `json:"embedding_status"`      // Embedding generation status

	// v2.0: Enrichment retry tracking
	EnrichmentAttempts int     `json:"enrichment_attempts"`              // Number of enrichment attempts
	EnrichmentError    string  `json:"enrichment_error,omitempty"`       // Last error message if enrichment failed
	EnrichedAt         *time.Time `json:"enriched_at,omitempty"`        // When enrichment completed

	// Embedding fields
	Embedding          []float32 `json:"embedding,omitempty"`           // Vector embedding for semantic search
	EmbeddingModel     string    `json:"embedding_model,omitempty"`     // Model used for embedding (e.g., "text-embedding-3-small")
	EmbeddingDimension int       `json:"embedding_dimension,omitempty"` // Dimension of embedding vector

	// LLM enrichment fields (populated after async enrichment)
	Summary    string   `json:"summary,omitempty"`     // LLM-generated summary
	Entities   []string `json:"entities,omitempty"`    // Extracted entity IDs
	Keywords   []string `json:"keywords,omitempty"`    // Extracted keywords
	Sentiment  string   `json:"sentiment,omitempty"`   // Sentiment analysis result
	Importance float64  `json:"importance,omitempty"`  // Importance score (0.0-1.0)

	// Lifecycle management
	State          string     `json:"state,omitempty"`          // Lifecycle state (planning, active, paused, blocked, completed, cancelled, archived)
	StateUpdatedAt *time.Time `json:"state_updated_at,omitempty"` // When the lifecycle state was last changed

	// Provenance tracking
	CreatedBy     string                 `json:"created_by,omitempty"`     // Agent or user that created this memory
	SessionID     string                 `json:"session_id,omitempty"`     // Session in which memory was created
	SourceContext map[string]interface{} `json:"source_context,omitempty"` // Arbitrary context about the source

	// Quality signals
	AccessCount    int        `json:"access_count"`               // Number of times memory has been accessed
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"` // Timestamp of most recent access
	DecayScore     float64    `json:"decay_score"`                // Decay score for memory relevance (0.0-1.0)
	DecayUpdatedAt *time.Time `json:"decay_updated_at,omitempty"` // When decay score was last recalculated

	// Soft delete (grace period for recovery)
	DeletedAt *time.Time `json:"deleted_at,omitempty"` // Timestamp when soft-deleted (null = not deleted)

	// Content deduplication
	ContentHash string `json:"content_hash,omitempty"` // SHA-256 hash of content for deduplication

	// Evolution chain (tracks which memory this supersedes)
	SupersedesID string `json:"supersedes_id,omitempty"` // ID of the memory this one supersedes
}
