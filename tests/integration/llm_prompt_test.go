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

// TestEntityExtractionPromptQuality verifies entity extraction accuracy
func TestEntityExtractionPromptQuality(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Get model from environment, default to qwen2.5:7b
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

	testCases := []struct {
		name             string
		content          string
		expectedEntities map[string]string // name -> type
		minAccuracy      float64
	}{
		{
			name: "Personal and Professional Mix",
			content: `MJ Bonanno is the founder of Scrypster. He is married to Norma Bonanno who works at Google.
Their son Rosario is a software engineer. The Memento project uses Python and PostgreSQL.
MJ specializes in Go and machine learning. The team also uses Docker for deployment.`,
			expectedEntities: map[string]string{
				"MJ Bonanno":      "person",
				"Scrypster":       "organization",
				"Norma Bonanno":   "person",
				"Google":          "organization",
				"Rosario":         "person",
				"Python":          "tool",
				"PostgreSQL":      "tool",
				"Go":              "tool",
				"Docker":          "tool",
				"Memento":         "project",
			},
			minAccuracy: 0.70,
		},
		{
			name: "Technology Stack",
			content: `Our microservices architecture uses Go for backends and React for frontend.
We store data in PostgreSQL and cache with Redis. Kubernetes orchestrates our containers.
Docker is used for all deployments. TypeScript is used for type safety.`,
			expectedEntities: map[string]string{
				"Go":           "tool",
				"React":        "tool",
				"PostgreSQL":   "tool",
				"Redis":        "tool",
				"Kubernetes":   "tool",
				"Docker":       "tool",
				"TypeScript":   "tool",
			},
			minAccuracy: 0.70,
		},
		{
			name: "Organizations and People",
			content: `Alice works at Microsoft as an engineer. Bob is at Apple in design. Charlie leads at Google.
Diana manages operations at Amazon. Elena is a consultant at Meta working on AI.
Frank works at OpenAI on safety. Grace is the CTO at DeepMind.`,
			expectedEntities: map[string]string{
				"Alice":      "person",
				"Microsoft":  "organization",
				"Bob":        "person",
				"Apple":      "organization",
				"Charlie":    "person",
				"Google":     "organization",
				"Diana":      "person",
				"Amazon":     "organization",
				"Elena":      "person",
				"Meta":       "organization",
				"Frank":      "person",
				"OpenAI":     "organization",
				"Grace":      "person",
				"DeepMind":   "organization",
			},
			minAccuracy: 0.75,
		},
	}

	t.Logf("Testing with model: %s", model)
	t.Logf("Base URL: %s\n", baseURL)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate entity extraction prompt
			prompt := llm.EntityExtractionPrompt(tc.content)

			// Call LLM
			response, err := client.Complete(ctx, prompt)
			if err != nil {
				t.Fatalf("LLM completion failed: %v", err)
			}

			// Parse response
			entities, err := llm.ParseEntityResponse(response)
			if err != nil {
				t.Logf("Failed to parse entity response: %v", err)
				t.Logf("Raw response:\n%s\n", response)
				return
			}

			// Build extraction map
			extracted := make(map[string]*llm.EntityResponse)
			for i := range entities {
				extracted[entities[i].Name] = &entities[i]
			}

			// Score accuracy
			verified := 0
			correctType := 0

			for expectedName, expectedType := range tc.expectedEntities {
				entity, found := extracted[expectedName]
				if !found {
					t.Logf("  ⚠ Missing: %q (expected type: %s)", expectedName, expectedType)
					continue
				}

				verified++
				if entity.Type == expectedType {
					correctType++
					t.Logf("  ✓ %s (%s) confidence: %.2f", entity.Name, entity.Type, entity.Confidence)
				} else {
					t.Logf("  ◐ %s (got %q, expected %q) confidence: %.2f", entity.Name, entity.Type, expectedType, entity.Confidence)
				}
			}

			accuracy := float64(correctType) / float64(len(tc.expectedEntities))
			extractionRate := float64(verified) / float64(len(tc.expectedEntities))

			t.Logf("\nResults:")
			t.Logf("  Extracted: %d/%d entities (%.0f%%)", verified, len(tc.expectedEntities), extractionRate*100)
			t.Logf("  Correct types: %d/%d entities (%.0f%%)", correctType, len(tc.expectedEntities), accuracy*100)
			t.Logf("  Total entities found: %d\n", len(entities))

			if accuracy >= tc.minAccuracy {
				t.Logf("✓ PASS: Accuracy %.0f%% meets minimum %.0f%%", accuracy*100, tc.minAccuracy*100)
			} else {
				t.Logf("✗ FAIL: Accuracy %.0f%% below minimum %.0f%%", accuracy*100, tc.minAccuracy*100)
				t.Fail()
			}
		})
	}
}

