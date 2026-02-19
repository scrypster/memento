package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	_ "modernc.org/sqlite" // SQLite driver

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// RunMigrations applies all pending database migrations from the given directory.
// This is the recommended way to initialize the schema when using migrations
// instead of the embedded Schema constant.
func (s *MemoryStore) RunMigrations(migrationsDir string) error {
	mgr, err := storage.NewMigrationManager(s.db, migrationsDir)
	if err != nil {
		return fmt.Errorf("sqlite: failed to create migration manager: %w", err)
	}
	defer mgr.Close()

	if err := mgr.Up(); err != nil {
		return fmt.Errorf("sqlite: failed to run migrations: %w", err)
	}

	return nil
}

// MemoryStore implements storage.MemoryStore using SQLite.
type MemoryStore struct {
	db *sql.DB
}

// NewMemoryStore creates a new SQLite memory store with WAL self-healing.
// If the initial open fails due to stale WAL files (left behind by a crashed
// process), it verifies no other process holds them and retries once after
// removing the stale -shm/-wal files.
func NewMemoryStore(dsn string) (*MemoryStore, error) {
	store, err := openMemoryStore(dsn)
	if err == nil {
		return store, nil
	}

	if !isRecoverableWALError(err) {
		return nil, err
	}

	dbPath := dbPathFromDSN(dsn)
	if dbPath == "" || dbPath == ":memory:" {
		return nil, err
	}

	if !isWALStale(dbPath) {
		return nil, err
	}

	removeStaleWAL(dbPath)

	store, retryErr := openMemoryStore(dsn)
	if retryErr != nil {
		return nil, fmt.Errorf("failed after WAL recovery: %w (original: %v)", retryErr, err)
	}

	log.Printf("sqlite: recovered from stale WAL files for %s", dbPath)
	return store, nil
}

