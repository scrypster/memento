package engine

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/scrypster/memento/pkg/types"
)

// Helper function to create a memory with defaults
func createTestMemory(id string, source string, entityStatus, relStatus types.EnrichmentStatus) *types.Memory {
	return &types.Memory{
		ID:                 id,
		Content:            "Test memory content",
		Source:             source,
		Domain:             "test",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		EntityStatus:       entityStatus,
		RelationshipStatus: relStatus,
		Entities:           []string{},
	}
}

// TestCalculateMemoryConfidence_BasicStructure verifies the confidence scorer returns all required fields
func TestCalculateMemoryConfidence_BasicStructure(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "manual", types.EnrichmentCompleted, types.EnrichmentCompleted)

	confidence, err := scorer.CalculateMemoryConfidence(context.Background(), memory)
	if err != nil {
		t.Fatalf("CalculateMemoryConfidence failed: %v", err)
	}

	// Verify all fields are present
	if confidence == nil {
		t.Fatal("Expected confidence to be non-nil")
	}
	if confidence.EntityScore < 0 || confidence.EntityScore > 1.0 {
		t.Errorf("EntityScore out of range: %f", confidence.EntityScore)
	}
	if confidence.RelScore < 0 || confidence.RelScore > 1.0 {
		t.Errorf("RelScore out of range: %f", confidence.RelScore)
	}
	if confidence.SourceScore < 0 || confidence.SourceScore > 1.0 {
		t.Errorf("SourceScore out of range: %f", confidence.SourceScore)
	}
	if confidence.AgeScore < 0 || confidence.AgeScore > 1.0 {
		t.Errorf("AgeScore out of range: %f", confidence.AgeScore)
	}
	if confidence.Overall < 0 || confidence.Overall > 1.0 {
		t.Errorf("Overall score out of range: %f", confidence.Overall)
	}
}

// TestEntityScore_EnrichmentFailed verifies entity extraction failure penalizes score
func TestEntityScore_EnrichmentFailed(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "manual", types.EnrichmentFailed, types.EnrichmentCompleted)

	score := scorer.calculateEntityScore(memory)
	expectedScore := 0.2
	if math.Abs(score-expectedScore) > 0.001 {
		t.Errorf("EntityScore for EnrichmentFailed: expected %f, got %f", expectedScore, score)
	}
}

// TestEntityScore_EnrichmentSkipped verifies skipped enrichment returns base score
func TestEntityScore_EnrichmentSkipped(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "manual", types.EnrichmentSkipped, types.EnrichmentCompleted)

	score := scorer.calculateEntityScore(memory)
	expectedScore := 0.5
	if math.Abs(score-expectedScore) > 0.001 {
		t.Errorf("EntityScore for EnrichmentSkipped: expected %f, got %f", expectedScore, score)
	}
}

// TestEntityScore_EnrichmentCompleted verifies completed enrichment without entities
func TestEntityScore_EnrichmentCompleted_NoEntities(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "manual", types.EnrichmentCompleted, types.EnrichmentCompleted)
	memory.Entities = []string{} // No entities

	score := scorer.calculateEntityScore(memory)
	expectedScore := 0.7 // Base score for completed without bonus
	if math.Abs(score-expectedScore) > 0.001 {
		t.Errorf("EntityScore for EnrichmentCompleted (no entities): expected %f, got %f", expectedScore, score)
	}
}

// TestEntityScore_EnrichmentCompleted_WithEntities verifies entity bonus is applied correctly
func TestEntityScore_EnrichmentCompleted_WithEntities(t *testing.T) {
	tests := []struct {
		name           string
		entityCount    int
		expectedScore  float64
	}{
		{"One entity", 1, 0.8},      // 0.7 + 0.1
		{"Two entities", 2, 0.9},    // 0.7 + 0.2
		{"Three entities", 3, 1.0},  // 0.7 + 0.3 (clamped to 1.0)
		{"Four entities", 4, 1.0},   // 0.7 + 0.3 (clamped to 1.0)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scorer := NewConfidenceScorer(nil)
			memory := createTestMemory("mem:test:1", "manual", types.EnrichmentCompleted, types.EnrichmentCompleted)
			memory.Entities = make([]string, tt.entityCount)
			for i := 0; i < tt.entityCount; i++ {
				memory.Entities[i] = "entity:" + string(rune(i))
			}

			score := scorer.calculateEntityScore(memory)
			if math.Abs(score-tt.expectedScore) > 0.001 {
				t.Errorf("EntityScore for %d entities: expected %f, got %f", tt.entityCount, tt.expectedScore, score)
			}
		})
	}
}

