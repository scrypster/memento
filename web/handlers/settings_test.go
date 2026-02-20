package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scrypster/memento/internal/services"
	"github.com/scrypster/memento/pkg/types"
)

// setupTestDB creates an in-memory SQLite database for testing.
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)

	// Create the connection_settings table
	schema := `
	CREATE TABLE IF NOT EXISTS connection_settings (
		id TEXT PRIMARY KEY,
		connection_id TEXT NOT NULL UNIQUE,
		custom_entity_types TEXT,
		custom_relationship_types TEXT,
		custom_memory_types TEXT,
		custom_classification_schemas TEXT,
		active_classification_category TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_connection_settings_connection_id
	ON connection_settings(connection_id);

	CREATE TRIGGER IF NOT EXISTS connection_settings_updated_at
	AFTER UPDATE ON connection_settings
	FOR EACH ROW
	BEGIN
		UPDATE connection_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
	END;
	`

	_, err = db.Exec(schema)
	require.NoError(t, err)

	return db
}

// TestSettingsHandlers_GetSettings tests retrieving settings with system defaults and custom additions.
func TestSettingsHandlers_GetSettings(t *testing.T) {
	tests := []struct {
		name                 string
		connectionID         string
		setup                func(*sql.DB, string) // Setup function that prepares custom settings
		expectedStatusCode   int
		validateResponse     func(*testing.T, *types.SettingsResponse)
	}{
		{
			name:               "get settings for connection with no customizations",
			connectionID:       "test-connection-1",
			setup:              func(*sql.DB, string) {}, // No setup = no custom settings
			expectedStatusCode: http.StatusOK,
			validateResponse: func(t *testing.T, resp *types.SettingsResponse) {
				// Should have system defaults only
				assert.NotEmpty(t, resp.SystemEntityTypes)
				assert.NotEmpty(t, resp.SystemRelationshipTypes)
				assert.NotEmpty(t, resp.SystemMemoryTypes)
				assert.NotEmpty(t, resp.SystemClassificationSchemas)

				// No custom additions
				assert.Empty(t, resp.CustomEntityTypes)
				assert.Empty(t, resp.CustomRelationshipTypes)
				assert.Empty(t, resp.CustomMemoryTypes)

				// All lists should include system defaults
				assert.GreaterOrEqual(t, len(resp.AllEntityTypes), len(resp.SystemEntityTypes))
				assert.GreaterOrEqual(t, len(resp.AllRelationshipTypes), len(resp.SystemRelationshipTypes))
				assert.GreaterOrEqual(t, len(resp.AllMemoryTypes), len(resp.SystemMemoryTypes))
			},
		},
		{
			name:           "get settings for non-existent connection",
			connectionID:   "non-existent",
			setup:          func(*sql.DB, string) {}, // No setup
			expectedStatusCode: http.StatusOK,
			validateResponse: func(t *testing.T, resp *types.SettingsResponse) {
				// Should still return system defaults
				assert.NotEmpty(t, resp.SystemEntityTypes)
				assert.Empty(t, resp.CustomEntityTypes)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer func() { _ = db.Close() }()

			// Setup custom settings if needed
			tt.setup(db, tt.connectionID)

			// Create handlers
			settingsService := services.NewSettingsService(db)
			handler := NewSettingsHandlers(settingsService)

			// Create request
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/connections/%s/settings", tt.connectionID), nil)
			req.SetPathValue("id", tt.connectionID)
			w := httptest.NewRecorder()

			// Call handler
			handler.GetSettings(w, req)

			// Check status code
			assert.Equal(t, tt.expectedStatusCode, w.Code)

			// Parse response
			var resp types.SettingsResponse
			err := json.NewDecoder(w.Body).Decode(&resp)
			require.NoError(t, err)

			// Validate response
			tt.validateResponse(t, &resp)
		})
	}
}

