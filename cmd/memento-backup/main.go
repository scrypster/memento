// Command memento-backup runs the automated database backup service.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/scrypster/memento/internal/backup"
	"github.com/scrypster/memento/internal/config"
)

var (
	configPath = flag.String("config", "", "Path to config file (optional, uses env vars by default)")
	dbPath     = flag.String("db", "", "Path to database file (overrides config)")
	backupDir  = flag.String("backup-dir", "", "Backup directory path (overrides config)")
	interval   = flag.Duration("interval", 0, "Backup interval (overrides config)")
	verify     = flag.Bool("verify", true, "Verify backups after creation")
	oneshot    = flag.Bool("oneshot", false, "Perform a single backup and exit")
	restore    = flag.String("restore", "", "Restore database from backup file and exit")
	healthCmd  = flag.Bool("health", false, "Check backup service health and exit")
	listCmd    = flag.Bool("list", false, "List all available backups and exit")
)

func main() {
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Override config with command-line flags
	dbPathFinal := cfg.Storage.DataPath + "/memento.db"
	if *dbPath != "" {
		dbPathFinal = *dbPath
	}

	backupDirFinal := cfg.Backup.BackupPath
	if *backupDir != "" {
		backupDirFinal = *backupDir
	}

	intervalFinal := 1 * time.Hour
	if cfg.Backup.BackupInterval != "" {
		if d, err := time.ParseDuration(cfg.Backup.BackupInterval); err == nil {
			intervalFinal = d
		}
	}
	if *interval > 0 {
		intervalFinal = *interval
	}

	// Create backup service
	service, err := backup.NewBackupService(backup.BackupConfig{
		DBPath:    dbPathFinal,
		BackupDir: backupDirFinal,
		Interval:  intervalFinal,
		Retention: backup.RetentionPolicy{
			Hourly:  24,
			Daily:   7,
			Weekly:  4,
			Monthly: 12,
		},
		VerifyBackups: *verify,
	})
	if err != nil {
		log.Fatalf("Failed to create backup service: %v", err)
	}

	ctx := context.Background()

	// Handle command modes
	if *restore != "" {
		handleRestore(ctx, service, *restore)
		return
	}

	if *healthCmd {
		handleHealth(service)
		return
	}

	if *listCmd {
		handleList(service)
		return
	}

	if *oneshot {
		handleOneshot(ctx, service)
		return
	}

	// Start continuous backup service
	runService(ctx, service)
}

func handleRestore(ctx context.Context, service *backup.BackupService, backupPath string) {
	log.Printf("Restoring database from backup: %s", backupPath)

	if err := service.RestoreBackup(ctx, backupPath); err != nil {
		log.Fatalf("Restore failed: %v", err)
	}

	log.Println("Database restored successfully")
}

func handleHealth(service *backup.BackupService) {
	health, err := service.HealthCheck()
	if err != nil {
		log.Fatalf("Health check failed: %v", err)
	}

	fmt.Printf("Status: %s\n", health.Status)
	if health.Message != "" {
		fmt.Printf("Message: %s\n", health.Message)
	}
	fmt.Printf("Total Backups: %d\n", health.TotalBackups)
	fmt.Printf("Disk Space Used: %.2f MB\n", float64(health.DiskSpaceUsed)/(1024*1024))
	fmt.Printf("Backup Directory: %s\n", health.BackupDir)

	if !health.LastBackup.IsZero() {
		fmt.Printf("Last Backup: %s (%s ago)\n",
			health.LastBackup.Format(time.RFC3339),
			time.Since(health.LastBackup).Round(time.Minute))
	} else {
		fmt.Println("Last Backup: Never")
	}

	if !health.NextBackup.IsZero() {
		fmt.Printf("Next Backup: %s (in %s)\n",
			health.NextBackup.Format(time.RFC3339),
			time.Until(health.NextBackup).Round(time.Minute))
	}

	if health.Status != "healthy" {
		os.Exit(1)
	}
}

func handleList(service *backup.BackupService) {
	backups, err := service.ListBackups()
	if err != nil {
		log.Fatalf("Failed to list backups: %v", err)
	}

	if len(backups) == 0 {
		fmt.Println("No backups found")
		return
	}

	fmt.Printf("Found %d backup(s):\n\n", len(backups))
	for i, b := range backups {
		fmt.Printf("%d. %s\n", i+1, b.Path)
		fmt.Printf("   Size: %.2f MB\n", float64(b.Size)/(1024*1024))
		fmt.Printf("   Created: %s (%s ago)\n",
			b.Timestamp.Format(time.RFC3339),
			time.Since(b.Timestamp).Round(time.Minute))
		fmt.Println()
	}
}

func handleOneshot(ctx context.Context, service *backup.BackupService) {
	log.Println("Performing one-time backup...")

	result, err := service.BackupNow(ctx)
	if err != nil {
		log.Fatalf("Backup failed: %v", err)
	}

	log.Printf("Backup completed successfully:")
	log.Printf("  Path: %s", result.Path)
	log.Printf("  Size: %.2f MB", float64(result.Size)/(1024*1024))
	log.Printf("  Duration: %v", result.Duration)
	log.Printf("  Verified: %v", result.Verified)
}

func runService(ctx context.Context, service *backup.BackupService) {
	// Start service in background
	go func() {
		if err := service.Start(ctx); err != nil {
			if err != context.Canceled {
				log.Printf("Backup service error: %v", err)
			}
		}
	}()

	log.Println("Memento backup service started")
	log.Println("Press Ctrl+C to stop")

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down backup service...")
	if err := service.Stop(); err != nil {
		log.Printf("Warning: %v", err)
	}

	log.Println("Backup service stopped")
}