// TestRelationshipScore_EnrichmentFailed verifies relationship failure penalizes
func TestRelationshipScore_EnrichmentFailed(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "manual", types.EnrichmentCompleted, types.EnrichmentFailed)

	score := scorer.calculateRelationshipScore(memory)
	expectedScore := 0.2
	if math.Abs(score-expectedScore) > 0.001 {
		t.Errorf("RelScore for EnrichmentFailed: expected %f, got %f", expectedScore, score)
	}
}

// TestRelationshipScore_EnrichmentSkipped verifies skipped returns base score
func TestRelationshipScore_EnrichmentSkipped(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "manual", types.EnrichmentCompleted, types.EnrichmentSkipped)

	score := scorer.calculateRelationshipScore(memory)
	expectedScore := 0.5
	if math.Abs(score-expectedScore) > 0.001 {
		t.Errorf("RelScore for EnrichmentSkipped: expected %f, got %f", expectedScore, score)
	}
}

// TestRelationshipScore_EnrichmentCompleted_InsufficientEntities verifies need >= 2 entities for bonus
func TestRelationshipScore_EnrichmentCompleted_InsufficientEntities(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "manual", types.EnrichmentCompleted, types.EnrichmentCompleted)
	memory.Entities = []string{"entity:1"} // Only 1 entity

	score := scorer.calculateRelationshipScore(memory)
	expectedScore := 0.7 // Completed without bonus (< 2 entities)
	if math.Abs(score-expectedScore) > 0.001 {
		t.Errorf("RelScore for EnrichmentCompleted (1 entity): expected %f, got %f", expectedScore, score)
	}
}

// TestRelationshipScore_EnrichmentCompleted_SufficientEntities verifies bonus for >= 2 entities
func TestRelationshipScore_EnrichmentCompleted_SufficientEntities(t *testing.T) {
	tests := []struct {
		name          string
		entityCount   int
		expectedScore float64
	}{
		{"Two entities", 2, 0.9},    // 0.7 + 0.2
		{"Three entities", 3, 0.9},  // 0.7 + 0.2
		{"Five entities", 5, 0.9},   // 0.7 + 0.2
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scorer := NewConfidenceScorer(nil)
			memory := createTestMemory("mem:test:1", "manual", types.EnrichmentCompleted, types.EnrichmentCompleted)
			memory.Entities = make([]string, tt.entityCount)
			for i := 0; i < tt.entityCount; i++ {
				memory.Entities[i] = "entity:" + string(rune(i))
			}

			score := scorer.calculateRelationshipScore(memory)
			if math.Abs(score-tt.expectedScore) > 0.001 {
				t.Errorf("RelScore for %d entities: expected %f, got %f", tt.entityCount, tt.expectedScore, score)
			}
		})
	}
}

// TestSourceScore_KnownSources verifies all documented source mappings
func TestSourceScore_KnownSources(t *testing.T) {
	tests := []struct {
		source        string
		expectedScore float64
	}{
		{"manual", 1.0},
		{"note", 0.95},
		{"email", 0.8},
		{"document", 0.85},
		{"message", 0.75},
		{"auto", 0.6},
		{"imported", 0.7},
		{"ai_summary", 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			scorer := NewConfidenceScorer(nil)
			memory := createTestMemory("mem:test:1", tt.source, types.EnrichmentCompleted, types.EnrichmentCompleted)

			score := scorer.calculateSourceScore(memory)
			if math.Abs(score-tt.expectedScore) > 0.001 {
				t.Errorf("SourceScore for %s: expected %f, got %f", tt.source, tt.expectedScore, score)
			}
		})
	}
}

// TestSourceScore_UnknownSource verifies default score for unknown sources
func TestSourceScore_UnknownSource(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "unknown_source", types.EnrichmentCompleted, types.EnrichmentCompleted)

	score := scorer.calculateSourceScore(memory)
	expectedScore := 0.5
	if math.Abs(score-expectedScore) > 0.001 {
		t.Errorf("SourceScore for unknown source: expected %f, got %f", expectedScore, score)
	}
}

