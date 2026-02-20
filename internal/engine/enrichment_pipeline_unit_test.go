package engine

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/scrypster/memento/pkg/types"
)

// mockLLMClient is a mock implementation of llm.TextGenerator for testing
type mockLLMClient struct {
	responses []string // responses to return in order
	errors    []error  // errors to return in order (nil for success)
	callCount int
	model     string
}

func newMockLLMClient() *mockLLMClient {
	return &mockLLMClient{
		responses: []string{},
		errors:    []error{},
		callCount: 0,
		model:     "mock-model",
	}
}

func (m *mockLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
	defer func() { m.callCount++ }()

	if m.callCount < len(m.errors) && m.errors[m.callCount] != nil {
		return "", m.errors[m.callCount]
	}

	if m.callCount < len(m.responses) {
		return m.responses[m.callCount], nil
	}

	// Fallback if we run out of responses
	return "", errors.New("mock LLM: no more responses configured")
}

func (m *mockLLMClient) GetModel() string {
	return m.model
}

// setupTestDB creates an in-memory SQLite database with the minimal schema needed for testing
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create in-memory database: %v", err)
	}

	schema := `
	CREATE TABLE memories (
		id TEXT PRIMARY KEY,
		content TEXT,
		memory_type TEXT,
		category TEXT,
		classification TEXT,
		priority TEXT,
		context_labels TEXT,
		tags TEXT,
		summary TEXT,
		key_points TEXT,
		entity_status TEXT,
		relationship_status TEXT,
		classification_status TEXT,
		summarization_status TEXT,
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	);
	CREATE TABLE entities (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		description TEXT,
		created_at TIMESTAMP,
		updated_at TIMESTAMP,
		UNIQUE(name, type)
	);
	CREATE TABLE relationships (
		id TEXT PRIMARY KEY,
		source_id TEXT NOT NULL,
		target_id TEXT NOT NULL,
		type TEXT NOT NULL,
		weight REAL,
		created_at TIMESTAMP,
		updated_at TIMESTAMP,
		UNIQUE(source_id, target_id, type)
	);
	CREATE TABLE memory_entities (
		memory_id TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		frequency INTEGER DEFAULT 1,
		confidence REAL,
		created_at TIMESTAMP,
		PRIMARY KEY(memory_id, entity_id)
	);
	CREATE TABLE unknown_type_stats (
		domain TEXT NOT NULL,
		type_name TEXT NOT NULL,
		count INTEGER DEFAULT 1,
		first_seen TIMESTAMP,
		last_seen TIMESTAMP,
		PRIMARY KEY(domain, type_name)
	);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	return db
}

// insertTestMemory creates a test memory in the database
func insertTestMemory(t *testing.T, db *sql.DB, memoryID, content string) {
	now := time.Now()
	_, err := db.Exec(
		`INSERT INTO memories (id, content, entity_status, relationship_status,
			classification_status, summarization_status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		memoryID, content,
		types.EnrichmentPending, types.EnrichmentPending,
		types.EnrichmentPending, types.EnrichmentPending,
		now, now,
	)
	if err != nil {
		t.Fatalf("Failed to insert test memory: %v", err)
	}
}

