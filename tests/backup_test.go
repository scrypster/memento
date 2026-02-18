package tests

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

// TestBackupService_BackupNow tests immediate backup creation and verification.
func TestBackupService_BackupNow(t *testing.T) {
	// Setup: create a test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup directory: %v", err)
	}

	// Create and populate test database
	db, err := createTestDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Insert test data
	_, err = db.Exec("INSERT INTO memories (id, content, created_at, updated_at) VALUES (?, ?, ?, ?)",
		"test-id", "test content", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}
	db.Close()

	// Create backup service
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:        dbPath,
		BackupDir:     backupDir,
		Interval:      1 * time.Hour,
		VerifyBackups: true,
		Retention: backup.RetentionPolicy{
			Hourly:  24,
			Daily:   7,
			Weekly:  4,
			Monthly: 12,
		},
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

	// Verify result
	if result.Path == "" {
		t.Error("Expected backup path, got empty string")
	}

	if result.Size == 0 {
		t.Error("Expected backup size > 0")
	}

	if !result.Verified {
		t.Error("Expected backup to be verified")
	}

	if result.Duration == 0 {
		t.Error("Expected backup duration > 0")
	}

	// Verify backup file exists
	if _, err := os.Stat(result.Path); err != nil {
		t.Errorf("Backup file not found: %v", err)
	}

	// List backups
	backups, err := service.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	if len(backups) != 1 {
		t.Errorf("Expected 1 backup, got %d", len(backups))
	}
}

// TestBackupService_Verify tests backup integrity verification.
func TestBackupService_Verify(t *testing.T) {
	// Setup: create a test backup
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup directory: %v", err)
	}

	// Create test database
	db, err := createTestDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	db.Close()

	// Create backup service with verification enabled
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:        dbPath,
		BackupDir:     backupDir,
		VerifyBackups: true,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Create verified backup
	ctx := context.Background()
	result, err := service.BackupNow(ctx)
	if err != nil {
		t.Fatalf("BackupNow failed: %v", err)
	}

	// Check that backup was verified
	if !result.Verified {
		t.Error("Expected backup to be verified")
	}

	// Test with verification disabled
	serviceNoVerify, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:        dbPath,
		BackupDir:     backupDir,
		VerifyBackups: false,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	result2, err := serviceNoVerify.BackupNow(ctx)
	if err != nil {
		t.Fatalf("BackupNow failed: %v", err)
	}

	if result2.Verified {
		t.Error("Expected backup to not be verified when VerifyBackups=false")
	}
}

// TestBackupService_Retention tests the retention policy application.
func TestBackupService_Retention(t *testing.T) {
	// Setup: create test database and backups
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup directory: %v", err)
	}

	// Create test database
	db, err := createTestDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	db.Close()

	// Create backup service with strict retention policy
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPath,
		BackupDir: backupDir,
		Retention: backup.RetentionPolicy{
			Hourly:  2, // Keep only 2 hourly backups
			Daily:   1, // Keep only 1 daily backup
			Weekly:  1, // Keep only 1 weekly backup
			Monthly: 1, // Keep only 1 monthly backup
		},
		VerifyBackups: false, // Faster for testing
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Create test backups at different intervals
	now := time.Now()
	backupFiles := []struct {
		name string
		age  time.Duration
	}{
		{"memento-backup-hour1.db", 1 * time.Hour},
		{"memento-backup-hour2.db", 2 * time.Hour},
		{"memento-backup-hour3.db", 3 * time.Hour}, // Should be deleted (>2 hourly)
		{"memento-backup-day1.db", 26 * time.Hour},
		{"memento-backup-day2.db", 48 * time.Hour},   // Should be deleted (>1 daily)
		{"memento-backup-week1.db", 8 * 24 * time.Hour},
		{"memento-backup-week2.db", 14 * 24 * time.Hour}, // Should be deleted (>1 weekly)
		{"memento-backup-month1.db", 31 * 24 * time.Hour},
	}

	for _, bf := range backupFiles {
		path := filepath.Join(backupDir, bf.name)
		// Create a valid SQLite database for each backup
		testDB, err := createTestDatabase(path)
		if err != nil {
			t.Fatalf("Failed to create test backup: %v", err)
		}
		testDB.Close()

		// Set modification time to simulate age
		mtime := now.Add(-bf.age)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("Failed to set backup timestamp: %v", err)
		}
	}

	// Trigger retention by creating a new backup
	ctx := context.Background()
	_, err = service.BackupNow(ctx)
	if err != nil {
		t.Fatalf("BackupNow failed: %v", err)
	}

	// Check remaining backups
	remaining, err := service.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	// Should have: 2 hourly + 1 daily + 1 weekly + 1 monthly + 1 new = 6 backups
	// (The retention policy keeps the newest backups in each tier)
	if len(remaining) > 6 {
		t.Errorf("Expected at most 6 backups after retention, got %d", len(remaining))
		for _, b := range remaining {
			t.Logf("Remaining backup: %s (age: %v)", b.Path, time.Since(b.Timestamp))
		}
	}
}

