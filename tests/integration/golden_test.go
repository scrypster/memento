package integration

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/scrypster/memento/internal/llm"
	"github.com/scrypster/memento/pkg/types"
)

var update = flag.Bool("update", false, "update golden files")

// TestGolden_EntityExtractionPrompt tests entity extraction prompt generation.
// Run with -update flag to regenerate golden files.
func TestGolden_EntityExtractionPrompt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // path to golden file
	}{
		{
			name:     "simple_person_org",
			input:    "John Doe works at Acme Corp",
			expected: "testdata/golden/entity_prompt_01.txt",
		},
		{
			name:     "multiple_entities",
			input:    "Alice and Bob collaborate on Project Phoenix using Go and Python",
			expected: "testdata/golden/entity_prompt_02.txt",
		},
		{
			name:     "complex_technical",
			input:    "The Kubernetes API server uses etcd for distributed consensus and stores configuration in ConfigMaps",
			expected: "testdata/golden/entity_prompt_03.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := llm.EntityExtractionPrompt(tt.input)

			goldenPath := filepath.Join("..", "..", "tests", "integration", tt.expected)

			// Update golden file if -update flag is set
			if *update {
				dir := filepath.Dir(goldenPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("Failed to create golden dir: %v", err)
				}
				if err := os.WriteFile(goldenPath, []byte(prompt), 0644); err != nil {
					t.Fatalf("Failed to update golden file: %v", err)
				}
				t.Logf("Updated golden file: %s", goldenPath)
				return
			}

			// Compare with golden file
			golden, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("Failed to read golden file: %v (run with -update to create)", err)
			}

			if prompt != string(golden) {
				t.Errorf("Prompt mismatch.\n\nGot:\n%s\n\nExpected:\n%s", prompt, golden)
			}
		})
	}
}

// TestGolden_RelationshipExtractionPrompt tests relationship extraction prompt generation.
func TestGolden_RelationshipExtractionPrompt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		entities []types.Entity
		expected string
	}{
		{
			name:  "simple_relationship",
			input: "Alice mentors Bob",
			entities: []types.Entity{
				{ID: "ent:person:alice", Name: "Alice", Type: "person"},
				{ID: "ent:person:bob", Name: "Bob", Type: "person"},
			},
			expected: "testdata/golden/relationship_prompt_01.txt",
		},
		{
			name:  "project_relationship",
			input: "The authentication service depends on the user database",
			entities: []types.Entity{
				{ID: "ent:api:authentication-service", Name: "Authentication Service", Type: "api"},
				{ID: "ent:database:user-database", Name: "User Database", Type: "database"},
			},
			expected: "testdata/golden/relationship_prompt_02.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := llm.RelationshipExtractionPrompt(tt.input, tt.entities)

			goldenPath := filepath.Join("..", "..", "tests", "integration", tt.expected)

			// Update golden file if -update flag is set
			if *update {
				dir := filepath.Dir(goldenPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("Failed to create golden dir: %v", err)
				}
				if err := os.WriteFile(goldenPath, []byte(prompt), 0644); err != nil {
					t.Fatalf("Failed to update golden file: %v", err)
				}
				t.Logf("Updated golden file: %s", goldenPath)
				return
			}

			// Compare with golden file
			golden, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("Failed to read golden file: %v (run with -update to create)", err)
			}

			if prompt != string(golden) {
				t.Errorf("Prompt mismatch.\n\nGot:\n%s\n\nExpected:\n%s", prompt, golden)
			}
		})
	}
}

// TestGolden_EntityResponseParsing tests parsing of entity extraction responses.
func TestGolden_EntityResponseParsing(t *testing.T) {
	tests := []struct {
		name            string
		responseFile    string
		expectedCount   int
		expectedTypes   []string
	}{
		{
			name:          "valid_person_org_response",
			responseFile:  "testdata/golden/entity_response_01.json",
			expectedCount: 2,
			expectedTypes: []string{"person", "organization"},
		},
		{
			name:          "technical_entities_response",
			responseFile:  "testdata/golden/entity_response_02.json",
			expectedCount: 3,
			expectedTypes: []string{"api", "database", "tool"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responsePath := filepath.Join("..", "..", "tests", "integration", tt.responseFile)

			// Read response file
			responseData, err := os.ReadFile(responsePath)
			if err != nil {
				t.Skipf("Golden file not found: %v (run with -update to create)", err)
			}

			// Parse response
			entities, err := llm.ParseEntityResponse(string(responseData))
			if err != nil {
				t.Fatalf("ParseEntityResponse failed: %v", err)
			}

			// Verify entity count
			if len(entities) != tt.expectedCount {
				t.Errorf("Expected %d entities, got %d", tt.expectedCount, len(entities))
			}

			// Verify entity types
			foundTypes := make(map[string]bool)
			for _, entity := range entities {
				foundTypes[entity.Type] = true
			}

			for _, expectedType := range tt.expectedTypes {
				if !foundTypes[expectedType] {
					t.Errorf("Expected entity type %q not found", expectedType)
				}
			}
		})
	}
}

// TestGolden_RelationshipResponseParsing tests parsing of relationship extraction responses.
func TestGolden_RelationshipResponseParsing(t *testing.T) {
	tests := []struct {
		name          string
		responseFile  string
		expectedCount int
		expectedTypes []string
	}{
		{
			name:          "mentorship_response",
			responseFile:  "testdata/golden/relationship_response_01.json",
			expectedCount: 1,
			expectedTypes: []string{"knows"},
		},
		{
			name:          "project_dependencies_response",
			responseFile:  "testdata/golden/relationship_response_02.json",
			expectedCount: 2,
			expectedTypes: []string{"depends_on", "uses"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responsePath := filepath.Join("..", "..", "tests", "integration", tt.responseFile)

			// Read response file
			responseData, err := os.ReadFile(responsePath)
			if err != nil {
				t.Skipf("Golden file not found: %v (run with -update to create)", err)
			}

			// Parse response
			relationships, err := llm.ParseRelationshipResponse(string(responseData))
			if err != nil {
				t.Fatalf("ParseRelationshipResponse failed: %v", err)
			}

			// Verify relationship count
			if len(relationships) != tt.expectedCount {
				t.Errorf("Expected %d relationships, got %d", tt.expectedCount, len(relationships))
			}

			// Verify relationship types
			foundTypes := make(map[string]bool)
			for _, rel := range relationships {
				foundTypes[rel.Type] = true
			}

			for _, expectedType := range tt.expectedTypes {
				if !foundTypes[expectedType] {
					t.Errorf("Expected relationship type %q not found", expectedType)
				}
			}
		})
	}
}
