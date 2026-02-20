package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// Ensure *MemoryStore implements storage.SearchProvider at compile time.
var _ storage.SearchProvider = (*MemoryStore)(nil)

// FullTextSearch performs FTS5-backed full-text search across memory content.
//
// The FTS5 virtual table (memories_fts) is kept in sync with the memories
// table via INSERT/UPDATE/DELETE triggers defined in schema.go.
//
// When opts.Query is empty the method falls back to a full table scan ordered
// by created_at DESC so the caller still receives a useful result set.
//
// FTS5 rank values are negative (more negative == better match), so ordering
// by rank ASC gives the best results first.
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

	// Sanitise the raw query string so it is safe to pass to FTS5's MATCH
	// operator.  FTS5 syntax is powerful but fragile: an unbalanced quote or
	// stray operator keyword will cause SQLite to return "fts5: syntax error".
	// We convert the free-form user input into a simple prefix query that
	// searches for each word individually (OR semantics).
	ftsQuery := sanitiseFTSQuery(opts.Query)

	const querySQL = `
		SELECT
			m.id, m.content, m.source, m.domain, m.timestamp, m.status,
			m.entity_status, m.relationship_status, m.embedding_status,
			m.enrichment_attempts, m.enrichment_error,
			m.created_at, m.updated_at, m.enriched_at,
			m.metadata, m.tags,
			m.state, m.state_updated_at,
			m.created_by, m.session_id, m.source_context,
			m.access_count, m.last_accessed_at, m.decay_score, m.decay_updated_at
		FROM memories_fts fts
		JOIN memories m ON m.rowid = fts.rowid
		WHERE memories_fts MATCH ? AND m.deleted_at IS NULL
		ORDER BY rank
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.QueryContext(ctx, querySQL, ftsQuery, opts.Limit, opts.Offset)
	if err != nil {
		// FTS5 can still error on malformed input that slipped past sanitisation.
		// Wrap the error with enough context for callers to diagnose.
		return nil, fmt.Errorf("sqlite: FullTextSearch MATCH %q: %w", opts.Query, err)
	}
	defer func() { _ = rows.Close() }()

	memories, err := scanMemories(rows)
	if err != nil {
		return nil, fmt.Errorf("sqlite: FullTextSearch scan: %w", err)
	}

	// Count total matching rows (without LIMIT/OFFSET) so the caller can
	// determine whether more pages exist.
	const countSQL = `
		SELECT COUNT(*)
		FROM memories_fts fts
		JOIN memories m ON m.rowid = fts.rowid
		WHERE memories_fts MATCH ? AND m.deleted_at IS NULL
	`
	var total int
	if err := s.db.QueryRowContext(ctx, countSQL, ftsQuery).Scan(&total); err != nil {
		return nil, fmt.Errorf("sqlite: FullTextSearch count: %w", err)
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

// vectorSearchMaxCandidates caps the number of embeddings loaded into memory
// during a vector search. Embeddings are selected in recency order (newest first)
// so the most recently-created memories are always considered. For typical
// personal-memory datasets (< 10k memories) this limit is never hit.
// For very large datasets, migrate to PostgreSQL + pgvector for indexed ANN search.
const vectorSearchMaxCandidates = 10_000

// VectorSearch performs semantic similarity search using stored embeddings.
// Embeddings are loaded into Go memory and ranked by cosine similarity.
// The candidate pool is capped at vectorSearchMaxCandidates (most-recent first)
// to avoid excessive memory use on large datasets.
func (s *MemoryStore) VectorSearch(ctx context.Context, query []float64, opts storage.SearchOptions) (*storage.PaginatedResult[types.Memory], error) {
	opts.Normalize()

	if len(query) == 0 {
		return &storage.PaginatedResult[types.Memory]{Items: []types.Memory{}, PageSize: opts.Limit}, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT e.memory_id, e.embedding, e.dimension
		FROM embeddings e
		JOIN memories m ON m.id = e.memory_id
		WHERE m.deleted_at IS NULL
		ORDER BY m.created_at DESC
		LIMIT ?`, vectorSearchMaxCandidates)
	if err != nil {
		return nil, fmt.Errorf("failed to load embeddings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type scored struct {
		memoryID string
		score    float64
	}
	var candidates []scored

	for rows.Next() {
		var memID string
		var blob []byte
		var dim int
		if err := rows.Scan(&memID, &blob, &dim); err != nil {
			continue
		}
		embedding, err := deserializeEmbedding(blob, dim)
		if err != nil {
			continue
		}
		sim := cosineSimilarity(query, embedding)
		candidates = append(candidates, scored{memID, sim})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating embeddings: %w", err)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	total := len(candidates)
	offset := opts.Offset
	if offset >= total {
		return &storage.PaginatedResult[types.Memory]{Items: []types.Memory{}, Total: total, PageSize: opts.Limit}, nil
	}
	end := offset + opts.Limit
	if end > total {
		end = total
	}

	var memories []types.Memory
	for _, c := range candidates[offset:end] {
		mem, err := s.Get(ctx, c.memoryID)
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

// HybridSearch combines full-text search and vector similarity search using
// Reciprocal Rank Fusion (RRF) to merge and re-rank results.
// When no vector is provided, it falls back to FullTextSearch.
func (s *MemoryStore) HybridSearch(ctx context.Context, text string, vector []float64, opts storage.SearchOptions) (*storage.PaginatedResult[types.Memory], error) {
	if len(vector) == 0 {
		opts.Query = text
		return s.FullTextSearch(ctx, opts)
	}

	// Fetch more candidates for merging (3× requested limit for each source)
	candidateLimit := opts.Limit * 3
	if candidateLimit < 30 {
		candidateLimit = 30
	}

	ftsOpts := storage.SearchOptions{Query: text, Limit: candidateLimit}
	ftsResult, err := s.FullTextSearch(ctx, ftsOpts)
	if err != nil {
		return nil, fmt.Errorf("hybrid search FTS failed: %w", err)
	}

	vecOpts := storage.SearchOptions{Limit: candidateLimit}
	vecResult, err := s.VectorSearch(ctx, vector, vecOpts)
	if err != nil {
		// Vector search failure is non-fatal — fall back to FTS only
		opts.Query = text
		return s.FullTextSearch(ctx, opts)
	}

	// Reciprocal Rank Fusion (k=60 is a well-tuned default)
	const rrfK = 60.0
	scores := make(map[string]float64)
	for rank, mem := range ftsResult.Items {
		scores[mem.ID] += 1.0 / (rrfK + float64(rank+1))
	}
	for rank, mem := range vecResult.Items {
		scores[mem.ID] += 1.0 / (rrfK + float64(rank+1))
	}

	// Build a deduplicated list of all candidate memory IDs, sorted by RRF score
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

// cosineSimilarity computes cosine similarity between two equal-length vectors.
// Returns 0 if either vector has zero magnitude or lengths differ.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// sanitiseFTSQuery converts a free-form user query into a safe FTS5 MATCH
// expression. It strips FTS5-special characters, removes common stop words,
// and uses prefix matching (term*) for better recall.
//
// Example: "What is Memento?" → "memento*"
// Example: "MJ coding preferences" → "mj* OR coding* OR preferences*"
func sanitiseFTSQuery(query string) string {
	// Strip FTS5 special characters.
	replacer := strings.NewReplacer(
		`"`, ` `,
		`'`, ` `,
		`(`, ` `,
		`)`, ` `,
		`*`, ` `,
		`-`, ` `,
		`^`, ` `,
		`?`, ` `,
		`:`, ` `,
	)
	cleaned := replacer.Replace(query)

	// Split into lowercase words.
	words := strings.Fields(strings.ToLower(cleaned))

	// Filter stop words that carry no discriminative value.
	stopWords := map[string]bool{
		"a": true, "an": true, "the": true,
		"is": true, "are": true, "was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true,
		"will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "shall": true, "can": true,
		"to": true, "of": true, "in": true, "on": true, "at": true,
		"by": true, "for": true, "with": true, "from": true, "as": true,
		"about": true, "into": true, "through": true, "during": true,
		"before": true, "after": true, "above": true, "below": true,
		"between": true, "out": true, "off": true, "over": true, "under": true,
		"what": true, "how": true, "when": true, "where": true, "why": true,
		"who": true, "which": true,
		"this": true, "that": true, "these": true, "those": true,
		"i": true, "you": true, "he": true, "she": true, "it": true, "we": true, "they": true,
		"and": true, "or": true, "but": true, "if": true, "not": true,
		"s": true, "t": true, // post-apostrophe fragments e.g. "MJ's" → "MJ" + "s"
	}

	var terms []string
	for _, w := range words {
		if !stopWords[w] && len(w) >= 2 {
			terms = append(terms, w+"*")
		}
	}

	if len(terms) == 0 {
		// All words were stop words — fall back to a lowercased form of the
		// cleaned text so FTS5 does not interpret uppercase AND/OR/NOT as operators.
		return strings.ToLower(strings.TrimSpace(cleaned))
	}

	return strings.Join(terms, " OR ")
}

// scanMemories reads all rows returned by a query into a []types.Memory slice.
// The SELECT column order must match the order used in FullTextSearch above,
// which mirrors the order used in Get and List.
func scanMemories(rows *sql.Rows) ([]types.Memory, error) {
	var memories []types.Memory

	for rows.Next() {
		var memory types.Memory
		var metadataJSON, tagsJSON sql.NullString
		var enrichedAt, timestamp sql.NullTime
		var domain sql.NullString
		var enrichmentError sql.NullString

		var state, createdBy, sessionID sql.NullString
		var sourceContextJSON sql.NullString
		var stateUpdatedAt, lastAccessedAt, decayUpdatedAt sql.NullTime

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
		)
		if err != nil {
			return nil, fmt.Errorf("scan memory row: %w", err)
		}

		if enrichmentError.Valid {
			memory.EnrichmentError = enrichmentError.String
		}
		if err := unmarshalMemoryFields(
			&memory,
			metadataJSON, tagsJSON, sourceContextJSON,
			enrichedAt, timestamp, domain,
			state, stateUpdatedAt,
			createdBy, sessionID,
			lastAccessedAt, decayUpdatedAt,
		); err != nil {
			return nil, err
		}

		memories = append(memories, memory)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return memories, nil
}

// unmarshalMemoryFields decodes the nullable SQL fields into the types.Memory
// struct.  This is extracted so it can be shared between FullTextSearch and
// any future scan sites.
func unmarshalMemoryFields(
	memory *types.Memory,
	metadataJSON, tagsJSON, sourceContextJSON sql.NullString,
	enrichedAt, timestamp sql.NullTime,
	domain sql.NullString,
	state sql.NullString,
	stateUpdatedAt sql.NullTime,
	createdBy, sessionID sql.NullString,
	lastAccessedAt, decayUpdatedAt sql.NullTime,
) error {
	import_json_for_unmarshal := func(ns sql.NullString, dest interface{}) error {
		if !ns.Valid || ns.String == "" {
			return nil
		}
		return json.Unmarshal([]byte(ns.String), dest)
	}

	if err := import_json_for_unmarshal(metadataJSON, &memory.Metadata); err != nil {
		return fmt.Errorf("unmarshal metadata: %w", err)
	}
	if err := import_json_for_unmarshal(tagsJSON, &memory.Tags); err != nil {
		return fmt.Errorf("unmarshal tags: %w", err)
	}
	if err := import_json_for_unmarshal(sourceContextJSON, &memory.SourceContext); err != nil {
		return fmt.Errorf("unmarshal source_context: %w", err)
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

	return nil
}
