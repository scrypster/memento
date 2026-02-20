package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scrypster/memento/internal/connections"
)

// setupTestConnectionsManager creates a temporary connections manager for testing
// Returns the manager and the path to the config file (which should be cleaned up).
func setupTestConnectionsManager(t *testing.T) (*connections.Manager, string) {
	// Create a temporary directory
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "connections.json")

	// Create a minimal connections config file
	config := connections.ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []connections.Connection{
			{
				Name:        "default",
				DisplayName: "Default Connection",
				Description: "Default in-memory connection",
				Enabled:     true,
				CreatedAt:   time.Now().Format(time.RFC3339),
				Database: connections.DatabaseConfig{
					Type: "sqlite",
					Path: filepath.Join(tmpDir, "default.db"),
				},
				LLM: connections.LLMConfig{
					Provider: "ollama",
					Model:    "neural-chat",
					BaseURL:  "http://localhost:11434",
				},
			},
		},
		Settings: struct {
			AutoCreateDefault bool `json:"auto_create_default"`
			MaxConnections    int  `json:"max_connections"`
			AllowUserCreate   bool `json:"allow_user_create"`
		}{
			AutoCreateDefault: true,
			MaxConnections:    10,
			AllowUserCreate:   true,
		},
	}

	// Marshal to JSON and write to file
	configData, err := json.MarshalIndent(config, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(configPath, configData, 0644)
	require.NoError(t, err)

	// Create manager
	manager, err := connections.NewManager(configPath)
	require.NoError(t, err)

	return manager, configPath
}

