package tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/scrypster/memento/pkg/types"
)

// TestMemoryBasicFields tests the basic structure and JSON serialization of Memory
func TestMemoryBasicFields(t *testing.T) {
	now := time.Now()
	memory := types.Memory{
		ID:        "mem:test:123",
		Content:   "Test memory content",
		Source:    "manual",
		Domain:    "test",
		Timestamp: now,
		CreatedAt: now,
		UpdatedAt: now,
		Tags:      []string{"test", "example"},
		Metadata: map[string]interface{}{
			"key": "value",
		},
	}

	// Test basic field access
	if memory.ID != "mem:test:123" {
		t.Errorf("Expected ID to be 'mem:test:123', got '%s'", memory.ID)
	}
	if memory.Content != "Test memory content" {
		t.Errorf("Expected Content to match, got '%s'", memory.Content)
	}
	if memory.Domain != "test" {
		t.Errorf("Expected Domain to be 'test', got '%s'", memory.Domain)
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(memory)
	if err != nil {
		t.Fatalf("Failed to marshal Memory to JSON: %v", err)
	}

	// Test JSON deserialization
	var unmarshaled types.Memory
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal Memory from JSON: %v", err)
	}

	if unmarshaled.ID != memory.ID {
		t.Errorf("JSON roundtrip failed for ID: expected '%s', got '%s'", memory.ID, unmarshaled.ID)
	}
	if unmarshaled.Content != memory.Content {
		t.Errorf("JSON roundtrip failed for Content")
	}
}

// TestMemoryEnrichmentFields tests v2.0 async enrichment fields
func TestMemoryEnrichmentFields(t *testing.T) {
	memory := types.Memory{
		ID:                    "mem:test:456",
		Content:               "Content to enrich",
		Status:                types.StatusPending,
		EnrichmentAttempts:    0,
		EnrichmentError:       "",
		EmbeddingModel:        "text-embedding-3-small",
		EmbeddingDimension:    1536,
	}

	// Test status enum
	if memory.Status != types.StatusPending {
		t.Errorf("Expected Status to be StatusPending, got '%s'", memory.Status)
	}

	// Test enrichment tracking
	if memory.EnrichmentAttempts != 0 {
		t.Errorf("Expected EnrichmentAttempts to be 0, got %d", memory.EnrichmentAttempts)
	}

	// Test embedding model tracking
	if memory.EmbeddingModel != "text-embedding-3-small" {
		t.Errorf("Expected EmbeddingModel, got '%s'", memory.EmbeddingModel)
	}
	if memory.EmbeddingDimension != 1536 {
		t.Errorf("Expected EmbeddingDimension to be 1536, got %d", memory.EmbeddingDimension)
	}

	// Test enrichment status transitions
	memory.Status = types.StatusEnriched
	if memory.Status != types.StatusEnriched {
		t.Errorf("Failed to update Status to StatusEnriched")
	}

	// Test error handling
	memory.Status = types.StatusFailed
	memory.EnrichmentError = "API rate limit exceeded"
	memory.EnrichmentAttempts = 3
	if memory.EnrichmentError != "API rate limit exceeded" {
		t.Errorf("Failed to set enrichment error message")
	}
}

// TestEntityBasicFields tests the Entity struct
func TestEntityBasicFields(t *testing.T) {
	now := time.Now()
	entity := types.Entity{
		ID:          "ent:person:john-doe",
		Name:        "John Doe",
		Type:        types.EntityTypePerson,
		Description: "Software engineer",
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata: map[string]interface{}{
			"email": "john@example.com",
		},
	}

	// Test basic fields
	if entity.ID != "ent:person:john-doe" {
		t.Errorf("Expected ID, got '%s'", entity.ID)
	}
	if entity.Name != "John Doe" {
		t.Errorf("Expected Name, got '%s'", entity.Name)
	}
	if entity.Type != types.EntityTypePerson {
		t.Errorf("Expected Type to be EntityTypePerson, got '%s'", entity.Type)
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(entity)
	if err != nil {
		t.Fatalf("Failed to marshal Entity to JSON: %v", err)
	}

	var unmarshaled types.Entity
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal Entity from JSON: %v", err)
	}

	if unmarshaled.Name != entity.Name {
		t.Errorf("JSON roundtrip failed for Name")
	}
}

