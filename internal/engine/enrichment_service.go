package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/scrypster/memento/internal/llm"
	"github.com/scrypster/memento/pkg/types"
)

// EnrichmentService handles LLM-powered enrichment of memories.
// It extracts entities, relationships, and generates embeddings.
// Uses ExtractionPipeline for orchestrating entity and relationship extraction.
type EnrichmentService struct {
	llmClient          llm.TextGenerator       // for entity/relationship extraction
	embeddingClient    llm.EmbeddingGenerator  // for vector embeddings (nomic-embed-text)
	db                 *sql.DB
	embeddingProvider  EmbeddingProvider
	ExtractionPipeline *ExtractionPipeline
}

// EmbeddingProvider defines the interface for storing embeddings.
type EmbeddingProvider interface {
	// StoreEmbedding stores a vector embedding for a memory.
	StoreEmbedding(ctx context.Context, memoryID string, embedding []float64, dimension int, model string) error

	// GetEmbedding retrieves the embedding for a memory.
	GetEmbedding(ctx context.Context, memoryID string) ([]float64, error)

	// DeleteEmbedding removes an embedding.
	DeleteEmbedding(ctx context.Context, memoryID string) error

	// GetDimension returns the embedding dimension for a model.
	GetDimension(ctx context.Context, model string) (int, error)
}

// NewEnrichmentService creates a new enrichment service.
func NewEnrichmentService(llmClient llm.TextGenerator, db *sql.DB) *EnrichmentService {
	return &EnrichmentService{
		llmClient:          llmClient,
		db:                 db,
		ExtractionPipeline: NewExtractionPipeline(llmClient, db),
	}
}

// NewEnrichmentServiceWithEmbeddings creates a new enrichment service with embedding support.
// llmClient is used for entity/relationship extraction.
// embeddingClient is used for vector embedding generation (e.g. nomic-embed-text).
// Pass nil for embeddingClient to reuse llmClient for embeddings (not recommended).
func NewEnrichmentServiceWithEmbeddings(llmClient llm.TextGenerator, embeddingClient llm.EmbeddingGenerator, db *sql.DB, embeddingProvider EmbeddingProvider) *EnrichmentService {
	return &EnrichmentService{
		llmClient:          llmClient,
		embeddingClient:    embeddingClient,
		db:                 db,
		embeddingProvider:  embeddingProvider,
		ExtractionPipeline: NewExtractionPipeline(llmClient, db),
	}
}

// Embed generates a vector embedding for the given text.
// Uses the dedicated embeddingClient (nomic-embed-text) if available.
// Returns an error if no embedding client is configured.
func (s *EnrichmentService) Embed(ctx context.Context, text string) ([]float64, error) {
	if s.embeddingClient == nil {
		return nil, fmt.Errorf("no embedding client available for embedding")
	}
	vec32, err := s.embeddingClient.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	vec64 := make([]float64, len(vec32))
	for i, v := range vec32 {
		vec64[i] = float64(v)
	}
	return vec64, nil
}

// EnrichMemory performs full enrichment of a memory using the extraction pipeline:
// 1. Call 1: Extract entities using LLM
// 2. Call 2: Extract relationships using LLM
// 3. Generate embeddings if provider available
//
// Uses ExtractionPipeline for orchestrated entity and relationship extraction.
// Handles partial failures gracefully (entities preserved even if relationships fail).
func (s *EnrichmentService) EnrichMemory(ctx context.Context, memoryID, content string) error {
	log.Printf("Enriching memory %s using extraction pipeline", memoryID)

	// Run the two-call extraction pipeline
	pipelineResult, err := s.ExtractionPipeline.Extract(ctx, memoryID, content)
	if err != nil {
		log.Printf("ERROR: Extraction pipeline failed for memory %s: %v", memoryID, err)
		// If entity extraction failed, return error (relationships weren't even attempted)
		return fmt.Errorf("extraction pipeline failed: %w", err)
	}

	// Log pipeline results
	log.Printf("Pipeline result for memory %s: EntityStatus=%s, RelationshipStatus=%s",
		memoryID, pipelineResult.EntityStatus, pipelineResult.RelationshipStatus)

	// Step 2: Generate embeddings if embedding provider is available
	if s.embeddingProvider != nil {
		if err := s.generateEmbeddings(ctx, memoryID, content); err != nil {
			log.Printf("Warning: Failed to generate embeddings for memory %s: %v", memoryID, err)
			// Don't fail the entire enrichment if embedding generation fails
			// The memory will still be enriched with entities and relationships
		}
	}

	log.Printf("Enrichment complete for memory %s: %d entities, %d relationships",
		memoryID, len(pipelineResult.Entities), len(pipelineResult.Relationships))
	return nil
}

