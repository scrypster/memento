#!/bin/bash
# Automated backup script for Memento SQLite database
# This script runs in a loop, creating timestamped backups and cleaning up old ones

set -e  # Exit on error

# Configuration from environment variables with sensible defaults
BACKUP_INTERVAL=${BACKUP_INTERVAL:-3600}        # Default: 1 hour
BACKUP_RETENTION_DAYS=${BACKUP_RETENTION_DAYS:-7}  # Default: 7 days
BACKUP_SOURCE=${BACKUP_SOURCE:-/data}
BACKUP_DEST=${BACKUP_DEST:-/backups}

# Database file path
DB_FILE="${BACKUP_SOURCE}/memento.db"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')] INFO:${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] WARN:${NC} $1"
}

log_error() {
    echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] ERROR:${NC} $1"
}

# Function to perform backup
perform_backup() {
    local timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_file="${BACKUP_DEST}/memento_${timestamp}.db"
    local temp_backup="${backup_file}.tmp"

    log_info "Starting backup to ${backup_file}"

    # Check if database exists
    if [ ! -f "${DB_FILE}" ]; then
        log_warn "Database file not found: ${DB_FILE}"
        return 1
    fi

    # Check database size
    local db_size=$(stat -f%z "${DB_FILE}" 2>/dev/null || stat -c%s "${DB_FILE}" 2>/dev/null)
    log_info "Database size: $((db_size / 1024)) KB"

    # Create backup using SQLite backup API
    # This method ensures consistency even while database is in use
    if sqlite3 "${DB_FILE}" ".backup '${temp_backup}'" 2>&1; then
        # Move temp backup to final location
        mv "${temp_backup}" "${backup_file}"
        log_info "Backup created successfully"

        # Verify backup integrity
        if sqlite3 "${backup_file}" "PRAGMA integrity_check;" > /dev/null 2>&1; then
            log_info "Backup integrity verified"

            # Calculate and log backup size
            local backup_size=$(stat -f%z "${backup_file}" 2>/dev/null || stat -c%s "${backup_file}" 2>/dev/null)
            log_info "Backup size: $((backup_size / 1024)) KB"

            return 0
        else
            log_error "Backup integrity check failed"
            rm -f "${backup_file}"
            return 1
        fi
    else
        log_error "Backup creation failed"
        rm -f "${temp_backup}"
        return 1
    fi
}

# Function to clean up old backups
cleanup_old_backups() {
    log_info "Cleaning up backups older than ${BACKUP_RETENTION_DAYS} days"

    # Count backups before cleanup
    local backup_count=$(find "${BACKUP_DEST}" -name "memento_*.db" -type f | wc -l | tr -d ' ')
    log_info "Current backup count: ${backup_count}"

    # Find and delete old backups
    local deleted_count=0
    while IFS= read -r old_backup; do
        if [ -n "$old_backup" ]; then
            log_info "Deleting old backup: $(basename "$old_backup")"
            rm -f "$old_backup"
            ((deleted_count++))
        fi
    done < <(find "${BACKUP_DEST}" -name "memento_*.db" -type f -mtime +${BACKUP_RETENTION_DAYS})

    if [ $deleted_count -gt 0 ]; then
        log_info "Deleted ${deleted_count} old backup(s)"
    else
        log_info "No old backups to delete"
    fi

    # Count backups after cleanup
    backup_count=$(find "${BACKUP_DEST}" -name "memento_*.db" -type f | wc -l | tr -d ' ')
    log_info "Remaining backup count: ${backup_count}"
}

# Function to calculate disk usage
check_disk_usage() {
    local backup_dir_size=$(du -sh "${BACKUP_DEST}" 2>/dev/null | cut -f1)
    log_info "Backup directory size: ${backup_dir_size}"
}

# Main loop
log_info "=== Memento Backup Service Started ==="
log_info "Configuration:"
log_info "  - Backup interval: ${BACKUP_INTERVAL} seconds ($((BACKUP_INTERVAL / 60)) minutes)"
log_info "  - Retention period: ${BACKUP_RETENTION_DAYS} days"
log_info "  - Source: ${BACKUP_SOURCE}"
log_info "  - Destination: ${BACKUP_DEST}"

# Ensure backup directory exists
mkdir -p "${BACKUP_DEST}"

# Wait for database to be created on first run
log_info "Waiting for database to be available..."
while [ ! -f "${DB_FILE}" ]; do
    sleep 5
done
log_info "Database detected, starting backup cycle"

# Main backup loop
backup_count=0
while true; do
    log_info "=== Backup Cycle #$((++backup_count)) ==="

    # Perform backup
    if perform_backup; then
        # Clean up old backups after successful backup
        cleanup_old_backups

        # Check disk usage
        check_disk_usage
    else
        log_error "Backup failed, will retry on next cycle"
    fi

    # Wait for next backup interval
    log_info "Next backup in ${BACKUP_INTERVAL} seconds ($((BACKUP_INTERVAL / 60)) minutes)"
    sleep ${BACKUP_INTERVAL}
done
