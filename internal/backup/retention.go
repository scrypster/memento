package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// listBackups lists all backup files in the backup directory with their metadata.
func listBackups(backupDir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only include .db files
		if !strings.HasSuffix(entry.Name(), ".db") {
			continue
		}

		path := filepath.Join(backupDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue // Skip files we can't stat
		}

		backups = append(backups, BackupInfo{
			Path:      path,
			Timestamp: info.ModTime(),
			Size:      info.Size(),
			Verified:  false, // Will be set during verification
		})
	}

	// Sort by timestamp, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

// applyRetention removes old backups according to the retention policy.
// It categorizes backups by age and keeps only the specified number in each tier.
func applyRetention(backupDir string, policy RetentionPolicy) error {
	backups, err := listBackups(backupDir)
	if err != nil {
		return err
	}

	if len(backups) == 0 {
		return nil // Nothing to do
	}

	now := time.Now()
	toDelete := []string{}

	// Categorize backups by age tier
	hourly := []BackupInfo{}
	daily := []BackupInfo{}
	weekly := []BackupInfo{}
	monthly := []BackupInfo{}

	for _, backup := range backups {
		age := now.Sub(backup.Timestamp)
		switch {
		case age < 24*time.Hour:
			hourly = append(hourly, backup)
		case age < 7*24*time.Hour:
			daily = append(daily, backup)
		case age < 30*24*time.Hour:
			weekly = append(weekly, backup)
		case age < 365*24*time.Hour:
			monthly = append(monthly, backup)
		default:
			// Backups older than 1 year are always deleted
			toDelete = append(toDelete, backup.Path)
		}
	}

	// Apply retention policy to each tier
	if len(hourly) > policy.Hourly {
		for _, backup := range hourly[policy.Hourly:] {
			toDelete = append(toDelete, backup.Path)
		}
	}

	if len(daily) > policy.Daily {
		for _, backup := range daily[policy.Daily:] {
			toDelete = append(toDelete, backup.Path)
		}
	}

	if len(weekly) > policy.Weekly {
		for _, backup := range weekly[policy.Weekly:] {
			toDelete = append(toDelete, backup.Path)
		}
	}

	if len(monthly) > policy.Monthly {
		for _, backup := range monthly[policy.Monthly:] {
			toDelete = append(toDelete, backup.Path)
		}
	}

	// Delete old backups
	var lastErr error
	for _, path := range toDelete {
		if err := os.Remove(path); err != nil {
			lastErr = err
			// Continue deleting other backups even if one fails
		}
	}

	if lastErr != nil {
		return fmt.Errorf("failed to delete some backups: %w", lastErr)
	}

	return nil
}

// calculateDiskUsage calculates total bytes used by all backups.
func calculateDiskUsage(backupDir string) (int64, error) {
	backups, err := listBackups(backupDir)
	if err != nil {
		return 0, err
	}

	var total int64
	for _, backup := range backups {
		total += backup.Size
	}

	return total, nil
}
