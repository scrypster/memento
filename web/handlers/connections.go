package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/scrypster/memento/internal/connections"
)

// ConnectionHandlers contains HTTP handlers for connection management.
type ConnectionHandlers struct {
	manager *connections.Manager
}

// NewConnectionHandlers creates a new ConnectionHandlers instance.
func NewConnectionHandlers(manager *connections.Manager) *ConnectionHandlers {
	return &ConnectionHandlers{
		manager: manager,
	}
}

// ListConnections handles GET /api/connections - returns all connections.
func (h *ConnectionHandlers) ListConnections(w http.ResponseWriter, r *http.Request) {
	connections := h.manager.ListConnections()
	defaultConn := h.manager.GetDefaultConnection()

	response := map[string]interface{}{
		"connections":        connections,
		"default_connection": defaultConn,
	}

	respondJSON(w, http.StatusOK, response)
}

// CreateConnectionRequest represents the request body for creating a connection.
type CreateConnectionRequest struct {
	Name             string                      `json:"name"`
	DisplayName      string                      `json:"display_name"`
	Description      string                      `json:"description"`
	Database         connections.DatabaseConfig  `json:"database"`
	LLM              connections.LLMConfig       `json:"llm"`
	CategoryTemplate string                      `json:"category_template,omitempty"`
	Categories       []string                    `json:"categories,omitempty"`
}

// CreateConnection handles POST /api/connections - creates a new connection.
func (h *ConnectionHandlers) CreateConnection(w http.ResponseWriter, r *http.Request) {
	var req CreateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	// Validate required fields
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "connection name is required", nil)
		return
	}
	if req.DisplayName == "" {
		respondError(w, http.StatusBadRequest, "display name is required", nil)
		return
	}
	if req.Database.Type == "" {
		respondError(w, http.StatusBadRequest, "database type is required", nil)
		return
	}
	// Validate LLM configuration
	if err := validateLLMConfig(req.LLM); err != nil {
		respondError(w, http.StatusBadRequest, "invalid LLM configuration: "+err.Error(), nil)
		return
	}

	// Validate database configuration
	switch req.Database.Type {
	case "sqlite":
		if req.Database.Path == "" {
			respondError(w, http.StatusBadRequest, "database path is required for SQLite", nil)
			return
		}
	case "postgresql":
		if req.Database.Host == "" {
			respondError(w, http.StatusBadRequest, "database host is required for PostgreSQL", nil)
			return
		}
		if req.Database.Database == "" {
			respondError(w, http.StatusBadRequest, "database name is required for PostgreSQL", nil)
			return
		}
	default:
		respondError(w, http.StatusBadRequest, "unsupported database type", nil)
		return
	}

	// Create connection object
	conn := connections.Connection{
		Name:             req.Name,
		DisplayName:      req.DisplayName,
		Description:      req.Description,
		Enabled:          true,
		CreatedAt:        time.Now().Format(time.RFC3339),
		Database:         req.Database,
		LLM:              req.LLM,
		CategoryTemplate: req.CategoryTemplate,
		Categories:       req.Categories,
	}

	// Add connection
	if err := h.manager.AddConnection(r.Context(), conn); err != nil {
		respondError(w, http.StatusBadRequest, "failed to create connection", err)
		return
	}

	respondJSON(w, http.StatusCreated, conn)
}

// UpdateConnection handles PUT /api/connections/{name} - updates an existing connection.
func (h *ConnectionHandlers) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	name := extractID(r, "name")
	// Handle /api/connections/default specifically
	if name == "" && strings.HasSuffix(r.URL.Path, "/default") {
		name = "default"
	}
	if name == "" {
		respondError(w, http.StatusBadRequest, "connection name is required", nil)
		return
	}

	var req CreateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	// Validate LLM configuration if provided
	if req.LLM.Provider != "" {
		if err := validateLLMConfig(req.LLM); err != nil {
			respondError(w, http.StatusBadRequest, "invalid LLM configuration: "+err.Error(), nil)
			return
		}
	}

	// Create connection object (name from URL, other fields from request)
	conn := connections.Connection{
		Name:             name, // Preserve original name from URL
		DisplayName:      req.DisplayName,
		Description:      req.Description,
		Enabled:          true,
		Database:         req.Database,
		LLM:              req.LLM,
		CategoryTemplate: req.CategoryTemplate,
		Categories:       req.Categories,
	}

	// Update connection
	if err := h.manager.UpdateConnection(r.Context(), name, conn); err != nil {
		respondError(w, http.StatusBadRequest, "failed to update connection", err)
		return
	}

	respondJSON(w, http.StatusOK, conn)
}

// DeleteConnection handles DELETE /api/connections/{name} - deletes a connection.
func (h *ConnectionHandlers) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	name := extractID(r, "name")
	if name == "" {
		respondError(w, http.StatusBadRequest, "connection name is required", nil)
		return
	}

	if err := h.manager.DeleteConnection(r.Context(), name); err != nil {
		respondError(w, http.StatusBadRequest, "failed to delete connection", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "connection deleted successfully",
	})
}

// TestConnectionRequest represents the request body for testing a connection.
type TestConnectionRequest struct {
	Database connections.DatabaseConfig `json:"database"`
	LLM      connections.LLMConfig      `json:"llm"`
}

// TestConnection handles POST /api/connections/test - tests a connection configuration.
func (h *ConnectionHandlers) TestConnection(w http.ResponseWriter, r *http.Request) {
	var req TestConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	// Create temporary connection for testing
	conn := connections.Connection{
		Name:     "test",
		Database: req.Database,
		LLM:      req.LLM,
	}

	if err := h.manager.TestConnection(r.Context(), conn); err != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "connection test successful",
	})
}

// SetDefaultConnectionRequest represents the request body for setting default connection.
type SetDefaultConnectionRequest struct {
	Name string `json:"name"`
}

// SetDefaultConnection handles POST /api/connections/default - sets the default connection.
func (h *ConnectionHandlers) SetDefaultConnection(w http.ResponseWriter, r *http.Request) {
	var req SetDefaultConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "connection name is required", nil)
		return
	}

	if err := h.manager.SetDefaultConnection(r.Context(), req.Name); err != nil {
		respondError(w, http.StatusBadRequest, "failed to set default connection", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":            "default connection updated",
		"default_connection": req.Name,
	})
}

// validateLLMConfig validates LLM provider configuration
// Returns error if the configuration is invalid, nil if valid
func validateLLMConfig(cfg connections.LLMConfig) error {
	if cfg.Provider == "" {
		return fmt.Errorf("LLM provider is required")
	}

	switch cfg.Provider {
	case "ollama":
		// Ollama validation
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://localhost:11434" // Use default
		}
		if cfg.Model == "" {
			return fmt.Errorf("model is required for Ollama")
		}
		return nil

	case "openai":
		// OpenAI validation
		if cfg.APIKey == "" {
			return fmt.Errorf("API key is required for OpenAI")
		}
		if cfg.Model == "" {
			return fmt.Errorf("model is required for OpenAI")
		}
		return nil

	case "anthropic":
		// Anthropic validation
		if cfg.APIKey == "" {
			return fmt.Errorf("API key is required for Anthropic")
		}
		if cfg.Model == "" {
			return fmt.Errorf("model is required for Anthropic")
		}
		return nil

	default:
		return fmt.Errorf("unsupported LLM provider: %q", cfg.Provider)
	}
}
