package services

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// TestSettingsService_GetSettings_NoCustomizations tests that system defaults are returned
// when no custom settings exist for a connection.
func TestSettingsService_GetSettings_NoCustomizations(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	settings, err := service.GetSettings("test-connection-1")

	require.NoError(t, err)
	assert.NotNil(t, settings)

	// Verify system defaults are present
	assert.NotEmpty(t, settings.SystemEntityTypes)
	assert.NotEmpty(t, settings.SystemRelationshipTypes)
	assert.NotEmpty(t, settings.SystemMemoryTypes)
	assert.NotEmpty(t, settings.SystemClassificationSchemas)

	// Verify no custom types exist
	assert.Empty(t, settings.CustomEntityTypes)
	assert.Empty(t, settings.CustomRelationshipTypes)
	assert.Empty(t, settings.CustomMemoryTypes)
	assert.Empty(t, settings.CustomClassificationSchemas)

	// Verify merged lists contain at least system defaults
	assert.GreaterOrEqual(t, len(settings.AllEntityTypes), len(settings.SystemEntityTypes))
	assert.GreaterOrEqual(t, len(settings.AllRelationshipTypes), len(settings.SystemRelationshipTypes))
	assert.GreaterOrEqual(t, len(settings.AllMemoryTypes), len(settings.SystemMemoryTypes))
	assert.GreaterOrEqual(t, len(settings.AllClassificationSchemas), len(settings.SystemClassificationSchemas))
}

// TestSettingsService_GetSettings_NonExistentConnection tests that system defaults are returned
// for a connection that has never been saved to the database.
func TestSettingsService_GetSettings_NonExistentConnection(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	settings, err := service.GetSettings("non-existent-connection")

	require.NoError(t, err)
	assert.NotNil(t, settings)

	// Should return system defaults even for non-existent connection
	assert.NotEmpty(t, settings.SystemEntityTypes)
	assert.Empty(t, settings.CustomEntityTypes)
}

// TestSettingsService_SaveEntityTypes_NewConnection tests saving custom entity types
// to a new connection.
func TestSettingsService_SaveEntityTypes_NewConnection(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	connectionID := "test-conn-save-entity"

	// Define custom entity types
	customTypes := []types.CustomEntityType{
		{ID: "customer", Name: "Customer", Description: "A customer entity"},
		{ID: "vendor", Name: "Vendor", Description: "A vendor entity"},
	}

	// Save custom entity types
	err := service.SaveEntityTypes(connectionID, customTypes)
	require.NoError(t, err)

	// Retrieve settings and verify they were saved
	settings, err := service.GetSettings(connectionID)
	require.NoError(t, err)

	assert.Len(t, settings.CustomEntityTypes, 2)
	assert.Equal(t, "customer", settings.CustomEntityTypes[0].ID)
	assert.Equal(t, "Customer", settings.CustomEntityTypes[0].Name)
	assert.Equal(t, "vendor", settings.CustomEntityTypes[1].ID)
	assert.Equal(t, "Vendor", settings.CustomEntityTypes[1].Name)

	// Verify merged list includes custom types
	assert.Greater(t, len(settings.AllEntityTypes), len(settings.SystemEntityTypes))
}

// TestSettingsService_SaveEntityTypes_Overwrite tests that saving entity types overwrites
// previously saved entity types.
func TestSettingsService_SaveEntityTypes_Overwrite(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	connectionID := "test-conn-overwrite"

	// Save initial entity types
	initialTypes := []types.CustomEntityType{
		{ID: "customer", Name: "Customer"},
	}
	err := service.SaveEntityTypes(connectionID, initialTypes)
	require.NoError(t, err)

	// Verify initial save
	settings, err := service.GetSettings(connectionID)
	require.NoError(t, err)
	assert.Len(t, settings.CustomEntityTypes, 1)
	assert.Equal(t, "customer", settings.CustomEntityTypes[0].ID)

	// Overwrite with new entity types
	newTypes := []types.CustomEntityType{
		{ID: "product", Name: "Product"},
		{ID: "asset", Name: "Asset"},
		{ID: "contract", Name: "Contract"},
	}
	err = service.SaveEntityTypes(connectionID, newTypes)
	require.NoError(t, err)

	// Verify overwrite
	settings, err = service.GetSettings(connectionID)
	require.NoError(t, err)
	assert.Len(t, settings.CustomEntityTypes, 3)
	assert.Equal(t, "product", settings.CustomEntityTypes[0].ID)
	assert.Equal(t, "asset", settings.CustomEntityTypes[1].ID)
	assert.Equal(t, "contract", settings.CustomEntityTypes[2].ID)
}

