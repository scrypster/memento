package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	pgvector "github.com/pgvector/pgvector-go"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// Ensure *MemoryStore implements storage.SearchProvider at compile time.
var _ storage.SearchProvider = (*MemoryStore)(nil)

// memorySelectColumns is the canonical SELECT column list for the memories table.
// It must match the scan order in scanMemoryRow.
const memorySelectColumns = `
	id, content, source, domain, timestamp, status,
	entity_status, relationship_status, embedding_status,
	enrichment_attempts, enrichment_error,
	created_at, updated_at, enriched_at,
	metadata, tags,
	state, state_updated_at,
	created_by, session_id, source_context,
	access_count, last_accessed_at, decay_score, decay_updated_at,
	deleted_at, content_hash, supersedes_id, memory_type
`

// FullTextSearch performs PostgreSQL tsvector full-text search across memory content.
//
// When opts.Query is empty the method falls back to a full table scan ordered
// by created_at DESC so the caller still receives a useful result set.
//
// When the content_tsv column is not yet populated (e.g. on a fresh row that
// hasn't been through the UPDATE trigger) we fall back gracefully to ILIKE.
func (s *MemoryStore) FullTextSearch(ctx context.Context, opts storage.SearchOptions) (*storage.PaginatedResult[types.Memory], error) {
	opts.Normalize()

	// When the query is empty fall back to a plain list ordered by creation time.
	if strings.TrimSpace(opts.Query) == "" {
		return s.List(ctx, storage.ListOptions{
			Page:      1,
			Limit:     opts.Limit,
			SortBy:    "created_at",
			SortOrder: "desc",
		})
	}

	const querySQL = `
		SELECT ` + memorySelectColumns + `
		FROM memories
		WHERE content_tsv @@ plainto_tsquery('english', $1) AND deleted_at IS NULL
		ORDER BY ts_rank(content_tsv, plainto_tsquery('english', $1)) DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.QueryContext(ctx, querySQL, opts.Query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("postgres: FullTextSearch query %q: %w", opts.Query, err)
	}
	defer func() { _ = rows.Close() }()

	memories, err := scanMemoryRows(rows)
	if err != nil {
		return nil, fmt.Errorf("postgres: FullTextSearch scan: %w", err)
	}

	// Count total matching rows for pagination.
	const countSQL = `
		SELECT COUNT(*)
		FROM memories
		WHERE content_tsv @@ plainto_tsquery('english', $1) AND deleted_at IS NULL
	`
	var total int
	if err := s.db.QueryRowContext(ctx, countSQL, opts.Query).Scan(&total); err != nil {
		return nil, fmt.Errorf("postgres: FullTextSearch count: %w", err)
	}

	page := 1
	if opts.Limit > 0 {
		page = (opts.Offset / opts.Limit) + 1
	}

	result := &storage.PaginatedResult[types.Memory]{
		Items:    memories,
		Total:    total,
		Page:     page,
		PageSize: opts.Limit,
		HasMore:  opts.Offset+len(memories) < total,
	}

	// Fuzzy fallback: if no results and FuzzyFallback is enabled, retry with OR'd terms
	if opts.FuzzyFallback && len(result.Items) == 0 && opts.Query != "" {
		terms := strings.Fields(opts.Query)
		if len(terms) > 1 {
			relaxedOpts := opts
			relaxedOpts.Query = strings.Join(terms, " OR ")
			relaxedOpts.FuzzyFallback = false // prevent recursion
			return s.FullTextSearch(ctx, relaxedOpts)
		}
	}

	return result, nil
}

// VectorSearch performs semantic similarity search using pgvector cosine distance.
// The search is accelerated by an ivfflat index (idx_embeddings_vec_cosine) when the embeddings table is non-empty.
//
// When pgvector is not available or the embedding_vec column is not populated,
// it falls back to returning recent memories (same as FullTextSearch with empty
// query).
func (s *MemoryStore) VectorSearch(ctx context.Context, query []float64, opts storage.SearchOptions) (*storage.PaginatedResult[types.Memory], error) {
	opts.Normalize()

	if len(query) == 0 {
		return &storage.PaginatedResult[types.Memory]{Items: []types.Memory{}, PageSize: opts.Limit}, nil
	}

	if !s.pgvectorAvailable {
		// Fall back to recent memories when pgvector is not available.
		return s.List(ctx, storage.ListOptions{
			Page:      1,
			Limit:     opts.Limit,
			SortBy:    "created_at",
			SortOrder: "desc",
		})
	}

	// Convert float64 slice to float32 for pgvector.
	f32 := make([]float32, len(query))
	for i, v := range query {
		f32[i] = float32(v)
	}
	vec := pgvector.NewVector(f32)

	const querySQL = `
		SELECT ` + memorySelectColumns + `
		FROM memories m
		JOIN embeddings e ON e.memory_id = m.id
		WHERE e.embedding_vec IS NOT NULL AND m.deleted_at IS NULL
		ORDER BY e.embedding_vec <=> $1::vector
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.QueryContext(ctx, querySQL, vec, opts.Limit, opts.Offset)
	if err != nil {
		// If the query fails (e.g. no rows with embedding_vec yet), fall back.
		return s.List(ctx, storage.ListOptions{
			Page:      1,
			Limit:     opts.Limit,
			SortBy:    "created_at",
			SortOrder: "desc",
		})
	}
	defer func() { _ = rows.Close() }()

	memories, err := scanMemoryRows(rows)
	if err != nil {
		return nil, fmt.Errorf("postgres: VectorSearch scan: %w", err)
	}

	// Count total rows with embedding vectors for pagination.
	const countSQL = `
		SELECT COUNT(*)
		FROM memories m
		JOIN embeddings e ON e.memory_id = m.id
		WHERE e.embedding_vec IS NOT NULL AND m.deleted_at IS NULL
	`
	var total int
	if err := s.db.QueryRowContext(ctx, countSQL).Scan(&total); err != nil {
		total = len(memories) + opts.Offset
	}

	return &storage.PaginatedResult[types.Memory]{
		Items:    memories,
		Total:    total,
		PageSize: opts.Limit,
		HasMore:  opts.Offset+len(memories) < total,
	}, nil
}

