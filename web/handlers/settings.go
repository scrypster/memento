package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/scrypster/memento/internal/services"
	"github.com/scrypster/memento/pkg/types"
)

// SettingsHandlers contains HTTP handlers for connection settings management.
type SettingsHandlers struct {
	settingsService *services.SettingsService
}

// NewSettingsHandlers creates a new SettingsHandlers instance.
func NewSettingsHandlers(settingsService *services.SettingsService) *SettingsHandlers {
	return &SettingsHandlers{
		settingsService: settingsService,
	}
}

// GetSettings handles GET /api/connections/{id}/settings
// Returns merged view of system defaults + custom settings for the connection.
func (h *SettingsHandlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	connectionID := extractID(r, "id")
	if connectionID == "" {
		respondError(w, http.StatusBadRequest, "connection ID is required", nil)
		return
	}

	settings, err := h.settingsService.GetSettings(connectionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get settings", err)
		return
	}

	respondJSON(w, http.StatusOK, settings)
}

// UpdateEntityTypes handles PUT /api/connections/{id}/settings/entities
// Updates custom entity types for the connection.
func (h *SettingsHandlers) UpdateEntityTypes(w http.ResponseWriter, r *http.Request) {
	connectionID := extractID(r, "id")
	if connectionID == "" {
		respondError(w, http.StatusBadRequest, "connection ID is required", nil)
		return
	}

	var req types.UpdateEntityTypesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	// Validate entity types
	if err := validateEntityTypes(req.CustomEntityTypes); err != nil {
		respondError(w, http.StatusBadRequest, "invalid entity types", err)
		return
	}

	// Save custom entity types
	if err := h.settingsService.SaveEntityTypes(connectionID, req.CustomEntityTypes); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save entity types", err)
		return
	}

	// Return updated settings
	settings, err := h.settingsService.GetSettings(connectionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get updated settings", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "entity types updated successfully",
		"settings": settings,
	})
}

// UpdateRelationshipTypes handles PUT /api/connections/{id}/settings/relationships
// Updates custom relationship types for the connection.
func (h *SettingsHandlers) UpdateRelationshipTypes(w http.ResponseWriter, r *http.Request) {
	connectionID := extractID(r, "id")
	if connectionID == "" {
		respondError(w, http.StatusBadRequest, "connection ID is required", nil)
		return
	}

	var req types.UpdateRelationshipTypesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	// Validate relationship types
	if err := validateRelationshipTypes(req.CustomRelationshipTypes); err != nil {
		respondError(w, http.StatusBadRequest, "invalid relationship types", err)
		return
	}

	// Save custom relationship types
	if err := h.settingsService.SaveRelationshipTypes(connectionID, req.CustomRelationshipTypes); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save relationship types", err)
		return
	}

	// Return updated settings
	settings, err := h.settingsService.GetSettings(connectionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get updated settings", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "relationship types updated successfully",
		"settings": settings,
	})
}

// UpdateMemoryTypes handles PUT /api/connections/{id}/settings/memory-types
// Updates custom memory types for the connection.
func (h *SettingsHandlers) UpdateMemoryTypes(w http.ResponseWriter, r *http.Request) {
	connectionID := extractID(r, "id")
	if connectionID == "" {
		respondError(w, http.StatusBadRequest, "connection ID is required", nil)
		return
	}

	var req types.UpdateMemoryTypesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	// Validate memory types
	if err := validateMemoryTypes(req.CustomMemoryTypes); err != nil {
		respondError(w, http.StatusBadRequest, "invalid memory types", err)
		return
	}

	// Save custom memory types
	if err := h.settingsService.SaveMemoryTypes(connectionID, req.CustomMemoryTypes); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save memory types", err)
		return
	}

	// Return updated settings
	settings, err := h.settingsService.GetSettings(connectionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get updated settings", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "memory types updated successfully",
		"settings": settings,
	})
}

// UpdateClassifications handles PUT /api/connections/{id}/settings/classifications
// Updates custom classification schemas for the connection.
func (h *SettingsHandlers) UpdateClassifications(w http.ResponseWriter, r *http.Request) {
	connectionID := extractID(r, "id")
	if connectionID == "" {
		respondError(w, http.StatusBadRequest, "connection ID is required", nil)
		return
	}

	var req types.UpdateClassificationsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	// Validate classification schemas
	if err := validateClassificationSchemas(req.CustomClassificationSchemas); err != nil {
		respondError(w, http.StatusBadRequest, "invalid classification schemas", err)
		return
	}

	// Save custom classification schemas
	if err := h.settingsService.SaveClassificationSchemas(connectionID, req.CustomClassificationSchemas); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save classification schemas", err)
		return
	}

	// Return updated settings
	settings, err := h.settingsService.GetSettings(connectionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get updated settings", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "classification schemas updated successfully",
		"settings": settings,
	})
}

// UpdateActiveCategory handles PUT /api/connections/{id}/settings/active-category
// Sets the active classification category for the connection.
func (h *SettingsHandlers) UpdateActiveCategory(w http.ResponseWriter, r *http.Request) {
	connectionID := extractID(r, "id")
	if connectionID == "" {
		respondError(w, http.StatusBadRequest, "connection ID is required", nil)
		return
	}

	var req types.UpdateActiveCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	if err := h.settingsService.SaveActiveClassificationCategory(connectionID, req.ActiveClassificationCategory); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save active category", err)
		return
	}

	settings, err := h.settingsService.GetSettings(connectionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get updated settings", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "active category updated successfully",
		"settings": settings,
	})
}

// Validation helper functions

func validateEntityTypes(customTypes []types.CustomEntityType) error {
	for _, t := range customTypes {
		if t.ID == "" {
			return NewValidationError("entity type ID is required")
		}
		if t.Name == "" {
			return NewValidationError("entity type name is required")
		}
	}
	return nil
}

func validateRelationshipTypes(customTypes []types.CustomRelationshipType) error {
	for _, t := range customTypes {
		if t.ID == "" {
			return NewValidationError("relationship type ID is required")
		}
		if t.Name == "" {
			return NewValidationError("relationship type name is required")
		}
	}
	return nil
}

func validateMemoryTypes(customTypes []types.CustomMemoryType) error {
	for _, t := range customTypes {
		if t.ID == "" {
			return NewValidationError("memory type ID is required")
		}
		if t.Name == "" {
			return NewValidationError("memory type name is required")
		}
	}
	return nil
}

func validateClassificationSchemas(schemas []types.ClassificationSchema) error {
	for _, schema := range schemas {
		if schema.Category == "" {
			return NewValidationError("classification category name is required")
		}
		if len(schema.Classifications) == 0 {
			return NewValidationError("classification category must have at least one classification")
		}
		for _, classification := range schema.Classifications {
			if classification.ID == "" {
				return NewValidationError("classification ID is required")
			}
			if classification.Name == "" {
				return NewValidationError("classification name is required")
			}
		}
	}
	return nil
}

// ValidationError represents a validation error
type ValidationError struct {
	message string
}

// NewValidationError creates a new ValidationError
func NewValidationError(message string) error {
	return &ValidationError{message: message}
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	return e.message
}
