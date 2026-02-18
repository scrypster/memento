package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/scrypster/memento/internal/attribution"
	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// APIHandlers contains HTTP handlers for the REST API.
type APIHandlers struct {
	store              storage.MemoryStore
	config             *config.Config
	db                 *sql.DB // Optional database connection for direct operations
	connectionManager  *connections.Manager
}

// NewAPIHandlers creates a new APIHandlers instance.
func NewAPIHandlers(store storage.MemoryStore, cfg *config.Config) *APIHandlers {
	return &APIHandlers{
		store:  store,
		config: cfg,
		db:     nil,
		connectionManager: nil,
	}
}

// NewAPIHandlersWithDB creates a new APIHandlers instance with database support.
func NewAPIHandlersWithDB(store storage.MemoryStore, cfg *config.Config, db *sql.DB) *APIHandlers {
	return &APIHandlers{
		store:  store,
		config: cfg,
		db:     db,
		connectionManager: nil,
	}
}

// NewAPIHandlersWithConnManager creates a new APIHandlers instance with connection manager support.
func NewAPIHandlersWithConnManager(store storage.MemoryStore, cfg *config.Config, db *sql.DB, connManager *connections.Manager) *APIHandlers {
	return &APIHandlers{
		store:  store,
		config: cfg,
		db:     db,
		connectionManager: connManager,
	}
}

// SetConnectionManager sets the connection manager for multi-connection support.
func (h *APIHandlers) SetConnectionManager(connManager *connections.Manager) {
	h.connectionManager = connManager
}

// ListMemories handles GET /api/memories - list memories with pagination and filtering.
// Supports connection switching via "connection" query parameter or X-Connection-ID header.
func (h *APIHandlers) ListMemories(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	page := parseInt(r.URL.Query().Get("page"), 1)
	limit := parseInt(r.URL.Query().Get("limit"), 10)

	// Enforce maximum pagination limit to prevent resource exhaustion
	if limit > 1000 {
		limit = 1000
	}

	sortBy := r.URL.Query().Get("sort_by")
	sortOrder := r.URL.Query().Get("sort_order")
	status := r.URL.Query().Get("status")
	domain := r.URL.Query().Get("domain")
	connectionName := r.URL.Query().Get("connection")

	// Try header if query parameter not provided
	if connectionName == "" {
		connectionName = r.Header.Get("X-Connection-ID")
	}

	// Get the appropriate store for the connection
	store := h.store
	if connectionName != "" && h.connectionManager != nil {
		var err error
		store, err = h.connectionManager.GetStore(connectionName)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid connection", err)
			return
		}
	}

	// Build list options
	opts := storage.ListOptions{
		Page:      page,
		Limit:     limit,
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Filter:    make(map[string]interface{}),
	}

	// Add filters
	if status != "" {
		opts.Filter["status"] = status
	}
	if domain != "" {
		opts.Filter["domain"] = domain
	}

	// Normalize options (applies defaults)
	opts.Normalize()

	// Fetch memories from store
	result, err := store.List(r.Context(), opts)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list memories", err)
		return
	}

	// Return paginated result
	respondJSON(w, http.StatusOK, result)
}

// GetMemory handles GET /api/memories/{id} - get a single memory by ID.
// Supports connection switching via "connection" query parameter or X-Connection-ID header.
func (h *APIHandlers) GetMemory(w http.ResponseWriter, r *http.Request) {
	// Extract memory ID from path
	id := extractID(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "memory ID is required", nil)
		return
	}

	// Get connection from query param or header
	connectionName := r.URL.Query().Get("connection")
	if connectionName == "" {
		connectionName = r.Header.Get("X-Connection-ID")
	}

	// Get the appropriate store for the connection
	store := h.store
	if connectionName != "" && h.connectionManager != nil {
		var err error
		store, err = h.connectionManager.GetStore(connectionName)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid connection", err)
			return
		}
	}

	// Fetch memory from store
	memory, err := store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			respondError(w, http.StatusNotFound, "memory not found", err)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get memory", err)
		return
	}

	// Return memory
	respondJSON(w, http.StatusOK, memory)
}

