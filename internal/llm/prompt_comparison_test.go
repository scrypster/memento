// +build integration

package llm

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestPromptComparison compares different prompt formulations
func TestPromptComparison(t *testing.T) {
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
	t.Logf("Comparing prompts with model: %s\n\n", model)

	// Test content
	testContent := `MJ Bonanno is the founder of Scrypster and is married to Norma Bonanno.
Norma works at Google as a senior engineer. Their son Rosario is a software engineer.
The Memento project uses Python and PostgreSQL for its backend. MJ specializes in Go.`

	// Define prompt variations with inline generators
	variations := []struct {
		Name    string
		Prompt  string
	}{
		{
			Name:   "CURRENT (Ultra-Strict with Format Example)",
			Prompt: EntityExtractionPrompt(testContent),
		},
		{
			Name: "MINIMAL",
			Prompt: fmt.Sprintf(`Extract entities: person|organization|tool|project
JSON: {"entities":[{"name":"X","type":"person","description":"...","confidence":0.9}]}

Text: %s

JSON:`, testContent),
		},
		{
			Name: "EXAMPLES",
			Prompt: fmt.Sprintf(`Extract entities from text.

EXAMPLES:
- Input: "Alice works at Google"
  Output: {"entities":[{"name":"Alice","type":"person","description":"works at Google","confidence":0.9},{"name":"Google","type":"organization","description":"tech company","confidence":0.9}]}

Text: %s

Return JSON:`, testContent),
		},
		{
			Name: "STEP_BY_STEP",
			Prompt: fmt.Sprintf(`Extract entities from text.

STEP 1: Find all person names
STEP 2: Find all organization names
STEP 3: Find all tools/technologies
STEP 4: Find all projects
STEP 5: Return as JSON

Types: person, organization, tool, project

Text: %s

Return ONLY JSON:
{"entities":[...]}`, testContent),
		},
	}

	// Test each variation
	for _, variation := range variations {
		t.Logf("\n=== Testing: %s ===\n", variation.Name)

		start := time.Now()
		response, err := client.Complete(ctx, variation.Prompt)
		duration := time.Since(start)

		if err != nil {
			t.Logf("❌ LLM Error: %v (%.2fs)\n", err, duration.Seconds())
			continue
		}

		// Show response preview
		preview := response
		if len(preview) > 150 {
			preview = preview[:150] + "..."
		}
		t.Logf("Response preview: %s\n", preview)

		// Try to parse
		entities, parseErr := ParseEntityResponse(response)

		if parseErr != nil {
			t.Logf("❌ Parse Error: %v (%.2fs)\n", parseErr, duration.Seconds())
		} else {
			t.Logf("✅ Parsed successfully: %d entities (%.2fs)\n", len(entities), duration.Seconds())
			if len(entities) > 0 {
				for i, e := range entities {
					if i >= 3 { // Show first 3
						t.Logf("   ... and %d more", len(entities)-3)
						break
					}
					t.Logf("   - %s (%s)\n", e.Name, e.Type)
				}
			}
		}
	}

	t.Logf("\n=== COMPARISON SUMMARY ===\n")
	t.Logf("Check which prompt variation produces the best results.\n")
	t.Logf("Best result will be used for optimization cycle.\n")
}

// TestEntityExtractionConsistency tests if the prompt produces consistent results
func TestEntityExtractionConsistency(t *testing.T) {
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
	t.Logf("Testing consistency with model: %s (3 runs)\n\n", model)

	testContent := `MJ Bonanno is the founder of Scrypster and is married to Norma Bonanno.
Norma works at Google as a senior engineer. Their son Rosario is a software engineer.
The Memento project uses Python and PostgreSQL for its backend. MJ specializes in Go.`

	successCount := 0
	totalEntityCount := 0

	for i := 1; i <= 3; i++ {
		t.Logf("Run %d:\n", i)

		prompt := EntityExtractionPrompt(testContent)
		response, err := client.Complete(ctx, prompt)

		if err != nil {
			t.Logf("  ❌ Error: %v\n", err)
			continue
		}

		entities, parseErr := ParseEntityResponse(response)

		if parseErr != nil {
			t.Logf("  ❌ Parse Error: %v\n", parseErr)
		} else {
			t.Logf("  ✅ Success: %d entities\n", len(entities))
			successCount++
			totalEntityCount += len(entities)
		}
	}

	t.Logf("\n=== RESULTS ===\n")
	t.Logf("Success Rate: %d/3 (%.0f%%)\n", successCount, float64(successCount)/3.0*100)
	if successCount > 0 {
		t.Logf("Average entities per run: %.1f\n", float64(totalEntityCount)/float64(successCount))
	}
}