// openMemoryStore opens a SQLite database, configures WAL mode, and creates the schema.
func openMemoryStore(dsn string) (*MemoryStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// SQLite only supports one concurrent writer. Using a single open connection
	// serialises writes and avoids SQLITE_BUSY errors under concurrent load.
	// WAL mode allows concurrent readers to proceed without blocking the writer.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0) // Connections live for the lifetime of the store.

	// Enable WAL mode for better read concurrency (readers don't block writers).
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Set busy timeout so that callers wait instead of getting an immediate
	// SQLITE_BUSY error when the connection is held by another goroutine.
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Create schema
	if _, err := db.Exec(Schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return &MemoryStore{db: db}, nil
}

// maxSourceContextBytes is the maximum allowed serialized size of SourceContext (Opus Issue #9).
const maxSourceContextBytes = 4096

// Store creates or updates a memory (upsert semantics).
func (s *MemoryStore) Store(ctx context.Context, memory *types.Memory) error {
	if memory == nil {
		return storage.ErrInvalidInput
	}

	if memory.ID == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	if memory.Content == "" {
		return fmt.Errorf("%w: memory content is required", storage.ErrInvalidInput)
	}

	// Compute and store content hash (used for dedup at the MCP layer via
	// deterministic ID generation; stored here for analytics/querying).
	memory.ContentHash = fmt.Sprintf("%x", sha256.Sum256([]byte(memory.Content)))

	// Marshal metadata, tags, and key_points to JSON
	var (
		metadataJSON, tagsJSON, keyPointsJSON []byte
		err                                   error
	)

	if memory.Metadata != nil {
		metadataJSON, err = json.Marshal(memory.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	if memory.Tags != nil && len(memory.Tags) > 0 {
		tagsJSON, err = json.Marshal(memory.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags: %w", err)
		}
	}

	if memory.Keywords != nil && len(memory.Keywords) > 0 {
		keyPointsJSON, err = json.Marshal(memory.Keywords)
		if err != nil {
			return fmt.Errorf("failed to marshal keywords: %w", err)
		}
	}

	// Marshal source_context to JSON and validate size (Opus Issue #9).
	var sourceContextJSON []byte
	if memory.SourceContext != nil {
		sourceContextJSON, err = json.Marshal(memory.SourceContext)
		if err != nil {
			return fmt.Errorf("failed to marshal source_context: %w", err)
		}
		if len(sourceContextJSON) > maxSourceContextBytes {
			return fmt.Errorf("source_context exceeds maximum allowed size of %d bytes (got %d bytes)",
				maxSourceContextBytes, len(sourceContextJSON))
		}
	}

	// Set default timestamps if not provided
	if memory.CreatedAt.IsZero() {
		memory.CreatedAt = time.Now()
	}
	if memory.UpdatedAt.IsZero() {
		memory.UpdatedAt = time.Now()
	}

	// Set default status if not provided
	if memory.Status == "" {
		memory.Status = types.StatusPending
	}

	// Set default enrichment statuses
	if memory.EntityStatus == "" {
		memory.EntityStatus = types.EnrichmentPending
	}
	if memory.RelationshipStatus == "" {
		memory.RelationshipStatus = types.EnrichmentPending
	}
	if memory.ClassificationStatus == "" {
		memory.ClassificationStatus = types.EnrichmentPending
	}
	if memory.SummarizationStatus == "" {
		memory.SummarizationStatus = types.EnrichmentPending
	}
	if memory.EmbeddingStatus == "" {
		memory.EmbeddingStatus = types.EnrichmentPending
	}

	// Upsert the memory
	query := `
		INSERT INTO memories (
			id, content, source, domain, timestamp, status,
			entity_status, relationship_status, embedding_status,
			enrichment_attempts, enrichment_error,
			created_at, updated_at, enriched_at,
			metadata, tags,
			summary, key_points,
			classification_status, summarization_status,
			state, state_updated_at,
			created_by, session_id, source_context,
			access_count, last_accessed_at, decay_score, decay_updated_at, deleted_at, content_hash, supersedes_id,
			memory_type
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content = excluded.content,
			source = excluded.source,
			domain = excluded.domain,
			timestamp = excluded.timestamp,
			status = excluded.status,
			entity_status = excluded.entity_status,
			relationship_status = excluded.relationship_status,
			embedding_status = excluded.embedding_status,
			enrichment_attempts = excluded.enrichment_attempts,
			enrichment_error = excluded.enrichment_error,
			updated_at = excluded.updated_at,
			enriched_at = excluded.enriched_at,
			metadata = excluded.metadata,
			tags = excluded.tags,
			summary = excluded.summary,
			key_points = excluded.key_points,
			classification_status = excluded.classification_status,
			summarization_status = excluded.summarization_status,
			state = excluded.state,
			state_updated_at = excluded.state_updated_at,
			created_by = excluded.created_by,
			session_id = excluded.session_id,
			source_context = excluded.source_context,
			access_count = excluded.access_count,
			last_accessed_at = excluded.last_accessed_at,
			decay_score = excluded.decay_score,
			decay_updated_at = excluded.decay_updated_at,
			deleted_at = excluded.deleted_at,
			content_hash = excluded.content_hash,
			supersedes_id = excluded.supersedes_id,
			memory_type = excluded.memory_type
	`

	_, err = s.db.ExecContext(ctx, query,
		memory.ID,
		memory.Content,
		memory.Source,
		memory.Domain,
		nullableTime(&memory.Timestamp),
		memory.Status,
		memory.EntityStatus,
		memory.RelationshipStatus,
		memory.EmbeddingStatus,
		memory.EnrichmentAttempts,
		memory.EnrichmentError,
		memory.CreatedAt,
		memory.UpdatedAt,
		nullableTime(memory.EnrichedAt),
		nullableBytes(metadataJSON),
		nullableBytes(tagsJSON),
		nullableString(memory.Summary),
		nullableBytes(keyPointsJSON),
		memory.ClassificationStatus,
		memory.SummarizationStatus,
		nullableString(memory.State),
		nullableTime(memory.StateUpdatedAt),
		nullableString(memory.CreatedBy),
		nullableString(memory.SessionID),
		nullableBytes(sourceContextJSON),
		memory.AccessCount,
		nullableTime(memory.LastAccessedAt),
		memory.DecayScore,
		nullableTime(memory.DecayUpdatedAt),
		nullableTime(memory.DeletedAt),
		nullableString(memory.ContentHash),
		nullableString(memory.SupersedesID),
		nullableString(memory.MemoryType),
	)

	if err != nil {
		return fmt.Errorf("failed to store memory: %w", err)
	}

	return nil
}

// Get retrieves a memory by ID.
func (s *MemoryStore) Get(ctx context.Context, id string) (*types.Memory, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	query := `
		SELECT
			id, content, source, domain, timestamp, status,
			entity_status, relationship_status, embedding_status,
			enrichment_attempts, enrichment_error,
			created_at, updated_at, enriched_at,
			metadata, tags,
			summary, key_points,
			classification_status, summarization_status,
			state, state_updated_at,
			created_by, session_id, source_context,
			access_count, last_accessed_at, decay_score, decay_updated_at, deleted_at, content_hash, supersedes_id,
			memory_type
		FROM memories
		WHERE id = ? AND deleted_at IS NULL
	`

	var memory types.Memory
	var metadataJSON, tagsJSON, keyPointsJSON sql.NullString
	var enrichedAt, timestamp sql.NullTime
	var domain sql.NullString

	// Nullable fields for new columns
	var state, createdBy, sessionID, enrichmentError, summary, contentHash, supersedesID sql.NullString
	var memoryType sql.NullString
	var sourceContextJSON sql.NullString
	var stateUpdatedAt, lastAccessedAt, decayUpdatedAt, deletedAt sql.NullTime
	var classificationStatus, summarizationStatus sql.NullString

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&memory.ID,
		&memory.Content,
		&memory.Source,
		&domain,
		&timestamp,
		&memory.Status,
		&memory.EntityStatus,
		&memory.RelationshipStatus,
		&memory.EmbeddingStatus,
		&memory.EnrichmentAttempts,
		&enrichmentError,
		&memory.CreatedAt,
		&memory.UpdatedAt,
		&enrichedAt,
		&metadataJSON,
		&tagsJSON,
		&summary,
		&keyPointsJSON,
		&classificationStatus,
		&summarizationStatus,
		&state,
		&stateUpdatedAt,
		&createdBy,
		&sessionID,
		&sourceContextJSON,
		&memory.AccessCount,
		&lastAccessedAt,
		&memory.DecayScore,
		&decayUpdatedAt,
		&deletedAt,
		&contentHash,
		&supersedesID,
		&memoryType,
	)

	if err == sql.ErrNoRows {
		return nil, storage.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get memory: %w", err)
	}

	// Unmarshal JSON fields
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &memory.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	if tagsJSON.Valid && tagsJSON.String != "" {
		if err := json.Unmarshal([]byte(tagsJSON.String), &memory.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}

	if keyPointsJSON.Valid && keyPointsJSON.String != "" {
		if err := json.Unmarshal([]byte(keyPointsJSON.String), &memory.Keywords); err != nil {
			return nil, fmt.Errorf("failed to unmarshal keywords: %w", err)
		}
	}

	if sourceContextJSON.Valid && sourceContextJSON.String != "" {
		if err := json.Unmarshal([]byte(sourceContextJSON.String), &memory.SourceContext); err != nil {
			return nil, fmt.Errorf("failed to unmarshal source_context: %w", err)
		}
	}

	if enrichedAt.Valid {
		memory.EnrichedAt = &enrichedAt.Time
	}

	if domain.Valid {
		memory.Domain = domain.String
	}

	if timestamp.Valid {
		memory.Timestamp = timestamp.Time
	}

	// Enrichment error
	if enrichmentError.Valid {
		memory.EnrichmentError = enrichmentError.String
	}

	// Summary
	if summary.Valid {
		memory.Summary = summary.String
	}

	// Classification and summarization status
	if classificationStatus.Valid {
		memory.ClassificationStatus = types.EnrichmentStatus(classificationStatus.String)
	}
	if summarizationStatus.Valid {
		memory.SummarizationStatus = types.EnrichmentStatus(summarizationStatus.String)
	}

	// Lifecycle state
	if state.Valid {
		memory.State = state.String
	}
	if stateUpdatedAt.Valid {
		t := stateUpdatedAt.Time
		memory.StateUpdatedAt = &t
	}

	// Provenance
	if createdBy.Valid {
		memory.CreatedBy = createdBy.String
	}
	if sessionID.Valid {
		memory.SessionID = sessionID.String
	}

	// Quality signals
	if lastAccessedAt.Valid {
		t := lastAccessedAt.Time
		memory.LastAccessedAt = &t
	}
	if decayUpdatedAt.Valid {
		t := decayUpdatedAt.Time
		memory.DecayUpdatedAt = &t
	}

	// Soft delete
	if deletedAt.Valid {
		t := deletedAt.Time
		memory.DeletedAt = &t
	}

	// Content hash
	if contentHash.Valid {
		memory.ContentHash = contentHash.String
	}

	// Evolution chain (supersedes)
	if supersedesID.Valid {
		memory.SupersedesID = supersedesID.String
	}

	// Memory type classification
	if memoryType.Valid {
		memory.MemoryType = memoryType.String
	}

	return &memory, nil
}

// List retrieves memories with pagination and filtering.
func (s *MemoryStore) List(ctx context.Context, opts storage.ListOptions) (*storage.PaginatedResult[types.Memory], error) {
	// Normalize options (must be done before ORDER BY construction to prevent SQL injection)
	opts.Normalize()

	// Build query with filtering
	query := `
		SELECT
			id, content, source, domain, timestamp, status,
			entity_status, relationship_status, embedding_status,
			enrichment_attempts, enrichment_error,
			created_at, updated_at, enriched_at,
			metadata, tags,
			summary, key_points,
			classification_status, summarization_status,
			state, state_updated_at,
			created_by, session_id, source_context,
			access_count, last_accessed_at, decay_score, decay_updated_at, deleted_at, content_hash, supersedes_id,
			memory_type
		FROM memories
	`

	// Build WHERE clause from typed filter fields and legacy map.
	var conditions []string
	var args []interface{}

	// Legacy map-based filter (backward compat — status only).
	if statusFilter, ok := opts.Filter["status"]; ok {
		var statusStr string
		switch v := statusFilter.(type) {
		case string:
			statusStr = v
		case types.MemoryStatus:
			statusStr = string(v)
		}
		if statusStr != "" {
			conditions = append(conditions, "status = ?")
			args = append(args, statusStr)
		}
	}

	// Typed filter fields (Opus Issue #5).
	if opts.State != "" {
		conditions = append(conditions, "state = ?")
		args = append(args, opts.State)
	}

	if opts.CreatedBy != "" {
		conditions = append(conditions, "created_by = ?")
		args = append(args, opts.CreatedBy)
	}

	if !opts.CreatedAfter.IsZero() {
		conditions = append(conditions, "created_at > ?")
		args = append(args, opts.CreatedAfter)
	}

	if !opts.CreatedBefore.IsZero() {
		conditions = append(conditions, "created_at < ?")
		args = append(args, opts.CreatedBefore)
	}

	if opts.MinDecayScore > 0 {
		conditions = append(conditions, "decay_score >= ?")
		args = append(args, opts.MinDecayScore)
	}

	if opts.SessionID != "" {
		conditions = append(conditions, "session_id = ?")
		args = append(args, opts.SessionID)
	}

	// Exclude soft-deleted memories unless explicitly requested
	if !opts.IncludeDeleted {
		conditions = append(conditions, "deleted_at IS NULL")
	}

	// When OnlyDeleted is set, restrict to soft-deleted rows only.
	if opts.OnlyDeleted {
		conditions = append(conditions, "deleted_at IS NOT NULL")
	}

	// Filter by memory_type when set.
	if opts.MemoryType != "" {
		conditions = append(conditions, "memory_type = ?")
		args = append(args, opts.MemoryType)
	}

	var whereClause string
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	query += whereClause

	// Add sorting (safe from SQL injection due to Normalize() whitelist validation above)
	query += fmt.Sprintf(" ORDER BY %s %s", opts.SortBy, opts.SortOrder)

	// Add pagination
	query += " LIMIT ? OFFSET ?"
	args = append(args, opts.Limit, opts.Offset())

	// Execute query
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}
	defer rows.Close()

	// Scan results
	var memories []types.Memory

	for rows.Next() {
		var memory types.Memory
		var metadataJSON, tagsJSON, keyPointsJSON sql.NullString
		var enrichedAt, timestamp sql.NullTime
		var domain sql.NullString

		// Nullable fields for new columns
		var state, createdBy, sessionID, enrichmentError, summary, contentHash, supersedesID sql.NullString
		var memTypeNull sql.NullString
		var sourceContextJSON sql.NullString
		var stateUpdatedAt, lastAccessedAt, decayUpdatedAt, deletedAt sql.NullTime
		var classificationStatus, summarizationStatus sql.NullString

		err := rows.Scan(
			&memory.ID,
			&memory.Content,
			&memory.Source,
			&domain,
			&timestamp,
			&memory.Status,
			&memory.EntityStatus,
			&memory.RelationshipStatus,
			&memory.EmbeddingStatus,
			&memory.EnrichmentAttempts,
			&enrichmentError,
			&memory.CreatedAt,
			&memory.UpdatedAt,
			&enrichedAt,
			&metadataJSON,
			&tagsJSON,
			&summary,
			&keyPointsJSON,
			&classificationStatus,
			&summarizationStatus,
			&state,
			&stateUpdatedAt,
			&createdBy,
			&sessionID,
			&sourceContextJSON,
			&memory.AccessCount,
			&lastAccessedAt,
			&memory.DecayScore,
			&decayUpdatedAt,
			&deletedAt,
			&contentHash,
			&supersedesID,
			&memTypeNull,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan memory: %w", err)
		}

		// Unmarshal JSON fields
		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &memory.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		if tagsJSON.Valid && tagsJSON.String != "" {
			if err := json.Unmarshal([]byte(tagsJSON.String), &memory.Tags); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
			}
		}

		if keyPointsJSON.Valid && keyPointsJSON.String != "" {
			if err := json.Unmarshal([]byte(keyPointsJSON.String), &memory.Keywords); err != nil {
				return nil, fmt.Errorf("failed to unmarshal keywords: %w", err)
			}
		}

		if sourceContextJSON.Valid && sourceContextJSON.String != "" {
			if err := json.Unmarshal([]byte(sourceContextJSON.String), &memory.SourceContext); err != nil {
				return nil, fmt.Errorf("failed to unmarshal source_context: %w", err)
			}
		}

		if enrichedAt.Valid {
			memory.EnrichedAt = &enrichedAt.Time
		}

		if domain.Valid {
			memory.Domain = domain.String
		}

		if timestamp.Valid {
			memory.Timestamp = timestamp.Time
		}

		// Summary
		if summary.Valid {
			memory.Summary = summary.String
		}

		// Classification and summarization status
		if classificationStatus.Valid {
			memory.ClassificationStatus = types.EnrichmentStatus(classificationStatus.String)
		}
		if summarizationStatus.Valid {
			memory.SummarizationStatus = types.EnrichmentStatus(summarizationStatus.String)
		}

		// Lifecycle state
		if state.Valid {
			memory.State = state.String
		}
		if stateUpdatedAt.Valid {
			t := stateUpdatedAt.Time
			memory.StateUpdatedAt = &t
		}

		// Provenance
		if createdBy.Valid {
			memory.CreatedBy = createdBy.String
		}
		if sessionID.Valid {
			memory.SessionID = sessionID.String
		}

		// Quality signals
		if lastAccessedAt.Valid {
			t := lastAccessedAt.Time
			memory.LastAccessedAt = &t
		}
		if decayUpdatedAt.Valid {
			t := decayUpdatedAt.Time
			memory.DecayUpdatedAt = &t
		}

		// Soft delete
		if deletedAt.Valid {
			t := deletedAt.Time
			memory.DeletedAt = &t
		}

		// Content hash
		if contentHash.Valid {
			memory.ContentHash = contentHash.String
		}

		// Evolution chain (supersedes)
		if supersedesID.Valid {
			memory.SupersedesID = supersedesID.String
		}

		// Memory type classification
		if memTypeNull.Valid {
			memory.MemoryType = memTypeNull.String
		}

		memories = append(memories, memory)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating memories: %w", err)
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM memories" + whereClause
	var total int
	err = s.db.QueryRowContext(ctx, countQuery, args[:len(args)-2]...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("failed to count memories: %w", err)
	}

	// Build paginated result
	result := &storage.PaginatedResult[types.Memory]{
		Items:    memories,
		Total:    total,
		Page:     opts.Page,
		PageSize: opts.Limit,
		HasMore:  opts.Offset()+len(memories) < total,
	}

	return result, nil
}

// Update modifies an existing memory.
func (s *MemoryStore) Update(ctx context.Context, memory *types.Memory) error {
	if memory == nil {
		return storage.ErrInvalidInput
	}

	if memory.ID == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	// Check if memory exists
	exists, err := s.exists(ctx, memory.ID)
	if err != nil {
		return err
	}

	if !exists {
		return storage.ErrNotFound
	}

	// Update the memory using Store (upsert semantics)
	memory.UpdatedAt = time.Now()
	return s.Store(ctx, memory)
}

// Delete removes a memory by ID.
func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	// Soft delete: set deleted_at timestamp
	result, err := s.db.ExecContext(ctx, "UPDATE memories SET deleted_at = CURRENT_TIMESTAMP WHERE id = ? AND deleted_at IS NULL", id)
	if err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return storage.ErrNotFound
	}

	return nil
}