// TestConnectionHandlers_ListConnections tests listing all connections.
func TestConnectionHandlers_ListConnections(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	req := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	w := httptest.NewRecorder()

	handler.ListConnections(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Parse response
	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify response structure
	assert.NotNil(t, response["connections"])
	assert.NotNil(t, response["default_connection"])

	// Verify we have at least the default connection
	connections, ok := response["connections"].([]interface{})
	assert.True(t, ok)
	assert.GreaterOrEqual(t, len(connections), 1)

	// Verify default connection is set
	defaultConn, ok := response["default_connection"].(string)
	assert.True(t, ok)
	assert.Equal(t, "default", defaultConn)
}

// TestConnectionHandlers_CreateConnection_Success tests creating a valid new connection.
func TestConnectionHandlers_CreateConnection_Success(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	// Create request
	req := CreateConnectionRequest{
		Name:        "test-connection",
		DisplayName: "Test Connection",
		Description: "A test connection",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/test.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
			BaseURL:  "http://localhost:11434",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.CreateConnection(w, httpReq)

	assert.Equal(t, http.StatusCreated, w.Code)

	// Parse response
	var response connections.Connection
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify connection details
	assert.Equal(t, "test-connection", response.Name)
	assert.Equal(t, "Test Connection", response.DisplayName)
	assert.Equal(t, "A test connection", response.Description)
	assert.Equal(t, "sqlite", response.Database.Type)
	assert.Equal(t, "/tmp/test.db", response.Database.Path)
	assert.Equal(t, "ollama", response.LLM.Provider)
	assert.True(t, response.Enabled)
	assert.NotEmpty(t, response.CreatedAt)

	// Verify it appears in list
	listReq := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	listW := httptest.NewRecorder()
	handler.ListConnections(listW, listReq)

	var listResponse map[string]interface{}
	_ = json.NewDecoder(listW.Body).Decode(&listResponse)
	connections := listResponse["connections"].([]interface{})

	// Should now have 2 connections (default + new one)
	assert.Equal(t, 2, len(connections))
}

// TestConnectionHandlers_CreateConnection_DuplicateName tests that duplicate connection names are rejected.
func TestConnectionHandlers_CreateConnection_DuplicateName(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	// First, create a connection
	req := CreateConnectionRequest{
		Name:        "duplicate-test",
		DisplayName: "First Connection",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/test1.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.CreateConnection(w, httpReq)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Try to create another with the same name
	req2 := CreateConnectionRequest{
		Name:        "duplicate-test",
		DisplayName: "Second Connection",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/test2.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body2, _ := json.Marshal(req2)
	httpReq2 := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body2))
	w2 := httptest.NewRecorder()
	handler.CreateConnection(w2, httpReq2)

	// Should fail with 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, w2.Code)

	// Verify error response
	var errResponse ErrorResponse
	_ = json.NewDecoder(w2.Body).Decode(&errResponse)
	assert.Contains(t, errResponse.Error, "failed to create connection")
}

// TestConnectionHandlers_CreateConnection_MissingName tests that missing name is rejected.
func TestConnectionHandlers_CreateConnection_MissingName(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	req := CreateConnectionRequest{
		Name: "", // Missing
		DisplayName: "Test Connection",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/test.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.CreateConnection(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResponse ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResponse)
	assert.Contains(t, errResponse.Error, "connection name is required")
}

// TestConnectionHandlers_CreateConnection_MissingDisplayName tests that missing display name is rejected.
func TestConnectionHandlers_CreateConnection_MissingDisplayName(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	req := CreateConnectionRequest{
		Name:        "test-conn",
		DisplayName: "", // Missing
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/test.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.CreateConnection(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResponse ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResponse)
	assert.Contains(t, errResponse.Error, "display name is required")
}

// TestConnectionHandlers_CreateConnection_MissingDatabaseType tests that missing database type is rejected.
func TestConnectionHandlers_CreateConnection_MissingDatabaseType(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	req := CreateConnectionRequest{
		Name:        "test-conn",
		DisplayName: "Test",
		Database: connections.DatabaseConfig{
			Type: "", // Missing
			Path: "/tmp/test.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.CreateConnection(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResponse ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResponse)
	assert.Contains(t, errResponse.Error, "database type is required")
}

// TestConnectionHandlers_CreateConnection_SQLiteMissingPath tests that SQLite requires path.
func TestConnectionHandlers_CreateConnection_SQLiteMissingPath(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	req := CreateConnectionRequest{
		Name:        "test-conn",
		DisplayName: "Test",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "", // Missing
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.CreateConnection(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResponse ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResponse)
	assert.Contains(t, errResponse.Error, "database path is required for SQLite")
}

// TestConnectionHandlers_CreateConnection_PostgreSQLMissingHost tests that PostgreSQL requires host.
func TestConnectionHandlers_CreateConnection_PostgreSQLMissingHost(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	req := CreateConnectionRequest{
		Name:        "test-conn",
		DisplayName: "Test",
		Database: connections.DatabaseConfig{
			Type:     "postgresql",
			Host:     "", // Missing
			Database: "testdb",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.CreateConnection(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResponse ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResponse)
	assert.Contains(t, errResponse.Error, "database host is required for PostgreSQL")
}

// TestConnectionHandlers_CreateConnection_UnsupportedDatabaseType tests that unsupported database types are rejected.
func TestConnectionHandlers_CreateConnection_UnsupportedDatabaseType(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	req := CreateConnectionRequest{
		Name:        "test-conn",
		DisplayName: "Test",
		Database: connections.DatabaseConfig{
			Type: "mongodb", // Unsupported
			Path: "/tmp/test",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.CreateConnection(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResponse ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResponse)
	assert.Contains(t, errResponse.Error, "unsupported database type")
}

// TestConnectionHandlers_CreateConnection_InvalidLLMConfig tests that invalid LLM configs are rejected.
func TestConnectionHandlers_CreateConnection_InvalidLLMConfig(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	tests := []struct {
		name    string
		llm     connections.LLMConfig
		wantErr string
	}{
		{
			name: "missing provider",
			llm: connections.LLMConfig{
				Provider: "",
				Model:    "gpt-3.5-turbo",
			},
			wantErr: "LLM provider is required",
		},
		{
			name: "ollama missing model",
			llm: connections.LLMConfig{
				Provider: "ollama",
				Model:    "", // Missing
				BaseURL:  "http://localhost:11434",
			},
			wantErr: "model is required for Ollama",
		},
		{
			name: "openai missing api key",
			llm: connections.LLMConfig{
				Provider: "openai",
				Model:    "gpt-3.5-turbo",
				APIKey:   "", // Missing
			},
			wantErr: "API key is required for OpenAI",
		},
		{
			name: "openai missing model",
			llm: connections.LLMConfig{
				Provider: "openai",
				Model:    "", // Missing
				APIKey:   "sk-test",
			},
			wantErr: "model is required for OpenAI",
		},
		{
			name: "anthropic missing api key",
			llm: connections.LLMConfig{
				Provider: "anthropic",
				Model:    "claude-3-opus",
				APIKey:   "", // Missing
			},
			wantErr: "API key is required for Anthropic",
		},
		{
			name: "unsupported provider",
			llm: connections.LLMConfig{
				Provider: "invalid-provider",
				Model:    "some-model",
			},
			wantErr: "unsupported LLM provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := CreateConnectionRequest{
				Name:        "test-conn",
				DisplayName: "Test",
				Database: connections.DatabaseConfig{
					Type: "sqlite",
					Path: "/tmp/test.db",
				},
				LLM: tt.llm,
			}

			body, _ := json.Marshal(req)
			httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
			w := httptest.NewRecorder()

			handler.CreateConnection(w, httpReq)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var errResponse ErrorResponse
			_ = json.NewDecoder(w.Body).Decode(&errResponse)
			assert.Contains(t, errResponse.Error, "invalid LLM configuration")
		})
	}
}

// TestConnectionHandlers_DeleteConnection_Success tests deleting a non-default connection.
func TestConnectionHandlers_DeleteConnection_Success(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	// First create a connection to delete
	req := CreateConnectionRequest{
		Name:        "to-delete",
		DisplayName: "To Delete",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/delete.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.CreateConnection(w, httpReq)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Verify it exists
	listReq := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	listW := httptest.NewRecorder()
	handler.ListConnections(listW, listReq)
	var listResponse map[string]interface{}
	_ = json.NewDecoder(listW.Body).Decode(&listResponse)
	assert.Equal(t, 2, len(listResponse["connections"].([]interface{})))

	// Delete it
	delReq := httptest.NewRequest(http.MethodDelete, "/api/connections/to-delete", nil)
	delReq.SetPathValue("name", "to-delete")
	delW := httptest.NewRecorder()
	handler.DeleteConnection(delW, delReq)

	assert.Equal(t, http.StatusOK, delW.Code)

	var delResponse map[string]interface{}
	_ = json.NewDecoder(delW.Body).Decode(&delResponse)
	assert.Contains(t, delResponse["message"], "deleted successfully")

	// Verify it no longer exists
	listReq2 := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	listW2 := httptest.NewRecorder()
	handler.ListConnections(listW2, listReq2)
	var listResponse2 map[string]interface{}
	_ = json.NewDecoder(listW2.Body).Decode(&listResponse2)
	assert.Equal(t, 1, len(listResponse2["connections"].([]interface{})))
}

// TestConnectionHandlers_DeleteConnection_DefaultRejected tests that deleting the default connection is rejected.
func TestConnectionHandlers_DeleteConnection_DefaultRejected(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	delReq := httptest.NewRequest(http.MethodDelete, "/api/connections/default", nil)
	delReq.SetPathValue("name", "default")
	delW := httptest.NewRecorder()
	handler.DeleteConnection(delW, delReq)

	assert.Equal(t, http.StatusBadRequest, delW.Code)

	var errResponse ErrorResponse
	_ = json.NewDecoder(delW.Body).Decode(&errResponse)
	assert.Contains(t, errResponse.Error, "failed to delete connection")
}

// TestConnectionHandlers_DeleteConnection_NotFound tests that deleting a non-existent connection fails gracefully.
func TestConnectionHandlers_DeleteConnection_NotFound(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	delReq := httptest.NewRequest(http.MethodDelete, "/api/connections/nonexistent", nil)
	delReq.SetPathValue("name", "nonexistent")
	delW := httptest.NewRecorder()
	handler.DeleteConnection(delW, delReq)

	assert.Equal(t, http.StatusBadRequest, delW.Code)

	var errResponse ErrorResponse
	_ = json.NewDecoder(delW.Body).Decode(&errResponse)
	assert.Contains(t, errResponse.Error, "failed to delete connection")
}

// TestConnectionHandlers_TestConnection_Success tests a successful connection test.
func TestConnectionHandlers_TestConnection_Success(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	// Create a test request with a valid SQLite config pointing to an existing file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create an empty SQLite database file
	f, err := os.Create(dbPath)
	require.NoError(t, err)
	_ = f.Close()

	req := TestConnectionRequest{
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: dbPath,
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
			BaseURL:  "http://localhost:11434",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections/test", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.TestConnection(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&response)
	assert.Equal(t, true, response["success"])
	assert.Contains(t, response["message"], "successful")
}

// TestConnectionHandlers_TestConnection_InvalidPath tests connection test with invalid path.
func TestConnectionHandlers_TestConnection_InvalidPath(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	req := TestConnectionRequest{
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/nonexistent/path/to/database.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections/test", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.TestConnection(w, httpReq)

	// Note: TestConnection returns 200 OK even on failure, with success=false
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&response)
	assert.Equal(t, false, response["success"])
	assert.NotEmpty(t, response["error"])
}

// TestConnectionHandlers_SetDefaultConnection_Success tests setting a new default connection.
func TestConnectionHandlers_SetDefaultConnection_Success(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	// Create a new connection
	createReq := CreateConnectionRequest{
		Name:        "new-default",
		DisplayName: "New Default",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/new.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(createReq)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.CreateConnection(w, httpReq)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Set it as default
	setDefaultReq := SetDefaultConnectionRequest{
		Name: "new-default",
	}

	body2, _ := json.Marshal(setDefaultReq)
	httpReq2 := httptest.NewRequest(http.MethodPost, "/api/connections/default", bytes.NewReader(body2))
	w2 := httptest.NewRecorder()
	handler.SetDefaultConnection(w2, httpReq2)

	assert.Equal(t, http.StatusOK, w2.Code)

	var response map[string]interface{}
	_ = json.NewDecoder(w2.Body).Decode(&response)
	assert.Contains(t, response["message"], "default connection updated")
	assert.Equal(t, "new-default", response["default_connection"])

	// Verify it's now the default
	listReq := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	listW := httptest.NewRecorder()
	handler.ListConnections(listW, listReq)

	var listResponse map[string]interface{}
	_ = json.NewDecoder(listW.Body).Decode(&listResponse)
	assert.Equal(t, "new-default", listResponse["default_connection"])
}

// TestConnectionHandlers_SetDefaultConnection_NotFound tests setting a non-existent connection as default.
func TestConnectionHandlers_SetDefaultConnection_NotFound(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	req := SetDefaultConnectionRequest{
		Name: "nonexistent",
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections/default", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.SetDefaultConnection(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResponse ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResponse)
	assert.Contains(t, errResponse.Error, "failed to set default connection")
}

// TestConnectionHandlers_SetDefaultConnection_MissingName tests that missing name is rejected.
func TestConnectionHandlers_SetDefaultConnection_MissingName(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	req := SetDefaultConnectionRequest{
		Name: "", // Missing
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections/default", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.SetDefaultConnection(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResponse ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResponse)
	assert.Contains(t, errResponse.Error, "connection name is required")
}

// TestConnectionHandlers_UpdateConnection_Success tests successfully updating a connection.
func TestConnectionHandlers_UpdateConnection_Success(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	// Create a connection to update
	createReq := CreateConnectionRequest{
		Name:        "to-update",
		DisplayName: "Original Name",
		Description: "Original description",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/update.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(createReq)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.CreateConnection(w, httpReq)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Update it
	updateReq := CreateConnectionRequest{
		DisplayName: "Updated Name",
		Description: "Updated description",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/updated.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "mistral",
			BaseURL:  "http://localhost:11434",
		},
	}

	body2, _ := json.Marshal(updateReq)
	httpReq2 := httptest.NewRequest(http.MethodPut, "/api/connections/to-update", bytes.NewReader(body2))
	httpReq2.SetPathValue("name", "to-update")
	w2 := httptest.NewRecorder()
	handler.UpdateConnection(w2, httpReq2)

	assert.Equal(t, http.StatusOK, w2.Code)

	var response connections.Connection
	_ = json.NewDecoder(w2.Body).Decode(&response)
	assert.Equal(t, "to-update", response.Name) // Name should be preserved
	assert.Equal(t, "Updated Name", response.DisplayName)
	assert.Equal(t, "Updated description", response.Description)
	assert.Equal(t, "/tmp/updated.db", response.Database.Path)
	assert.Equal(t, "mistral", response.LLM.Model)
}

// TestConnectionHandlers_UpdateConnection_NotFound tests updating a non-existent connection.
func TestConnectionHandlers_UpdateConnection_NotFound(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	updateReq := CreateConnectionRequest{
		DisplayName: "Updated Name",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/update.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(updateReq)
	httpReq := httptest.NewRequest(http.MethodPut, "/api/connections/nonexistent", bytes.NewReader(body))
	httpReq.SetPathValue("name", "nonexistent")
	w := httptest.NewRecorder()
	handler.UpdateConnection(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResponse ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResponse)
	assert.Contains(t, errResponse.Error, "failed to update connection")
}

// TestConnectionHandlers_InvalidJSONBody tests handling of malformed JSON.
func TestConnectionHandlers_InvalidJSONBody(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	tests := []struct {
		name       string
		method     func(*ConnectionHandlers, http.ResponseWriter, *http.Request)
		url        string
		pathValue  string
	}{
		{
			name:   "CreateConnection with invalid JSON",
			method: (*ConnectionHandlers).CreateConnection,
			url:    "/api/connections",
		},
		{
			name:      "UpdateConnection with invalid JSON",
			method:    (*ConnectionHandlers).UpdateConnection,
			url:       "/api/connections/test",
			pathValue: "name=test",
		},
		{
			name:   "TestConnection with invalid JSON",
			method: (*ConnectionHandlers).TestConnection,
			url:    "/api/connections/test",
		},
		{
			name:   "SetDefaultConnection with invalid JSON",
			method: (*ConnectionHandlers).SetDefaultConnection,
			url:    "/api/connections/default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpReq := httptest.NewRequest(http.MethodPost, tt.url, bytes.NewReader([]byte("invalid json")))
			if tt.pathValue != "" {
				parts := bytes.Split([]byte(tt.pathValue), []byte("="))
				httpReq.SetPathValue(string(parts[0]), string(parts[1]))
			}
			w := httptest.NewRecorder()

			tt.method(handler, w, httpReq)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			var errResponse ErrorResponse
			_ = json.NewDecoder(w.Body).Decode(&errResponse)
			assert.NotEmpty(t, errResponse.Error)
		})
	}
}

// TestConnectionHandlers_CreateConnectionWithCategories tests creating a connection with categories.
func TestConnectionHandlers_CreateConnectionWithCategories(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	req := CreateConnectionRequest{
		Name:        "with-categories",
		DisplayName: "With Categories",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/cat.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
		Categories: []string{"personal", "work", "projects"},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.CreateConnection(w, httpReq)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response connections.Connection
	_ = json.NewDecoder(w.Body).Decode(&response)
	assert.Equal(t, []string{"personal", "work", "projects"}, response.Categories)
}

// TestConnectionHandlers_CreateConnectionWithCategoryTemplate tests creating a connection with category template.
func TestConnectionHandlers_CreateConnectionWithCategoryTemplate(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	req := CreateConnectionRequest{
		Name:             "with-template",
		DisplayName:      "With Template",
		CategoryTemplate: "personal_org",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/template.db",
		},
		LLM: connections.LLMConfig{
			Provider: "openai",
			Model:    "gpt-3.5-turbo",
			APIKey:   "sk-test-key",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.CreateConnection(w, httpReq)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response connections.Connection
	_ = json.NewDecoder(w.Body).Decode(&response)
	assert.Equal(t, "personal_org", response.CategoryTemplate)
}

// TestConnectionHandlers_Context tests that handlers properly use request context.
func TestConnectionHandlers_Context(t *testing.T) {
	manager, _ := setupTestConnectionsManager(t)
	defer func() { _ = manager.Close() }()

	handler := NewConnectionHandlers(manager)

	// Create a request with a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := CreateConnectionRequest{
		Name:        "context-test",
		DisplayName: "Context Test",
		Database: connections.DatabaseConfig{
			Type: "sqlite",
			Path: "/tmp/ctx.db",
		},
		LLM: connections.LLMConfig{
			Provider: "ollama",
			Model:    "neural-chat",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(body))
	httpReq = httpReq.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.CreateConnection(w, httpReq)

	// Should succeed even though context is cancellable
	assert.Equal(t, http.StatusCreated, w.Code)
}
