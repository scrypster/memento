// Package types defines the core data structures for the Memento memory system.
// These types represent memories, entities, relationships, and their metadata
// following the v2.0 architecture with async enrichment support.
package types

// MemoryStatus represents the overall processing status of a memory.
type MemoryStatus string

// EnrichmentStatus represents the status of a specific enrichment task.
type EnrichmentStatus string

// Memory overall status constants
const (
	// StatusPending indicates memory is newly created, pending enrichment
	StatusPending MemoryStatus = "pending"

	// StatusProcessing indicates memory is currently being processed
	StatusProcessing MemoryStatus = "processing"

	// StatusEnriched indicates memory has been successfully enriched
	StatusEnriched MemoryStatus = "enriched"

	// StatusFailed indicates memory enrichment failed after retries
	StatusFailed MemoryStatus = "failed"
)

// Enrichment task status constants
const (
	// EnrichmentPending indicates enrichment task is queued
	EnrichmentPending EnrichmentStatus = "pending"

	// EnrichmentProcessing indicates enrichment task is in progress
	EnrichmentProcessing EnrichmentStatus = "processing"

	// EnrichmentCompleted indicates enrichment task completed successfully
	EnrichmentCompleted EnrichmentStatus = "completed"

	// EnrichmentFailed indicates enrichment task failed
	EnrichmentFailed EnrichmentStatus = "failed"

	// EnrichmentSkipped indicates enrichment task was skipped
	EnrichmentSkipped EnrichmentStatus = "skipped"
)

// Entity type constants - 20+ types for comprehensive entity modeling
const (
	// Core entity types
	EntityTypePerson       = "person"
	EntityTypeOrganization = "organization"
	EntityTypeProject      = "project"
	EntityTypeLocation     = "location"
	EntityTypeEvent        = "event"

	// Document and content types
	EntityTypeDocument    = "document"
	EntityTypeNote        = "note"
	EntityTypeFile        = "file"
	EntityTypeURL         = "url"
	EntityTypeEmail       = "email"
	EntityTypeMessage     = "message"

	// Knowledge types
	EntityTypeConcept = "concept"
	EntityTypeTask    = "task"

	// Technical types
	EntityTypeRepository  = "repository"
	EntityTypeCodeSnippet = "code_snippet"
	EntityTypeAPI         = "api"
	EntityTypeDatabase    = "database"
	EntityTypeServer      = "server"

	// Development types
	EntityTypeTool      = "tool"
	EntityTypeFramework = "framework"
	EntityTypeLanguage  = "language"
	EntityTypeLibrary   = "library"
)

// ValidEntityTypes is a slice of all valid entity types for validation
var ValidEntityTypes = []string{
	EntityTypePerson,
	EntityTypeOrganization,
	EntityTypeProject,
	EntityTypeLocation,
	EntityTypeEvent,
	EntityTypeDocument,
	EntityTypeNote,
	EntityTypeFile,
	EntityTypeURL,
	EntityTypeEmail,
	EntityTypeMessage,
	EntityTypeConcept,
	EntityTypeTask,
	EntityTypeRepository,
	EntityTypeCodeSnippet,
	EntityTypeAPI,
	EntityTypeDatabase,
	EntityTypeServer,
	EntityTypeTool,
	EntityTypeFramework,
	EntityTypeLanguage,
	EntityTypeLibrary,
}

// Relationship type constants - bidirectional and asymmetric relationships
const (
	// Bidirectional relationships (symmetric)
	RelUses         = "uses"          // Entity uses another entity
	RelUsedBy       = "used_by"       // Inverse of uses
	RelKnows        = "knows"         // Person knows another person
	RelKnownBy      = "known_by"      // Inverse of knows
	RelWorksWith    = "works_with"    // Collaborative relationship
	RelMarriedTo    = "married_to"    // Marriage relationship
	RelFriendOf     = "friend_of"     // Friendship relationship
	RelColleagueOf  = "colleague_of"  // Professional relationship
	RelConflictsWith = "conflicts_with" // Conflicting relationship
	RelSiblingOf    = "sibling_of"    // Sibling relationship
	RelEmployedBy   = "employed_by"   // Employment relationship
	RelRelatesTo    = "relates_to"    // Generic relationship

	// Asymmetric relationship pairs
	RelParentOf   = "parent_of"   // Parent-child relationship
	RelChildOf    = "child_of"    // Child-parent relationship
	RelDependsOn  = "depends_on"  // Dependency relationship
	RelRequiredBy = "required_by" // Inverse dependency
	RelContains   = "contains"    // Container relationship
	RelBelongsTo  = "belongs_to"  // Membership relationship
	RelBlocks     = "blocks"      // Blocking relationship
	RelBlockedBy  = "blocked_by"  // Inverse blocking

	// One-way relationships
	RelImplements = "implements" // Implementation relationship
	RelAddresses  = "addresses"  // Addresses/solves relationship
	RelSupersedes = "supersedes" // Replacement relationship
	RelReferences = "references" // Reference relationship
	RelDocuments  = "documents"  // Documentation relationship
	RelWorksOn    = "works_on"   // Person works on project/task

	// Employment & org structure
	RelEmploys   = "employs"    // Org employs person (inverse of employed_by)
	RelManages   = "manages"    // Person/org manages another
	RelManagedBy = "managed_by" // Inverse of manages
	RelReportsTo = "reports_to" // Employee reports to manager
	RelLeads     = "leads"      // Person leads team/project
	RelLedBy     = "led_by"     // Inverse of leads
	RelMemberOf  = "member_of"  // Person/org is member of group
	RelHasMember = "has_member" // Group has a member

	// Ownership & creation
	RelOwns      = "owns"        // Entity owns another
	RelOwnedBy   = "owned_by"    // Inverse of owns
	RelFounded   = "founded"     // Person/org founded another org
	RelFoundedBy = "founded_by"  // Org was founded by person/org
	RelCreates   = "creates"     // Entity creates artifact
	RelCreatedBy = "created_by"  // Artifact created by entity

	// Service & supply
	RelProvides   = "provides"    // Entity provides service/resource
	RelProvidedBy = "provided_by" // Service provided by entity

	// Collaboration
	RelPartnersWith   = "partners_with"   // Partnership (bidirectional)
	RelContributesTo  = "contributes_to"  // Entity contributes to another
)

