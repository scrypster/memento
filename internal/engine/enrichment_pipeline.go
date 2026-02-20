package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/scrypster/memento/internal/llm"
	"github.com/scrypster/memento/internal/services"
	"github.com/scrypster/memento/pkg/types"
)

// ExtractionPipeline implements a two-call LLM extraction pipeline for entity and relationship extraction.
// Call 1: Extract entities from memory content
// Call 2: Extract relationships between extracted entities
//
// The pipeline maintains atomicity by tracking status updates separately for each call,
// allowing graceful degradation if one call fails while the other succeeds.
type ExtractionPipeline struct {
	llmClient       llm.TextGenerator
	db              *sql.DB
	settingsService *services.SettingsService
}

// NewExtractionPipeline creates a new extraction pipeline with the given LLM client and database.
func NewExtractionPipeline(llmClient llm.TextGenerator, db *sql.DB) *ExtractionPipeline {
	var settingsSvc *services.SettingsService
	if db != nil {
		settingsSvc = services.NewSettingsService(db)
	}
	return &ExtractionPipeline{
		llmClient:       llmClient,
		db:              db,
		settingsService: settingsSvc,
	}
}

// ExtractPipelineResult holds the results of the extraction pipeline execution.
// It tracks which calls succeeded or failed and stores extracted data.
type ExtractPipelineResult struct {
	// EntityStatus reflects the result of Call 1 (entity extraction)
	EntityStatus types.EnrichmentStatus
	EntityError  string
	Entities     []llm.EntityResponse
	EntityIDs    map[string]string // name -> entity_id mapping

	// RelationshipStatus reflects the result of Call 2 (relationship extraction)
	RelationshipStatus types.EnrichmentStatus
	RelationshipError  string
	Relationships      []llm.RelationshipResponse

	// ClassificationStatus reflects the result of Call 3 (classification extraction)
	ClassificationStatus types.EnrichmentStatus
	ClassificationError  string
	Classification      *llm.ClassificationResponse

	// SummarizationStatus reflects the result of Call 4 (summarization)
	SummarizationStatus types.EnrichmentStatus
	SummarizationError  string
	Summary             *llm.SummarizationResponse

	// ExecutedAt tracks when the pipeline completed
	ExecutedAt time.Time
}