// Purge hard-deletes a memory by ID (permanent removal).
func (s *MemoryStore) Purge(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	result, err := s.db.ExecContext(ctx, "DELETE FROM memories WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to purge memory: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return storage.ErrNotFound
	}

	return nil
}

// UpdateStatus updates the processing status of a memory.
func (s *MemoryStore) UpdateStatus(ctx context.Context, id string, status types.MemoryStatus) error {
	if id == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	query := "UPDATE memories SET status = ?, updated_at = ? WHERE id = ?"
	result, err := s.db.ExecContext(ctx, query, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return storage.ErrNotFound
	}

	return nil
}

// UpdateEnrichment updates enrichment metadata for a memory.
func (s *MemoryStore) UpdateEnrichment(ctx context.Context, id string, enrichment storage.EnrichmentUpdate) error {
	if id == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	query := `
		UPDATE memories
		SET
			entity_status = ?,
			relationship_status = ?,
			embedding_status = ?,
			enrichment_attempts = ?,
			enrichment_error = ?,
			enriched_at = ?,
			updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query,
		enrichment.EntityStatus,
		enrichment.RelationshipStatus,
		enrichment.EmbeddingStatus,
		enrichment.EnrichmentAttempts,
		enrichment.EnrichmentError,
		nullableTime(enrichment.EnrichedAt),
		time.Now(),
		id,
	)

	if err != nil {
		return fmt.Errorf("failed to update enrichment: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return storage.ErrNotFound
	}

	return nil
}

// UpdateState updates the lifecycle state of a memory with state transition validation (Opus Issue #6).
func (s *MemoryStore) UpdateState(ctx context.Context, id string, state string) error {
	if id == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	if state == "" {
		return fmt.Errorf("%w: state is required", storage.ErrInvalidInput)
	}

	if !types.IsValidLifecycleState(state) {
		return fmt.Errorf("%w: invalid state: %s", storage.ErrInvalidInput, state)
	}

	// Get current state to validate transition
	currentMem, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	// Validate state transition
	if !types.IsValidStateTransition(currentMem.State, state) {
		return fmt.Errorf("invalid state transition: cannot transition from '%s' to '%s'", currentMem.State, state)
	}

	now := time.Now()
	query := `
		UPDATE memories
		SET
			state = ?,
			state_updated_at = ?,
			updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query, state, now, now, id)
	if err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return storage.ErrNotFound
	}

	return nil
}

