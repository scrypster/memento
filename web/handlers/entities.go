package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
)

// EntityHandler handles entity-related requests.
type EntityHandler struct {
	store              storage.MemoryStore
	connectionManager  *connections.Manager
}

// NewEntityHandler creates a new EntityHandler instance.
func NewEntityHandler(store storage.MemoryStore, connectionManager *connections.Manager) *EntityHandler {
	return &EntityHandler{
		store:             store,
		connectionManager: connectionManager,
	}
}

// EntityResponse represents a single entity in API responses.
type EntityResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
	MemoryCount int    `json:"memory_count"` // Number of memories this entity appears in
}

// EntitiesListResponse represents the paginated entity list response.
type EntitiesListResponse struct {
	Items    []EntityResponse `json:"items"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	HasMore  bool             `json:"has_more"`
}

// RelationshipResponse represents a single relationship in API responses.
type RelationshipResponse struct {
	ID         string  `json:"id"`
	SourceName string  `json:"source_name"`
	SourceType string  `json:"source_type"`
	TargetName string  `json:"target_name"`
	TargetType string  `json:"target_type"`
	Type       string  `json:"type"`
	Weight     float64 `json:"weight"`
	CreatedAt  string  `json:"created_at"`
}

// RelationshipsListResponse represents the paginated relationship list response.
type RelationshipsListResponse struct {
	Items    []RelationshipResponse `json:"items"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	HasMore  bool                   `json:"has_more"`
}

// ListEntities handles GET /api/entities - returns paginated list of entities.
func (h *EntityHandler) ListEntities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract connection parameter
	connectionName := r.URL.Query().Get("connection")
	store, err := h.connectionManager.GetStore(connectionName)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid connection", err)
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	// Accept both "page_size" and "limit" for backwards compatibility
	limit, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if limit < 1 {
		limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	search := r.URL.Query().Get("search")
	entityType := r.URL.Query().Get("type")

	// Access database directly
	sqliteStore, ok := store.(*sqlite.MemoryStore)
	if !ok {
		respondError(w, http.StatusInternalServerError, "database access not available", nil)
		return
	}

	db := sqliteStore.GetDB()

	// Build query
	query := `
		SELECT e.id, e.name, e.type, e.description, e.created_at,
		       COUNT(DISTINCT me.memory_id) as memory_count
		FROM entities e
		LEFT JOIN memory_entities me ON e.id = me.entity_id
		WHERE 1=1
	`
	countQuery := "SELECT COUNT(*) FROM entities e WHERE 1=1"
	args := []interface{}{}
	countArgs := []interface{}{}

	if search != "" {
		query += " AND (e.name LIKE ? OR e.description LIKE ?)"
		countQuery += " AND (e.name LIKE ? OR e.description LIKE ?)"
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern)
		countArgs = append(countArgs, searchPattern, searchPattern)
	}

	if entityType != "" {
		query += " AND e.type = ?"
		countQuery += " AND e.type = ?"
		args = append(args, entityType)
		countArgs = append(countArgs, entityType)
	}

	query += " GROUP BY e.id ORDER BY memory_count DESC, e.created_at DESC LIMIT ? OFFSET ?"
	offset := (page - 1) * limit
	args = append(args, limit, offset)

	// Get total count
	var total int
	if err := db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count entities", err)
		return
	}

	// Get entities
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query entities", err)
		return
	}
	defer func() { _ = rows.Close() }()

	entities := []EntityResponse{}
	for rows.Next() {
		var entity EntityResponse
		var description sql.NullString
		var createdAt sql.NullTime

		if err := rows.Scan(&entity.ID, &entity.Name, &entity.Type, &description,
			&createdAt, &entity.MemoryCount); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan entity", err)
			return
		}

		if description.Valid {
			entity.Description = description.String
		}
		if createdAt.Valid {
			entity.CreatedAt = createdAt.Time.Format("2006-01-02 15:04:05")
		}

		entities = append(entities, entity)
	}

	response := EntitiesListResponse{
		Items:    entities,
		Total:    total,
		Page:     page,
		PageSize: limit,
		HasMore:  offset+len(entities) < total,
	}

	respondJSON(w, http.StatusOK, response)
}

