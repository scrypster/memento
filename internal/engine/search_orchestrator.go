package engine

import (
	"context"
	"slices"
	"strings"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// SearchOrchestrator coordinates memory search operations.
// It provides intelligent search with relevance scoring, pagination,
// and filtering capabilities.
type SearchOrchestrator struct {
	memoryStore    storage.MemoryStore
	searchProvider storage.SearchProvider
}

// NewSearchOrchestrator creates a new search orchestrator.
// If the provided store also implements storage.SearchProvider (e.g.
// *sqlite.MemoryStore), FTS5 full-text search is used automatically.
func NewSearchOrchestrator(store storage.MemoryStore) *SearchOrchestrator {
	o := &SearchOrchestrator{
		memoryStore: store,
	}
	// Prefer FTS5 when available via type assertion.
	if sp, ok := store.(storage.SearchProvider); ok {
		o.searchProvider = sp
	}
	return o
}

// SearchOptions configures search behavior.
type SearchOptions struct {
	// Query is the search query string.
	Query string

	// Limit is the maximum number of results to return.
	Limit int

	// Offset is the number of results to skip (for pagination).
	Offset int

	// Domain filters results by domain (optional).
	Domain string

	// MinScore is the minimum relevance score (0.0 to 1.0).
	MinScore float64

	// IncludeOld includes old/decayed memories in results.
	IncludeOld bool
}

// SearchResult represents a memory with relevance score and reasoning.
type SearchResult struct {
	// Memory is the matched memory.
	Memory *types.Memory

	// Score is the overall relevance score (0.0 to 1.0).
	Score float64

	// Reason explains why this memory was matched.
	Reason string

	// Components breaks down the score into individual factors.
	Components ScoreComponents
}

// ScoreComponents breaks down relevance score into individual factors.
type ScoreComponents struct {
	// TextMatch is the text matching score (0.0 to 1.0).
	TextMatch float64

	// Recency is the recency score based on creation time (0.0 to 1.0).
	Recency float64

	// Importance is the memory's importance score (0.0 to 1.0).
	Importance float64

	// Confidence is the memory's confidence score (0.0 to 1.0).
	Confidence float64

	// UsageBoost is the boost from recent access (0.0 to 1.0).
	UsageBoost float64
}

// Search performs intelligent memory search with relevance scoring.
//
// When a SearchProvider is available (e.g. the SQLite FTS5 backend), it
// delegates to FullTextSearch for ranking-aware retrieval before applying the
// weighted scoring overlay.  Otherwise it falls back to the original
// list-then-filter in-memory approach.
func (s *SearchOrchestrator) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	// Normalize options
	if opts.Limit <= 0 {
		opts.Limit = 10
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}

	var candidates []SearchResult

	// ------------------------------------------------------------------
	// FTS5 path — use the SearchProvider when available.
	// ------------------------------------------------------------------
	if s.searchProvider != nil && opts.Query != "" {
		searchOpts := storage.SearchOptions{
			Query:         opts.Query,
			Limit:         opts.Limit + opts.Offset, // fetch enough for pagination
			Offset:        0,
			FuzzyFallback: true,
		}
		ftsResult, err := s.searchProvider.FullTextSearch(ctx, searchOpts)
		if err != nil {
			return nil, err
		}

		queryLower := strings.ToLower(opts.Query)
		for i := range ftsResult.Items {
			memory := &ftsResult.Items[i]

			// Domain post-filter (FTS5 searches content only).
			if opts.Domain != "" && memory.Domain != opts.Domain {
				continue
			}

			score, components := s.calculateRelevance(memory, queryLower)
			if score < opts.MinScore {
				continue
			}

			candidates = append(candidates, SearchResult{
				Memory:     memory,
				Score:      score,
				Components: components,
				Reason:     s.buildReason(memory, queryLower, components),
			})
		}
	} else {
		// ------------------------------------------------------------------
		// Fallback: list-then-filter in memory (no FTS5 or empty query).
		// ------------------------------------------------------------------
		filter := make(map[string]interface{})
		if opts.Domain != "" {
			filter["domain"] = opts.Domain
		}

		fetchLimit := (opts.Offset + opts.Limit) * 2
		if fetchLimit < 100 {
			fetchLimit = 100
		}
		listOpts := storage.ListOptions{
			Page:      1,
			Limit:     fetchLimit,
			SortBy:    "created_at",
			SortOrder: "desc",
			Filter:    filter,
		}

		result, err := s.memoryStore.List(ctx, listOpts)
		if err != nil {
			return nil, err
		}

		queryLower := strings.ToLower(opts.Query)
		for i := range result.Items {
			memory := &result.Items[i]
			score, components := s.calculateRelevance(memory, queryLower)
			if score < opts.MinScore {
				continue
			}
			candidates = append(candidates, SearchResult{
				Memory:     memory,
				Score:      score,
				Components: components,
				Reason:     s.buildReason(memory, queryLower, components),
			})
		}
	}

	// Sort by score descending.
	slices.SortFunc(candidates, func(a, b SearchResult) int {
		if a.Score > b.Score {
			return -1
		}
		if a.Score < b.Score {
			return 1
		}
		return 0
	})

	// Apply pagination.
	start := 0
	if opts.Offset < len(candidates) {
		start = opts.Offset
	}

	end := start + opts.Limit
	if end > len(candidates) {
		end = len(candidates)
	}

	if start >= len(candidates) {
		return []SearchResult{}, nil
	}

	return candidates[start:end], nil
}

