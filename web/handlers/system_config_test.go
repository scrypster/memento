package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// createTestConfigFile creates a temporary test config file.
func createTestConfigFile(t *testing.T) string {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "categories.json")

	// Create test config
	config := SystemConfig{}
	config.EntityTypes.Default = []EntityType{
		{Type: "person", Description: "Individual people", Icon: "👤"},
		{Type: "organization", Description: "Companies, institutions", Icon: "🏢"},
	}
	config.EntityTypes.Custom = []EntityType{}
	config.RelationshipTypes.Default = []RelationshipType{
		{Type: "married_to", Description: "Spouse relationship", Bidirectional: true, Category: "family"},
		{Type: "works_on", Description: "Person works on Project", Bidirectional: false, Category: "professional"},
	}
	config.RelationshipTypes.Custom = []RelationshipType{}
	config.CategoryTemplates = make(map[string]interface{})
	config.SystemDefaults = make(map[string]interface{})
	config.LegacyCategories = make(map[string]interface{})
	config.ContextLabels = make(map[string]interface{})
	config.ProjectHierarchy = []map[string]interface{}{}
	config.Settings = make(map[string]interface{})

	// Write to file
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	return configPath
}

func TestGetSystemConfig(t *testing.T) {
	configPath := createTestConfigFile(t)
	handler := NewSystemConfigHandlers(configPath)

	req := httptest.NewRequest(http.MethodGet, "/api/system-config", nil)
	w := httptest.NewRecorder()

	handler.GetSystemConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var config SystemConfig
	if err := json.NewDecoder(w.Body).Decode(&config); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify default types exist
	if len(config.EntityTypes.Default) != 2 {
		t.Errorf("expected 2 default entity types, got %d", len(config.EntityTypes.Default))
	}
	if len(config.RelationshipTypes.Default) != 2 {
		t.Errorf("expected 2 default relationship types, got %d", len(config.RelationshipTypes.Default))
	}
}

