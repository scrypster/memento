package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestListBackupsEmpty tests listBackups with an empty directory.
func TestListBackupsEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	backups, err := listBackups(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(backups) != 0 {
		t.Errorf("expected 0 backups, got %d", len(backups))
	}
}

// TestListBackupsNonexistentDirectory tests listBackups with a non-existent directory.
func TestListBackupsNonexistentDirectory(t *testing.T) {
	_, err := listBackups("/nonexistent/backup/dir")
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

// TestListBackupsIgnoresNonDbFiles tests that listBackups ignores non-.db files.
func TestListBackupsIgnoresNonDbFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some non-.db files
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "data.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create one .db file
	dbFile := filepath.Join(tmpDir, "backup.db")
	if err := os.WriteFile(dbFile, []byte("sqlite"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	backups, err := listBackups(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(backups))
	}

	if backups[0].Path != dbFile {
		t.Errorf("expected path %s, got %s", dbFile, backups[0].Path)
	}
}

// TestListBackupsIgnoresDirectories tests that listBackups ignores subdirectories.
func TestListBackupsIgnoresDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory with a .db file inside
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "backup.db"), []byte("sqlite"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create a .db file in the root
	rootDB := filepath.Join(tmpDir, "backup.db")
	if err := os.WriteFile(rootDB, []byte("sqlite"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	backups, err := listBackups(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(backups))
	}

	if backups[0].Path != rootDB {
		t.Errorf("expected path %s, got %s", rootDB, backups[0].Path)
	}
}

// TestListBackupsSortNewestFirst tests that backups are sorted by timestamp, newest first.
func TestListBackupsSortNewestFirst(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()

	// Create backups with different timestamps
	files := []struct {
		name string
		time time.Time
	}{
		{"backup1.db", now.Add(-2 * time.Hour)},
		{"backup2.db", now.Add(-1 * time.Hour)},
		{"backup3.db", now},
		{"backup4.db", now.Add(-3 * time.Hour)},
	}

	for _, f := range files {
		path := filepath.Join(tmpDir, f.name)
		if err := os.WriteFile(path, []byte("sqlite"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
		if err := os.Chtimes(path, f.time, f.time); err != nil {
			t.Fatalf("failed to set file time: %v", err)
		}
	}

	backups, err := listBackups(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(backups) != 4 {
		t.Errorf("expected 4 backups, got %d", len(backups))
	}

	// Verify sorted by timestamp, newest first
	for i := 0; i < len(backups)-1; i++ {
		if backups[i].Timestamp.Before(backups[i+1].Timestamp) {
			t.Errorf("backups not sorted newest first: backup %d is older than backup %d", i, i+1)
		}
	}

	// Verify exact order
	if backups[0].Path != filepath.Join(tmpDir, "backup3.db") {
		t.Errorf("expected backup3.db first, got %s", filepath.Base(backups[0].Path))
	}
	if backups[1].Path != filepath.Join(tmpDir, "backup2.db") {
		t.Errorf("expected backup2.db second, got %s", filepath.Base(backups[1].Path))
	}
}

// TestListBackupsMetadata tests that metadata (Path, Timestamp, Size) is correctly populated.
func TestListBackupsMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "backup.db")
	content := []byte("sqlite database content")

	if err := os.WriteFile(dbPath, content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	now := time.Now()
	if err := os.Chtimes(dbPath, now, now); err != nil {
		t.Fatalf("failed to set file time: %v", err)
	}

	backups, err := listBackups(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(backups))
	}

	backup := backups[0]
	if backup.Path != dbPath {
		t.Errorf("expected path %s, got %s", dbPath, backup.Path)
	}

	if backup.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), backup.Size)
	}

	// Timestamp should be approximately now (within 1 second)
	if backup.Timestamp.Unix() != now.Unix() {
		t.Errorf("expected timestamp %v, got %v", now.Unix(), backup.Timestamp.Unix())
	}

	if backup.Verified {
		t.Errorf("expected Verified to be false, got true")
	}
}

