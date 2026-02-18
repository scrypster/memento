// Package storage provides composable storage interfaces for the Memento system.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ErrNoMigration indicates no migration has been applied yet.
var ErrNoMigration = errors.New("no migration")

// MigrationManager manages database schema migrations using plain SQL files.
// It reads NNN_name.up.sql / NNN_name.down.sql files from a directory and
// applies them in order, tracking the current version in a schema_migrations
// table. This implementation is CGO-free and works with modernc.org/sqlite.
type MigrationManager struct {
	db            *sql.DB
	migrationsDir string
}

// migration represents a single up/down migration pair.
type migration struct {
	version uint
	name    string
	upFile  string
	downFile string
}

// NewMigrationManager creates a new MigrationManager for the given database
// and migrations directory.
//
// migrationsDir must be an absolute path or a path relative to the process
// working directory. The directory must contain numbered migration files in
// the format NNN_name.up.sql / NNN_name.down.sql.
func NewMigrationManager(db *sql.DB, migrationsDir string) (*MigrationManager, error) {
	if db == nil {
		return nil, fmt.Errorf("migrations: database connection is required")
	}

	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("migrations: directory does not exist: %s", migrationsDir)
	}

	mgr := &MigrationManager{
		db:            db,
		migrationsDir: migrationsDir,
	}

	// Ensure migrations tracking table exists
	if err := mgr.ensureSchemaTable(); err != nil {
		return nil, fmt.Errorf("migrations: failed to create schema table: %w", err)
	}

	return mgr, nil
}

// ensureSchemaTable creates the schema_migrations table if it doesn't exist.
func (mgr *MigrationManager) ensureSchemaTable() error {
	_, err := mgr.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// Up applies all pending migrations in ascending version order.
// Returns nil if already up-to-date.
func (mgr *MigrationManager) Up() error {
	migrations, err := mgr.loadMigrations()
	if err != nil {
		return fmt.Errorf("migrations: failed to load migration files: %w", err)
	}

	currentVersion, _, err := mgr.Version()
	if err != nil && !errors.Is(err, ErrNoMigration) {
		return fmt.Errorf("migrations: failed to get current version: %w", err)
	}

	// Apply migrations with version > currentVersion
	applied := 0
	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		sql, err := os.ReadFile(m.upFile)
		if err != nil {
			return fmt.Errorf("migrations: failed to read %s: %w", m.upFile, err)
		}

		if _, err := mgr.db.Exec(string(sql)); err != nil {
			return fmt.Errorf("migrations: failed to apply version %d (%s): %w", m.version, m.name, err)
		}

		if _, err := mgr.db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			return fmt.Errorf("migrations: failed to record version %d: %w", m.version, err)
		}

		applied++
	}

	if applied == 0 {
		// Already up to date - this is not an error
		return nil
	}

	return nil
}

// Down rolls back all applied migrations in descending version order.
func (mgr *MigrationManager) Down() error {
	migrations, err := mgr.loadMigrations()
	if err != nil {
		return fmt.Errorf("migrations: failed to load migration files: %w", err)
	}

	currentVersion, _, err := mgr.Version()
	if errors.Is(err, ErrNoMigration) {
		return nil // Nothing to roll back
	}
	if err != nil {
		return fmt.Errorf("migrations: failed to get current version: %w", err)
	}

	// Roll back in reverse order
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version > migrations[j].version
	})

	for _, m := range migrations {
		if m.version > currentVersion {
			continue
		}

		sql, err := os.ReadFile(m.downFile)
		if err != nil {
			return fmt.Errorf("migrations: failed to read %s: %w", m.downFile, err)
		}

		if _, err := mgr.db.Exec(string(sql)); err != nil {
			return fmt.Errorf("migrations: failed to roll back version %d (%s): %w", m.version, m.name, err)
		}

		if _, err := mgr.db.Exec("DELETE FROM schema_migrations WHERE version = ?", m.version); err != nil {
			return fmt.Errorf("migrations: failed to remove version %d: %w", m.version, err)
		}
	}

	return nil
}

// Version returns the highest applied migration version.
// Returns (0, false, ErrNoMigration) when no migration has been applied.
func (mgr *MigrationManager) Version() (uint, bool, error) {
	var version uint
	err := mgr.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, false, fmt.Errorf("migrations: failed to query version: %w", err)
	}

	if version == 0 {
		return 0, false, ErrNoMigration
	}

	return version, false, nil
}

// Close is a no-op for this implementation (the db is managed externally).
func (mgr *MigrationManager) Close() error {
	return nil
}

// loadMigrations reads and parses migration files from the directory.
// Files must be named NNN_name.up.sql (where NNN is a zero-padded integer).
// Returns migrations sorted by version ascending.
func (mgr *MigrationManager) loadMigrations() ([]migration, error) {
	entries, err := os.ReadDir(mgr.migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("migrations: failed to read directory: %w", err)
	}

	// Collect up migration files and pair with down files
	migrationMap := make(map[uint]*migration)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		// Parse: NNN_name.up.sql or NNN_name.down.sql
		var versionStr, rest string
		underscoreIdx := strings.Index(name, "_")
		if underscoreIdx < 0 {
			continue
		}
		versionStr = name[:underscoreIdx]
		rest = name[underscoreIdx+1:]

		versionInt, err := strconv.ParseUint(versionStr, 10, 64)
		if err != nil {
			continue // Skip non-numeric prefix files
		}
		version := uint(versionInt)

		fullPath := filepath.Join(mgr.migrationsDir, name)

		m, ok := migrationMap[version]
		if !ok {
			m = &migration{version: version}
			migrationMap[version] = m
		}

		// Determine migration name from the base (strip .up.sql / .down.sql)
		if strings.HasSuffix(rest, ".up.sql") {
			m.name = strings.TrimSuffix(rest, ".up.sql")
			m.upFile = fullPath
		} else if strings.HasSuffix(rest, ".down.sql") {
			m.downFile = fullPath
		}
	}

	// Convert map to sorted slice
	migrations := make([]migration, 0, len(migrationMap))
	for _, m := range migrationMap {
		if m.upFile == "" {
			continue // Skip entries without an up file
		}
		migrations = append(migrations, *m)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}