// CreateMemoryRequest represents the request body for creating a memory.
type CreateMemoryRequest struct {
	Content      string                 `json:"content"`
	Source       string                 `json:"source"`
	Domain       string                 `json:"domain,omitempty"`
	ConnectionID string                 `json:"connection_id,omitempty"` // Selects connection; sets domain when no explicit domain given
	Tags         []string               `json:"tags,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	Timestamp    *time.Time             `json:"timestamp,omitempty"`
	CreatedBy    string                 `json:"created_by,omitempty"`
}

// CreateMemory handles POST /api/memories - create a new memory.
// Memory is created with status "pending" and enrichment happens asynchronously.
// Supports connection switching via "connection" query parameter or X-Connection-ID header.
func (h *APIHandlers) CreateMemory(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req CreateMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	// Validate required fields
	if req.Content == "" {
		respondError(w, http.StatusBadRequest, "content is required", nil)
		return
	}
	if req.Source == "" {
		req.Source = "api" // Default source
	}

	// Resolve connection: query param > header > body field
	connectionName := r.URL.Query().Get("connection")
	if connectionName == "" {
		connectionName = r.Header.Get("X-Connection-ID")
	}
	if connectionName == "" {
		connectionName = req.ConnectionID
	}

	// Get the appropriate store for the connection
	store := h.store
	if connectionName != "" && h.connectionManager != nil {
		var err error
		store, err = h.connectionManager.GetStore(connectionName)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid connection", err)
			return
		}
	}

	// Use connection name as domain when no explicit domain provided.
	// The domain becomes the second segment of "mem:<domain>:<slug>", which
	// fetchSettings() uses to look up per-connection enrichment settings.
	domain := req.Domain
	if domain == "" && connectionName != "" {
		domain = connectionName
	}

	// Generate memory ID
	id := generateMemoryID(domain)

	// Create memory with pending status
	now := time.Now()
	memory := &types.Memory{
		ID:        id,
		Content:   req.Content,
		Source:    req.Source,
		Domain:    domain,
		Tags:      req.Tags,
		Metadata:  req.Metadata,
		Timestamp: now,
		CreatedAt: now,
		UpdatedAt: now,

		// Set pending status (enrichment happens asynchronously)
		Status:             types.StatusPending,
		EntityStatus:       types.EnrichmentPending,
		RelationshipStatus: types.EnrichmentPending,
		EmbeddingStatus:    types.EnrichmentPending,
	}

	// Set created_by: use explicit request value if provided, otherwise auto-detect
	if req.CreatedBy != "" {
		memory.CreatedBy = req.CreatedBy
	} else {
		memory.CreatedBy = attribution.DetectAgent()
	}

	// Override timestamp if provided
	if req.Timestamp != nil && !req.Timestamp.IsZero() {
		memory.Timestamp = *req.Timestamp
	}

	// Store memory
	if err := store.Store(r.Context(), memory); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create memory", err)
		return
	}

	// Return created memory with 201 status
	// Note: Status is "pending" since enrichment is async
	respondJSON(w, http.StatusCreated, memory)
}

// UpdateMemoryRequest represents the request body for updating a memory.
// All fields are optional for partial updates.
type UpdateMemoryRequest struct {
	Content  *string                 `json:"content,omitempty"`
	Source   *string                 `json:"source,omitempty"`
	Domain   *string                 `json:"domain,omitempty"`
	Tags     *[]string               `json:"tags,omitempty"`
	Metadata *map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateMemory handles PUT /api/memories/{id} - update an existing memory.
// Supports partial updates (only updates fields that are provided).
// Supports connection switching via "connection" query parameter or X-Connection-ID header.
func (h *APIHandlers) UpdateMemory(w http.ResponseWriter, r *http.Request) {
	// Extract memory ID from path
	id := extractID(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "memory ID is required", nil)
		return
	}

	// Parse request body
	var req UpdateMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	// Get connection from query param or header
	connectionName := r.URL.Query().Get("connection")
	if connectionName == "" {
		connectionName = r.Header.Get("X-Connection-ID")
	}

	// Get the appropriate store for the connection
	store := h.store
	if connectionName != "" && h.connectionManager != nil {
		var err error
		store, err = h.connectionManager.GetStore(connectionName)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid connection", err)
			return
		}
	}

	// Fetch existing memory
	memory, err := store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			respondError(w, http.StatusNotFound, "memory not found", err)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get memory", err)
		return
	}

	// Apply partial updates
	if req.Content != nil {
		memory.Content = *req.Content
	}
	if req.Source != nil {
		memory.Source = *req.Source
	}
	if req.Domain != nil {
		memory.Domain = *req.Domain
	}
	if req.Tags != nil {
		memory.Tags = *req.Tags
	}
	if req.Metadata != nil {
		memory.Metadata = *req.Metadata
	}

	// Update timestamp
	memory.UpdatedAt = time.Now()

	// Update memory in store
	if err := store.Update(r.Context(), memory); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update memory", err)
		return
	}

	// Return updated memory
	respondJSON(w, http.StatusOK, memory)
}

// DeleteMemory handles DELETE /api/memories/{id} - delete a memory.
// Supports connection switching via "connection" query parameter or X-Connection-ID header.
func (h *APIHandlers) DeleteMemory(w http.ResponseWriter, r *http.Request) {
	// Extract memory ID from path
	id := extractID(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "memory ID is required", nil)
		return
	}

	// Get connection from query param or header
	connectionName := r.URL.Query().Get("connection")
	if connectionName == "" {
		connectionName = r.Header.Get("X-Connection-ID")
	}

	// Get the appropriate store for the connection
	store := h.store
	if connectionName != "" && h.connectionManager != nil {
		var err error
		store, err = h.connectionManager.GetStore(connectionName)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid connection", err)
			return
		}
	}

	// Delete memory from store
	if err := store.Delete(r.Context(), id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			respondError(w, http.StatusNotFound, "memory not found", err)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to delete memory", err)
		return
	}

	// Return 204 No Content
	w.WriteHeader(http.StatusNoContent)
}

// UserConfigRequest represents the request body for user config updates.
type UserConfigRequest struct {
	UserName string `json:"user_name"`
}

// UserConfigResponse represents the response format for GET /api/config/user.
type UserConfigResponse struct {
	UserName string `json:"user_name"`
}

// GetUserConfig handles GET /api/config/user - retrieve user configuration.
func (h *APIHandlers) GetUserConfig(w http.ResponseWriter, r *http.Request) {
	// If we have a database connection, load fresh from DB to ensure latest values
	userName := h.config.User.UserName
	if h.db != nil {
		// Query directly from settings table for latest value
		var dbUserName string
		err := h.db.QueryRow("SELECT value FROM settings WHERE key = ?", "user_name").Scan(&dbUserName)
		if err == nil {
			userName = dbUserName
		}
		// If not found in DB or error, fall back to in-memory value
	}

	response := UserConfigResponse{
		UserName: userName,
	}
	respondJSON(w, http.StatusOK, response)
}

// PostUserConfig handles POST /api/config/user - update user configuration.
func (h *APIHandlers) PostUserConfig(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req UserConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	// Update config in memory
	h.config.User.UserName = req.UserName

	// Persist to database if database connection is available
	if h.db != nil {
		if err := h.config.SaveConfig(h.db); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to save config", err)
			return
		}
	}

	response := UserConfigResponse{
		UserName: h.config.User.UserName,
	}
	respondJSON(w, http.StatusOK, response)
}

// Helper functions

// extractID extracts a path parameter from the request.
func extractID(r *http.Request, key string) string {
	return r.PathValue(key)
}

// parseInt parses an integer from a string, returning defaultValue if parsing fails.
func parseInt(s string, defaultValue int) int {
	if s == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return defaultValue
	}
	return val
}

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// If encoding fails, log but don't try to write another response
		// (headers already sent)
		fmt.Printf("failed to encode JSON response: %v\n", err)
	}
}

// respondError writes an error response with the given status code.
func respondError(w http.ResponseWriter, statusCode int, message string, err error) {
	errResp := ErrorResponse{
		Error: message,
		Code:  http.StatusText(statusCode),
	}

	if err != nil {
		errResp.Details = map[string]interface{}{
			"error": err.Error(),
		}
	}

	respondJSON(w, statusCode, errResp)
}

// generateMemoryID generates a unique memory ID in the format mem:domain:uuid.
// If domain is empty, uses "default" as the domain.
func generateMemoryID(domain string) string {
	if domain == "" {
		domain = "default"
	}
	// Generate short UUID (8 chars)
	shortUUID := uuid.New().String()[:8]
	return fmt.Sprintf("mem:%s:%s", domain, shortUUID)
}
