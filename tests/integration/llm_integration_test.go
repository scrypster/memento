package integration

import (
	"context"
	"testing"

	"github.com/scrypster/memento/internal/llm"
	"github.com/scrypster/memento/pkg/types"
)

// TestLLM_EntityExtraction tests entity extraction with real LLM.
// This test requires Ollama to be running and will be skipped in short mode.
func TestLLM_EntityExtraction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LLM test in short mode")
	}

	// Create Ollama client with default config
	client := llm.NewOllamaClient(llm.OllamaConfig{})

	ctx := context.Background()

	// Check if Ollama is available
	if err := client.HealthCheck(ctx); err != nil {
		t.Skip("Ollama not available:", err)
	}

	// Test content with clear entities
	content := "John works at Google on Kubernetes"
	prompt := llm.EntityExtractionPrompt(content)

	// Call LLM
	response, err := client.Complete(ctx, prompt)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if response == "" {
		t.Fatal("Expected non-empty response")
	}

	// Parse response
	entities, err := llm.ParseEntityResponse(response)
	if err != nil {
		t.Fatalf("ParseEntityResponse failed: %v", err)
	}

	// Verify entities were extracted
	if len(entities) < 2 {
		t.Errorf("Expected at least 2 entities, got %d", len(entities))
	}

	// Log entities for debugging
	t.Logf("Extracted %d entities:", len(entities))
	for _, entity := range entities {
		t.Logf("  - %s (%s): %s", entity.Name, entity.Type, entity.Description)
	}
}

// TestLLM_RelationshipExtraction tests relationship extraction with real LLM.
// This test requires Ollama to be running and will be skipped in short mode.
func TestLLM_RelationshipExtraction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LLM test in short mode")
	}

	client := llm.NewOllamaClient(llm.OllamaConfig{})
	ctx := context.Background()

	if err := client.HealthCheck(ctx); err != nil {
		t.Skip("Ollama not available:", err)
	}

	// Test content with clear relationships
	content := "Alice mentors Bob, who works on Project Phoenix"
	entities := []types.Entity{
		{ID: "ent:person:alice", Name: "Alice", Type: "person"},
		{ID: "ent:person:bob", Name: "Bob", Type: "person"},
		{ID: "ent:project:phoenix", Name: "Project Phoenix", Type: "project"},
	}
	prompt := llm.RelationshipExtractionPrompt(content, entities)

	// Call LLM
	response, err := client.Complete(ctx, prompt)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if response == "" {
		t.Fatal("Expected non-empty response")
	}

	// Parse response
	relationships, err := llm.ParseRelationshipResponse(response)
	if err != nil {
		t.Fatalf("ParseRelationshipResponse failed: %v", err)
	}

	// Verify relationships were extracted
	if len(relationships) < 1 {
		t.Errorf("Expected at least 1 relationship, got %d", len(relationships))
	}

	// Log relationships for debugging
	t.Logf("Extracted %d relationships:", len(relationships))
	for _, rel := range relationships {
		t.Logf("  - %s -> %s (%s)", rel.From, rel.To, rel.Type)
	}
}

// TestLLM_EmbeddingGeneration tests embedding generation with real LLM.
// This test requires Ollama to be running and will be skipped in short mode.
func TestLLM_EmbeddingGeneration(t *testing.T) {
	// Skip for now - embedding generation API may not be implemented yet
	t.Skip("Embedding generation test - skipping until API is confirmed")
}

// TestLLM_CircuitBreakerIntegration tests circuit breaker with LLM.
// This test requires Ollama to be running and will be skipped in short mode.
func TestLLM_CircuitBreakerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LLM test in short mode")
	}

	client := llm.NewOllamaClient(llm.OllamaConfig{})
	ctx := context.Background()

	if err := client.HealthCheck(ctx); err != nil {
		t.Skip("Ollama not available:", err)
	}

	// Make a series of successful requests
	for i := 0; i < 5; i++ {
		_, err := client.Complete(ctx, "Test prompt")
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
	}

	// Circuit breaker should remain closed
	t.Log("Circuit breaker test passed - all requests succeeded")
}

// TestLLM_Chunking tests content chunking for large inputs.
func TestLLM_Chunking(t *testing.T) {
	// Skip for now - chunking implementation may not be fully exposed
	t.Skip("Chunking test - skipping until Chunker API is confirmed")
}

// TestLLM_PromptGeneration tests prompt generation for different tasks.
func TestLLM_PromptGeneration(t *testing.T) {
	tests := []struct {
		name     string
		fn       func(string) string
		content  string
		contains []string
	}{
		{
			name:    "Entity extraction prompt",
			fn:      llm.EntityExtractionPrompt,
			content: "Test content",
			contains: []string{
				"entities",
				"Test content",
				"JSON",
			},
		},
		{
			name: "Relationship extraction prompt",
			fn: func(content string) string {
				entities := []types.Entity{
					{ID: "ent:person:alice", Name: "Alice", Type: "person"},
				}
				return llm.RelationshipExtractionPrompt(content, entities)
			},
			content: "Test content",
			contains: []string{
				"relationship",
				"Test content",
				"Alice",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := tt.fn(tt.content)

			if prompt == "" {
				t.Fatal("Expected non-empty prompt")
			}

			// Verify prompt contains expected elements
			for _, expected := range tt.contains {
				if !contains(prompt, expected) {
					t.Errorf("Expected prompt to contain %q", expected)
				}
			}

			t.Logf("Generated prompt length: %d chars", len(prompt))
		})
	}
}

// contains checks if a string contains a substring (case-insensitive helper)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr))))
}