// TestEnrichmentPipeline_HappyPath tests successful entity and relationship extraction
func TestEnrichmentPipeline_HappyPath(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	memoryID := "mem:test:001"
	content := "MJ works with Norma and they live in New York. MJ specializes in Go."

	insertTestMemory(t, db, memoryID, content)

	// Mock LLM responses
	mock := newMockLLMClient()
	mock.responses = []string{
		// Call 1: Entity extraction response
		`{"entities": [
			{"name": "MJ", "type": "person", "description": "Software engineer", "confidence": 0.95},
			{"name": "Norma", "type": "person", "description": "Colleague", "confidence": 0.90},
			{"name": "Go", "type": "language", "description": "Programming language", "confidence": 0.98}
		]}`,
		// Call 2: Relationship extraction response
		`{"relationships": [
			{"from": "MJ", "to": "Norma", "type": "works_with", "confidence": 0.92},
			{"from": "MJ", "to": "Go", "type": "uses", "confidence": 0.95}
		]}`,
		// Call 3: Classification response
		`{"memory_type": "decision", "category": "Technology", "classification": "Career",
		  "priority": "High", "context_labels": ["Technical", "Career"], "tags": ["golang", "work"], "confidence": 0.88}`,
		// Call 4: Summarization response
		`{"summary": "MJ works with Norma and specializes in Go programming.",
		  "key_points": ["Works with colleague Norma", "Specializes in Go", "Located in New York"]}`,
	}

	pipeline := NewExtractionPipeline(mock, db)
	result, err := pipeline.Extract(ctx, memoryID, content)

	// Check that pipeline completed without fatal error
	if err != nil {
		t.Fatalf("Pipeline should not return error on happy path: %v", err)
	}

	// Verify entity extraction succeeded
	if result.EntityStatus != types.EnrichmentCompleted {
		t.Errorf("Expected entity status %s, got %s", types.EnrichmentCompleted, result.EntityStatus)
	}
	if len(result.Entities) != 3 {
		t.Errorf("Expected 3 entities, got %d", len(result.Entities))
	}

	// Verify relationship extraction succeeded
	if result.RelationshipStatus != types.EnrichmentCompleted {
		t.Errorf("Expected relationship status %s, got %s", types.EnrichmentCompleted, result.RelationshipStatus)
	}
	if len(result.Relationships) != 2 {
		t.Errorf("Expected 2 relationships, got %d", len(result.Relationships))
	}

	// Verify classification extraction succeeded
	if result.ClassificationStatus != types.EnrichmentCompleted {
		t.Errorf("Expected classification status %s, got %s", types.EnrichmentCompleted, result.ClassificationStatus)
	}
	if result.Classification == nil {
		t.Fatal("Expected classification to be non-nil")
	}
	if result.Classification.MemoryType != "decision" {
		t.Errorf("Expected memory_type 'decision', got '%s'", result.Classification.MemoryType)
	}

	// Verify summarization succeeded
	if result.SummarizationStatus != types.EnrichmentCompleted {
		t.Errorf("Expected summarization status %s, got %s", types.EnrichmentCompleted, result.SummarizationStatus)
	}
	if result.Summary == nil {
		t.Fatal("Expected summary to be non-nil")
	}
	if len(result.Summary.KeyPoints) != 3 {
		t.Errorf("Expected 3 key points, got %d", len(result.Summary.KeyPoints))
	}

	// Verify entities were stored in database
	var entityCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entityCount)
	if err != nil {
		t.Fatalf("Failed to count entities: %v", err)
	}
	if entityCount != 3 {
		t.Errorf("Expected 3 entities in DB, got %d", entityCount)
	}

	// Verify relationships were stored in database
	var relCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM relationships`).Scan(&relCount)
	if err != nil {
		t.Fatalf("Failed to count relationships: %v", err)
	}
	if relCount != 2 {
		t.Errorf("Expected 2 relationships in DB, got %d", relCount)
	}

	// Verify memory-entity links were created
	var linkCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM memory_entities`).Scan(&linkCount)
	if err != nil {
		t.Fatalf("Failed to count memory_entities: %v", err)
	}
	if linkCount != 3 {
		t.Errorf("Expected 3 memory-entity links, got %d", linkCount)
	}
}

