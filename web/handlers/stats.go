package handlers

import (
	"net/http"

	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
)

// QueueSizeGetter defines the interface for getting queue size.
type QueueSizeGetter interface {
	GetQueueSize() int
}

// StatsHandler handles statistics endpoint requests.
type StatsHandler struct {
	store              storage.MemoryStore
	connectionManager  *connections.Manager
	queueGetter        QueueSizeGetter
}

// NewStatsHandler creates a new StatsHandler instance.
func NewStatsHandler(store storage.MemoryStore, connectionManager *connections.Manager, queueGetter QueueSizeGetter) *StatsHandler {
	return &StatsHandler{
		store:             store,
		connectionManager: connectionManager,
		queueGetter:       queueGetter,
	}
}

// GetStats handles GET /api/stats - returns system statistics.
func (h *StatsHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract connection parameter (query param or X-Connection-ID header)
	connectionName := r.URL.Query().Get("connection")
	if connectionName == "" {
		connectionName = r.Header.Get("X-Connection-ID")
	}
	store, err := h.connectionManager.GetStore(connectionName)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid connection", err)
		return
	}

	// Get total memories count
	memories, err := store.List(ctx, storage.ListOptions{Limit: 1})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count memories", err)
		return
	}

	// Get entities and relationships counts from database
	entities := 0
	relationships := 0

	// Access database directly if using SQLite store
	if sqliteStore, ok := store.(*sqlite.MemoryStore); ok {
		db := sqliteStore.GetDB()

		// Count entities
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM entities").Scan(&entities); err != nil {
			// Log error but don't fail the request
			// respondError(w, http.StatusInternalServerError, "failed to count entities", err)
			// return
		}

		// Count relationships
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM relationships").Scan(&relationships); err != nil {
			// Log error but don't fail the request
		}
	}

	// Get queue size from engine
	queueSize := 0
	if h.queueGetter != nil {
		queueSize = h.queueGetter.GetQueueSize()
	}

	stats := StatsResponse{
		Memories:      memories.Total,
		Entities:      entities,
		Relationships: relationships,
		QueueSize:     queueSize,
	}

	respondJSON(w, http.StatusOK, stats)
}
