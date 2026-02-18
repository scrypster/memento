package tests

import (
	"strings"
	"testing"

	"github.com/scrypster/memento/internal/llm"
	"github.com/scrypster/memento/pkg/types"
)

// TestEntityExtractionPrompt verifies the entity extraction prompt format
func TestEntityExtractionPrompt(t *testing.T) {
	content := "John Doe works at Acme Corp on the Memento project."
	prompt := llm.EntityExtractionPrompt(content)

	// Verify prompt is non-empty
	if prompt == "" {
		t.Fatal("EntityExtractionPrompt returned empty string")
	}

	// Verify JSON output instruction (exact wording may vary)
	if !strings.Contains(prompt, "JSON") {
		t.Error("Prompt missing JSON output instruction")
	}

	// Verify core entity types are listed (the prompt may use a curated subset
	// for better LLM extraction quality rather than all 22+ types).
	coreEntityTypes := []string{
		"person", "organization", "project", "tool",
	}

	for _, entityType := range coreEntityTypes {
		if !strings.Contains(prompt, entityType) {
			t.Errorf("Prompt missing core entity type: %s", entityType)
		}
	}

	// Verify confidence scoring is mentioned
	if !strings.Contains(prompt, "confidence") {
		t.Error("Prompt missing confidence scoring instruction")
	}

	// Verify content is included
	if !strings.Contains(prompt, content) {
		t.Error("Prompt missing input content")
	}

	// Verify example JSON structure is shown
	if !strings.Contains(prompt, "entities") {
		t.Error("Prompt missing example JSON structure")
	}
}

// TestRelationshipExtractionPrompt verifies the relationship extraction prompt format
func TestRelationshipExtractionPrompt(t *testing.T) {
	content := "John Doe works at Acme Corp on the Memento project."
	entities := []types.Entity{
		{Name: "John Doe", Type: types.EntityTypePerson},
		{Name: "Acme Corp", Type: types.EntityTypeOrganization},
		{Name: "Memento", Type: types.EntityTypeProject},
	}

	prompt := llm.RelationshipExtractionPrompt(content, entities)

	// Verify prompt is non-empty
	if prompt == "" {
		t.Fatal("RelationshipExtractionPrompt returned empty string")
	}

	// Verify strict JSON-only instruction
	if !strings.Contains(prompt, "Return ONLY valid JSON") {
		t.Error("Prompt missing strict JSON-only instruction")
	}

	// Verify core relationship types are listed (the prompt uses a curated
	// subset of the most common relationship types for better LLM output).
	coreRelTypes := []string{
		"married_to", "parent_of", "child_of", "colleague_of",
		"works_on", "uses", "depends_on", "contains", "belongs_to",
	}

	for _, relType := range coreRelTypes {
		if !strings.Contains(prompt, relType) {
			t.Errorf("Prompt missing core relationship type: %s", relType)
		}
	}

	// Verify entities are included
	for _, entity := range entities {
		if !strings.Contains(prompt, entity.Name) {
			t.Errorf("Prompt missing entity: %s", entity.Name)
		}
	}

	// Verify confidence scoring
	if !strings.Contains(prompt, "confidence") {
		t.Error("Prompt missing confidence scoring instruction")
	}

	// Verify example JSON structure
	if !strings.Contains(prompt, "relationships") {
		t.Error("Prompt missing example JSON structure")
	}
}

// TestSummarizationPrompt verifies the summarization prompt format
func TestSummarizationPrompt(t *testing.T) {
	content := "This is a long piece of content that needs summarization."
	prompt := llm.SummarizationPrompt(content)

	// Verify prompt is non-empty
	if prompt == "" {
		t.Fatal("SummarizationPrompt returned empty string")
	}

	// Verify strict JSON-only instruction
	if !strings.Contains(prompt, "Return ONLY valid JSON") {
		t.Error("Prompt missing strict JSON-only instruction")
	}

	// Verify content is included
	if !strings.Contains(prompt, content) {
		t.Error("Prompt missing input content")
	}

	// Verify clear summarization instructions
	if !strings.Contains(prompt, "summary") && !strings.Contains(prompt, "summarize") {
		t.Error("Prompt missing summarization instruction")
	}
}

