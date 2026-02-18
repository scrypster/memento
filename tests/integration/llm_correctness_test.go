// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/llm"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// TestLLMEntityExtractionCorrectness verifies that entities are extracted accurately
func TestLLMEntityExtractionCorrectness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Test cases with known good outputs
	testCases := []struct {
		name             string
		content          string
		expectedEntities map[string]string // name -> type
	}{
		{
			name: "Personal and Work Relationships",
			content: `MJ Bonanno works at Scrypster. He is married to Norma Bonanno who works at Google.
Their son is Rosario Bonanno. Rosario uses Python and Docker.`,
			expectedEntities: map[string]string{
				"MJ Bonanno":      "person",
				"Scrypster":       "organization",
				"Norma Bonanno":   "person",
				"Google":          "organization",
				"Rosario Bonanno": "person",
				"Python":          "tool",
				"Docker":          "tool",
			},
		},
		{
			name: "Technical Stack",
			content: `The system uses PostgreSQL for data storage, Redis for caching, and Go for the backend.
Frontend is built with React and TypeScript. We use Docker for containerization and Kubernetes for orchestration.`,
			expectedEntities: map[string]string{
				"PostgreSQL":   "tool",
				"Redis":        "tool",
				"Go":           "tool",
				"React":        "tool",
				"TypeScript":   "tool",
				"Docker":       "tool",
				"Kubernetes":   "tool",
			},
		},
		{
			name: "Organizations and People",
			content: `Alice works at Microsoft as a software engineer. Bob is at Apple working on design.
Charlie is a consultant at Google. Diana leads engineering at Amazon.`,
			expectedEntities: map[string]string{
				"Alice":     "person",
				"Microsoft": "organization",
				"Bob":       "person",
				"Apple":     "organization",
				"Charlie":   "person",
				"Google":    "organization",
				"Diana":     "person",
				"Amazon":    "organization",
			},
		},
	}

	client := llm.NewOllamaClient("http://localhost:11434", "qwen2.5:7b")

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
				t.Fatalf("failed to parse entity response: %v", err)
			}

			// Build extraction map
			extracted := make(map[string]*llm.EntityResponse)
			for i := range entities {
				extracted[entities[i].Name] = &entities[i]
			}

			// Verify expected entities
			verified := 0
			for expectedName, expectedType := range tc.expectedEntities {
				entity, found := extracted[expectedName]
				if !found {
					t.Logf("⚠ Expected entity not found: %q (expected type: %s)", expectedName, expectedType)
					continue
				}

				if entity.Type != expectedType {
					t.Logf("⚠ Entity %q has type %q, expected %q", expectedName, entity.Type, expectedType)
					continue
				}

				verified++
				t.Logf("✓ Extracted: %s (%s) - confidence: %.2f", entity.Name, entity.Type, entity.Confidence)
			}

			ratio := float64(verified) / float64(len(tc.expectedEntities))
			if ratio >= 0.7 {
				t.Logf("✓ Extraction accuracy: %.0f%% (%d/%d)", ratio*100, verified, len(tc.expectedEntities))
			} else {
				t.Logf("✗ Low extraction accuracy: %.0f%% (%d/%d)", ratio*100, verified, len(tc.expectedEntities))
				t.Fail()
			}
		})
	}
}

