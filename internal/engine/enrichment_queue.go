package engine

import (
	"context"
	"log"
	"time"
)

// queueEnrichmentJob attempts to queue an enrichment job.
// Returns true if the job was queued successfully, false if the queue is full or closed.
func (e *MemoryEngine) queueEnrichmentJob(job *EnrichmentJob) bool {
	// Check if worker context is cancelled (shutdown in progress)
	if e.workerCtx != nil && e.workerCtx.Err() != nil {
		return false
	}

	// Try to queue (non-blocking)
	select {
	case e.enrichmentQueue <- job:
		return true
	default:
		// Queue is full or closed
		log.Printf("WARNING: Enrichment queue full (size=%d), dropping job for memory %s",
			e.config.QueueSize, job.MemoryID)
		return false
	}
}

// createEnrichmentJob creates a new enrichment job from memory data.
func (e *MemoryEngine) createEnrichmentJob(memoryID, content string, attempt int) *EnrichmentJob {
	return &EnrichmentJob{
		MemoryID:  memoryID,
		Content:   content,
		Timestamp: time.Now(),
		Attempt:   attempt,
	}
}

// requeueEnrichmentJob attempts to requeue a failed enrichment job.
// Returns true if the job was requeued, false if max retries exceeded or queue full.
func (e *MemoryEngine) requeueEnrichmentJob(ctx context.Context, job *EnrichmentJob) bool {
	// Check if worker context is cancelled (shutdown in progress)
	if e.workerCtx != nil && e.workerCtx.Err() != nil {
		log.Printf("WARNING: Failed to requeue job for memory %s, shutdown in progress", job.MemoryID)
		return false
	}

	// Check if max retries exceeded
	if job.Attempt >= e.config.MaxRetries {
		log.Printf("Max retries (%d) exceeded for memory %s, giving up",
			e.config.MaxRetries, job.MemoryID)
		return false
	}

	// Increment attempt counter
	job.Attempt++

	// Try to requeue (non-blocking to avoid panic on closed channel)
	select {
	case e.enrichmentQueue <- job:
		log.Printf("Requeued enrichment job for memory %s (attempt %d/%d)",
			job.MemoryID, job.Attempt, e.config.MaxRetries)
		return true
	case <-time.After(10 * time.Millisecond):
		// Timeout - queue might be full or closed
		log.Printf("WARNING: Failed to requeue job for memory %s, queue timeout",
			job.MemoryID)
		return false
	}
}

// getQueueLength returns the current number of jobs in the queue.
func (e *MemoryEngine) getQueueLength() int {
	return len(e.enrichmentQueue)
}
