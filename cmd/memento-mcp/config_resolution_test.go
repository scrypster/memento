// config_resolution_test.go exercises the connections config resolution logic.
//
// Tests verify that resolveConnectionsConfig() respects the priority order:
//  1. MEMENTO_CONNECTIONS_CONFIG env var (must exist)
//  2. config/connections.json relative to the executable directory
//  3. config/connections.json relative to the current working directory
//  4. Empty string if nothing found
//
// Tests use temporary directories and files to simulate various layouts
// without polluting the actual filesystem.
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveConnectionsConfig_EnvVarSet verifies that when
// MEMENTO_CONNECTIONS_CONFIG is set to an existing file, that path is returned.
func TestResolveConnectionsConfig_EnvVarSet(t *testing.T) {
	// Create a temporary file to represent the connections config.
	tmpFile, err := os.CreateTemp("", "connections*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	// Set the env var to point to our temp file.
	t.Setenv("MEMENTO_CONNECTIONS_CONFIG", tmpFile.Name())

	// Call resolveConnectionsConfig and verify it returns the env var value.
	result := resolveConnectionsConfig()
	if result != tmpFile.Name() {
		t.Errorf("resolveConnectionsConfig() = %q, want %q", result, tmpFile.Name())
	}
}

// TestResolveConnectionsConfig_EnvVarPointsToMissingFile verifies that if
// MEMENTO_CONNECTIONS_CONFIG is set but the file doesn't exist, resolution
// continues to the next tier (does not return the missing path).
func TestResolveConnectionsConfig_EnvVarPointsToMissingFile(t *testing.T) {
	// Create a CWD fallback file so resolution can succeed at tier 3.
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "memento-cwd-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = os.Chdir(originalDir) }()

	// Change to the temp directory.
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Create config/connections.json in the CWD.
	if err := os.Mkdir("config", 0o755); err != nil {
		t.Fatalf("failed to mkdir config: %v", err)
	}
	if err := os.WriteFile("config/connections.json", []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write config/connections.json: %v", err)
	}

	// Set env var to a path that doesn't exist.
	nonExistentPath := filepath.Join(tmpDir, "does_not_exist.json")
	t.Setenv("MEMENTO_CONNECTIONS_CONFIG", nonExistentPath)

	// Resolution should skip the missing env var path and fall back to CWD.
	result := resolveConnectionsConfig()
	wantPath := "config/connections.json" // CWD fallback returns relative path
	if result != wantPath {
		t.Errorf("resolveConnectionsConfig() = %q, want %q (CWD fallback)", result, wantPath)
	}
}

// TestResolveConnectionsConfig_CWDFallback verifies that when no env var is
// set and no executable-adjacent config exists, config/connections.json
// relative to CWD is used (tier 3).
func TestResolveConnectionsConfig_CWDFallback(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "memento-cwd-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = os.Chdir(originalDir) }()

	// Change to the temp directory.
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Create config/connections.json in the CWD.
	if err := os.Mkdir("config", 0o755); err != nil {
		t.Fatalf("failed to mkdir config: %v", err)
	}
	if err := os.WriteFile("config/connections.json", []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write config/connections.json: %v", err)
	}

	// Clear the env var.
	t.Setenv("MEMENTO_CONNECTIONS_CONFIG", "")

	// Resolution should find the CWD config.
	result := resolveConnectionsConfig()
	expectedPath := "config/connections.json"
	if result != expectedPath {
		t.Errorf("resolveConnectionsConfig() = %q, want %q (CWD fallback)", result, expectedPath)
	}
}

// TestResolveConnectionsConfig_NothingFound verifies that when no config is
// found at any tier, an empty string is returned.
func TestResolveConnectionsConfig_NothingFound(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "memento-empty-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = os.Chdir(originalDir) }()

	// Change to the temp directory (which has no config/ subdirectory).
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Clear the env var.
	t.Setenv("MEMENTO_CONNECTIONS_CONFIG", "")

	// Resolution should return empty string when nothing is found.
	result := resolveConnectionsConfig()
	if result != "" {
		t.Errorf("resolveConnectionsConfig() = %q, want empty string when nothing found", result)
	}
}

// TestResolveConnectionsConfig_EnvVarEmpty verifies that an empty env var is
// treated as "not set" and resolution continues to the next tier.
func TestResolveConnectionsConfig_EnvVarEmpty(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "memento-empty-var-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = os.Chdir(originalDir) }()

	// Change to the temp directory.
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Create config/connections.json in CWD.
	if err := os.Mkdir("config", 0o755); err != nil {
		t.Fatalf("failed to mkdir config: %v", err)
	}
	if err := os.WriteFile("config/connections.json", []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write config/connections.json: %v", err)
	}

	// Set env var to empty string (effectively unset).
	t.Setenv("MEMENTO_CONNECTIONS_CONFIG", "")

	// Resolution should skip the empty env var and find the CWD config.
	result := resolveConnectionsConfig()
	expectedPath := "config/connections.json"
	if result != expectedPath {
		t.Errorf("resolveConnectionsConfig() = %q, want %q (empty env var treated as unset)", result, expectedPath)
	}
}

// TestResolveConnectionsConfig_EnvVarTakesPriority verifies that when both the
// env var and CWD config exist, the env var is returned (tier 1 > tier 3).
func TestResolveConnectionsConfig_EnvVarTakesPriority(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "memento-priority-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = os.Chdir(originalDir) }()

	// Change to the temp directory.
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Create config/connections.json in CWD (tier 3 candidate).
	if err := os.Mkdir("config", 0o755); err != nil {
		t.Fatalf("failed to mkdir config: %v", err)
	}
	if err := os.WriteFile("config/connections.json", []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write config/connections.json: %v", err)
	}

	// Create an env var config file outside the CWD (tier 1 candidate).
	envConfigFile, err := os.CreateTemp("", "env-config*.json")
	if err != nil {
		t.Fatalf("failed to create temp file for env var: %v", err)
	}
	defer func() { _ = os.Remove(envConfigFile.Name()) }()
	_ = envConfigFile.Close()

	t.Setenv("MEMENTO_CONNECTIONS_CONFIG", envConfigFile.Name())

	// Resolution should return the env var path (tier 1), not the CWD path (tier 3).
	result := resolveConnectionsConfig()
	if result != envConfigFile.Name() {
		t.Errorf("resolveConnectionsConfig() = %q, want %q (env var takes priority)", result, envConfigFile.Name())
	}
}
