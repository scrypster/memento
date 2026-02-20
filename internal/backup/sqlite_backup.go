package backup

import (
	"database/sql"
	"fmt"
	"io"
	"os"

	_ "modernc.org/sqlite"
)

// backupSQLite creates a consistent backup of a SQLite database.
// It uses SQLite's VACUUM INTO command which handles WAL mode correctly
// and creates a consistent point-in-time backup.
func backupSQLite(sourcePath, destPath string) error {
	// Open source database in read-only mode
	sourceDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", sourcePath))
	if err != nil {
		return fmt.Errorf("failed to open source database: %w", err)
	}
	defer func() { _ = sourceDB.Close() }()

	// Verify source database is accessible
	if err := sourceDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping source database: %w", err)
	}

	// Use VACUUM INTO to create backup
	// This creates a consistent backup even with WAL mode
	_, err = sourceDB.Exec(fmt.Sprintf("VACUUM INTO '%s'", destPath))
	if err != nil {
		return fmt.Errorf("failed to backup database: %w", err)
	}

	return nil
}

// verifyBackup checks the integrity of a SQLite backup.
// It opens the backup database and runs SQLite's integrity_check pragma.
func verifyBackup(backupPath string) error {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", backupPath))
	if err != nil {
		return fmt.Errorf("failed to open backup: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Run integrity check
	var result string
	err = db.QueryRow("PRAGMA integrity_check").Scan(&result)
	if err != nil {
		return fmt.Errorf("failed to run integrity check: %w", err)
	}

	if result != "ok" {
		return fmt.Errorf("integrity check failed: %s", result)
	}

	return nil
}

// restoreSQLite restores a database from a backup.
// It copies the backup file to the target location.
// The target database should not be in use when calling this function.
func restoreSQLite(backupPath, targetPath string) error {
	// Verify backup before restoring
	if err := verifyBackup(backupPath); err != nil {
		return fmt.Errorf("backup verification failed: %w", err)
	}

	// Open source backup file
	src, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup: %w", err)
	}
	defer func() { _ = src.Close() }()

	// Create destination file
	dst, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer func() { _ = dst.Close() }()

	// Copy backup to target
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy backup: %w", err)
	}

	// Ensure data is written to disk
	if err := dst.Sync(); err != nil {
		return fmt.Errorf("failed to sync target file: %w", err)
	}

	// Verify restored database
	if err := verifyBackup(targetPath); err != nil {
		return fmt.Errorf("restored database verification failed: %w", err)
	}

	return nil
}
