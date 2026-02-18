package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/storage"
)

// dbGetter is satisfied by stores that expose their underlying *sql.DB.
type dbGetter interface {
	GetDB() *sql.DB
}

// SearchHandler handles search-related API endpoints.
type SearchHandler struct {
	store             storage.MemoryStore
	connectionManager *connections.Manager
}

// NewSearchHandler creates a new SearchHandler instance.
func NewSearchHandler(store storage.MemoryStore, connectionManager *connections.Manager) *SearchHandler {
	return &SearchHandler{
		store:             store,
		connectionManager: connectionManager,
	}
}

// UnifiedSearchResult represents a unified search result across all types.
type UnifiedSearchResult struct {
	Type        string  `json:"type"` // "memory", "entity", or "relationship"
	ID          string  `json:"id"`
	Name        string  `json:"name,omitempty"`
	Content     string  `json:"content,omitempty"`
	Description string  `json:"description,omitempty"`
	Source      string  `json:"source,omitempty"`
	CreatedAt   string  `json:"created_at,omitempty"`
	Relevance   float64 `json:"relevance,omitempty"`
}

// Search handles GET /api/search — full-text search across memories, entities, and relationships.
//
// Query parameters:
//   - q          — search query (optional; omit to browse all memories)
//   - page       — page number (default 1)
//   - page_size  — results per page (default 20, max 100)
//   - connection — connection name (falls back to X-Connection-ID header, then default)
//
// Memory results use FTS5 when available (SQLite and PostgreSQL). Entity and
// relationship results are appended as supplemental matches via a LIKE filter
// (SQLite connections only) and do not affect pagination.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Resolve connection.
	connectionName := r.URL.Query().Get("connection")
	if connectionName == "" {
		connectionName = r.Header.Get("X-Connection-ID")
	}
	store, err := h.connectionManager.GetStore(connectionName)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid connection", err)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	page := parseInt(r.URL.Query().Get("page"), 1)
	if page < 1 {
		page = 1
	}
	pageSize := parseInt(r.URL.Query().Get("page_size"), 20)
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	results := []UnifiedSearchResult{}
	total := 0
	hasMore := false

	// ------------------------------------------------------------------
	// Memory search — prefer FTS / SearchProvider when available.
	// Both SQLite (FTS5) and PostgreSQL (tsvector) implement this interface.
	// Falls back to raw LIKE when the store does not implement SearchProvider.
	// ------------------------------------------------------------------
	if searcher, ok := store.(storage.SearchProvider); ok {
		opts := storage.SearchOptions{
			Query:  query,
			Limit:  pageSize,
			Offset: offset,
		}
		if ftsResult, ftsErr := searcher.FullTextSearch(ctx, opts); ftsErr == nil {
			for _, mem := range ftsResult.Items {
				item := UnifiedSearchResult{
					Type:      "memory",
					ID:        mem.ID,
					Content:   mem.Content,
					Source:    mem.Source,
					CreatedAt: mem.CreatedAt.Format("2006-01-02 15:04:05"),
				}
				results = append(results, item)
			}
			total = ftsResult.Total
			hasMore = ftsResult.HasMore
		}
	} else if dbStore, ok := store.(dbGetter); ok {
		results, total, hasMore = searchMemoriesLike(ctx, dbStore.GetDB(), query, pageSize, offset)
	}

	// ------------------------------------------------------------------
	// Supplemental entity / relationship search (text queries only,
	// requires direct DB access — SQLite connections only).
	// These results are appended after memory results and do not affect
	// the pagination total.
	// ------------------------------------------------------------------
	if query != "" {
		if dbStore, ok := store.(dbGetter); ok {
			db := dbStore.GetDB()
			results = append(results, searchEntitiesLike(ctx, db, query, pageSize)...)
			results = append(results, searchRelationshipsLike(ctx, db, query, pageSize)...)
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"query":     query,
		"results":   results,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"has_more":  hasMore,
	})
}

