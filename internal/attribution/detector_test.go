package attribution

import (
	"os"
	"testing"
)

func TestDetectAgentFromMementoAgentName(t *testing.T) {
	os.Setenv("MEMENTO_AGENT_NAME", "my-agent")
	defer os.Unsetenv("MEMENTO_AGENT_NAME")
	got := detectAgentUncached()
	if got != "my-agent" {
		t.Errorf("expected my-agent, got %s", got)
	}
}

func TestDetectAgentFromMementoUser(t *testing.T) {
	os.Unsetenv("MEMENTO_AGENT_NAME")
	os.Setenv("MEMENTO_USER", "mjbonanno")
	defer os.Unsetenv("MEMENTO_USER")
	got := detectAgentUncached()
	if got != "mjbonanno" {
		t.Errorf("expected mjbonanno, got %s", got)
	}
}

func TestDetectAgentFallback(t *testing.T) {
	os.Unsetenv("MEMENTO_AGENT_NAME")
	os.Unsetenv("MEMENTO_USER")
	got := detectAgentUncached()
	// Should be either a real git name or "unknown" — not empty
	if got == "" {
		t.Error("expected non-empty result")
	}
}