// ValidRelationshipTypes is a slice of all valid relationship types for validation
var ValidRelationshipTypes = []string{
	// Symmetric / bidirectional
	RelUses, RelUsedBy,
	RelKnows, RelKnownBy,
	RelWorksWith,
	RelMarriedTo,
	RelFriendOf,
	RelColleagueOf,
	RelConflictsWith,
	RelSiblingOf,
	RelPartnersWith,
	// Employment & org structure
	RelEmployedBy, RelEmploys,
	RelManages, RelManagedBy,
	RelReportsTo,
	RelLeads, RelLedBy,
	RelMemberOf, RelHasMember,
	// Ownership & creation
	RelOwns, RelOwnedBy,
	RelFounded, RelFoundedBy,
	RelCreates, RelCreatedBy,
	// Service & supply
	RelProvides, RelProvidedBy,
	// Contribution
	RelContributesTo,
	// Hierarchical
	RelParentOf, RelChildOf,
	RelContains, RelBelongsTo,
	// Technical
	RelDependsOn, RelRequiredBy,
	RelBlocks, RelBlockedBy,
	RelImplements,
	RelAddresses,
	RelSupersedes,
	RelReferences,
	RelDocuments,
	RelWorksOn,
	// Generic
	RelRelatesTo,
}

// IsValidEntityType checks if the given entity type is valid
func IsValidEntityType(entityType string) bool {
	for _, validType := range ValidEntityTypes {
		if validType == entityType {
			return true
		}
	}
	return false
}

// IsValidRelationshipType checks if the given relationship type is valid
func IsValidRelationshipType(relType string) bool {
	for _, validType := range ValidRelationshipTypes {
		if validType == relType {
			return true
		}
	}
	return false
}

// Memory type constants - classify the purpose/nature of a memory
const (
	MemoryTypeDecision   = "decision"    // Important choices or decisions made
	MemoryTypeProcess    = "process"     // Step-by-step procedures or workflows
	MemoryTypeConcept    = "concept"     // Ideas, principles, or theories
	MemoryTypeEvent      = "event"       // Meetings, incidents, or occurrences
	MemoryTypePerson     = "person"      // Information about people
	MemoryTypeSystem     = "system"      // Systems, architectures, or infrastructure
	MemoryTypeRule       = "rule"        // Business rules or technical standards
	MemoryTypeProject    = "project"     // Project information or descriptions
	MemoryTypeEpic       = "epic"        // Large initiatives or features
	MemoryTypePhase      = "phase"       // Project phases or milestones
	MemoryTypeMilestone  = "milestone"   // Important checkpoints
	MemoryTypeTask       = "task"        // Individual work items
	MemoryTypeStep       = "step"        // Sub-steps or sub-tasks
)

// ValidMemoryTypes is a slice of all valid memory types for validation
var ValidMemoryTypes = []string{
	MemoryTypeDecision,
	MemoryTypeProcess,
	MemoryTypeConcept,
	MemoryTypeEvent,
	MemoryTypePerson,
	MemoryTypeSystem,
	MemoryTypeRule,
	MemoryTypeProject,
	MemoryTypeEpic,
	MemoryTypePhase,
	MemoryTypeMilestone,
	MemoryTypeTask,
	MemoryTypeStep,
}

// IsValidMemoryType checks if the given memory type is valid
func IsValidMemoryType(memoryType string) bool {
	for _, validType := range ValidMemoryTypes {
		if validType == memoryType {
			return true
		}
	}
	return false
}
