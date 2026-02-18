package types

import "time"

// RelationshipMetadata contains metadata about relationship directionality.
// This enables bidirectional relationship modeling where relationships can be
// symmetric (e.g., "friend_of") or asymmetric (e.g., "parent_of"/"child_of").
type RelationshipMetadata struct {
	Bidirectional bool   `json:"bidirectional"`        // True if relationship is symmetric
	Inverse       string `json:"inverse,omitempty"`    // Inverse relationship type (e.g., "child_of" for "parent_of")
}

// Relationship represents a connection between two entities.
// Relationships have types (e.g., "works_on", "friend_of"), strength scores,
// and bidirectional metadata for semantic graph traversal.
type Relationship struct {
	// Core identification fields
	ID     string `json:"id"`      // Unique identifier (format: rel:uuid)
	FromID string `json:"from_id"` // Source entity ID
	ToID   string `json:"to_id"`   // Target entity ID
	Type   string `json:"type"`    // Relationship type (e.g., "works_on", "friend_of")

	// Relationship properties
	Strength  float64   `json:"strength,omitempty"`  // Relationship strength (0.0-1.0)
	CreatedAt time.Time `json:"created_at"`          // Creation timestamp
	UpdatedAt time.Time `json:"updated_at"`          // Last update timestamp

	// v2.0: Bidirectional metadata
	RelationshipMetadata RelationshipMetadata `json:"relationship_metadata"` // Bidirectional and inverse info

	// Additional context
	Metadata map[string]interface{} `json:"metadata,omitempty"` // Arbitrary relationship metadata
	Evidence []string               `json:"evidence,omitempty"` // Memory IDs that support this relationship
}

// IsBidirectional returns true if this relationship is symmetric.
// Bidirectional relationships imply the reverse relationship also exists
// (e.g., if Alice is friend_of Bob, then Bob is friend_of Alice).
func (r *Relationship) IsBidirectional() bool {
	return r.RelationshipMetadata.Bidirectional
}

// GetInverse returns the inverse relationship type.
// For bidirectional relationships, this is often the same as the forward type.
// For unidirectional relationships, this returns the complementary type
// (e.g., "parent_of" returns "child_of").
func (r *Relationship) GetInverse() string {
	return r.RelationshipMetadata.Inverse
}
