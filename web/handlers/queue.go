package handlers

import (
	"net/http"
	"time"

	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/storage/sqlite"
)

// QueueHandler handles the enrichment queue monitoring endpoint.
type QueueHandler struct {
	connectionManager *connections.Manager
	queueGetter       QueueSizeGetter
}

// NewQueueHandler creates a new QueueHandler instance.
func NewQueueHandler(connectionManager *connections.Manager, queueGetter QueueSizeGetter) *QueueHandler {
	return &QueueHandler{
		connectionManager: connectionManager,
		queueGetter:       queueGetter,
	}
}

// QueueStatsResponse contains enrichment queue statistics.
type QueueStatsResponse struct {
	// Live channel depth (jobs buffered in the in-memory Go channel)
	ChannelDepth int `json:"channel_depth"`

	// DB-backed status counts
	Pending    int `json:"pending"`
	Processing int `json:"processing"`
	Enriched   int `json:"enriched"`
	Failed     int `json:"failed"`

	// Enriched today (UTC day)
	CompletedToday int `json:"completed_today"`
}

// QueueItemResponse represents a single pending/processing memory in the queue.
type QueueItemResponse struct {
	ID                 string     `json:"id"`
	Content            string     `json:"content"`
	Status             string     `json:"status"`
	EnrichmentAttempts int        `json:"enrichment_attempts"`
	EnrichmentError    string     `json:"enrichment_error,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	EnrichedAt         *time.Time `json:"enriched_at,omitempty"`
}

// QueueResponse is the full response for GET /api/queue.
type QueueResponse struct {
	Stats QueueStatsResponse  `json:"stats"`
	Items []QueueItemResponse `json:"items"`
}

// GetQueue handles GET /api/queue - returns enrichment queue stats and pending items.
func (h *QueueHandler) GetQueue(w http.ResponseWriter, r *http.Request) {
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

	// Live channel depth from the in-memory engine queue
	channelDepth := 0
	if h.queueGetter != nil {
		channelDepth = h.queueGetter.GetQueueSize()
	}

	stats := QueueStatsResponse{
		ChannelDepth: channelDepth,
	}
	var items []QueueItemResponse

	// Access database directly if using SQLite store for richer queue data
	if sqliteStore, ok := store.(*sqlite.MemoryStore); ok {
		db := sqliteStore.GetDB()

		// Count by enrichment status
		statusQuery := `
			SELECT status, COUNT(*)
			FROM memories
			GROUP BY status
		`
		rows, err := db.QueryContext(ctx, statusQuery)
		if err == nil {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var status string
				var count int
				if err := rows.Scan(&status, &count); err != nil {
					continue
				}
				switch status {
				case "pending":
					stats.Pending = count
				case "processing":
					stats.Processing = count
				case "enriched":
					stats.Enriched = count
				case "failed":
					stats.Failed = count
				}
			}
		}

		// Count enriched today (UTC day boundary)
		todayQuery := `
			SELECT COUNT(*) FROM memories
			WHERE status = 'enriched'
			AND enriched_at >= date('now', 'start of day')
		`
		_ = db.QueryRowContext(ctx, todayQuery).Scan(&stats.CompletedToday)

		// Fetch pending and processing items (up to 50) for display
		itemsQuery := `
			SELECT id, content, status, enrichment_attempts,
			       COALESCE(enrichment_error, '') as enrichment_error,
			       created_at, updated_at, enriched_at
			FROM memories
			WHERE status IN ('pending', 'processing')
			ORDER BY created_at ASC
			LIMIT 50
		`
		itemRows, err := db.QueryContext(ctx, itemsQuery)
		if err == nil {
			defer func() { _ = itemRows.Close() }()
			for itemRows.Next() {
				var item QueueItemResponse
				var enrichedAt *time.Time
				if err := itemRows.Scan(
					&item.ID, &item.Content, &item.Status,
					&item.EnrichmentAttempts, &item.EnrichmentError,
					&item.CreatedAt, &item.UpdatedAt, &enrichedAt,
				); err != nil {
					continue
				}
				item.EnrichedAt = enrichedAt
				// Truncate content for display
				if len(item.Content) > 200 {
					item.Content = item.Content[:200] + "..."
				}
				items = append(items, item)
			}
		}
	}

	if items == nil {
		items = []QueueItemResponse{}
	}

	respondJSON(w, http.StatusOK, QueueResponse{
		Stats: stats,
		Items: items,
	})
}