// TestSettingsHandlers_UpdateEntityTypes tests updating custom entity types.
func TestSettingsHandlers_UpdateEntityTypes(t *testing.T) {
	tests := []struct {
		name               string
		connectionID       string
		request            types.UpdateEntityTypesRequest
		expectedStatusCode int
		validateResponse   func(*testing.T, map[string]interface{})
	}{
		{
			name:         "successfully add custom entity types",
			connectionID: "test-conn-1",
			request: types.UpdateEntityTypesRequest{
				CustomEntityTypes: []types.CustomEntityType{
					{ID: "customer", Name: "Customer", Description: "Customer entity"},
					{ID: "vendor", Name: "Vendor", Description: "Vendor entity"},
				},
			},
			expectedStatusCode: http.StatusOK,
			validateResponse: func(t *testing.T, body map[string]interface{}) {
				assert.Equal(t, "entity types updated successfully", body["message"])
				assert.NotNil(t, body["settings"])

				settings := body["settings"].(map[string]interface{})
				customTypes := settings["custom_entity_types"].([]interface{})
				assert.Len(t, customTypes, 2)
			},
		},
		{
			name:         "reject invalid entity types (missing ID)",
			connectionID: "test-conn-2",
			request: types.UpdateEntityTypesRequest{
				CustomEntityTypes: []types.CustomEntityType{
					{Name: "Customer"}, // Missing ID
				},
			},
			expectedStatusCode: http.StatusBadRequest,
			validateResponse: func(t *testing.T, body map[string]interface{}) {
				assert.NotNil(t, body["error"])
			},
		},
		{
			name:         "reject invalid entity types (missing Name)",
			connectionID: "test-conn-3",
			request: types.UpdateEntityTypesRequest{
				CustomEntityTypes: []types.CustomEntityType{
					{ID: "customer"}, // Missing Name
				},
			},
			expectedStatusCode: http.StatusBadRequest,
			validateResponse: func(t *testing.T, body map[string]interface{}) {
				assert.NotNil(t, body["error"])
			},
		},
		{
			name:         "successfully clear entity types",
			connectionID: "test-conn-4",
			request: types.UpdateEntityTypesRequest{
				CustomEntityTypes: []types.CustomEntityType{}, // Empty list
			},
			expectedStatusCode: http.StatusOK,
			validateResponse: func(t *testing.T, body map[string]interface{}) {
				assert.Equal(t, "entity types updated successfully", body["message"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer func() { _ = db.Close() }()

			settingsService := services.NewSettingsService(db)
			handler := NewSettingsHandlers(settingsService)

			// Create request body
			requestBody, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(
				http.MethodPut,
				fmt.Sprintf("/api/connections/%s/settings/entities", tt.connectionID),
				bytes.NewReader(requestBody),
			)
			req.SetPathValue("id", tt.connectionID)
			w := httptest.NewRecorder()

			// Call handler
			handler.UpdateEntityTypes(w, req)

			// Check status code
			assert.Equal(t, tt.expectedStatusCode, w.Code)

			// Parse response
			var body map[string]interface{}
			err := json.NewDecoder(w.Body).Decode(&body)
			require.NoError(t, err)

			// Validate response
			tt.validateResponse(t, body)
		})
	}
}

// TestSettingsHandlers_UpdateRelationshipTypes tests updating custom relationship types.
func TestSettingsHandlers_UpdateRelationshipTypes(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	settingsService := services.NewSettingsService(db)
	handler := NewSettingsHandlers(settingsService)

	connectionID := "test-conn-rel"
	request := types.UpdateRelationshipTypesRequest{
		CustomRelationshipTypes: []types.CustomRelationshipType{
			{
				ID:            "manages",
				Name:          "Manages",
				Description:   "Person manages another person",
				Bidirectional: false,
				Keywords:      []string{"manages", "supervises"},
			},
			{
				ID:            "partners_with",
				Name:          "Partners With",
				Description:   "Two entities partner together",
				Bidirectional: true,
				Keywords:      []string{"partner", "partnership"},
			},
		},
	}

	requestBody, _ := json.Marshal(request)
	req := httptest.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/api/connections/%s/settings/relationships", connectionID),
		bytes.NewReader(requestBody),
	)
	req.SetPathValue("id", connectionID)
	w := httptest.NewRecorder()

	handler.UpdateRelationshipTypes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "relationship types updated successfully", body["message"])
	assert.NotNil(t, body["settings"])
}

// TestSettingsHandlers_UpdateMemoryTypes tests updating custom memory types.
func TestSettingsHandlers_UpdateMemoryTypes(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	settingsService := services.NewSettingsService(db)
	handler := NewSettingsHandlers(settingsService)

	connectionID := "test-conn-mem"
	request := types.UpdateMemoryTypesRequest{
		CustomMemoryTypes: []types.CustomMemoryType{
			{
				ID:          "incident",
				Name:        "Incident",
				Description: "An incident that occurred",
				Keywords:    []string{"incident", "issue", "problem"},
			},
			{
				ID:          "feature_idea",
				Name:        "Feature Idea",
				Description: "An idea for a new feature",
				Keywords:    []string{"feature", "idea", "enhancement"},
			},
		},
	}

	requestBody, _ := json.Marshal(request)
	req := httptest.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/api/connections/%s/settings/memory-types", connectionID),
		bytes.NewReader(requestBody),
	)
	req.SetPathValue("id", connectionID)
	w := httptest.NewRecorder()

	handler.UpdateMemoryTypes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "memory types updated successfully", body["message"])
	assert.NotNil(t, body["settings"])
}