// TestRelationshipExtractionPromptQuality verifies relationship extraction accuracy
func TestRelationshipExtractionPromptQuality(t *testing.T) {
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

	testCases := []struct {
		name        string
		content     string
		entities    []types.Entity
		expectedRels map[string]int // type -> min count
	}{
		{
			name: "Family Relationships",
			content: `MJ Bonanno is married to Norma Bonanno. They have a son named Rosario.
MJ's brother is Anthony. Norma's sister is Sofia.`,
			entities: []types.Entity{
				{Name: "MJ Bonanno", Type: "person"},
				{Name: "Norma Bonanno", Type: "person"},
				{Name: "Rosario", Type: "person"},
				{Name: "Anthony", Type: "person"},
				{Name: "Sofia", Type: "person"},
			},
			expectedRels: map[string]int{
				"married_to":  2, // bidirectional
				"parent_of":   2, // both parents to child
				"sibling_of":  2, // bidirectional
			},
		},
		{
			name: "Professional Relationships",
			content: `Alice and Bob are colleagues at TechCorp working on the Memento project.
Alice uses Python. Bob uses Go. Both use Docker.
Memento depends on PostgreSQL.`,
			entities: []types.Entity{
				{Name: "Alice", Type: "person"},
				{Name: "Bob", Type: "person"},
				{Name: "TechCorp", Type: "organization"},
				{Name: "Memento", Type: "project"},
				{Name: "Python", Type: "tool"},
				{Name: "Go", Type: "tool"},
				{Name: "Docker", Type: "tool"},
				{Name: "PostgreSQL", Type: "tool"},
			},
			expectedRels: map[string]int{
				"colleague_of": 2, // bidirectional
				"employed_by":  2, // both at TechCorp
				"works_on":     2, // both on Memento
				"uses":         3, // Alice->Python, Bob->Go, both->Docker
				"depends_on":   1, // Memento->PostgreSQL
			},
		},
	}

	t.Logf("Testing with model: %s\n", model)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prompt := llm.RelationshipExtractionPrompt(tc.content, tc.entities)

			response, err := client.Complete(ctx, prompt)
			if err != nil {
				t.Fatalf("LLM completion failed: %v", err)
			}

			relationships, err := llm.ParseRelationshipResponse(response)
			if err != nil {
				t.Logf("Failed to parse relationship response: %v", err)
				t.Logf("Raw response:\n%s\n", response)
				return
			}

			// Count relationships by type
			relCount := make(map[string]int)
			for _, rel := range relationships {
				relCount[rel.Type]++
				t.Logf("  %s -> [%s] -> %s (%.2f)", rel.From, rel.Type, rel.To, rel.Confidence)
			}

			t.Logf("\nRelationship Summary:")
			t.Logf("  Total relationships: %d", len(relationships))

			allMet := true
			for relType, minCount := range tc.expectedRels {
				count := relCount[relType]
				if count >= minCount {
					t.Logf("  ✓ %s: %d (expected ≥%d)", relType, count, minCount)
				} else {
					t.Logf("  ✗ %s: %d (expected ≥%d)", relType, count, minCount)
					allMet = false
				}
			}

			if allMet {
				t.Logf("\n✓ PASS: All relationship types extracted")
			} else {
				t.Logf("\n✗ FAIL: Some relationships missing")
				t.Fail()
			}
		})
	}
}

// TestPromptResponseConsistency verifies that prompts produce consistent, parseable output
func TestPromptResponseConsistency(t *testing.T) {
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

	content := "Alice works at Google using Python. Bob uses Go at Microsoft. Charlie works on Memento project using Docker."
	entities := []types.Entity{
		{Name: "Alice", Type: "person"},
		{Name: "Google", Type: "organization"},
		{Name: "Python", Type: "tool"},
		{Name: "Bob", Type: "person"},
		{Name: "Go", Type: "tool"},
		{Name: "Microsoft", Type: "organization"},
		{Name: "Charlie", Type: "person"},
		{Name: "Memento", Type: "project"},
		{Name: "Docker", Type: "tool"},
	}

	t.Logf("Testing prompt consistency with model: %s\n", model)

	// Test entity extraction consistency
	t.Run("EntityExtractionConsistency", func(t *testing.T) {
		successCount := 0
		for i := 0; i < 3; i++ {
			prompt := llm.EntityExtractionPrompt(content)
			response, err := client.Complete(ctx, prompt)
			if err != nil {
				t.Logf("  Attempt %d: LLM error: %v", i+1, err)
				continue
			}

			_, err = llm.ParseEntityResponse(response)
			if err != nil {
				t.Logf("  Attempt %d: Parse error: %v", i+1, err)
				continue
			}

			successCount++
		}

		if successCount >= 2 {
			t.Logf("✓ Entity extraction: %d/3 attempts successful", successCount)
		} else {
			t.Logf("✗ Entity extraction: Only %d/3 attempts successful", successCount)
			t.Fail()
		}
	})

	// Test relationship extraction consistency
	t.Run("RelationshipExtractionConsistency", func(t *testing.T) {
		successCount := 0
		for i := 0; i < 3; i++ {
			prompt := llm.RelationshipExtractionPrompt(content, entities)
			response, err := client.Complete(ctx, prompt)
			if err != nil {
				t.Logf("  Attempt %d: LLM error: %v", i+1, err)
				continue
			}

			_, err = llm.ParseRelationshipResponse(response)
			if err != nil {
				t.Logf("  Attempt %d: Parse error: %v", i+1, err)
				continue
			}

			successCount++
		}

		if successCount >= 2 {
			t.Logf("✓ Relationship extraction: %d/3 attempts successful", successCount)
		} else {
			t.Logf("✗ Relationship extraction: Only %d/3 attempts successful", successCount)
			t.Fail()
		}
	})

	// Test summarization consistency
	t.Run("SummarizationConsistency", func(t *testing.T) {
		successCount := 0
		for i := 0; i < 3; i++ {
			prompt := llm.SummarizationPrompt(content)
			response, err := client.Complete(ctx, prompt)
			if err != nil {
				t.Logf("  Attempt %d: LLM error: %v", i+1, err)
				continue
			}

			summary, err := llm.ParseSummarizationResponse(response)
			if err != nil {
				t.Logf("  Attempt %d: Parse error: %v", i+1, err)
				continue
			}

			if summary.Summary != "" && len(summary.KeyPoints) > 0 {
				successCount++
				t.Logf("  Attempt %d: ✓", i+1)
			}
		}

		if successCount >= 2 {
			t.Logf("✓ Summarization: %d/3 attempts successful", successCount)
		} else {
			t.Logf("✗ Summarization: Only %d/3 attempts successful", successCount)
			t.Fail()
		}
	})
}
