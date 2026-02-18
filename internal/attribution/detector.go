package attribution

import (
	"os"
	"os/exec"
	"strings"
	"sync"
)

var (
	cachedName string
	once       sync.Once
)

// DetectAgent returns the best available agent/developer name.
// Checks in order: MEMENTO_AGENT_NAME env, MEMENTO_USER env, git config user.name, "unknown".
// The git config result is cached after first call.
func DetectAgent() string {
	once.Do(func() {
		cachedName = detectAgentUncached()
	})
	return cachedName
}

// detectAgentUncached performs detection without caching. Used for testing.
func detectAgentUncached() string {
	if name := os.Getenv("MEMENTO_AGENT_NAME"); name != "" {
		return name
	}
	if name := os.Getenv("MEMENTO_USER"); name != "" {
		return name
	}
	if name := gitUserName(); name != "" {
		return name
	}
	return "unknown"
}

// gitUserName runs `git config --get user.name` and returns the trimmed result.
// Returns empty string on any error.
func gitUserName() string {
	out, err := exec.Command("git", "config", "--get", "user.name").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