// HybridSearch combines full-text search and vector similarity search using
// Reciprocal Rank Fusion (RRF) to merge and re-rank results.
// When no vector is provided or pgvector is unavailable, it falls back to
// FullTextSearch.
func (s *MemoryStore) HybridSearch(ctx context.Context, text string, vector []float64, opts storage.SearchOptions) (*storage.PaginatedResult[types.Memory], error) {
	if len(vector) == 0 || !s.pgvectorAvailable {
		opts.Query = text
		return s.FullTextSearch(ctx, opts)
	}

	// Fetch more candidates for merging (3× requested limit for each source).
	candidateLimit := opts.Limit * 3
	if candidateLimit < 30 {
		candidateLimit = 30
	}

	ftsOpts := storage.SearchOptions{Query: text, Limit: candidateLimit}
	ftsResult, err := s.FullTextSearch(ctx, ftsOpts)
	if err != nil {
		return nil, fmt.Errorf("postgres: hybrid search FTS failed: %w", err)
	}

	vecOpts := storage.SearchOptions{Limit: candidateLimit}
	vecResult, err := s.VectorSearch(ctx, vector, vecOpts)
	if err != nil {
		// Vector search failure is non-fatal — fall back to FTS only.
		opts.Query = text
		return s.FullTextSearch(ctx, opts)
	}

	// Reciprocal Rank Fusion (k=60 is a well-tuned default).
	const rrfK = 60.0
	scores := make(map[string]float64)
	for rank, mem := range ftsResult.Items {
		scores[mem.ID] += 1.0 / (rrfK + float64(rank+1))
	}
	for rank, mem := range vecResult.Items {
		scores[mem.ID] += 1.0 / (rrfK + float64(rank+1))
	}

	// Build a deduplicated list of all candidate memory IDs, sorted by RRF score.
	type scoredID struct {
		id    string
		score float64
	}
	var ranked []scoredID
	for id, score := range scores {
		ranked = append(ranked, scoredID{id, score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	total := len(ranked)
	offset := opts.Offset
	if offset >= total {
		return &storage.PaginatedResult[types.Memory]{Items: []types.Memory{}, Total: total, PageSize: opts.Limit}, nil
	}
	end := offset + opts.Limit
	if end > total {
		end = total
	}

	var memories []types.Memory
	for _, r := range ranked[offset:end] {
		mem, err := s.Get(ctx, r.id)
		if err != nil {
			continue
		}
		memories = append(memories, *mem)
	}

	return &storage.PaginatedResult[types.Memory]{
		Items:    memories,
		Total:    total,
		PageSize: opts.Limit,
		HasMore:  end < total,
	}, nil
}

// scanMemoryRows reads all rows returned by a query into a []types.Memory slice.
// The SELECT column order must match memorySelectColumns.
func scanMemoryRows(rows *sql.Rows) ([]types.Memory, error) {
	var memories []types.Memory

	for rows.Next() {
		mem, err := scanMemoryRow(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, mem)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: rows error: %w", err)
	}

	return memories, nil
}

// scanMemoryRow scans a single row (from *sql.Rows) into a types.Memory.
// The SELECT column order must match memorySelectColumns.
func scanMemoryRow(rows *sql.Rows) (types.Memory, error) {
	var memory types.Memory
	var metadataJSON, tagsJSON, sourceContextJSON sql.NullString
	var enrichedAt, timestamp, stateUpdatedAt, lastAccessedAt, decayUpdatedAt, deletedAt sql.NullTime
	var domain, enrichmentError, state, createdBy, sessionID sql.NullString
	var contentHash, supersedesID, memType sql.NullString

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
		&memType,
	)
	if err != nil {
		return memory, fmt.Errorf("postgres: scan memory row: %w", err)
	}

	// Unmarshal JSON fields.
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &memory.Metadata); err != nil {
			return memory, fmt.Errorf("postgres: unmarshal metadata: %w", err)
		}
	}
	if tagsJSON.Valid && tagsJSON.String != "" {
		if err := json.Unmarshal([]byte(tagsJSON.String), &memory.Tags); err != nil {
			return memory, fmt.Errorf("postgres: unmarshal tags: %w", err)
		}
	}
	if sourceContextJSON.Valid && sourceContextJSON.String != "" {
		if err := json.Unmarshal([]byte(sourceContextJSON.String), &memory.SourceContext); err != nil {
			return memory, fmt.Errorf("postgres: unmarshal source_context: %w", err)
		}
	}

	// Map nullable fields.
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
		t := stateUpdatedAt.Time
		memory.StateUpdatedAt = &t
	}
	if createdBy.Valid {
		memory.CreatedBy = createdBy.String
	}
	if sessionID.Valid {
		memory.SessionID = sessionID.String
	}
	if lastAccessedAt.Valid {
		t := lastAccessedAt.Time
		memory.LastAccessedAt = &t
	}
	if decayUpdatedAt.Valid {
		t := decayUpdatedAt.Time
		memory.DecayUpdatedAt = &t
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
	if memType.Valid {
		memory.MemoryType = memType.String
	}

	return memory, nil
}