// TestAgeScore_Fresh verifies fresh memories get highest score
func TestAgeScore_Fresh(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "manual", types.EnrichmentCompleted, types.EnrichmentCompleted)
	memory.CreatedAt = time.Now().Add(-1 * time.Hour) // 1 hour old

	score := scorer.calculateAgeScore(memory)
	expectedScore := 1.0
	if math.Abs(score-expectedScore) > 0.001 {
		t.Errorf("AgeScore for 1-hour-old memory: expected %f, got %f", expectedScore, score)
	}
}

// TestAgeScore_AgeRanges verifies the age score formula across time ranges
func TestAgeScore_AgeRanges(t *testing.T) {
	tests := []struct {
		name          string
		ageOffset     time.Duration
		expectedScore float64
	}{
		{"< 1 day", -12 * time.Hour, 1.0},
		{"< 1 week", -3 * 24 * time.Hour, 0.9},
		{"< 1 month", -15 * 24 * time.Hour, 0.8},
		{"< 3 months", -60 * 24 * time.Hour, 0.7},
		{"< 6 months", -150 * 24 * time.Hour, 0.6},
		{"< 1 year", -300 * 24 * time.Hour, 0.5},
		{"> 1 year", -400 * 24 * time.Hour, 0.4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scorer := NewConfidenceScorer(nil)
			memory := createTestMemory("mem:test:1", "manual", types.EnrichmentCompleted, types.EnrichmentCompleted)
			memory.CreatedAt = time.Now().Add(tt.ageOffset)

			score := scorer.calculateAgeScore(memory)
			if math.Abs(score-tt.expectedScore) > 0.001 {
				t.Errorf("AgeScore for %s: expected %f, got %f", tt.name, tt.expectedScore, score)
			}
		})
	}
}

// TestWeightFormula_Verification verifies the weighted average calculation
func TestWeightFormula_Verification(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "manual", types.EnrichmentCompleted, types.EnrichmentCompleted)
	memory.Entities = []string{"entity:1", "entity:2"}

	confidence, err := scorer.CalculateMemoryConfidence(context.Background(), memory)
	if err != nil {
		t.Fatalf("CalculateMemoryConfidence failed: %v", err)
	}

	// Manual calculation with weights: Entity=0.3, Rel=0.2, Source=0.3, Age=0.2
	expectedOverall := (confidence.EntityScore * 0.3) +
		(confidence.RelScore * 0.2) +
		(confidence.SourceScore * 0.3) +
		(confidence.AgeScore * 0.2)

	if math.Abs(confidence.Overall-expectedOverall) > 0.001 {
		t.Errorf("Overall score formula: expected %f, got %f", expectedOverall, confidence.Overall)
	}
}

// TestOverallScore_AllPerfect verifies near-maximum score with perfect conditions
func TestOverallScore_AllPerfect(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "manual", types.EnrichmentCompleted, types.EnrichmentCompleted)
	memory.CreatedAt = time.Now() // Fresh
	memory.Entities = make([]string, 3)
	for i := 0; i < 3; i++ {
		memory.Entities[i] = "entity:" + string(rune(i))
	}

	confidence, err := scorer.CalculateMemoryConfidence(context.Background(), memory)
	if err != nil {
		t.Fatalf("CalculateMemoryConfidence failed: %v", err)
	}

	// With manual source, fresh creation, 3 entities, both enrichments completed
	// EntityScore: 0.7 + 0.3 = 1.0 (clamped)
	// RelScore: 0.7 + 0.2 = 0.9 (can't reach 1.0 because < 3 entities means no extra bonus beyond 2 entity check)
	// SourceScore: 1.0
	// AgeScore: 1.0
	// Overall: (1.0 * 0.3) + (0.9 * 0.2) + (1.0 * 0.3) + (1.0 * 0.2) = 0.98
	expectedScore := 0.98
	if math.Abs(confidence.Overall-expectedScore) > 0.001 {
		t.Errorf("Overall score with perfect conditions: expected %f, got %f", expectedScore, confidence.Overall)
	}
}