// GetDB returns the underlying database connection.
// This is used for direct database operations like config persistence.
func (s *MemoryStore) GetDB() *sql.DB {
	return s.db
}

// IncrementAccessCount atomically increments access_count and sets
// last_accessed_at to the current UTC time for the given memory ID.
// Returns ErrNotFound if the memory does not exist.
func (s *MemoryStore) IncrementAccessCount(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	query := `
		UPDATE memories
		SET access_count = access_count + 1,
		    last_accessed_at = ?,
		    decay_score = MIN(decay_score + 0.1, 1.0)
		WHERE id = ? AND deleted_at IS NULL
	`

	result, err := s.db.ExecContext(ctx, query, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("sqlite: failed to increment access count: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return storage.ErrNotFound
	}

	return nil
}

// UpdateDecayScores applies time-based decay to all active memories.
// This should be called periodically (e.g., daily). Returns count of updated rows.
// Uses a simple linear approximation: factor = 1/(1 + daysSince/halfLife)
// At 60 days: factor ≈ 0.5 (half). At 120 days: factor ≈ 0.33.
func (s *MemoryStore) UpdateDecayScores(ctx context.Context) (int, error) {
	query := `
		UPDATE memories
		SET decay_score = MAX(0.0,
			decay_score * CASE
				WHEN (julianday('now') - julianday(COALESCE(last_accessed_at, created_at))) > 0
				THEN (1.0 / (1.0 + (julianday('now') - julianday(COALESCE(last_accessed_at, created_at))) / 60.0))
				ELSE 1.0
			END
		),
		decay_updated_at = CURRENT_TIMESTAMP
		WHERE deleted_at IS NULL
		  AND (state IS NULL OR state = 'active')
	`

	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("sqlite: failed to update decay scores: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite: failed to get rows affected: %w", err)
	}

	return int(n), nil
}