// TestApplyRetentionEmptyDir tests applyRetention with an empty directory.
func TestApplyRetentionEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	policy := RetentionPolicy{Hourly: 24, Daily: 7, Weekly: 4, Monthly: 12}

	err := applyRetention(tmpDir, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestApplyRetentionDeletesFilesOlderThanOneYear tests that backups older than 365 days are deleted.
func TestApplyRetentionDeletesFilesOlderThanOneYear(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	policy := RetentionPolicy{Hourly: 24, Daily: 7, Weekly: 4, Monthly: 12}

	// Create a backup older than 1 year
	oldFile := filepath.Join(tmpDir, "backup_old.db")
	if err := os.WriteFile(oldFile, []byte("old backup"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	oldTime := now.Add(-366 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set file time: %v", err)
	}

	// Create a recent backup (should be kept)
	recentFile := filepath.Join(tmpDir, "backup_recent.db")
	if err := os.WriteFile(recentFile, []byte("recent backup"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.Chtimes(recentFile, now, now); err != nil {
		t.Fatalf("failed to set file time: %v", err)
	}

	err := applyRetention(tmpDir, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify old file was deleted
	if _, err := os.Stat(oldFile); err == nil {
		t.Errorf("expected old backup to be deleted")
	}

	// Verify recent file still exists
	if _, err := os.Stat(recentFile); err != nil {
		t.Errorf("expected recent backup to exist: %v", err)
	}
}

// TestApplyRetentionHourlyTier tests that only the specified number of hourly backups are kept.
func TestApplyRetentionHourlyTier(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	policy := RetentionPolicy{Hourly: 2, Daily: 0, Weekly: 0, Monthly: 0}

	// Create 5 backups in the hourly tier (< 24 hours old)
	for i := 0; i < 5; i++ {
		path := filepath.Join(tmpDir, "backup_hourly_"+string(rune(48+i))+".db")
		if err := os.WriteFile(path, []byte("backup"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
		// Space them 1 hour apart, newest first in creation
		timestamp := now.Add(-time.Duration(i) * time.Hour)
		if err := os.Chtimes(path, timestamp, timestamp); err != nil {
			t.Fatalf("failed to set file time: %v", err)
		}
	}

	err := applyRetention(tmpDir, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// List remaining files
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read backup directory: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 backups to remain, got %d", len(entries))
	}
}

// TestApplyRetentionDailyTier tests that only the specified number of daily backups are kept.
func TestApplyRetentionDailyTier(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	policy := RetentionPolicy{Hourly: 0, Daily: 2, Weekly: 0, Monthly: 0}

	// Create 4 backups in the daily tier (1-7 days old)
	for i := 0; i < 4; i++ {
		path := filepath.Join(tmpDir, "backup_daily_"+string(rune(48+i))+".db")
		if err := os.WriteFile(path, []byte("backup"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
		// Space them 1 day apart, all between 1 and 7 days old
		timestamp := now.Add(-time.Duration(2+i) * 24 * time.Hour)
		if err := os.Chtimes(path, timestamp, timestamp); err != nil {
			t.Fatalf("failed to set file time: %v", err)
		}
	}

	err := applyRetention(tmpDir, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read backup directory: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 backups to remain, got %d", len(entries))
	}
}

// TestApplyRetentionWeeklyTier tests that only the specified number of weekly backups are kept.
func TestApplyRetentionWeeklyTier(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	policy := RetentionPolicy{Hourly: 0, Daily: 0, Weekly: 1, Monthly: 0}

	// Create 3 backups in the weekly tier (7-30 days old)
	for i := 0; i < 3; i++ {
		path := filepath.Join(tmpDir, "backup_weekly_"+string(rune(48+i))+".db")
		if err := os.WriteFile(path, []byte("backup"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
		// Space them between 7 and 30 days old
		timestamp := now.Add(-time.Duration(8+i*7) * 24 * time.Hour)
		if err := os.Chtimes(path, timestamp, timestamp); err != nil {
			t.Fatalf("failed to set file time: %v", err)
		}
	}

	err := applyRetention(tmpDir, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read backup directory: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 backup to remain, got %d", len(entries))
	}
}

// TestApplyRetentionMonthlyTier tests that only the specified number of monthly backups are kept.
func TestApplyRetentionMonthlyTier(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	policy := RetentionPolicy{Hourly: 0, Daily: 0, Weekly: 0, Monthly: 2}

	// Create 4 backups in the monthly tier (30-365 days old)
	for i := 0; i < 4; i++ {
		path := filepath.Join(tmpDir, "backup_monthly_"+string(rune(48+i))+".db")
		if err := os.WriteFile(path, []byte("backup"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
		// Space them between 30 and 365 days old
		timestamp := now.Add(-time.Duration(31+i*90) * 24 * time.Hour)
		if err := os.Chtimes(path, timestamp, timestamp); err != nil {
			t.Fatalf("failed to set file time: %v", err)
		}
	}

	err := applyRetention(tmpDir, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read backup directory: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 backups to remain, got %d", len(entries))
	}
}

// TestApplyRetentionMixedTiers tests retention with backups spanning multiple tiers.
func TestApplyRetentionMixedTiers(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	policy := RetentionPolicy{Hourly: 2, Daily: 2, Weekly: 1, Monthly: 1}

	// Hourly tier: 3 backups (keep 2)
	for i := 0; i < 3; i++ {
		path := filepath.Join(tmpDir, "hourly_"+string(rune(48+i))+".db")
		if err := os.WriteFile(path, []byte("backup"), 0644); err != nil {
			t.Fatalf("failed to create hourly backup: %v", err)
		}
		timestamp := now.Add(-time.Duration(i) * 30 * time.Minute)
		if err := os.Chtimes(path, timestamp, timestamp); err != nil {
			t.Fatalf("failed to set file time: %v", err)
		}
	}

	// Daily tier: 3 backups (keep 2)
	for i := 0; i < 3; i++ {
		path := filepath.Join(tmpDir, "daily_"+string(rune(48+i))+".db")
		if err := os.WriteFile(path, []byte("backup"), 0644); err != nil {
			t.Fatalf("failed to create daily backup: %v", err)
		}
		timestamp := now.Add(-time.Duration(2+i) * 24 * time.Hour)
		if err := os.Chtimes(path, timestamp, timestamp); err != nil {
			t.Fatalf("failed to set file time: %v", err)
		}
	}

	// Weekly tier: 2 backups (keep 1)
	for i := 0; i < 2; i++ {
		path := filepath.Join(tmpDir, "weekly_"+string(rune(48+i))+".db")
		if err := os.WriteFile(path, []byte("backup"), 0644); err != nil {
			t.Fatalf("failed to create weekly backup: %v", err)
		}
		timestamp := now.Add(-time.Duration(8+i*7) * 24 * time.Hour)
		if err := os.Chtimes(path, timestamp, timestamp); err != nil {
			t.Fatalf("failed to set file time: %v", err)
		}
	}

	// Monthly tier: 2 backups (keep 1)
	for i := 0; i < 2; i++ {
		path := filepath.Join(tmpDir, "monthly_"+string(rune(48+i))+".db")
		if err := os.WriteFile(path, []byte("backup"), 0644); err != nil {
			t.Fatalf("failed to create monthly backup: %v", err)
		}
		timestamp := now.Add(-time.Duration(31+i*90) * 24 * time.Hour)
		if err := os.Chtimes(path, timestamp, timestamp); err != nil {
			t.Fatalf("failed to set file time: %v", err)
		}
	}

	err := applyRetention(tmpDir, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Total: 2 hourly + 2 daily + 1 weekly + 1 monthly = 6 remaining
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read backup directory: %v", err)
	}

	if len(entries) != 6 {
		t.Errorf("expected 6 backups to remain, got %d", len(entries))
	}
}

// TestApplyRetentionKeepsExactlyNeeded tests that exactly the right number of files are kept.
func TestApplyRetentionKeepsExactlyNeeded(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	policy := RetentionPolicy{Hourly: 3, Daily: 0, Weekly: 0, Monthly: 0}

	// Create exactly 3 hourly backups
	for i := 0; i < 3; i++ {
		path := filepath.Join(tmpDir, "backup_"+string(rune(48+i))+".db")
		if err := os.WriteFile(path, []byte("backup"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
		timestamp := now.Add(-time.Duration(i) * time.Hour)
		if err := os.Chtimes(path, timestamp, timestamp); err != nil {
			t.Fatalf("failed to set file time: %v", err)
		}
	}

	err := applyRetention(tmpDir, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read backup directory: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 backups to remain, got %d", len(entries))
	}
}

// TestApplyRetentionNonexistentDirectory tests applyRetention with non-existent directory.
func TestApplyRetentionNonexistentDirectory(t *testing.T) {
	policy := RetentionPolicy{Hourly: 24, Daily: 7, Weekly: 4, Monthly: 12}
	err := applyRetention("/nonexistent/backup/dir", policy)
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

// TestCalculateDiskUsageEmpty tests calculateDiskUsage with an empty directory.
func TestCalculateDiskUsageEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	usage, err := calculateDiskUsage(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if usage != 0 {
		t.Errorf("expected 0 bytes, got %d", usage)
	}
}

// TestCalculateDiskUsageSingleFile tests calculateDiskUsage with a single file.
func TestCalculateDiskUsageSingleFile(t *testing.T) {
	tmpDir := t.TempDir()
	content := []byte("hello world backup")
	dbPath := filepath.Join(tmpDir, "backup.db")

	if err := os.WriteFile(dbPath, content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	usage, err := calculateDiskUsage(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := int64(len(content))
	if usage != expected {
		t.Errorf("expected %d bytes, got %d", expected, usage)
	}
}

// TestCalculateDiskUsageMultipleFiles tests calculateDiskUsage with multiple files.
func TestCalculateDiskUsageMultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()
	sizes := []int64{100, 250, 500, 1000}
	var expectedTotal int64

	for i, size := range sizes {
		content := make([]byte, size)
		path := filepath.Join(tmpDir, "backup_0"+string(rune(48+i))+".db")
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
		expectedTotal += size
	}

	usage, err := calculateDiskUsage(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if usage != expectedTotal {
		t.Errorf("expected %d bytes, got %d", expectedTotal, usage)
	}
}

// TestCalculateDiskUsageIgnoresNonDbFiles tests that calculateDiskUsage ignores non-.db files.
func TestCalculateDiskUsageIgnoresNonDbFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create non-.db files
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("large file with 100 bytes here!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"), 0644); err != nil {
		t.Fatalf("failed to create non-db file: %v", err)
	}

	// Create a .db file with 50 bytes
	content := make([]byte, 50)
	if err := os.WriteFile(filepath.Join(tmpDir, "backup.db"), content, 0644); err != nil {
		t.Fatalf("failed to create db file: %v", err)
	}

	usage, err := calculateDiskUsage(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only count the .db file
	if usage != 50 {
		t.Errorf("expected 50 bytes (only .db file), got %d", usage)
	}
}

// TestCalculateDiskUsageNonexistentDirectory tests calculateDiskUsage with non-existent directory.
func TestCalculateDiskUsageNonexistentDirectory(t *testing.T) {
	_, err := calculateDiskUsage("/nonexistent/backup/dir")
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

// TestCalculateDiskUsageLargeFiles tests calculateDiskUsage with large files.
func TestCalculateDiskUsageLargeFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a 1MB file
	largeContent := make([]byte, 1024*1024)
	path1 := filepath.Join(tmpDir, "large_01.db")
	if err := os.WriteFile(path1, largeContent, 0644); err != nil {
		t.Fatalf("failed to create large file: %v", err)
	}

	// Create a 500KB file
	mediumContent := make([]byte, 512*1024)
	path2 := filepath.Join(tmpDir, "large_02.db")
	if err := os.WriteFile(path2, mediumContent, 0644); err != nil {
		t.Fatalf("failed to create medium file: %v", err)
	}

	usage, err := calculateDiskUsage(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := int64(1024*1024 + 512*1024)
	if usage != expected {
		t.Errorf("expected %d bytes, got %d", expected, usage)
	}
}
