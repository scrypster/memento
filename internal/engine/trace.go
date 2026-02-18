package engine

import "time"

// TraceEventKind classifies each trace event by type.
type TraceEventKind string

const (
	// KindSearchStarted is emitted at the beginning of a search.
	KindSearchStarted TraceEventKind = "search_started"

	// KindCandidatesFound is emitted after the initial candidate set is resolved.
	KindCandidatesFound TraceEventKind = "candidates_found"

	// KindScoredCandidate is emitted once per candidate that received scoring.
	KindScoredCandidate TraceEventKind = "scored_candidate"

	// KindFilteredOut is emitted for every candidate that was discarded.
	KindFilteredOut TraceEventKind = "filtered_out"

	// KindResultsReturned is emitted after pagination to record the final set.
	KindResultsReturned TraceEventKind = "results_returned"
)

// TraceEvent is a single structured event emitted during a search operation.
type TraceEvent struct {
	// Kind identifies the event type.
	Kind TraceEventKind `json:"kind"`

	// At is the wall-clock time the event was recorded.
	At time.Time `json:"at"`

	// MemoryID is populated for per-memory events (scored_candidate, filtered_out).
	MemoryID string `json:"memory_id,omitempty"`

	// Source is the retrieval backend that produced candidates ("fts5", "list").
	Source string `json:"source,omitempty"`

	// Count is used by candidates_found and results_returned.
	Count int `json:"count,omitempty"`

	// Scores holds per-component relevance data for scored_candidate events.
	Scores *TraceScores `json:"scores,omitempty"`

	// TotalScore is the combined relevance score for scored_candidate events.
	TotalScore float64 `json:"total_score,omitempty"`

	// FilterReason is a human-readable explanation for filtered_out events.
	FilterReason string `json:"filter_reason,omitempty"`

	// Query is the original search query, populated in search_started.
	Query string `json:"query,omitempty"`

	// Filters captures the active filter options for search_started events.
	Filters map[string]string `json:"filters,omitempty"`

	// MemoryIDs lists all returned IDs for results_returned events.
	MemoryIDs []string `json:"memory_ids,omitempty"`
}

// TraceScores breaks down a scored candidate's component scores.
type TraceScores struct {
	TextMatch  float64 `json:"text_match"`
	Recency    float64 `json:"recency"`
	Importance float64 `json:"importance"`
	Confidence float64 `json:"confidence"`
	UsageBoost float64 `json:"usage_boost"`
}

// newTraceEvent is a convenience constructor that timestamps the event.
func newTraceEvent(kind TraceEventKind) TraceEvent {
	return TraceEvent{Kind: kind, At: time.Now()}
}

// EventSearchStarted creates a search_started trace event.
func EventSearchStarted(query string, filters map[string]string) TraceEvent {
	e := newTraceEvent(KindSearchStarted)
	e.Query = query
	e.Filters = filters
	return e
}

// EventCandidatesFound creates a candidates_found trace event.
func EventCandidatesFound(count int, source string) TraceEvent {
	e := newTraceEvent(KindCandidatesFound)
	e.Count = count
	e.Source = source
	return e
}

// EventScoredCandidate creates a scored_candidate trace event.
func EventScoredCandidate(memoryID string, components ScoreComponents, total float64) TraceEvent {
	e := newTraceEvent(KindScoredCandidate)
	e.MemoryID = memoryID
	e.TotalScore = total
	e.Scores = &TraceScores{
		TextMatch:  components.TextMatch,
		Recency:    components.Recency,
		Importance: components.Importance,
		Confidence: components.Confidence,
		UsageBoost: components.UsageBoost,
	}
	return e
}

// EventFilteredOut creates a filtered_out trace event.
func EventFilteredOut(memoryID, reason string) TraceEvent {
	e := newTraceEvent(KindFilteredOut)
	e.MemoryID = memoryID
	e.FilterReason = reason
	return e
}

// EventResultsReturned creates a results_returned trace event.
func EventResultsReturned(memoryIDs []string) TraceEvent {
	e := newTraceEvent(KindResultsReturned)
	e.MemoryIDs = memoryIDs
	e.Count = len(memoryIDs)
	return e
}