// Close flushes the WAL into the main database file and releases resources.
// The TRUNCATE checkpoint removes the -shm and -wal files so that other
// processes (e.g., memento-mcp after memento-web exits) can open the database
// without encountering stale WAL state.
func (s *MemoryStore) Close() error {
	if s.db == nil {
		return nil
	}

	if _, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		log.Printf("sqlite: WAL checkpoint on close failed (non-fatal): %v", err)
	}

	return s.db.Close()
}

// GetRelatedMemories returns the IDs of memories that share at least one
// entity with the given memory. This provides 1-hop graph traversal support
// for the GraphTraversal engine.
func (s *MemoryStore) GetRelatedMemories(ctx context.Context, memoryID string) ([]string, error) {
	query := `
		SELECT DISTINCT me2.memory_id
		FROM memory_entities me1
		JOIN memory_entities me2 ON me1.entity_id = me2.entity_id
		WHERE me1.memory_id = ?
		  AND me2.memory_id != ?
	`
	rows, err := s.db.QueryContext(ctx, query, memoryID, memoryID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: GetRelatedMemories: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sqlite: GetRelatedMemories scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: GetRelatedMemories rows: %w", err)
	}
	return ids, nil
}

// Restore un-deletes a soft-deleted memory by clearing its deleted_at timestamp.
func (s *MemoryStore) Restore(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	result, err := s.db.ExecContext(ctx,
		"UPDATE memories SET deleted_at = NULL, updated_at = ? WHERE id = ? AND deleted_at IS NOT NULL",
		time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("sqlite: failed to restore memory: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return storage.ErrNotFound
	}

	return nil
}

// GetEvolutionChain returns the full version history for a memory,
// ordered oldest → newest. It walks backward via supersedes_id and forward
// via reverse lookup. Capped at 50 hops to prevent loops.
func (s *MemoryStore) GetEvolutionChain(ctx context.Context, memoryID string) ([]*types.Memory, error) {
	if memoryID == "" {
		return nil, fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	const maxChain = 50

	// Fetch a memory by ID, ignoring soft-delete filter so we can see superseded versions.
	fetchByID := func(id string) (*types.Memory, error) {
		query := `
			SELECT
				id, content, source, domain, timestamp, status,
				entity_status, relationship_status, embedding_status,
				enrichment_attempts, enrichment_error,
				created_at, updated_at, enriched_at,
				metadata, tags,
				summary, key_points,
				classification_status, summarization_status,
				state, state_updated_at,
				created_by, session_id, source_context,
				access_count, last_accessed_at, decay_score, decay_updated_at, deleted_at, content_hash, supersedes_id
			FROM memories WHERE id = ?`

		var m types.Memory
		var metadataJSON, tagsJSON, keyPointsJSON sql.NullString
		var enrichedAt, timestamp sql.NullTime
		var domain sql.NullString
		var state, createdBy, sessionID, enrichmentError, summary, contentHash, supersedesID sql.NullString
		var sourceContextJSON sql.NullString
		var stateUpdatedAt, lastAccessedAt, decayUpdatedAt, deletedAt sql.NullTime
		var classificationStatus, summarizationStatus sql.NullString

		err := s.db.QueryRowContext(ctx, query, id).Scan(
			&m.ID, &m.Content, &m.Source, &domain, &timestamp, &m.Status,
			&m.EntityStatus, &m.RelationshipStatus, &m.EmbeddingStatus,
			&m.EnrichmentAttempts, &enrichmentError,
			&m.CreatedAt, &m.UpdatedAt, &enrichedAt,
			&metadataJSON, &tagsJSON,
			&summary, &keyPointsJSON,
			&classificationStatus, &summarizationStatus,
			&state, &stateUpdatedAt,
			&createdBy, &sessionID, &sourceContextJSON,
			&m.AccessCount, &lastAccessedAt, &m.DecayScore, &decayUpdatedAt, &deletedAt, &contentHash, &supersedesID,
		)
		if err == sql.ErrNoRows {
			return nil, storage.ErrNotFound
		}
		if err != nil {
			return nil, err
		}

		if domain.Valid {
			m.Domain = domain.String
		}
		if state.Valid {
			m.State = state.String
		}
		if supersedesID.Valid {
			m.SupersedesID = supersedesID.String
		}
		if contentHash.Valid {
			m.ContentHash = contentHash.String
		}
		if summary.Valid {
			m.Summary = summary.String
		}
		if createdBy.Valid {
			m.CreatedBy = createdBy.String
		}
		if sessionID.Valid {
			m.SessionID = sessionID.String
		}
		if enrichmentError.Valid {
			m.EnrichmentError = enrichmentError.String
		}
		if timestamp.Valid {
			m.Timestamp = timestamp.Time
		}
		if enrichedAt.Valid {
			m.EnrichedAt = &enrichedAt.Time
		}
		if stateUpdatedAt.Valid {
			t := stateUpdatedAt.Time
			m.StateUpdatedAt = &t
		}
		if lastAccessedAt.Valid {
			t := lastAccessedAt.Time
			m.LastAccessedAt = &t
		}
		if decayUpdatedAt.Valid {
			t := decayUpdatedAt.Time
			m.DecayUpdatedAt = &t
		}
		if deletedAt.Valid {
			t := deletedAt.Time
			m.DeletedAt = &t
		}
		if classificationStatus.Valid {
			m.ClassificationStatus = types.EnrichmentStatus(classificationStatus.String)
		}
		if summarizationStatus.Valid {
			m.SummarizationStatus = types.EnrichmentStatus(summarizationStatus.String)
		}
		if metadataJSON.Valid && metadataJSON.String != "" {
			_ = json.Unmarshal([]byte(metadataJSON.String), &m.Metadata)
		}
		if tagsJSON.Valid && tagsJSON.String != "" {
			_ = json.Unmarshal([]byte(tagsJSON.String), &m.Tags)
		}
		return &m, nil
	}

	// Walk backward to find the oldest ancestor.
	current, err := fetchByID(memoryID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: GetEvolutionChain: %w", err)
	}

	// Build the chain: start from current, walk back to root.
	var chain []*types.Memory
	visited := map[string]bool{current.ID: true}
	node := current

	for len(chain) < maxChain {
		if node.SupersedesID == "" {
			break
		}
		if visited[node.SupersedesID] {
			break // cycle guard
		}
		parent, err := fetchByID(node.SupersedesID)
		if err != nil {
			break // ancestor may have been purged
		}
		visited[parent.ID] = true
		chain = append([]*types.Memory{parent}, chain...) // prepend
		node = parent
	}

	// Append the starting memory.
	chain = append(chain, current)

	// Walk forward: find memories that supersede any node in the chain.
	tip := chain[len(chain)-1]
	for len(chain) < maxChain {
		rows, err := s.db.QueryContext(ctx,
			`SELECT id FROM memories WHERE supersedes_id = ? LIMIT 1`, tip.ID)
		if err != nil {
			break
		}
		var nextID string
		if rows.Next() {
			_ = rows.Scan(&nextID)
		}
		rows.Close()

		if nextID == "" || visited[nextID] {
			break
		}
		next, err := fetchByID(nextID)
		if err != nil {
			break
		}
		visited[nextID] = true
		chain = append(chain, next)
		tip = next
	}

	return chain, nil
}

// CreateMemoryLink creates a typed link between two memories in the memory_links table.
func (s *MemoryStore) CreateMemoryLink(ctx context.Context, id, sourceID, targetID, linkType string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO memory_links (id, source_id, target_id, type) VALUES (?, ?, ?, ?)`,
		id, sourceID, targetID, linkType,
	)
	if err != nil {
		return fmt.Errorf("sqlite: CreateMemoryLink: %w", err)
	}
	return nil
}

// GetMemoriesByRelationType returns memories connected to memoryID via
// memory_links of the given type (e.g. "CONTAINS").
func (s *MemoryStore) GetMemoriesByRelationType(ctx context.Context, memoryID string, relType string) ([]*types.Memory, error) {
	if memoryID == "" {
		return nil, fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}
	if relType == "" {
		return nil, fmt.Errorf("%w: relation type is required", storage.ErrInvalidInput)
	}

	query := `
		SELECT DISTINCT m.id
		FROM memory_links ml
		JOIN memories m ON m.id = ml.target_id
		WHERE ml.source_id = ? AND ml.type = ? AND m.deleted_at IS NULL
	`
	rows, err := s.db.QueryContext(ctx, query, memoryID, relType)
	if err != nil {
		return nil, fmt.Errorf("sqlite: GetMemoriesByRelationType: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sqlite: GetMemoriesByRelationType scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: GetMemoriesByRelationType rows: %w", err)
	}

	var memories []*types.Memory
	for _, id := range ids {
		m, err := s.Get(ctx, id)
		if err != nil {
			continue // skip if not found (e.g. deleted between queries)
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// exists checks if a memory with the given ID exists.
func (s *MemoryStore) exists(ctx context.Context, id string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memories WHERE id = ?", id).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}
	return count > 0, nil
}

// nullableTime converts a time pointer to sql.NullTime.
func nullableTime(t *time.Time) sql.NullTime {
	if t == nil || t.IsZero() {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// nullableBytes converts a byte slice to sql.NullString.
func nullableBytes(b []byte) sql.NullString {
	if b == nil || len(b) == 0 {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: string(b), Valid: true}
}

// nullableString converts a string to sql.NullString.
// An empty string is treated as NULL.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// dbPathFromDSN extracts the filesystem path from a SQLite DSN.
// Handles bare paths ("/path/to/db.sqlite") and file: URIs ("file:/path/to/db.sqlite?mode=rwc").
// Returns empty string for in-memory databases or unparseable DSNs.
func dbPathFromDSN(dsn string) string {
	if dsn == ":memory:" || dsn == "" {
		return ""
	}

	if strings.HasPrefix(dsn, "file:") {
		u, err := url.Parse(dsn)
		if err != nil {
			return ""
		}
		path := u.Path
		if path == "" {
			path = u.Opaque
		}
		if path == ":memory:" || path == "" {
			return ""
		}
		return path
	}

	return dsn
}

// isRecoverableWALError returns true if the error matches patterns caused by
// stale WAL files left behind after a crash (SIGKILL, OOM, etc.).
func isRecoverableWALError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "disk I/O error") ||
		strings.Contains(msg, "database is locked")
}

// isWALStale checks whether -shm/-wal files exist for the given database path
// AND no other process currently holds them open (via lsof).
// Returns false if lsof is unavailable (conservative: no deletion).
func isWALStale(dbPath string) bool {
	shmPath := dbPath + "-shm"
	walPath := dbPath + "-wal"

	if !fileExists(shmPath) && !fileExists(walPath) {
		return false
	}

	// Check if any process has the database or WAL files open.
	lsofPath, err := exec.LookPath("lsof")
	if err != nil {
		// lsof not available (e.g., Alpine Docker) — conservative fallback.
		return false
	}

	// Check the main db file, -shm, and -wal in a single lsof invocation.
	cmd := exec.Command(lsofPath, "-t", dbPath, shmPath, walPath)
	output, err := cmd.Output()
	if err != nil {
		// lsof returns exit code 1 when no files are open — that means stale.
		return true
	}

	// If lsof produced output, some process has these files open — not stale.
	return strings.TrimSpace(string(output)) == ""
}

// removeStaleWAL removes -shm and -wal files for the given database path.
func removeStaleWAL(dbPath string) {
	for _, suffix := range []string{"-shm", "-wal"} {
		path := dbPath + suffix
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("sqlite: failed to remove stale %s: %v", path, err)
		}
	}
}

// fileExists returns true if the path exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
