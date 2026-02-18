// Package backup provides automated database backup and restore functionality
// with tiered retention policies and integrity verification.
package backup

import (
	"time"
)

// BackupConfig holds backup service configuration.
type BackupConfig struct {
	// DBPath is the path to the SQLite database file to backup
	DBPath string

	// BackupDir is the directory where backups will be stored
	BackupDir string

	// Interval is the duration between automated backups (default: 1 hour)
	Interval time.Duration

	// Retention defines how long to keep backups at different intervals
	Retention RetentionPolicy

	// VerifyBackups enables integrity checking after each backup (default: true)
	VerifyBackups bool
}

// RetentionPolicy defines how many backups to keep at each tier.
// Backups are categorized by age:
// - Hourly: backups less than 24 hours old
// - Daily: backups between 1-7 days old
// - Weekly: backups between 7-30 days old
// - Monthly: backups between 30-365 days old
type RetentionPolicy struct {
	// Hourly is the number of hourly backups to keep (default: 24)
	Hourly int

	// Daily is the number of daily backups to keep (default: 7)
	Daily int

	// Weekly is the number of weekly backups to keep (default: 4)
	Weekly int

	// Monthly is the number of monthly backups to keep (default: 12)
	Monthly int
}

// BackupInfo contains metadata about a backup file.
type BackupInfo struct {
	// Path is the full path to the backup file
	Path string

	// Timestamp is when the backup was created
	Timestamp time.Time

	// Size is the backup file size in bytes
	Size int64

	// Verified indicates if the backup passed integrity check
	Verified bool
}

// BackupResult contains the result of a backup operation.
type BackupResult struct {
	// Path is the path to the created backup file
	Path string

	// Duration is how long the backup took
	Duration time.Duration

	// Size is the backup file size in bytes
	Size int64

	// Verified indicates if the backup was verified successfully
	Verified bool

	// Error is any error that occurred during backup
	Error error
}

// HealthStatus represents the health of the backup service.
type HealthStatus struct {
	// Status is the overall health status: "healthy", "warning", or "error"
	Status string

	// Message provides additional context about the status
	Message string

	// LastBackup is when the last successful backup completed
	LastBackup time.Time

	// NextBackup is when the next backup is scheduled
	NextBackup time.Time

	// TotalBackups is the number of backups currently stored
	TotalBackups int

	// BackupDir is the backup storage directory
	BackupDir string

	// DiskSpaceUsed is total bytes used by all backups
	DiskSpaceUsed int64
}