// extractEntities uses LLM to extract entities from content
func (s *EnrichmentService) extractEntities(ctx context.Context, content string) ([]llm.EntityResponse, error) {
	prompt := llm.EntityExtractionPrompt(content)

	response, err := s.llmClient.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	entities, err := llm.ParseEntityResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse entity response: %w", err)
	}

	return entities, nil
}

// extractRelationships uses LLM to extract relationships from content
func (s *EnrichmentService) extractRelationships(ctx context.Context, content string, entities []types.Entity) ([]llm.RelationshipResponse, error) {
	prompt := llm.RelationshipExtractionPrompt(content, entities)

	response, err := s.llmClient.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	relationships, err := llm.ParseRelationshipResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse relationship response: %w", err)
	}

	return relationships, nil
}

// storeEntity stores an entity in the database (upsert)
func (s *EnrichmentService) storeEntity(ctx context.Context, entity llm.EntityResponse) (string, error) {
	// Generate entity ID from name and type
	entityID := fmt.Sprintf("ent:%s:%s", entity.Type, hashForEntity(entity.Name))

	query := `
		INSERT INTO entities (id, name, type, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(name, type) DO UPDATE SET
			description = excluded.description,
			updated_at = excluded.updated_at
		RETURNING id
	`

	now := time.Now()
	var returnedID string
	err := s.db.QueryRowContext(ctx, query,
		entityID, entity.Name, entity.Type, entity.Description, now, now,
	).Scan(&returnedID)

	if err != nil {
		return "", fmt.Errorf("failed to store entity: %w", err)
	}

	return returnedID, nil
}

// storeRelationship stores a relationship in the database
func (s *EnrichmentService) storeRelationship(ctx context.Context, sourceID, targetID, relType string, confidence float64) error {
	// Generate relationship ID
	relID := fmt.Sprintf("rel:%s:%s:%s", sourceID, targetID, relType)

	query := `
		INSERT INTO relationships (id, source_id, target_id, type, weight, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id, target_id, type) DO UPDATE SET
			weight = excluded.weight,
			updated_at = excluded.updated_at
	`

	now := time.Now()
	_, err := s.db.ExecContext(ctx, query,
		relID, sourceID, targetID, relType, confidence, now, now,
	)

	if err != nil {
		return fmt.Errorf("failed to store relationship: %w", err)
	}

	return nil
}

// linkEntityToMemory creates a memory-entity association
func (s *EnrichmentService) linkEntityToMemory(ctx context.Context, memoryID, entityID string, confidence float64) error {
	query := `
		INSERT INTO memory_entities (memory_id, entity_id, frequency, confidence, created_at)
		VALUES (?, ?, 1, ?, ?)
		ON CONFLICT(memory_id, entity_id) DO UPDATE SET
			frequency = frequency + 1,
			confidence = MAX(confidence, excluded.confidence)
	`

	now := time.Now()
	_, err := s.db.ExecContext(ctx, query, memoryID, entityID, confidence, now)

	if err != nil {
		return fmt.Errorf("failed to link entity to memory: %w", err)
	}

	return nil
}

// GenerateEmbeddings generates vector embeddings for a memory using Ollama.
// The embeddings are stored in the embedding provider if successful.
// This method can be called directly for testing or standalone embedding generation.
func (s *EnrichmentService) GenerateEmbeddings(ctx context.Context, memoryID, content string) error {
	return s.generateEmbeddings(ctx, memoryID, content)
}

// generateEmbeddings generates vector embeddings for a memory.
// The embeddings are stored in the embedding provider if successful.
func (s *EnrichmentService) generateEmbeddings(ctx context.Context, memoryID, content string) error {
	if s.embeddingProvider == nil {
		return fmt.Errorf("embedding provider not available")
	}

	if s.embeddingClient == nil {
		return fmt.Errorf("no embedding client available for embedding generation")
	}

	// Call the embedding client to generate embeddings
	embeddingVector, err := s.embeddingClient.Embed(ctx, content)
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	if len(embeddingVector) == 0 {
		return fmt.Errorf("embedding vector is empty")
	}

	// Convert float32 embedding to float64 for storage
	embedding := make([]float64, len(embeddingVector))
	for i, v := range embeddingVector {
		embedding[i] = float64(v)
	}

	dimension := len(embedding)
	model := s.embeddingClient.GetModel()

	// Store embedding in the database
	if err := s.embeddingProvider.StoreEmbedding(ctx, memoryID, embedding, dimension, model); err != nil {
		return fmt.Errorf("failed to store embedding: %w", err)
	}

	log.Printf("Successfully generated and stored embedding for memory %s (dimension: %d, model: %s)",
		memoryID, dimension, model)
	return nil
}
