package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// ConfidenceScorer calculates confidence scores for memories and relationships.
// It uses multi-factor analysis including entity quality, relationship strength,
// source reliability, and temporal factors.
type ConfidenceScorer struct {
	memoryStore storage.MemoryStore
}

// NewConfidenceScorer creates a new confidence scorer.
func NewConfidenceScorer(store storage.MemoryStore) *ConfidenceScorer {
	return &ConfidenceScorer{
		memoryStore: store,
	}
}

// MemoryConfidence represents the overall confidence score and its components.
type MemoryConfidence struct {
	// Overall is the weighted average of all factors (0.0 to 1.0).
	Overall float64

	// EntityScore reflects the quality of entity extraction (0.0 to 1.0).
	EntityScore float64

	// RelScore reflects the quality of relationship extraction (0.0 to 1.0).
	RelScore float64

	// SourceScore reflects the reliability of the memory source (0.0 to 1.0).
	SourceScore float64

	// AgeScore reflects the recency of the memory (0.0 to 1.0).
	AgeScore float64
}

// CalculateMemoryConfidence computes multi-factor confidence for a memory.
// Weights: Entity=0.3, Relationship=0.2, Source=0.3, Age=0.2
func (c *ConfidenceScorer) CalculateMemoryConfidence(ctx context.Context, memory *types.Memory) (*MemoryConfidence, error) {
	confidence := &MemoryConfidence{}

	// Calculate entity score
	confidence.EntityScore = c.calculateEntityScore(memory)

	// Calculate relationship score
	confidence.RelScore = c.calculateRelationshipScore(memory)

	// Calculate source score
	confidence.SourceScore = c.calculateSourceScore(memory)

	// Calculate age score
	confidence.AgeScore = c.calculateAgeScore(memory)

	// Calculate weighted overall score
	confidence.Overall = (confidence.EntityScore * 0.3) +
		(confidence.RelScore * 0.2) +
		(confidence.SourceScore * 0.3) +
		(confidence.AgeScore * 0.2)

	return confidence, nil
}

// calculateEntityScore calculates confidence based on entity extraction quality.
func (c *ConfidenceScorer) calculateEntityScore(memory *types.Memory) float64 {
	score := 0.5 // Base score

	// Penalize if entity extraction failed
	if memory.EntityStatus == types.EnrichmentFailed {
		return 0.2
	}

	// Reward if entity extraction skipped intentionally
	if memory.EntityStatus == types.EnrichmentSkipped {
		return 0.5
	}

	// Reward if entity extraction completed
	if memory.EntityStatus == types.EnrichmentCompleted {
		score = 0.7

		// Bonus for having entities
		if len(memory.Entities) > 0 {
			// More entities = higher confidence (up to 1.0)
			entityBonus := min(0.3, float64(len(memory.Entities))*0.1)
			score += entityBonus
		}
	}

	return min(1.0, score)
}

// calculateRelationshipScore calculates confidence based on relationship extraction.
func (c *ConfidenceScorer) calculateRelationshipScore(memory *types.Memory) float64 {
	score := 0.5 // Base score

	// Penalize if relationship extraction failed
	if memory.RelationshipStatus == types.EnrichmentFailed {
		return 0.2
	}

	// Reward if relationship extraction skipped
	if memory.RelationshipStatus == types.EnrichmentSkipped {
		return 0.5
	}

	// Reward if relationship extraction completed
	if memory.RelationshipStatus == types.EnrichmentCompleted {
		score = 0.7

		// Bonus if we have entities (prerequisite for relationships)
		if len(memory.Entities) >= 2 {
			score += 0.2
		}
	}

	return min(1.0, score)
}

// calculateSourceScore calculates confidence based on source reliability.
// Manual sources are more reliable than automated ones.
func (c *ConfidenceScorer) calculateSourceScore(memory *types.Memory) float64 {
	source := memory.Source

	// Source reliability mapping
	sourceScores := map[string]float64{
		"manual":     1.0,  // User-entered, highly reliable
		"note":       0.95, // User notes, very reliable
		"email":      0.8,  // Email content, reliable
		"document":   0.85, // Document content, reliable
		"message":    0.75, // Messages, moderately reliable
		"auto":       0.6,  // Auto-generated, less reliable
		"imported":   0.7,  // Imported data, moderate reliability
		"ai_summary": 0.5,  // AI summaries, lower reliability
	}

	if score, ok := sourceScores[source]; ok {
		return score
	}

	// Default for unknown sources
	return 0.5
}