// TestSettingsService_SaveEntityTypes_ClearTypes tests that entity types can be cleared
// by saving an empty slice.
func TestSettingsService_SaveEntityTypes_ClearTypes(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	connectionID := "test-conn-clear"

	// Save some entity types
	customTypes := []types.CustomEntityType{
		{ID: "customer", Name: "Customer"},
	}
	err := service.SaveEntityTypes(connectionID, customTypes)
	require.NoError(t, err)

	// Verify they exist
	settings, err := service.GetSettings(connectionID)
	require.NoError(t, err)
	assert.Len(t, settings.CustomEntityTypes, 1)

	// Clear by saving empty slice
	err = service.SaveEntityTypes(connectionID, []types.CustomEntityType{})
	require.NoError(t, err)

	// Verify they're cleared
	settings, err = service.GetSettings(connectionID)
	require.NoError(t, err)
	assert.Empty(t, settings.CustomEntityTypes)
}

// TestSettingsService_SaveRelationshipTypes tests saving and retrieving custom relationship types.
func TestSettingsService_SaveRelationshipTypes(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	connectionID := "test-conn-rel"

	customTypes := []types.CustomRelationshipType{
		{
			ID:            "manages",
			Name:          "Manages",
			Description:   "A person manages another entity",
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
	}

	err := service.SaveRelationshipTypes(connectionID, customTypes)
	require.NoError(t, err)

	// Verify settings were saved
	settings, err := service.GetSettings(connectionID)
	require.NoError(t, err)

	assert.Len(t, settings.CustomRelationshipTypes, 2)
	assert.Equal(t, "manages", settings.CustomRelationshipTypes[0].ID)
	assert.False(t, settings.CustomRelationshipTypes[0].Bidirectional)
	assert.Equal(t, "partners_with", settings.CustomRelationshipTypes[1].ID)
	assert.True(t, settings.CustomRelationshipTypes[1].Bidirectional)
}

// TestSettingsService_SaveMemoryTypes tests saving and retrieving custom memory types.
func TestSettingsService_SaveMemoryTypes(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	connectionID := "test-conn-mem"

	customTypes := []types.CustomMemoryType{
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
	}

	err := service.SaveMemoryTypes(connectionID, customTypes)
	require.NoError(t, err)

	// Verify settings were saved
	settings, err := service.GetSettings(connectionID)
	require.NoError(t, err)

	assert.Len(t, settings.CustomMemoryTypes, 2)
	assert.Equal(t, "incident", settings.CustomMemoryTypes[0].ID)
	assert.Equal(t, "Feature Idea", settings.CustomMemoryTypes[1].Name)
}

// TestSettingsService_SaveClassificationSchemas tests saving and retrieving custom classification schemas.
func TestSettingsService_SaveClassificationSchemas(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	connectionID := "test-conn-class"

	customSchemas := []types.ClassificationSchema{
		{
			Category:    "Custom Category",
			Description: "A custom classification category",
			IsSystem:    false,
			Classifications: []types.Classification{
				{
					ID:          "custom-class-1",
					Name:        "Custom Classification 1",
					Description: "First custom classification",
					Keywords:    []string{"custom", "one"},
					Icon:        "🔧",
				},
			},
		},
	}

	err := service.SaveClassificationSchemas(connectionID, customSchemas)
	require.NoError(t, err)

	// Verify settings were saved
	settings, err := service.GetSettings(connectionID)
	require.NoError(t, err)

	assert.Len(t, settings.CustomClassificationSchemas, 1)
	assert.Equal(t, "Custom Category", settings.CustomClassificationSchemas[0].Category)
	assert.False(t, settings.CustomClassificationSchemas[0].IsSystem)
}

// TestSettingsService_SaveActiveClassificationCategory tests saving and retrieving
// the active classification category.
func TestSettingsService_SaveActiveClassificationCategory(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	connectionID := "test-conn-active"

	// Save an active category
	err := service.SaveActiveClassificationCategory(connectionID, "Software Development")
	require.NoError(t, err)

	// Verify it was saved
	settings, err := service.GetSettings(connectionID)
	require.NoError(t, err)
	assert.Equal(t, "Software Development", settings.ActiveClassificationCategory)

	// Update to a different category
	err = service.SaveActiveClassificationCategory(connectionID, "Business & Operations")
	require.NoError(t, err)

	// Verify it was updated
	settings, err = service.GetSettings(connectionID)
	require.NoError(t, err)
	assert.Equal(t, "Business & Operations", settings.ActiveClassificationCategory)
}

// TestSettingsService_MergeEntityTypes tests the mergeEntityTypes function
// to ensure system and custom types are properly merged.
func TestSettingsService_MergeEntityTypes(t *testing.T) {
	systemTypes := []string{"person", "organization", "project"}
	customTypes := []types.CustomEntityType{
		{ID: "customer", Name: "Customer"},
		{ID: "vendor", Name: "Vendor"},
	}

	merged := mergeEntityTypes(systemTypes, customTypes)

	// Should contain all system types plus custom IDs
	assert.Len(t, merged, 5)
	assert.Contains(t, merged, "person")
	assert.Contains(t, merged, "organization")
	assert.Contains(t, merged, "project")
	assert.Contains(t, merged, "customer")
	assert.Contains(t, merged, "vendor")
}

// TestSettingsService_MergeRelationshipTypes tests the mergeRelationshipTypes function.
func TestSettingsService_MergeRelationshipTypes(t *testing.T) {
	systemTypes := []string{"knows", "works_with"}
	customTypes := []types.CustomRelationshipType{
		{ID: "manages", Name: "Manages"},
		{ID: "supervises", Name: "Supervises"},
	}

	merged := mergeRelationshipTypes(systemTypes, customTypes)

	assert.Len(t, merged, 4)
	assert.Contains(t, merged, "knows")
	assert.Contains(t, merged, "works_with")
	assert.Contains(t, merged, "manages")
	assert.Contains(t, merged, "supervises")
}

// TestSettingsService_MergeMemoryTypes tests the mergeMemoryTypes function.
func TestSettingsService_MergeMemoryTypes(t *testing.T) {
	systemTypes := []string{"decision", "process", "concept"}
	customTypes := []types.CustomMemoryType{
		{ID: "incident", Name: "Incident"},
	}

	merged := mergeMemoryTypes(systemTypes, customTypes)

	assert.Len(t, merged, 4)
	assert.Contains(t, merged, "decision")
	assert.Contains(t, merged, "process")
	assert.Contains(t, merged, "concept")
	assert.Contains(t, merged, "incident")
}

// TestSettingsService_MergeClassificationSchemas_ReplaceSystem tests that custom classification
// schemas with the same category name as system schemas replace them.
func TestSettingsService_MergeClassificationSchemas_ReplaceSystem(t *testing.T) {
	systemSchemas := []types.ClassificationSchema{
		{
			Category:    "Software Development",
			Description: "System software development category",
			IsSystem:    true,
			Classifications: []types.Classification{
				{ID: "bug", Name: "Bug"},
				{ID: "feature", Name: "Feature"},
			},
		},
	}

	customSchemas := []types.ClassificationSchema{
		{
			Category:    "Software Development",
			Description: "Custom software development category",
			IsSystem:    false,
			Classifications: []types.Classification{
				{ID: "custom-bug", Name: "Custom Bug"},
			},
		},
	}

	merged := mergeClassificationSchemas(systemSchemas, customSchemas)

	// Should have exactly 1 schema (custom replaces system)
	assert.Len(t, merged, 1)
	assert.Equal(t, "Software Development", merged[0].Category)
	assert.False(t, merged[0].IsSystem)
	assert.Equal(t, "Custom software development category", merged[0].Description)
	assert.Len(t, merged[0].Classifications, 1)
	assert.Equal(t, "custom-bug", merged[0].Classifications[0].ID)
}

// TestSettingsService_MergeClassificationSchemas_AppendNew tests that custom classification
// schemas with new category names are appended.
func TestSettingsService_MergeClassificationSchemas_AppendNew(t *testing.T) {
	systemSchemas := []types.ClassificationSchema{
		{
			Category:    "Software Development",
			Description: "System software development",
			IsSystem:    true,
			Classifications: []types.Classification{
				{ID: "bug", Name: "Bug"},
			},
		},
	}

	customSchemas := []types.ClassificationSchema{
		{
			Category:    "Custom Category",
			Description: "A brand new custom category",
			IsSystem:    false,
			Classifications: []types.Classification{
				{ID: "custom-1", Name: "Custom 1"},
			},
		},
	}

	merged := mergeClassificationSchemas(systemSchemas, customSchemas)

	// Should have 2 schemas (1 system + 1 new custom)
	assert.Len(t, merged, 2)

	// Verify first is still the system schema
	assert.Equal(t, "Software Development", merged[0].Category)
	assert.True(t, merged[0].IsSystem)

	// Verify second is the new custom schema
	assert.Equal(t, "Custom Category", merged[1].Category)
	assert.False(t, merged[1].IsSystem)
}

// TestSettingsService_MergeClassificationSchemas_Mixed tests merging with multiple system
// schemas, some replaced and some new custom schemas added.
func TestSettingsService_MergeClassificationSchemas_Mixed(t *testing.T) {
	systemSchemas := []types.ClassificationSchema{
		{
			Category:    "Software Development",
			IsSystem:    true,
			Classifications: []types.Classification{{ID: "bug", Name: "Bug"}},
		},
		{
			Category:    "Project Management",
			IsSystem:    true,
			Classifications: []types.Classification{{ID: "task", Name: "Task"}},
		},
	}

	customSchemas := []types.ClassificationSchema{
		{
			Category:    "Software Development",
			IsSystem:    false,
			Classifications: []types.Classification{{ID: "custom-bug", Name: "Custom Bug"}},
		},
		{
			Category:    "New Custom",
			IsSystem:    false,
			Classifications: []types.Classification{{ID: "new", Name: "New"}},
		},
	}

	merged := mergeClassificationSchemas(systemSchemas, customSchemas)

	// Should have 3 schemas (1 replaced system + 1 unchanged system + 1 new custom)
	assert.Len(t, merged, 3)

	// Check categories
	categories := []string{merged[0].Category, merged[1].Category, merged[2].Category}
	assert.Contains(t, categories, "Software Development")
	assert.Contains(t, categories, "Project Management")
	assert.Contains(t, categories, "New Custom")

	// Verify Software Development was replaced (should be non-system)
	var softwareDev *types.ClassificationSchema
	for i := range merged {
		if merged[i].Category == "Software Development" {
			softwareDev = &merged[i]
			break
		}
	}
	require.NotNil(t, softwareDev)
	assert.False(t, softwareDev.IsSystem)
	assert.Len(t, softwareDev.Classifications, 1)
	assert.Equal(t, "custom-bug", softwareDev.Classifications[0].ID)

	// Verify Project Management was not replaced
	var projectMgmt *types.ClassificationSchema
	for i := range merged {
		if merged[i].Category == "Project Management" {
			projectMgmt = &merged[i]
			break
		}
	}
	require.NotNil(t, projectMgmt)
	assert.True(t, projectMgmt.IsSystem)
}

// TestSettingsService_MultipleConnections tests that settings are properly isolated
// between different connections.
func TestSettingsService_MultipleConnections(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)

	// Save different settings for two connections
	conn1Types := []types.CustomEntityType{
		{ID: "customer", Name: "Customer"},
	}
	conn2Types := []types.CustomEntityType{
		{ID: "product", Name: "Product"},
	}

	err := service.SaveEntityTypes("conn-1", conn1Types)
	require.NoError(t, err)

	err = service.SaveEntityTypes("conn-2", conn2Types)
	require.NoError(t, err)

	// Verify each connection has its own custom types
	settings1, err := service.GetSettings("conn-1")
	require.NoError(t, err)
	assert.Len(t, settings1.CustomEntityTypes, 1)
	assert.Equal(t, "customer", settings1.CustomEntityTypes[0].ID)

	settings2, err := service.GetSettings("conn-2")
	require.NoError(t, err)
	assert.Len(t, settings2.CustomEntityTypes, 1)
	assert.Equal(t, "product", settings2.CustomEntityTypes[0].ID)
}