func TestAddCustomEntityType(t *testing.T) {
	configPath := createTestConfigFile(t)
	handler := NewSystemConfigHandlers(configPath)

	tests := []struct {
		name       string
		request    AddEntityTypeRequest
		wantStatus int
		wantError  bool
	}{
		{
			name: "valid entity type",
			request: AddEntityTypeRequest{
				Type:        "service",
				Description: "Backend services",
				Icon:        "⚙️",
			},
			wantStatus: http.StatusCreated,
			wantError:  false,
		},
		{
			name: "missing type",
			request: AddEntityTypeRequest{
				Description: "Backend services",
				Icon:        "⚙️",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "missing description",
			request: AddEntityTypeRequest{
				Type: "service",
				Icon: "⚙️",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "missing icon",
			request: AddEntityTypeRequest{
				Type:        "service",
				Description: "Backend services",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "duplicate default type",
			request: AddEntityTypeRequest{
				Type:        "person",
				Description: "Duplicate type",
				Icon:        "🧑",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/system-config/entity-types", bytes.NewReader(body))
			w := httptest.NewRecorder()

			handler.AddCustomEntityType(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			if !tt.wantError {
				var config SystemConfig
				if err := json.NewDecoder(w.Body).Decode(&config); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				// Verify custom type was added
				found := false
				for _, et := range config.EntityTypes.Custom {
					if et.Type == tt.request.Type {
						found = true
						if et.Description != tt.request.Description {
							t.Errorf("expected description %s, got %s", tt.request.Description, et.Description)
						}
						if et.Icon != tt.request.Icon {
							t.Errorf("expected icon %s, got %s", tt.request.Icon, et.Icon)
						}
						break
					}
				}
				if !found {
					t.Error("custom entity type not found in response")
				}
			}
		})
	}
}

func TestRemoveCustomEntityType(t *testing.T) {
	configPath := createTestConfigFile(t)
	handler := NewSystemConfigHandlers(configPath)

	// First add a custom entity type
	addReq := AddEntityTypeRequest{
		Type:        "service",
		Description: "Backend services",
		Icon:        "⚙️",
	}
	body, _ := json.Marshal(addReq)
	req := httptest.NewRequest(http.MethodPost, "/api/system-config/entity-types", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.AddCustomEntityType(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to add custom entity type: status %d", w.Code)
	}

	// Now remove it
	req = httptest.NewRequest(http.MethodDelete, "/api/system-config/entity-types/service", nil)
	req.SetPathValue("type", "service")
	w = httptest.NewRecorder()

	handler.RemoveCustomEntityType(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["message"] != "entity type removed successfully" {
		t.Errorf("unexpected response message: %v", response["message"])
	}

	// Verify it was removed
	configData := response["config"].(map[string]interface{})
	entityTypes := configData["entity_types"].(map[string]interface{})
	customTypes := entityTypes["custom"].([]interface{})

	if len(customTypes) != 0 {
		t.Errorf("expected 0 custom types after removal, got %d", len(customTypes))
	}
}

func TestRemoveCustomEntityType_NotFound(t *testing.T) {
	configPath := createTestConfigFile(t)
	handler := NewSystemConfigHandlers(configPath)

	req := httptest.NewRequest(http.MethodDelete, "/api/system-config/entity-types/nonexistent", nil)
	req.SetPathValue("type", "nonexistent")
	w := httptest.NewRecorder()

	handler.RemoveCustomEntityType(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestAddCustomRelationshipType(t *testing.T) {
	configPath := createTestConfigFile(t)
	handler := NewSystemConfigHandlers(configPath)

	tests := []struct {
		name       string
		request    AddRelationshipTypeRequest
		wantStatus int
		wantError  bool
	}{
		{
			name: "valid relationship type",
			request: AddRelationshipTypeRequest{
				Type:          "manages",
				Description:   "Management relationship",
				Bidirectional: false,
				Category:      "professional",
			},
			wantStatus: http.StatusCreated,
			wantError:  false,
		},
		{
			name: "missing type",
			request: AddRelationshipTypeRequest{
				Description:   "Management relationship",
				Bidirectional: false,
				Category:      "professional",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "missing description",
			request: AddRelationshipTypeRequest{
				Type:          "manages",
				Bidirectional: false,
				Category:      "professional",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "missing category",
			request: AddRelationshipTypeRequest{
				Type:          "manages",
				Description:   "Management relationship",
				Bidirectional: false,
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "duplicate default type",
			request: AddRelationshipTypeRequest{
				Type:          "married_to",
				Description:   "Duplicate type",
				Bidirectional: true,
				Category:      "family",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/system-config/relationship-types", bytes.NewReader(body))
			w := httptest.NewRecorder()

			handler.AddCustomRelationshipType(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			if !tt.wantError {
				var config SystemConfig
				if err := json.NewDecoder(w.Body).Decode(&config); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				// Verify custom type was added
				found := false
				for _, rt := range config.RelationshipTypes.Custom {
					if rt.Type == tt.request.Type {
						found = true
						if rt.Description != tt.request.Description {
							t.Errorf("expected description %s, got %s", tt.request.Description, rt.Description)
						}
						if rt.Bidirectional != tt.request.Bidirectional {
							t.Errorf("expected bidirectional %v, got %v", tt.request.Bidirectional, rt.Bidirectional)
						}
						if rt.Category != tt.request.Category {
							t.Errorf("expected category %s, got %s", tt.request.Category, rt.Category)
						}
						break
					}
				}
				if !found {
					t.Error("custom relationship type not found in response")
				}
			}
		})
	}
}

func TestRemoveCustomRelationshipType(t *testing.T) {
	configPath := createTestConfigFile(t)
	handler := NewSystemConfigHandlers(configPath)

	// First add a custom relationship type
	addReq := AddRelationshipTypeRequest{
		Type:          "manages",
		Description:   "Management relationship",
		Bidirectional: false,
		Category:      "professional",
	}
	body, _ := json.Marshal(addReq)
	req := httptest.NewRequest(http.MethodPost, "/api/system-config/relationship-types", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.AddCustomRelationshipType(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to add custom relationship type: status %d", w.Code)
	}

	// Now remove it
	req = httptest.NewRequest(http.MethodDelete, "/api/system-config/relationship-types/manages", nil)
	req.SetPathValue("type", "manages")
	w = httptest.NewRecorder()

	handler.RemoveCustomRelationshipType(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["message"] != "relationship type removed successfully" {
		t.Errorf("unexpected response message: %v", response["message"])
	}

	// Verify it was removed
	configData := response["config"].(map[string]interface{})
	relationshipTypes := configData["relationship_types"].(map[string]interface{})
	customTypes := relationshipTypes["custom"].([]interface{})

	if len(customTypes) != 0 {
		t.Errorf("expected 0 custom types after removal, got %d", len(customTypes))
	}
}

func TestRemoveCustomRelationshipType_NotFound(t *testing.T) {
	configPath := createTestConfigFile(t)
	handler := NewSystemConfigHandlers(configPath)

	req := httptest.NewRequest(http.MethodDelete, "/api/system-config/relationship-types/nonexistent", nil)
	req.SetPathValue("type", "nonexistent")
	w := httptest.NewRecorder()

	handler.RemoveCustomRelationshipType(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestDuplicateCustomEntityType(t *testing.T) {
	configPath := createTestConfigFile(t)
	handler := NewSystemConfigHandlers(configPath)

	// Add a custom entity type
	addReq := AddEntityTypeRequest{
		Type:        "service",
		Description: "Backend services",
		Icon:        "⚙️",
	}
	body, _ := json.Marshal(addReq)
	req := httptest.NewRequest(http.MethodPost, "/api/system-config/entity-types", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.AddCustomEntityType(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to add custom entity type: status %d", w.Code)
	}

	// Try to add the same type again
	body, _ = json.Marshal(addReq)
	req = httptest.NewRequest(http.MethodPost, "/api/system-config/entity-types", bytes.NewReader(body))
	w = httptest.NewRecorder()
	handler.AddCustomEntityType(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestDuplicateCustomRelationshipType(t *testing.T) {
	configPath := createTestConfigFile(t)
	handler := NewSystemConfigHandlers(configPath)

	// Add a custom relationship type
	addReq := AddRelationshipTypeRequest{
		Type:          "manages",
		Description:   "Management relationship",
		Bidirectional: false,
		Category:      "professional",
	}
	body, _ := json.Marshal(addReq)
	req := httptest.NewRequest(http.MethodPost, "/api/system-config/relationship-types", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.AddCustomRelationshipType(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to add custom relationship type: status %d", w.Code)
	}

	// Try to add the same type again
	body, _ = json.Marshal(addReq)
	req = httptest.NewRequest(http.MethodPost, "/api/system-config/relationship-types", bytes.NewReader(body))
	w = httptest.NewRecorder()
	handler.AddCustomRelationshipType(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