// searchMemoriesLike is a fallback LIKE-based memory search used when the store
// does not implement storage.SearchProvider.
func searchMemoriesLike(ctx context.Context, db *sql.DB, query string, limit, offset int) ([]UnifiedSearchResult, int, bool) {
	var results []UnifiedSearchResult

	if query == "" {
		var total int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memories").Scan(&total); err != nil {
			return results, 0, false
		}
		rows, err := db.QueryContext(ctx, `SELECT id, content, source, created_at FROM memories ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
		if err != nil {
			return results, 0, false
		}
		defer rows.Close()
		for rows.Next() {
			var id, content, source string
			var createdAt sql.NullTime
			if err := rows.Scan(&id, &content, &source, &createdAt); err == nil {
				item := UnifiedSearchResult{Type: "memory", ID: id, Content: content, Source: source}
				if createdAt.Valid {
					item.CreatedAt = createdAt.Time.Format("2006-01-02 15:04:05")
				}
				results = append(results, item)
			}
		}
		return results, total, offset+len(results) < total
	}

	pattern := "%" + strings.ToLower(query) + "%"
	rows, err := db.QueryContext(ctx,
		`SELECT id, content, source, created_at FROM memories WHERE LOWER(content) LIKE ? OR LOWER(source) LIKE ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		pattern, pattern, limit, offset)
	if err != nil {
		return results, 0, false
	}
	defer rows.Close()
	for rows.Next() {
		var id, content, source string
		var createdAt sql.NullTime
		if err := rows.Scan(&id, &content, &source, &createdAt); err == nil {
			item := UnifiedSearchResult{Type: "memory", ID: id, Content: content, Source: source}
			if createdAt.Valid {
				item.CreatedAt = createdAt.Time.Format("2006-01-02 15:04:05")
			}
			results = append(results, item)
		}
	}
	return results, len(results), false
}

// searchEntitiesLike performs a LIKE search on entities.
func searchEntitiesLike(ctx context.Context, db *sql.DB, query string, limit int) []UnifiedSearchResult {
	pattern := "%" + strings.ToLower(query) + "%"
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, type, description, created_at FROM entities WHERE LOWER(name) LIKE ? OR LOWER(description) LIKE ? OR LOWER(type) LIKE ? ORDER BY created_at DESC LIMIT ?`,
		pattern, pattern, pattern, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []UnifiedSearchResult
	for rows.Next() {
		var id, name, entityType string
		var description sql.NullString
		var createdAt sql.NullTime
		if err := rows.Scan(&id, &name, &entityType, &description, &createdAt); err == nil {
			item := UnifiedSearchResult{Type: "entity", ID: id, Name: name}
			if description.Valid {
				item.Description = description.String
			}
			if createdAt.Valid {
				item.CreatedAt = createdAt.Time.Format("2006-01-02 15:04:05")
			}
			results = append(results, item)
		}
	}
	return results
}

// searchRelationshipsLike performs a LIKE search on relationships.
func searchRelationshipsLike(ctx context.Context, db *sql.DB, query string, limit int) []UnifiedSearchResult {
	pattern := "%" + strings.ToLower(query) + "%"
	rows, err := db.QueryContext(ctx,
		`SELECT r.id, e1.name, e2.name, r.type, r.created_at
		FROM relationships r
		JOIN entities e1 ON r.source_id = e1.id
		JOIN entities e2 ON r.target_id = e2.id
		WHERE LOWER(e1.name) LIKE ? OR LOWER(e2.name) LIKE ? OR LOWER(r.type) LIKE ?
		ORDER BY r.created_at DESC LIMIT ?`,
		pattern, pattern, pattern, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []UnifiedSearchResult
	for rows.Next() {
		var id, sourceName, targetName, relType string
		var createdAt sql.NullTime
		if err := rows.Scan(&id, &sourceName, &targetName, &relType, &createdAt); err == nil {
			item := UnifiedSearchResult{
				Type:        "relationship",
				ID:          id,
				Name:        sourceName + " → " + targetName,
				Description: relType,
			}
			if createdAt.Valid {
				item.CreatedAt = createdAt.Time.Format("2006-01-02 15:04:05")
			}
			results = append(results, item)
		}
	}
	return results
}
