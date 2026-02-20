package engine

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/llm"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// MemoryEngine is the core orchestrator for memory storage and enrichment.
// It provides non-blocking Store() operations (<10ms) with async LLM enrichment
// via a worker pool and job queue architecture.
type MemoryEngine struct {
	// Configuration
	config Config

	// Storage layer
	memoryStore storage.MemoryStore

	// Enrichment pipeline
	enrichmentQueue chan *EnrichmentJob
	workerWaitGroup sync.WaitGroup
	workerCtx       context.Context
	workerCancel    context.CancelFunc

	// Intelligence layer
	searchOrchestrator *SearchOrchestrator
	inferenceEngine    *InferenceEngine
	decayManager       *DecayManager
	confidenceScorer   *ConfidenceScorer

	// Enrichment service
	enrichmentService *EnrichmentService

	// State management
	started        bool
	shuttingDown   bool
	mu             sync.RWMutex

	// Callbacks
	onMemoryCreated      func(memoryID string)
	onEnrichmentStarted  func(memoryID string)
	onEnrichmentComplete func(memoryID string)
}

// NewMemoryEngine creates a new memory engine with the given configuration.
// The store parameter provides the storage backend for memories.
// The globalConfig parameter provides LLM and system configuration.
// Use DefaultConfig() for sensible defaults.
func NewMemoryEngine(store storage.MemoryStore, engineConfig Config, globalConfig *config.Config) (*MemoryEngine, error) {
	if store == nil {
		return nil, fmt.Errorf("memory store is required")
	}

	if err := engineConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	engine := &MemoryEngine{
		config:          engineConfig,
		memoryStore:     store,
		enrichmentQueue: make(chan *EnrichmentJob, engineConfig.QueueSize),
		started:         false,
		shuttingDown:    false,
	}

	// Initialize intelligence layer
	engine.searchOrchestrator = NewSearchOrchestrator(store)
	engine.inferenceEngine = NewInferenceEngine(store)
	engine.decayManager = NewDecayManager()
	engine.confidenceScorer = NewConfidenceScorer(store)

	// Initialize enrichment service with LLM client via factory
	if globalConfig != nil {
		connCfg := llmConfigFromGlobal(globalConfig)
		llmClient, err := llm.NewTextGenerator(connCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create LLM client: %w", err)
		}

		embeddingModel := globalConfig.LLM.OllamaEmbeddingModel
		embeddingClient, embErr := llm.NewEmbeddingGenerator(connCfg, embeddingModel)
		if embErr != nil {
			log.Printf("warning: failed to create embedding client: %v", embErr)
			embeddingClient = nil
		}

		// Get database connection from SQLite store
		if sqliteStore, ok := store.(*sqlite.MemoryStore); ok {
			embeddingProvider := sqlite.NewEmbeddingProvider(sqliteStore.GetDB())
			engine.enrichmentService = NewEnrichmentServiceWithEmbeddings(llmClient, embeddingClient, sqliteStore.GetDB(), embeddingProvider)
			log.Printf("Enrichment service initialized with provider=%s model=%s", connCfg.Provider, connCfg.Model)
		} else {
			log.Println("Warning: Enrichment service not initialized (non-SQLite store)")
		}
	} else {
		log.Println("Warning: Enrichment service not initialized (no config provided)")
	}

	return engine, nil
}

// QueueEnrichmentForMemory queues a memory for immediate enrichment.
// Returns true if the job was queued, false if the queue is full or engine not started.
func (e *MemoryEngine) QueueEnrichmentForMemory(memoryID, content string) bool {
	e.mu.RLock()
	canQueue := e.started && !e.shuttingDown
	e.mu.RUnlock()
	if !canQueue {
		return false
	}
	job := e.createEnrichmentJob(memoryID, content, 0)
	return e.queueEnrichmentJob(job)
}

// QueueEmbeddingForMemory queues a memory for embedding-only processing.
// This skips the full LLM extraction pipeline and only generates vector embeddings.
// Returns true if the job was queued, false if the queue is full or engine not started.
func (e *MemoryEngine) QueueEmbeddingForMemory(memoryID, content string) bool {
	e.mu.RLock()
	canQueue := e.started && !e.shuttingDown
	e.mu.RUnlock()
	if !canQueue {
		return false
	}
	job := &EnrichmentJob{
		MemoryID:      memoryID,
		Content:       content,
		EmbeddingOnly: true,
	}
	return e.queueEnrichmentJob(job)
}

// Embed generates a vector embedding for the given text using the embedding model.
// Returns an error if no embedding client is configured.
func (e *MemoryEngine) Embed(ctx context.Context, text string) ([]float64, error) {
	if e.enrichmentService == nil {
		return nil, fmt.Errorf("enrichment service not available")
	}
	return e.enrichmentService.Embed(ctx, text)
}

// Summarize sends a prompt to the LLM and returns the completion text.
// Used by the MCP consolidate_memories tool for LLM-assisted merging.
// Returns an error if no LLM client is configured.
func (e *MemoryEngine) Summarize(ctx context.Context, prompt string) (string, error) {
	if e.enrichmentService == nil {
		return "", fmt.Errorf("enrichment service not available")
	}
	return e.enrichmentService.llmClient.Complete(ctx, prompt)
}