// TestEnrichmentPipeline_Call1Fails tests when entity extraction fails
func TestEnrichmentPipeline_Call1Fails(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	memoryID := "mem:test:002"
	content := "Test content for entity extraction"

	insertTestMemory(t, db, memoryID, content)

	// Mock LLM to fail on first call (entity extraction)
	mock := newMockLLMClient()
	mock.errors = []error{
		errors.New("LLM service unavailable"),
		nil, nil, nil, // These should not be reached
	}
	mock.responses = []string{
		"", // No response for failed call
		`{"relationships": []}`,
		`{}`,
		`{}`,
	}

	pipeline := NewExtractionPipeline(mock, db)
	result, err := pipeline.Extract(ctx, memoryID, content)

	// Pipeline should return error for Call 1 failure
	if err == nil {
		t.Fatal("Expected pipeline to return error when entity extraction fails")
	}

	// Entity extraction should have failed
	if result.EntityStatus != types.EnrichmentFailed {
		t.Errorf("Expected entity status %s, got %s", types.EnrichmentFailed, result.EntityStatus)
	}

	// Verify mock was only called once (no calls to 2, 3, 4)
	if mock.callCount != 1 {
		t.Errorf("Expected LLM to be called once (fail fast), but was called %d times", mock.callCount)
	}

	// Verify no entities were stored
	var entityCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entityCount)
	if err != nil {
		t.Fatalf("Failed to count entities: %v", err)
	}
	if entityCount != 0 {
		t.Errorf("Expected 0 entities in DB after failure, got %d", entityCount)
	}
}