// Extract orchestrates the multi-call extraction pipeline.
//
// Call 1: Entity Extraction (sequential, required)
//   - Sends content to LLM for entity extraction
//   - Stores extracted entities in database
//   - Returns error if failed (skips remaining calls)
//
// Call 2: Relationship Extraction (sequential, depends on entities)
//   - Passes extracted entities to relationship extraction prompt
//   - Stores relationships in database
//   - Non-fatal if fails (preserves entities)
//
// Call 3: Classification Extraction (independent)
//   - Extracts memory type, category, priority, context labels, tags
//   - Stores classification in memory record
//   - Non-fatal if fails
//
// Call 4: Summarization (independent)
//   - Extracts summary and key points
//   - Stores summary in memory record
//   - Non-fatal if fails
//
// Error Handling:
//   - Call 1 failure: Returns error immediately, skips remaining calls
//   - Calls 2-4 failures: Logged but non-fatal, other extractions preserved
//   - Each call's status tracked independently
//   - Partial failures are OK (store what succeeds)
//
// Atomicity:
//   - Each call's results are committed independently
//   - Status updates reflect actual outcome of each call
//   - Caller responsible for final memory status after pipeline completes
func (p *ExtractionPipeline) Extract(ctx context.Context, memoryID, content string) (*ExtractPipelineResult, error) {
	result := &ExtractPipelineResult{
		EntityIDs:            make(map[string]string),
		ClassificationStatus: types.EnrichmentPending,
		SummarizationStatus:  types.EnrichmentPending,
		ExecutedAt:           time.Now(),
	}

	// Fetch per-connection settings (custom entity/relationship/memory types)
	settings := p.fetchSettings(memoryID)
	if settings != nil {
		log.Printf("Pipeline: Loaded settings for memory %s (active category: %q, custom entity types: %d, custom rel types: %d)",
			memoryID, settings.ActiveClassificationCategory,
			len(settings.CustomEntityTypes), len(settings.CustomRelationshipTypes))
	}

	// Call 1: Entity Extraction
	log.Printf("Pipeline: Starting entity extraction for memory %s", memoryID)
	entities, entityIDMap, entityErr := p.extractAndStoreEntities(ctx, memoryID, content, settings)
	if entityErr != nil {
		log.Printf("Pipeline: Entity extraction failed for %s: %v", memoryID, entityErr)
		result.EntityStatus = types.EnrichmentFailed
		result.EntityError = entityErr.Error()
		// Return error to skip remaining extractions
		return result, fmt.Errorf("entity extraction failed: %w", entityErr)
	}

	result.EntityStatus = types.EnrichmentCompleted
	result.Entities = entities
	result.EntityIDs = entityIDMap
	log.Printf("Pipeline: Successfully extracted %d entities for memory %s", len(entities), memoryID)

	// Call 2: Relationship Extraction (only if entities were extracted)
	if len(entities) > 0 {
		log.Printf("Pipeline: Starting relationship extraction for memory %s", memoryID)
		relationships, relErr := p.extractAndStoreRelationships(ctx, memoryID, content, entities, entityIDMap, settings)
		if relErr != nil {
			log.Printf("Pipeline: Relationship extraction failed for %s: %v", memoryID, relErr)
			result.RelationshipStatus = types.EnrichmentFailed
			result.RelationshipError = relErr.Error()
			log.Printf("Pipeline: WARNING - Relationship extraction failed, but entities were preserved")
		} else {
			result.RelationshipStatus = types.EnrichmentCompleted
			result.Relationships = relationships
			log.Printf("Pipeline: Successfully extracted %d relationships for memory %s", len(relationships), memoryID)
		}
	} else {
		log.Printf("Pipeline: Skipping relationship extraction (no entities extracted)")
		result.RelationshipStatus = types.EnrichmentSkipped
	}

	// Call 3: Classification Extraction (independent of entities/relationships)
	log.Printf("Pipeline: Starting classification extraction for memory %s", memoryID)
	classification, classErr := p.extractAndStoreClassification(ctx, memoryID, content, settings)
	if classErr != nil {
		log.Printf("Pipeline: Classification extraction failed for %s: %v", memoryID, classErr)
		result.ClassificationStatus = types.EnrichmentFailed
		result.ClassificationError = classErr.Error()
		log.Printf("Pipeline: WARNING - Classification extraction failed, but other extractions preserved")
	} else {
		result.ClassificationStatus = types.EnrichmentCompleted
		result.Classification = classification
		log.Printf("Pipeline: Successfully extracted classification for memory %s: %s/%s", memoryID, classification.Category, classification.Classification)
	}

	// Call 4: Summarization (independent of other extractions)
	log.Printf("Pipeline: Starting summarization for memory %s", memoryID)
	summary, summErr := p.extractAndStoreSummary(ctx, memoryID, content)
	if summErr != nil {
		log.Printf("Pipeline: Summarization failed for %s: %v", memoryID, summErr)
		result.SummarizationStatus = types.EnrichmentFailed
		result.SummarizationError = summErr.Error()
		log.Printf("Pipeline: WARNING - Summarization failed, but other extractions preserved")
	} else {
		result.SummarizationStatus = types.EnrichmentCompleted
		result.Summary = summary
		log.Printf("Pipeline: Successfully created summary for memory %s", memoryID)
	}

	return result, nil
}

// fetchSettings retrieves per-connection settings for the given memoryID.
// Memory IDs have the format mem:domain:slug; the domain part is the connection ID.
// Returns nil if settings cannot be loaded (caller uses defaults).
func (p *ExtractionPipeline) fetchSettings(memoryID string) *types.SettingsResponse {
	if p.settingsService == nil {
		return nil
	}
	// Extract connection ID from mem:domain:slug
	parts := strings.SplitN(memoryID, ":", 3)
	if len(parts) < 2 || parts[1] == "" {
		return nil
	}
	connectionID := parts[1]
	settings, err := p.settingsService.GetSettings(connectionID)
	if err != nil {
		log.Printf("Pipeline: Warning - failed to get settings for connection %s: %v, using defaults", connectionID, err)
		return nil
	}
	return settings
}

