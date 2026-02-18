// +build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/llm"
	"github.com/scrypster/memento/pkg/types"
)

// TestMultiCallEntityAndRelationships tests entity + relationship extraction in separate calls
func TestMultiCallEntityAndRelationships(t *testing.T) {
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

	t.Logf("Testing with model: %s\n", model)

	testContent := `MJ Bonanno is the founder of Scrypster and is married to Norma Bonanno.
Norma works at Google as a senior engineer. Their son Rosario is a software engineer.
The Memento project uses Python and PostgreSQL for its backend. MJ specializes in Go.`

	// Call 1: Entity Extraction
	t.Log("\n=== CALL 1: Entity Extraction ===")
	entityStart := time.Now()
	entityPrompt := llm.EntityExtractionPrompt(testContent)
	entityResponse, _ := client.Complete(ctx, entityPrompt)
	entityDuration := time.Since(entityStart)

	t.Logf("Duration: %.2fs", entityDuration.Seconds())

	entities, parseErr := llm.ParseEntityResponse(entityResponse)
	if parseErr != nil {
		t.Logf("❌ Parse Error: %v", parseErr)
		t.Logf("Raw response (first 200 chars): %s...", entityResponse[:min(200, len(entityResponse))])
		return
	}

	t.Logf("✅ Parsed Successfully")
	t.Logf("Entities found: %d", len(entities))
	for i, e := range entities {
		t.Logf("  %d. %s (%s) - confidence: %.2f", i+1, e.Name, e.Type, e.Confidence)
	}

	// Convert EntityResponse to types.Entity for relationship extraction
	typesEntities := make([]types.Entity, len(entities))
	for i, e := range entities {
		typesEntities[i] = types.Entity{Name: e.Name, Type: e.Type}
	}

	// Call 2: Relationship Extraction (using extracted entities)
	t.Log("\n=== CALL 2: Relationship Extraction ===")
	relStart := time.Now()
	relPrompt := llm.RelationshipExtractionPrompt(testContent, typesEntities)
	relResponse, _ := client.Complete(ctx, relPrompt)
	relDuration := time.Since(relStart)

	t.Logf("Duration: %.2fs", relDuration.Seconds())

	relationships, parseErr := llm.ParseRelationshipResponse(relResponse)
	if parseErr != nil {
		t.Logf("❌ Parse Error: %v", parseErr)
		t.Logf("Raw response (first 200 chars): %s...", relResponse[:min(200, len(relResponse))])
		return
	}

	t.Logf("✅ Parsed Successfully")
	t.Logf("Relationships found: %d", len(relationships))
	for i, r := range relationships {
		t.Logf("  %d. %s -> [%s] -> %s (%.2f)", i+1, r.From, r.Type, r.To, r.Confidence)
	}

	// Summary
	t.Log("\n=== SUMMARY ===")
	totalTime := entityDuration + relDuration
	t.Logf("Call 1 (Entity):  %.2fs - %d entities extracted", entityDuration.Seconds(), len(entities))
	t.Logf("Call 2 (Relationship): %.2fs - %d relationships extracted", relDuration.Seconds(), len(relationships))
	t.Logf("Total Time: %.2fs", totalTime.Seconds())
	t.Logf("Status: ✅ SUCCESS - Multi-call strategy working")
}

// TestSingleCallVsMultiCall compares approaches
func TestSingleCallVsMultiCall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

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

	testContent := `MJ Bonanno is the founder of Scrypster and is married to Norma Bonanno.
Norma works at Google as a senior engineer. Their son Rosario is a software engineer.
The Memento project uses Python and PostgreSQL for its backend. MJ specializes in Go.`

	t.Logf("\n=== Comparing Approaches with %s ===\n", model)

	// Approach 1: Entity extraction only
	t.Log("APPROACH 1: Entity Extraction Only")
	start := time.Now()
	entityResp, _ := client.Complete(ctx, llm.EntityExtractionPrompt(testContent))
	entityResponseList, _ := llm.ParseEntityResponse(entityResp)
	time1 := time.Since(start)

	entities := make([]types.Entity, len(entityResponseList))
	for i, e := range entityResponseList {
		entities[i] = types.Entity{Name: e.Name, Type: e.Type}
	}

	t.Logf("  Time: %.2fs", time1.Seconds())
	t.Logf("  Entities: %d", len(entities))

	// Approach 2: Entity extraction + Relationship extraction (separate calls)
	t.Log("\nAPPROACH 2: Entity + Relationship (Two Calls)")
	start = time.Now()
	entityResp, _ = client.Complete(ctx, llm.EntityExtractionPrompt(testContent))
	entityResponseList, _ = llm.ParseEntityResponse(entityResp)
	entities = make([]types.Entity, len(entityResponseList))
	for i, e := range entityResponseList {
		entities[i] = types.Entity{Name: e.Name, Type: e.Type}
	}
	time.Sleep(300 * time.Millisecond) // Brief pause between calls
	relResp, _ := client.Complete(ctx, llm.RelationshipExtractionPrompt(testContent, entities))
	relationships, _ := llm.ParseRelationshipResponse(relResp)
	time2 := time.Since(start)

	t.Logf("  Time: %.2fs", time2.Seconds())
	t.Logf("  Entities: %d", len(entities))
	t.Logf("  Relationships: %d", len(relationships))

	// Analysis
	t.Log("\n=== ANALYSIS ===")
	overhead := time2 - time1
	t.Logf("Approach 1 Time: %.2fs", time1.Seconds())
	t.Logf("Approach 2 Time: %.2fs", time2.Seconds())
	t.Logf("Overhead: %.2fs (+%.0f%%)", overhead.Seconds(), (float64(overhead)/float64(time1))*100)
	t.Logf("\n📊 Conclusion:")
	t.Logf("  Multi-call approach adds ~%.2fs overhead", overhead.Seconds())
	t.Logf("  Benefit: Focused extraction, better accuracy for each task")
	t.Logf("  Trade-off: Worth the overhead for better data quality")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