// TestEnrichmentPipeline_Call2Fails tests when relationship extraction fails but entities succeed
func TestEnrichmentPipeline_Call2Fails(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	memoryID := "mem:test:003"
	content := "MJ works with Norma on Go projects."

	insertTestMemory(t, db, memoryID, content)

	// Mock LLM: success on Call 1, failure on Call 2
	mock := newMockLLMClient()
	mock.errors = []error{
		nil, // Call 1 succeeds
		errors.New("Relationship extraction timeout"),
		nil, // Call 3 succeeds
		nil, // Call 4 succeeds
	}
	mock.responses = []string{
		// Call 1: Success
		`{"entities": [
			{"name": "MJ", "type": "person", "confidence": 0.95},
			{"name": "Norma", "type": "person", "confidence": 0.90}
		]}`,
		// Call 2: Will fail
		"",
		// Call 3: Classification
		`{"memory_type": "process", "category": "Work", "classification": "Collaboration",
		  "priority": "Medium", "context_labels": [], "tags": [], "confidence": 0.80}`,
		// Call 4: Summary
		`{"summary": "MJ and Norma work together.", "key_points": ["Collaboration"]}`,
	}

	pipeline := NewExtractionPipeline(mock, db)
	result, err := pipeline.Extract(ctx, memoryID, content)

	// Pipeline should NOT return error (partial success is OK)
	if err != nil {
		t.Fatalf("Pipeline should handle Call 2 failure gracefully, got error: %v", err)
	}

	// Entity extraction should have succeeded
	if result.EntityStatus != types.EnrichmentCompleted {
		t.Errorf("Expected entity status %s, got %s", types.EnrichmentCompleted, result.EntityStatus)
	}

	// Relationship extraction should have failed
	if result.RelationshipStatus != types.EnrichmentFailed {
		t.Errorf("Expected relationship status %s, got %s", types.EnrichmentFailed, result.RelationshipStatus)
	}

	// Classification should have succeeded
	if result.ClassificationStatus != types.EnrichmentCompleted {
		t.Errorf("Expected classification status %s, got %s", types.EnrichmentCompleted, result.ClassificationStatus)
	}

	// Verify entities were stored despite Call 2 failure
	var entityCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entityCount)
	if err != nil {
		t.Fatalf("Failed to count entities: %v", err)
	}
	if entityCount != 2 {
		t.Errorf("Expected 2 entities in DB, got %d", entityCount)
	}

	// Verify no relationships were stored
	var relCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM relationships`).Scan(&relCount)
	if err != nil {
		t.Fatalf("Failed to count relationships: %v", err)
	}
	if relCount != 0 {
		t.Errorf("Expected 0 relationships in DB (Call 2 failed), got %d", relCount)
	}
}

// TestEnrichmentPipeline_EmptyEntityResponse tests when LLM returns no entities
func TestEnrichmentPipeline_EmptyEntityResponse(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	memoryID := "mem:test:004"
	content := "Some generic content with no identifiable entities"

	insertTestMemory(t, db, memoryID, content)

	// Mock LLM to return empty entities
	mock := newMockLLMClient()
	mock.responses = []string{
		// Call 1: No entities
		`{"entities": []}`,
		// Call 2: Skipped (no entities to relate)
		`{"relationships": []}`,
		// Call 3: Classification
		`{"memory_type": "process", "category": "General", "classification": "Abstract",
		  "priority": "Low", "context_labels": [], "tags": [], "confidence": 0.60}`,
		// Call 4: Summary
		`{"summary": "Generic content.", "key_points": ["General topic"]}`,
	}

	pipeline := NewExtractionPipeline(mock, db)
	result, err := pipeline.Extract(ctx, memoryID, content)

	// Pipeline should complete successfully
	if err != nil {
		t.Fatalf("Pipeline should handle empty entities gracefully: %v", err)
	}

	// Entity extraction should have completed (with zero entities)
	if result.EntityStatus != types.EnrichmentCompleted {
		t.Errorf("Expected entity status %s, got %s", types.EnrichmentCompleted, result.EntityStatus)
	}

	// Should have zero entities
	if len(result.Entities) != 0 {
		t.Errorf("Expected 0 entities in result, got %d", len(result.Entities))
	}

	// Relationship extraction should be skipped (no entities)
	if result.RelationshipStatus != types.EnrichmentSkipped {
		t.Errorf("Expected relationship status %s, got %s", types.EnrichmentSkipped, result.RelationshipStatus)
	}

	// Verify no entities in database
	var entityCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entityCount)
	if err != nil {
		t.Fatalf("Failed to count entities: %v", err)
	}
	if entityCount != 0 {
		t.Errorf("Expected 0 entities in DB, got %d", entityCount)
	}

	// Classification may fail if empty content causes parsing issues - that's OK
	// The important thing is that other extractions were attempted
}


// TestLinkEntityToMemory_FrequencyIncrement tests that repeated entity links increment frequency
func TestLinkEntityToMemory_FrequencyIncrement(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	memoryID := "mem:test:005"
	entityID := "ent:person:abc123"

	// Setup: Insert entity
	now := time.Now()
	_, err := db.Exec(
		`INSERT INTO entities (id, name, type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		entityID, "TestEntity", "person", now, now,
	)
	if err != nil {
		t.Fatalf("Failed to insert entity: %v", err)
	}

	pipeline := NewExtractionPipeline(nil, db)

	// Link entity to memory (first time)
	err = pipeline.linkEntityToMemory(ctx, memoryID, entityID, 0.90)
	if err != nil {
		t.Fatalf("First linkEntityToMemory failed: %v", err)
	}

	// Verify frequency is 1
	var freq1 int
	err = db.QueryRow(
		`SELECT frequency FROM memory_entities WHERE memory_id = ? AND entity_id = ?`,
		memoryID, entityID,
	).Scan(&freq1)
	if err != nil {
		t.Fatalf("Failed to query frequency after first link: %v", err)
	}
	if freq1 != 1 {
		t.Errorf("Expected frequency 1 after first link, got %d", freq1)
	}

	// Link the same entity to the same memory again
	err = pipeline.linkEntityToMemory(ctx, memoryID, entityID, 0.85)
	if err != nil {
		t.Fatalf("Second linkEntityToMemory failed: %v", err)
	}

	// Verify frequency incremented to 2
	var freq2 int
	err = db.QueryRow(
		`SELECT frequency FROM memory_entities WHERE memory_id = ? AND entity_id = ?`,
		memoryID, entityID,
	).Scan(&freq2)
	if err != nil {
		t.Fatalf("Failed to query frequency after second link: %v", err)
	}
	if freq2 != 2 {
		t.Errorf("Expected frequency 2 after second link, got %d", freq2)
	}

	// Verify there's still only one row (not two)
	var count int
	err = db.QueryRow(
		`SELECT COUNT(*) FROM memory_entities WHERE memory_id = ? AND entity_id = ?`,
		memoryID, entityID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count memory_entities: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 row in memory_entities, got %d (upsert failed)", count)
	}
}

