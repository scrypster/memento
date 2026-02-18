// Package postgres provides a PostgreSQL implementation of storage interfaces.
package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

const (
	// maxSourceContextBytes is the maximum allowed size for source_context JSON (4 KB).
	maxSourceContextBytes = 4 * 1024
)

// MemoryStore implements storage.MemoryStore using PostgreSQL.
type MemoryStore struct {
	db               *sql.DB
	pgvectorAvailable bool // true when the pgvector extension is present
}

// NewMemoryStore creates a new PostgreSQL memory store.
// The dsn parameter is the PostgreSQL connection string (e.g., "postgres://user:pass@host/db?sslmode=disable").
func NewMemoryStore(dsn string) (*MemoryStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres: failed to ping database: %w", err)
	}

	s := &MemoryStore{db: db}

	// Apply the base schema (idempotent — all statements use IF NOT EXISTS).
	if _, err := db.Exec(Schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres: failed to apply schema: %w", err)
	}

	// Try to enable the pgvector extension. This may fail on servers without
	// pgvector installed — log a warning but continue without vector support.
	if _, err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
		log.Printf("postgres: pgvector extension not available (vector search disabled): %v", err)
		s.pgvectorAvailable = false
	} else {
		s.pgvectorAvailable = true
	}

	// Apply FTS migration (idempotent).
	if _, err := db.Exec(MigrationFTS); err != nil {
		// FTS is important but not fatal — log and continue.
		log.Printf("postgres: failed to apply FTS migration (full-text search degraded): %v", err)
	}

	// Apply pgvector column migration only when the extension is available.
	if s.pgvectorAvailable {
		if _, err := db.Exec(MigrationPgvector); err != nil {
			log.Printf("postgres: failed to apply pgvector migration (vector search disabled): %v", err)
			s.pgvectorAvailable = false
		}
	}

	return s, nil
}

// GetDB returns the underlying database connection.
// This is used for direct database operations like config persistence.
func (s *MemoryStore) GetDB() *sql.DB {
	return s.db
}

// UpdateDecayScores applies time-based decay to all active memories.
// Uses a simple linear approximation: factor = 1/(1 + daysSince/halfLife)
// At 60 days: factor ~= 0.5 (half). At 120 days: factor ~= 0.33.
func (s *MemoryStore) UpdateDecayScores(ctx context.Context) (int, error) {
	query := `
		UPDATE memories
		SET decay_score = GREATEST(0.0,
			decay_score * CASE
				WHEN EXTRACT(EPOCH FROM (NOW() - COALESCE(last_accessed_at, created_at))) / 86400.0 > 0
				THEN (1.0 / (1.0 + EXTRACT(EPOCH FROM (NOW() - COALESCE(last_accessed_at, created_at))) / 86400.0 / 60.0))
				ELSE 1.0
			END
		),
		decay_updated_at = NOW()
		WHERE deleted_at IS NULL
		  AND (state IS NULL OR state = 'active')
	`

	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("postgres: failed to update decay scores: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("postgres: failed to get rows affected: %w", err)
	}

	return int(n), nil
}

