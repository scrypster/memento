// +build integration

package llm

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/scrypster/memento/pkg/types"
)

// TestMultiCallEntityExtractionOnly tests just the entity extraction part of multi-call
// This isolates the entity extraction step to debug why it returns 0 entities
func TestMultiCallEntityExtractionOnly(t *testing.T) {
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

	config := OllamaConfig{
		BaseURL: baseURL,
		Model:   model,
		Timeout: 30 * time.Second,
	}

	client := NewOllamaClient(config)
	t.Logf("Testing entity extraction (multi-call scenario) with model: %s\n", model)

	// Use EXACT test content from multi-call test
	testContent := `MJ Bonanno is the founder of Scrypster and is married to Norma Bonanno.
Norma works at Google as a senior engineer. Their son Rosario is a software engineer.
The Memento project uses Python and PostgreSQL for its backend. MJ specializes in Go.`

	t.Logf("Test Content:\n%s\n\n", testContent)

	// Call 1: Entity Extraction (with improved prompt)
	t.Log("=== CALL 1: Entity Extraction ===")
	entityStart := time.Now()
	entityPrompt := EntityExtractionPrompt(testContent)
	entityResponse, err := client.Complete(ctx, entityPrompt)
	entityDuration := time.Since(entityStart)

	t.Logf("Duration: %.2fs\n", entityDuration.Seconds())

	if err != nil {
		t.Logf("❌ LLM Error: %v\n", err)
		return
	}

	// Show raw response for debugging
	t.Logf("\nRaw Response (full):\n%s\n", entityResponse)

	// Parse the response
	entities, parseErr := ParseEntityResponse(entityResponse)
	if parseErr != nil {
		t.Logf("❌ Parse Error: %v\n", parseErr)
		t.Logf("\nDebug Info:")
		t.Logf("  Response length: %d bytes\n", len(entityResponse))
		t.Logf("  First 100 chars: %s\n", entityResponse[:min(100, len(entityResponse))])
		return
	}

	t.Logf("✅ Parsed Successfully\n")
	t.Logf("Entities found: %d\n", len(entities))

	if len(entities) == 0 {
		t.Logf("⚠️  WARNING: Zero entities extracted!\n")
		t.Logf("This is the issue being investigated.\n")
		return
	}

	// Show extracted entities
	for i, e := range entities {
		t.Logf("  %d. %s (%s) - description: %s - confidence: %.2f\n",
			i+1, e.Name, e.Type, e.Description, e.Confidence)
	}

	// Convert to types.Entity (as done in multi-call test)
	t.Log("\n=== Converting to types.Entity ===")
	typesEntities := make([]types.Entity, len(entities))
	for i, e := range entities {
		typesEntities[i] = types.Entity{
			Name: e.Name,
			Type: e.Type,
		}
		t.Logf("  %d. %s (%s)\n", i+1, typesEntities[i].Name, typesEntities[i].Type)
	}

	// Verify conversion
	if len(typesEntities) == len(entities) {
		t.Logf("\n✅ Successfully converted %d entities to types.Entity\n", len(typesEntities))
	} else {
		t.Logf("\n❌ Conversion failed: expected %d, got %d\n", len(entities), len(typesEntities))
	}

	// Show what would be passed to relationship extraction
	t.Logf("\n=== Entities for Relationship Extraction ===\n")
	t.Logf("Would pass %d entities to Call 2\n", len(typesEntities))
	if len(typesEntities) == 0 {
		t.Logf("⚠️  WARNING: Zero entities to pass for relationship extraction!\n")
	}

	t.Logf("\n=== SUMMARY ===\n")
	t.Logf("Call 1 Duration: %.2fs\n", entityDuration.Seconds())
	t.Logf("Entities Extracted: %d\n", len(entities))
	t.Logf("Status: ✅ Entity extraction %s\n", func() string {
		if len(entities) > 0 {
			return "WORKING"
		}
		return "NOT WORKING - 0 entities"
	}())
}

// TestEntityExtractionWithTestPayloads tests entity extraction on all test payloads
func TestEntityExtractionWithTestPayloads(t *testing.T) {
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

	config := OllamaConfig{
		BaseURL: baseURL,
		Model:   model,
		Timeout: 30 * time.Second,
	}

	client := NewOllamaClient(config)
	t.Logf("Testing entity extraction on all test payloads with model: %s\n\n", model)

	// Test payloads
	payloads := map[string]string{
		"simple":   `Alice works at Google using Python. She manages the DataPipeline project.`,
		"medium":   `MJ Bonanno is the founder of Scrypster and is married to Norma Bonanno.\nNorma works at Google as a senior engineer. Their son Rosario is a software engineer.\nThe Memento project uses Python and PostgreSQL for its backend. MJ specializes in Go.`,
		"complex":  `Alice and Bob are colleagues at TechCorp working on the Memento project.\nAlice uses Python and manages technical infrastructure. Bob specializes in Go and backend development.\nBoth use Docker for deployment. The Memento project depends on PostgreSQL for data storage.\nThey also collaborate with Charlie from the DevOps team. The entire stack includes React for frontend,\nNode.js for services, Kubernetes for orchestration, and Redis for caching.`,
	}

	results := make(map[string]int)

	for payloadName, content := range payloads {
		t.Logf("=== Testing: %s ===\n", payloadName)

		prompt := EntityExtractionPrompt(content)
		response, err := client.Complete(ctx, prompt)

		if err != nil {
			t.Logf("❌ LLM Error: %v\n", err)
			results[payloadName] = -1
			continue
		}

		entities, parseErr := ParseEntityResponse(response)

		if parseErr != nil {
			t.Logf("❌ Parse Error: %v\n", parseErr)
			results[payloadName] = -1
			continue
		}

		t.Logf("✅ Entities: %d\n", len(entities))
		results[payloadName] = len(entities)

		// Show first few entities
		for i := 0; i < len(entities) && i < 3; i++ {
			t.Logf("   - %s (%s)\n", entities[i].Name, entities[i].Type)
		}
		if len(entities) > 3 {
			t.Logf("   ... and %d more\n", len(entities)-3)
		}
		t.Logf("")
	}

	t.Logf("\n=== SUMMARY ===\n")
	successCount := 0
	for name, count := range results {
		if count > 0 {
			t.Logf("✅ %s: %d entities\n", name, count)
			successCount++
		} else {
			t.Logf("❌ %s: Failed\n", name)
		}
	}
	t.Logf("\nSuccess Rate: %d/%d\n", successCount, len(results))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
