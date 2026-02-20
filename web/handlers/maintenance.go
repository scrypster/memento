package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/scrypster/memento/internal/connections"
)

// MaintenanceEngine is the engine interface needed for maintenance operations.
type MaintenanceEngine interface {
	QueueEnrichmentForMemory(memoryID, content string) bool
	QueueEmbeddingForMemory(memoryID, content string) bool
	GetQueueSize() int
}

// MaintenanceHandler handles maintenance operations per connection.
type MaintenanceHandler struct {
	connManager *connections.Manager
	engine      MaintenanceEngine // may be nil
}

// NewMaintenanceHandler creates a new MaintenanceHandler.
func NewMaintenanceHandler(cm *connections.Manager, engine MaintenanceEngine) *MaintenanceHandler {
	return &MaintenanceHandler{connManager: cm, engine: engine}
}

// MaintenanceStatus is the response for GET /api/connections/{name}/maintenance.
type MaintenanceStatus struct {
	NeedsEnrichment   int          `json:"needs_enrichment"`
	MissingEmbeddings int          `json:"missing_embeddings"`
	ModelMismatches   int          `json:"model_mismatches"`
	CurrentModel      string       `json:"current_model"`
	StoredModels      []ModelCount `json:"stored_models"`
}

// ModelCount represents a model name and the number of embeddings stored with it.
type ModelCount struct {
	Model string `json:"model"`
	Count int    `json:"count"`
}

// dbWithGetter is a store that exposes the underlying *sql.DB.
type dbWithGetter interface {
	GetDB() *sql.DB
}

