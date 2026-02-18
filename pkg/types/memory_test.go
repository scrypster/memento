package types_test

import (
	"testing"
	"time"

	"github.com/scrypster/memento/pkg/types"
)

// TestMemoryProvenanceFields verifies that provenance fields can be set and read back.
func TestMemoryProvenanceFields(t *testing.T) {
	m := types.Memory{}

	m.CreatedBy = "claude-opus-4"
	m.SessionID = "session-abc-123"
	m.SourceContext = map[string]interface{}{
		"file":   "notes.md",
		"offset": 42,
	}

	if m.CreatedBy != "claude-opus-4" {
		t.Errorf("expected CreatedBy %q, got %q", "claude-opus-4", m.CreatedBy)
	}
	if m.SessionID != "session-abc-123" {
		t.Errorf("expected SessionID %q, got %q", "session-abc-123", m.SessionID)
	}
	if m.SourceContext["file"] != "notes.md" {
		t.Errorf("expected SourceContext[file] %q, got %v", "notes.md", m.SourceContext["file"])
	}
	if m.SourceContext["offset"] != 42 {
		t.Errorf("expected SourceContext[offset] 42, got %v", m.SourceContext["offset"])
	}
}

// TestMemoryLifecycleFields verifies that lifecycle state fields can be set and read back.
func TestMemoryLifecycleFields(t *testing.T) {
	now := time.Now()
	m := types.Memory{}

	m.State = types.StateActive
	m.StateUpdatedAt = &now

	if m.State != types.StateActive {
		t.Errorf("expected State %q, got %q", types.StateActive, m.State)
	}
	if m.StateUpdatedAt == nil {
		t.Fatal("expected StateUpdatedAt to be non-nil")
	}
	if !m.StateUpdatedAt.Equal(now) {
		t.Errorf("expected StateUpdatedAt %v, got %v", now, *m.StateUpdatedAt)
	}
}

// TestMemoryQualitySignalFields verifies that quality signal fields can be set and read back.
func TestMemoryQualitySignalFields(t *testing.T) {
	now := time.Now()
	m := types.Memory{}

	m.AccessCount = 7
	m.LastAccessedAt = &now
	m.DecayScore = 0.85
	m.DecayUpdatedAt = &now

	if m.AccessCount != 7 {
		t.Errorf("expected AccessCount 7, got %d", m.AccessCount)
	}
	if m.LastAccessedAt == nil {
		t.Fatal("expected LastAccessedAt to be non-nil")
	}
	if !m.LastAccessedAt.Equal(now) {
		t.Errorf("expected LastAccessedAt %v, got %v", now, *m.LastAccessedAt)
	}
	if m.DecayScore != 0.85 {
		t.Errorf("expected DecayScore 0.85, got %f", m.DecayScore)
	}
	if m.DecayUpdatedAt == nil {
		t.Fatal("expected DecayUpdatedAt to be non-nil")
	}
	if !m.DecayUpdatedAt.Equal(now) {
		t.Errorf("expected DecayUpdatedAt %v, got %v", now, *m.DecayUpdatedAt)
	}
}

// TestMemoryNewFieldDefaults verifies that all new fields have correct zero values
// when a Memory is created without setting them.
func TestMemoryNewFieldDefaults(t *testing.T) {
	m := types.Memory{}

	// Lifecycle management
	if m.State != "" {
		t.Errorf("expected State to default to empty string, got %q", m.State)
	}
	if m.StateUpdatedAt != nil {
		t.Errorf("expected StateUpdatedAt to default to nil, got %v", m.StateUpdatedAt)
	}

	// Provenance tracking
	if m.CreatedBy != "" {
		t.Errorf("expected CreatedBy to default to empty string, got %q", m.CreatedBy)
	}
	if m.SessionID != "" {
		t.Errorf("expected SessionID to default to empty string, got %q", m.SessionID)
	}
	if m.SourceContext != nil {
		t.Errorf("expected SourceContext to default to nil, got %v", m.SourceContext)
	}

	// Quality signals
	if m.AccessCount != 0 {
		t.Errorf("expected AccessCount to default to 0, got %d", m.AccessCount)
	}
	if m.LastAccessedAt != nil {
		t.Errorf("expected LastAccessedAt to default to nil, got %v", m.LastAccessedAt)
	}
	if m.DecayScore != 0.0 {
		t.Errorf("expected DecayScore to default to 0.0, got %f", m.DecayScore)
	}
	if m.DecayUpdatedAt != nil {
		t.Errorf("expected DecayUpdatedAt to default to nil, got %v", m.DecayUpdatedAt)
	}
}