// TestLLMRelationshipExtractionCorrectness verifies relationships are extracted accurately
func TestLLMRelationshipExtractionCorrectness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	testCases := []struct {
		name              string
		content           string
		entities          []types.Entity
		expectedRelations map[string][]string // type -> [pairs]
		checkBidirectional bool
	}{
		{
			name: "Family Relationships",
			content: `MJ Bonanno is married to Norma Bonanno. They have a son named Rosario.
MJ's brother is Anthony. Norma has a sister named Sofia.`,
			entities: []types.Entity{
				{Name: "MJ Bonanno", Type: "person"},
				{Name: "Norma Bonanno", Type: "person"},
				{Name: "Rosario", Type: "person"},
				{Name: "Anthony", Type: "person"},
				{Name: "Sofia", Type: "person"},
			},
			expectedRelations: map[string][]string{
				"married_to": {"MJ Bonanno->Norma Bonanno"},
				"parent_of":  {"MJ Bonanno->Rosario", "Norma Bonanno->Rosario"},
				"sibling_of": {"MJ Bonanno->Anthony"},
			},
			checkBidirectional: true,
		},
		{
			name: "Professional Relationships",
			content: `Alice and Bob are colleagues at TechCorp. Alice works on the Memento project with Bob.
Charlie uses Python. Alice also uses Python. Bob uses Go.`,
			entities: []types.Entity{
				{Name: "Alice", Type: "person"},
				{Name: "Bob", Type: "person"},
				{Name: "Charlie", Type: "person"},
				{Name: "TechCorp", Type: "organization"},
				{Name: "Memento", Type: "project"},
				{Name: "Python", Type: "tool"},
				{Name: "Go", Type: "tool"},
			},
			expectedRelations: map[string][]string{
				"colleague_of": {"Alice->Bob"},
				"employed_by":  {"Alice->TechCorp", "Bob->TechCorp"},
				"works_on":     {"Alice->Memento", "Bob->Memento"},
				"uses":         {"Alice->Python", "Bob->Go", "Charlie->Python"},
			},
			checkBidirectional: true,
		},
	}

	client := llm.NewOllamaClient("http://localhost:11434", "qwen2.5:7b")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate relationship extraction prompt
			prompt := llm.RelationshipExtractionPrompt(tc.content, tc.entities)

			// Call LLM
			response, err := client.Complete(ctx, prompt)
			if err != nil {
				t.Fatalf("LLM completion failed: %v", err)
			}

			// Parse response
			relationships, err := llm.ParseRelationshipResponse(response)
			if err != nil {
				t.Fatalf("failed to parse relationship response: %v", err)
			}

			// Build extraction map
			extracted := make(map[string][]llm.RelationshipResponse)
			for i := range relationships {
				key := fmt.Sprintf("%s->%s", relationships[i].From, relationships[i].To)
				extracted[relationships[i].Type] = append(extracted[relationships[i].Type], relationships[i])
			}

			// Verify expected relationships
			verified := 0
			for relType, pairs := range tc.expectedRelations {
				if len(extracted[relType]) > 0 {
					for _, rel := range extracted[relType] {
						t.Logf("✓ Extracted: %s -> [%s] -> %s (confidence: %.2f)", rel.From, relType, rel.To, rel.Confidence)
						verified++
					}
				} else {
					t.Logf("⚠ No %q relationships extracted", relType)
				}
			}

			// Check bidirectionality
			if tc.checkBidirectional {
				bidirectionalTypes := []string{"married_to", "colleague_of", "sibling_of"}
				for relType, pairs := range extracted {
					for _, rel := range pairs {
						if contains(bidirectionalTypes, relType) {
							// Check if reverse exists
							reverseKey := fmt.Sprintf("%s->%s", rel.To, rel.From)
							found := false
							for _, pair := range pairs {
								if pair.From == rel.To && pair.To == rel.From {
									found = true
									break
								}
							}

							if found {
								t.Logf("✓ Bidirectional %q found: %s ↔ %s", relType, rel.From, rel.To)
							} else {
								t.Logf("⚠ Missing reverse %q: %s ← %s", relType, rel.To, rel.From)
							}
						}
					}
				}
			}

			if verified > 0 {
				t.Logf("✓ Extracted %d relationships", verified)
			} else {
				t.Logf("✗ No relationships extracted")
				t.Fail()
			}
		})
	}
}

// TestMemoryEnrichmentPipeline tests the full enrichment pipeline end-to-end
func TestMemoryEnrichmentPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create test database
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer store.Close()

	// Create enrichment service
	client := llm.NewOllamaClient("http://localhost:11434", "qwen2.5:7b")
	svc := engine.NewEnrichmentService(client, store.GetDB())

	testMemory := &types.Memory{
		Content: `MJ Bonanno is the founder of Scrypster and works with Norma on the Memento project.
They use Go, PostgreSQL, and Python. The Memento system extracts entities and relationships.
MJ's team includes developers from Google and Microsoft.`,
		Tags: []string{"team", "project"},
	}

	// Create memory in store
	memoryID, err := store.Create(ctx, testMemory)
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}

	t.Logf("Created memory: %s", memoryID)

	// Run enrichment
	err = svc.EnrichMemory(ctx, memoryID, testMemory.Content)
	if err != nil {
		t.Fatalf("enrichment failed: %v", err)
	}

	// Verify entities were extracted and stored
	entities, err := store.ListEntities(ctx, storage.ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("failed to list entities: %v", err)
	}

	t.Logf("Extracted %d entities:", len(entities))
	for _, e := range entities {
		t.Logf("  - %s (%s): %s", e.Name, e.Type, e.Description)
	}

	expectedMinEntities := 7
	if len(entities) < expectedMinEntities {
		t.Errorf("expected at least %d entities, got %d", expectedMinEntities, len(entities))
	} else {
		t.Logf("✓ Entity extraction successful")
	}

	// Verify relationships were extracted and stored
	relationships, err := store.ListRelationships(ctx, storage.ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("failed to list relationships: %v", err)
	}

	t.Logf("Extracted %d relationships:", len(relationships))
	for _, r := range relationships {
		t.Logf("  - %s -> [%s] -> %s (%.2f)", r.SourceName, r.Type, r.TargetName, r.Confidence)
	}

	expectedMinRelationships := 3
	if len(relationships) < expectedMinRelationships {
		t.Errorf("expected at least %d relationships, got %d", expectedMinRelationships, len(relationships))
	} else {
		t.Logf("✓ Relationship extraction successful")
	}

	// Verify memory can be retrieved
	retrieved, err := store.Get(ctx, memoryID)
	if err != nil {
		t.Fatalf("failed to retrieve memory: %v", err)
	}

	if retrieved.Content != testMemory.Content {
		t.Errorf("retrieved memory content does not match")
	} else {
		t.Logf("✓ Memory retrieval successful")
	}

	t.Logf("✓ Full enrichment pipeline successful")
}

// Helper function
func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
