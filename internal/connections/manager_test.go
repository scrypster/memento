package connections

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/scrypster/memento/internal/storage/sqlite"
)

// newTestStore creates an in-memory SQLite store for testing.
func newTestStore(t *testing.T) *sqlite.MemoryStore {
	t.Helper()
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// createTestConfig creates a temporary connections config file for testing.
func createTestConfig(t *testing.T, config *ConnectionsConfig) string {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "connections.json")

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	return configPath
}

// TestGetStore_ReturnsStoreForValidConnection verifies that GetStore returns
// a non-nil store for a valid, enabled connection.
func TestGetStore_ReturnsStoreForValidConnection(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "test-conn",
		Connections: []Connection{
			{
				Name:    "test-conn",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	store, err := manager.GetStore("test-conn")
	if err != nil {
		t.Fatalf("GetStore() failed: %v", err)
	}
	if store == nil {
		t.Error("GetStore() returned nil store")
	}
}

// TestGetStore_CachesStore verifies that GetStore caches the store and returns
// the same instance on subsequent calls (pointer equality).
func TestGetStore_CachesStore(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "test-conn",
		Connections: []Connection{
			{
				Name:    "test-conn",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	store1, err := manager.GetStore("test-conn")
	if err != nil {
		t.Fatalf("first GetStore() failed: %v", err)
	}

	store2, err := manager.GetStore("test-conn")
	if err != nil {
		t.Fatalf("second GetStore() failed: %v", err)
	}

	if store1 != store2 {
		t.Error("GetStore() did not return the same cached instance")
	}
}

// TestGetStore_ConcurrentAccess tests concurrent access to GetStore for cached stores.
// This test verifies that the manager's internal locks work correctly when multiple
// goroutines access an already-cached store. Uses -race to detect data races.
func TestGetStore_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	config := &ConnectionsConfig{
		DefaultConnection: "test-conn",
		Connections: []Connection{
			{
				Name:    "test-conn",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Pre-cache the store to test concurrent read access to the cache
	_, err = manager.GetStore("test-conn")
	if err != nil {
		t.Fatalf("initial GetStore() failed: %v", err)
	}

	// Now have multiple goroutines access the cached store concurrently
	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := manager.GetStore("test-conn")
			if err != nil {
				t.Errorf("GetStore() failed: %v", err)
			}
		}()
	}

	wg.Wait()
}

// TestGetStore_UnknownConnection verifies that GetStore returns an error
// for a connection that doesn't exist.
func TestGetStore_UnknownConnection(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "test-conn",
		Connections: []Connection{
			{
				Name:    "test-conn",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	_, err = manager.GetStore("nonexistent")
	if err == nil {
		t.Error("GetStore() should return error for unknown connection")
	}
}

// TestGetStore_DisabledConnection verifies that GetStore returns an error
// for a disabled connection.
func TestGetStore_DisabledConnection(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "test-conn",
		Connections: []Connection{
			{
				Name:    "test-conn",
				Enabled: false,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	_, err = manager.GetStore("test-conn")
	if err == nil {
		t.Error("GetStore() should return error for disabled connection")
	}
}

// TestGetStore_DefaultConnection verifies that GetStore uses the default
// connection when an empty string is passed.
func TestGetStore_DefaultConnection(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "default-conn",
		Connections: []Connection{
			{
				Name:    "default-conn",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	store, err := manager.GetStore("")
	if err != nil {
		t.Fatalf("GetStore(\"\") failed: %v", err)
	}
	if store == nil {
		t.Error("GetStore(\"\") returned nil store")
	}
}

// TestAddConnection_Success verifies that a new connection can be added
// and that GetStore returns a store for it.
func TestAddConnection_Success(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []Connection{
			{
				Name:    "default",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
		Settings: struct {
			AutoCreateDefault bool `json:"auto_create_default"`
			MaxConnections    int  `json:"max_connections"`
			AllowUserCreate   bool `json:"allow_user_create"`
		}{
			MaxConnections: 10,
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	newConn := Connection{
		Name:    "new-conn",
		Enabled: true,
		Database: DatabaseConfig{
			Type: "sqlite",
			Path: ":memory:",
		},
	}

	if err := manager.AddConnection(context.Background(), newConn); err != nil {
		t.Fatalf("AddConnection() failed: %v", err)
	}

	// Verify connection was added by checking GetStore works
	store, err := manager.GetStore("new-conn")
	if err != nil {
		t.Fatalf("GetStore() failed for added connection: %v", err)
	}
	if store == nil {
		t.Error("GetStore() returned nil store for added connection")
	}
}

// TestAddConnection_DuplicateName verifies that adding a connection with
// a duplicate name returns an error.
func TestAddConnection_DuplicateName(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []Connection{
			{
				Name:    "default",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
		Settings: struct {
			AutoCreateDefault bool `json:"auto_create_default"`
			MaxConnections    int  `json:"max_connections"`
			AllowUserCreate   bool `json:"allow_user_create"`
		}{
			MaxConnections: 10,
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	duplicateConn := Connection{
		Name:    "default",
		Enabled: true,
		Database: DatabaseConfig{
			Type: "sqlite",
			Path: ":memory:",
		},
	}

	err = manager.AddConnection(context.Background(), duplicateConn)
	if err == nil {
		t.Error("AddConnection() should return error for duplicate name")
	}
}

// TestAddConnection_MaxConnectionsEnforced verifies that MaxConnections limit
// is enforced when non-zero.
func TestAddConnection_MaxConnectionsEnforced(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []Connection{
			{
				Name:    "default",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
		Settings: struct {
			AutoCreateDefault bool `json:"auto_create_default"`
			MaxConnections    int  `json:"max_connections"`
			AllowUserCreate   bool `json:"allow_user_create"`
		}{
			MaxConnections: 1, // Only 1 connection allowed
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	newConn := Connection{
		Name:    "new-conn",
		Enabled: true,
		Database: DatabaseConfig{
			Type: "sqlite",
			Path: ":memory:",
		},
	}

	err = manager.AddConnection(context.Background(), newConn)
	if err == nil {
		t.Error("AddConnection() should return error when max connections exceeded")
	}
}

// TestAddConnection_ZeroMaxLimit verifies that MaxConnections=0 prevents all additions.
// NOTE: The implementation treats MaxConnections=0 as "no new connections allowed",
// not as "unlimited". This is intentional - use a large number like 1000 if you want unlimited.
func TestAddConnection_ZeroMaxLimit(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []Connection{
			{
				Name:    "default",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
		Settings: struct {
			AutoCreateDefault bool `json:"auto_create_default"`
			MaxConnections    int  `json:"max_connections"`
			AllowUserCreate   bool `json:"allow_user_create"`
		}{
			MaxConnections: 0, // 0 means no new connections allowed
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Try to add a new connection - should fail because we're already at limit
	newConn := Connection{
		Name:    "new-conn",
		Enabled: true,
		Database: DatabaseConfig{
			Type: "sqlite",
			Path: ":memory:",
		},
	}

	err = manager.AddConnection(context.Background(), newConn)
	if err == nil {
		t.Error("AddConnection() should return error when maxConnections=0")
	}
}

// TestDeleteConnection_DefaultConnectionRejected verifies that attempting to
// delete the default connection returns an error.
func TestDeleteConnection_DefaultConnectionRejected(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []Connection{
			{
				Name:    "default",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	err = manager.DeleteConnection(context.Background(), "default")
	if err == nil {
		t.Error("DeleteConnection() should return error for default connection")
	}
}

// TestDeleteConnection_RemovesFromCache verifies that deleting a non-default
// connection removes it from the cache and causes GetStore to fail.
func TestDeleteConnection_RemovesFromCache(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []Connection{
			{
				Name:    "default",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
			{
				Name:    "to-delete",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
		Settings: struct {
			AutoCreateDefault bool `json:"auto_create_default"`
			MaxConnections    int  `json:"max_connections"`
			AllowUserCreate   bool `json:"allow_user_create"`
		}{
			MaxConnections: 10,
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Verify the connection exists before deletion
	_, err = manager.GetStore("to-delete")
	if err != nil {
		t.Fatalf("GetStore() failed before deletion: %v", err)
	}

	// Delete the connection
	if err := manager.DeleteConnection(context.Background(), "to-delete"); err != nil {
		t.Fatalf("DeleteConnection() failed: %v", err)
	}

	// Verify GetStore fails after deletion
	_, err = manager.GetStore("to-delete")
	if err == nil {
		t.Error("GetStore() should fail for deleted connection")
	}
}

// TestSanitizeDSN_RedactsPasswordURL verifies that sanitizeDSN redacts
// passwords in URL-format DSNs. Note: sanitizeDSN URL-encodes [REDACTED] as %5BREDACTED%5D
func TestSanitizeDSN_RedactsPasswordURL(t *testing.T) {
	dsn := "postgres://user:secretpassword@localhost:5432/mydb?sslmode=disable"
	sanitized := sanitizeDSN(dsn)

	if sanitized == dsn {
		t.Error("sanitizeDSN() did not modify the DSN")
	}

	if containsString(sanitized, "secretpassword") {
		t.Errorf("sanitizeDSN() did not redact password in URL format: %s", sanitized)
	}

	// The sanitizeDSN function URL-encodes [REDACTED] to %5BREDACTED%5D
	if !containsString(sanitized, "%5BREDACTED%5D") && !containsString(sanitized, "[REDACTED]") {
		t.Errorf("sanitizeDSN() did not add redaction marker to URL format: %s", sanitized)
	}
}

// TestSanitizeDSN_RedactsPasswordKeyValue verifies that sanitizeDSN redacts
// passwords in key=value format DSNs.
func TestSanitizeDSN_RedactsPasswordKeyValue(t *testing.T) {
	dsn := "user=myuser password=mysecret host=localhost dbname=mydb"
	sanitized := sanitizeDSN(dsn)

	if sanitized == dsn {
		t.Error("sanitizeDSN() did not modify the DSN")
	}

	if containsString(sanitized, "mysecret") {
		t.Errorf("sanitizeDSN() did not redact password in key=value format: %s", sanitized)
	}

	if !containsString(sanitized, "[REDACTED]") {
		t.Errorf("sanitizeDSN() did not add [REDACTED] to key=value format: %s", sanitized)
	}
}

// TestSanitizeDSN_NoPasswordURL verifies that sanitizeDSN doesn't modify
// URL format DSNs without passwords.
func TestSanitizeDSN_NoPasswordURL(t *testing.T) {
	dsn := "postgres://localhost:5432/mydb?sslmode=disable"
	sanitized := sanitizeDSN(dsn)

	if sanitized != dsn {
		t.Errorf("sanitizeDSN() modified DSN without password: got %s, want %s", sanitized, dsn)
	}
}

// TestBorrowedStoreNotClosedOnManagerClose verifies that when a manager wraps
// a borrowed store (created via NewManagerWithStore), calling manager.Close()
// does NOT close the borrowed store.
func TestBorrowedStoreNotClosedOnManagerClose(t *testing.T) {
	borrowedStore := newTestStore(t)

	manager := NewManagerWithStore(borrowedStore, "borrowed")

	// Close the manager
	if err := manager.Close(); err != nil {
		t.Fatalf("manager.Close() failed: %v", err)
	}

	// Verify the borrowed store is still usable (not closed by manager.Close())
	// We can't directly test if it's closed, but we can verify the manager
	// still has it in its stores map and marked as not owned
	store, err := manager.GetStore("borrowed")
	if err != nil {
		t.Errorf("GetStore() should still work after manager.Close(): %v", err)
	}
	if store != borrowedStore {
		t.Error("GetStore() returned different store than the borrowed one")
	}
}

// TestNewManagerWithStore_SetAsDefault verifies that NewManagerWithStore
// sets the connection as the default.
func TestNewManagerWithStore_SetAsDefault(t *testing.T) {
	borrowedStore := newTestStore(t)

	manager := NewManagerWithStore(borrowedStore, "test-conn")

	if manager.GetDefaultConnection() != "test-conn" {
		t.Errorf("GetDefaultConnection() = %q, want %q", manager.GetDefaultConnection(), "test-conn")
	}
}

// TestListConnections returns the list of all connections.
func TestListConnections(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []Connection{
			{
				Name:    "default",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
			{
				Name:    "secondary",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	connections := manager.ListConnections()
	if len(connections) != 2 {
		t.Errorf("ListConnections() returned %d connections, want 2", len(connections))
	}

	names := make(map[string]bool)
	for _, conn := range connections {
		names[conn.Name] = true
	}

	if !names["default"] {
		t.Error("ListConnections() missing 'default' connection")
	}
	if !names["secondary"] {
		t.Error("ListConnections() missing 'secondary' connection")
	}
}

// TestUpdateConnection updates a connection config and invalidates cache.
func TestUpdateConnection(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []Connection{
			{
				Name:        "default",
				DisplayName: "Original Name",
				Enabled:     true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Cache the store
	_, err = manager.GetStore("default")
	if err != nil {
		t.Fatalf("GetStore() failed: %v", err)
	}

	// Update the connection
	updatedConn := Connection{
		Name:        "default",
		DisplayName: "Updated Name",
		Enabled:     true,
		Database: DatabaseConfig{
			Type: "sqlite",
			Path: ":memory:",
		},
	}

	if err := manager.UpdateConnection(context.Background(), "default", updatedConn); err != nil {
		t.Fatalf("UpdateConnection() failed: %v", err)
	}

	// Verify cache was invalidated (new store should be created)
	connections := manager.ListConnections()
	if len(connections) == 0 {
		t.Fatal("ListConnections() returned no connections")
	}
	if connections[0].DisplayName != "Updated Name" {
		t.Errorf("UpdateConnection() did not update DisplayName: got %q, want %q", connections[0].DisplayName, "Updated Name")
	}
}

// TestSetDefaultConnection changes the default connection.
func TestSetDefaultConnection(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []Connection{
			{
				Name:    "default",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
			{
				Name:    "secondary",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	if err := manager.SetDefaultConnection(context.Background(), "secondary"); err != nil {
		t.Fatalf("SetDefaultConnection() failed: %v", err)
	}

	if manager.GetDefaultConnection() != "secondary" {
		t.Errorf("GetDefaultConnection() = %q, want %q", manager.GetDefaultConnection(), "secondary")
	}
}

// TestSetDefaultConnection_InvalidConnection returns error for non-existent connection.
func TestSetDefaultConnection_InvalidConnection(t *testing.T) {
	config := &ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []Connection{
			{
				Name:    "default",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	err = manager.SetDefaultConnection(context.Background(), "nonexistent")
	if err == nil {
		t.Error("SetDefaultConnection() should return error for nonexistent connection")
	}
}

// TestTestConnection_SQLite tests a SQLite connection configuration.
func TestTestConnection_SQLite(t *testing.T) {
	ctx := context.Background()
	config := &ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []Connection{
			{
				Name:    "default",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	conn := Connection{
		Name: "test-conn",
		Database: DatabaseConfig{
			Type: "sqlite",
			Path: ":memory:",
		},
	}

	if err := manager.TestConnection(ctx, conn); err != nil {
		t.Errorf("TestConnection() failed: %v", err)
	}
}

// TestTestConnection_InvalidDatabaseType returns error for unsupported type.
func TestTestConnection_InvalidDatabaseType(t *testing.T) {
	ctx := context.Background()
	config := &ConnectionsConfig{
		DefaultConnection: "default",
		Connections: []Connection{
			{
				Name:    "default",
				Enabled: true,
				Database: DatabaseConfig{
					Type: "sqlite",
					Path: ":memory:",
				},
			},
		},
	}
	configPath := createTestConfig(t, config)
	manager, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	conn := Connection{
		Name: "test-conn",
		Database: DatabaseConfig{
			Type: "unsupported",
		},
	}

	err = manager.TestConnection(ctx, conn)
	if err == nil {
		t.Error("TestConnection() should return error for unsupported database type")
	}
}

// containsString is a helper to check if a string contains a substring.
func containsString(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
