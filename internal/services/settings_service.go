package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scrypster/memento/pkg/types"
)

// SettingsService manages per-connection taxonomy customization.
type SettingsService struct {
	db *sql.DB
}

// NewSettingsService creates a new SettingsService instance.
func NewSettingsService(db *sql.DB) *SettingsService {
	return &SettingsService{db: db}
}

// GetSettings retrieves the settings for a connection, including system defaults merged with customizations.
func (s *SettingsService) GetSettings(connectionID string) (*types.SettingsResponse, error) {
	// Get custom settings from database
	settings, err := s.getCustomSettings(connectionID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get custom settings: %w", err)
	}

	// Build response with system defaults and custom additions
	response := &types.SettingsResponse{
		SystemEntityTypes:         types.ValidEntityTypes,
		SystemRelationshipTypes:   types.ValidRelationshipTypes,
		SystemMemoryTypes:         types.ValidMemoryTypes,
		SystemClassificationSchemas: types.DefaultClassificationSchemas(),
	}

	// Add custom types if they exist
	if settings != nil {
		response.CustomEntityTypes = settings.CustomEntityTypes
		response.CustomRelationshipTypes = settings.CustomRelationshipTypes
		response.CustomMemoryTypes = settings.CustomMemoryTypes
		response.CustomClassificationSchemas = settings.CustomClassificationSchemas
		response.ActiveClassificationCategory = settings.ActiveClassificationCategory
	}

	// Build merged lists
	response.AllEntityTypes = mergeEntityTypes(response.SystemEntityTypes, response.CustomEntityTypes)
	response.AllRelationshipTypes = mergeRelationshipTypes(response.SystemRelationshipTypes, response.CustomRelationshipTypes)
	response.AllMemoryTypes = mergeMemoryTypes(response.SystemMemoryTypes, response.CustomMemoryTypes)
	response.AllClassificationSchemas = mergeClassificationSchemas(response.SystemClassificationSchemas, response.CustomClassificationSchemas)

	return response, nil
}

// SaveEntityTypes saves custom entity types for a connection.
func (s *SettingsService) SaveEntityTypes(connectionID string, customTypes []types.CustomEntityType) error {
	settings, err := s.getOrCreateSettings(connectionID)
	if err != nil {
		return err
	}

	settings.CustomEntityTypes = customTypes
	return s.saveSettings(settings)
}

// SaveRelationshipTypes saves custom relationship types for a connection.
func (s *SettingsService) SaveRelationshipTypes(connectionID string, customTypes []types.CustomRelationshipType) error {
	settings, err := s.getOrCreateSettings(connectionID)
	if err != nil {
		return err
	}

	settings.CustomRelationshipTypes = customTypes
	return s.saveSettings(settings)
}

// SaveMemoryTypes saves custom memory types for a connection.
func (s *SettingsService) SaveMemoryTypes(connectionID string, customTypes []types.CustomMemoryType) error {
	settings, err := s.getOrCreateSettings(connectionID)
	if err != nil {
		return err
	}

	settings.CustomMemoryTypes = customTypes
	return s.saveSettings(settings)
}

// SaveClassificationSchemas saves custom classification schemas for a connection.
func (s *SettingsService) SaveClassificationSchemas(connectionID string, schemas []types.ClassificationSchema) error {
	settings, err := s.getOrCreateSettings(connectionID)
	if err != nil {
		return err
	}

	settings.CustomClassificationSchemas = schemas
	return s.saveSettings(settings)
}

// SaveActiveClassificationCategory saves the selected classification category for a connection.
func (s *SettingsService) SaveActiveClassificationCategory(connectionID string, category string) error {
	settings, err := s.getOrCreateSettings(connectionID)
	if err != nil {
		return err
	}

	settings.ActiveClassificationCategory = category
	return s.saveSettings(settings)
}

// Private helper methods

// getCustomSettings retrieves custom settings from the database for a connection.
func (s *SettingsService) getCustomSettings(connectionID string) (*types.ConnectionSettings, error) {
	var saved types.SavedConnectionSettings

	err := s.db.QueryRow(
		"SELECT id, connection_id, custom_entity_types, custom_relationship_types, custom_memory_types, custom_classification_schemas, active_classification_category, created_at, updated_at FROM connection_settings WHERE connection_id = ?",
		connectionID,
	).Scan(
		&saved.ID,
		&saved.ConnectionID,
		&saved.CustomEntityTypes,
		&saved.CustomRelationshipTypes,
		&saved.CustomMemoryTypes,
		&saved.CustomClassificationSchemas,
		&saved.ActiveClassificationCategory,
		&saved.CreatedAt,
		&saved.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	// Unmarshal JSON fields
	customEntityTypes, err := types.UnmarshalCustomEntityTypes(saved.CustomEntityTypes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal custom entity types: %w", err)
	}

	customRelationshipTypes, err := types.UnmarshalCustomRelationshipTypes(saved.CustomRelationshipTypes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal custom relationship types: %w", err)
	}

	customMemoryTypes, err := types.UnmarshalCustomMemoryTypes(saved.CustomMemoryTypes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal custom memory types: %w", err)
	}

	customClassificationSchemas, err := types.UnmarshalCustomClassificationSchemas(saved.CustomClassificationSchemas)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal custom classification schemas: %w", err)
	}

	return &types.ConnectionSettings{
		ID:                              saved.ID,
		ConnectionID:                    saved.ConnectionID,
		CustomEntityTypes:               customEntityTypes,
		CustomRelationshipTypes:         customRelationshipTypes,
		CustomMemoryTypes:               customMemoryTypes,
		CustomClassificationSchemas:     customClassificationSchemas,
		ActiveClassificationCategory:    saved.ActiveClassificationCategory,
		CreatedAt:                       saved.CreatedAt,
		UpdatedAt:                       saved.UpdatedAt,
	}, nil
}

