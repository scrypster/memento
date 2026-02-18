package engine

import (
	"context"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TraceEvent constructors
// ---------------------------------------------------------------------------

func TestEventSearchStarted(t *testing.T) {
	filters := map[string]string{"domain": "work", "min_score": "0.50"}
	e := EventSearchStarted("project goals", filters)

	if e.Kind != KindSearchStarted {
		t.Errorf("Kind: got %q, want %q", e.Kind, KindSearchStarted)
	}
	if e.Query != "project goals" {
		t.Errorf("Query: got %q, want %q", e.Query, "project goals")
	}
	if e.Filters["domain"] != "work" {
		t.Errorf("Filters[domain]: got %q, want %q", e.Filters["domain"], "work")
	}
	if e.At.IsZero() {
		t.Error("At should not be zero")
	}
}

func TestEventCandidatesFound(t *testing.T) {
	e := EventCandidatesFound(42, "fts5")
	if e.Kind != KindCandidatesFound {
		t.Errorf("Kind: got %q, want %q", e.Kind, KindCandidatesFound)
	}
	if e.Count != 42 {
		t.Errorf("Count: got %d, want %d", e.Count, 42)
	}
	if e.Source != "fts5" {
		t.Errorf("Source: got %q, want %q", e.Source, "fts5")
	}
}

func TestEventScoredCandidate(t *testing.T) {
	components := ScoreComponents{
		TextMatch:  0.9,
		Recency:    0.8,
		Importance: 0.7,
		Confidence: 0.6,
		UsageBoost: 0.0,
	}
	e := EventScoredCandidate("mem:work:abc", components, 0.82)

	if e.Kind != KindScoredCandidate {
		t.Errorf("Kind: got %q, want %q", e.Kind, KindScoredCandidate)
	}
	if e.MemoryID != "mem:work:abc" {
		t.Errorf("MemoryID: got %q", e.MemoryID)
	}
	if e.TotalScore != 0.82 {
		t.Errorf("TotalScore: got %f, want %f", e.TotalScore, 0.82)
	}
	if e.Scores == nil {
		t.Fatal("Scores must not be nil")
	}
	if e.Scores.TextMatch != 0.9 {
		t.Errorf("Scores.TextMatch: got %f, want %f", e.Scores.TextMatch, 0.9)
	}
}

func TestEventFilteredOut(t *testing.T) {
	e := EventFilteredOut("mem:default:xyz", "score 0.12 below min_score 0.50")
	if e.Kind != KindFilteredOut {
		t.Errorf("Kind: got %q, want %q", e.Kind, KindFilteredOut)
	}
	if e.MemoryID != "mem:default:xyz" {
		t.Errorf("MemoryID: got %q", e.MemoryID)
	}
	if e.FilterReason == "" {
		t.Error("FilterReason must not be empty")
	}
}

func TestEventResultsReturned(t *testing.T) {
	ids := []string{"mem:a:1", "mem:a:2"}
	e := EventResultsReturned(ids)
	if e.Kind != KindResultsReturned {
		t.Errorf("Kind: got %q, want %q", e.Kind, KindResultsReturned)
	}
	if e.Count != 2 {
		t.Errorf("Count: got %d, want %d", e.Count, 2)
	}
	if len(e.MemoryIDs) != 2 {
		t.Errorf("MemoryIDs len: got %d, want %d", len(e.MemoryIDs), 2)
	}
}

// ---------------------------------------------------------------------------
// TraceCollector
// ---------------------------------------------------------------------------

func TestTraceCollector_EmitAndRetrieve(t *testing.T) {
	tc := NewTraceCollector()
	if len(tc.Events()) != 0 {
		t.Fatal("new collector should have no events")
	}

	tc.Emit(EventSearchStarted("hello", nil))
	tc.Emit(EventCandidatesFound(5, "list"))

	events := tc.Events()
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
	if events[0].Kind != KindSearchStarted {
		t.Errorf("first event: got %q", events[0].Kind)
	}
	if events[1].Kind != KindCandidatesFound {
		t.Errorf("second event: got %q", events[1].Kind)
	}
}

func TestTraceCollector_ElapsedMS(t *testing.T) {
	tc := NewTraceCollector()
	time.Sleep(5 * time.Millisecond)
	elapsed := tc.ElapsedMS()
	if elapsed < 1 {
		t.Errorf("ElapsedMS expected >= 1, got %d", elapsed)
	}
}

// ---------------------------------------------------------------------------
// Context round-trip
// ---------------------------------------------------------------------------

func TestWithTraceCollector_RoundTrip(t *testing.T) {
	tc := NewTraceCollector()
	ctx := WithTraceCollector(context.Background(), tc)

	got, ok := TraceCollectorFromContext(ctx)
	if !ok {
		t.Fatal("TraceCollectorFromContext returned false")
	}
	if got != tc {
		t.Error("retrieved collector is not the same instance")
	}
}

func TestTraceCollectorFromContext_Missing(t *testing.T) {
	_, ok := TraceCollectorFromContext(context.Background())
	if ok {
		t.Error("expected false for context without collector")
	}
}

func TestEmitToContext_NoCollector(t *testing.T) {
	// emitToContext must not panic when no collector is present.
	emitToContext(context.Background(), EventSearchStarted("test", nil))
}

func TestEmitToContext_WithCollector(t *testing.T) {
	tc := NewTraceCollector()
	ctx := WithTraceCollector(context.Background(), tc)
	emitToContext(ctx, EventSearchStarted("test", nil))

	if len(tc.Events()) != 1 {
		t.Errorf("expected 1 event, got %d", len(tc.Events()))
	}
}

// ---------------------------------------------------------------------------
// BuildDebugResult
// ---------------------------------------------------------------------------

func TestBuildDebugResult_Empty(t *testing.T) {
	result := BuildDebugResult(nil, 10)
	if result.TimingMS != 10 {
		t.Errorf("TimingMS: got %d", result.TimingMS)
	}
	if result.ScoredResults == nil {
		t.Error("ScoredResults must be non-nil slice")
	}
	if result.FilteredOut == nil {
		t.Error("FilteredOut must be non-nil slice")
	}
	if result.Returned == nil {
		t.Error("Returned must be non-nil slice")
	}
}

func TestBuildDebugResult_FullFlow(t *testing.T) {
	events := []TraceEvent{
		EventSearchStarted("goals", map[string]string{"domain": "work"}),
		EventCandidatesFound(3, "fts5"),
		EventScoredCandidate("mem:work:a", ScoreComponents{TextMatch: 0.9, Recency: 0.8, Importance: 0.7, Confidence: 0.6}, 0.78),
		EventScoredCandidate("mem:work:b", ScoreComponents{TextMatch: 0.5, Recency: 0.5, Importance: 0.5, Confidence: 0.5}, 0.50),
		EventFilteredOut("mem:work:c", "score 0.20 below min_score 0.40"),
		EventResultsReturned([]string{"mem:work:a", "mem:work:b"}),
	}

	result := BuildDebugResult(events, 42)

	if result.TimingMS != 42 {
		t.Errorf("TimingMS: got %d", result.TimingMS)
	}
	if result.QueryParams["query"] != "goals" {
		t.Errorf("QueryParams[query]: got %q", result.QueryParams["query"])
	}
	if result.QueryParams["domain"] != "work" {
		t.Errorf("QueryParams[domain]: got %q", result.QueryParams["domain"])
	}
	if result.CandidatesFound != 3 {
		t.Errorf("CandidatesFound: got %d, want %d", result.CandidatesFound, 3)
	}
	if len(result.ScoredResults) != 2 {
		t.Errorf("ScoredResults len: got %d, want %d", len(result.ScoredResults), 2)
	}
	if result.ScoredResults[0].MemoryID != "mem:work:a" {
		t.Errorf("first ScoredResult MemoryID: got %q", result.ScoredResults[0].MemoryID)
	}
	if result.ScoredResults[0].Scores.TextMatch != 0.9 {
		t.Errorf("TextMatch: got %f", result.ScoredResults[0].Scores.TextMatch)
	}
	if len(result.FilteredOut) != 1 {
		t.Errorf("FilteredOut len: got %d, want %d", len(result.FilteredOut), 1)
	}
	if result.FilteredOut[0].MemoryID != "mem:work:c" {
		t.Errorf("FilteredOut MemoryID: got %q", result.FilteredOut[0].MemoryID)
	}
	if len(result.Returned) != 2 {
		t.Errorf("Returned len: got %d, want %d", len(result.Returned), 2)
	}
}

func TestBuildDebugResult_MultipleCandidateFoundEvents(t *testing.T) {
	// Two candidate-found events (e.g. FTS5 + vector) should accumulate.
	events := []TraceEvent{
		EventCandidatesFound(10, "fts5"),
		EventCandidatesFound(5, "vector"),
	}
	result := BuildDebugResult(events, 0)
	if result.CandidatesFound != 15 {
		t.Errorf("CandidatesFound: got %d, want %d", result.CandidatesFound, 15)
	}
}
