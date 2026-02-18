package storage

import (
	"errors"
	"time"

	"github.com/scrypster/memento/pkg/types"
)

var (
	// ErrNotFound indicates that the requested resource was not found.
	ErrNotFound = errors.New("resource not found")

	// ErrInvalidInput indicates that the input parameters are invalid.
	ErrInvalidInput = errors.New("invalid input")

	// ErrGraphBoundsExceeded indicates that graph traversal exceeded bounds.
	ErrGraphBoundsExceeded = errors.New("graph bounds exceeded")
)

// PaginatedResult represents a paginated result set with type safety using generics.
type PaginatedResult[T any] struct {
	// Items is the slice of results for the current page.
	Items []T

	// Total is the total number of items across all pages.
	Total int

	// Page is the current page number (1-indexed).
	Page int

	// PageSize is the number of items per page.
	PageSize int

	// HasMore indicates whether there are more pages available.
	HasMore bool
}

// ListOptions provides pagination and filtering options for list operations.
type ListOptions struct {
	// Page is the page number to retrieve (1-indexed, default: 1).
	Page int

	// Limit is the number of items per page (default: 10, max: 100).
	Limit int

	// SortBy specifies the field to sort by (e.g., "created_at", "updated_at").
	SortBy string

	// SortOrder specifies the sort direction ("asc" or "desc", default: "desc").
	SortOrder string

	// Filter provides key-value pairs for filtering results.
	// Deprecated: Use typed filter fields instead. Retained for backward compatibility.
	Filter map[string]interface{}

	// State filters by lifecycle state (e.g., "active", "archived").
	// Empty string means no filter on state.
	State string

	// CreatedBy filters by the agent or user that created the memory.
	// Empty string means no filter on created_by.
	CreatedBy string

	// CreatedAfter filters to memories created strictly after this time.
	// Zero value means no lower bound.
	CreatedAfter time.Time

	// CreatedBefore filters to memories created strictly before this time.
	// Zero value means no upper bound.
	CreatedBefore time.Time

	// MinDecayScore filters to memories with a decay_score >= this value.
	// Zero value means no minimum score filter.
	MinDecayScore float64

	// SessionID filters to memories that belong to a specific session.
	// Empty string means no filter on session_id.
	SessionID string

	// IncludeDeleted includes soft-deleted memories in results.
	// By default (false), soft-deleted memories are excluded from all queries.
	IncludeDeleted bool

	// OnlyDeleted restricts results to soft-deleted memories only.
	// When true, only memories with deleted_at IS NOT NULL are returned.
	// IncludeDeleted must also be true (or the query will return nothing).
	OnlyDeleted bool

	// MemoryType filters memories by their memory_type classification value
	// (e.g. "project", "epic", "task"). Empty string means no filter.
	MemoryType string
}

// Normalize applies defaults and validates the ListOptions.
func (o *ListOptions) Normalize() {
	// Whitelist validation for SortBy to prevent SQL injection
	allowedSortFields := map[string]bool{
		"created_at":   true,
		"updated_at":   true,
		"id":           true,
		"status":       true,
		"decay_score":  true,
		"access_count": true,
	}

	if !allowedSortFields[o.SortBy] {
		o.SortBy = "created_at" // Default sort field
	}

	if o.SortOrder != "asc" && o.SortOrder != "desc" {
		o.SortOrder = "desc" // Default sort order
	}

	if o.Page < 1 {
		o.Page = 1
	}

	if o.Limit < 1 {
		o.Limit = 10 // Default limit
	}

	if o.Limit > 100 {
		o.Limit = 100 // Max limit
	}

	if o.Filter == nil {
		o.Filter = make(map[string]interface{})
	}
}

// Offset calculates the offset for SQL queries based on page and limit.
func (o *ListOptions) Offset() int {
	return (o.Page - 1) * o.Limit
}

// SearchOptions provides options for search operations.
type SearchOptions struct {
	// Query is the search query string.
	Query string

	// Limit is the maximum number of results to return (default: 10, max: 100).
	Limit int

	// Offset is the number of results to skip.
	Offset int

	// MinScore is the minimum relevance score (0.0 to 1.0).
	MinScore float64

	// Filter provides additional filtering criteria.
	Filter map[string]interface{}

	// FuzzyFallback enables fallback to relaxed OR-based search if no results are found.
	// When true and the initial search returns zero results, the query will be split
	// into individual terms and searched with OR semantics instead of AND.
	FuzzyFallback bool
}

