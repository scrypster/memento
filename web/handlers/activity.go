package handlers

import (
	"net/http"
	"time"

	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
)

// ActivityHandler handles the /api/activity endpoint.
type ActivityHandler struct {
	store             storage.MemoryStore
	connectionManager *connections.Manager
}

// NewActivityHandler creates a new ActivityHandler.
func NewActivityHandler(store storage.MemoryStore, connManager *connections.Manager) *ActivityHandler {
	return &ActivityHandler{
		store:             store,
		connectionManager: connManager,
	}
}

// ActivityPoint represents a single data point in the activity time series.
type ActivityPoint struct {
	Time  string `json:"time"`  // ISO-8601 timestamp (bucket start)
	Count int    `json:"count"` // Number of memories created in this bucket
}

// ActivityResponse is the JSON response for GET /api/activity.
type ActivityResponse struct {
	Points    []ActivityPoint `json:"points"`
	Range     string          `json:"range"`
	BucketSec int             `json:"bucket_sec"`
}

// GetActivity handles GET /api/activity?range={5min|1hour|24hour|week}
// It returns a time series of memory creation counts bucketed by an
// appropriate interval for the requested range.
func (h *ActivityHandler) GetActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Resolve connection
	connectionName := r.URL.Query().Get("connection")
	if connectionName == "" {
		connectionName = r.Header.Get("X-Connection-ID")
	}
	store, err := h.connectionManager.GetStore(connectionName)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid connection", err)
		return
	}

	// Only SQLite stores support direct SQL queries for bucketed counts.
	sqliteStore, ok := store.(*sqlite.MemoryStore)
	if !ok {
		// Fallback: return empty data for non-SQLite stores.
		respondJSON(w, http.StatusOK, ActivityResponse{
			Points:    []ActivityPoint{},
			Range:     r.URL.Query().Get("range"),
			BucketSec: 60,
		})
		return
	}

	rangeParam := r.URL.Query().Get("range")

	// Determine window duration and bucket size based on range.
	var windowDur time.Duration
	var bucketSec int
	switch rangeParam {
	case "5min":
		windowDur = 5 * time.Minute
		bucketSec = 10 // 10-second buckets → 30 points
	case "1hour":
		windowDur = time.Hour
		bucketSec = 120 // 2-minute buckets → 30 points
	case "week":
		windowDur = 7 * 24 * time.Hour
		bucketSec = 4 * 3600 // 4-hour buckets → 42 points
	default: // "24hour"
		rangeParam = "24hour"
		windowDur = 24 * time.Hour
		bucketSec = 3600 // 1-hour buckets → 24 points
	}

	now := time.Now().UTC()
	since := now.Add(-windowDur)

	db := sqliteStore.GetDB()

	// Query: count memories grouped into fixed-width buckets using SQLite's
	// integer division trick: bucket = (epoch_seconds / bucketSec) * bucketSec
	//
	// substr(created_at, 1, 19) normalizes the stored format — Go's SQLite driver
	// stores time.Time as "2006-01-02 15:04:05.999999999 -0700 MST m=+..." which
	// SQLite's strftime cannot parse. Extracting the first 19 chars gives a clean
	// "YYYY-MM-DD HH:MM:SS" that SQLite handles correctly for both old and new records.
	const query = `
SELECT
  datetime((CAST(strftime('%s', substr(created_at, 1, 19)) AS INTEGER) / ?) * ?, 'unixepoch') AS bucket,
  COUNT(*) AS cnt
FROM memories
WHERE substr(created_at, 1, 19) >= ?
GROUP BY bucket
ORDER BY bucket ASC
`
	rows, err := db.QueryContext(ctx, query, bucketSec, bucketSec, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query activity", err)
		return
	}
	defer rows.Close()

	// Build a map of bucket → count from the query results.
	bucketCounts := make(map[string]int)
	for rows.Next() {
		var bucket string
		var cnt int
		if err := rows.Scan(&bucket, &cnt); err != nil {
			continue
		}
		bucketCounts[bucket] = cnt
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read activity rows", err)
		return
	}

	// Generate all expected bucket timestamps so zero-count periods are visible.
	points := generateBuckets(since, now, bucketSec, bucketCounts)

	respondJSON(w, http.StatusOK, ActivityResponse{
		Points:    points,
		Range:     rangeParam,
		BucketSec: bucketSec,
	})
}

// generateBuckets creates a complete slice of ActivityPoints for every bucket
// between since and now, filling in zero counts for buckets with no data.
func generateBuckets(since, now time.Time, bucketSec int, counts map[string]int) []ActivityPoint {
	// Align since to bucket boundary.
	sinceEpoch := since.Unix()
	startEpoch := (sinceEpoch / int64(bucketSec)) * int64(bucketSec)

	var points []ActivityPoint
	for t := startEpoch; t <= now.Unix(); t += int64(bucketSec) {
		bucket := time.Unix(t, 0).UTC()
		// SQLite returns "YYYY-MM-DD HH:MM:SS" from datetime().
		key := bucket.Format("2006-01-02 15:04:05")
		cnt := counts[key]
		points = append(points, ActivityPoint{
			Time:  bucket.Format(time.RFC3339),
			Count: cnt,
		})
	}
	return points
}

