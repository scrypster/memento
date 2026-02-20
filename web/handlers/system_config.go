package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
)

// SystemConfigHandlers contains HTTP handlers for managing system configuration.
type SystemConfigHandlers struct {
	configPath string
	mu         sync.RWMutex
}

// SystemConfig represents the structure of categories.json.
type SystemConfig struct {
	EntityTypes struct {
		Default []EntityType `json:"default"`
		Custom  []EntityType `json:"custom"`
	} `json:"entity_types"`
	RelationshipTypes struct {
		Default []RelationshipType `json:"default"`
		Custom  []RelationshipType `json:"custom"`
	} `json:"relationship_types"`
	CategoryTemplates map[string]interface{}   `json:"category_templates"`
	SystemDefaults    map[string]interface{}   `json:"system_defaults"`
	LegacyCategories  map[string]interface{}   `json:"legacy_categories"`
	ContextLabels     map[string]interface{}   `json:"context_labels"`
	ProjectHierarchy  []map[string]interface{} `json:"project_hierarchy"`
	Settings          map[string]interface{}   `json:"settings"`
}

// EntityType represents a custom entity type.
type EntityType struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

// RelationshipType represents a custom relationship type.
type RelationshipType struct {
	Type          string `json:"type"`
	Description   string `json:"description"`
	Bidirectional bool   `json:"bidirectional"`
	Category      string `json:"category"`
}

// AddEntityTypeRequest represents the request body for adding a custom entity type.
type AddEntityTypeRequest struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

// AddRelationshipTypeRequest represents the request body for adding a custom relationship type.
type AddRelationshipTypeRequest struct {
	Type          string `json:"type"`
	Description   string `json:"description"`
	Bidirectional bool   `json:"bidirectional"`
	Category      string `json:"category"`
}

// NewSystemConfigHandlers creates a new SystemConfigHandlers instance.
func NewSystemConfigHandlers(configPath string) *SystemConfigHandlers {
	return &SystemConfigHandlers{
		configPath: configPath,
	}
}

// loadConfig loads the system configuration from the JSON file.
func (h *SystemConfigHandlers) loadConfig() (*SystemConfig, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	data, err := os.ReadFile(h.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config SystemConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// saveConfig saves the system configuration to the JSON file.
func (h *SystemConfigHandlers) saveConfig(config *SystemConfig) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(h.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetSystemConfig handles GET /api/system-config - returns the system configuration.
func (h *SystemConfigHandlers) GetSystemConfig(w http.ResponseWriter, r *http.Request) {
	config, err := h.loadConfig()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load system config", err)
		return
	}

	respondJSON(w, http.StatusOK, config)
}

// AddCustomEntityType handles POST /api/system-config/entity-types - adds a custom entity type.
func (h *SystemConfigHandlers) AddCustomEntityType(w http.ResponseWriter, r *http.Request) {
	var req AddEntityTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	// Validate required fields
	if req.Type == "" {
		respondError(w, http.StatusBadRequest, "type is required", nil)
		return
	}
	if req.Description == "" {
		respondError(w, http.StatusBadRequest, "description is required", nil)
		return
	}
	if req.Icon == "" {
		respondError(w, http.StatusBadRequest, "icon is required", nil)
		return
	}

	// Load current config
	config, err := h.loadConfig()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load system config", err)
		return
	}

	// Check if type already exists in default types
	for _, et := range config.EntityTypes.Default {
		if et.Type == req.Type {
			respondError(w, http.StatusBadRequest, "entity type already exists in default types", nil)
			return
		}
	}

	// Check if type already exists in custom types
	for _, et := range config.EntityTypes.Custom {
		if et.Type == req.Type {
			respondError(w, http.StatusBadRequest, "entity type already exists in custom types", nil)
			return
		}
	}

	// Add to custom types
	newEntityType := EntityType(req)
	config.EntityTypes.Custom = append(config.EntityTypes.Custom, newEntityType)

	// Save config
	if err := h.saveConfig(config); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save system config", err)
		return
	}

	respondJSON(w, http.StatusCreated, config)
}

// RemoveCustomEntityType handles DELETE /api/system-config/entity-types/{type} - removes a custom entity type.
func (h *SystemConfigHandlers) RemoveCustomEntityType(w http.ResponseWriter, r *http.Request) {
	entityType := extractID(r, "type")
	if entityType == "" {
		respondError(w, http.StatusBadRequest, "entity type is required", nil)
		return
	}

	// Load current config
	config, err := h.loadConfig()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load system config", err)
		return
	}

	// Find and remove from custom types
	found := false
	newCustomTypes := make([]EntityType, 0)
	for _, et := range config.EntityTypes.Custom {
		if et.Type == entityType {
			found = true
			continue
		}
		newCustomTypes = append(newCustomTypes, et)
	}

	if !found {
		respondError(w, http.StatusNotFound, "entity type not found in custom types", nil)
		return
	}

	config.EntityTypes.Custom = newCustomTypes

	// Save config
	if err := h.saveConfig(config); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save system config", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "entity type removed successfully",
		"config":  config,
	})
}

// AddCustomRelationshipType handles POST /api/system-config/relationship-types - adds a custom relationship type.
func (h *SystemConfigHandlers) AddCustomRelationshipType(w http.ResponseWriter, r *http.Request) {
	var req AddRelationshipTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	// Validate required fields
	if req.Type == "" {
		respondError(w, http.StatusBadRequest, "type is required", nil)
		return
	}
	if req.Description == "" {
		respondError(w, http.StatusBadRequest, "description is required", nil)
		return
	}
	if req.Category == "" {
		respondError(w, http.StatusBadRequest, "category is required", nil)
		return
	}

	// Load current config
	config, err := h.loadConfig()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load system config", err)
		return
	}

	// Check if type already exists in default types
	for _, rt := range config.RelationshipTypes.Default {
		if rt.Type == req.Type {
			respondError(w, http.StatusBadRequest, "relationship type already exists in default types", nil)
			return
		}
	}

	// Check if type already exists in custom types
	for _, rt := range config.RelationshipTypes.Custom {
		if rt.Type == req.Type {
			respondError(w, http.StatusBadRequest, "relationship type already exists in custom types", nil)
			return
		}
	}

	// Add to custom types
	newRelationshipType := RelationshipType(req)
	config.RelationshipTypes.Custom = append(config.RelationshipTypes.Custom, newRelationshipType)

	// Save config
	if err := h.saveConfig(config); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save system config", err)
		return
	}

	respondJSON(w, http.StatusCreated, config)
}

// RemoveCustomRelationshipType handles DELETE /api/system-config/relationship-types/{type} - removes a custom relationship type.
func (h *SystemConfigHandlers) RemoveCustomRelationshipType(w http.ResponseWriter, r *http.Request) {
	relationshipType := extractID(r, "type")
	if relationshipType == "" {
		respondError(w, http.StatusBadRequest, "relationship type is required", nil)
		return
	}

	// Load current config
	config, err := h.loadConfig()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load system config", err)
		return
	}

	// Find and remove from custom types
	found := false
	newCustomTypes := make([]RelationshipType, 0)
	for _, rt := range config.RelationshipTypes.Custom {
		if rt.Type == relationshipType {
			found = true
			continue
		}
		newCustomTypes = append(newCustomTypes, rt)
	}

	if !found {
		respondError(w, http.StatusNotFound, "relationship type not found in custom types", nil)
		return
	}

	config.RelationshipTypes.Custom = newCustomTypes

	// Save config
	if err := h.saveConfig(config); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save system config", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "relationship type removed successfully",
		"config":  config,
	})
}
