package engine

import (
	"context"
	"log"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// RecoverPendingEnrichments recovers pending enrichments from previous runs.
// It queries the storage for memories with StatusPending and queues them for enrichment.
// This is called automatically during Start() to ensure no enrichments are lost.
func (e *MemoryEngine) RecoverPendingEnrichments(ctx context.Context) error {
	log.Println("Starting enrichment recovery for pending memories...")

	totalQueued := 0

	for page := 1; ; page++ {
		opts := storage.ListOptions{
			Filter: map[string]interface{}{
				"status": types.StatusPending,
			},
			Limit: e.config.RecoveryBatchSize,
			Page:  page,
		}

		result, err := e.memoryStore.List(ctx, opts)
		if err != nil {
			log.Printf("ERROR: Failed to list pending memories for recovery: %v", err)
			return err
		}

		if len(result.Items) == 0 {
			log.Println("No pending memories to recover")
			return nil
		}

		// Queue all pending memories for enrichment
		for _, memory := range result.Items {
			job := e.createEnrichmentJob(memory.ID, memory.Content, 0)

			if e.queueEnrichmentJob(job) {
				totalQueued++
			} else {
				// Queue is full, mark as failed
				if err := e.memoryStore.UpdateStatus(ctx, memory.ID, types.StatusFailed); err != nil {
					log.Printf("ERROR: Failed to mark memory %s as failed: %v", memory.ID, err)
				}
			}
		}

		if !result.HasMore {
			break
		}

		log.Printf("More pending memories found (%d total), processing next batch...", result.Total)
	}

	log.Printf("Recovery complete: queued %d pending enrichments", totalQueued)
	return nil
}
