package handlers

import (
	"net/http"
	"strconv"

	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/storage"
)

// DebugHandler exposes memory-retrieval debug endpoints.
type DebugHandler struct {
	orchestrator    *engine.DebugOrchestrator
	templateBasePath string
}

// NewDebugHandler creates a DebugHandler backed by the given store.
// templateBasePath should be the project root (same value returned by findBasePath
// in server.go) so that debug.html can be served correctly.
func NewDebugHandler(store storage.MemoryStore, templateBasePath string) *DebugHandler {
	return &DebugHandler{
		orchestrator:    engine.NewDebugOrchestrator(store),
		templateBasePath: templateBasePath,
	}
}

// RecallTrace handles GET /api/debug/recall-trace
//
// Query parameters:
//   - query    (string) – the search query (may be empty)
//   - domain   (string) – optional domain filter
//   - state    (string) – alias for domain kept for spec compatibility
//   - limit    (int)    – max results per page (default 10, max 100)
//   - offset   (int)    – pagination offset (default 0)
//   - min_score (float) – minimum relevance score 0.0–1.0 (default 0.0)
func (h *DebugHandler) RecallTrace(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	query := q.Get("query")
	if query == "" {
		// Allow "q" as an alias used by the search page.
		query = q.Get("q")
	}

	// Domain / state filter.
	domain := q.Get("domain")
	if domain == "" {
		domain = q.Get("state") // spec names this param "state"
	}

	limit := 10
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	minScore := 0.0
	if v := q.Get("min_score"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			minScore = f
		}
	}

	opts := engine.SearchOptions{
		Query:    query,
		Domain:   domain,
		Limit:    limit,
		Offset:   offset,
		MinScore: minScore,
	}

	result, err := h.orchestrator.DebugSearch(r.Context(), opts)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "debug search failed", err)
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// DebugPage serves the HTML debug dashboard.
func (h *DebugHandler) DebugPage(w http.ResponseWriter, r *http.Request) {
	path := h.templateBasePath + "/web/templates/debug.html"
	http.ServeFile(w, r, path)
}