// Normalize applies defaults and validates the SearchOptions.
func (o *SearchOptions) Normalize() {
	if o.Limit < 1 {
		o.Limit = 10
	}

	if o.Limit > 100 {
		o.Limit = 100
	}

	if o.Offset < 0 {
		o.Offset = 0
	}

	if o.MinScore < 0.0 {
		o.MinScore = 0.0
	}

	if o.MinScore > 1.0 {
		o.MinScore = 1.0
	}

	if o.Filter == nil {
		o.Filter = make(map[string]interface{})
	}
}

// GraphBounds prevents combinatorial explosion during graph traversal.
type GraphBounds struct {
	// MaxHops is the maximum number of hops from the starting node.
	MaxHops int

	// MaxNodes is the maximum number of nodes to return.
	MaxNodes int

	// MaxEdges is the maximum number of edges to traverse.
	MaxEdges int

	// Timeout is the maximum duration for the traversal operation.
	Timeout time.Duration

	// CreatedAfter restricts traversal to memories created strictly after this
	// time. Zero value means no lower bound on creation time.
	CreatedAfter time.Time

	// CreatedBefore restricts traversal to memories created strictly before this
	// time. Zero value means no upper bound on creation time.
	CreatedBefore time.Time
}

// Normalize applies defaults and validates the GraphBounds.
func (g *GraphBounds) Normalize() {
	if g.MaxHops < 1 {
		g.MaxHops = 3 // Default max hops
	}

	if g.MaxHops > 10 {
		g.MaxHops = 10 // Cap max hops
	}

	if g.MaxNodes < 1 {
		g.MaxNodes = 100 // Default max nodes
	}

	if g.MaxNodes > 1000 {
		g.MaxNodes = 1000 // Cap max nodes
	}

	if g.MaxEdges < 1 {
		g.MaxEdges = 500 // Default max edges
	}

	if g.MaxEdges > 5000 {
		g.MaxEdges = 5000 // Cap max edges
	}

	if g.Timeout == 0 {
		g.Timeout = 30 * time.Second // Default timeout
	}

	if g.Timeout > 5*time.Minute {
		g.Timeout = 5 * time.Minute // Cap timeout
	}
}

// MatchesTemporalBounds reports whether the given createdAt timestamp falls
// within the temporal window defined by CreatedAfter and CreatedBefore.
// A zero value for either bound means that bound is unconstrained.
func (g *GraphBounds) MatchesTemporalBounds(createdAt time.Time) bool {
	if !g.CreatedAfter.IsZero() && !createdAt.After(g.CreatedAfter) {
		return false
	}
	if !g.CreatedBefore.IsZero() && !createdAt.Before(g.CreatedBefore) {
		return false
	}
	return true
}

// GraphResult represents the result of a graph traversal operation.
type GraphResult struct {
	// Nodes is the list of memory IDs in the graph.
	Nodes []string

	// Edges represents connections between nodes (from -> to).
	Edges []GraphEdge

	// BoundsReached indicates which bounds were hit during traversal.
	BoundsReached []string
}

// GraphEdge represents a directed edge in the memory graph.
type GraphEdge struct {
	// From is the source memory ID.
	From string

	// To is the target memory ID.
	To string

	// RelationType is the type of relationship (e.g., "references", "follows").
	RelationType string

	// Weight is the strength of the relationship (0.0 to 1.0).
	Weight float64
}

// TraversalResult represents a memory found via graph traversal through the
// entity relationship graph (memory → entities → relationships → entities → memory).
type TraversalResult struct {
	// Memory is the discovered memory node.
	Memory *types.Memory

	// HopDistance is the number of entity-relationship hops from the starting memory.
	// A distance of 1 means the memory shares an entity directly with the start.
	// A distance of 2 means it is reachable via one intermediate entity relationship.
	HopDistance int

	// SharedEntities contains the names of entities that connect this memory
	// to the traversal path. Useful for explaining why a memory was surfaced.
	SharedEntities []string
}