// ListRelationships handles GET /api/relationships - returns paginated list of relationships.
func (h *EntityHandler) ListRelationships(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract connection parameter
	connectionName := r.URL.Query().Get("connection")
	store, err := h.connectionManager.GetStore(connectionName)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid connection", err)
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	// Accept both "page_size" and "limit" for backwards compatibility
	limit, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if limit < 1 {
		limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	search := r.URL.Query().Get("search")
	relType := r.URL.Query().Get("type")

	// Access database directly
	sqliteStore, ok := store.(*sqlite.MemoryStore)
	if !ok {
		respondError(w, http.StatusInternalServerError, "database access not available", nil)
		return
	}

	db := sqliteStore.GetDB()

	// Build query
	query := `
		SELECT r.id, e1.name, e1.type, e2.name, e2.type, r.type, r.weight, r.created_at
		FROM relationships r
		JOIN entities e1 ON r.source_id = e1.id
		JOIN entities e2 ON r.target_id = e2.id
		WHERE 1=1
	`
	countQuery := `
		SELECT COUNT(*)
		FROM relationships r
		JOIN entities e1 ON r.source_id = e1.id
		JOIN entities e2 ON r.target_id = e2.id
		WHERE 1=1
	`
	args := []interface{}{}
	countArgs := []interface{}{}

	if search != "" {
		query += " AND (e1.name LIKE ? OR e2.name LIKE ?)"
		countQuery += " AND (e1.name LIKE ? OR e2.name LIKE ?)"
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern)
		countArgs = append(countArgs, searchPattern, searchPattern)
	}

	if relType != "" {
		query += " AND r.type = ?"
		countQuery += " AND r.type = ?"
		args = append(args, relType)
		countArgs = append(countArgs, relType)
	}

	query += " ORDER BY r.created_at DESC LIMIT ? OFFSET ?"
	offset := (page - 1) * limit
	args = append(args, limit, offset)

	// Get total count
	var total int
	if err := db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count relationships", err)
		return
	}

	// Get relationships
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query relationships", err)
		return
	}
	defer func() { _ = rows.Close() }()

	relationships := []RelationshipResponse{}
	for rows.Next() {
		var rel RelationshipResponse
		var createdAt sql.NullTime

		if err := rows.Scan(&rel.ID, &rel.SourceName, &rel.SourceType,
			&rel.TargetName, &rel.TargetType, &rel.Type, &rel.Weight, &createdAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan relationship", err)
			return
		}

		if createdAt.Valid {
			rel.CreatedAt = createdAt.Time.Format("2006-01-02 15:04:05")
		}

		relationships = append(relationships, rel)
	}

	response := RelationshipsListResponse{
		Items:    relationships,
		Total:    total,
		Page:     page,
		PageSize: limit,
		HasMore:  offset+len(relationships) < total,
	}

	respondJSON(w, http.StatusOK, response)
}

// --- Graph API types ---

// GraphNode represents a node in the entity graph response.
type GraphNode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	MemoryCount int    `json:"memory_count"`
}

// GraphEdge represents an edge in the entity graph response.
type GraphEdge struct {
	ID     string  `json:"id"`
	Source string  `json:"source"`
	Target string  `json:"target"`
	Type   string  `json:"type"`
	Weight float64 `json:"weight"`
}

// GraphMeta contains metadata about the graph response.
type GraphMeta struct {
	CenterID  string `json:"center_id"`
	Depth     int    `json:"depth"`
	NodeCount int    `json:"node_count"`
	EdgeCount int    `json:"edge_count"`
}

// GraphResponse is the response format for GET /api/entities/{id}/graph.
type GraphResponse struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
	Meta  GraphMeta   `json:"meta"`
}

