package types

// Lifecycle state constants for memory work tracking
const (
	StatePlanning   = "planning"   // Being planned or designed
	StateActive     = "active"     // Currently being worked on
	StatePaused     = "paused"     // Temporarily paused or on hold
	StateBlocked    = "blocked"    // Blocked by dependencies or issues
	StateCompleted  = "completed"  // Finished but preserved for reference
	StateCancelled  = "cancelled"  // Cancelled or abandoned
	StateArchived   = "archived"   // Archived for historical reference
	StateSuperseded = "superseded" // Superseded by a newer version
)

// ValidLifecycleStates contains all valid lifecycle state values
var ValidLifecycleStates = []string{
	StatePlanning,
	StateActive,
	StatePaused,
	StateBlocked,
	StateCompleted,
	StateCancelled,
	StateArchived,
	StateSuperseded,
}

// IsValidLifecycleState checks if the given state is a valid lifecycle state.
// Empty string is considered valid (means state not set).
func IsValidLifecycleState(state string) bool {
	if state == "" {
		return true // Empty is valid (not set)
	}

	for _, validState := range ValidLifecycleStates {
		if state == validState {
			return true
		}
	}
	return false
}

// IsValidStateTransition validates state transitions according to the state machine
// defined in design doc Appendix B (Opus Issue #6).
//
// Valid transitions:
//
//	(empty) -> planning
//	planning -> active | cancelled
//	active -> paused | blocked | completed | cancelled | superseded
//	paused -> active | cancelled | superseded
//	blocked -> active | cancelled | superseded
//	completed -> archived | superseded
//	cancelled -> archived | superseded
//	superseded -> archived
//	archived -> (terminal, no transitions out)
func IsValidStateTransition(currentState, newState string) bool {
	// If trying to transition to empty state, reject it
	if newState == "" {
		return false
	}

	// State machine transition rules
	switch currentState {
	case "": // empty state
		return newState == StatePlanning || newState == StateSuperseded

	case StatePlanning:
		return newState == StateActive || newState == StateCancelled

	case StateActive:
		return newState == StatePaused || newState == StateBlocked ||
			newState == StateCompleted || newState == StateCancelled || newState == StateSuperseded

	case StatePaused:
		return newState == StateActive || newState == StateCancelled || newState == StateSuperseded

	case StateBlocked:
		return newState == StateActive || newState == StateCancelled || newState == StateSuperseded

	case StateCompleted:
		return newState == StateArchived || newState == StateSuperseded

	case StateCancelled:
		return newState == StateArchived || newState == StateSuperseded

	case StateSuperseded:
		return newState == StateArchived

	case StateArchived:
		return false // Terminal state, no transitions out

	default:
		return false // Unknown current state
	}
}