// Close releases any resources held by the store.
func (s *MemoryStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

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

	// Marshal metadata and tags to JSON
	var metadataJSON, tagsJSON []byte
	var err error

	if memory.Metadata != nil {
		metadataJSON, err = json.Marshal(memory.Metadata)
		if err != nil {
			return fmt.Errorf("postgres: failed to marshal metadata: %w", err)
		}
	}

	if len(memory.Tags) > 0 {
		tagsJSON, err = json.Marshal(memory.Tags)
		if err != nil {
			return fmt.Errorf("postgres: failed to marshal tags: %w", err)
		}
	}

	// Marshal source_context to JSON and enforce 4 KB limit
	var sourceContextJSON []byte
	if memory.SourceContext != nil {
		sourceContextJSON, err = json.Marshal(memory.SourceContext)
		if err != nil {
			return fmt.Errorf("postgres: failed to marshal source_context: %w", err)
		}
		if len(sourceContextJSON) > maxSourceContextBytes {
			return fmt.Errorf("%w: source_context exceeds 4KB limit (%d bytes)", storage.ErrInvalidInput, len(sourceContextJSON))
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
	if memory.EmbeddingStatus == "" {
		memory.EmbeddingStatus = types.EnrichmentPending
	}

	// Upsert the memory using PostgreSQL ON CONFLICT syntax
	query := `
		INSERT INTO memories (
			id, content, source, domain, timestamp, status,
			entity_status, relationship_status, embedding_status,
			enrichment_attempts, enrichment_error,
			created_at, updated_at, enriched_at,
			metadata, tags,
			state, state_updated_at,
			created_by, session_id, source_context,
			access_count, last_accessed_at, decay_score, decay_updated_at,
			deleted_at, content_hash, supersedes_id, memory_type
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9,
			$10, $11,
			$12, $13, $14,
			$15, $16,
			$17, $18,
			$19, $20, $21,
			$22, $23, $24, $25,
			$26, $27, $28, $29
		)
		ON CONFLICT(id) DO UPDATE SET
			content = EXCLUDED.content,
			source = EXCLUDED.source,
			domain = EXCLUDED.domain,
			timestamp = EXCLUDED.timestamp,
			status = EXCLUDED.status,
			entity_status = EXCLUDED.entity_status,
			relationship_status = EXCLUDED.relationship_status,
			embedding_status = EXCLUDED.embedding_status,
			enrichment_attempts = EXCLUDED.enrichment_attempts,
			enrichment_error = EXCLUDED.enrichment_error,
			updated_at = EXCLUDED.updated_at,
			enriched_at = EXCLUDED.enriched_at,
			metadata = EXCLUDED.metadata,
			tags = EXCLUDED.tags,
			state = EXCLUDED.state,
			state_updated_at = EXCLUDED.state_updated_at,
			created_by = EXCLUDED.created_by,
			session_id = EXCLUDED.session_id,
			source_context = EXCLUDED.source_context,
			access_count = EXCLUDED.access_count,
			last_accessed_at = EXCLUDED.last_accessed_at,
			decay_score = EXCLUDED.decay_score,
			decay_updated_at = EXCLUDED.decay_updated_at,
			deleted_at = EXCLUDED.deleted_at,
			content_hash = EXCLUDED.content_hash,
			supersedes_id = EXCLUDED.supersedes_id,
			memory_type = EXCLUDED.memory_type
	`

	_, err = s.db.ExecContext(ctx, query,
		memory.ID,
		memory.Content,
		memory.Source,
		nullableString(memory.Domain),
		nullableTime(&memory.Timestamp),
		memory.Status,
		memory.EntityStatus,
		memory.RelationshipStatus,
		memory.EmbeddingStatus,
		memory.EnrichmentAttempts,
		nullableString(memory.EnrichmentError),
		memory.CreatedAt,
		memory.UpdatedAt,
		nullableTimePtr(memory.EnrichedAt),
		nullableBytes(metadataJSON),
		nullableBytes(tagsJSON),
		nullableString(memory.State),
		nullableTimePtr(memory.StateUpdatedAt),
		nullableString(memory.CreatedBy),
		nullableString(memory.SessionID),
		nullableBytes(sourceContextJSON),
		memory.AccessCount,
		nullableTimePtr(memory.LastAccessedAt),
		memory.DecayScore,
		nullableTimePtr(memory.DecayUpdatedAt),
		nullableTimePtr(memory.DeletedAt),
		nullableString(memory.ContentHash),
		nullableString(memory.SupersedesID),
		nullableString(memory.MemoryType),
	)

	if err != nil {
		return fmt.Errorf("postgres: failed to store memory: %w", err)
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
			state, state_updated_at,
			created_by, session_id, source_context,
			access_count, last_accessed_at, decay_score, decay_updated_at,
			deleted_at, content_hash, supersedes_id, memory_type
		FROM memories
		WHERE id = $1 AND deleted_at IS NULL
	`

	var memory types.Memory
	var metadataJSON, tagsJSON, sourceContextJSON sql.NullString
	var enrichedAt, timestamp, stateUpdatedAt, lastAccessedAt, decayUpdatedAt, deletedAt sql.NullTime
	var domain, enrichmentError, state, createdBy, sessionID sql.NullString
	var contentHash, supersedesID, memoryType sql.NullString

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
		return nil, fmt.Errorf("postgres: failed to get memory: %w", err)
	}

	// Unmarshal JSON fields
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &memory.Metadata); err != nil {
			return nil, fmt.Errorf("postgres: failed to unmarshal metadata: %w", err)
		}
	}

	if tagsJSON.Valid && tagsJSON.String != "" {
		if err := json.Unmarshal([]byte(tagsJSON.String), &memory.Tags); err != nil {
			return nil, fmt.Errorf("postgres: failed to unmarshal tags: %w", err)
		}
	}

	if sourceContextJSON.Valid && sourceContextJSON.String != "" {
		if err := json.Unmarshal([]byte(sourceContextJSON.String), &memory.SourceContext); err != nil {
			return nil, fmt.Errorf("postgres: failed to unmarshal source_context: %w", err)
		}
	}

	// Map nullable fields
	if domain.Valid {
		memory.Domain = domain.String
	}
	if enrichmentError.Valid {
		memory.EnrichmentError = enrichmentError.String
	}
	if timestamp.Valid {
		memory.Timestamp = timestamp.Time
	}
	if enrichedAt.Valid {
		memory.EnrichedAt = &enrichedAt.Time
	}
	if state.Valid {
		memory.State = state.String
	}
	if stateUpdatedAt.Valid {
		memory.StateUpdatedAt = &stateUpdatedAt.Time
	}
	if createdBy.Valid {
		memory.CreatedBy = createdBy.String
	}
	if sessionID.Valid {
		memory.SessionID = sessionID.String
	}
	if lastAccessedAt.Valid {
		memory.LastAccessedAt = &lastAccessedAt.Time
	}
	if decayUpdatedAt.Valid {
		memory.DecayUpdatedAt = &decayUpdatedAt.Time
	}
	if deletedAt.Valid {
		memory.DeletedAt = &deletedAt.Time
	}
	if contentHash.Valid {
		memory.ContentHash = contentHash.String
	}
	if supersedesID.Valid {
		memory.SupersedesID = supersedesID.String
	}
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
	baseQuery := `
		SELECT
			id, content, source, domain, timestamp, status,
			entity_status, relationship_status, embedding_status,
			enrichment_attempts, enrichment_error,
			created_at, updated_at, enriched_at,
			metadata, tags,
			state, state_updated_at,
			created_by, session_id, source_context,
			access_count, last_accessed_at, decay_score, decay_updated_at,
			deleted_at, content_hash, supersedes_id, memory_type
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
			args = append(args, statusStr)
			conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
		}
	}

	// Typed filter fields.
	if opts.State != "" {
		args = append(args, opts.State)
		conditions = append(conditions, fmt.Sprintf("state = $%d", len(args)))
	}

	if opts.CreatedBy != "" {
		args = append(args, opts.CreatedBy)
		conditions = append(conditions, fmt.Sprintf("created_by = $%d", len(args)))
	}

	if !opts.CreatedAfter.IsZero() {
		args = append(args, opts.CreatedAfter)
		conditions = append(conditions, fmt.Sprintf("created_at > $%d", len(args)))
	}

	if !opts.CreatedBefore.IsZero() {
		args = append(args, opts.CreatedBefore)
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", len(args)))
	}

	if opts.MinDecayScore > 0 {
		args = append(args, opts.MinDecayScore)
		conditions = append(conditions, fmt.Sprintf("decay_score >= $%d", len(args)))
	}

	if opts.SessionID != "" {
		args = append(args, opts.SessionID)
		conditions = append(conditions, fmt.Sprintf("session_id = $%d", len(args)))
	}

	// Exclude soft-deleted memories unless explicitly requested.
	if !opts.IncludeDeleted {
		conditions = append(conditions, "deleted_at IS NULL")
	}

	// When OnlyDeleted is set, restrict to soft-deleted rows only.
	if opts.OnlyDeleted {
		conditions = append(conditions, "deleted_at IS NOT NULL")
	}

	// Filter by memory_type when set.
	if opts.MemoryType != "" {
		args = append(args, opts.MemoryType)
		conditions = append(conditions, fmt.Sprintf("memory_type = $%d", len(args)))
	}

	var whereClause string
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// Build full query with sorting and pagination (safe from SQL injection due to Normalize() whitelist validation above)
	argOffset := len(args) + 1
	query := baseQuery + whereClause
	query += fmt.Sprintf(" ORDER BY %s %s", opts.SortBy, opts.SortOrder)
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argOffset, argOffset+1)
	args = append(args, opts.Limit, opts.Offset())

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to list memories: %w", err)
	}
	defer rows.Close()

	var memories []types.Memory

	for rows.Next() {
		var memory types.Memory
		var metadataJSON, tagsJSON, sourceContextJSON sql.NullString
		var enrichedAt, timestamp, stateUpdatedAt, lastAccessedAt, decayUpdatedAt, deletedAt sql.NullTime
		var domain, enrichmentError, state, createdBy, sessionID sql.NullString
		var contentHash, supersedesID, memTypeNull sql.NullString

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
			return nil, fmt.Errorf("postgres: failed to scan memory: %w", err)
		}

		// Unmarshal JSON fields
		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &memory.Metadata); err != nil {
				return nil, fmt.Errorf("postgres: failed to unmarshal metadata: %w", err)
			}
		}
		if tagsJSON.Valid && tagsJSON.String != "" {
			if err := json.Unmarshal([]byte(tagsJSON.String), &memory.Tags); err != nil {
				return nil, fmt.Errorf("postgres: failed to unmarshal tags: %w", err)
			}
		}
		if sourceContextJSON.Valid && sourceContextJSON.String != "" {
			if err := json.Unmarshal([]byte(sourceContextJSON.String), &memory.SourceContext); err != nil {
				return nil, fmt.Errorf("postgres: failed to unmarshal source_context: %w", err)
			}
		}

		// Map nullable fields
		if domain.Valid {
			memory.Domain = domain.String
		}
		if enrichmentError.Valid {
			memory.EnrichmentError = enrichmentError.String
		}
		if timestamp.Valid {
			memory.Timestamp = timestamp.Time
		}
		if enrichedAt.Valid {
			memory.EnrichedAt = &enrichedAt.Time
		}
		if state.Valid {
			memory.State = state.String
		}
		if stateUpdatedAt.Valid {
			memory.StateUpdatedAt = &stateUpdatedAt.Time
		}
		if createdBy.Valid {
			memory.CreatedBy = createdBy.String
		}
		if sessionID.Valid {
			memory.SessionID = sessionID.String
		}
		if lastAccessedAt.Valid {
			memory.LastAccessedAt = &lastAccessedAt.Time
		}
		if decayUpdatedAt.Valid {
			memory.DecayUpdatedAt = &decayUpdatedAt.Time
		}
		if deletedAt.Valid {
			memory.DeletedAt = &deletedAt.Time
		}
		if contentHash.Valid {
			memory.ContentHash = contentHash.String
		}
		if supersedesID.Valid {
			memory.SupersedesID = supersedesID.String
		}
		if memTypeNull.Valid {
			memory.MemoryType = memTypeNull.String
		}

		memories = append(memories, memory)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: error iterating memories: %w", err)
	}

	// Get total count using a separate query (without pagination args)
	countArgs := args[:len(args)-2]
	countQuery := "SELECT COUNT(*) FROM memories" + whereClause
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("postgres: failed to count memories: %w", err)
	}

	return &storage.PaginatedResult[types.Memory]{
		Items:    memories,
		Total:    total,
		Page:     opts.Page,
		PageSize: opts.Limit,
		HasMore:  opts.Offset()+len(memories) < total,
	}, nil
}

