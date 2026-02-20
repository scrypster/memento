package types_test

import (
	"testing"

	"github.com/scrypster/memento/pkg/types"
)

// TestIsValidEntityType_AllValidTypes tests that all 22 entity types are recognized as valid
func TestIsValidEntityType_AllValidTypes(t *testing.T) {
	validEntityTypes := []string{
		// Core entity types
		types.EntityTypePerson,
		types.EntityTypeOrganization,
		types.EntityTypeProject,
		types.EntityTypeLocation,
		types.EntityTypeEvent,
		// Document and content types
		types.EntityTypeDocument,
		types.EntityTypeNote,
		types.EntityTypeFile,
		types.EntityTypeURL,
		types.EntityTypeEmail,
		types.EntityTypeMessage,
		// Knowledge types
		types.EntityTypeConcept,
		types.EntityTypeTask,
		// Technical types
		types.EntityTypeRepository,
		types.EntityTypeCodeSnippet,
		types.EntityTypeAPI,
		types.EntityTypeDatabase,
		types.EntityTypeServer,
		// Development types
		types.EntityTypeTool,
		types.EntityTypeFramework,
		types.EntityTypeLanguage,
		types.EntityTypeLibrary,
	}

	for _, entityType := range validEntityTypes {
		t.Run("valid_"+entityType, func(t *testing.T) {
			if !types.IsValidEntityType(entityType) {
				t.Errorf("IsValidEntityType(%q) = false, want true", entityType)
			}
		})
	}
}

// TestIsValidEntityType_InvalidTypes tests that invalid entity types are rejected
func TestIsValidEntityType_InvalidTypes(t *testing.T) {
	invalidTypes := []string{
		"",                    // empty string
		"PERSON",              // uppercase
		"Person",              // mixed case
		"unknown",             // unknown type
		"foo",                 // random string
		" person",             // leading whitespace
		"person ",             // trailing whitespace
		"per",                 // prefix of valid type
		"person_type",         // suffix addition
		"_person",             // prefix addition
		"code_snippet_extra",  // extended valid type
		"123",                 // numeric
		"person!",             // special character
	}

	for _, invalidType := range invalidTypes {
		t.Run("invalid_"+invalidType, func(t *testing.T) {
			if types.IsValidEntityType(invalidType) {
				t.Errorf("IsValidEntityType(%q) = true, want false", invalidType)
			}
		})
	}
}