// TestEntityTypes tests all 20+ entity types
func TestEntityTypes(t *testing.T) {
	entityTypes := []string{
		types.EntityTypePerson,
		types.EntityTypeOrganization,
		types.EntityTypeProject,
		types.EntityTypeLocation,
		types.EntityTypeEvent,
		types.EntityTypeDocument,
		types.EntityTypeConcept,
		types.EntityTypeTask,
		types.EntityTypeNote,
		types.EntityTypeFile,
		types.EntityTypeURL,
		types.EntityTypeEmail,
		types.EntityTypeMessage,
		types.EntityTypeRepository,
		types.EntityTypeCodeSnippet,
		types.EntityTypeAPI,
		types.EntityTypeDatabase,
		types.EntityTypeServer,
		types.EntityTypeTool,
		types.EntityTypeFramework,
		types.EntityTypeLanguage,
		types.EntityTypeLibrary,
	}

	// Verify we have at least 20 entity types
	if len(entityTypes) < 20 {
		t.Errorf("Expected at least 20 entity types, got %d", len(entityTypes))
	}

	// Test creating entities with different types
	for _, entityType := range entityTypes {
		entity := types.Entity{
			ID:   "ent:" + entityType + ":test",
			Name: "Test " + entityType,
			Type: entityType,
		}

		if entity.Type != entityType {
			t.Errorf("Failed to create entity with type '%s'", entityType)
		}
	}
}

// TestRelationshipBasicFields tests the Relationship struct
func TestRelationshipBasicFields(t *testing.T) {
	now := time.Now()
	rel := types.Relationship{
		ID:         "rel:123",
		FromID:     "ent:person:alice",
		ToID:       "ent:project:memento",
		Type:       "works_on",
		Strength:   0.9,
		CreatedAt:  now,
		UpdatedAt:  now,
		Metadata: map[string]interface{}{
			"role": "developer",
		},
	}

	// Test basic fields
	if rel.ID != "rel:123" {
		t.Errorf("Expected ID, got '%s'", rel.ID)
	}
	if rel.FromID != "ent:person:alice" {
		t.Errorf("Expected FromID, got '%s'", rel.FromID)
	}
	if rel.ToID != "ent:project:memento" {
		t.Errorf("Expected ToID, got '%s'", rel.ToID)
	}
	if rel.Type != "works_on" {
		t.Errorf("Expected Type, got '%s'", rel.Type)
	}
	if rel.Strength != 0.9 {
		t.Errorf("Expected Strength 0.9, got %f", rel.Strength)
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(rel)
	if err != nil {
		t.Fatalf("Failed to marshal Relationship to JSON: %v", err)
	}

	var unmarshaled types.Relationship
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal Relationship from JSON: %v", err)
	}

	if unmarshaled.Type != rel.Type {
		t.Errorf("JSON roundtrip failed for Type")
	}
}

// TestRelationshipBidirectional tests bidirectional relationship metadata
func TestRelationshipBidirectional(t *testing.T) {
	// Create a bidirectional relationship (e.g., friendship)
	rel := types.Relationship{
		ID:     "rel:friendship:1",
		FromID: "ent:person:alice",
		ToID:   "ent:person:bob",
		Type:   "friend_of",
		RelationshipMetadata: types.RelationshipMetadata{
			Bidirectional: true,
			Inverse:       "friend_of",
		},
	}

	// Test bidirectional flag
	if !rel.IsBidirectional() {
		t.Error("Expected relationship to be bidirectional")
	}

	// Test inverse relationship type
	inverse := rel.GetInverse()
	if inverse != "friend_of" {
		t.Errorf("Expected inverse to be 'friend_of', got '%s'", inverse)
	}

	// Test unidirectional relationship
	uniRel := types.Relationship{
		ID:     "rel:parent:1",
		FromID: "ent:person:alice",
		ToID:   "ent:person:charlie",
		Type:   "parent_of",
		RelationshipMetadata: types.RelationshipMetadata{
			Bidirectional: false,
			Inverse:       "child_of",
		},
	}

	if uniRel.IsBidirectional() {
		t.Error("Expected relationship to be unidirectional")
	}

	inverse = uniRel.GetInverse()
	if inverse != "child_of" {
		t.Errorf("Expected inverse to be 'child_of', got '%s'", inverse)
	}
}