// GetEntityGraph handles GET /api/entities/{id}/graph - returns the entity's
// neighborhood as a topology-only graph (no layout coordinates).
// Query params:
//   - depth: traversal depth (1-3, default 1)
//   - connection: connection name (default: default connection)
func (h *EntityHandler) GetEntityGraph(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract entity ID from path
	entityID := extractID(r, "id")
	if entityID == "" {
		respondError(w, http.StatusBadRequest, "entity ID is required", nil)
		return
	}

	// Extract connection parameter
	connectionName := r.URL.Query().Get("connection")
	store, err := h.connectionManager.GetStore(connectionName)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid connection", err)
		return
	}

	// Parse depth parameter (1-3, default 1)
	depth, _ := strconv.Atoi(r.URL.Query().Get("depth"))
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	// Access database directly
	sqliteStore, ok := store.(*sqlite.MemoryStore)
	if !ok {
		respondError(w, http.StatusInternalServerError, "database access not available", nil)
		return
	}

	db := sqliteStore.GetDB()

	// Verify the center entity exists
	var centerExists bool
	err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM entities WHERE id = ?)", entityID).Scan(&centerExists)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check entity", err)
		return
	}
	if !centerExists {
		respondError(w, http.StatusNotFound, fmt.Sprintf("entity '%s' not found", entityID), nil)
		return
	}

	// Collect entity IDs at each depth level using iterative BFS.
	// visitedEntities tracks all entity IDs we have seen.
	visitedEntities := map[string]bool{entityID: true}
	// frontier is the set of entity IDs to expand in the current BFS level.
	frontier := []string{entityID}

	// edgeSet deduplicates edges by relationship ID.
	edgeSet := map[string]GraphEdge{}

	for d := 0; d < depth && len(frontier) > 0; d++ {
		// Build placeholder list for the IN clause
		placeholders := ""
		args := make([]interface{}, len(frontier))
		for i, id := range frontier {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args[i] = id
		}

		// Find relationships where any frontier entity is source or target
		query := fmt.Sprintf(`
			SELECT r.id, r.source_id, r.target_id, r.type, r.weight
			FROM relationships r
			WHERE r.source_id IN (%s) OR r.target_id IN (%s)
		`, placeholders, placeholders)

		// We need to pass args twice (once for source_id IN, once for target_id IN)
		fullArgs := append(args, args...)

		rows, err := db.QueryContext(ctx, query, fullArgs...)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to query relationships", err)
			return
		}

		nextFrontier := []string{}
		for rows.Next() {
			var edge GraphEdge
			if err := rows.Scan(&edge.ID, &edge.Source, &edge.Target, &edge.Type, &edge.Weight); err != nil {
				_ = rows.Close()
				respondError(w, http.StatusInternalServerError, "failed to scan relationship", err)
				return
			}

			edgeSet[edge.ID] = edge

			// Discover new entities
			if !visitedEntities[edge.Source] {
				visitedEntities[edge.Source] = true
				nextFrontier = append(nextFrontier, edge.Source)
			}
			if !visitedEntities[edge.Target] {
				visitedEntities[edge.Target] = true
				nextFrontier = append(nextFrontier, edge.Target)
			}
		}
		_ = rows.Close()

		frontier = nextFrontier
	}

	// Fetch node details for all visited entities
	entityIDs := make([]string, 0, len(visitedEntities))
	for id := range visitedEntities {
		entityIDs = append(entityIDs, id)
	}

	nodes := make([]GraphNode, 0, len(entityIDs))
	if len(entityIDs) > 0 {
		placeholders := ""
		args := make([]interface{}, len(entityIDs))
		for i, id := range entityIDs {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args[i] = id
		}

		nodeQuery := fmt.Sprintf(`
			SELECT e.id, e.name, e.type, e.description,
			       COUNT(DISTINCT me.memory_id) as memory_count
			FROM entities e
			LEFT JOIN memory_entities me ON e.id = me.entity_id
			WHERE e.id IN (%s)
			GROUP BY e.id
		`, placeholders)

		rows, err := db.QueryContext(ctx, nodeQuery, args...)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to query entities", err)
			return
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var node GraphNode
			var description sql.NullString
			if err := rows.Scan(&node.ID, &node.Name, &node.Type, &description, &node.MemoryCount); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to scan entity", err)
				return
			}
			if description.Valid {
				node.Description = description.String
			}
			nodes = append(nodes, node)
		}
	}

	// Collect edges
	edges := make([]GraphEdge, 0, len(edgeSet))
	for _, edge := range edgeSet {
		edges = append(edges, edge)
	}

	response := GraphResponse{
		Nodes: nodes,
		Edges: edges,
		Meta: GraphMeta{
			CenterID:  entityID,
			Depth:     depth,
			NodeCount: len(nodes),
			EdgeCount: len(edges),
		},
	}

	respondJSON(w, http.StatusOK, response)
}
