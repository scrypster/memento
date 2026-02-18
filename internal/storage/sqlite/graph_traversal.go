package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// Traverse performs a multi-hop BFS through the entity relationship graph
// starting from startMemoryID and returns up to limit connected memories
// reachable within maxHops.
//
// Algorithm:
//  1. Look up entities for startMemoryID via memory_entities.
//     These seed entities form the hop-0 frontier.
//  2. BFS loop (hop = 1..maxHops):
//     a. Find memories connected to the current frontier entities.
//        These memories are at distance `hop` from the start.
//     b. Expand the frontier: query relationships from frontier entities
//        to obtain their neighbours (new, unvisited entities).
//        The neighbours become the frontier for the next iteration.
//  3. Fetch Memory objects for all discovered memory IDs.
//  4. Return sorted by hopDistance ASC, then DecayScore DESC.
//
// Cycle detection: visitedEntities prevents re-visiting the same entity,
// and seenMemories prevents the same memory from appearing more than once.
func (s *MemoryStore) Traverse(ctx context.Context, startMemoryID string, maxHops int, limit int) ([]storage.TraversalResult, error) {
	if startMemoryID == "" {
		return nil, fmt.Errorf("sqlite: Traverse: startMemoryID is required")
	}
	if maxHops < 1 {
		maxHops = 2
	}
	if limit < 1 {
		limit = 10
	}

	db := s.GetDB()

	// --- Step 1: seed entities from the start memory (hop-0 frontier) ---
	startEntities, err := s.getEntityIDsForMemory(ctx, db, startMemoryID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: Traverse: seed entities: %w", err)
	}
	if len(startEntities) == 0 {
		return nil, nil // no entities → no traversal possible
	}

	// Track visited entities to avoid cycles.
	visitedEntities := make(map[string]bool, len(startEntities))
	for _, eid := range startEntities {
		visitedEntities[eid] = true
	}

	// Track discovered memories: memoryID → (hopDistance, sharedEntityNames).
	type discovered struct {
		hop   int
		names []string
	}
	foundMemories := make(map[string]discovered)

	// Track the start memory so we never include it in results.
	seenMemories := map[string]bool{startMemoryID: true}

	// entityNameCache maps entity IDs to their display names.
	// Pre-populate with names of the seed entities.
	entityNameCache, err := s.getEntityNamesByIDs(ctx, db, startEntities)
	if err != nil {
		return nil, fmt.Errorf("sqlite: Traverse: seed entity names: %w", err)
	}

	// --- Step 2: BFS ---
	// frontier holds the entity IDs whose memories we will surface at this hop.
	frontier := startEntities

	for hop := 1; hop <= maxHops; hop++ {
		if len(frontier) == 0 {
			break
		}

		// 2a. Discover memories connected to the current frontier entities.
		//     These memories are reachable in exactly `hop` steps.
		for _, eid := range frontier {
			memIDs, err := s.getMemoryIDsForEntity(ctx, db, eid)
			if err != nil {
				return nil, fmt.Errorf("sqlite: Traverse hop %d entity %s: %w", hop, eid, err)
			}
			// Look up entity name for display (cache to avoid redundant queries).
			name := entityNameCache[eid]
			if name == "" {
				name = eid // fallback to ID
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

		// 2b. Expand frontier: find entities reachable via relationships from
		//     the current frontier (both directions). These become the next frontier.
		neighbourEntities, entityNames, err := s.getNeighbourEntities(ctx, db, frontier, visitedEntities)
		if err != nil {
			return nil, fmt.Errorf("sqlite: Traverse hop %d expand: %w", hop, err)
		}

		// Cache entity names returned by the expansion.
		for id, name := range entityNames {
			entityNameCache[id] = name
		}

		// Mark newly found entities as visited.
		for _, eid := range neighbourEntities {
			visitedEntities[eid] = true
		}

		// Advance the frontier for the next hop.
		frontier = neighbourEntities
	}

	if len(foundMemories) == 0 {
		return nil, nil
	}

	// --- Step 3: Fetch Memory objects ---
	memIDs := make([]string, 0, len(foundMemories))
	for mid := range foundMemories {
		memIDs = append(memIDs, mid)
	}

	memories, err := s.getMemoriesByIDs(ctx, memIDs)
	if err != nil {
		return nil, fmt.Errorf("sqlite: Traverse: fetch memories: %w", err)
	}

	// --- Step 4: Build and sort results ---
	results := make([]storage.TraversalResult, 0, len(memories))
	for _, mem := range memories {
		d := foundMemories[mem.ID]
		memCopy := mem // avoid loop variable capture
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
		// Higher decay score is "more important".
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
		return nil, fmt.Errorf("sqlite: GetMemoryEntities: memoryID is required")
	}

	query := `
		SELECT e.id, e.name, e.type, e.description, e.created_at, e.updated_at
		FROM entities e
		JOIN memory_entities me ON e.id = me.entity_id
		WHERE me.memory_id = ?
		ORDER BY e.name ASC
	`

	rows, err := s.GetDB().QueryContext(ctx, query, memoryID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: GetMemoryEntities: %w", err)
	}
	defer rows.Close()

	var entities []*types.Entity
	for rows.Next() {
		e := &types.Entity{}
		var desc sql.NullString
		if err := rows.Scan(&e.ID, &e.Name, &e.Type, &desc, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: GetMemoryEntities scan: %w", err)
		}
		if desc.Valid {
			e.Description = desc.String
		}
		entities = append(entities, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: GetMemoryEntities rows: %w", err)
	}
	return entities, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// getEntityNamesByIDs returns a map of entityID → name for the given IDs.
func (s *MemoryStore) getEntityNamesByIDs(ctx context.Context, db *sql.DB, ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return make(map[string]string), nil
	}
	inClause := buildInClause(len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := db.QueryContext(ctx,
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

// getEntityIDsForMemory returns all entity IDs linked to the given memory.
func (s *MemoryStore) getEntityIDsForMemory(ctx context.Context, db *sql.DB, memoryID string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT entity_id FROM memory_entities WHERE memory_id = ?`, memoryID)
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
// It also returns a name map so callers can track which entity was the bridge.
func (s *MemoryStore) getNeighbourEntities(ctx context.Context, db *sql.DB, frontier []string, visited map[string]bool) ([]string, map[string]string, error) {
	if len(frontier) == 0 {
		return nil, nil, nil
	}

	// Build placeholder list for IN clause.
	placeholders := make([]interface{}, len(frontier))
	for i, id := range frontier {
		placeholders[i] = id
	}
	inClause := buildInClause(len(frontier))

	// Query relationships in both directions (source→target and target→source).
	query := fmt.Sprintf(`
		SELECT r.source_id, r.target_id,
		       COALESCE(e_src.name, r.source_id) AS source_name,
		       COALESCE(e_tgt.name, r.target_id) AS target_name
		FROM relationships r
		LEFT JOIN entities e_src ON e_src.id = r.source_id
		LEFT JOIN entities e_tgt ON e_tgt.id = r.target_id
		WHERE r.source_id IN (%s) OR r.target_id IN (%s)
	`, inClause, inClause)

	// Double the placeholders: once for source_id IN, once for target_id IN.
	args := append(placeholders, placeholders...)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	// Build a set of frontier IDs for quick lookup.
	frontierSet := make(map[string]bool, len(frontier))
	for _, id := range frontier {
		frontierSet[id] = true
	}

	newEntities := make(map[string]string) // entityID → name (bridge entity name)
	for rows.Next() {
		var srcID, tgtID, srcName, tgtName string
		if err := rows.Scan(&srcID, &tgtID, &srcName, &tgtName); err != nil {
			return nil, nil, err
		}

		// If source is in frontier, add target as neighbour and vice versa.
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

// getMemoryIDsForEntity returns all memory IDs linked to the given entity.
func (s *MemoryStore) getMemoryIDsForEntity(ctx context.Context, db *sql.DB, entityID string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT memory_id FROM memory_entities WHERE entity_id = ?`, entityID)
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

// getMemoriesByIDs fetches Memory objects for a list of IDs.
// Soft-deleted memories are excluded.
func (s *MemoryStore) getMemoriesByIDs(ctx context.Context, ids []string) ([]types.Memory, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	inClause := buildInClause(len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	query := fmt.Sprintf(`
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
		var (
			metadataJSON, tagsJSON, keyPointsJSON sql.NullString
			enrichedAt, timestamp                 sql.NullTime
			domain                                sql.NullString
			state, createdBy, sessionID           sql.NullString
			enrichmentError, summary              sql.NullString
			contentHash, supersedesID             sql.NullString
			sourceContextJSON                     sql.NullString
			stateUpdatedAt, lastAccessedAt        sql.NullTime
			decayUpdatedAt, deletedAt             sql.NullTime
			classificationStatus                  sql.NullString
			summarizationStatus                   sql.NullString
		)

		if err := rows.Scan(
			&mem.ID, &mem.Content, &mem.Source, &domain, &timestamp, &mem.Status,
			&mem.EntityStatus, &mem.RelationshipStatus, &mem.EmbeddingStatus,
			&mem.EnrichmentAttempts, &enrichmentError,
			&mem.CreatedAt, &mem.UpdatedAt, &enrichedAt,
			&metadataJSON, &tagsJSON,
			&summary, &keyPointsJSON,
			&classificationStatus, &summarizationStatus,
			&state, &stateUpdatedAt,
			&createdBy, &sessionID, &sourceContextJSON,
			&mem.AccessCount, &lastAccessedAt, &mem.DecayScore, &decayUpdatedAt, &deletedAt, &contentHash, &supersedesID,
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
		if summary.Valid {
			mem.Summary = summary.String
		}
		if classificationStatus.Valid {
			mem.ClassificationStatus = types.EnrichmentStatus(classificationStatus.String)
		}
		if summarizationStatus.Valid {
			mem.SummarizationStatus = types.EnrichmentStatus(summarizationStatus.String)
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

		memories = append(memories, mem)
	}
	return memories, rows.Err()
}

// buildInClause returns a comma-separated string of n "?" placeholders.
func buildInClause(n int) string {
	if n == 0 {
		return ""
	}
	clause := make([]byte, 0, n*2-1)
	for i := 0; i < n; i++ {
		if i > 0 {
			clause = append(clause, ',')
		}
		clause = append(clause, '?')
	}
	return string(clause)
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

