package engine

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// enrichmentWorker is a worker goroutine that processes enrichment jobs.
// It runs continuously until the enrichment queue is closed.
func (e *MemoryEngine) enrichmentWorker(ctx context.Context, workerID int) {
	defer e.workerWaitGroup.Done()

	log.Printf("Enrichment worker %d started", workerID)

	for job := range e.enrichmentQueue {
		e.processEnrichmentJob(ctx, workerID, job)
	}

	log.Printf("Enrichment worker %d stopped", workerID)
}

// processEnrichmentJob processes a single enrichment job using the extraction pipeline.
// Tracks EntityStatus and RelationshipStatus separately for granular enrichment visibility.
// Handles partial failures gracefully (entities preserved even if relationships fail).
// If job.EmbeddingOnly is true, skips the full LLM extraction and only generates embeddings.
func (e *MemoryEngine) processEnrichmentJob(ctx context.Context, workerID int, job *EnrichmentJob) {
	log.Printf("Worker %d processing memory %s (attempt %d, embeddingOnly=%v)", workerID, job.MemoryID, job.Attempt, job.EmbeddingOnly)

	// Use background context for database operations to avoid cancellation during shutdown
	dbCtx := context.Background()

	// Apply exponential backoff for retries to reduce database lock contention
	if job.Attempt > 0 {
		backoffDuration := time.Duration(job.Attempt*job.Attempt) * 100 * time.Millisecond // 100ms, 400ms, 900ms...
		log.Printf("Worker %d: Waiting %v before retry (attempt %d)", workerID, backoffDuration, job.Attempt)
		time.Sleep(backoffDuration)
	}

	// EmbeddingOnly path: just generate embeddings and return.
	if job.EmbeddingOnly {
		if e.enrichmentService != nil {
			if embErr := e.enrichmentService.GenerateEmbeddings(ctx, job.MemoryID, job.Content); embErr != nil {
				log.Printf("Worker %d: WARNING - embedding-only generation failed for %s: %v", workerID, job.MemoryID, embErr)
			} else {
				log.Printf("Worker %d: embedding-only job completed for %s", workerID, job.MemoryID)
			}
		} else {
			log.Printf("Worker %d: embedding-only job skipped (no enrichment service) for %s", workerID, job.MemoryID)
		}
		if e.onEnrichmentComplete != nil {
			e.onEnrichmentComplete(job.MemoryID)
		}
		return
	}

	// Update status to processing
	if err := e.memoryStore.UpdateStatus(dbCtx, job.MemoryID, types.StatusProcessing); err != nil {
		log.Printf("ERROR: Worker %d failed to update status to processing for %s: %v",
			workerID, job.MemoryID, err)
		// Try to requeue the job
		if !e.requeueEnrichmentJob(ctx, job) {
			// Failed to requeue, mark as failed
			e.memoryStore.UpdateStatus(dbCtx, job.MemoryID, types.StatusFailed)
		}
		return
	}

	// Initialize enrichment tracking
	var entityStatus types.EnrichmentStatus
	var relationshipStatus types.EnrichmentStatus
	var enrichmentError string
	now := time.Now()

	// Phase 3 integration: Call LLM extraction pipeline for entity and relationship extraction
	var embeddingStatus types.EnrichmentStatus
	if e.enrichmentService != nil {
		pipelineResult, err := e.enrichmentService.ExtractionPipeline.Extract(ctx, job.MemoryID, job.Content)
		if err != nil {
			log.Printf("ERROR: Worker %d entity extraction failed for %s: %v", workerID, job.MemoryID, err)
			enrichmentError = err.Error()
			// Mark as failed and try to requeue
			if !e.requeueEnrichmentJob(ctx, job) {
				// Failed to requeue, mark as failed
				e.memoryStore.UpdateStatus(dbCtx, job.MemoryID, types.StatusFailed)
			}
			return
		}

		// Track entity and relationship statuses separately
		entityStatus = pipelineResult.EntityStatus
		relationshipStatus = pipelineResult.RelationshipStatus

		// Capture relationship error if it occurred (entity extraction succeeded but relationship failed)
		if pipelineResult.RelationshipError != "" {
			enrichmentError = fmt.Sprintf("entity: success, relationship: %s", pipelineResult.RelationshipError)
		}

		log.Printf("Worker %d pipeline results for %s: Entity=%s, Relationship=%s",
			workerID, job.MemoryID, entityStatus, relationshipStatus)

		// Generate vector embedding
		if embErr := e.enrichmentService.GenerateEmbeddings(ctx, job.MemoryID, job.Content); embErr != nil {
			log.Printf("Worker %d: WARNING - embedding generation failed for %s: %v", workerID, job.MemoryID, embErr)
			embeddingStatus = types.EnrichmentFailed
		} else {
			embeddingStatus = types.EnrichmentCompleted
			log.Printf("Worker %d: embedding generated for %s", workerID, job.MemoryID)
		}
	} else {
		// Fallback: simulate enrichment work if service not initialized
		log.Printf("Warning: Enrichment service not available, skipping LLM enrichment for %s", job.MemoryID)
		time.Sleep(100 * time.Millisecond)
		entityStatus = types.EnrichmentSkipped
		relationshipStatus = types.EnrichmentSkipped
		embeddingStatus = types.EnrichmentSkipped
	}

	// Update status to enriched
	if err := e.memoryStore.UpdateStatus(dbCtx, job.MemoryID, types.StatusEnriched); err != nil {
		log.Printf("ERROR: Worker %d failed to update status to enriched for %s: %v",
			workerID, job.MemoryID, err)
		// Try to requeue the job
		if !e.requeueEnrichmentJob(ctx, job) {
			// Failed to requeue, mark as failed
			e.memoryStore.UpdateStatus(dbCtx, job.MemoryID, types.StatusFailed)
		}
		return
	}

	// Update enrichment metadata with separate Call 1, Call 2, and embedding tracking
	enrichment := storage.EnrichmentUpdate{
		EntityStatus:       entityStatus,
		RelationshipStatus: relationshipStatus,
		EmbeddingStatus:    embeddingStatus,
		EnrichmentAttempts: job.Attempt + 1,
		EnrichmentError:    enrichmentError,
		EnrichedAt:         &now,
	}

	if err := e.memoryStore.UpdateEnrichment(ctx, job.MemoryID, enrichment); err != nil {
		log.Printf("WARNING: Worker %d failed to update enrichment metadata for %s: %v",
			workerID, job.MemoryID, err)
	}

	log.Printf("Worker %d completed enrichment for memory %s (Entity=%s, Relationship=%s)",
		workerID, job.MemoryID, entityStatus, relationshipStatus)

	// Trigger callback for UI updates (e.g., WebSocket broadcast)
	if e.onEnrichmentComplete != nil {
		e.onEnrichmentComplete(job.MemoryID)
	}
}

// startWorkerPool starts the worker goroutines.
func (e *MemoryEngine) startWorkerPool(ctx context.Context) {
	for i := 0; i < e.config.NumWorkers; i++ {
		e.workerWaitGroup.Add(1)
		go e.enrichmentWorker(ctx, i)
	}

	log.Printf("Started %d enrichment workers", e.config.NumWorkers)
}

// stopWorkerPool stops the worker goroutines gracefully.
func (e *MemoryEngine) stopWorkerPool(ctx context.Context) error {
	// Close the enrichment queue (no more jobs)
	close(e.enrichmentQueue)

	// Wait for workers to drain (with timeout)
	done := make(chan struct{})
	go func() {
		e.workerWaitGroup.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All enrichment workers finished gracefully")
		return nil
	case <-time.After(e.config.ShutdownTimeout):
		remaining := e.getQueueLength()
		log.Printf("WARNING: Shutdown timeout reached, %d enrichment jobs may be dropped", remaining)
		return nil
	case <-ctx.Done():
		remaining := e.getQueueLength()
		log.Printf("WARNING: Context cancelled, %d enrichment jobs may be dropped", remaining)
		return ctx.Err()
	}
}
