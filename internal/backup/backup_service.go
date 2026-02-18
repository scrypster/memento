package backup

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BackupService handles automated database backups with verification and retention.
type BackupService struct {
	dbPath        string
	backupDir     string
	interval      time.Duration
	retention     RetentionPolicy
	verifyBackups bool

	// Internal state
	mu             sync.Mutex
	running        bool
	stopCh         chan struct{}
	lastBackupTime time.Time
	nextBackupTime time.Time
}

// NewBackupService creates a new backup service with the given configuration.
func NewBackupService(config BackupConfig) (*BackupService, error) {
	// Validate configuration
	if config.DBPath == "" {
		return nil, fmt.Errorf("database path is required")
	}

	if config.BackupDir == "" {
		return nil, fmt.Errorf("backup directory is required")
	}

	if config.Interval <= 0 {
		config.Interval = 1 * time.Hour // Default to 1 hour
	}

	// Set default retention policy if not specified
	if config.Retention.Hourly == 0 {
		config.Retention.Hourly = 24
	}
	if config.Retention.Daily == 0 {
		config.Retention.Daily = 7
	}
	if config.Retention.Weekly == 0 {
		config.Retention.Weekly = 4
	}
	if config.Retention.Monthly == 0 {
		config.Retention.Monthly = 12
	}

	// Create backup directory if it doesn't exist
	if err := os.MkdirAll(config.BackupDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	return &BackupService{
		dbPath:        config.DBPath,
		backupDir:     config.BackupDir,
		interval:      config.Interval,
		retention:     config.Retention,
		verifyBackups: config.VerifyBackups,
		stopCh:        make(chan struct{}),
	}, nil
}

// Start starts the automated backup service.
// It runs in the background and performs backups at the configured interval.
func (s *BackupService) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("backup service is already running")
	}
	s.running = true
	s.nextBackupTime = time.Now().Add(s.interval)
	s.mu.Unlock()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	log.Printf("Backup service started: interval=%v, backup_dir=%s", s.interval, s.backupDir)

	for {
		select {
		case <-ctx.Done():
			log.Println("Backup service stopping (context cancelled)")
			return ctx.Err()

		case <-s.stopCh:
			log.Println("Backup service stopping (stop requested)")
			return nil

		case <-ticker.C:
			log.Println("Starting scheduled backup...")
			result, err := s.BackupNow(ctx)
			if err != nil {
				log.Printf("Scheduled backup failed: %v", err)
			} else {
				log.Printf("Scheduled backup completed: path=%s, size=%d bytes, duration=%v, verified=%v",
					result.Path, result.Size, result.Duration, result.Verified)
			}

			// Update next backup time
			s.mu.Lock()
			s.nextBackupTime = time.Now().Add(s.interval)
			s.mu.Unlock()
		}
	}
}

// Stop stops the backup service gracefully.
func (s *BackupService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("backup service is not running")
	}

	close(s.stopCh)
	s.running = false
	return nil
}

// BackupNow performs an immediate backup of the database.
// It creates a timestamped backup file, optionally verifies it,
// and applies the retention policy.
func (s *BackupService) BackupNow(ctx context.Context) (*BackupResult, error) {
	startTime := time.Now()

	// Check if database exists
	if _, err := os.Stat(s.dbPath); err != nil {
		return nil, fmt.Errorf("database not found: %w", err)
	}

	// Generate backup filename with timestamp (including microseconds for uniqueness)
	timestamp := time.Now().Format("20060102-150405.000000")
	backupName := fmt.Sprintf("memento-backup-%s.db", timestamp)
	backupPath := filepath.Join(s.backupDir, backupName)

	// Perform backup
	if err := backupSQLite(s.dbPath, backupPath); err != nil {
		return &BackupResult{
			Path:     backupPath,
			Duration: time.Since(startTime),
			Error:    err,
		}, err
	}

	// Get backup file size
	info, err := os.Stat(backupPath)
	if err != nil {
		return &BackupResult{
			Path:     backupPath,
			Duration: time.Since(startTime),
			Error:    fmt.Errorf("failed to stat backup: %w", err),
		}, err
	}

	result := &BackupResult{
		Path:     backupPath,
		Duration: time.Since(startTime),
		Size:     info.Size(),
		Verified: false,
	}

	// Verify backup if enabled
	if s.verifyBackups {
		if err := verifyBackup(backupPath); err != nil {
			result.Error = fmt.Errorf("backup verification failed: %w", err)
			return result, result.Error
		}
		result.Verified = true
	}

	// Update last backup time
	s.mu.Lock()
	s.lastBackupTime = time.Now()
	s.mu.Unlock()

	// Apply retention policy
	if err := applyRetention(s.backupDir, s.retention); err != nil {
		log.Printf("Warning: failed to apply retention policy: %v", err)
		// Don't fail the backup operation due to retention errors
	}

	return result, nil
}

// ListBackups lists all available backups.
func (s *BackupService) ListBackups() ([]BackupInfo, error) {
	return listBackups(s.backupDir)
}

// RestoreBackup restores the database from a backup file.
// The service must be stopped before calling this function.
func (s *BackupService) RestoreBackup(ctx context.Context, backupPath string) error {
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()

	if running {
		return fmt.Errorf("cannot restore while backup service is running")
	}

	// Check if backup exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	// Create a temporary backup of the current database
	tempBackup := s.dbPath + ".pre-restore"
	if _, err := os.Stat(s.dbPath); err == nil {
		if err := backupSQLite(s.dbPath, tempBackup); err != nil {
			return fmt.Errorf("failed to create pre-restore backup: %w", err)
		}
		defer func() {
			// Clean up temp backup on success
			os.Remove(tempBackup)
		}()
	}

	// Restore from backup
	if err := restoreSQLite(backupPath, s.dbPath); err != nil {
		// Try to restore from temp backup on failure
		if _, statErr := os.Stat(tempBackup); statErr == nil {
			if restoreErr := restoreSQLite(tempBackup, s.dbPath); restoreErr != nil {
				return fmt.Errorf("restore failed and rollback failed: %v (restore error: %w)", restoreErr, err)
			}
			return fmt.Errorf("restore failed, rolled back to previous state: %w", err)
		}
		return err
	}

	log.Printf("Database restored from backup: %s", backupPath)
	return nil
}

// HealthCheck returns the current health status of the backup service.
func (s *BackupService) HealthCheck() (*HealthStatus, error) {
	s.mu.Lock()
	lastBackup := s.lastBackupTime
	nextBackup := s.nextBackupTime
	s.mu.Unlock()

	// Count backups
	backups, err := s.ListBackups()
	if err != nil {
		return nil, fmt.Errorf("failed to list backups: %w", err)
	}

	// Calculate disk usage
	diskUsage, err := calculateDiskUsage(s.backupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate disk usage: %w", err)
	}

	status := &HealthStatus{
		LastBackup:    lastBackup,
		NextBackup:    nextBackup,
		TotalBackups:  len(backups),
		BackupDir:     s.backupDir,
		DiskSpaceUsed: diskUsage,
		Status:        "healthy",
	}

	// Check if backup is overdue
	if !lastBackup.IsZero() && time.Since(lastBackup) > s.interval*2 {
		status.Status = "warning"
		status.Message = fmt.Sprintf("Backup overdue by %v", time.Since(lastBackup)-s.interval)
	} else if lastBackup.IsZero() {
		status.Status = "healthy"
		status.Message = "No backups yet"
	} else {
		status.Message = fmt.Sprintf("Last backup: %v ago", time.Since(lastBackup).Round(time.Minute))
	}

	return status, nil
}
