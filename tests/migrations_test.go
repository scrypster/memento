package tests

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// TestMigrationUp verifies that the up migration creates the schema correctly.
func TestMigrationUp(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test_migration_up.db")
	dsn := "file:" + dbPath

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Enable WAL and foreign keys (like MemoryStore does)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("Failed to enable WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	// Get migrations directory
	migrationsDir := getMigrationsDir(t)

	// Create migration manager
	mgr, err := storage.NewMigrationManager(db, migrationsDir)
	if err != nil {
		t.Fatalf("Failed to create migration manager: %v", err)
	}
	defer mgr.Close()

	// Run migrations
	if err := mgr.Up(); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Verify all tables exist
	tables := []string{
		"memories",
		"entities",
		"relationships",
		"memory_entities",
		"embeddings",
		"memories_fts",
		"settings",
		"unknown_type_stats",
		"memory_links",
	}

	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("Table %s does not exist", table)
		} else if err != nil {
			t.Fatalf("Failed to check table %s: %v", table, err)
		}
	}

	// Verify indexes exist (sample check)
	indexes := []string{
		"idx_memories_status",
		"idx_memories_entity_status",
		"idx_entities_type",
		"idx_relationships_source",
	}

	for _, index := range indexes {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name=?", index).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("Index %s does not exist", index)
		} else if err != nil {
			t.Fatalf("Failed to check index %s: %v", index, err)
		}
	}

	// Verify triggers exist (sample check)
	triggers := []string{
		"memories_updated_at",
		"entities_updated_at",
		"memories_fts_insert",
	}

	for _, trigger := range triggers {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='trigger' AND name=?", trigger).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("Trigger %s does not exist", trigger)
		} else if err != nil {
			t.Fatalf("Failed to check trigger %s: %v", trigger, err)
		}
	}
}

// TestMigrationDown verifies that the down migration rolls back correctly.
func TestMigrationDown(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test_migration_down.db")
	dsn := "file:" + dbPath

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("Failed to enable WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	migrationsDir := getMigrationsDir(t)
	mgr, err := storage.NewMigrationManager(db, migrationsDir)
	if err != nil {
		t.Fatalf("Failed to create migration manager: %v", err)
	}
	defer mgr.Close()

	// Apply schema
	if err := mgr.Up(); err != nil {
		t.Fatalf("Failed to run up migration: %v", err)
	}

	// Roll back
	if err := mgr.Down(); err != nil {
		t.Fatalf("Failed to run down migration: %v", err)
	}

	// Verify core tables are gone
	for _, table := range []string{"memories", "entities", "settings", "memory_links"} {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err == nil {
			t.Errorf("Table %s should have been dropped", table)
		}
	}
}

// TestMigrationVersion verifies that migration version tracking works.
func TestMigrationVersion(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test_migration_version.db")
	dsn := "file:" + dbPath

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Enable WAL and foreign keys
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("Failed to enable WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	// Get migrations directory
	migrationsDir := getMigrationsDir(t)

	// Create migration manager
	mgr, err := storage.NewMigrationManager(db, migrationsDir)
	if err != nil {
		t.Fatalf("Failed to create migration manager: %v", err)
	}
	defer mgr.Close()

	// Check version before migration (should be 0 or no version)
	version, dirty, err := mgr.Version()
	if err != nil && err.Error() != "no migration" {
		t.Fatalf("Unexpected error getting version: %v", err)
	}

	// Run up migration
	if err := mgr.Up(); err != nil {
		t.Fatalf("Failed to run up migration: %v", err)
	}

	// Check version after migration
	version, dirty, err = mgr.Version()
	if err != nil {
		t.Fatalf("Failed to get version after migration: %v", err)
	}

	if version != 1 {
		t.Errorf("Expected version 1, got %d", version)
	}

	if dirty {
		t.Error("Migration is marked as dirty")
	}
}

// TestMigrationIdempotent verifies that running migrations twice is safe.
func TestMigrationIdempotent(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test_migration_idempotent.db")
	dsn := "file:" + dbPath

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Enable WAL and foreign keys
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("Failed to enable WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	// Get migrations directory
	migrationsDir := getMigrationsDir(t)

	// Create migration manager
	mgr, err := storage.NewMigrationManager(db, migrationsDir)
	if err != nil {
		t.Fatalf("Failed to create migration manager: %v", err)
	}
	defer mgr.Close()

	// Run up migration first time
	if err := mgr.Up(); err != nil {
		t.Fatalf("Failed to run up migration first time: %v", err)
	}

	// Run up migration second time (should be no-op)
	err = mgr.Up()
	if err != nil && err.Error() != "no change" {
		t.Fatalf("Expected 'no change' error or nil, got: %v", err)
	}

	// Verify tables still exist and work
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='memories'").Scan(&name)
	if err != nil {
		t.Fatalf("Failed to verify memories table after second migration: %v", err)
	}
}

// TestMigrationOrder verifies migrations run in the correct order.
func TestMigrationOrder(t *testing.T) {
	// Create temp database
	dbPath := filepath.Join(t.TempDir(), "test_migration_order.db")
	dsn := "file:" + dbPath

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Enable WAL and foreign keys
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("Failed to enable WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	// Get migrations directory
	migrationsDir := getMigrationsDir(t)

	// Create migration manager
	mgr, err := storage.NewMigrationManager(db, migrationsDir)
	if err != nil {
		t.Fatalf("Failed to create migration manager: %v", err)
	}
	defer mgr.Close()

	// Run up migration
	if err := mgr.Up(); err != nil {
		t.Fatalf("Failed to run migration: %v", err)
	}

	// Verify version is 1 (single consolidated migration)
	version, dirty, err := mgr.Version()
	if err != nil {
		t.Fatalf("Failed to get version: %v", err)
	}

	if version != 1 {
		t.Errorf("Expected version 1 after all migrations, got %d", version)
	}

	if dirty {
		t.Error("Migration is marked as dirty")
	}
}

// TestMemoryStoreWithMigrations verifies that MemoryStore works after being
// initialised with the full embedded Schema (which already includes all columns
// from migrations 000001 and 000002). The RunMigrations path is covered by the
// dedicated TestMigration* tests that operate on a raw database file.
func TestMemoryStoreWithMigrations(t *testing.T) {
	// Use an in-memory store — NewMemoryStore initialises the full Schema which
	// includes all current columns, so no additional migrations are needed.
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	// Verify we can use the store
	ctx := context.Background()

	// Create a test memory
	memory := createSimpleTestMemory("test-1", "Test content")
	if err := store.Store(ctx, memory); err != nil {
		t.Fatalf("Failed to store memory: %v", err)
	}

	// Retrieve it
	retrieved, err := store.Get(ctx, "test-1")
	if err != nil {
		t.Fatalf("Failed to get memory: %v", err)
	}

	if retrieved.Content != "Test content" {
		t.Errorf("Expected content 'Test content', got '%s'", retrieved.Content)
	}
}

// getMigrationsDir returns the path to the migrations directory.
func getMigrationsDir(t *testing.T) string {
	// Get the project root (go up from tests/ to project root)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// migrations/ is at project root
	projectRoot := filepath.Dir(wd)
	migrationsDir := filepath.Join(projectRoot, "migrations")

	// Verify migrations directory exists
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		t.Fatalf("Migrations directory does not exist: %s", migrationsDir)
	}

	return migrationsDir
}

// createSimpleTestMemory creates a simple test memory for migration testing.
func createSimpleTestMemory(id, content string) *types.Memory {
	return &types.Memory{
		ID:      id,
		Content: content,
		Source:  "test",
	}
}