// TestKeywordExtractionPrompt verifies the keyword extraction prompt format
func TestKeywordExtractionPrompt(t *testing.T) {
	content := "Memento is an AI-native memory system built in Go."
	prompt := llm.KeywordExtractionPrompt(content)

	// Verify prompt is non-empty
	if prompt == "" {
		t.Fatal("KeywordExtractionPrompt returned empty string")
	}

	// Verify strict JSON-only instruction
	if !strings.Contains(prompt, "Return ONLY valid JSON") {
		t.Error("Prompt missing strict JSON-only instruction")
	}

	// Verify content is included
	if !strings.Contains(prompt, content) {
		t.Error("Prompt missing input content")
	}

	// Verify keyword extraction instruction
	if !strings.Contains(prompt, "keyword") {
		t.Error("Prompt missing keyword extraction instruction")
	}
}

// TestParseEntityResponse tests parsing valid entity JSON
func TestParseEntityResponse(t *testing.T) {
	jsonStr := `{
		"entities": [
			{
				"name": "John Doe",
				"type": "person",
				"description": "Software engineer",
				"confidence": 0.95
			},
			{
				"name": "Memento",
				"type": "project",
				"description": "AI-native memory system",
				"confidence": 0.98
			}
		]
	}`

	entities, err := llm.ParseEntityResponse(jsonStr)
	if err != nil {
		t.Fatalf("ParseEntityResponse failed: %v", err)
	}

	if len(entities) != 2 {
		t.Fatalf("Expected 2 entities, got %d", len(entities))
	}

	// Verify first entity
	if entities[0].Name != "John Doe" {
		t.Errorf("Expected name 'John Doe', got '%s'", entities[0].Name)
	}
	if entities[0].Type != "person" {
		t.Errorf("Expected type 'person', got '%s'", entities[0].Type)
	}
	if entities[0].Confidence != 0.95 {
		t.Errorf("Expected confidence 0.95, got %f", entities[0].Confidence)
	}

	// Verify second entity
	if entities[1].Name != "Memento" {
		t.Errorf("Expected name 'Memento', got '%s'", entities[1].Name)
	}
	if entities[1].Type != "project" {
		t.Errorf("Expected type 'project', got '%s'", entities[1].Type)
	}
}

// TestParseEntityResponse_InvalidType tests that unknown entity types are skipped (not fatal).
// ParseEntityResponse silently drops entities with unrecognised types so that one
// bad LLM output doesn't abort enrichment for the whole memory.
func TestParseEntityResponse_InvalidType(t *testing.T) {
	jsonStr := `{
		"entities": [
			{
				"name": "Invalid Entity",
				"type": "invalid_type",
				"description": "This should be skipped",
				"confidence": 0.95
			}
		]
	}`

	entities, err := llm.ParseEntityResponse(jsonStr)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("Expected entity with unknown type to be skipped, got %d entities", len(entities))
	}
}

