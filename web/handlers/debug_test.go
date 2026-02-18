package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// ---------------------------------------------------------------------------
// In-memory stub for storage.MemoryStore
// ---------------------------------------------------------------------------

type stubStore struct {
	memories []types.Memory
}

func (s *stubStore) Store(_ context.Context, m *types.Memory) error {
	s.memories = append(s.memories, *m)
	return nil
}

func (s *stubStore) Get(_ context.Context, id string) (*types.Memory, error) {
	for i := range s.memories {
		if s.memories[i].ID == id {
			return &s.memories[i], nil
		}
	}
	return nil, storage.ErrNotFound
}

func (s *stubStore) List(_ context.Context, _ storage.ListOptions) (*storage.PaginatedResult[types.Memory], error) {
	return &storage.PaginatedResult[types.Memory]{Items: s.memories, Total: len(s.memories)}, nil
}

func (s *stubStore) Update(_ context.Context, m *types.Memory) error {
	for i := range s.memories {
		if s.memories[i].ID == m.ID {
			s.memories[i] = *m
			return nil
		}
	}
	return storage.ErrNotFound
}

func (s *stubStore) Delete(_ context.Context, id string) error {
	for i, m := range s.memories {
		if m.ID == id {
			s.memories = append(s.memories[:i], s.memories[i+1:]...)
			return nil
		}
	}
	return storage.ErrNotFound
}

func (s *stubStore) Purge(_ context.Context, id string) error {
	for i, m := range s.memories {
		if m.ID == id {
			s.memories = append(s.memories[:i], s.memories[i+1:]...)
			return nil
		}
	}
	return storage.ErrNotFound
}

func (s *stubStore) UpdateStatus(_ context.Context, _ string, _ types.MemoryStatus) error {
	return nil
}

func (s *stubStore) UpdateEnrichment(_ context.Context, _ string, _ storage.EnrichmentUpdate) error {
	return nil
}

func (s *stubStore) IncrementAccessCount(_ context.Context, _ string) error {
	return nil
}

func (s *stubStore) UpdateState(_ context.Context, _ string, _ string) error {
	return nil
}

func (s *stubStore) UpdateDecayScores(_ context.Context) (int, error) {
	return 0, nil
}