// NewMemoryEngineWithEmbeddings creates a new memory engine with embedding support.
// llmClient is used for entity/relationship extraction.
// embeddingClient is used for vector embedding generation (e.g. nomic-embed-text).
func NewMemoryEngineWithEmbeddings(store storage.MemoryStore, engineConfig Config, llmClient llm.TextGenerator, embeddingClient llm.EmbeddingGenerator, embeddingProvider storage.EmbeddingProvider) (*MemoryEngine, error) {
	if store == nil {
		return nil, fmt.Errorf("memory store is required")
	}

	if llmClient == nil {
		return nil, fmt.Errorf("LLM client is required")
	}

	if embeddingProvider == nil {
		return nil, fmt.Errorf("embedding provider is required")
	}

	if err := engineConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	engine := &MemoryEngine{
		config:          engineConfig,
		memoryStore:     store,
		enrichmentQueue: make(chan *EnrichmentJob, engineConfig.QueueSize),
		started:         false,
		shuttingDown:    false,
	}

	// Initialize intelligence layer
	engine.searchOrchestrator = NewSearchOrchestrator(store)
	engine.inferenceEngine = NewInferenceEngine(store)
	engine.decayManager = NewDecayManager()
	engine.confidenceScorer = NewConfidenceScorer(store)

	// Initialize enrichment service with embedding support
	if sqliteStore, ok := store.(*sqlite.MemoryStore); ok {
		engine.enrichmentService = NewEnrichmentServiceWithEmbeddings(llmClient, embeddingClient, sqliteStore.GetDB(), embeddingProvider)
		log.Println("Enrichment service initialized with LLM and embedding support")
	} else {
		log.Println("Warning: Enrichment service initialized without embedding support (non-SQLite store)")
		engine.enrichmentService = NewEnrichmentService(llmClient, nil)
	}

	return engine, nil
}