// extractAndStoreEntities implements Call 1 of the pipeline: Extract entities and store them.
//
// This method:
//   1. Calls LLM with entity extraction prompt
//   2. Parses response using existing ParseEntityResponse()
//   3. Stores entities in entities table
//   4. Links entities to memory in memory_entities table
//   5. Builds entity ID mapping for relationship extraction
//
// Returns:
//   - Slice of extracted EntityResponse objects
//   - Error if LLM call or storage fails (partial failures logged but not fatal)
func (p *ExtractionPipeline) extractAndStoreEntities(ctx context.Context, memoryID, content string, settings *types.SettingsResponse) ([]llm.EntityResponse, map[string]string, error) {
	// Call LLM for entity extraction using settings-aware prompt
	prompt := llm.EntityExtractionPromptWithSettings(content, settings)
	response, err := p.llmClient.Complete(ctx, prompt)
	if err != nil {
		return nil, nil, fmt.Errorf("LLM entity extraction failed: %w", err)
	}

	// Parse response, validating against merged type list
	var entities []llm.EntityResponse
	var entitySkipped []llm.SkippedTypeInfo
	if settings != nil && len(settings.AllEntityTypes) > 0 {
		entities, entitySkipped, err = llm.ParseEntityResponseWithTypesDetailed(response, settings.AllEntityTypes)
	} else {
		entities, entitySkipped, err = llm.ParseEntityResponseDetailed(response)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse entity response: %w", err)
	}
	p.recordUnknownTypes(ctx, entitySkipped)

	entityIDMap := make(map[string]string, len(entities))

	if len(entities) == 0 {
		log.Printf("Pipeline: LLM returned no entities for memory %s", memoryID)
		return entities, entityIDMap, nil // Not an error; just no entities
	}

	// Store entities in database
	var allowedEntityTypes []string
	if settings != nil {
		allowedEntityTypes = settings.AllEntityTypes
	}
	for _, entity := range entities {
		// Validate entity against merged types list
		if err := p.validateEntityWithTypes(entity, allowedEntityTypes); err != nil {
			log.Printf("Pipeline: Skipping invalid entity %s: %v", entity.Name, err)
			continue
		}

		// Store entity — use the DB-returned ID (may differ from computed hash
		// when entity already existed from a previous memory's enrichment).
		entityID, err := p.storeEntity(ctx, entity)
		if err != nil {
			log.Printf("Pipeline: WARNING - Failed to store entity %s: %v", entity.Name, err)
			// Continue storing other entities
			continue
		}

		entityIDMap[entity.Name] = entityID

		// Link entity to memory
		if err := p.linkEntityToMemory(ctx, memoryID, entityID, entity.Confidence); err != nil {
			log.Printf("Pipeline: WARNING - Failed to link entity %s to memory: %v", entity.Name, err)
		}
	}

	return entities, entityIDMap, nil
}