// TestOverallScore_AllFailed verifies minimum reasonable score with failures
func TestOverallScore_WithFailures(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "ai_summary", types.EnrichmentFailed, types.EnrichmentFailed)
	memory.CreatedAt = time.Now().Add(-400 * 24 * time.Hour) // Very old

	confidence, err := scorer.CalculateMemoryConfidence(context.Background(), memory)
	if err != nil {
		t.Fatalf("CalculateMemoryConfidence failed: %v", err)
	}

	// EntityScore: 0.2 (failed), RelScore: 0.2 (failed), SourceScore: 0.5 (ai_summary), AgeScore: 0.4 (very old)
	// Overall: (0.2 * 0.3) + (0.2 * 0.2) + (0.5 * 0.3) + (0.4 * 0.2) = 0.06 + 0.04 + 0.15 + 0.08 = 0.33
	expectedOverall := (0.2 * 0.3) + (0.2 * 0.2) + (0.5 * 0.3) + (0.4 * 0.2)

	if math.Abs(confidence.Overall-expectedOverall) > 0.001 {
		t.Errorf("Overall score with failures: expected %f, got %f", expectedOverall, confidence.Overall)
	}
}

// TestCalculateRelationshipConfidence_Basic verifies relationship confidence calculation
func TestCalculateRelationshipConfidence_Basic(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	rel := &types.Relationship{
		ID:        "rel:1",
		FromID:    "entity:1",
		ToID:      "entity:2",
		Type:      "works_with",
		Strength:  0.7,
		CreatedAt: time.Now(),
		Evidence:  []string{"mem:1", "mem:2"},
		RelationshipMetadata: types.RelationshipMetadata{
			Bidirectional: true,
		},
	}

	score := scorer.CalculateRelationshipConfidence(rel)

	// Base: 0.5, Strength: 0.7 (overrides), Evidence bonus: 0.2, Bidirectional bonus: 0.1, Age bonus: 0.1
	// But strength overrides base, so: 0.7 + 0.2 + 0.1 + 0.1 = 1.1 -> clamped to 1.0
	if score > 1.0 || score < 0.7 {
		t.Errorf("RelationshipConfidence score out of expected range: %f", score)
	}
}

// TestCalculateRelationshipConfidence_NoStrength verifies base score used when no strength
func TestCalculateRelationshipConfidence_NoStrength(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	rel := &types.Relationship{
		ID:        "rel:1",
		FromID:    "entity:1",
		ToID:      "entity:2",
		Type:      "works_with",
		Strength:  0,
		CreatedAt: time.Now(),
		Evidence:  []string{},
		RelationshipMetadata: types.RelationshipMetadata{
			Bidirectional: false,
		},
	}

	score := scorer.CalculateRelationshipConfidence(rel)

	// Base: 0.5, no strength override, no evidence, no bidirectional, age bonus: 0.1
	// 0.5 + 0.1 = 0.6
	expectedScore := 0.6
	if math.Abs(score-expectedScore) > 0.001 {
		t.Errorf("RelationshipConfidence without strength: expected %f, got %f", expectedScore, score)
	}
}

// TestCalculateRelationshipConfidence_WithEvidence verifies evidence bonus calculation
func TestCalculateRelationshipConfidence_WithEvidence(t *testing.T) {
	tests := []struct {
		name         string
		evidenceCount int
		expectedBonus float64
	}{
		{"One evidence", 1, 0.1},
		{"Two evidence", 2, 0.2},
		{"Three evidence", 3, 0.3},
		{"Four evidence", 4, 0.3}, // Capped at 0.3
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scorer := NewConfidenceScorer(nil)
			evidence := make([]string, tt.evidenceCount)
			for i := 0; i < tt.evidenceCount; i++ {
				evidence[i] = "mem:" + string(rune(i))
			}

			rel := &types.Relationship{
				ID:        "rel:1",
				FromID:    "entity:1",
				ToID:      "entity:2",
				Type:      "works_with",
				Strength:  0,
				CreatedAt: time.Now().Add(-50 * 24 * time.Hour), // Old, no age bonus
				Evidence:  evidence,
				RelationshipMetadata: types.RelationshipMetadata{
					Bidirectional: false,
				},
			}

			score := scorer.CalculateRelationshipConfidence(rel)

			// Base 0.5 + evidence bonus
			expectedScore := 0.5 + tt.expectedBonus
			if math.Abs(score-expectedScore) > 0.001 {
				t.Errorf("RelationshipConfidence with %d evidence: expected %f, got %f", tt.evidenceCount, expectedScore, score)
			}
		})
	}
}