// SetOnMemoryCreated sets a callback fired when a new memory is stored (before enrichment).
func (e *MemoryEngine) SetOnMemoryCreated(callback func(memoryID string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onMemoryCreated = callback
}

// SetOnEnrichmentStarted sets a callback fired when enrichment begins processing a memory.
func (e *MemoryEngine) SetOnEnrichmentStarted(callback func(memoryID string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onEnrichmentStarted = callback
}

// SetOnEnrichmentComplete sets a callback to be called when enrichment completes for a memory.
// The callback receives the memory ID. This is useful for triggering UI updates via WebSocket.
func (e *MemoryEngine) SetOnEnrichmentComplete(callback func(memoryID string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onEnrichmentComplete = callback
}

// Start starts the memory engine and its worker pool.
// It also initiates recovery of pending enrichments from previous runs.
// This must be called before using Store().
func (e *MemoryEngine) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.started {
		return fmt.Errorf("engine already started")
	}

	log.Println("Starting memory engine...")

	// Create worker context
	e.workerCtx, e.workerCancel = context.WithCancel(ctx)

	// Start worker pool
	e.startWorkerPool(e.workerCtx)

	// Recover pending enrichments in background
	// (non-blocking so Start() returns quickly)
	go func() {
		if err := e.RecoverPendingEnrichments(ctx); err != nil {
			log.Printf("ERROR: Enrichment recovery failed: %v", err)
		}
	}()

	e.started = true
	log.Println("Memory engine started successfully")

	return nil
}

// Store stores a new memory with non-blocking enrichment.
// It synchronously writes the memory to storage (<5ms) and queues it for
// async enrichment, then returns immediately (<10ms total).
//
// The memory is initially stored with StatusPending. Worker goroutines
// will process the enrichment asynchronously and update the status.
func (e *MemoryEngine) Store(ctx context.Context, content string) (*types.Memory, error) {
	e.mu.RLock()
	if !e.started {
		e.mu.RUnlock()
		return nil, fmt.Errorf("engine not started")
	}
	e.mu.RUnlock()

	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	// Create memory with unique ID
	memory := &types.Memory{
		ID:                 GenerateMemoryID("", ""),
		Content:            content,
		Status:             types.StatusPending,
		EntityStatus:       types.EnrichmentPending,
		RelationshipStatus: types.EnrichmentPending,
		EmbeddingStatus:    types.EnrichmentPending,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	// Fast synchronous storage (<5ms)
	if err := e.memoryStore.Store(ctx, memory); err != nil {
		return nil, fmt.Errorf("failed to store memory: %w", err)
	}

	// Notify listeners that a new memory was created
	if e.onMemoryCreated != nil {
		e.onMemoryCreated(memory.ID)
	}

	// Queue async enrichment (non-blocking)
	job := e.createEnrichmentJob(memory.ID, content, 0)

	if !e.queueEnrichmentJob(job) {
		// Queue is full - mark as failed for manual retry
		if err := e.memoryStore.UpdateStatus(ctx, memory.ID, types.StatusFailed); err != nil {
			log.Printf("ERROR: Failed to mark memory %s as failed: %v", memory.ID, err)
		}
		return memory, fmt.Errorf("enrichment queue full, memory stored but not queued")
	}

	return memory, nil
}

// Get retrieves a memory by ID.
func (e *MemoryEngine) Get(ctx context.Context, id string) (*types.Memory, error) {
	return e.memoryStore.Get(ctx, id)
}

// List retrieves memories with pagination and filtering.
func (e *MemoryEngine) List(ctx context.Context, opts storage.ListOptions) (*storage.PaginatedResult[types.Memory], error) {
	return e.memoryStore.List(ctx, opts)
}

// Shutdown gracefully shuts down the memory engine.
// It closes the enrichment queue and waits for workers to drain (with timeout).
// Any pending jobs in the queue will be processed before shutdown completes.
func (e *MemoryEngine) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.started {
		return fmt.Errorf("engine not started")
	}

	log.Println("Shutting down memory engine...")

	// Mark as shutting down (prevents requeueing)
	e.shuttingDown = true

	// Cancel worker context (stops workers from requeueing)
	if e.workerCancel != nil {
		e.workerCancel()
	}

	// Stop worker pool gracefully
	if err := e.stopWorkerPool(ctx); err != nil {
		log.Printf("WARNING: Worker pool shutdown had errors: %v", err)
	}

	e.started = false
	e.shuttingDown = false
	log.Println("Memory engine shut down successfully")

	return nil
}

// Search performs intelligent memory search with relevance scoring.
// It delegates to the SearchOrchestrator for advanced search capabilities.
func (e *MemoryEngine) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.started {
		return nil, fmt.Errorf("engine not started")
	}

	return e.searchOrchestrator.Search(ctx, opts)
}

// SearchSimilar finds memories similar to a given memory.
// It delegates to the SearchOrchestrator for similarity search.
func (e *MemoryEngine) SearchSimilar(ctx context.Context, memoryID string, limit int) ([]SearchResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.started {
		return nil, fmt.Errorf("engine not started")
	}

	return e.searchOrchestrator.SearchSimilar(ctx, memoryID, limit)
}

// InferConnections discovers implicit connections between memories.
// It delegates to the InferenceEngine for graph-based reasoning.
func (e *MemoryEngine) InferConnections(ctx context.Context, memoryID string, opts InferenceOptions) ([]InferenceResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.started {
		return nil, fmt.Errorf("engine not started")
	}

	return e.inferenceEngine.InferConnections(ctx, memoryID, opts)
}

// FindPatterns identifies recurring patterns across memories.
// It delegates to the InferenceEngine for pattern discovery.
func (e *MemoryEngine) FindPatterns(ctx context.Context, domain string) ([]Pattern, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.started {
		return nil, fmt.Errorf("engine not started")
	}

	return e.inferenceEngine.FindPatterns(ctx, domain)
}

// ApplyDecay applies exponential decay to all memories.
// Bulk decay across all stored memories is not yet implemented.
func (e *MemoryEngine) ApplyDecay(ctx context.Context) (int, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.started {
		return 0, fmt.Errorf("engine not started")
	}

	// Bulk decay across all memories is not yet implemented.
	// Individual memories can be decayed via the DecayManager directly.
	return 0, fmt.Errorf("bulk ApplyDecay not yet implemented")
}

// BoostMemory increases importance due to usage.
// Per-memory usage boosting is not yet implemented.
func (e *MemoryEngine) BoostMemory(ctx context.Context, memoryID string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.started {
		return fmt.Errorf("engine not started")
	}

	// BoostMemory is not yet implemented on the current DecayManager.
	return fmt.Errorf("BoostMemory not yet implemented")
}

// UpdateConfidence recalculates and stores confidence for a memory.
// It delegates to the ConfidenceScorer for multi-factor scoring.
func (e *MemoryEngine) UpdateConfidence(ctx context.Context, memoryID string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.started {
		return fmt.Errorf("engine not started")
	}

	return e.confidenceScorer.UpdateConfidence(ctx, memoryID)
}

// GetQueueSize returns the current number of jobs in the enrichment queue.
func (e *MemoryEngine) GetQueueSize() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.enrichmentQueue)
}

// llmConfigFromGlobal maps the global application config to a connections.LLMConfig
// that the factory functions can consume.
func llmConfigFromGlobal(cfg *config.Config) connections.LLMConfig {
	switch cfg.LLM.LLMProvider {
	case "openai":
		return connections.LLMConfig{
			Provider: "openai",
			APIKey:   cfg.LLM.OpenAIAPIKey,
			Model:    cfg.LLM.OpenAIModel,
		}
	case "anthropic":
		return connections.LLMConfig{
			Provider: "anthropic",
			APIKey:   cfg.LLM.AnthropicAPIKey,
			Model:    cfg.LLM.AnthropicModel,
		}
	default:
		return connections.LLMConfig{
			Provider: "ollama",
			BaseURL:  cfg.LLM.OllamaURL,
			Model:    cfg.LLM.OllamaModel,
		}
	}
}
