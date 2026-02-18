package types_test

import (
	"testing"

	"github.com/scrypster/memento/pkg/types"
)

func TestValidLifecycleStates(t *testing.T) {
	validStates := []string{
		"planning", "active", "paused", "blocked",
		"completed", "cancelled", "archived",
	}

	for _, state := range validStates {
		if !types.IsValidLifecycleState(state) {
			t.Errorf("Expected %s to be valid lifecycle state", state)
		}
	}
}

func TestInvalidLifecycleStates(t *testing.T) {
	invalidStates := []string{"invalid", "unknown", "test"}

	for _, state := range invalidStates {
		if types.IsValidLifecycleState(state) {
			t.Errorf("Expected %s to be invalid lifecycle state", state)
		}
	}
}

func TestEmptyStateIsValid(t *testing.T) {
	if !types.IsValidLifecycleState("") {
		t.Error("Empty state should be valid (means not set)")
	}
}