// GetStatus handles GET /api/connections/{name}/maintenance.
func (h *MaintenanceHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "connection name is required", http.StatusBadRequest)
		return
	}

	store, err := h.connManager.GetStore(name)
	if err != nil {
		http.Error(w, "connection not found: "+err.Error(), http.StatusNotFound)
		return
	}

	dbGetter, ok := store.(dbWithGetter)
	if !ok {
		http.Error(w, "store does not expose database connection", http.StatusInternalServerError)
		return
	}
	db := dbGetter.GetDB()

	status := MaintenanceStatus{}

	// needs_enrichment: memories in pending or failed state
	if err := db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM memories WHERE status IN ('pending', 'failed')`).
		Scan(&status.NeedsEnrichment); err != nil {
		log.Printf("maintenance: failed to query needs_enrichment for %s: %v", name, err)
	}

	// missing_embeddings: enriched memories with no embedding row
	if err := db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM memories m
		 WHERE m.status = 'enriched'
		 AND NOT EXISTS (SELECT 1 FROM embeddings e WHERE e.memory_id = m.id)`).
		Scan(&status.MissingEmbeddings); err != nil {
		log.Printf("maintenance: failed to query missing_embeddings for %s: %v", name, err)
	}

	// stored models and counts
	rows, err := db.QueryContext(r.Context(),
		`SELECT model, COUNT(*) FROM embeddings GROUP BY model`)
	if err != nil {
		log.Printf("maintenance: failed to query stored models for %s: %v", name, err)
	} else {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var mc ModelCount
			if err := rows.Scan(&mc.Model, &mc.Count); err == nil {
				status.StoredModels = append(status.StoredModels, mc)
			}
		}
	}
	if status.StoredModels == nil {
		status.StoredModels = []ModelCount{}
	}

	// current_model from connection LLM config
	for _, conn := range h.connManager.ListConnections() {
		if conn.Name == name {
			embModel := conn.LLM.EmbeddingModel
			if embModel == "" {
				embModel = conn.LLM.Model
			}
			status.CurrentModel = embModel
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// backfillRequest is the request body for POST /api/connections/{name}/maintenance/backfill.
type backfillRequest struct {
	Type string `json:"type"` // "enrichment", "embeddings", "re-embed-all"
}

// backfillResponse is the response for the backfill endpoint.
type backfillResponse struct {
	Queued  int    `json:"queued"`
	Message string `json:"message"`
}

// RunBackfill handles POST /api/connections/{name}/maintenance/backfill.
func (h *MaintenanceHandler) RunBackfill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "connection name is required", http.StatusBadRequest)
		return
	}

	var req backfillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	store, err := h.connManager.GetStore(name)
	if err != nil {
		http.Error(w, "connection not found: "+err.Error(), http.StatusNotFound)
		return
	}

	dbGetter, ok := store.(dbWithGetter)
	if !ok {
		http.Error(w, "store does not expose database connection", http.StatusInternalServerError)
		return
	}
	db := dbGetter.GetDB()

	resp := backfillResponse{}

	switch req.Type {
	case "enrichment":
		rows, err := db.QueryContext(r.Context(),
			`SELECT id, content FROM memories WHERE status IN ('pending', 'failed')`)
		if err != nil {
			http.Error(w, "failed to query memories: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var id, content string
			if err := rows.Scan(&id, &content); err != nil {
				continue
			}
			if h.engine != nil {
				if h.engine.QueueEnrichmentForMemory(id, content) {
					resp.Queued++
				}
			} else {
				resp.Queued++
			}
		}
		resp.Message = "queued for enrichment"
		if h.engine == nil {
			resp.Message = "queued for next startup (engine not running)"
		}

	case "embeddings":
		rows, err := db.QueryContext(r.Context(),
			`SELECT m.id, m.content FROM memories m
			 WHERE m.status = 'enriched'
			 AND NOT EXISTS (SELECT 1 FROM embeddings e WHERE e.memory_id = m.id)`)
		if err != nil {
			http.Error(w, "failed to query memories: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var id, content string
			if err := rows.Scan(&id, &content); err != nil {
				continue
			}
			if h.engine != nil {
				if h.engine.QueueEmbeddingForMemory(id, content) {
					resp.Queued++
				}
			} else {
				resp.Queued++
			}
		}
		resp.Message = "queued for embedding generation"
		if h.engine == nil {
			resp.Message = "queued for next startup (engine not running)"
		}

	case "re-embed-all":
		// Delete all existing embeddings
		if _, err := db.ExecContext(r.Context(), `DELETE FROM embeddings`); err != nil {
			http.Error(w, "failed to delete embeddings: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Queue all enriched memories for embedding
		rows, err := db.QueryContext(r.Context(),
			`SELECT id, content FROM memories WHERE status = 'enriched'`)
		if err != nil {
			http.Error(w, "failed to query memories: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var id, content string
			if err := rows.Scan(&id, &content); err != nil {
				continue
			}
			if h.engine != nil {
				if h.engine.QueueEmbeddingForMemory(id, content) {
					resp.Queued++
				}
			} else {
				resp.Queued++
			}
		}
		resp.Message = "deleted all embeddings and queued for re-embedding"
		if h.engine == nil {
			resp.Message = "deleted all embeddings; queued for next startup (engine not running)"
		}

	default:
		http.Error(w, "invalid type: must be enrichment, embeddings, or re-embed-all", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// RetryEnrichment handles POST /api/memories/{id}/retry.
// Resets a failed memory to pending and queues it for re-enrichment.
func (h *MaintenanceHandler) RetryEnrichment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	memoryID := r.PathValue("id")
	if memoryID == "" {
		http.Error(w, `{"error":"memory ID is required"}`, http.StatusBadRequest)
		return
	}

	// Get store for the active connection
	connName := r.Header.Get("X-Connection")
	if connName == "" {
		connName = "default"
	}
	store, err := h.connManager.GetStore(connName)
	if err != nil {
		http.Error(w, `{"error":"connection not found"}`, http.StatusNotFound)
		return
	}

	// Fetch the memory
	mem, err := store.Get(r.Context(), memoryID)
	if err != nil || mem == nil {
		http.Error(w, `{"error":"memory not found"}`, http.StatusNotFound)
		return
	}

	if mem.Status != "failed" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      memoryID,
			"queued":  false,
			"message": "Memory is not in failed state (current: " + string(mem.Status) + ")",
		})
		return
	}

	// Reset status to pending
	mem.Status = "pending"
	if err := store.Update(r.Context(), mem); err != nil {
		http.Error(w, `{"error":"failed to update memory status"}`, http.StatusInternalServerError)
		return
	}

	// Queue for enrichment
	queued := false
	if h.engine != nil {
		queued = h.engine.QueueEnrichmentForMemory(memoryID, mem.Content)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      memoryID,
		"queued":  queued,
		"message": "Memory queued for enrichment retry",
	})
}

// UnknownTypeStat is a single row from the unknown_type_stats table.
type UnknownTypeStat struct {
	TypeName  string `json:"type_name"`
	Count     int    `json:"count"`
	FirstSeen string `json:"first_seen"`
	LastSeen  string `json:"last_seen"`
}

// UnknownTypesResponse groups unknown type stats by domain.
type UnknownTypesResponse struct {
	Entities      []UnknownTypeStat `json:"entities"`
	Relationships []UnknownTypeStat `json:"relationships"`
}

// GetUnknownTypes handles GET /api/connections/{name}/maintenance/unknown-types.
// Returns entity and relationship type names that the LLM returned but were not
// in the allowed list, along with occurrence counts.
func (h *MaintenanceHandler) GetUnknownTypes(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "connection name is required", http.StatusBadRequest)
		return
	}

	store, err := h.connManager.GetStore(name)
	if err != nil {
		http.Error(w, "connection not found: "+err.Error(), http.StatusNotFound)
		return
	}

	dbGetter, ok := store.(dbWithGetter)
	if !ok {
		http.Error(w, "store does not expose database connection", http.StatusInternalServerError)
		return
	}
	db := dbGetter.GetDB()

	resp := UnknownTypesResponse{
		Entities:      []UnknownTypeStat{},
		Relationships: []UnknownTypeStat{},
	}

	rows, err := db.QueryContext(r.Context(),
		`SELECT domain, type_name, count, first_seen, last_seen
		 FROM unknown_type_stats
		 ORDER BY count DESC, type_name ASC`)
	if err != nil {
		// Table may not exist yet (migration not run); return empty result
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var domain, typeName, firstSeen, lastSeen string
		var count int
		if err := rows.Scan(&domain, &typeName, &count, &firstSeen, &lastSeen); err != nil {
			continue
		}
		stat := UnknownTypeStat{TypeName: typeName, Count: count, FirstSeen: firstSeen, LastSeen: lastSeen}
		switch domain {
		case "entity":
			resp.Entities = append(resp.Entities, stat)
		case "relationship":
			resp.Relationships = append(resp.Relationships, stat)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