// extractAndStoreRelationships implements Call 2 of the pipeline: Extract relationships and store them.
//
// This method:
//   1. Calls LLM with relationship extraction prompt (includes extracted entities)
//   2. Parses response using existing ParseRelationshipResponse()
//   3. Validates relationships reference valid entities
//   4. Stores relationships in relationships table
//   5. Handles cases where referenced entities don't exist (skips those relationships)
//
// Returns:
//   - Slice of extracted RelationshipResponse objects
//   - Error if LLM call fails (storage errors are logged but non-fatal)
func (p *ExtractionPipeline) extractAndStoreRelationships(
	ctx context.Context,
	memoryID string,
	content string,
	entities []llm.EntityResponse,
	entityIDMap map[string]string,
	settings *types.SettingsResponse,
) ([]llm.RelationshipResponse, error) {
	// Convert EntityResponse to types.Entity for prompt
	typedEntities := make([]types.Entity, len(entities))
	for i, e := range entities {
		typedEntities[i] = types.Entity{
			Name: e.Name,
			Type: e.Type,
		}
	}

	// Call LLM for relationship extraction using settings-aware prompt
	prompt := llm.RelationshipExtractionPromptWithSettings(content, typedEntities, settings)
	response, err := p.llmClient.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM relationship extraction failed: %w", err)
	}

	// Parse response, validating against merged type list
	var relationships []llm.RelationshipResponse
	var relSkipped []llm.SkippedTypeInfo
	if settings != nil && len(settings.AllRelationshipTypes) > 0 {
		relationships, relSkipped, err = llm.ParseRelationshipResponseWithTypesDetailed(response, settings.AllRelationshipTypes)
	} else {
		relationships, relSkipped, err = llm.ParseRelationshipResponseDetailed(response)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse relationship response: %w", err)
	}
	p.recordUnknownTypes(ctx, relSkipped)

	if len(relationships) == 0 {
		log.Printf("Pipeline: LLM returned no relationships for memory %s", memoryID)
		return relationships, nil // Not an error; just no relationships
	}

	// Store relationships
	var allowedRelTypes []string
	if settings != nil {
		allowedRelTypes = settings.AllRelationshipTypes
	}
	for _, rel := range relationships {
		// Validate relationship against merged types list
		if err := p.validateRelationshipWithTypes(rel, allowedRelTypes); err != nil {
			log.Printf("Pipeline: Skipping invalid relationship %s -> %s: %v", rel.From, rel.To, err)
			continue
		}

		// Get entity IDs
		sourceID := entityIDMap[rel.From]
		targetID := entityIDMap[rel.To]

		if sourceID == "" || targetID == "" {
			log.Printf("Pipeline: Skipping relationship %s -> %s (entity not found in extraction)", rel.From, rel.To)
			continue
		}

		// Store relationship
		if err := p.storeRelationship(ctx, sourceID, targetID, rel.Type, rel.Confidence); err != nil {
			log.Printf("Pipeline: WARNING - Failed to store relationship %s -> %s: %v", rel.From, rel.To, err)
			// Continue storing other relationships
		}
	}

	return relationships, nil
}

// storeEntity stores an entity in the database (upsert).
// Returns the entity ID on success.
func (p *ExtractionPipeline) storeEntity(ctx context.Context, entity llm.EntityResponse) (string, error) {
	// Generate entity ID from name and type (consistent with enrichment_service.go)
	entityID := fmt.Sprintf("ent:%s:%s", entity.Type, hashForEntity(entity.Name))

	query := `
		INSERT INTO entities (id, name, type, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(name, type) DO UPDATE SET
			description = excluded.description,
			updated_at = excluded.updated_at
		RETURNING id
	`

	now := time.Now()
	var returnedID string
	err := p.db.QueryRowContext(ctx, query,
		entityID, entity.Name, entity.Type, entity.Description, now, now,
	).Scan(&returnedID)

	if err != nil {
		return "", fmt.Errorf("failed to store entity: %w", err)
	}

	return returnedID, nil
}

// storeRelationship stores a relationship in the database (upsert).
func (p *ExtractionPipeline) storeRelationship(ctx context.Context, sourceID, targetID, relType string, confidence float64) error {
	// Generate relationship ID
	relID := fmt.Sprintf("rel:%s:%s:%s", sourceID, targetID, relType)

	query := `
		INSERT INTO relationships (id, source_id, target_id, type, weight, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id, target_id, type) DO UPDATE SET
			weight = excluded.weight,
			updated_at = excluded.updated_at
	`

	now := time.Now()
	_, err := p.db.ExecContext(ctx, query,
		relID, sourceID, targetID, relType, confidence, now, now,
	)

	if err != nil {
		return fmt.Errorf("failed to store relationship: %w", err)
	}

	return nil
}