// TestSettingsService_PersistenceAcrossMultipleCalls tests that settings persist
// when multiple service methods are called on the same connection.
func TestSettingsService_PersistenceAcrossMultipleCalls(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	connectionID := "test-conn-persist"

	// Save entity types
	entityTypes := []types.CustomEntityType{
		{ID: "customer", Name: "Customer"},
	}
	err := service.SaveEntityTypes(connectionID, entityTypes)
	require.NoError(t, err)

	// Save relationship types
	relTypes := []types.CustomRelationshipType{
		{ID: "manages", Name: "Manages"},
	}
	err = service.SaveRelationshipTypes(connectionID, relTypes)
	require.NoError(t, err)

	// Save memory types
	memTypes := []types.CustomMemoryType{
		{ID: "incident", Name: "Incident"},
	}
	err = service.SaveMemoryTypes(connectionID, memTypes)
	require.NoError(t, err)

	// Save active category
	err = service.SaveActiveClassificationCategory(connectionID, "Software Development")
	require.NoError(t, err)

	// Retrieve all settings and verify everything is present
	settings, err := service.GetSettings(connectionID)
	require.NoError(t, err)

	assert.Len(t, settings.CustomEntityTypes, 1)
	assert.Equal(t, "customer", settings.CustomEntityTypes[0].ID)

	assert.Len(t, settings.CustomRelationshipTypes, 1)
	assert.Equal(t, "manages", settings.CustomRelationshipTypes[0].ID)

	assert.Len(t, settings.CustomMemoryTypes, 1)
	assert.Equal(t, "incident", settings.CustomMemoryTypes[0].ID)

	assert.Equal(t, "Software Development", settings.ActiveClassificationCategory)
}