// Update modifies an existing memory.
func (s *MemoryStore) Update(ctx context.Context, memory *types.Memory) error {
	if memory == nil {
		return storage.ErrInvalidInput
	}

	if memory.ID == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	exists, err := s.exists(ctx, memory.ID)
	if err != nil {
		return err
	}

	if !exists {
		return storage.ErrNotFound
	}

	memory.UpdatedAt = time.Now()
	return s.Store(ctx, memory)
}

// Delete removes a memory by ID.
func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	// Soft delete: set deleted_at timestamp
	result, err := s.db.ExecContext(ctx, "UPDATE memories SET deleted_at = CURRENT_TIMESTAMP WHERE id = $1 AND deleted_at IS NULL", id)
	if err != nil {
		return fmt.Errorf("postgres: failed to delete memory: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: failed to check rows affected: %w", err)
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

	result, err := s.db.ExecContext(ctx, "DELETE FROM memories WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("postgres: failed to purge memory: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: failed to check rows affected: %w", err)
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

	query := "UPDATE memories SET status = $1, updated_at = $2 WHERE id = $3"
	result, err := s.db.ExecContext(ctx, query, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("postgres: failed to update status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: failed to check rows affected: %w", err)
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
			entity_status = $1,
			relationship_status = $2,
			embedding_status = $3,
			enrichment_attempts = $4,
			enrichment_error = $5,
			enriched_at = $6,
			updated_at = $7
		WHERE id = $8
	`

	result, err := s.db.ExecContext(ctx, query,
		enrichment.EntityStatus,
		enrichment.RelationshipStatus,
		enrichment.EmbeddingStatus,
		enrichment.EnrichmentAttempts,
		nullableString(enrichment.EnrichmentError),
		nullableTimePtr(enrichment.EnrichedAt),
		time.Now(),
		id,
	)

	if err != nil {
		return fmt.Errorf("postgres: failed to update enrichment: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: failed to check rows affected: %w", err)
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
			state = $1,
			state_updated_at = $2,
			updated_at = $3
		WHERE id = $4
	`

	result, err := s.db.ExecContext(ctx, query, state, now, now, id)
	if err != nil {
		return fmt.Errorf("postgres: failed to update state: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return storage.ErrNotFound
	}

	return nil
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
		    last_accessed_at = $1,
		    decay_score = LEAST(decay_score + 0.1, 1.0)
		WHERE id = $2 AND deleted_at IS NULL
	`

	result, err := s.db.ExecContext(ctx, query, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("postgres: failed to increment access count: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return storage.ErrNotFound
	}

	return nil
}

// GetRelatedMemories returns the IDs of memories that share at least one
// entity with the given memory. This provides 1-hop graph traversal support
// for the GraphTraversal engine.
func (s *MemoryStore) GetRelatedMemories(ctx context.Context, memoryID string) ([]string, error) {
	query := `
		SELECT DISTINCT me2.memory_id
		FROM memory_entities me1
		JOIN memory_entities me2 ON me1.entity_id = me2.entity_id
		WHERE me1.memory_id = $1
		  AND me2.memory_id != $1
	`
	rows, err := s.db.QueryContext(ctx, query, memoryID)
	if err != nil {
		return nil, fmt.Errorf("postgres: GetRelatedMemories: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("postgres: GetRelatedMemories scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: GetRelatedMemories rows: %w", err)
	}
	return ids, nil
}

// Restore un-deletes a soft-deleted memory by clearing its deleted_at timestamp.
func (s *MemoryStore) Restore(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	result, err := s.db.ExecContext(ctx,
		"UPDATE memories SET deleted_at = NULL, updated_at = $1 WHERE id = $2 AND deleted_at IS NOT NULL",
		time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("postgres: failed to restore memory: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return storage.ErrNotFound
	}

	return nil
}

// GetEvolutionChain returns the full version history for a memory,
// ordered oldest -> newest. Walks backward via supersedes_id and forward
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
				state, state_updated_at,
				created_by, session_id, source_context,
				access_count, last_accessed_at, decay_score, decay_updated_at,
				deleted_at, content_hash, supersedes_id, memory_type
			FROM memories WHERE id = $1`

		var m types.Memory
		var metadataJSON, tagsJSON, sourceContextJSON sql.NullString
		var enrichedAt, timestamp sql.NullTime
		var domain, enrichmentError sql.NullString
		var state, createdBy, sessionID sql.NullString
		var contentHash, supersedesID, memType sql.NullString
		var stateUpdatedAt, lastAccessedAt, decayUpdatedAt, deletedAt sql.NullTime

		err := s.db.QueryRowContext(ctx, query, id).Scan(
			&m.ID, &m.Content, &m.Source, &domain, &timestamp, &m.Status,
			&m.EntityStatus, &m.RelationshipStatus, &m.EmbeddingStatus,
			&m.EnrichmentAttempts, &enrichmentError,
			&m.CreatedAt, &m.UpdatedAt, &enrichedAt,
			&metadataJSON, &tagsJSON,
			&state, &stateUpdatedAt,
			&createdBy, &sessionID, &sourceContextJSON,
			&m.AccessCount, &lastAccessedAt, &m.DecayScore, &decayUpdatedAt,
			&deletedAt, &contentHash, &supersedesID, &memType,
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
		if createdBy.Valid {
			m.CreatedBy = createdBy.String
		}
		if sessionID.Valid {
			m.SessionID = sessionID.String
		}
		if enrichmentError.Valid {
			m.EnrichmentError = enrichmentError.String
		}
		if memType.Valid {
			m.MemoryType = memType.String
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
		return nil, fmt.Errorf("postgres: GetEvolutionChain: %w", err)
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
		var nextID string
		err := s.db.QueryRowContext(ctx,
			`SELECT id FROM memories WHERE supersedes_id = $1 LIMIT 1`, tip.ID,
		).Scan(&nextID)
		if err != nil || nextID == "" || visited[nextID] {
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
		WHERE ml.source_id = $1 AND ml.type = $2 AND m.deleted_at IS NULL
	`
	rows, err := s.db.QueryContext(ctx, query, memoryID, relType)
	if err != nil {
		return nil, fmt.Errorf("postgres: GetMemoriesByRelationType: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("postgres: GetMemoriesByRelationType scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: GetMemoriesByRelationType rows: %w", err)
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

// CreateMemoryLink creates a typed link between two memories in the memory_links table.
func (s *MemoryStore) CreateMemoryLink(ctx context.Context, id, sourceID, targetID, linkType string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memory_links (id, source_id, target_id, type) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
		id, sourceID, targetID, linkType,
	)
	if err != nil {
		return fmt.Errorf("postgres: CreateMemoryLink: %w", err)
	}
	return nil
}

// Traverse performs a multi-hop BFS through the entity relationship graph
// starting from startMemoryID and returns up to limit connected memories
// reachable within maxHops.
func (s *MemoryStore) Traverse(ctx context.Context, startMemoryID string, maxHops int, limit int) ([]storage.TraversalResult, error) {
	if startMemoryID == "" {
		return nil, fmt.Errorf("postgres: Traverse: startMemoryID is required")
	}
	if maxHops < 1 {
		maxHops = 2
	}
	if limit < 1 {
		limit = 10
	}

	// Step 1: seed entities from the start memory (hop-0 frontier)
	startEntities, err := s.getEntityIDsForMemory(ctx, startMemoryID)
	if err != nil {
		return nil, fmt.Errorf("postgres: Traverse: seed entities: %w", err)
	}
	if len(startEntities) == 0 {
		return nil, nil // no entities -> no traversal possible
	}

	// Track visited entities to avoid cycles.
	visitedEntities := make(map[string]bool, len(startEntities))
	for _, eid := range startEntities {
		visitedEntities[eid] = true
	}

	// Track discovered memories: memoryID -> (hopDistance, sharedEntityNames).
	type discovered struct {
		hop   int
		names []string
	}
	foundMemories := make(map[string]discovered)

	// Track the start memory so we never include it in results.
	seenMemories := map[string]bool{startMemoryID: true}

	// entityNameCache maps entity IDs to their display names.
	entityNameCache, err := s.getEntityNamesByIDs(ctx, startEntities)
	if err != nil {
		return nil, fmt.Errorf("postgres: Traverse: seed entity names: %w", err)
	}

	// Step 2: BFS
	frontier := startEntities

	for hop := 1; hop <= maxHops; hop++ {
		if len(frontier) == 0 {
			break
		}

		// 2a. Discover memories connected to the current frontier entities.
		for _, eid := range frontier {
			memIDs, err := s.getMemoryIDsForEntity(ctx, eid)
			if err != nil {
				return nil, fmt.Errorf("postgres: Traverse hop %d entity %s: %w", hop, eid, err)
			}
			name := entityNameCache[eid]
			if name == "" {
				name = eid
			}
			for _, mid := range memIDs {
				if seenMemories[mid] {
					continue
				}
				seenMemories[mid] = true
				existing := foundMemories[mid]
				if existing.hop == 0 {
					existing.hop = hop
				}
				existing.names = append(existing.names, name)
				foundMemories[mid] = existing
			}
		}

		// 2b. Expand frontier via relationships.
		neighbourEntities, entityNames, err := s.getNeighbourEntities(ctx, frontier, visitedEntities)
		if err != nil {
			return nil, fmt.Errorf("postgres: Traverse hop %d expand: %w", hop, err)
		}
		for id, name := range entityNames {
			entityNameCache[id] = name
		}
		for _, eid := range neighbourEntities {
			visitedEntities[eid] = true
		}
		frontier = neighbourEntities
	}

	if len(foundMemories) == 0 {
		return nil, nil
	}

	// Step 3: Fetch Memory objects.
	memIDs := make([]string, 0, len(foundMemories))
	for mid := range foundMemories {
		memIDs = append(memIDs, mid)
	}

	memories, err := s.getMemoriesByIDs(ctx, memIDs)
	if err != nil {
		return nil, fmt.Errorf("postgres: Traverse: fetch memories: %w", err)
	}

	// Step 4: Build and sort results.
	results := make([]storage.TraversalResult, 0, len(memories))
	for _, mem := range memories {
		d := foundMemories[mem.ID]
		memCopy := mem
		results = append(results, storage.TraversalResult{
			Memory:         &memCopy,
			HopDistance:    d.hop,
			SharedEntities: uniqueStrings(d.names),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].HopDistance != results[j].HopDistance {
			return results[i].HopDistance < results[j].HopDistance
		}
		return results[i].Memory.DecayScore > results[j].Memory.DecayScore
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetMemoryEntities returns the entities associated with a specific memory.
func (s *MemoryStore) GetMemoryEntities(ctx context.Context, memoryID string) ([]*types.Entity, error) {
	if memoryID == "" {
		return nil, fmt.Errorf("postgres: GetMemoryEntities: memoryID is required")
	}

	query := `
		SELECT e.id, e.name, e.type, e.description, e.created_at, e.updated_at
		FROM entities e
		JOIN memory_entities me ON e.id = me.entity_id
		WHERE me.memory_id = $1
		ORDER BY e.name ASC
	`

	rows, err := s.db.QueryContext(ctx, query, memoryID)
	if err != nil {
		return nil, fmt.Errorf("postgres: GetMemoryEntities: %w", err)
	}
	defer rows.Close()

	var entities []*types.Entity
	for rows.Next() {
		e := &types.Entity{}
		var desc sql.NullString
		if err := rows.Scan(&e.ID, &e.Name, &e.Type, &desc, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("postgres: GetMemoryEntities scan: %w", err)
		}
		if desc.Valid {
			e.Description = desc.String
		}
		entities = append(entities, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: GetMemoryEntities rows: %w", err)
	}
	return entities, nil
}

func (s *MemoryStore) exists(ctx context.Context, id string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memories WHERE id = $1", id).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("postgres: failed to check existence: %w", err)
	}
	return count > 0, nil
}

// nullableString converts a string to sql.NullString (NULL when empty).
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullableTime converts a time.Time to sql.NullTime (NULL when zero).
func nullableTime(t *time.Time) sql.NullTime {
	if t == nil || t.IsZero() {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// nullableTimePtr converts a *time.Time pointer to sql.NullTime (NULL when nil).
func nullableTimePtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// nullableBytes converts a byte slice to sql.NullString (NULL when nil or empty).
func nullableBytes(b []byte) sql.NullString {
	if len(b) == 0 {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: string(b), Valid: true}
}

// ---------------------------------------------------------------------------
// Graph traversal helpers
// ---------------------------------------------------------------------------

// getEntityIDsForMemory returns all entity IDs linked to the given memory.
func (s *MemoryStore) getEntityIDsForMemory(ctx context.Context, memoryID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT entity_id FROM memory_entities WHERE memory_id = $1`, memoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// getEntityNamesByIDs returns a map of entityID -> name for the given IDs.
func (s *MemoryStore) getEntityNamesByIDs(ctx context.Context, ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return make(map[string]string), nil
	}
	inClause, args := buildPgInClause(ids)
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf("SELECT id, name FROM entities WHERE id IN (%s)", inClause), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string, len(ids))
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		result[id] = name
	}
	return result, rows.Err()
}

// getMemoryIDsForEntity returns all memory IDs linked to the given entity.
func (s *MemoryStore) getMemoryIDsForEntity(ctx context.Context, entityID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT memory_id FROM memory_entities WHERE entity_id = $1`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// getNeighbourEntities returns entity IDs reachable from the given frontier
// entities via the relationships table (both directions), excluding already-
// visited entity IDs.
func (s *MemoryStore) getNeighbourEntities(ctx context.Context, frontier []string, visited map[string]bool) ([]string, map[string]string, error) {
	if len(frontier) == 0 {
		return nil, nil, nil
	}

	inClause, placeholders := buildPgInClause(frontier)
	// Double the placeholders: once for source_id IN, once for target_id IN.
	args := append(placeholders, placeholders...)

	// Adjust the second set of $N references for PostgreSQL.
	offset := len(frontier)
	inClause2Parts := make([]string, len(frontier))
	for i := range frontier {
		inClause2Parts[i] = fmt.Sprintf("$%d", offset+i+1)
	}
	inClause2 := strings.Join(inClause2Parts, ",")

	query := fmt.Sprintf(`
		SELECT r.source_id, r.target_id,
		       COALESCE(e_src.name, r.source_id) AS source_name,
		       COALESCE(e_tgt.name, r.target_id) AS target_name
		FROM relationships r
		LEFT JOIN entities e_src ON e_src.id = r.source_id
		LEFT JOIN entities e_tgt ON e_tgt.id = r.target_id
		WHERE r.source_id IN (%s) OR r.target_id IN (%s)
	`, inClause, inClause2)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	frontierSet := make(map[string]bool, len(frontier))
	for _, id := range frontier {
		frontierSet[id] = true
	}

	newEntities := make(map[string]string) // entityID -> name
	for rows.Next() {
		var srcID, tgtID, srcName, tgtName string
		if err := rows.Scan(&srcID, &tgtID, &srcName, &tgtName); err != nil {
			return nil, nil, err
		}
		if frontierSet[srcID] && !visited[tgtID] {
			newEntities[tgtID] = srcName
		}
		if frontierSet[tgtID] && !visited[srcID] {
			newEntities[srcID] = tgtName
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	ids := make([]string, 0, len(newEntities))
	for id := range newEntities {
		ids = append(ids, id)
	}
	return ids, newEntities, nil
}

// getMemoriesByIDs fetches Memory objects for a list of IDs.
// Soft-deleted memories are excluded.
func (s *MemoryStore) getMemoriesByIDs(ctx context.Context, ids []string) ([]types.Memory, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	inClause, args := buildPgInClause(ids)

	query := fmt.Sprintf(`
		SELECT
			id, content, source, domain, timestamp, status,
			entity_status, relationship_status, embedding_status,
			enrichment_attempts, enrichment_error,
			created_at, updated_at, enriched_at,
			metadata, tags,
			state, state_updated_at,
			created_by, session_id, source_context,
			access_count, last_accessed_at, decay_score, decay_updated_at,
			deleted_at, content_hash, supersedes_id, memory_type
		FROM memories
		WHERE id IN (%s) AND deleted_at IS NULL
	`, inClause)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []types.Memory
	for rows.Next() {
		var mem types.Memory
		var metadataJSON, tagsJSON, sourceContextJSON sql.NullString
		var enrichedAt, timestamp sql.NullTime
		var domain, enrichmentError sql.NullString
		var state, createdBy, sessionID sql.NullString
		var contentHash, supersedesID, memType sql.NullString
		var stateUpdatedAt, lastAccessedAt, decayUpdatedAt, deletedAt sql.NullTime

		if err := rows.Scan(
			&mem.ID, &mem.Content, &mem.Source, &domain, &timestamp, &mem.Status,
			&mem.EntityStatus, &mem.RelationshipStatus, &mem.EmbeddingStatus,
			&mem.EnrichmentAttempts, &enrichmentError,
			&mem.CreatedAt, &mem.UpdatedAt, &enrichedAt,
			&metadataJSON, &tagsJSON,
			&state, &stateUpdatedAt,
			&createdBy, &sessionID, &sourceContextJSON,
			&mem.AccessCount, &lastAccessedAt, &mem.DecayScore, &decayUpdatedAt,
			&deletedAt, &contentHash, &supersedesID, &memType,
		); err != nil {
			return nil, err
		}

		if domain.Valid {
			mem.Domain = domain.String
		}
		if timestamp.Valid {
			mem.Timestamp = timestamp.Time
		}
		if enrichedAt.Valid {
			t := enrichedAt.Time
			mem.EnrichedAt = &t
		}
		if enrichmentError.Valid {
			mem.EnrichmentError = enrichmentError.String
		}
		if state.Valid {
			mem.State = state.String
		}
		if stateUpdatedAt.Valid {
			t := stateUpdatedAt.Time
			mem.StateUpdatedAt = &t
		}
		if createdBy.Valid {
			mem.CreatedBy = createdBy.String
		}
		if sessionID.Valid {
			mem.SessionID = sessionID.String
		}
		if lastAccessedAt.Valid {
			t := lastAccessedAt.Time
			mem.LastAccessedAt = &t
		}
		if decayUpdatedAt.Valid {
			t := decayUpdatedAt.Time
			mem.DecayUpdatedAt = &t
		}
		if deletedAt.Valid {
			t := deletedAt.Time
			mem.DeletedAt = &t
		}
		if contentHash.Valid {
			mem.ContentHash = contentHash.String
		}
		if supersedesID.Valid {
			mem.SupersedesID = supersedesID.String
		}
		if memType.Valid {
			mem.MemoryType = memType.String
		}
		if metadataJSON.Valid && metadataJSON.String != "" {
			_ = json.Unmarshal([]byte(metadataJSON.String), &mem.Metadata)
		}
		if tagsJSON.Valid && tagsJSON.String != "" {
			_ = json.Unmarshal([]byte(tagsJSON.String), &mem.Tags)
		}

		memories = append(memories, mem)
	}
	return memories, rows.Err()
}

// buildPgInClause returns a PostgreSQL-compatible parameterized IN clause
// (e.g., "$1,$2,$3") and the corresponding args slice.
func buildPgInClause(ids []string) (string, []interface{}) {
	parts := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	return strings.Join(parts, ","), args
}

// uniqueStrings deduplicates a string slice while preserving order.
func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