// TestMemoryStateValidation verifies that State field works with lifecycle state constants.
func TestMemoryStateValidation(t *testing.T) {
	m := types.Memory{}

	states := []string{
		types.StatePlanning,
		types.StateActive,
		types.StatePaused,
		types.StateBlocked,
		types.StateCompleted,
		types.StateCancelled,
		types.StateArchived,
	}

	for _, state := range states {
		m.State = state
		if !types.IsValidLifecycleState(m.State) {
			t.Errorf("expected State %q to be a valid lifecycle state", state)
		}
	}
}

// TestStateTransitionValidation_ValidTransitions tests all valid state transitions (Opus Issue #6).
func TestStateTransitionValidation_ValidTransitions(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		expected bool
	}{
		// Empty to planning
		{"empty_to_planning", "", types.StatePlanning, true},
		// Planning transitions
		{"planning_to_active", types.StatePlanning, types.StateActive, true},
		{"planning_to_cancelled", types.StatePlanning, types.StateCancelled, true},
		// Active transitions
		{"active_to_paused", types.StateActive, types.StatePaused, true},
		{"active_to_blocked", types.StateActive, types.StateBlocked, true},
		{"active_to_completed", types.StateActive, types.StateCompleted, true},
		{"active_to_cancelled", types.StateActive, types.StateCancelled, true},
		// Paused transitions
		{"paused_to_active", types.StatePaused, types.StateActive, true},
		{"paused_to_cancelled", types.StatePaused, types.StateCancelled, true},
		// Blocked transitions
		{"blocked_to_active", types.StateBlocked, types.StateActive, true},
		{"blocked_to_cancelled", types.StateBlocked, types.StateCancelled, true},
		// Terminal transitions
		{"completed_to_archived", types.StateCompleted, types.StateArchived, true},
		{"cancelled_to_archived", types.StateCancelled, types.StateArchived, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := types.IsValidStateTransition(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("expected IsValidStateTransition(%q, %q) = %v, got %v", tt.from, tt.to, tt.expected, result)
			}
		})
	}
}

// TestStateTransitionValidation_InvalidTransitions tests that invalid transitions are rejected.
func TestStateTransitionValidation_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
	}{
		// Planning invalid transitions
		{"planning_to_paused", types.StatePlanning, types.StatePaused},
		{"planning_to_blocked", types.StatePlanning, types.StateBlocked},
		{"planning_to_completed", types.StatePlanning, types.StateCompleted},
		{"planning_to_archived", types.StatePlanning, types.StateArchived},
		// Active invalid transitions
		{"active_to_planning", types.StateActive, types.StatePlanning},
		// Paused invalid transitions
		{"paused_to_planning", types.StatePaused, types.StatePlanning},
		{"paused_to_blocked", types.StatePaused, types.StateBlocked},
		{"paused_to_completed", types.StatePaused, types.StateCompleted},
		// Blocked invalid transitions
		{"blocked_to_planning", types.StateBlocked, types.StatePlanning},
		{"blocked_to_paused", types.StateBlocked, types.StatePaused},
		{"blocked_to_completed", types.StateBlocked, types.StateCompleted},
		// Completed invalid transitions (terminal)
		{"completed_to_planning", types.StateCompleted, types.StatePlanning},
		{"completed_to_active", types.StateCompleted, types.StateActive},
		{"completed_to_completed", types.StateCompleted, types.StateCompleted},
		// Cancelled invalid transitions
		{"cancelled_to_planning", types.StateCancelled, types.StatePlanning},
		{"cancelled_to_active", types.StateCancelled, types.StateActive},
		// Archived invalid transitions (terminal)
		{"archived_to_planning", types.StateArchived, types.StatePlanning},
		{"archived_to_active", types.StateArchived, types.StateActive},
		{"archived_to_archived", types.StateArchived, types.StateArchived},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := types.IsValidStateTransition(tt.from, tt.to)
			if result {
				t.Errorf("expected IsValidStateTransition(%q, %q) = false, got %v", tt.from, tt.to, result)
			}
		})
	}
}

// TestStateTransitionValidation_EmptyToEmpty should reject empty state transitions.
func TestStateTransitionValidation_EmptyToEmpty(t *testing.T) {
	result := types.IsValidStateTransition("", "")
	if result {
		t.Errorf("expected IsValidStateTransition(\"\", \"\") = false, got %v", result)
	}
}
