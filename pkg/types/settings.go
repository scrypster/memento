package types

import (
	"encoding/json"
	"time"
)

// ConnectionSettings represents per-connection custom taxonomy definitions.
// It stores custom extensions to the system defaults for entities, relationships,
// memory types, and customized classification schemas.
type ConnectionSettings struct {
	ID                              string                   `json:"id"`
	ConnectionID                    string                   `json:"connection_id"`
	CustomEntityTypes               []CustomEntityType       `json:"custom_entity_types,omitempty"`
	CustomRelationshipTypes         []CustomRelationshipType `json:"custom_relationship_types,omitempty"`
	CustomMemoryTypes               []CustomMemoryType       `json:"custom_memory_types,omitempty"`
	CustomClassificationSchemas     []ClassificationSchema   `json:"custom_classification_schemas,omitempty"`
	ActiveClassificationCategory    string                   `json:"active_classification_category,omitempty"`
	CreatedAt                       time.Time                `json:"created_at"`
	UpdatedAt                       time.Time                `json:"updated_at"`
}

// CustomEntityType represents a user-added entity type that extends the system defaults.
type CustomEntityType struct {
	// Unique identifier for the entity type (e.g., "customer", "vendor", "asset")
	ID string `json:"id"`

	// Display name for the entity type
	Name string `json:"name"`

	// Description of what this entity type represents
	Description string `json:"description,omitempty"`
}

// CustomRelationshipType represents a user-added relationship type that extends system defaults.
type CustomRelationshipType struct {
	// Unique identifier for the relationship (e.g., "manages", "supervises", "sponsors")
	ID string `json:"id"`

	// Display name for the relationship
	Name string `json:"name"`

	// Description of what this relationship represents
	Description string `json:"description,omitempty"`

	// Whether this relationship is bidirectional (e.g., "married_to" is bidirectional)
	Bidirectional bool `json:"bidirectional"`

	// Keywords to help the LLM identify this relationship
	Keywords []string `json:"keywords,omitempty"`
}

// CustomMemoryType represents a user-added memory type that extends system defaults.
type CustomMemoryType struct {
	// Unique identifier for the memory type (e.g., "incident", "feature_idea", "announcement")
	ID string `json:"id"`

	// Display name for the memory type
	Name string `json:"name"`

	// Description of what this memory type represents
	Description string `json:"description,omitempty"`

	// Keywords to help the LLM identify this memory type
	Keywords []string `json:"keywords,omitempty"`
}

// SettingsResponse represents the API response for GET /api/connections/:id/settings
// It includes both system defaults and per-connection customizations, merged together.
type SettingsResponse struct {
	// System defaults (not editable via API, but included for reference)
	SystemEntityTypes         []string                `json:"system_entity_types"`
	SystemRelationshipTypes   []string                `json:"system_relationship_types"`
	SystemMemoryTypes         []string                `json:"system_memory_types"`
	SystemClassificationSchemas []ClassificationSchema `json:"system_classification_schemas"`

	// Per-connection customizations
	CustomEntityTypes         []CustomEntityType       `json:"custom_entity_types"`
	CustomRelationshipTypes   []CustomRelationshipType `json:"custom_relationship_types"`
	CustomMemoryTypes         []CustomMemoryType       `json:"custom_memory_types"`
	CustomClassificationSchemas []ClassificationSchema `json:"custom_classification_schemas"`

	// Merged views (for LLM enrichment pipeline)
	AllEntityTypes           []string                `json:"all_entity_types"`
	AllRelationshipTypes     []string                `json:"all_relationship_types"`
	AllMemoryTypes           []string                `json:"all_memory_types"`
	AllClassificationSchemas []ClassificationSchema  `json:"all_classification_schemas"`

	// The user-selected classification category for this connection's enrichment
	ActiveClassificationCategory string `json:"active_classification_category"`
}

// UpdateEntityTypesRequest represents the request body for PUT /api/connections/:id/settings/entities
type UpdateEntityTypesRequest struct {
	CustomEntityTypes []CustomEntityType `json:"custom_entity_types"`
}

// UpdateRelationshipTypesRequest represents the request body for PUT /api/connections/:id/settings/relationships
type UpdateRelationshipTypesRequest struct {
	CustomRelationshipTypes []CustomRelationshipType `json:"custom_relationship_types"`
}

// UpdateMemoryTypesRequest represents the request body for PUT /api/connections/:id/settings/memory-types
type UpdateMemoryTypesRequest struct {
	CustomMemoryTypes []CustomMemoryType `json:"custom_memory_types"`
}

// UpdateClassificationsRequest represents the request body for PUT /api/connections/:id/settings/classifications
type UpdateClassificationsRequest struct {
	CustomClassificationSchemas []ClassificationSchema `json:"custom_classification_schemas"`
}

// UpdateActiveCategoryRequest represents the request body for PUT /api/connections/:id/settings/active-category
type UpdateActiveCategoryRequest struct {
	ActiveClassificationCategory string `json:"active_classification_category"`
}

// SavedConnectionSettings represents what's actually stored in the database (JSON strings)
type SavedConnectionSettings struct {
	ID                              string
	ConnectionID                    string
	CustomEntityTypes               string    // JSON string
	CustomRelationshipTypes         string    // JSON string
	CustomMemoryTypes               string    // JSON string
	CustomClassificationSchemas     string    // JSON string
	ActiveClassificationCategory    string
	CreatedAt                       time.Time
	UpdatedAt                       time.Time
}

// UnmarshalCustomEntityTypes unmarshals the JSON string into a slice of CustomEntityType
func UnmarshalCustomEntityTypes(jsonStr string) ([]CustomEntityType, error) {
	if jsonStr == "" {
		return []CustomEntityType{}, nil
	}
	var types []CustomEntityType
	if err := json.Unmarshal([]byte(jsonStr), &types); err != nil {
		return nil, err
	}
	return types, nil
}

// UnmarshalCustomRelationshipTypes unmarshals the JSON string into a slice of CustomRelationshipType
func UnmarshalCustomRelationshipTypes(jsonStr string) ([]CustomRelationshipType, error) {
	if jsonStr == "" {
		return []CustomRelationshipType{}, nil
	}
	var types []CustomRelationshipType
	if err := json.Unmarshal([]byte(jsonStr), &types); err != nil {
		return nil, err
	}
	return types, nil
}

// UnmarshalCustomMemoryTypes unmarshals the JSON string into a slice of CustomMemoryType
func UnmarshalCustomMemoryTypes(jsonStr string) ([]CustomMemoryType, error) {
	if jsonStr == "" {
		return []CustomMemoryType{}, nil
	}
	var types []CustomMemoryType
	if err := json.Unmarshal([]byte(jsonStr), &types); err != nil {
		return nil, err
	}
	return types, nil
}

// UnmarshalCustomClassificationSchemas unmarshals the JSON string into a slice of ClassificationSchema
func UnmarshalCustomClassificationSchemas(jsonStr string) ([]ClassificationSchema, error) {
	if jsonStr == "" {
		return []ClassificationSchema{}, nil
	}
	var schemas []ClassificationSchema
	if err := json.Unmarshal([]byte(jsonStr), &schemas); err != nil {
		return nil, err
	}
	return schemas, nil
}