// TestSettingsService_SaveAfterGetNonExistent tests that we can save settings
// for a connection that previously didn't exist.
func TestSettingsService_SaveAfterGetNonExistent(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	connectionID := "test-conn-new"

	// Get settings for non-existent connection (should return system defaults)
	settings1, err := service.GetSettings(connectionID)
	require.NoError(t, err)
	assert.Empty(t, settings1.CustomEntityTypes)

	// Now save some custom types
	customTypes := []types.CustomEntityType{
		{ID: "custom", Name: "Custom"},
	}
	err = service.SaveEntityTypes(connectionID, customTypes)
	require.NoError(t, err)

	// Get settings again and verify custom types are now present
	settings2, err := service.GetSettings(connectionID)
	require.NoError(t, err)
	assert.Len(t, settings2.CustomEntityTypes, 1)
	assert.Equal(t, "custom", settings2.CustomEntityTypes[0].ID)
}

// TestSettingsService_EmptySlices tests handling of empty slices for various settings.
func TestSettingsService_EmptySlices(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	connectionID := "test-conn-empty"

	// Save empty slices
	err := service.SaveEntityTypes(connectionID, []types.CustomEntityType{})
	require.NoError(t, err)

	err = service.SaveRelationshipTypes(connectionID, []types.CustomRelationshipType{})
	require.NoError(t, err)

	err = service.SaveMemoryTypes(connectionID, []types.CustomMemoryType{})
	require.NoError(t, err)

	err = service.SaveClassificationSchemas(connectionID, []types.ClassificationSchema{})
	require.NoError(t, err)

	// Verify all are empty
	settings, err := service.GetSettings(connectionID)
	require.NoError(t, err)

	assert.Empty(t, settings.CustomEntityTypes)
	assert.Empty(t, settings.CustomRelationshipTypes)
	assert.Empty(t, settings.CustomMemoryTypes)
	assert.Empty(t, settings.CustomClassificationSchemas)
}

// TestSettingsService_LargeCustomTypesList tests handling of large lists of custom types.
func TestSettingsService_LargeCustomTypesList(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	service := NewSettingsService(db)
	connectionID := "test-conn-large"

	// Create a large list of custom entity types
	var customTypes []types.CustomEntityType
	for i := 0; i < 100; i++ {
		customTypes = append(customTypes, types.CustomEntityType{
			ID:   "type" + string(rune(i)),
			Name: "Type " + string(rune(i)),
		})
	}

	// Save and retrieve
	err := service.SaveEntityTypes(connectionID, customTypes)
	require.NoError(t, err)

	settings, err := service.GetSettings(connectionID)
	require.NoError(t, err)

	assert.Len(t, settings.CustomEntityTypes, 100)
}