// getOrCreateSettings retrieves settings or creates a new entry if none exists.
func (s *SettingsService) getOrCreateSettings(connectionID string) (*types.ConnectionSettings, error) {
	settings, err := s.getCustomSettings(connectionID)
	if err == sql.ErrNoRows {
		// Create new settings entry
		settings = &types.ConnectionSettings{
			ID:           uuid.New().String(),
			ConnectionID: connectionID,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		return settings, nil
	}
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// saveSettings saves settings to the database.
func (s *SettingsService) saveSettings(settings *types.ConnectionSettings) error {
	// Marshal JSON fields
	customEntityTypesJSON := ""
	if len(settings.CustomEntityTypes) > 0 {
		data, err := json.Marshal(settings.CustomEntityTypes)
		if err != nil {
			return fmt.Errorf("failed to marshal custom entity types: %w", err)
		}
		customEntityTypesJSON = string(data)
	}

	customRelationshipTypesJSON := ""
	if len(settings.CustomRelationshipTypes) > 0 {
		data, err := json.Marshal(settings.CustomRelationshipTypes)
		if err != nil {
			return fmt.Errorf("failed to marshal custom relationship types: %w", err)
		}
		customRelationshipTypesJSON = string(data)
	}

	customMemoryTypesJSON := ""
	if len(settings.CustomMemoryTypes) > 0 {
		data, err := json.Marshal(settings.CustomMemoryTypes)
		if err != nil {
			return fmt.Errorf("failed to marshal custom memory types: %w", err)
		}
		customMemoryTypesJSON = string(data)
	}

	customClassificationSchemasJSON := ""
	if len(settings.CustomClassificationSchemas) > 0 {
		data, err := json.Marshal(settings.CustomClassificationSchemas)
		if err != nil {
			return fmt.Errorf("failed to marshal custom classification schemas: %w", err)
		}
		customClassificationSchemasJSON = string(data)
	}

	// Use UPSERT for SQLite compatibility
	_, err := s.db.Exec(
		`INSERT INTO connection_settings (id, connection_id, custom_entity_types, custom_relationship_types, custom_memory_types, custom_classification_schemas, active_classification_category, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(connection_id) DO UPDATE SET
		 	custom_entity_types = excluded.custom_entity_types,
		 	custom_relationship_types = excluded.custom_relationship_types,
		 	custom_memory_types = excluded.custom_memory_types,
		 	custom_classification_schemas = excluded.custom_classification_schemas,
		 	active_classification_category = excluded.active_classification_category,
		 	updated_at = excluded.updated_at`,
		settings.ID,
		settings.ConnectionID,
		customEntityTypesJSON,
		customRelationshipTypesJSON,
		customMemoryTypesJSON,
		customClassificationSchemasJSON,
		settings.ActiveClassificationCategory,
		settings.CreatedAt,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to save settings: %w", err)
	}
	return nil
}

// Merge functions

func mergeEntityTypes(systemTypes []string, customTypes []types.CustomEntityType) []string {
	result := make([]string, len(systemTypes))
	copy(result, systemTypes)

	customIDSet := make(map[string]bool)
	for _, ct := range customTypes {
		customIDSet[ct.ID] = true
		result = append(result, ct.ID)
	}

	return result
}

func mergeRelationshipTypes(systemTypes []string, customTypes []types.CustomRelationshipType) []string {
	result := make([]string, len(systemTypes))
	copy(result, systemTypes)

	for _, ct := range customTypes {
		result = append(result, ct.ID)
	}

	return result
}

func mergeMemoryTypes(systemTypes []string, customTypes []types.CustomMemoryType) []string {
	result := make([]string, len(systemTypes))
	copy(result, systemTypes)

	for _, ct := range customTypes {
		result = append(result, ct.ID)
	}

	return result
}

func mergeClassificationSchemas(systemSchemas []types.ClassificationSchema, customSchemas []types.ClassificationSchema) []types.ClassificationSchema {
	// Build map of system categories by name for easy lookup
	systemMap := make(map[string]*types.ClassificationSchema)
	result := make([]types.ClassificationSchema, 0, len(systemSchemas)+len(customSchemas))

	// Add system schemas
	for _, schema := range systemSchemas {
		systemMap[schema.Category] = &schema
		result = append(result, schema)
	}

	// Add custom schemas, replacing system ones with same category name
	for _, schema := range customSchemas {
		if _, exists := systemMap[schema.Category]; exists {
			// Replace system schema with custom version
			for i := range result {
				if result[i].Category == schema.Category {
					result[i] = schema
					break
				}
			}
		} else {
			// New custom category
			result = append(result, schema)
		}
	}

	return result
}