// TestBackupService_Schedule tests automated scheduled backups.
func TestBackupService_Schedule(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping scheduled backup test in short mode")
	}

	// Setup: create test database and backup service
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup directory: %v", err)
	}

	// Create test database
	db, err := createTestDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	db.Close()

	// Create backup service with short interval for testing
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:        dbPath,
		BackupDir:     backupDir,
		Interval:      500 * time.Millisecond, // Short interval for testing
		VerifyBackups: false,                   // Faster for testing
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Start service in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		service.Start(ctx)
		close(done)
	}()

	// Wait for at least 2 backups to be created using polling
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	backupsCreated := false
	for !backupsCreated {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for backups to be created")
		case <-ticker.C:
			backups, err := service.ListBackups()
			if err != nil {
				// Keep trying
				continue
			}
			if len(backups) >= 2 {
				backupsCreated = true
			}
		}
	}

	// Stop service gracefully
	cancel()

	// Wait for service to stop
	<-done

	// Check that backups were created
	backups, err := service.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	if len(backups) < 2 {
		t.Errorf("Expected at least 2 scheduled backups, got %d", len(backups))
	}
}

// TestBackupService_RestoreBackup tests restoring from a backup.
func TestBackupService_RestoreBackup(t *testing.T) {
	// Setup: create test database and backup
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup directory: %v", err)
	}

	// Create and populate test database
	db, err := createTestDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Insert test data
	originalData := "original memory content"
	_, err = db.Exec("INSERT INTO memories (id, content, created_at, updated_at) VALUES (?, ?, ?, ?)",
		"test-id", originalData, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}
	db.Close()

	// Create backup service
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:        dbPath,
		BackupDir:     backupDir,
		VerifyBackups: true,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Create backup
	ctx := context.Background()
	result, err := service.BackupNow(ctx)
	if err != nil {
		t.Fatalf("BackupNow failed: %v", err)
	}

	// Modify original database
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	modifiedData := "modified content"
	_, err = db.Exec("UPDATE memories SET content = ? WHERE id = ?", modifiedData, "test-id")
	if err != nil {
		t.Fatalf("Failed to modify database: %v", err)
	}
	db.Close()

	// Verify modification
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	var content string
	err = db.QueryRow("SELECT content FROM memories WHERE id = ?", "test-id").Scan(&content)
	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}
	db.Close()
	if content != modifiedData {
		t.Fatalf("Expected modified content %q, got %q", modifiedData, content)
	}

	// Restore from backup
	err = service.RestoreBackup(ctx, result.Path)
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// Verify restoration
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	err = db.QueryRow("SELECT content FROM memories WHERE id = ?", "test-id").Scan(&content)
	if err != nil {
		t.Fatalf("Failed to query restored database: %v", err)
	}
	db.Close()

	if content != originalData {
		t.Errorf("Expected original content %q after restore, got %q", originalData, content)
	}
}