// linkEntityToMemory creates a memory-entity association.
func (p *ExtractionPipeline) linkEntityToMemory(ctx context.Context, memoryID, entityID string, confidence float64) error {
	query := `
		INSERT INTO memory_entities (memory_id, entity_id, frequency, confidence, created_at)
		VALUES (?, ?, 1, ?, ?)
		ON CONFLICT(memory_id, entity_id) DO UPDATE SET
			frequency = frequency + 1,
			confidence = MAX(confidence, excluded.confidence)
	`

	now := time.Now()
	_, err := p.db.ExecContext(ctx, query, memoryID, entityID, confidence, now)

	if err != nil {
		return fmt.Errorf("failed to link entity to memory: %w", err)
	}

	return nil
}

// validateEntityWithTypes validates an extracted entity.
// If allowedTypes is non-empty, validates against that list (system + custom).
// Otherwise falls back to system types only.
func (p *ExtractionPipeline) validateEntityWithTypes(entity llm.EntityResponse, allowedTypes []string) error {
	if entity.Name == "" {
		return fmt.Errorf("entity name is required")
	}
	if entity.Type == "" {
		return fmt.Errorf("entity type is required")
	}

	if len(allowedTypes) > 0 {
		allowed := make(map[string]bool, len(allowedTypes))
		for _, t := range allowedTypes {
			allowed[t] = true
		}
		if !allowed[entity.Type] {
			return fmt.Errorf("invalid entity type: %s", entity.Type)
		}
	} else if !types.IsValidEntityType(entity.Type) {
		return fmt.Errorf("invalid entity type: %s", entity.Type)
	}

	if entity.Confidence < 0.0 || entity.Confidence > 1.0 {
		return fmt.Errorf("invalid confidence score: %f (must be 0.0-1.0)", entity.Confidence)
	}
	return nil
}

// validateRelationshipWithTypes validates an extracted relationship.
// If allowedTypes is non-empty, validates against that list (system + custom).
// Otherwise falls back to system types only.
func (p *ExtractionPipeline) validateRelationshipWithTypes(rel llm.RelationshipResponse, allowedTypes []string) error {
	if rel.From == "" {
		return fmt.Errorf("relationship source is required")
	}
	if rel.To == "" {
		return fmt.Errorf("relationship target is required")
	}
	if rel.Type == "" {
		return fmt.Errorf("relationship type is required")
	}

	if len(allowedTypes) > 0 {
		allowed := make(map[string]bool, len(allowedTypes))
		for _, t := range allowedTypes {
			allowed[t] = true
		}
		if !allowed[rel.Type] {
			return fmt.Errorf("invalid relationship type: %s", rel.Type)
		}
	} else if !types.IsValidRelationshipType(rel.Type) {
		return fmt.Errorf("invalid relationship type: %s", rel.Type)
	}

	if rel.Confidence < 0.0 || rel.Confidence > 1.0 {
		return fmt.Errorf("invalid confidence score: %f (must be 0.0-1.0)", rel.Confidence)
	}
	return nil
}

// extractAndStoreClassification implements Call 3 of the pipeline: Extract classification and store it.
//
// This method:
//   1. Calls LLM with classification extraction prompt
//   2. Parses response using ParseClassificationResponse()
//   3. Updates memory record with extracted fields:
//      - memory_type (decision, process, concept, etc.)
//      - category (Software Development, Business, etc.)
//      - classification (specific classification within category)
//      - priority (Critical, High, Medium, Low)
//      - context_labels (Technical, Critical, Decision, etc.)
//      - tags (free-form tags)
//      - classification_status (completed/failed)
//   4. Sets status in database
//
// Returns:
//   - ClassificationResponse object with extracted data
//   - Error if LLM call fails (storage errors are logged)
func (p *ExtractionPipeline) extractAndStoreClassification(ctx context.Context, memoryID, content string, settings *types.SettingsResponse) (*llm.ClassificationResponse, error) {
	// Call LLM for classification extraction using settings-aware prompt
	prompt := llm.ClassificationExtractionPromptWithSettings(content, settings)
	response, err := p.llmClient.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM classification extraction failed: %w", err)
	}

	// Parse response, validating memory type against merged list
	classification, err := llm.ParseClassificationResponseWithSettings(response, settings)
	if err != nil {
		return nil, fmt.Errorf("failed to parse classification response: %w", err)
	}

	// Store classification in memory record
	query := `
		UPDATE memories
		SET memory_type = ?, category = ?, classification = ?, priority = ?,
		    context_labels = ?, tags = ?, classification_status = ?, updated_at = ?
		WHERE id = ?
	`

	now := time.Now()
	contextLabelsJSON := formatStringArray(classification.ContextLabels)
	tagsJSON := formatStringArray(classification.Tags)

	_, err = p.db.ExecContext(ctx, query,
		classification.MemoryType,
		classification.Category,
		classification.Classification,
		classification.Priority,
		contextLabelsJSON,
		tagsJSON,
		types.EnrichmentCompleted,
		now,
		memoryID,
	)

	if err != nil {
		log.Printf("Pipeline: WARNING - Failed to store classification for memory %s: %v", memoryID, err)
		// Don't return error; classification failure shouldn't block other extractions
	}

	return classification, nil
}