func (s *stubStore) GetRelatedMemories(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (s *stubStore) Traverse(_ context.Context, _ string, _ int, _ int) ([]storage.TraversalResult, error) {
	return nil, nil
}

func (s *stubStore) GetMemoryEntities(_ context.Context, _ string) ([]*types.Entity, error) {
	return nil, nil
}

func (s *stubStore) Restore(_ context.Context, _ string) error { return nil }

func (s *stubStore) GetEvolutionChain(_ context.Context, _ string) ([]*types.Memory, error) {
	return nil, nil
}

func (s *stubStore) GetMemoriesByRelationType(_ context.Context, _ string, _ string) ([]*types.Memory, error) {
	return nil, nil
}

func (s *stubStore) Close() error {
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newStubMemory(id, content, domain string, importance float64, status types.MemoryStatus) types.Memory {
	return types.Memory{
		ID:         id,
		Content:    content,
		Domain:     domain,
		Source:     "manual",
		Importance: importance,
		Status:     status,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Timestamp:  time.Now(),
	}
}

func newDebugHandlerForTest(store storage.MemoryStore) *DebugHandler {
	return NewDebugHandler(store, ".")
}

// ---------------------------------------------------------------------------
// RecallTrace tests
// ---------------------------------------------------------------------------

func TestRecallTrace_EmptyStore(t *testing.T) {
	store := &stubStore{}
	h := newDebugHandlerForTest(store)

	req := httptest.NewRequest(http.MethodGet, "/api/debug/recall-trace?query=anything", nil)
	w := httptest.NewRecorder()

	h.RecallTrace(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var result engine.DebugRecallResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if result.CandidatesFound != 0 {
		t.Errorf("CandidatesFound: got %d, want 0", result.CandidatesFound)
	}
	if result.ScoredResults == nil {
		t.Error("ScoredResults must not be nil")
	}
	if result.FilteredOut == nil {
		t.Error("FilteredOut must not be nil")
	}
	if result.Returned == nil {
		t.Error("Returned must not be nil")
	}
}

func TestRecallTrace_ReturnsJSON(t *testing.T) {
	store := &stubStore{
		memories: []types.Memory{
			newStubMemory("mem:work:abc", "project planning goals", "work", 0.9, types.StatusEnriched),
			newStubMemory("mem:work:def", "budget review meeting", "work", 0.6, types.StatusEnriched),
		},
	}
	h := newDebugHandlerForTest(store)

	req := httptest.NewRequest(http.MethodGet, "/api/debug/recall-trace?query=planning&limit=10", nil)
	w := httptest.NewRecorder()

	h.RecallTrace(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var result engine.DebugRecallResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if result.CandidatesFound == 0 {
		t.Error("CandidatesFound should be > 0")
	}
	if result.TimingMS < 0 {
		t.Errorf("TimingMS must be non-negative, got %d", result.TimingMS)
	}
}

func TestRecallTrace_QueryParamsMirrored(t *testing.T) {
	store := &stubStore{}
	h := newDebugHandlerForTest(store)

	req := httptest.NewRequest(http.MethodGet, "/api/debug/recall-trace?query=hello&state=personal&min_score=0.5", nil)
	w := httptest.NewRecorder()

	h.RecallTrace(w, req)

	var result engine.DebugRecallResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if result.QueryParams["query"] != "hello" {
		t.Errorf("query_params.query: got %q, want %q", result.QueryParams["query"], "hello")
	}
	if result.QueryParams["domain"] != "personal" {
		t.Errorf("query_params.domain: got %q, want %q", result.QueryParams["domain"], "personal")
	}
	if result.QueryParams["min_score"] != "0.50" {
		t.Errorf("query_params.min_score: got %q, want %q", result.QueryParams["min_score"], "0.50")
	}
}

func TestRecallTrace_MinScoreFiltersResults(t *testing.T) {
	// With min_score=0.99, only perfect-match memories should pass.
	store := &stubStore{
		memories: []types.Memory{
			newStubMemory("mem:x:low", "something unrelated", "default", 0.1, types.StatusPending),
		},
	}
	h := newDebugHandlerForTest(store)

	req := httptest.NewRequest(http.MethodGet, "/api/debug/recall-trace?query=unrelated&min_score=0.99&limit=10", nil)
	w := httptest.NewRecorder()

	h.RecallTrace(w, req)

	var result engine.DebugRecallResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Any returned item must genuinely score >= 0.99.
	for _, se := range result.ScoredResults {
		for _, rid := range result.Returned {
			if se.MemoryID == rid && se.Total < 0.99 {
				t.Errorf("returned memory %q has score %.4f < 0.99", rid, se.Total)
			}
		}
	}
}

func TestRecallTrace_AliasQParam(t *testing.T) {
	// The handler should accept both "query" and "q" params.
	store := &stubStore{}
	h := newDebugHandlerForTest(store)

	req := httptest.NewRequest(http.MethodGet, "/api/debug/recall-trace?q=test", nil)
	w := httptest.NewRecorder()

	h.RecallTrace(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status using 'q' param: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRecallTrace_InvalidLimitIgnored(t *testing.T) {
	store := &stubStore{}
	h := newDebugHandlerForTest(store)

	req := httptest.NewRequest(http.MethodGet, "/api/debug/recall-trace?limit=bad", nil)
	w := httptest.NewRecorder()

	h.RecallTrace(w, req)

	// Should still return 200, defaulting limit to 10.
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRecallTrace_ContentType(t *testing.T) {
	store := &stubStore{}
	h := newDebugHandlerForTest(store)

	req := httptest.NewRequest(http.MethodGet, "/api/debug/recall-trace", nil)
	w := httptest.NewRecorder()

	h.RecallTrace(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

func TestRecallTrace_ScoredResultsHaveScoreComponents(t *testing.T) {
	store := &stubStore{
		memories: []types.Memory{
			newStubMemory("mem:test:1", "integration test memory content", "test", 0.8, types.StatusEnriched),
		},
	}
	h := newDebugHandlerForTest(store)

	req := httptest.NewRequest(http.MethodGet, "/api/debug/recall-trace?query=integration+test&limit=10", nil)
	w := httptest.NewRecorder()

	h.RecallTrace(w, req)

	var result engine.DebugRecallResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(result.ScoredResults) == 0 {
		t.Skip("no scored results - store returned no candidates")
	}

	for _, se := range result.ScoredResults {
		if se.Total <= 0 || se.Total > 1.0 {
			t.Errorf("scored entry %q total score out of range: %f", se.MemoryID, se.Total)
		}
	}
}

func TestRecallTrace_FilteredAndReturnedAreMutuallyExclusive(t *testing.T) {
	store := &stubStore{
		memories: []types.Memory{
			newStubMemory("mem:a:1", "alpha beta gamma content", "alpha", 0.9, types.StatusEnriched),
			newStubMemory("mem:a:2", "delta epsilon content", "alpha", 0.2, types.StatusPending),
		},
	}
	h := newDebugHandlerForTest(store)

	req := httptest.NewRequest(http.MethodGet, "/api/debug/recall-trace?query=alpha&min_score=0.0&limit=10", nil)
	w := httptest.NewRecorder()

	h.RecallTrace(w, req)

	var result engine.DebugRecallResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// No memory ID should appear in both returned and filtered_out.
	filteredSet := make(map[string]bool)
	for _, f := range result.FilteredOut {
		filteredSet[f.MemoryID] = true
	}
	for _, id := range result.Returned {
		if filteredSet[id] {
			t.Errorf("memory %q appears in both returned and filtered_out", id)
		}
	}
}