// TestIsValidEntityType_CaseSensitivity tests that entity type validation is case-sensitive
func TestIsValidEntityType_CaseSensitivity(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"person", true},
		{"Person", false},
		{"PERSON", false},
		{"pErSoN", false},
		{"organization", true},
		{"Organization", false},
		{"ORGANIZATION", false},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := types.IsValidEntityType(tc.input)
			if result != tc.expected {
				t.Errorf("IsValidEntityType(%q) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}

// TestIsValidRelationshipType_AllValidTypes tests that all valid relationship types are recognized
func TestIsValidRelationshipType_AllValidTypes(t *testing.T) {
	validRelationshipTypes := []string{
		// Symmetric relationships
		types.RelUses, types.RelUsedBy,
		types.RelKnows, types.RelKnownBy,
		types.RelWorksWith,
		types.RelMarriedTo,
		types.RelFriendOf,
		types.RelColleagueOf,
		types.RelConflictsWith,
		types.RelSiblingOf,
		types.RelPartnersWith,
		// Employment & org structure
		types.RelEmployedBy, types.RelEmploys,
		types.RelManages, types.RelManagedBy,
		types.RelReportsTo,
		types.RelLeads, types.RelLedBy,
		types.RelMemberOf, types.RelHasMember,
		// Ownership & creation
		types.RelOwns, types.RelOwnedBy,
		types.RelFounded, types.RelFoundedBy,
		types.RelCreates, types.RelCreatedBy,
		// Service & supply
		types.RelProvides, types.RelProvidedBy,
		// Contribution
		types.RelContributesTo,
		// Hierarchical
		types.RelParentOf, types.RelChildOf,
		types.RelContains, types.RelBelongsTo,
		// Technical
		types.RelDependsOn, types.RelRequiredBy,
		types.RelBlocks, types.RelBlockedBy,
		types.RelImplements,
		types.RelAddresses,
		types.RelSupersedes,
		types.RelReferences,
		types.RelDocuments,
		types.RelWorksOn,
		// Generic
		types.RelRelatesTo,
	}

	for _, relType := range validRelationshipTypes {
		t.Run("valid_"+relType, func(t *testing.T) {
			if !types.IsValidRelationshipType(relType) {
				t.Errorf("IsValidRelationshipType(%q) = false, want true", relType)
			}
		})
	}
}

// TestIsValidRelationshipType_InvalidTypes tests that invalid relationship types are rejected
func TestIsValidRelationshipType_InvalidTypes(t *testing.T) {
	invalidTypes := []string{
		"",                    // empty string
		"KNOWS",               // uppercase
		"Knows",               // mixed case
		"unknown_rel",         // unknown type
		"foo",                 // random string
		" works_with",         // leading whitespace
		"employs ",            // trailing whitespace
		"work",                 // prefix of valid type
		"works_with_extra",    // extended valid type
		"_manages",            // prefix addition
		"123",                 // numeric
		"works-with",          // hyphen instead of underscore
		"manages!",            // special character
	}

	for _, invalidType := range invalidTypes {
		t.Run("invalid_"+invalidType, func(t *testing.T) {
			if types.IsValidRelationshipType(invalidType) {
				t.Errorf("IsValidRelationshipType(%q) = true, want false", invalidType)
			}
		})
	}
}

// TestIsValidRelationshipType_CaseSensitivity tests that relationship type validation is case-sensitive
func TestIsValidRelationshipType_CaseSensitivity(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"knows", true},
		{"Knows", false},
		{"KNOWS", false},
		{"kNoWs", false},
		{"works_with", true},
		{"Works_With", false},
		{"WORKS_WITH", false},
		{"employs", true},
		{"Employs", false},
		{"EMPLOYS", false},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := types.IsValidRelationshipType(tc.input)
			if result != tc.expected {
				t.Errorf("IsValidRelationshipType(%q) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}

// TestIsValidMemoryType_AllValidTypes tests that all valid memory types are recognized
func TestIsValidMemoryType_AllValidTypes(t *testing.T) {
	validMemoryTypes := []string{
		types.MemoryTypeDecision,
		types.MemoryTypeProcess,
		types.MemoryTypeConcept,
		types.MemoryTypeEvent,
		types.MemoryTypePerson,
		types.MemoryTypeSystem,
		types.MemoryTypeRule,
		types.MemoryTypeProject,
		types.MemoryTypeEpic,
		types.MemoryTypePhase,
		types.MemoryTypeMilestone,
		types.MemoryTypeTask,
		types.MemoryTypeStep,
	}

	for _, memType := range validMemoryTypes {
		t.Run("valid_"+memType, func(t *testing.T) {
			if !types.IsValidMemoryType(memType) {
				t.Errorf("IsValidMemoryType(%q) = false, want true", memType)
			}
		})
	}
}

// TestIsValidMemoryType_InvalidTypes tests that invalid memory types are rejected
func TestIsValidMemoryType_InvalidTypes(t *testing.T) {
	invalidTypes := []string{
		"",                 // empty string
		"DECISION",         // uppercase
		"Decision",         // mixed case
		"unknown_type",     // unknown type
		"foo",              // random string
		" process",         // leading whitespace
		"concept ",         // trailing whitespace
		"dec",              // prefix of valid type
		"decision_item",    // extended valid type
		"_task",            // prefix addition
		"123",              // numeric
		"decision!",        // special character
		"memory",           // common word but not valid type
	}

	for _, invalidType := range invalidTypes {
		t.Run("invalid_"+invalidType, func(t *testing.T) {
			if types.IsValidMemoryType(invalidType) {
				t.Errorf("IsValidMemoryType(%q) = true, want false", invalidType)
			}
		})
	}
}

// TestIsValidMemoryType_CaseSensitivity tests that memory type validation is case-sensitive
func TestIsValidMemoryType_CaseSensitivity(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"decision", true},
		{"Decision", false},
		{"DECISION", false},
		{"dEcIsIoN", false},
		{"process", true},
		{"Process", false},
		{"PROCESS", false},
		{"task", true},
		{"Task", false},
		{"TASK", false},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := types.IsValidMemoryType(tc.input)
			if result != tc.expected {
				t.Errorf("IsValidMemoryType(%q) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}

// TestIsValidStateTransition_AllValidTransitions tests all documented valid state transitions
func TestIsValidStateTransition_AllValidTransitions(t *testing.T) {
	validTransitions := []struct {
		name  string
		from  string
		to    string
	}{
		// Empty to planning or superseded
		{"empty_to_planning", "", types.StatePlanning},
		{"empty_to_superseded", "", types.StateSuperseded},
		// Planning transitions
		{"planning_to_active", types.StatePlanning, types.StateActive},
		{"planning_to_cancelled", types.StatePlanning, types.StateCancelled},
		// Active transitions
		{"active_to_paused", types.StateActive, types.StatePaused},
		{"active_to_blocked", types.StateActive, types.StateBlocked},
		{"active_to_completed", types.StateActive, types.StateCompleted},
		{"active_to_cancelled", types.StateActive, types.StateCancelled},
		{"active_to_superseded", types.StateActive, types.StateSuperseded},
		// Paused transitions
		{"paused_to_active", types.StatePaused, types.StateActive},
		{"paused_to_cancelled", types.StatePaused, types.StateCancelled},
		{"paused_to_superseded", types.StatePaused, types.StateSuperseded},
		// Blocked transitions
		{"blocked_to_active", types.StateBlocked, types.StateActive},
		{"blocked_to_cancelled", types.StateBlocked, types.StateCancelled},
		{"blocked_to_superseded", types.StateBlocked, types.StateSuperseded},
		// Completed transitions
		{"completed_to_archived", types.StateCompleted, types.StateArchived},
		{"completed_to_superseded", types.StateCompleted, types.StateSuperseded},
		// Cancelled transitions
		{"cancelled_to_archived", types.StateCancelled, types.StateArchived},
		{"cancelled_to_superseded", types.StateCancelled, types.StateSuperseded},
		// Superseded transitions
		{"superseded_to_archived", types.StateSuperseded, types.StateArchived},
	}

	for _, tt := range validTransitions {
		t.Run(tt.name, func(t *testing.T) {
			if !types.IsValidStateTransition(tt.from, tt.to) {
				t.Errorf("IsValidStateTransition(%q, %q) = false, want true", tt.from, tt.to)
			}
		})
	}
}

// TestIsValidStateTransition_AllInvalidTransitions tests all documented invalid state transitions
func TestIsValidStateTransition_AllInvalidTransitions(t *testing.T) {
	invalidTransitions := []struct {
		name string
		from string
		to   string
	}{
		// Empty invalid transitions
		{"empty_to_empty", "", ""},
		{"empty_to_active", "", types.StateActive},
		{"empty_to_paused", "", types.StatePaused},
		{"empty_to_blocked", "", types.StateBlocked},
		{"empty_to_completed", "", types.StateCompleted},
		{"empty_to_cancelled", "", types.StateCancelled},
		{"empty_to_archived", "", types.StateArchived},
		// Planning invalid transitions
		{"planning_to_paused", types.StatePlanning, types.StatePaused},
		{"planning_to_blocked", types.StatePlanning, types.StateBlocked},
		{"planning_to_completed", types.StatePlanning, types.StateCompleted},
		{"planning_to_archived", types.StatePlanning, types.StateArchived},
		{"planning_to_superseded", types.StatePlanning, types.StateSuperseded},
		// Active invalid transitions (backward)
		{"active_to_planning", types.StateActive, types.StatePlanning},
		// Paused invalid transitions
		{"paused_to_planning", types.StatePaused, types.StatePlanning},
		{"paused_to_blocked", types.StatePaused, types.StateBlocked},
		{"paused_to_completed", types.StatePaused, types.StateCompleted},
		{"paused_to_archived", types.StatePaused, types.StateArchived},
		// Blocked invalid transitions
		{"blocked_to_planning", types.StateBlocked, types.StatePlanning},
		{"blocked_to_paused", types.StateBlocked, types.StatePaused},
		{"blocked_to_completed", types.StateBlocked, types.StateCompleted},
		{"blocked_to_archived", types.StateBlocked, types.StateArchived},
		// Completed invalid transitions
		{"completed_to_planning", types.StateCompleted, types.StatePlanning},
		{"completed_to_active", types.StateCompleted, types.StateActive},
		{"completed_to_paused", types.StateCompleted, types.StatePaused},
		{"completed_to_blocked", types.StateCompleted, types.StateBlocked},
		{"completed_to_completed", types.StateCompleted, types.StateCompleted},
		{"completed_to_cancelled", types.StateCompleted, types.StateCancelled},
		// Cancelled invalid transitions
		{"cancelled_to_planning", types.StateCancelled, types.StatePlanning},
		{"cancelled_to_active", types.StateCancelled, types.StateActive},
		{"cancelled_to_paused", types.StateCancelled, types.StatePaused},
		{"cancelled_to_blocked", types.StateCancelled, types.StateBlocked},
		{"cancelled_to_completed", types.StateCancelled, types.StateCompleted},
		{"cancelled_to_cancelled", types.StateCancelled, types.StateCancelled},
		// Superseded invalid transitions
		{"superseded_to_planning", types.StateSuperseded, types.StatePlanning},
		{"superseded_to_active", types.StateSuperseded, types.StateActive},
		{"superseded_to_paused", types.StateSuperseded, types.StatePaused},
		{"superseded_to_blocked", types.StateSuperseded, types.StateBlocked},
		{"superseded_to_completed", types.StateSuperseded, types.StateCompleted},
		{"superseded_to_cancelled", types.StateSuperseded, types.StateCancelled},
		{"superseded_to_superseded", types.StateSuperseded, types.StateSuperseded},
		// Archived invalid transitions (terminal state)
		{"archived_to_planning", types.StateArchived, types.StatePlanning},
		{"archived_to_active", types.StateArchived, types.StateActive},
		{"archived_to_paused", types.StateArchived, types.StatePaused},
		{"archived_to_blocked", types.StateArchived, types.StateBlocked},
		{"archived_to_completed", types.StateArchived, types.StateCompleted},
		{"archived_to_cancelled", types.StateArchived, types.StateCancelled},
		{"archived_to_superseded", types.StateArchived, types.StateSuperseded},
		{"archived_to_archived", types.StateArchived, types.StateArchived},
		// Unknown state
		{"unknown_to_planning", "unknown_state", types.StatePlanning},
		{"planning_to_unknown", types.StatePlanning, "unknown_state"},
	}

	for _, tt := range invalidTransitions {
		t.Run(tt.name, func(t *testing.T) {
			if types.IsValidStateTransition(tt.from, tt.to) {
				t.Errorf("IsValidStateTransition(%q, %q) = true, want false", tt.from, tt.to)
			}
		})
	}
}

// TestIsValidStateTransition_TerminalStates tests that archived is truly terminal
func TestIsValidStateTransition_TerminalStates(t *testing.T) {
	// Archived is terminal - no transitions out
	terminalState := types.StateArchived
	allStates := []string{
		types.StatePlanning,
		types.StateActive,
		types.StatePaused,
		types.StateBlocked,
		types.StateCompleted,
		types.StateCancelled,
		types.StateArchived,
		types.StateSuperseded,
	}

	for _, state := range allStates {
		t.Run("archived_cannot_transition_to_"+state, func(t *testing.T) {
			if types.IsValidStateTransition(terminalState, state) {
				t.Errorf("IsValidStateTransition(%q, %q) = true, want false (archived is terminal)", terminalState, state)
			}
		})
	}
}

// TestIsValidStateTransition_EdgeCases tests edge cases for state transitions
func TestIsValidStateTransition_EdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		from     string
		to       string
		expected bool
	}{
		// Empty string handling
		{"transition_to_empty_string", types.StatePlanning, "", false},
		{"transition_from_empty_to_empty", "", "", false},
		// Self transitions (looping)
		{"planning_to_planning", types.StatePlanning, types.StatePlanning, false},
		{"active_to_active", types.StateActive, types.StateActive, false},
		{"paused_to_paused", types.StatePaused, types.StatePaused, false},
		// Case sensitivity
		{"uppercase_state", "PLANNING", types.StateActive, false},
		{"mixed_case_state", "Planning", types.StateActive, false},
		// Whitespace handling
		{"whitespace_from", " planning", types.StateActive, false},
		{"whitespace_to", types.StatePlanning, "active ", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := types.IsValidStateTransition(tc.from, tc.to)
			if result != tc.expected {
				t.Errorf("IsValidStateTransition(%q, %q) = %v, want %v", tc.from, tc.to, result, tc.expected)
			}
		})
	}
}

// TestValidTypesSlices verifies that the ValidEntityTypes, ValidRelationshipTypes,
// and ValidMemoryTypes slices are properly populated
func TestValidTypesSlices(t *testing.T) {
	t.Run("ValidEntityTypes_count", func(t *testing.T) {
		expectedCount := 22
		if len(types.ValidEntityTypes) != expectedCount {
			t.Errorf("ValidEntityTypes length = %d, want %d", len(types.ValidEntityTypes), expectedCount)
		}
	})

	t.Run("ValidRelationshipTypes_count", func(t *testing.T) {
		expectedCount := 44
		if len(types.ValidRelationshipTypes) != expectedCount {
			t.Errorf("ValidRelationshipTypes length = %d, want %d", len(types.ValidRelationshipTypes), expectedCount)
		}
	})

	t.Run("ValidMemoryTypes_count", func(t *testing.T) {
		expectedCount := 13
		if len(types.ValidMemoryTypes) != expectedCount {
			t.Errorf("ValidMemoryTypes length = %d, want %d", len(types.ValidMemoryTypes), expectedCount)
		}
	})

	t.Run("ValidLifecycleStates_count", func(t *testing.T) {
		expectedCount := 8
		if len(types.ValidLifecycleStates) != expectedCount {
			t.Errorf("ValidLifecycleStates length = %d, want %d", len(types.ValidLifecycleStates), expectedCount)
		}
	})
}

// TestValidTypesSlices_NoEmptyValues verifies that validation slices don't contain empty strings
func TestValidTypesSlices_NoEmptyValues(t *testing.T) {
	t.Run("ValidEntityTypes_no_empty", func(t *testing.T) {
		for i, entityType := range types.ValidEntityTypes {
			if entityType == "" {
				t.Errorf("ValidEntityTypes[%d] is empty string", i)
			}
		}
	})

	t.Run("ValidRelationshipTypes_no_empty", func(t *testing.T) {
		for i, relType := range types.ValidRelationshipTypes {
			if relType == "" {
				t.Errorf("ValidRelationshipTypes[%d] is empty string", i)
			}
		}
	})

	t.Run("ValidMemoryTypes_no_empty", func(t *testing.T) {
		for i, memType := range types.ValidMemoryTypes {
			if memType == "" {
				t.Errorf("ValidMemoryTypes[%d] is empty string", i)
			}
		}
	})

	t.Run("ValidLifecycleStates_no_empty", func(t *testing.T) {
		for i, state := range types.ValidLifecycleStates {
			if state == "" {
				t.Errorf("ValidLifecycleStates[%d] is empty string", i)
			}
		}
	})
}

// TestValidTypesSlices_NoDuplicates verifies that validation slices don't contain duplicates
func TestValidTypesSlices_NoDuplicates(t *testing.T) {
	t.Run("ValidEntityTypes_no_duplicates", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, entityType := range types.ValidEntityTypes {
			if seen[entityType] {
				t.Errorf("ValidEntityTypes contains duplicate: %q", entityType)
			}
			seen[entityType] = true
		}
	})

	t.Run("ValidRelationshipTypes_no_duplicates", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, relType := range types.ValidRelationshipTypes {
			if seen[relType] {
				t.Errorf("ValidRelationshipTypes contains duplicate: %q", relType)
			}
			seen[relType] = true
		}
	})

	t.Run("ValidMemoryTypes_no_duplicates", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, memType := range types.ValidMemoryTypes {
			if seen[memType] {
				t.Errorf("ValidMemoryTypes contains duplicate: %q", memType)
			}
			seen[memType] = true
		}
	})

	t.Run("ValidLifecycleStates_no_duplicates", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, state := range types.ValidLifecycleStates {
			if seen[state] {
				t.Errorf("ValidLifecycleStates contains duplicate: %q", state)
			}
			seen[state] = true
		}
	})
}