// TestBackupService_HealthCheck tests the health monitoring.
func TestBackupService_HealthCheck(t *testing.T) {
	// Setup: create backup service
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup directory: %v", err)
	}

	// Create test database
	db, err := createTestDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	db.Close()

	// Create backup service
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:        dbPath,
		BackupDir:     backupDir,
		Interval:      1 * time.Hour,
		VerifyBackups: false,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Check initial health (no backups yet)
	health, err := service.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got %q", health.Status)
	}

	if health.TotalBackups != 0 {
		t.Errorf("Expected 0 backups, got %d", health.TotalBackups)
	}

	if health.DiskSpaceUsed != 0 {
		t.Errorf("Expected 0 disk space used, got %d", health.DiskSpaceUsed)
	}

	// Create a backup
	ctx := context.Background()
	_, err = service.BackupNow(ctx)
	if err != nil {
		t.Fatalf("BackupNow failed: %v", err)
	}

	// Check health after backup
	health, err = service.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got %q", health.Status)
	}

	if health.TotalBackups != 1 {
		t.Errorf("Expected 1 backup, got %d", health.TotalBackups)
	}

	if health.DiskSpaceUsed == 0 {
		t.Error("Expected disk space used > 0")
	}

	if health.LastBackup.IsZero() {
		t.Error("Expected LastBackup to be set")
	}
}

// TestBackupService_DiskSpace tests disk space monitoring.
func TestBackupService_DiskSpace(t *testing.T) {
	// Setup: create test database and multiple backups
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup directory: %v", err)
	}

	// Create test database
	db, err := createTestDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	// Add some data to make the database larger
	for i := 0; i < 100; i++ {
		_, err = db.Exec("INSERT INTO memories (id, content, created_at, updated_at) VALUES (?, ?, ?, ?)",
			fmt.Sprintf("id-%d", i), fmt.Sprintf("content-%d", i), time.Now(), time.Now())
		if err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}
	db.Close()

	// Create backup service
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:        dbPath,
		BackupDir:     backupDir,
		VerifyBackups: false,
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Create multiple backups
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, err = service.BackupNow(ctx)
		if err != nil {
			t.Fatalf("BackupNow failed: %v", err)
		}
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Check disk usage
	health, err := service.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if health.DiskSpaceUsed == 0 {
		t.Error("Expected disk space used > 0")
	}

	// Verify disk space calculation
	backups, err := service.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	var expectedSize int64
	for _, b := range backups {
		expectedSize += b.Size
	}

	if health.DiskSpaceUsed != expectedSize {
		t.Errorf("Expected disk space %d, got %d", expectedSize, health.DiskSpaceUsed)
	}
}

// TestBackupService_ConcurrentBackups tests thread-safe concurrent operations.
func TestBackupService_ConcurrentBackups(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent backup test in short mode")
	}

	// Setup: create test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup directory: %v", err)
	}

	// Create test database
	db, err := createTestDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	db.Close()

	// Create backup service
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:        dbPath,
		BackupDir:     backupDir,
		VerifyBackups: false, // Faster for testing
	})
	if err != nil {
		t.Fatalf("Failed to create backup service: %v", err)
	}

	// Test concurrent BackupNow calls
	ctx := context.Background()
	const numBackups = 5
	errCh := make(chan error, numBackups)

	for i := 0; i < numBackups; i++ {
		go func() {
			_, err := service.BackupNow(ctx)
			errCh <- err
		}()
		time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	}

	// Wait for all backups to complete
	for i := 0; i < numBackups; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("Concurrent backup failed: %v", err)
		}
	}

	// Verify all backups were created
	backups, err := service.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	if len(backups) < numBackups {
		t.Errorf("Expected at least %d backups, got %d", numBackups, len(backups))
	}

	// Test concurrent health checks
	const numHealthChecks = 10
	healthErrCh := make(chan error, numHealthChecks)

	for i := 0; i < numHealthChecks; i++ {
		go func() {
			_, err := service.HealthCheck()
			healthErrCh <- err
		}()
	}

	for i := 0; i < numHealthChecks; i++ {
		if err := <-healthErrCh; err != nil {
			t.Errorf("Concurrent health check failed: %v", err)
		}
	}
}

// Helper functions

// createTestDatabase creates a test SQLite database with schema.
func createTestDatabase(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Create minimal schema for testing
	schema := `
		CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return db, nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