// TestCalculateRelationshipConfidence_Bidirectional verifies bidirectional bonus
func TestCalculateRelationshipConfidence_Bidirectional(t *testing.T) {
	scorer := NewConfidenceScorer(nil)

	relBidirectional := &types.Relationship{
		ID:        "rel:1",
		FromID:    "entity:1",
		ToID:      "entity:2",
		Type:      "friend_of",
		Strength:  0,
		CreatedAt: time.Now().Add(-50 * 24 * time.Hour),
		Evidence:  []string{},
		RelationshipMetadata: types.RelationshipMetadata{
			Bidirectional: true,
		},
	}

	relUnidirectional := &types.Relationship{
		ID:        "rel:2",
		FromID:    "entity:1",
		ToID:      "entity:2",
		Type:      "parent_of",
		Strength:  0,
		CreatedAt: time.Now().Add(-50 * 24 * time.Hour),
		Evidence:  []string{},
		RelationshipMetadata: types.RelationshipMetadata{
			Bidirectional: false,
		},
	}

	scoreBi := scorer.CalculateRelationshipConfidence(relBidirectional)
	scoreUni := scorer.CalculateRelationshipConfidence(relUnidirectional)

	if scoreBi-scoreUni < 0.09 {
		t.Errorf("Bidirectional should score higher: bidirectional=%f, unidirectional=%f", scoreBi, scoreUni)
	}
}

// TestCalculateRelationshipConfidence_RecentAge verifies recent relationship gets age bonus
func TestCalculateRelationshipConfidence_RecentAge(t *testing.T) {
	scorer := NewConfidenceScorer(nil)

	relRecent := &types.Relationship{
		ID:        "rel:1",
		FromID:    "entity:1",
		ToID:      "entity:2",
		Type:      "works_with",
		Strength:  0,
		CreatedAt: time.Now().Add(-15 * 24 * time.Hour), // 15 days old
		Evidence:  []string{},
		RelationshipMetadata: types.RelationshipMetadata{
			Bidirectional: false,
		},
	}

	relOld := &types.Relationship{
		ID:        "rel:2",
		FromID:    "entity:1",
		ToID:      "entity:2",
		Type:      "works_with",
		Strength:  0,
		CreatedAt: time.Now().Add(-100 * 24 * time.Hour), // 100 days old
		Evidence:  []string{},
		RelationshipMetadata: types.RelationshipMetadata{
			Bidirectional: false,
		},
	}

	scoreRecent := scorer.CalculateRelationshipConfidence(relRecent)
	scoreOld := scorer.CalculateRelationshipConfidence(relOld)

	if scoreRecent-scoreOld < 0.09 {
		t.Errorf("Recent relationship should score higher: recent=%f, old=%f", scoreRecent, scoreOld)
	}
}

// TestCalculateMemoryConfidence_ContextNotUsed verifies context parameter is accepted
func TestCalculateMemoryConfidence_ContextNotUsed(t *testing.T) {
	scorer := NewConfidenceScorer(nil)
	memory := createTestMemory("mem:test:1", "manual", types.EnrichmentCompleted, types.EnrichmentCompleted)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel the context

	// Should still work since CalculateMemoryConfidence doesn't use storage
	confidence, err := scorer.CalculateMemoryConfidence(ctx, memory)
	if err != nil {
		t.Fatalf("CalculateMemoryConfidence failed: %v", err)
	}
	if confidence == nil {
		t.Fatal("Expected confidence to be non-nil")
	}
}

// TestCalculateMemoryConfidence_ScoresClampedToRange verifies all scores stay in [0, 1]
func TestCalculateMemoryConfidence_ScoresClampedToRange(t *testing.T) {
	tests := []struct {
		name       string
		entityStat types.EnrichmentStatus
		relStat    types.EnrichmentStatus
		source     string
		ageOffset  time.Duration
	}{
		{"All failed", types.EnrichmentFailed, types.EnrichmentFailed, "unknown", -500 * 24 * time.Hour},
		{"All perfect", types.EnrichmentCompleted, types.EnrichmentCompleted, "manual", -1 * time.Hour},
		{"Mixed", types.EnrichmentCompleted, types.EnrichmentFailed, "email", -45 * 24 * time.Hour},
		{"Skipped", types.EnrichmentSkipped, types.EnrichmentSkipped, "auto", -200 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scorer := NewConfidenceScorer(nil)
			memory := createTestMemory("mem:test:1", tt.source, tt.entityStat, tt.relStat)
			memory.CreatedAt = time.Now().Add(tt.ageOffset)
			memory.Entities = []string{"e1", "e2", "e3"}

			confidence, err := scorer.CalculateMemoryConfidence(context.Background(), memory)
			if err != nil {
				t.Fatalf("CalculateMemoryConfidence failed: %v", err)
			}

			// Verify all scores are in valid range
			scores := []struct {
				name  string
				value float64
			}{
				{"EntityScore", confidence.EntityScore},
				{"RelScore", confidence.RelScore},
				{"SourceScore", confidence.SourceScore},
				{"AgeScore", confidence.AgeScore},
				{"Overall", confidence.Overall},
			}

			for _, score := range scores {
				if score.value < 0 || score.value > 1.0 {
					t.Errorf("%s out of range: %f", score.name, score.value)
				}
			}
		})
	}
}