// TestEnrichmentPipeline_MultipleEntitiesandRelationships tests complex extraction scenarios
func TestEnrichmentPipeline_MultipleEntitiesandRelationships(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	memoryID := "mem:test:006"
	content := "MJ and Steve work together on OPS. They use Go and PostgreSQL."

	insertTestMemory(t, db, memoryID, content)

	// Mock responses with 5 entities and 4 relationships
	mock := newMockLLMClient()
	mock.responses = []string{
		// Call 1: Entity extraction
		`{"entities": [
			{"name": "MJ", "type": "person", "confidence": 0.99},
			{"name": "Steve", "type": "person", "confidence": 0.98},
			{"name": "OPS", "type": "organization", "confidence": 0.95},
			{"name": "Go", "type": "language", "confidence": 0.97},
			{"name": "PostgreSQL", "type": "database", "confidence": 0.96}
		]}`,
		// Call 2: Relationship extraction
		`{"relationships": [
			{"from": "MJ", "to": "Steve", "type": "works_with", "confidence": 0.90},
			{"from": "MJ", "to": "OPS", "type": "employed_by", "confidence": 0.92},
			{"from": "Steve", "to": "OPS", "type": "employed_by", "confidence": 0.91},
			{"from": "MJ", "to": "Go", "type": "uses", "confidence": 0.94}
		]}`,
		// Call 3: Classification
		`{"memory_type": "process", "category": "Technology", "classification": "Development",
		  "priority": "High", "context_labels": ["Technical"], "tags": ["golang", "database"], "confidence": 0.85}`,
		// Call 4: Summary
		`{"summary": "MJ and Steve collaborate on OPS using Go and PostgreSQL.",
		  "key_points": ["Team collaboration", "Technology stack", "Open source tools"]}`,
	}

	pipeline := NewExtractionPipeline(mock, db)
	result, err := pipeline.Extract(ctx, memoryID, content)

	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}

	// Verify all 5 entities extracted
	if len(result.Entities) != 5 {
		t.Errorf("Expected 5 entities, got %d", len(result.Entities))
	}

	// Verify all 4 relationships extracted
	if len(result.Relationships) != 4 {
		t.Errorf("Expected 4 relationships, got %d", len(result.Relationships))
	}

	// Verify entities in database
	var entityCount int
	db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entityCount)
	if entityCount != 5 {
		t.Errorf("Expected 5 entities in DB, got %d", entityCount)
	}

	// Verify relationships in database
	var relCount int
	db.QueryRow(`SELECT COUNT(*) FROM relationships`).Scan(&relCount)
	if relCount != 4 {
		t.Errorf("Expected 4 relationships in DB, got %d", relCount)
	}

	// Verify memory-entity links
	var linkCount int
	db.QueryRow(`SELECT COUNT(*) FROM memory_entities`).Scan(&linkCount)
	if linkCount != 5 {
		t.Errorf("Expected 5 memory-entity links, got %d", linkCount)
	}

	// Verify classification extracted
	if result.Classification == nil {
		t.Fatal("Expected classification to be non-nil")
	}
	if result.Classification.Category != "Technology" {
		t.Errorf("Expected category 'Technology', got '%s'", result.Classification.Category)
	}

	// Verify summary extracted
	if result.Summary == nil {
		t.Fatal("Expected summary to be non-nil")
	}
	if len(result.Summary.KeyPoints) != 3 {
		t.Errorf("Expected 3 key points, got %d", len(result.Summary.KeyPoints))
	}
}