// SearchSimilar finds memories similar to a given memory.
// It uses entity overlap and tag similarity to find related memories.
func (s *SearchOrchestrator) SearchSimilar(ctx context.Context, memoryID string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	// Get the source memory
	memory, err := s.memoryStore.Get(ctx, memoryID)
	if err != nil {
		return nil, err
	}

	// Build a search query from memory's tags and entities
	var queryParts []string
	queryParts = append(queryParts, memory.Tags...)
	// Add simplified entity names (remove prefixes)
	for _, entity := range memory.Entities {
		parts := strings.Split(entity, ":")
		if len(parts) > 0 {
			queryParts = append(queryParts, parts[len(parts)-1])
		}
	}

	query := strings.Join(queryParts, " ")

	// Search using the constructed query
	opts := SearchOptions{
		Query:  query,
		Limit:  limit + 1, // +1 to account for source memory
		Offset: 0,
	}

	results, err := s.Search(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Filter out the source memory itself
	var filtered []SearchResult
	for _, result := range results {
		if result.Memory.ID != memoryID {
			filtered = append(filtered, result)
		}
	}

	// Truncate to limit
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered, nil
}

// calculateRelevance calculates relevance score using weighted factors.
// Weights: TextMatch=0.4, Recency=0.2, Importance=0.3, Confidence=0.1
func (s *SearchOrchestrator) calculateRelevance(memory *types.Memory, queryLower string) (float64, ScoreComponents) {
	components := ScoreComponents{}

	// Text match score (0.0 to 1.0)
	components.TextMatch = s.calculateTextMatch(memory, queryLower)

	// Recency score (0.0 to 1.0) - newer is better
	components.Recency = s.calculateRecency(memory)

	// Importance score (already 0.0 to 1.0, default to 0.5 if not set)
	components.Importance = memory.Importance
	if components.Importance == 0 {
		components.Importance = 0.5
	}

	// Confidence score (default 0.8 for enriched memories)
	components.Confidence = 0.8
	if memory.Status != types.StatusEnriched {
		components.Confidence = 0.5
	}

	// Usage boost (currently not tracked, default to 0)
	components.UsageBoost = 0.0

	// Weighted combination
	score := (components.TextMatch * 0.4) +
		(components.Recency * 0.2) +
		(components.Importance * 0.3) +
		(components.Confidence * 0.1)

	return score, components
}

// calculateTextMatch calculates text matching score.
func (s *SearchOrchestrator) calculateTextMatch(memory *types.Memory, queryLower string) float64 {
	if queryLower == "" {
		return 1.0 // Empty query matches everything
	}

	contentLower := strings.ToLower(memory.Content)
	score := 0.0

	// Exact phrase match
	if strings.Contains(contentLower, queryLower) {
		score = 1.0
	} else {
		// Partial word match
		queryWords := strings.Fields(queryLower)
		matchedWords := 0
		for _, word := range queryWords {
			if strings.Contains(contentLower, word) {
				matchedWords++
			}
		}
		if len(queryWords) > 0 {
			score = float64(matchedWords) / float64(len(queryWords))
		}
	}

	// Boost for tag matches
	for _, tag := range memory.Tags {
		if strings.Contains(strings.ToLower(tag), queryLower) {
			score = min(1.0, score+0.2)
		}
	}

	// Boost for entity matches
	for _, entity := range memory.Entities {
		if strings.Contains(strings.ToLower(entity), queryLower) {
			score = min(1.0, score+0.1)
		}
	}

	return score
}

// calculateRecency calculates recency score (0.0 to 1.0).
// Memories from the last 30 days get full score, older ones decay.
func (s *SearchOrchestrator) calculateRecency(memory *types.Memory) float64 {
	// TODO: Use actual time-based decay
	// For now, return a simple score based on status
	if memory.Status == types.StatusEnriched {
		return 0.8
	}
	return 0.5
}

// buildReason constructs a human-readable explanation for the match.
func (s *SearchOrchestrator) buildReason(memory *types.Memory, queryLower string, components ScoreComponents) string {
	var reasons []string

	if components.TextMatch > 0.8 {
		reasons = append(reasons, "strong text match")
	} else if components.TextMatch > 0.5 {
		reasons = append(reasons, "partial text match")
	}

	if components.Importance > 0.7 {
		reasons = append(reasons, "high importance")
	}

	if components.Recency > 0.8 {
		reasons = append(reasons, "recent")
	}

	// Check for tag matches
	for _, tag := range memory.Tags {
		if strings.Contains(strings.ToLower(tag), queryLower) {
			reasons = append(reasons, "tag match")
			break
		}
	}

	if len(reasons) == 0 {
		return "matched content"
	}

	return strings.Join(reasons, ", ")
}

// min returns the minimum of two float64 values.
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