// calculateAgeScore calculates confidence based on memory age.
// Newer memories are generally more reliable (information may become stale).
func (c *ConfidenceScorer) calculateAgeScore(memory *types.Memory) float64 {
	age := time.Since(memory.CreatedAt)

	// Age scoring (exponential decay over 1 year)
	// Fresh (< 1 day): 1.0
	// Recent (< 1 week): 0.9
	// Current (< 1 month): 0.8
	// Recent (< 3 months): 0.7
	// Relevant (< 6 months): 0.6
	// Old (< 1 year): 0.5
	// Very old (> 1 year): 0.4

	switch {
	case age < 24*time.Hour:
		return 1.0
	case age < 7*24*time.Hour:
		return 0.9
	case age < 30*24*time.Hour:
		return 0.8
	case age < 90*24*time.Hour:
		return 0.7
	case age < 180*24*time.Hour:
		return 0.6
	case age < 365*24*time.Hour:
		return 0.5
	default:
		return 0.4
	}
}

// CalculateRelationshipConfidence computes confidence for a relationship.
// Based on relationship type, strength, and evidence.
func (c *ConfidenceScorer) CalculateRelationshipConfidence(rel *types.Relationship) float64 {
	score := 0.5 // Base score

	// Factor 1: Relationship strength (if provided)
	if rel.Strength > 0 {
		score = rel.Strength
	}

	// Factor 2: Evidence count
	if len(rel.Evidence) > 0 {
		// More evidence = higher confidence
		evidenceBonus := min(0.3, float64(len(rel.Evidence))*0.1)
		score += evidenceBonus
	}

	// Factor 3: Bidirectional relationships are more confident
	if rel.IsBidirectional() {
		score += 0.1
	}

	// Factor 4: Newer relationships are more confident
	age := time.Since(rel.CreatedAt)
	if age < 30*24*time.Hour {
		score += 0.1
	}

	return min(1.0, score)
}

// UpdateConfidence recalculates and stores confidence for a memory.
// The confidence score is stored in the memory's metadata.
func (c *ConfidenceScorer) UpdateConfidence(ctx context.Context, memoryID string) error {
	// Get memory
	memory, err := c.memoryStore.Get(ctx, memoryID)
	if err != nil {
		return fmt.Errorf("failed to get memory: %w", err)
	}

	// Calculate confidence
	confidence, err := c.CalculateMemoryConfidence(ctx, memory)
	if err != nil {
		return fmt.Errorf("failed to calculate confidence: %w", err)
	}

	// Store in metadata
	if memory.Metadata == nil {
		memory.Metadata = make(map[string]interface{})
	}

	memory.Metadata["confidence"] = confidence.Overall
	memory.Metadata["confidence_components"] = map[string]float64{
		"entity":       confidence.EntityScore,
		"relationship": confidence.RelScore,
		"source":       confidence.SourceScore,
		"age":          confidence.AgeScore,
	}
	memory.UpdatedAt = time.Now()

	// Update memory
	if err := c.memoryStore.Update(ctx, memory); err != nil {
		return fmt.Errorf("failed to update memory: %w", err)
	}

	return nil
}

// BatchUpdateConfidence updates confidence for multiple memories.
// Returns the number of memories successfully updated.
func (c *ConfidenceScorer) BatchUpdateConfidence(ctx context.Context, memoryIDs []string) (int, error) {
	updated := 0

	for _, id := range memoryIDs {
		if err := c.UpdateConfidence(ctx, id); err != nil {
			// Log error but continue with other memories
			continue
		}
		updated++
	}

	return updated, nil
}

// GetConfidence retrieves the stored confidence score for a memory.
// Returns 0.5 if no confidence score is stored.
func (c *ConfidenceScorer) GetConfidence(ctx context.Context, memoryID string) (float64, error) {
	memory, err := c.memoryStore.Get(ctx, memoryID)
	if err != nil {
		return 0, fmt.Errorf("failed to get memory: %w", err)
	}

	if memory.Metadata == nil {
		return 0.5, nil // Default confidence
	}

	confidence, ok := memory.Metadata["confidence"].(float64)
	if !ok {
		return 0.5, nil // Default confidence
	}

	return confidence, nil
}