// TestEnrichmentPipeline_InvalidEntityType tests that invalid entity types are skipped
func TestEnrichmentPipeline_InvalidEntityType(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	memoryID := "mem:test:007"
	content := "MJ is a person, ACME is a company, and mysterious is unknown."

	insertTestMemory(t, db, memoryID, content)

	// Mock LLM response with invalid entity type
	mock := newMockLLMClient()
	mock.responses = []string{
		// Call 1: Entity extraction with invalid type
		`{"entities": [
			{"name": "MJ", "type": "person", "confidence": 0.95},
			{"name": "ACME", "type": "invalid_type", "confidence": 0.90},
			{"name": "mysterious", "type": "unknown_thing", "confidence": 0.50}
		]}`,
		// Call 2: Relationships (should only include valid entity)
		`{"relationships": [
			{"from": "MJ", "to": "ACME", "type": "employed_by", "confidence": 0.80}
		]}`,
		// Call 3: Classification
		`{"memory_type": "concept", "category": "General", "classification": "Info",
		  "priority": "Low", "context_labels": [], "tags": [], "confidence": 0.70}`,
		// Call 4: Summary
		`{"summary": "Test content.", "key_points": ["Item 1"]}`,
	}

	pipeline := NewExtractionPipeline(mock, db)
	result, err := pipeline.Extract(ctx, memoryID, content)

	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}

	// Response parser filters out invalid types, so only valid entities are returned
	// In this case, 2 invalid types and 1 valid
	if len(result.Entities) != 1 {
		t.Errorf("Expected result to contain 1 valid entity after parsing, got %d", len(result.Entities))
	}

	// Only the valid entity should be stored in database
	var entityCount int
	db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entityCount)
	if entityCount != 1 {
		t.Errorf("Expected only 1 valid entity in DB, got %d", entityCount)
	}

	// Verify the valid entity is "MJ"
	var name string
	err = db.QueryRow(`SELECT name FROM entities LIMIT 1`).Scan(&name)
	if err != nil {
		t.Fatalf("Failed to query entity name: %v", err)
	}
	if name != "MJ" {
		t.Errorf("Expected entity name 'MJ', got '%s'", name)
	}
}

// TestEnrichmentPipeline_ConfidenceRangeValidation tests that invalid confidence scores are rejected
func TestEnrichmentPipeline_ConfidenceRangeValidation(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	memoryID := "mem:test:008"
	content := "Test content with confidence issues."

	insertTestMemory(t, db, memoryID, content)

	// Mock LLM response with invalid confidence scores
	mock := newMockLLMClient()
	mock.responses = []string{
		// Call 1: Entity with out-of-range confidence
		`{"entities": [
			{"name": "ValidEntity", "type": "person", "confidence": 0.95},
			{"name": "InvalidEntity", "type": "organization", "confidence": 1.5}
		]}`,
		// Call 2: Relationships (one with invalid entity)
		`{"relationships": [
			{"from": "ValidEntity", "to": "InvalidEntity", "type": "colleague_of", "confidence": 0.85}
		]}`,
		// Call 3: Classification
		`{"memory_type": "concept", "category": "General", "classification": "Test",
		  "priority": "Low", "context_labels": [], "tags": [], "confidence": 0.75}`,
		// Call 4: Summary
		`{"summary": "Test.", "key_points": ["Point"]}`,
	}

	pipeline := NewExtractionPipeline(mock, db)
	result, err := pipeline.Extract(ctx, memoryID, content)

	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}

	// Response parser filters out invalid confidence scores, so only valid entities are returned
	if len(result.Entities) != 1 {
		t.Errorf("Expected 1 valid entity in result (after filtering invalid confidence), got %d", len(result.Entities))
	}

	// Only valid entity should be stored
	var entityCount int
	db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entityCount)
	if entityCount != 1 {
		t.Errorf("Expected only 1 valid entity in DB, got %d", entityCount)
	}

	// Relationship should not be stored (target entity missing)
	var relCount int
	db.QueryRow(`SELECT COUNT(*) FROM relationships`).Scan(&relCount)
	if relCount != 0 {
		t.Errorf("Expected 0 relationships in DB (invalid target), got %d", relCount)
	}
}