// TestParseEntityResponse_InvalidJSON tests handling of malformed JSON
func TestParseEntityResponse_InvalidJSON(t *testing.T) {
	jsonStr := `{invalid json}`

	_, err := llm.ParseEntityResponse(jsonStr)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

// TestParseRelationshipResponse tests parsing valid relationship JSON
func TestParseRelationshipResponse(t *testing.T) {
	jsonStr := `{
		"relationships": [
			{
				"from": "John Doe",
				"to": "Memento",
				"type": "works_on",
				"confidence": 0.90
			},
			{
				"from": "John Doe",
				"to": "Jane Smith",
				"type": "colleague_of",
				"confidence": 0.85
			}
		]
	}`

	relationships, err := llm.ParseRelationshipResponse(jsonStr)
	if err != nil {
		t.Fatalf("ParseRelationshipResponse failed: %v", err)
	}

	if len(relationships) != 2 {
		t.Fatalf("Expected 2 relationships, got %d", len(relationships))
	}

	// Verify first relationship
	if relationships[0].From != "John Doe" {
		t.Errorf("Expected from 'John Doe', got '%s'", relationships[0].From)
	}
	if relationships[0].To != "Memento" {
		t.Errorf("Expected to 'Memento', got '%s'", relationships[0].To)
	}
	if relationships[0].Type != "works_on" {
		t.Errorf("Expected type 'works_on', got '%s'", relationships[0].Type)
	}
	if relationships[0].Confidence != 0.90 {
		t.Errorf("Expected confidence 0.90, got %f", relationships[0].Confidence)
	}
}

// TestParseRelationshipResponse_InvalidType tests that unknown relationship types are skipped.
func TestParseRelationshipResponse_InvalidType(t *testing.T) {
	jsonStr := `{
		"relationships": [
			{
				"from": "Entity A",
				"to": "Entity B",
				"type": "invalid_relationship",
				"confidence": 0.90
			}
		]
	}`

	rels, err := llm.ParseRelationshipResponse(jsonStr)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(rels) != 0 {
		t.Errorf("Expected relationship with unknown type to be skipped, got %d relationships", len(rels))
	}
}

// TestValidateEntityTypes ensures only valid entity types pass validation
func TestValidateEntityTypes(t *testing.T) {
	// Valid entities
	validEntities := []llm.EntityResponse{
		{Name: "John", Type: "person", Confidence: 0.9},
		{Name: "Acme", Type: "organization", Confidence: 0.95},
		{Name: "Go", Type: "language", Confidence: 0.98},
	}

	err := llm.ValidateEntityTypes(validEntities)
	if err != nil {
		t.Errorf("ValidateEntityTypes failed for valid entities: %v", err)
	}

	// Invalid entity type
	invalidEntities := []llm.EntityResponse{
		{Name: "Valid", Type: "person", Confidence: 0.9},
		{Name: "Invalid", Type: "not_a_real_type", Confidence: 0.9},
	}

	err = llm.ValidateEntityTypes(invalidEntities)
	if err == nil {
		t.Error("Expected error for invalid entity type, got nil")
	}
}

// TestValidateRelationshipTypes ensures ValidateRelationshipTypes filters to only valid types.
// ValidateRelationshipTypes is a filter function (returns []RelationshipResponse), not an
// error-returning validator.
func TestValidateRelationshipTypes(t *testing.T) {
	// Valid relationships — all should pass through
	validRels := []llm.RelationshipResponse{
		{From: "A", To: "B", Type: "works_on", Confidence: 0.9},
		{From: "C", To: "D", Type: "colleague_of", Confidence: 0.85},
	}

	result := llm.ValidateRelationshipTypes(validRels)
	if len(result) != len(validRels) {
		t.Errorf("ValidateRelationshipTypes filtered out valid relationships: got %d, want %d", len(result), len(validRels))
	}

	// Mix of valid and invalid — only the valid one should survive
	mixedRels := []llm.RelationshipResponse{
		{From: "A", To: "B", Type: "works_on", Confidence: 0.9},
		{From: "C", To: "D", Type: "invalid_rel", Confidence: 0.9},
	}

	result = llm.ValidateRelationshipTypes(mixedRels)
	if len(result) != 1 {
		t.Errorf("Expected 1 valid relationship after filtering, got %d", len(result))
	}
	if len(result) > 0 && result[0].Type != "works_on" {
		t.Errorf("Expected surviving relationship to be 'works_on', got '%s'", result[0].Type)
	}
}

// TestParseSummarizationResponse tests parsing summarization JSON
func TestParseSummarizationResponse(t *testing.T) {
	jsonStr := `{
		"summary": "This is a concise summary of the content.",
		"key_points": [
			"Point 1",
			"Point 2",
			"Point 3"
		]
	}`

	result, err := llm.ParseSummarizationResponse(jsonStr)
	if err != nil {
		t.Fatalf("ParseSummarizationResponse failed: %v", err)
	}

	if result.Summary != "This is a concise summary of the content." {
		t.Errorf("Unexpected summary: %s", result.Summary)
	}

	if len(result.KeyPoints) != 3 {
		t.Errorf("Expected 3 key points, got %d", len(result.KeyPoints))
	}
}

// TestParseKeywordResponse tests parsing keyword extraction JSON
func TestParseKeywordResponse(t *testing.T) {
	jsonStr := `{
		"keywords": ["memento", "AI", "memory", "system", "Go"]
	}`

	keywords, err := llm.ParseKeywordResponse(jsonStr)
	if err != nil {
		t.Fatalf("ParseKeywordResponse failed: %v", err)
	}

	if len(keywords) != 5 {
		t.Fatalf("Expected 5 keywords, got %d", len(keywords))
	}

	expectedKeywords := map[string]bool{
		"memento": true,
		"AI":      true,
		"memory":  true,
		"system":  true,
		"Go":      true,
	}

	for _, kw := range keywords {
		if !expectedKeywords[kw] {
			t.Errorf("Unexpected keyword: %s", kw)
		}
	}
}

// TestEntityExtractionPrompt_EmptyContent tests handling of empty content
func TestEntityExtractionPrompt_EmptyContent(t *testing.T) {
	prompt := llm.EntityExtractionPrompt("")
	if prompt == "" {
		t.Error("EntityExtractionPrompt should return prompt even for empty content")
	}
}

// TestRelationshipExtractionPrompt_NoEntities tests handling when no entities provided
func TestRelationshipExtractionPrompt_NoEntities(t *testing.T) {
	content := "Some content"
	entities := []types.Entity{}

	prompt := llm.RelationshipExtractionPrompt(content, entities)
	if prompt == "" {
		t.Error("RelationshipExtractionPrompt should return prompt even with no entities")
	}
}

// TestConfidenceScoreValidation tests that confidence scores outside [0.0, 1.0] are skipped.
// ParseEntityResponse no longer returns an error for bad confidence — it silently drops
// the offending entity so that one bad extraction doesn't kill the whole batch.
func TestConfidenceScoreValidation(t *testing.T) {
	tests := []struct {
		name          string
		jsonStr       string
		wantCount     int // expected number of valid entities returned
	}{
		{
			name: "valid confidence",
			jsonStr: `{
				"entities": [{"name": "Test", "type": "person", "confidence": 0.5}]
			}`,
			wantCount: 1,
		},
		{
			name: "confidence too high — entity skipped",
			jsonStr: `{
				"entities": [{"name": "Test", "type": "person", "confidence": 1.5}]
			}`,
			wantCount: 0,
		},
		{
			name: "negative confidence — entity skipped",
			jsonStr: `{
				"entities": [{"name": "Test", "type": "person", "confidence": -0.1}]
			}`,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := llm.ParseEntityResponse(tt.jsonStr)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if len(entities) != tt.wantCount {
				t.Errorf("got %d entities, want %d", len(entities), tt.wantCount)
			}
		})
	}
}

// TestJSONOutputStructure tests that example JSON in prompts is valid
func TestJSONOutputStructure(t *testing.T) {
	// Extract example JSON from entity extraction prompt
	prompt := llm.EntityExtractionPrompt("test content")

	// Find JSON example in prompt (between ```json and ```)
	// This is a heuristic test - the prompt should contain valid JSON examples
	if !strings.Contains(prompt, "{") || !strings.Contains(prompt, "}") {
		t.Error("Prompt should contain JSON structure examples")
	}
}
