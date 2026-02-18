// +build integration

package llm

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestEntityExtractionDebug debugs the entity extraction prompt and response
func TestEntityExtractionDebug(t *testing.T) {
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
	t.Logf("Testing entity extraction with model: %s\n", model)

	// Test content
	testContent := `MJ Bonanno is the founder of Scrypster and is married to Norma Bonanno.
Norma works at Google as a senior engineer. Their son Rosario is a software engineer.
The Memento project uses Python and PostgreSQL for its backend. MJ specializes in Go.`

	t.Logf("Test Content:\n%s\n", testContent)

	// Generate and show the prompt
	prompt := EntityExtractionPrompt(testContent)
	t.Logf("=== PROMPT (first 500 chars) ===\n%s...\n\n", prompt[:min(500, len(prompt))])

	// Call the LLM
	t.Log("=== CALLING LLM ===")
	start := time.Now()
	response, err := client.Complete(ctx, prompt)
	duration := time.Since(start)

	t.Logf("Duration: %.2fs\n", duration.Seconds())

	if err != nil {
		t.Logf("❌ LLM Call Error: %v\n", err)
		return
	}

	// Show raw response
	t.Logf("\n=== RAW RESPONSE (full) ===\n%s\n", response)

	// Try to parse
	t.Log("\n=== PARSING RESPONSE ===")
	entities, parseErr := ParseEntityResponse(response)

	if parseErr != nil {
		t.Logf("❌ Parse Error: %v\n", parseErr)
		t.Logf("\nThis could indicate:")
		t.Logf("1. LLM returned malformed JSON")
		t.Logf("2. LLM returned no entities")
		t.Logf("3. LLM returned markdown code blocks")
		t.Logf("4. Entity type validation failed")
		return
	}

	t.Logf("✅ Parsed Successfully\n")
	t.Logf("Entities found: %d\n", len(entities))

	if len(entities) == 0 {
		t.Logf("\n⚠️  WARNING: No entities extracted!")
		t.Logf("This suggests:")
		t.Logf("1. The LLM isn't finding entities in the text")
		t.Logf("2. The prompt formatting needs adjustment")
		t.Logf("3. The LLM might need more specific instructions")
		return
	}

	for i, e := range entities {
		t.Logf("  %d. %s (%s) - confidence: %.2f\n", i+1, e.Name, e.Type, e.Confidence)
	}

	t.Logf("\n✅ SUCCESS: Entity extraction working correctly")
}