// TestEnrichmentPipeline_Call3Fails tests when classification extraction fails
func TestEnrichmentPipeline_Call3Fails(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	memoryID := "mem:test:009"
	content := "MJ works with Go on important projects."

	insertTestMemory(t, db, memoryID, content)

	// Mock LLM: success on calls 1,2,4 but fail on call 3
	mock := newMockLLMClient()
	mock.errors = []error{
		nil, // Call 1 succeeds
		nil, // Call 2 succeeds
		errors.New("Classification model error"),
		nil, // Call 4 succeeds
	}
	mock.responses = []string{
		// Call 1: Entities
		`{"entities": [
			{"name": "MJ", "type": "person", "confidence": 0.95},
			{"name": "Go", "type": "language", "confidence": 0.98}
		]}`,
		// Call 2: Relationships
		`{"relationships": [
			{"from": "MJ", "to": "Go", "type": "uses", "confidence": 0.95}
		]}`,
		// Call 3: Will fail
		"",
		// Call 4: Summary
		`{"summary": "MJ uses Go.", "key_points": ["Go usage"]}`,
	}

	pipeline := NewExtractionPipeline(mock, db)
	result, err := pipeline.Extract(ctx, memoryID, content)

	// Should not return fatal error
	if err != nil {
		t.Fatalf("Pipeline should handle Call 3 failure gracefully: %v", err)
	}

	// Entities and relationships should succeed
	if result.EntityStatus != types.EnrichmentCompleted {
		t.Errorf("Expected entity status completed, got %s", result.EntityStatus)
	}
	if result.RelationshipStatus != types.EnrichmentCompleted {
		t.Errorf("Expected relationship status completed, got %s", result.RelationshipStatus)
	}

	// Classification should fail
	if result.ClassificationStatus != types.EnrichmentFailed {
		t.Errorf("Expected classification status failed, got %s", result.ClassificationStatus)
	}

	// Summary should succeed
	if result.SummarizationStatus != types.EnrichmentCompleted {
		t.Errorf("Expected summarization status completed, got %s", result.SummarizationStatus)
	}

	// Verify data was persisted
	var entityCount, relCount int
	db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entityCount)
	db.QueryRow(`SELECT COUNT(*) FROM relationships`).Scan(&relCount)
	if entityCount != 2 {
		t.Errorf("Expected 2 entities persisted, got %d", entityCount)
	}
	if relCount != 1 {
		t.Errorf("Expected 1 relationship persisted, got %d", relCount)
	}
}

// TestEnrichmentPipeline_Call4Fails tests when summarization extraction fails
func TestEnrichmentPipeline_Call4Fails(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	memoryID := "mem:test:010"
	content := "MJ and Norma work on important projects together."

	insertTestMemory(t, db, memoryID, content)

	// Mock LLM: success on calls 1-3, fail on call 4
	mock := newMockLLMClient()
	mock.errors = []error{
		nil, // Call 1 succeeds
		nil, // Call 2 succeeds
		nil, // Call 3 succeeds
		errors.New("Summary generation timeout"),
	}
	mock.responses = []string{
		// Call 1: Entities
		`{"entities": [
			{"name": "MJ", "type": "person", "confidence": 0.95},
			{"name": "Norma", "type": "person", "confidence": 0.92}
		]}`,
		// Call 2: Relationships
		`{"relationships": [
			{"from": "MJ", "to": "Norma", "type": "works_with", "confidence": 0.93}
		]}`,
		// Call 3: Classification
		`{"memory_type": "decision", "category": "Work", "classification": "Collaboration",
		  "priority": "High", "context_labels": ["Important"], "tags": ["teamwork"], "confidence": 0.90}`,
		// Call 4: Will fail
		"",
	}

	pipeline := NewExtractionPipeline(mock, db)
	result, err := pipeline.Extract(ctx, memoryID, content)

	// Should not return fatal error
	if err != nil {
		t.Fatalf("Pipeline should handle Call 4 failure gracefully: %v", err)
	}

	// First three should succeed
	if result.EntityStatus != types.EnrichmentCompleted {
		t.Errorf("Expected entity status completed, got %s", result.EntityStatus)
	}
	if result.RelationshipStatus != types.EnrichmentCompleted {
		t.Errorf("Expected relationship status completed, got %s", result.RelationshipStatus)
	}
	if result.ClassificationStatus != types.EnrichmentCompleted {
		t.Errorf("Expected classification status completed, got %s", result.ClassificationStatus)
	}

	// Summary should fail
	if result.SummarizationStatus != types.EnrichmentFailed {
		t.Errorf("Expected summarization status failed, got %s", result.SummarizationStatus)
	}

	// Verify other data was persisted
	var entityCount, relCount int
	db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entityCount)
	db.QueryRow(`SELECT COUNT(*) FROM relationships`).Scan(&relCount)
	if entityCount != 2 {
		t.Errorf("Expected 2 entities persisted, got %d", entityCount)
	}
	if relCount != 1 {
		t.Errorf("Expected 1 relationship persisted, got %d", relCount)
	}
}