// TestRelationshipBidirectionalWithMetadataMap tests the new v2.0 RelationshipMetadata map
func TestRelationshipBidirectionalWithMetadataMap(t *testing.T) {
	rel := types.Relationship{
		ID:     "rel:collab:1",
		FromID: "ent:person:alice",
		ToID:   "ent:person:bob",
		Type:   "collaborates_with",
		RelationshipMetadata: types.RelationshipMetadata{
			Bidirectional: true,
			Inverse:       "collaborates_with",
		},
		Metadata: map[string]interface{}{
			"project": "memento",
			"since":   "2024-01-01",
		},
	}

	// Test that both Metadata and RelationshipMetadata coexist
	if !rel.IsBidirectional() {
		t.Error("Expected bidirectional relationship")
	}

	if rel.Metadata["project"] != "memento" {
		t.Error("Expected Metadata to be preserved")
	}

	// Test JSON serialization preserves both
	jsonData, err := json.Marshal(rel)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var unmarshaled types.Relationship
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if !unmarshaled.IsBidirectional() {
		t.Error("JSON roundtrip lost bidirectional flag")
	}
	if unmarshaled.GetInverse() != "collaborates_with" {
		t.Error("JSON roundtrip lost inverse relationship")
	}
	if unmarshaled.Metadata["project"] != "memento" {
		t.Error("JSON roundtrip lost metadata")
	}
}

// TestEnrichmentStatusConstants tests that status constants are defined correctly
func TestEnrichmentStatusConstants(t *testing.T) {
	statuses := []types.MemoryStatus{
		types.StatusPending,
		types.StatusEnriched,
		types.StatusFailed,
	}

	for _, status := range statuses {
		if status == "" {
			t.Errorf("Status constant is empty")
		}
	}

	// Ensure they're distinct
	if types.StatusPending == types.StatusEnriched {
		t.Error("Status constants should be distinct")
	}
	if types.StatusEnriched == types.StatusFailed {
		t.Error("Status constants should be distinct")
	}
}

// TestMemoryWithEmbedding tests embedding vector handling
func TestMemoryWithEmbedding(t *testing.T) {
	embedding := make([]float32, 1536)
	for i := range embedding {
		embedding[i] = 0.1
	}

	memory := types.Memory{
		ID:                 "mem:test:embed",
		Content:            "Test content",
		Embedding:          embedding,
		EmbeddingModel:     "text-embedding-3-small",
		EmbeddingDimension: 1536,
		Status:             types.StatusEnriched,
	}

	// Test embedding dimension matches
	if len(memory.Embedding) != memory.EmbeddingDimension {
		t.Errorf("Embedding length %d doesn't match dimension %d",
			len(memory.Embedding), memory.EmbeddingDimension)
	}

	// Test that embedding serializes to JSON
	jsonData, err := json.Marshal(memory)
	if err != nil {
		t.Fatalf("Failed to marshal memory with embedding: %v", err)
	}

	var unmarshaled types.Memory
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal memory with embedding: %v", err)
	}

	if len(unmarshaled.Embedding) != 1536 {
		t.Errorf("Embedding not preserved in JSON roundtrip")
	}
}

// TestEntityAliases tests entity aliases
func TestEntityAliases(t *testing.T) {
	entity := types.Entity{
		ID:      "ent:person:john",
		Name:    "John Doe",
		Type:    types.EntityTypePerson,
		Aliases: []string{"Johnny", "JD", "John D."},
	}

	if len(entity.Aliases) != 3 {
		t.Errorf("Expected 3 aliases, got %d", len(entity.Aliases))
	}

	// Test JSON serialization of aliases
	jsonData, err := json.Marshal(entity)
	if err != nil {
		t.Fatalf("Failed to marshal entity with aliases: %v", err)
	}

	var unmarshaled types.Entity
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal entity with aliases: %v", err)
	}

	if len(unmarshaled.Aliases) != 3 {
		t.Errorf("Aliases not preserved in JSON roundtrip")
	}
}

// TestRelationshipTypes tests common relationship types
func TestRelationshipTypes(t *testing.T) {
	relationshipTypes := []struct {
		relType       string
		bidirectional bool
		inverse       string
	}{
		{"works_on", false, "has_worker"},
		{"friend_of", true, "friend_of"},
		{"parent_of", false, "child_of"},
		{"manages", false, "managed_by"},
		{"collaborates_with", true, "collaborates_with"},
		{"mentions", false, "mentioned_in"},
		{"depends_on", false, "dependency_of"},
	}

	for _, rt := range relationshipTypes {
		rel := types.Relationship{
			ID:     "rel:test:" + rt.relType,
			FromID: "ent:a",
			ToID:   "ent:b",
			Type:   rt.relType,
			RelationshipMetadata: types.RelationshipMetadata{
				Bidirectional: rt.bidirectional,
				Inverse:       rt.inverse,
			},
		}

		if rel.IsBidirectional() != rt.bidirectional {
			t.Errorf("Relationship type '%s' bidirectional mismatch", rt.relType)
		}

		if rel.GetInverse() != rt.inverse {
			t.Errorf("Relationship type '%s' inverse should be '%s', got '%s'",
				rt.relType, rt.inverse, rel.GetInverse())
		}
	}
}