// TestSettingsHandlers_UpdateClassifications tests updating custom classification schemas.
func TestSettingsHandlers_UpdateClassifications(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	settingsService := services.NewSettingsService(db)
	handler := NewSettingsHandlers(settingsService)

	connectionID := "test-conn-class"
	request := types.UpdateClassificationsRequest{
		CustomClassificationSchemas: []types.ClassificationSchema{
			{
				Category:    "Software Development",
				Description: "Software engineering and coding",
				IsSystem:    false,
				Classifications: []types.Classification{
					{
						ID:          "critical-bug",
						Name:        "Critical Bug",
						Description: "Critical bugs affecting production",
						Keywords:    []string{"critical", "bug", "production", "urgent"},
						Icon:        "❌",
					},
					{
						ID:          "feature-enhancement",
						Name:        "Feature Enhancement",
						Description: "Enhancement to existing features",
						Keywords:    []string{"enhancement", "feature", "improve"},
						Icon:        "⭐",
					},
				},
			},
		},
	}

	requestBody, _ := json.Marshal(request)
	req := httptest.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/api/connections/%s/settings/classifications", connectionID),
		bytes.NewReader(requestBody),
	)
	req.SetPathValue("id", connectionID)
	w := httptest.NewRecorder()

	handler.UpdateClassifications(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "classification schemas updated successfully", body["message"])
	assert.NotNil(t, body["settings"])
}

// TestSettingsHandlers_MissingConnectionID tests that handlers require connection ID.
func TestSettingsHandlers_MissingConnectionID(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	settingsService := services.NewSettingsService(db)
	handler := NewSettingsHandlers(settingsService)

	tests := []struct {
		name   string
		method func(*SettingsHandlers, http.ResponseWriter, *http.Request)
	}{
		{name: "GetSettings", method: (*SettingsHandlers).GetSettings},
		{name: "UpdateEntityTypes", method: (*SettingsHandlers).UpdateEntityTypes},
		{name: "UpdateRelationshipTypes", method: (*SettingsHandlers).UpdateRelationshipTypes},
		{name: "UpdateMemoryTypes", method: (*SettingsHandlers).UpdateMemoryTypes},
		{name: "UpdateClassifications", method: (*SettingsHandlers).UpdateClassifications},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request without setting path value (simulates missing ID)
			req := httptest.NewRequest(http.MethodGet, "/api/connections//settings", nil)
			// Don't set path value - this simulates missing ID
			w := httptest.NewRecorder()

			tt.method(handler, w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var body map[string]interface{}
			err := json.NewDecoder(w.Body).Decode(&body)
			require.NoError(t, err)
			assert.NotNil(t, body["error"])
		})
	}
}

// TestSettingsHandlers_InvalidJSON tests handling of malformed JSON requests.
func TestSettingsHandlers_InvalidJSON(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	settingsService := services.NewSettingsService(db)
	handler := NewSettingsHandlers(settingsService)

	connectionID := "test-conn-invalid"
	req := httptest.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/api/connections/%s/settings/entities", connectionID),
		bytes.NewReader([]byte("invalid json")),
	)
	w := httptest.NewRecorder()

	handler.UpdateEntityTypes(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.NotNil(t, body["error"])
}

// TestSettingsHandlers_PersistenceAcrossRequests tests that settings persist across multiple requests.
func TestSettingsHandlers_PersistenceAcrossRequests(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	settingsService := services.NewSettingsService(db)
	handler := NewSettingsHandlers(settingsService)

	connectionID := "test-conn-persist"

	// Step 1: Add entity types
	entityRequest := types.UpdateEntityTypesRequest{
		CustomEntityTypes: []types.CustomEntityType{
			{ID: "customer", Name: "Customer"},
		},
	}
	entityBody, _ := json.Marshal(entityRequest)
	req1 := httptest.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/api/connections/%s/settings/entities", connectionID),
		bytes.NewReader(entityBody),
	)
	req1.SetPathValue("id", connectionID)
	w1 := httptest.NewRecorder()
	handler.UpdateEntityTypes(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Step 2: Add relationship types
	relRequest := types.UpdateRelationshipTypesRequest{
		CustomRelationshipTypes: []types.CustomRelationshipType{
			{ID: "manages", Name: "Manages"},
		},
	}
	relBody, _ := json.Marshal(relRequest)
	req2 := httptest.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/api/connections/%s/settings/relationships", connectionID),
		bytes.NewReader(relBody),
	)
	req2.SetPathValue("id", connectionID)
	w2 := httptest.NewRecorder()
	handler.UpdateRelationshipTypes(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	// Step 3: Get settings and verify both are present
	req3 := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/api/connections/%s/settings", connectionID),
		nil,
	)
	req3.SetPathValue("id", connectionID)
	w3 := httptest.NewRecorder()
	handler.GetSettings(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code)

	var resp types.SettingsResponse
	err := json.NewDecoder(w3.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify both custom types are present
	assert.Len(t, resp.CustomEntityTypes, 1)
	assert.Len(t, resp.CustomRelationshipTypes, 1)
	assert.Equal(t, "customer", resp.CustomEntityTypes[0].ID)
	assert.Equal(t, "manages", resp.CustomRelationshipTypes[0].ID)
}