// TestEnrichmentPipeline_StoresIntoDatabase verifies entity/relationship storage in DB
func TestEnrichmentPipeline_StoresIntoDatabase(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	memoryID := "mem:test:011"
	content := "Alice collaborates with Bob on projects using Python."

	insertTestMemory(t, db, memoryID, content)

	mock := newMockLLMClient()
	mock.responses = []string{
		// Call 1: Entities
		`{"entities": [
			{"name": "Alice", "type": "person", "description": "Software engineer", "confidence": 0.98},
			{"name": "Bob", "type": "person", "description": "Project manager", "confidence": 0.95},
			{"name": "Python", "type": "language", "description": "Programming language", "confidence": 0.99}
		]}`,
		// Call 2: Relationships
		`{"relationships": [
			{"from": "Alice", "to": "Bob", "type": "colleague_of", "confidence": 0.96},
			{"from": "Alice", "to": "Python", "type": "uses", "confidence": 0.98}
		]}`,
		// Call 3: Classification
		`{"memory_type": "process", "category": "Development", "classification": "Collaboration",
		  "priority": "High", "context_labels": ["Technical"], "tags": [], "confidence": 0.92}`,
		// Call 4: Summary
		`{"summary": "Alice and Bob collaborate on Python projects.",
		  "key_points": ["Collaboration", "Python development", "Team project"]}`,
	}

	pipeline := NewExtractionPipeline(mock, db)
	_, err := pipeline.Extract(ctx, memoryID, content)

	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}

	// Verify entities in database have correct fields
	type EntityRow struct {
		ID          string
		Name        string
		Type        string
		Description string
	}

	rows, err := db.Query(`SELECT id, name, type, description FROM entities ORDER BY name`)
	if err != nil {
		t.Fatalf("Failed to query entities: %v", err)
	}
	defer rows.Close()

	var entities []EntityRow
	for rows.Next() {
		var e EntityRow
		if err := rows.Scan(&e.ID, &e.Name, &e.Type, &e.Description); err != nil {
			t.Fatalf("Failed to scan entity: %v", err)
		}
		entities = append(entities, e)
	}

	if len(entities) != 3 {
		t.Errorf("Expected 3 entities, got %d", len(entities))
	}

	// Check Alice
	if len(entities) > 0 {
		if entities[0].Name != "Alice" || entities[0].Type != "person" {
			t.Errorf("Expected Alice (person), got %s (%s)", entities[0].Name, entities[0].Type)
		}
		if entities[0].ID == "" {
			t.Error("Entity ID should not be empty")
		}
	}

	// Verify relationships stored with weights
	var relCount int
	var avgWeight sql.NullFloat64
	err = db.QueryRow(
		`SELECT COUNT(*), AVG(weight) FROM relationships`,
	).Scan(&relCount, &avgWeight)
	if err != nil {
		t.Fatalf("Failed to query relationships: %v", err)
	}

	if relCount != 2 {
		t.Errorf("Expected 2 relationships, got %d", relCount)
	}

	if avgWeight.Valid && (avgWeight.Float64 < 0.96 || avgWeight.Float64 > 0.98) {
		t.Errorf("Average relationship weight unexpected: %.2f", avgWeight.Float64)
	}

	// Verify memory-entity links with correct frequency and confidence
	var linkCount, totalFreq int
	var maxConf float64
	err = db.QueryRow(
		`SELECT COUNT(*), SUM(frequency), MAX(confidence) FROM memory_entities WHERE memory_id = ?`,
		memoryID,
	).Scan(&linkCount, &totalFreq, &maxConf)
	if err != nil {
		t.Fatalf("Failed to query memory_entities: %v", err)
	}

	if linkCount != 3 {
		t.Errorf("Expected 3 memory-entity links, got %d", linkCount)
	}
	if totalFreq != 3 {
		t.Errorf("Expected total frequency 3, got %d", totalFreq)
	}
}
