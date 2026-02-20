package attribution

import (
	"os"
	"testing"
)

func TestDetectAgentFromMementoAgentName(t *testing.T) {
	t.Setenv("MEMENTO_AGENT_NAME", "my-agent")
	got := detectAgentUncached()
	if got != "my-agent" {
		t.Errorf("expected my-agent, got %s", got)
	}
}

func TestDetectAgentFromMementoUser(t *testing.T) {
	_ = os.Unsetenv("MEMENTO_AGENT_NAME")
	t.Setenv("MEMENTO_USER", "mjbonanno")
	got := detectAgentUncached()
	if got != "mjbonanno" {
		t.Errorf("expected mjbonanno, got %s", got)
	}
}

func TestDetectAgentFallback(t *testing.T) {
	_ = os.Unsetenv("MEMENTO_AGENT_NAME")
	_ = os.Unsetenv("MEMENTO_USER")
	got := detectAgentUncached()
	// Should be either a real git name or "unknown" — not empty
	if got == "" {
		t.Error("expected non-empty result")
	}
}