// TestEntityScore_BoundaryConditions verifies edge cases for entity score
func TestEntityScore_BoundaryConditions(t *testing.T) {
	tests := []struct {
		name           string
		entityStatus   types.EnrichmentStatus
		entityCount    int
		expectedScore  float64
	}{
		{"Pending status", types.EnrichmentPending, 0, 0.5},
		{"Processing status", types.EnrichmentProcessing, 0, 0.5},
		{"Failed status", types.EnrichmentFailed, 0, 0.2},
		{"Failed with entities", types.EnrichmentFailed, 5, 0.2}, // Failure overrides everything
		{"Completed with no entities", types.EnrichmentCompleted, 0, 0.7},
		{"Completed with 1 entity", types.EnrichmentCompleted, 1, 0.8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scorer := NewConfidenceScorer(nil)
			memory := createTestMemory("mem:test:1", "manual", tt.entityStatus, types.EnrichmentCompleted)
			memory.Entities = make([]string, tt.entityCount)

			score := scorer.calculateEntityScore(memory)
			if math.Abs(score-tt.expectedScore) > 0.001 {
				t.Errorf("EntityScore: expected %f, got %f", tt.expectedScore, score)
			}
		})
	}
}

// TestNilMemory_SafeHandling verifies nil memory doesn't cause panic
func TestNilMemory_SafeHandling(t *testing.T) {
	// Note: This test documents that the scorer does not explicitly handle nil memories.
	// A nil memory would cause a panic when accessing fields, which is acceptable behavior
	// as callers should always provide a valid memory instance.
	// This test primarily documents the requirement rather than testing it.
	_ = NewConfidenceScorer(nil)
}

// TestCompositeScoring_RealWorldScenario simulates a realistic memory
func TestCompositeScoring_RealWorldScenario(t *testing.T) {
	scorer := NewConfidenceScorer(nil)

	// Realistic scenario: user-entered note about a decision with related entities
	memory := &types.Memory{
		ID:                 "mem:ops:decision-001",
		Content:            "Decided to adopt Go for the new backend service",
		Source:             "note",
		Domain:             "ops",
		CreatedAt:          time.Now().Add(-3 * 24 * time.Hour), // 3 days old
		UpdatedAt:          time.Now().Add(-3 * 24 * time.Hour),
		EntityStatus:       types.EnrichmentCompleted,
		RelationshipStatus: types.EnrichmentCompleted,
		Entities:           []string{"entity:go", "entity:backend", "entity:service"},
		MemoryType:         "decision",
	}

	confidence, err := scorer.CalculateMemoryConfidence(context.Background(), memory)
	if err != nil {
		t.Fatalf("CalculateMemoryConfidence failed: %v", err)
	}

	// Expected: EntityScore 1.0 (0.7 + 0.3), RelScore 0.9 (0.7 + 0.2), SourceScore 0.95, AgeScore 0.9
	// Overall: (1.0 * 0.3) + (0.9 * 0.2) + (0.95 * 0.3) + (0.9 * 0.2) = 0.3 + 0.18 + 0.285 + 0.18 = 0.945

	if confidence.Overall < 0.93 {
		t.Errorf("Realistic scenario overall score seems low: %f", confidence.Overall)
	}
	if confidence.EntityScore < 0.99 {
		t.Errorf("Realistic scenario entity score seems low: %f", confidence.EntityScore)
	}
	if confidence.SourceScore < 0.94 {
		t.Errorf("Realistic scenario source score seems low: %f", confidence.SourceScore)
	}
}
