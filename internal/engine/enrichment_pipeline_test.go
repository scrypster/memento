// +build integration

package engine

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/llm"
	"github.com/scrypster/memento/pkg/types"
)

// TestFullPipelineWithClassificationAndSummary tests the complete 4-call extraction pipeline
// Call 1: Entity Extraction
// Call 2: Relationship Extraction
// Call 3: Classification Extraction (NEW)
// Call 4: Summarization (NEW)
func TestFullPipelineWithClassificationAndSummary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Setup
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen2.5:7b"
	}

	baseURL := os.Getenv("OLLAMA_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	config := llm.OllamaConfig{
		BaseURL: baseURL,
		Model:   model,
		Timeout: 30 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	t.Logf("Testing full 4-call pipeline with model: %s\n", model)

	// Create in-memory SQLite database for testing
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Create minimal schema for testing
	schema := `
	CREATE TABLE memories (
		id TEXT PRIMARY KEY,
		content TEXT,
		memory_type TEXT,
		category TEXT,
		classification TEXT,
		priority TEXT,
		context_labels TEXT,
		tags TEXT,
		summary TEXT,
		key_points TEXT,
		entity_status TEXT,
		relationship_status TEXT,
		classification_status TEXT,
		summarization_status TEXT,
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	);
	CREATE TABLE entities (id TEXT PRIMARY KEY, name TEXT, type TEXT, description TEXT, created_at TIMESTAMP, updated_at TIMESTAMP);
	CREATE TABLE relationships (id TEXT PRIMARY KEY, source_id TEXT, target_id TEXT, type TEXT, weight REAL, created_at TIMESTAMP, updated_at TIMESTAMP);
	CREATE TABLE memory_entities (memory_id TEXT, entity_id TEXT, frequency INTEGER, confidence REAL, created_at TIMESTAMP);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	// Insert test memory
	memoryID := "mem:test:001"
	content := `MJ Bonanno is the founder of Scrypster and is married to Norma Bonanno.
Norma works at Google as a senior engineer. Their son Rosario is a software engineer.
The Memento project uses Python and PostgreSQL for its backend. MJ specializes in Go.
This is a critical architectural decision that impacts performance and scalability.`

	now := time.Now()
	_, err = db.Exec(
		"INSERT INTO memories (id, content, entity_status, relationship_status, classification_status, summarization_status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		memoryID, content, types.EnrichmentPending, types.EnrichmentPending, types.EnrichmentPending, types.EnrichmentPending, now, now,
	)
	if err != nil {
		t.Fatalf("Failed to insert test memory: %v", err)
	}

	// Create pipeline and extract
	pipeline := NewExtractionPipeline(client, db)
	result, err := pipeline.Extract(ctx, memoryID, content)

	if err != nil {
		t.Logf("Pipeline returned error (may be OK if at least some extractions succeeded): %v\n", err)
	}

	// Verify results
	t.Logf("\n=== FULL PIPELINE RESULTS ===\n")

	// Call 1: Entity Extraction
	t.Logf("CALL 1 - Entity Extraction:\n")
	t.Logf("  Status: %s\n", result.EntityStatus)
	if result.EntityStatus == types.EnrichmentFailed {
		t.Logf("  Error: %s\n", result.EntityError)
	} else {
		t.Logf("  Entities extracted: %d\n", len(result.Entities))
		for i, entity := range result.Entities {
			t.Logf("    %d. %s (%s) - Confidence: %.2f\n", i+1, entity.Name, entity.Type, entity.Confidence)
		}
	}

	// Call 2: Relationship Extraction
	t.Logf("\nCALL 2 - Relationship Extraction:\n")
	t.Logf("  Status: %s\n", result.RelationshipStatus)
	if result.RelationshipStatus == types.EnrichmentFailed {
		t.Logf("  Error: %s\n", result.RelationshipError)
	} else if result.RelationshipStatus == types.EnrichmentSkipped {
		t.Logf("  Skipped: No entities to relate\n")
	} else {
		t.Logf("  Relationships extracted: %d\n", len(result.Relationships))
		for i, rel := range result.Relationships {
			t.Logf("    %d. %s --[%s]--> %s (Confidence: %.2f)\n", i+1, rel.From, rel.Type, rel.To, rel.Confidence)
		}
	}

	// Call 3: Classification Extraction (NEW)
	t.Logf("\nCALL 3 - Classification Extraction (NEW):\n")
	t.Logf("  Status: %s\n", result.ClassificationStatus)
	if result.ClassificationStatus == types.EnrichmentFailed {
		t.Logf("  Error: %s\n", result.ClassificationError)
	} else if result.Classification != nil {
		t.Logf("  Memory Type: %s\n", result.Classification.MemoryType)
		t.Logf("  Category: %s\n", result.Classification.Category)
		t.Logf("  Classification: %s\n", result.Classification.Classification)
		t.Logf("  Priority: %s\n", result.Classification.Priority)
		t.Logf("  Context Labels: %v\n", result.Classification.ContextLabels)
		t.Logf("  Tags: %v\n", result.Classification.Tags)
		t.Logf("  Confidence: %.2f\n", result.Classification.Confidence)
	}

	// Call 4: Summarization (NEW)
	t.Logf("\nCALL 4 - Summarization (NEW):\n")
	t.Logf("  Status: %s\n", result.SummarizationStatus)
	if result.SummarizationStatus == types.EnrichmentFailed {
		t.Logf("  Error: %s\n", result.SummarizationError)
	} else if result.Summary != nil {
		summaryPreview := result.Summary.Summary
		if len(summaryPreview) > 100 {
			summaryPreview = summaryPreview[:100] + "..."
		}
		t.Logf("  Summary: %s\n", summaryPreview)
		t.Logf("  Key Points (%d):\n", len(result.Summary.KeyPoints))
		for i, point := range result.Summary.KeyPoints {
			t.Logf("    %d. %s\n", i+1, point)
		}
	}

	// Final verification
	t.Logf("\n=== PIPELINE SUMMARY ===\n")
	t.Logf("Execution Time: %.2f seconds\n", time.Since(result.ExecutedAt).Seconds())
	t.Logf("Entity Status:      %s\n", result.EntityStatus)
	t.Logf("Relationship Status: %s\n", result.RelationshipStatus)
	t.Logf("Classification Status: %s (NEW)\n", result.ClassificationStatus)
	t.Logf("Summarization Status:  %s (NEW)\n", result.SummarizationStatus)

	// Check success criteria
	if result.EntityStatus != types.EnrichmentCompleted {
		t.Error("Entity extraction should be completed")
	}
	if result.ClassificationStatus != types.EnrichmentCompleted {
		t.Error("Classification extraction should be completed")
	}
	if result.SummarizationStatus != types.EnrichmentCompleted {
		t.Error("Summarization should be completed")
	}

	if result.Classification == nil {
		t.Error("Classification should not be nil")
	} else {
		if !types.IsValidMemoryType(result.Classification.MemoryType) {
			t.Errorf("Invalid memory type: %s", result.Classification.MemoryType)
		}
		if result.Classification.Category == "" {
			t.Error("Category should not be empty")
		}
		if result.Classification.Priority == "" {
			t.Error("Priority should not be empty")
		}
	}

	if result.Summary == nil {
		t.Error("Summary should not be nil")
	} else {
		if result.Summary.Summary == "" {
			t.Error("Summary text should not be empty")
		}
		if len(result.Summary.KeyPoints) == 0 {
			t.Error("Key points should not be empty")
		}
	}

	t.Logf("\n✅ Full pipeline test completed successfully!\n")
}
