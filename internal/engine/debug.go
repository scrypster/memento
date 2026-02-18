package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/scrypster/memento/internal/storage"
)

// contextKey is an unexported type for context keys owned by this package.
type contextKey string

const traceKey contextKey = "recall_trace"

// TraceCollector accumulates TraceEvents for a single search operation.
type TraceCollector struct {
	events    []TraceEvent
	startedAt time.Time
}

// NewTraceCollector returns a fresh collector.
func NewTraceCollector() *TraceCollector {
	return &TraceCollector{startedAt: time.Now()}
}

// Emit appends an event to the collector.
func (tc *TraceCollector) Emit(e TraceEvent) {
	tc.events = append(tc.events, e)
}

// Events returns the collected events in emission order.
func (tc *TraceCollector) Events() []TraceEvent {
	return tc.events
}

// ElapsedMS returns the elapsed time since the collector was created, in milliseconds.
func (tc *TraceCollector) ElapsedMS() int64 {
	return time.Since(tc.startedAt).Milliseconds()
}

// WithTraceCollector stores a collector in the context.
func WithTraceCollector(ctx context.Context, tc *TraceCollector) context.Context {
	return context.WithValue(ctx, traceKey, tc)
}

// TraceCollectorFromContext retrieves the collector from the context.
// Returns (nil, false) if none is present.
func TraceCollectorFromContext(ctx context.Context) (*TraceCollector, bool) {
	tc, ok := ctx.Value(traceKey).(*TraceCollector)
	return tc, ok
}

// emitToContext is a helper used by the engine components to emit an event only
// when a collector is present in the context.
func emitToContext(ctx context.Context, e TraceEvent) {
	if tc, ok := TraceCollectorFromContext(ctx); ok {
		tc.Emit(e)
	}
}

// DebugRecallResult is the structured response returned by the debug endpoint.
type DebugRecallResult struct {
	// QueryParams mirrors the search options that were used.
	QueryParams map[string]string `json:"query_params"`

	// CandidatesFound is the total number of candidates retrieved from storage
	// before scoring and filtering.
	CandidatesFound int `json:"candidates_found"`

	// ScoredResults contains every candidate that passed the min-score filter.
	ScoredResults []ScoredEntry `json:"scored_results"`

	// FilteredOut contains every candidate that was discarded and why.
	FilteredOut []FilteredEntry `json:"filtered_out"`

	// Returned lists the IDs that appear in the final paginated result.
	Returned []string `json:"returned"`

	// TimingMS is the total search duration in milliseconds.
	TimingMS int64 `json:"timing_ms"`
}

// ScoredEntry represents a candidate that passed initial scoring.
type ScoredEntry struct {
	MemoryID string      `json:"memory_id"`
	Scores   TraceScores `json:"scores"`
	Total    float64     `json:"total"`
}

// FilteredEntry represents a candidate that was discarded.
type FilteredEntry struct {
	MemoryID string `json:"memory_id"`
	Reason   string `json:"reason"`
}

// BuildDebugResult converts collected trace events into a DebugRecallResult.
func BuildDebugResult(events []TraceEvent, elapsedMS int64) *DebugRecallResult {
	result := &DebugRecallResult{
		QueryParams: make(map[string]string),
		TimingMS:    elapsedMS,
	}

	for _, e := range events {
		switch e.Kind {
		case KindSearchStarted:
			result.QueryParams["query"] = e.Query
			for k, v := range e.Filters {
				result.QueryParams[k] = v
			}
		case KindCandidatesFound:
			result.CandidatesFound += e.Count
		case KindScoredCandidate:
			if e.Scores != nil {
				result.ScoredResults = append(result.ScoredResults, ScoredEntry{
					MemoryID: e.MemoryID,
					Scores:   *e.Scores,
					Total:    e.TotalScore,
				})
			}
		case KindFilteredOut:
			result.FilteredOut = append(result.FilteredOut, FilteredEntry{
				MemoryID: e.MemoryID,
				Reason:   e.FilterReason,
			})
		case KindResultsReturned:
			result.Returned = e.MemoryIDs
		}
	}

	// Guarantee non-nil slices for clean JSON output.
	if result.ScoredResults == nil {
		result.ScoredResults = []ScoredEntry{}
	}
	if result.FilteredOut == nil {
		result.FilteredOut = []FilteredEntry{}
	}
	if result.Returned == nil {
		result.Returned = []string{}
	}

	return result
}