// extractAndStoreSummary implements Call 4 of the pipeline: Extract summary and store it.
//
// This method:
//   1. Calls LLM with summarization prompt
//   2. Parses response using ParseSummarizationResponse()
//   3. Updates memory record with extracted fields:
//      - summary (2-3 sentence summary)
//      - key_points (3-5 key points as JSON array)
//      - summarization_status (completed/failed)
//   4. Sets status in database
//
// Returns:
//   - SummarizationResponse object with extracted data
//   - Error if LLM call fails (storage errors are logged)
func (p *ExtractionPipeline) extractAndStoreSummary(ctx context.Context, memoryID, content string) (*llm.SummarizationResponse, error) {
	// Call LLM for summarization
	prompt := llm.SummarizationPrompt(content)
	response, err := p.llmClient.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM summarization failed: %w", err)
	}

	// Parse response
	summary, err := llm.ParseSummarizationResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse summarization response: %w", err)
	}

	// Store summary in memory record
	query := `
		UPDATE memories
		SET summary = ?, key_points = ?, summarization_status = ?, updated_at = ?
		WHERE id = ?
	`

	now := time.Now()
	keyPointsJSON := formatStringArray(summary.KeyPoints)

	_, err = p.db.ExecContext(ctx, query,
		summary.Summary,
		keyPointsJSON,
		types.EnrichmentCompleted,
		now,
		memoryID,
	)

	if err != nil {
		log.Printf("Pipeline: WARNING - Failed to store summary for memory %s: %v", memoryID, err)
		// Don't return error; summarization failure shouldn't block other extractions
	}

	return summary, nil
}

// formatStringArray converts a string slice to JSON array format for storage.
// Used for storing context_labels, tags, and key_points in database.
func formatStringArray(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	// Simple JSON array formatting
	result := "["
	for i, item := range arr {
		if i > 0 {
			result += ","
		}
		// Escape quotes in item
		escaped := fmt.Sprintf("%q", item)
		result += escaped
	}
	result += "]"
	return result
}

// hashForEntity creates a simple hash for entity ID generation.
// This is a package-level helper used by both the pipeline and service.
func hashForEntity(s string) string {
	h := uint32(0)
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return fmt.Sprintf("%08x", h)
}

// recordUnknownTypes persists skipped-type observations to unknown_type_stats.
// Each call upserts: increments count and updates last_seen if the row exists,
// inserts a new row otherwise. Errors are logged but not fatal.
func (p *ExtractionPipeline) recordUnknownTypes(ctx context.Context, skipped []llm.SkippedTypeInfo) {
	if p.db == nil || len(skipped) == 0 {
		return
	}
	const upsertSQL = `
		INSERT INTO unknown_type_stats (domain, type_name, count, first_seen, last_seen)
		VALUES (?, ?, 1, strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		ON CONFLICT(domain, type_name) DO UPDATE SET
			count    = count + 1,
			last_seen = strftime('%Y-%m-%dT%H:%M:%SZ','now')`
	for _, s := range skipped {
		if _, err := p.db.ExecContext(ctx, upsertSQL, s.Domain, s.TypeName); err != nil {
			log.Printf("enrichment_pipeline: failed to record unknown type %q/%q: %v", s.Domain, s.TypeName, err)
		}
	}
}
