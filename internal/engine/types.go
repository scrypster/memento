// Package engine provides the core memory engine with async enrichment pipeline.
// The engine orchestrates non-blocking memory storage with background LLM enrichment
// using worker pools and job queues.
package engine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// EnrichmentJob represents a job for async memory enrichment.
// Jobs are queued when memories are stored and processed by worker goroutines.
type EnrichmentJob struct {
	// MemoryID is the unique identifier of the memory to enrich.
	MemoryID string

	// Content is the memory content to process.
	Content string

	// Timestamp is when the job was queued.
	Timestamp time.Time

	// Attempt tracks retry attempts for this job.
	Attempt int

	// EmbeddingOnly indicates that only embedding generation should be performed,
	// skipping the full LLM extraction pipeline.
	EmbeddingOnly bool
}

// Config holds configuration for the memory engine.
type Config struct {
	// NumWorkers is the number of enrichment worker goroutines (default: 4).
	NumWorkers int

	// QueueSize is the size of the enrichment job queue buffer (default: 1000).
	QueueSize int

	// ShutdownTimeout is the maximum time to wait for workers to drain on shutdown (default: 30s).
	ShutdownTimeout time.Duration

	// MaxRetries is the maximum number of enrichment retry attempts (default: 3).
	MaxRetries int

	// RecoveryBatchSize is the number of pending memories to recover per batch (default: 1000).
	RecoveryBatchSize int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		NumWorkers:        4,
		QueueSize:         1000,
		ShutdownTimeout:   30 * time.Second,
		MaxRetries:        3,
		RecoveryBatchSize: 1000,
	}
}

// Validate checks if the config is valid.
func (c *Config) Validate() error {
	if c.NumWorkers < 1 {
		return fmt.Errorf("NumWorkers must be >= 1, got %d", c.NumWorkers)
	}

	if c.QueueSize < 1 {
		return fmt.Errorf("QueueSize must be >= 1, got %d", c.QueueSize)
	}

	if c.ShutdownTimeout < 0 {
		return fmt.Errorf("ShutdownTimeout must be >= 0, got %v", c.ShutdownTimeout)
	}

	if c.MaxRetries < 0 {
		return fmt.Errorf("MaxRetries must be >= 0, got %d", c.MaxRetries)
	}

	if c.RecoveryBatchSize < 1 {
		return fmt.Errorf("RecoveryBatchSize must be >= 1, got %d", c.RecoveryBatchSize)
	}

	return nil
}

// GenerateMemoryID generates a unique memory ID in the format mem:domain:slug.
// If domain is empty, "default" is used.
// The slug is a random hex string to ensure uniqueness.
func GenerateMemoryID(domain, slug string) string {
	if domain == "" {
		domain = "default"
	}

	// If no slug provided, generate a random one
	if slug == "" {
		slug = generateRandomSlug()
	}

	// Sanitize domain and slug (remove colons and whitespace)
	domain = strings.ReplaceAll(strings.TrimSpace(domain), ":", "-")
	slug = strings.ReplaceAll(strings.TrimSpace(slug), ":", "-")

	return fmt.Sprintf("mem:%s:%s", domain, slug)
}

// generateRandomSlug generates a random hex slug for memory IDs.
func generateRandomSlug() string {
	// Generate 8 random bytes (16 hex characters)
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based slug if random generation fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