// DebugOrchestrator wraps SearchOrchestrator and exposes instrumented search
// that emits trace events and returns a DebugRecallResult.
type DebugOrchestrator struct {
	inner *SearchOrchestrator
}

// NewDebugOrchestrator creates a DebugOrchestrator backed by the given store.
func NewDebugOrchestrator(store storage.MemoryStore) *DebugOrchestrator {
	return &DebugOrchestrator{inner: NewSearchOrchestrator(store)}
}

// DebugSearch runs a fully-traced search and returns both the search results
// and the collected debug information.
func (d *DebugOrchestrator) DebugSearch(ctx context.Context, opts SearchOptions) (*DebugRecallResult, error) {
	tc := NewTraceCollector()
	ctx = WithTraceCollector(ctx, tc)

	// Emit search_started.
	filters := map[string]string{}
	if opts.Domain != "" {
		filters["domain"] = opts.Domain
	}
	if opts.MinScore > 0 {
		filters["min_score"] = fmt.Sprintf("%.2f", opts.MinScore)
	}
	tc.Emit(EventSearchStarted(opts.Query, filters))

	// Run the instrumented search.
	_, err := d.inner.SearchWithTrace(ctx, opts)
	if err != nil {
		return nil, err
	}

	return BuildDebugResult(tc.Events(), tc.ElapsedMS()), nil
}

// SearchWithTrace is identical to Search but emits trace events into the
// TraceCollector stored in ctx (if one is present).
func (s *SearchOrchestrator) SearchWithTrace(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	// Normalize options.
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

	if s.searchProvider != nil && opts.Query != "" {
		searchOpts := storage.SearchOptions{
			Query:         opts.Query,
			Limit:         opts.Limit + opts.Offset,
			Offset:        0,
			FuzzyFallback: true,
		}
		ftsResult, err := s.searchProvider.FullTextSearch(ctx, searchOpts)
		if err != nil {
			return nil, err
		}

		emitToContext(ctx, EventCandidatesFound(len(ftsResult.Items), "fts5"))

		queryLower := strings.ToLower(opts.Query)
		for i := range ftsResult.Items {
			memory := &ftsResult.Items[i]

			if opts.Domain != "" && memory.Domain != opts.Domain {
				emitToContext(ctx, EventFilteredOut(memory.ID,
					fmt.Sprintf("domain mismatch: got %q, want %q", memory.Domain, opts.Domain)))
				continue
			}

			score, components := s.calculateRelevance(memory, queryLower)
			emitToContext(ctx, EventScoredCandidate(memory.ID, components, score))

			if score < opts.MinScore {
				emitToContext(ctx, EventFilteredOut(memory.ID,
					fmt.Sprintf("score %.4f below min_score %.4f", score, opts.MinScore)))
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

		emitToContext(ctx, EventCandidatesFound(len(result.Items), "list"))

		queryLower := strings.ToLower(opts.Query)
		for i := range result.Items {
			memory := &result.Items[i]
			score, components := s.calculateRelevance(memory, queryLower)
			emitToContext(ctx, EventScoredCandidate(memory.ID, components, score))

			if score < opts.MinScore {
				emitToContext(ctx, EventFilteredOut(memory.ID,
					fmt.Sprintf("score %.4f below min_score %.4f", score, opts.MinScore)))
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
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Score > candidates[i].Score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Apply pagination.
	start := 0
	if opts.Offset < len(candidates) {
		start = opts.Offset
	}
	end := start + opts.Limit
	if end > len(candidates) {
		end = len(candidates)
	}

	var page []SearchResult
	if start < len(candidates) {
		page = candidates[start:end]
	} else {
		page = []SearchResult{}
	}

	// Emit results_returned.
	returnedIDs := make([]string, len(page))
	for i, r := range page {
		returnedIDs[i] = r.Memory.ID
	}
	emitToContext(ctx, EventResultsReturned(returnedIDs))

	return page, nil
}
