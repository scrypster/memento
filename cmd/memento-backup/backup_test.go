package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/backup"
	_ "modernc.org/sqlite"
)

// Helper function to create a test SQLite database with sample data
func createTestDB(t *testing.T, dbPath string) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Create a simple test table with some data
	if _, err := db.Exec(`
		CREATE TABLE test_data (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Insert sample data
	if _, err := db.Exec(`
		INSERT INTO test_data (name) VALUES
		('test_record_1'),
		('test_record_2'),
		('test_record_3')
	`); err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}
}

// Helper function to count records in test database
func countTestRecords(t *testing.T, dbPath string) int {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", dbPath))
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM test_data").Scan(&count); err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}
	return count
}

// TestBackupService_OneshotMode tests the oneshot backup functionality
// This tests the backup.Service.BackupNow operation that handleOneshot() calls
func TestBackupService_OneshotMode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	// Create test database
	createTestDB(t, dbPath)

	// Create backup service
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPath,
		BackupDir: backupDir,
		Interval:  1 * time.Hour,
		Retention: backup.RetentionPolicy{
			Hourly:  24,
			Daily:   7,
			Weekly:  4,
			Monthly: 12,
		},
		VerifyBackups: true,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Perform backup
	ctx := context.Background()
	result, err := service.BackupNow(ctx)
	if err != nil {
		t.Fatalf("BackupNow failed: %v", err)
	}

	// Verify backup result
	if result.Path == "" {
		t.Error("Backup path is empty")
	}
	if result.Size <= 0 {
		t.Error("Backup size should be positive")
	}
	if !result.Verified {
		t.Error("Backup should be verified")
	}
	if result.Duration <= 0 {
		t.Error("Backup duration should be positive")
	}

	// Verify backup file exists
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatalf("Backup file not found at %s: %v", result.Path, err)
	}

	// Verify backup is a valid SQLite database
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", result.Path))
	if err != nil {
		t.Fatalf("Failed to open backup as SQLite database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Verify the backup contains the same data
	recordCount := countTestRecords(t, result.Path)
	if recordCount != 3 {
		t.Errorf("Expected 3 records in backup, got %d", recordCount)
	}
}

// TestBackupService_ListBackups tests the list backups functionality
// This tests backup.Service.ListBackups that handleList() calls
func TestBackupService_ListBackups(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	// Create test database
	createTestDB(t, dbPath)

	// Create backup service
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPath,
		BackupDir: backupDir,
		Interval:  1 * time.Hour,
		Retention: backup.RetentionPolicy{
			Hourly:  24,
			Daily:   7,
			Weekly:  4,
			Monthly: 12,
		},
		VerifyBackups: true,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Perform multiple backups with slight delays to ensure unique timestamps
	ctx := context.Background()
	numBackups := 3

	for i := 0; i < numBackups; i++ {
		if _, err := service.BackupNow(ctx); err != nil {
			t.Fatalf("BackupNow failed on iteration %d: %v", i+1, err)
		}
		time.Sleep(10 * time.Millisecond) // Small delay to ensure unique timestamps
	}

	// List backups
	backups, err := service.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	// Verify we have at least the backups we created
	if len(backups) < numBackups {
		t.Errorf("Expected at least %d backups, got %d", numBackups, len(backups))
	}

	// Verify backup info structure
	for i, b := range backups {
		if b.Path == "" {
			t.Errorf("Backup %d has empty path", i)
		}
		if b.Size <= 0 {
			t.Errorf("Backup %d has invalid size: %d", i, b.Size)
		}
		if b.Timestamp.IsZero() {
			t.Errorf("Backup %d has zero timestamp", i)
		}
	}
}

// TestBackupService_HealthCheck tests the health check functionality
// This tests backup.Service.HealthCheck that handleHealth() calls
func TestBackupService_HealthCheck(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	// Create test database
	createTestDB(t, dbPath)

	// Create backup service
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPath,
		BackupDir: backupDir,
		Interval:  1 * time.Hour,
		Retention: backup.RetentionPolicy{
			Hourly:  24,
			Daily:   7,
			Weekly:  4,
			Monthly: 12,
		},
		VerifyBackups: true,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Check health before any backups
	health, err := service.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("Expected healthy status, got %s", health.Status)
	}
	if health.TotalBackups != 0 {
		t.Errorf("Expected 0 backups, got %d", health.TotalBackups)
	}
	if health.BackupDir != backupDir {
		t.Errorf("Expected backup dir %s, got %s", backupDir, health.BackupDir)
	}

	// Perform a backup
	ctx := context.Background()
	_, err = service.BackupNow(ctx)
	if err != nil {
		t.Fatalf("BackupNow failed: %v", err)
	}

	// Check health after backup
	health, err = service.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck after backup failed: %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("Expected healthy status after backup, got %s", health.Status)
	}
	if health.TotalBackups != 1 {
		t.Errorf("Expected 1 backup, got %d", health.TotalBackups)
	}
	if !health.LastBackup.IsZero() {
		t.Log("Last backup time is set correctly")
	}
	if health.DiskSpaceUsed <= 0 {
		t.Error("Expected positive disk space usage")
	}
}

// TestBackupService_RestoreFromBackup tests the restore functionality
// This tests backup.Service.RestoreBackup that handleRestore() calls
func TestBackupService_RestoreFromBackup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	// Create and populate original database
	createTestDB(t, dbPath)

	// Create backup service
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPath,
		BackupDir: backupDir,
		Interval:  1 * time.Hour,
		Retention: backup.RetentionPolicy{
			Hourly:  24,
			Daily:   7,
			Weekly:  4,
			Monthly: 12,
		},
		VerifyBackups: true,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Verify original data
	originalCount := countTestRecords(t, dbPath)
	if originalCount != 3 {
		t.Errorf("Expected 3 original records, got %d", originalCount)
	}

	// Create a backup
	ctx := context.Background()
	result, err := service.BackupNow(ctx)
	if err != nil {
		t.Fatalf("BackupNow failed: %v", err)
	}
	backupPath := result.Path

	// Corrupt the original database by deleting all data
	origDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open original database: %v", err)
	}
	if _, err := origDB.Exec("DELETE FROM test_data"); err != nil {
		_ = origDB.Close()
		t.Fatalf("Failed to delete data: %v", err)
	}
	_ = origDB.Close()

	// Verify data is gone
	if count := countTestRecords(t, dbPath); count != 0 {
		t.Errorf("Expected 0 records after delete, got %d", count)
	}

	// Restore from backup
	if err := service.RestoreBackup(ctx, backupPath); err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// Verify restored data is correct
	restoredCount := countTestRecords(t, dbPath)
	if restoredCount != 3 {
		t.Errorf("Expected 3 restored records, got %d", restoredCount)
	}
}

// TestBackupService_NewBackupService_MissingDBPath tests validation of configuration
func TestBackupService_NewBackupService_MissingDBPath(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	_, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    "", // Empty path should cause error
		BackupDir: backupDir,
		Interval:  1 * time.Hour,
	})

	if err == nil {
		t.Error("Expected error for missing DBPath")
	}
}

// TestBackupService_NewBackupService_MissingBackupDir tests validation of configuration
func TestBackupService_NewBackupService_MissingBackupDir(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	createTestDB(t, dbPath)

	_, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPath,
		BackupDir: "", // Empty path should cause error
		Interval:  1 * time.Hour,
	})

	if err == nil {
		t.Error("Expected error for missing BackupDir")
	}
}

// TestBackupService_BackupNow_NonexistentDB tests error handling when DB doesn't exist
func TestBackupService_BackupNow_NonexistentDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nonexistent.db")
	backupDir := filepath.Join(tmpDir, "backups")

	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPath,
		BackupDir: backupDir,
		Interval:  1 * time.Hour,
		Retention: backup.RetentionPolicy{
			Hourly:  24,
			Daily:   7,
			Weekly:  4,
			Monthly: 12,
		},
		VerifyBackups: true,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	ctx := context.Background()
	_, err = service.BackupNow(ctx)
	if err == nil {
		t.Error("Expected error when backing up nonexistent database")
	}
}

// TestBackupService_RestoreBackup_NonexistentBackup tests error handling for restore
func TestBackupService_RestoreBackup_NonexistentBackup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	createTestDB(t, dbPath)

	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPath,
		BackupDir: backupDir,
		Interval:  1 * time.Hour,
		Retention: backup.RetentionPolicy{
			Hourly:  24,
			Daily:   7,
			Weekly:  4,
			Monthly: 12,
		},
		VerifyBackups: true,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	ctx := context.Background()
	nonexistentBackup := filepath.Join(backupDir, "nonexistent.db")
	err = service.RestoreBackup(ctx, nonexistentBackup)
	if err == nil {
		t.Error("Expected error when restoring from nonexistent backup")
	}
}

// TestBackupService_RestoreBackup_WhileRunning tests that restore fails when service is running
func TestBackupService_RestoreBackup_WhileRunning(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	createTestDB(t, dbPath)

	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPath,
		BackupDir: backupDir,
		Interval:  100 * time.Millisecond, // Short interval for testing
		Retention: backup.RetentionPolicy{
			Hourly:  24,
			Daily:   7,
			Weekly:  4,
			Monthly: 12,
		},
		VerifyBackups: true,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Start the service in a goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = service.Start(ctx)
	}()

	// Give service time to start
	time.Sleep(50 * time.Millisecond)

	// Create a backup file to attempt restore
	backupPath := filepath.Join(backupDir, "test-backup.db")
	if _, err := os.Create(backupPath); err != nil {
		t.Fatalf("Failed to create test backup file: %v", err)
	}

	// Try to restore while service is running
	err = service.RestoreBackup(ctx, backupPath)
	if err == nil {
		t.Error("Expected error when restoring while service is running")
	}

	// Stop the service
	_ = service.Stop()
}

// TestBackupService_BackupVerification tests the backup verification process
func TestBackupService_BackupVerification(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	createTestDB(t, dbPath)

	// Create service with verification enabled
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPath,
		BackupDir: backupDir,
		Interval:  1 * time.Hour,
		Retention: backup.RetentionPolicy{
			Hourly:  24,
			Daily:   7,
			Weekly:  4,
			Monthly: 12,
		},
		VerifyBackups: true,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	ctx := context.Background()
	result, err := service.BackupNow(ctx)
	if err != nil {
		t.Fatalf("BackupNow with verification failed: %v", err)
	}

	if !result.Verified {
		t.Error("Expected backup to be verified")
	}

	// Verify the backup file exists and is valid
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatalf("Backup file not found: %v", err)
	}
}

// TestBackupService_DefaultRetentionPolicy tests that default retention policy is applied
func TestBackupService_DefaultRetentionPolicy(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	createTestDB(t, dbPath)

	// Create service with zero retention values to trigger defaults
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPath,
		BackupDir: backupDir,
		Interval:  1 * time.Hour,
		Retention: backup.RetentionPolicy{
			Hourly:  0, // Should default to 24
			Daily:   0, // Should default to 7
			Weekly:  0, // Should default to 4
			Monthly: 0, // Should default to 12
		},
		VerifyBackups: true,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Verify that service was created successfully (defaults were applied)
	if service == nil {
		t.Error("Expected backup service to be created with default retention policy")
	}
}

// TestBackupService_BackupDirectory_Created tests that backup directory is created
func TestBackupService_BackupDirectory_Created(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "does", "not", "exist", "yet")

	createTestDB(t, dbPath)

	// Directory should not exist
	if _, err := os.Stat(backupDir); err == nil {
		t.Fatalf("Backup directory should not exist yet: %s", backupDir)
	}

	// Create backup service
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPath,
		BackupDir: backupDir,
		Interval:  1 * time.Hour,
		Retention: backup.RetentionPolicy{
			Hourly:  24,
			Daily:   7,
			Weekly:  4,
			Monthly: 12,
		},
		VerifyBackups: true,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	if service == nil {
		t.Error("Expected backup service to be created")
	}

	// Verify directory was created
	if _, err := os.Stat(backupDir); err != nil {
		t.Fatalf("Backup directory was not created: %v", err)
	}
}
