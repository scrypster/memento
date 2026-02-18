package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scrypster/memento/internal/attribution"
	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// memoryEngine is the subset of engine.MemoryEngine used by the MCP server.
// Using an interface keeps the MCP package loosely coupled and testable.
type memoryEngine interface {
	QueueEnrichmentForMemory(memoryID, content string) bool
	Embed(ctx context.Context, text string) ([]float64, error)
	Summarize(ctx context.Context, prompt string) (string, error)
}

// Server implements the Model Context Protocol (MCP) for Memento.
// It provides JSON-RPC 2.0 based tools for AI assistants to interact
// with the memory system.
type Server struct {
	memoryStore        storage.MemoryStore
	searchProvider     storage.SearchProvider
	config             *config.Config
	detector           *engine.ContradictionDetector
	connectionManager  *connections.Manager
	engine             memoryEngine
	defaultConnection  string // connection used when no connection_id is provided
	sessionID          string // unique ID generated once per MCP server lifetime
}

// ServerOption is a functional option for configuring a Server.
// Using the functional options pattern means that existing call sites that
// call NewServer(store) continue to work without modification, while new
// callers can opt-in to additional configuration via option functions.
type ServerOption func(*Server)

// WithConfig injects a *config.Config into the Server.
// When this option is not provided the server's config field is nil, so
// callers that depend on the config should always supply this option.
func WithConfig(cfg *config.Config) ServerOption {
	return func(s *Server) {
		s.config = cfg
	}
}

// WithSearchProvider injects a storage.SearchProvider into the Server.
// When provided, FindRelated will use FTS5 full-text search instead of the
// slower in-memory strings.Contains fallback.
func WithSearchProvider(sp storage.SearchProvider) ServerOption {
	return func(s *Server) {
		s.searchProvider = sp
	}
}

// WithConnectionManager injects a *connections.Manager into the Server.
// When provided, store_memory calls with a connection_id will route writes
// to the appropriate per-connection store and set the memory domain to match.
func WithConnectionManager(cm *connections.Manager) ServerOption {
	return func(s *Server) {
		s.connectionManager = cm
	}
}

// WithEngine injects a MemoryEngine into the Server.
// When provided, store_memory immediately queues enrichment jobs rather than
// waiting for the next startup recovery pass, and search uses hybrid
// FTS+vector ranking when embeddings are available.
func WithEngine(e memoryEngine) ServerOption {
	return func(s *Server) {
		s.engine = e
	}
}

// WithDefaultConnection sets the connection used when a tool call does not
// include an explicit connection_id. This lets users pin a global default
// (via the MEMENTO_DEFAULT_CONNECTION env var) or project-level default
// (via CLAUDE.md / cursor rules) without passing connection_id every time.
// When empty, the server falls back to the store injected at construction.
func WithDefaultConnection(name string) ServerOption {
	return func(s *Server) {
		s.defaultConnection = name
	}
}

// NewServer creates a new MCP server instance.
//
// The variadic opts parameter accepts zero or more ServerOption values.
// Passing no options is valid and preserves backward compatibility with all
// existing call sites:
//
//	srv := mcp.NewServer(store)                        // existing call sites — unchanged
//	srv := mcp.NewServer(store, mcp.WithConfig(cfg))   // new call sites — with config
func NewServer(store storage.MemoryStore, opts ...ServerOption) *Server {
	s := &Server{
		memoryStore: store,
		detector:    engine.NewContradictionDetector(store),
		sessionID:   uuid.New().String(),
	}
	for _, opt := range opts {
		opt(s)
	}
	// If no explicit SearchProvider was injected, check whether the MemoryStore
	// itself also implements SearchProvider (e.g. *sqlite.MemoryStore does).
	// This keeps existing call sites working without any changes.
	if s.searchProvider == nil {
		if sp, ok := store.(storage.SearchProvider); ok {
			s.searchProvider = sp
		}
	}
	log.Printf("memento-mcp: session ID: %s", s.sessionID)
	return s
}

// Config returns the configuration that was injected via WithConfig, or nil
// if no config option was provided.
func (s *Server) Config() *config.Config {
	return s.config
}

// HandleRequest processes a JSON-RPC 2.0 request and returns a response.
// This is the main entry point for MCP protocol handling.
func (s *Server) HandleRequest(ctx context.Context, requestJSON []byte) ([]byte, error) {
	var req JSONRPCRequest
	if err := json.Unmarshal(requestJSON, &req); err != nil {
		return s.errorResponse(nil, ErrCodeParseError, "Parse error", err)
	}

	// Validate JSON-RPC version
	if req.JSONRPC != "2.0" {
		return s.errorResponse(req.ID, ErrCodeInvalidRequest, "Invalid JSON-RPC version", nil)
	}

	// Route to appropriate handler
	var result interface{}
	var err error

	switch req.Method {
	// Standard MCP protocol methods
	case "initialize":
		result, err = s.handleInitialize(ctx, req.Params)
	case "initialized":
		// Notification — no response body required; return empty object.
		result = map[string]interface{}{}
	case "tools/list":
		result, err = s.handleToolsList(ctx, req.Params)
	case "tools/call":
		result, err = s.handleToolsCall(ctx, req.Params)

	// Native JSON-RPC methods (kept for backward-compat with direct callers)
	case "store_memory":
		result, err = s.handleStoreMemory(ctx, req.Params)
	case "recall_memory":
		result, err = s.handleRecallMemory(ctx, req.Params)
	case "find_related":
		result, err = s.handleFindRelated(ctx, req.Params)
	case "retry_enrichment":
		result, err = s.handleRetryEnrichment(ctx, req.Params)
	case "explain_reasoning":
		result, err = s.handleExplainReasoning(ctx, req.Params)
	case "update_memory_state":
		result, err = s.handleUpdateMemoryState(ctx, req.Params)
	case "forget_memory":
		result, err = s.handleForgetMemory(ctx, req.Params)
	case "evolve_memory":
		result, err = s.handleEvolveMemory(ctx, req.Params)
	case "consolidate_memories":
		result, err = s.handleConsolidateMemories(ctx, req.Params)
	case "detect_contradictions":
		result, err = s.handleDetectContradictions(ctx, req.Params)
	case "update_memory":
		result, err = s.handleUpdateMemory(ctx, req.Params)
	case "get_session_context":
		result, err = s.handleGetSessionContext(ctx, req.Params)
	case "traverse_memory_graph":
		result, err = s.handleTraverseMemoryGraph(ctx, req.Params)
	case "restore_memory":
		result, err = s.handleRestoreMemory(ctx, req.Params)
	case "list_deleted_memories":
		result, err = s.handleListDeletedMemories(ctx, req.Params)
	case "get_evolution_chain":
		result, err = s.handleGetEvolutionChain(ctx, req.Params)
	case "create_project":
		result, err = s.handleCreateProject(ctx, req.Params)
	case "add_project_item":
		result, err = s.handleAddProjectItem(ctx, req.Params)
	case "get_project_tree":
		result, err = s.handleGetProjectTree(ctx, req.Params)
	case "list_projects":
		result, err = s.handleListProjects(ctx, req.Params)
	default:
		return s.errorResponse(req.ID, ErrCodeMethodNotFound, fmt.Sprintf("Method not found: %s", req.Method), nil)
	}

	if err != nil {
		return s.errorResponse(req.ID, ErrCodeServerError, err.Error(), nil)
	}

	return s.successResponse(req.ID, result)
}

// StoreMemory stores a new memory and returns immediately with pending status.
// This is the v2.0 behavior where enrichment happens asynchronously.
func (s *Server) StoreMemory(ctx context.Context, args StoreMemoryArgs) (*StoreMemoryResult, error) {
	// Validate input
	if err := s.validateStoreMemoryArgs(args); err != nil {
		return nil, err
	}

	// Resolve the effective connection name. Priority:
	//   1. args.ConnectionID (explicit per-call)
	//   2. args.Domain (legacy field)
	//   3. s.defaultConnection (server-level default, set via env var or per-project CLAUDE.md)
	//   4. "" → "general" (original default)
	effectiveConn := args.ConnectionID
	if effectiveConn == "" {
		effectiveConn = args.Domain
	}
	if effectiveConn == "" {
		effectiveConn = s.defaultConnection
	}

	// The domain segment of the memory ID matches the connection name so that
	// resolveStoreForID can route ID-based lookups back to the right store.
	domain := effectiveConn

	// Resolve which store to write to.
	store := s.memoryStore
	if effectiveConn != "" && s.connectionManager != nil {
		if connStore, err := s.connectionManager.GetStore(effectiveConn); err == nil {
			store = connStore
		} else if args.ConnectionID != "" {
			// Only hard-fail for an explicitly requested connection that doesn't exist.
			return nil, fmt.Errorf("unknown connection %q: %w", args.ConnectionID, err)
		}
	}

	// Generate memory ID
	memID := s.generateMemoryID(domain, args.Content)

	// Create memory with pending status
	memory := &types.Memory{
		ID:                 memID,
		Content:            args.Content,
		Source:             args.Source,
		Domain:             domain,
		Tags:               args.Tags,
		Metadata:           args.Metadata,
		Status:             types.StatusPending,
		EntityStatus:       types.EnrichmentPending,
		RelationshipStatus: types.EnrichmentPending,
		EmbeddingStatus:    types.EnrichmentPending,
		Timestamp:          time.Now(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	// Set created_by: use explicit arg if provided, otherwise auto-detect
	if args.CreatedBy != "" {
		memory.CreatedBy = args.CreatedBy
	} else {
		memory.CreatedBy = attribution.DetectAgent()
	}

	// Set session_id: use explicit arg override if provided, otherwise use server session ID
	if args.SessionID != "" {
		memory.SessionID = args.SessionID
	} else {
		memory.SessionID = s.sessionID
	}


	// Detect duplicate: same content produces the same deterministic ID via
	// generateMemoryID. If the record already exists, Get() will succeed
	// before Store() runs. We check before storing to distinguish new vs existing.
	wasDuplicate := false
	if _, err := store.Get(ctx, memID); err == nil {
		wasDuplicate = true
	}

	// Store memory (upsert — safe to call even for duplicates)
	if err := store.Store(ctx, memory); err != nil {
		return nil, fmt.Errorf("failed to store memory: %w", err)
	}

	result := &StoreMemoryResult{
		ID:     memory.ID,
		Status: types.StatusPending,
	}

	if wasDuplicate {
		result.Duplicate = true
		result.ExistingID = memory.ID
		result.Message = "Memory already exists with identical content."
	} else {
		result.Message = "Memory stored successfully. Enrichment will happen asynchronously."
		// Queue enrichment immediately if engine is available (only for new memories).
		if s.engine != nil {
			if !s.engine.QueueEnrichmentForMemory(memID, args.Content) {
				// Queue full — recovery on next restart will pick it up
			}
		}

		// Onboarding hint: if this is the very first memory, guide the user.
		countResult, countErr := store.List(ctx, storage.ListOptions{Limit: 1})
		if countErr == nil && countResult.Total == 1 {
			result.Message += " First memory stored! Tip: store a few more memories then use find_related to discover connections."
		}
	}

	return result, nil
}

// RecallMemory retrieves memories with three priority modes:
//  1. ID set → direct lookup by ID
//  2. Query set → full-text search (delegates to FTS, same engine as find_related)
//  3. Neither → list/filter mode with optional filter fields
func (s *Server) RecallMemory(ctx context.Context, args RecallMemoryArgs) (*RecallMemoryResult, error) {
	// ------------------------------------------------------------------
	// ID-lookup mode: auto-route to the connection inferred from the ID.
	// ------------------------------------------------------------------
	if args.ID != "" {
		store := s.resolveStoreForID(args.ID)
		memory, err := store.Get(ctx, args.ID)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return &RecallMemoryResult{Found: false}, nil
			}
			return nil, fmt.Errorf("failed to retrieve memory: %w", err)
		}

		// Track access (Opus Issue #3): call synchronously, it's fast enough.
		if incErr := store.IncrementAccessCount(ctx, memory.ID); incErr != nil {
			// Non-fatal: log but do not fail the recall.
			_ = incErr
		}

		return &RecallMemoryResult{
			Memory: memory,
			Found:  true,
		}, nil
	}

	// ------------------------------------------------------------------
	// Query/search mode — delegates to FTS when a query string is provided.
	// Passes connection_id through so the right store is searched.
	// ------------------------------------------------------------------
	if args.Query != "" {
		limit := args.Limit
		if limit <= 0 {
			limit = 10
		}
		ftsArgs := FindRelatedArgs{
			Query:        args.Query,
			Limit:        limit,
			ConnectionID: args.ConnectionID,
		}
		ftsResult, err := s.FindRelated(ctx, ftsArgs)
		if err != nil {
			return nil, err
		}
		return &RecallMemoryResult{
			Found:    len(ftsResult.Memories) > 0,
			Memories: ftsResult.Memories,
			Total:    ftsResult.Total,
			Page:     1,
		}, nil
	}

	// ------------------------------------------------------------------
	// List-filter mode — scoped to connection_id when provided.
	// ------------------------------------------------------------------

	// Resolve store for this connection.
	listStore, _ := s.resolveSearchStore(args.ConnectionID)

	// Parse and validate temporal bounds.
	var createdAfter, createdBefore time.Time

	if args.CreatedAfter != "" {
		t, err := time.Parse(time.RFC3339, args.CreatedAfter)
		if err != nil {
			return nil, fmt.Errorf("created_after: invalid RFC-3339 timestamp %q: %w", args.CreatedAfter, err)
		}
		createdAfter = t
	}

	if args.CreatedBefore != "" {
		t, err := time.Parse(time.RFC3339, args.CreatedBefore)
		if err != nil {
			return nil, fmt.Errorf("created_before: invalid RFC-3339 timestamp %q: %w", args.CreatedBefore, err)
		}
		createdBefore = t
	}

	// Validate that createdAfter < createdBefore when both are set.
	if !createdAfter.IsZero() && !createdBefore.IsZero() {
		if !createdAfter.Before(createdBefore) {
			return nil, fmt.Errorf("created_after (%s) must be before created_before (%s)",
				createdAfter.Format(time.RFC3339), createdBefore.Format(time.RFC3339))
		}
	}

	opts := storage.ListOptions{
		Page:          args.Page,
		Limit:         args.Limit,
		State:         args.State,
		CreatedBy:     args.CreatedBy,
		CreatedAfter:  createdAfter,
		CreatedBefore: createdBefore,
		MinDecayScore: args.MinDecayScore,
	}
	opts.Normalize()

	result, err := listStore.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}

	return &RecallMemoryResult{
		Found:    false,
		Memories: result.Items,
		Total:    result.Total,
		Page:     result.Page,
		HasMore:  result.HasMore,
	}, nil
}

// FindRelated finds memories related to a query.
// For v2.0, this uses simple text-based filtering with optional temporal bounds.
// Future versions will use vector search and semantic matching.
func (s *Server) FindRelated(ctx context.Context, args FindRelatedArgs) (*FindRelatedResult, error) {
	// Validate input
	if err := s.validateFindRelatedArgs(args); err != nil {
		return nil, err
	}

	// Parse and validate temporal bounds.
	var createdAfter, createdBefore time.Time

	if args.CreatedAfter != "" {
		t, err := time.Parse(time.RFC3339, args.CreatedAfter)
		if err != nil {
			return nil, fmt.Errorf("created_after: invalid RFC-3339 timestamp %q: %w", args.CreatedAfter, err)
		}
		createdAfter = t
	}

	if args.CreatedBefore != "" {
		t, err := time.Parse(time.RFC3339, args.CreatedBefore)
		if err != nil {
			return nil, fmt.Errorf("created_before: invalid RFC-3339 timestamp %q: %w", args.CreatedBefore, err)
		}
		createdBefore = t
	}

	// Validate that createdAfter < createdBefore when both are set.
	if !createdAfter.IsZero() && !createdBefore.IsZero() {
		if !createdAfter.Before(createdBefore) {
			return nil, fmt.Errorf("created_after (%s) must be before created_before (%s)",
				createdAfter.Format(time.RFC3339), createdBefore.Format(time.RFC3339))
		}
	}

	// Set default limit
	limit := args.Limit
	if limit <= 0 {
		limit = 10
	}

	// Resolve the store and search provider for this call.
	// When connection_id is set the search is scoped to that connection's data.
	callStore, callSearchProvider := s.resolveSearchStore(args.ConnectionID)

	// Use search when a SearchProvider is available.
	// Prefer hybrid (FTS + vector) search when engine embedding is available.
	if callSearchProvider != nil {
		searchOpts := storage.SearchOptions{
			Query:         args.Query,
			Limit:         limit,
			Offset:        0,
			FuzzyFallback: true,
		}

		var ftsResult *storage.PaginatedResult[types.Memory]
		var err error

		// Try hybrid search first (FTS + semantic vector)
		if s.engine != nil {
			if vec, embErr := s.engine.Embed(ctx, args.Query); embErr == nil {
				ftsResult, err = callSearchProvider.HybridSearch(ctx, args.Query, vec, searchOpts)
			}
		}
		// Fall back to FTS-only if hybrid unavailable or failed
		if ftsResult == nil {
			ftsResult, err = callSearchProvider.FullTextSearch(ctx, searchOpts)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to search memories: %w", err)
		}

		// Apply temporal bounds filter post-search (FTS5 searches content only).
		var filtered []types.Memory
		for _, mem := range ftsResult.Items {
			if !createdAfter.IsZero() && !mem.CreatedAt.After(createdAfter) {
				continue
			}
			if !createdBefore.IsZero() && !mem.CreatedAt.Before(createdBefore) {
				continue
			}
			if args.Domain != "" && mem.Domain != args.Domain {
				continue
			}
			filtered = append(filtered, mem)
		}

		// Track access for each returned memory (Opus Issue #3).
		for _, mem := range filtered {
			if incErr := callStore.IncrementAccessCount(ctx, mem.ID); incErr != nil {
				_ = incErr
			}
		}

		return &FindRelatedResult{
			Memories: filtered,
			Total:    len(filtered),
		}, nil
	}

	// Fallback: list-then-filter using strings.Contains (no SearchProvider available).
	listOpts := storage.ListOptions{
		Limit:         limit,
		Page:          1,
		CreatedAfter:  createdAfter,
		CreatedBefore: createdBefore,
	}

	if args.Domain != "" {
		listOpts.Filter = map[string]interface{}{
			"domain": args.Domain,
		}
	}

	result, err := callStore.List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}

	// Filter by query (simple text matching fallback)
	var filtered []types.Memory
	queryLower := strings.ToLower(args.Query)

	for _, mem := range result.Items {
		content := strings.ToLower(mem.Content)
		if strings.Contains(content, queryLower) {
			filtered = append(filtered, mem)
		}
	}

	// Track access for each returned memory (Opus Issue #3).
	for _, mem := range filtered {
		if incErr := callStore.IncrementAccessCount(ctx, mem.ID); incErr != nil {
			// Non-fatal: continue tracking remaining memories.
			_ = incErr
		}
	}

	return &FindRelatedResult{
		Memories: filtered,
		Total:    len(filtered),
	}, nil
}

// RetryEnrichment retries enrichment for a failed memory.
func (s *Server) RetryEnrichment(ctx context.Context, args RetryEnrichmentArgs) (*RetryEnrichmentResult, error) {
	// Validate input
	if args.ID == "" {
		return nil, errors.New("memory ID is required")
	}

	// Auto-route to the connection that owns this memory ID.
	store := s.resolveStoreForID(args.ID)

	// Retrieve memory
	memory, err := store.Get(ctx, args.ID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("memory not found: %s", args.ID)
		}
		return nil, fmt.Errorf("failed to retrieve memory: %w", err)
	}

	// Check if memory can be retried
	if memory.Status != types.StatusFailed {
		return &RetryEnrichmentResult{
			ID:      args.ID,
			Queued:  false,
			Message: fmt.Sprintf("Memory is not in failed state (current: %s)", memory.Status),
		}, nil
	}

	// Reset status to pending for retry
	if err := store.UpdateStatus(ctx, args.ID, types.StatusPending); err != nil {
		return nil, fmt.Errorf("failed to update memory status: %w", err)
	}

	// Reset enrichment statuses
	enrichment := storage.EnrichmentUpdate{
		EntityStatus:       types.EnrichmentPending,
		RelationshipStatus: types.EnrichmentPending,
		EmbeddingStatus:    types.EnrichmentPending,
		EnrichmentAttempts: memory.EnrichmentAttempts, // Keep existing attempts
		EnrichmentError:    "",                        // Clear error
	}

	if err := store.UpdateEnrichment(ctx, args.ID, enrichment); err != nil {
		return nil, fmt.Errorf("failed to update enrichment status: %w", err)
	}

	// Queue enrichment immediately if engine is available.
	if s.engine != nil {
		s.engine.QueueEnrichmentForMemory(args.ID, memory.Content)
	}

	return &RetryEnrichmentResult{
		ID:      args.ID,
		Queued:  true,
		Message: "Memory queued for enrichment retry",
	}, nil
}

// ExplainReasoning explains why certain memories were retrieved for a query.
// It fetches each memory by ID and identifies which query terms matched.
func (s *Server) ExplainReasoning(ctx context.Context, args ExplainReasoningArgs) (*ExplainReasoningResult, error) {
	if args.Query == "" {
		return nil, errors.New("query is required")
	}
	if len(args.MemoryIDs) == 0 {
		return nil, errors.New("at least one memory ID is required")
	}

	// Fetch each requested memory, auto-routing each ID to its connection.
	var fetched []*types.Memory
	var notFound []string
	for _, id := range args.MemoryIDs {
		mem, err := s.resolveStoreForID(id).Get(ctx, id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				notFound = append(notFound, id)
				continue
			}
			return nil, fmt.Errorf("failed to retrieve memory %s: %w", id, err)
		}
		fetched = append(fetched, mem)
	}

	// Tokenise the query into lowercase words for term-matching.
	queryTerms := strings.Fields(strings.ToLower(args.Query))

	// Build one reasoning step per fetched memory.
	var steps []ReasoningStep

	steps = append(steps, ReasoningStep{
		Step:        "Query",
		Description: fmt.Sprintf("%q → %d term(s): %s", args.Query, len(queryTerms), strings.Join(queryTerms, ", ")),
	})

	for _, mem := range fetched {
		lower := strings.ToLower(mem.Content)
		var matched []string
		for _, term := range queryTerms {
			if strings.Contains(lower, term) {
				matched = append(matched, term)
			}
		}
		matchStr := "no direct term overlap"
		if len(matched) > 0 {
			matchStr = fmt.Sprintf("matched terms: %s", strings.Join(matched, ", "))
		}
		steps = append(steps, ReasoningStep{
			Step:        "Memory",
			Description: fmt.Sprintf("[%s] %s — %s", mem.ID, mem.Content, matchStr),
		})
	}

	if len(notFound) > 0 {
		steps = append(steps, ReasoningStep{
			Step:        "Not Found",
			Description: fmt.Sprintf("IDs not found in store: %s", strings.Join(notFound, ", ")),
		})
	}

	explanation := fmt.Sprintf("Explaining %d of %d requested memories for query: %q", len(fetched), len(args.MemoryIDs), args.Query)

	return &ExplainReasoningResult{
		Explanation: explanation,
		Reasoning:   steps,
	}, nil
}

// UpdateMemoryState updates the lifecycle state of a memory with state transition validation (Opus Issue #6).
func (s *Server) UpdateMemoryState(ctx context.Context, args UpdateMemoryStateArgs) (*UpdateMemoryStateResult, error) {
	// Validate input
	if args.ID == "" {
		return nil, errors.New("memory ID is required")
	}
	if args.State == "" {
		return nil, errors.New("state is required")
	}

	// Validate that the state is a valid lifecycle state
	if !types.IsValidLifecycleState(args.State) {
		return nil, fmt.Errorf("invalid state: %s", args.State)
	}

	// Auto-route to the connection that owns this memory ID.
	store := s.resolveStoreForID(args.ID)

	// Get the current memory to capture previous state
	memory, err := store.Get(ctx, args.ID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("memory not found: %s", args.ID)
		}
		return nil, fmt.Errorf("failed to retrieve memory: %w", err)
	}

	previousState := memory.State

	// Update the state (state transition validation happens in the store)
	if err := store.UpdateState(ctx, args.ID, args.State); err != nil {
		return nil, fmt.Errorf("failed to update state: %w", err)
	}

	return &UpdateMemoryStateResult{
		ID:            args.ID,
		PreviousState: previousState,
		NewState:      args.State,
		Message:       fmt.Sprintf("Successfully transitioned from '%s' to '%s'", previousState, args.State),
	}, nil
}

// ForgetMemory soft-deletes or permanently purges a memory.
func (s *Server) ForgetMemory(ctx context.Context, args ForgetMemoryArgs) (*ForgetMemoryResult, error) {
	if args.ID == "" {
		return nil, errors.New("id is required")
	}

	store := s.resolveStoreForID(args.ID)

	if args.HardDelete {
		// Permanent removal
		if err := store.Purge(ctx, args.ID); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return nil, fmt.Errorf("memory not found: %s", args.ID)
			}
			return nil, fmt.Errorf("failed to purge memory: %w", err)
		}
		return &ForgetMemoryResult{ID: args.ID, Deleted: true, Purged: true}, nil
	}

	// Soft delete
	if err := store.Delete(ctx, args.ID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("memory not found: %s", args.ID)
		}
		return nil, fmt.Errorf("failed to forget memory: %w", err)
	}
	return &ForgetMemoryResult{ID: args.ID, Deleted: true}, nil
}

// EvolveMemory creates a new version of a memory that supersedes the old one.
func (s *Server) EvolveMemory(ctx context.Context, args EvolveMemoryArgs) (*EvolveMemoryResult, error) {
	if args.ID == "" || args.NewContent == "" {
		return nil, errors.New("id and new_content are required")
	}

	// Auto-route to the connection that owns this memory ID.
	store := s.resolveStoreForID(args.ID)

	// Get the old memory to verify it exists and copy its metadata
	old, err := store.Get(ctx, args.ID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("memory not found: %s", args.ID)
		}
		return nil, fmt.Errorf("failed to retrieve memory to evolve: %w", err)
	}

	// Create the new memory that supersedes the old one
	newID := "mem:" + uuid.New().String()
	newMem := &types.Memory{
		ID:                  newID,
		Content:             args.NewContent,
		Source:              old.Source,
		Domain:              old.Domain,
		Tags:                old.Tags,
		Metadata:            old.Metadata,
		SupersedesID:        old.ID,
		CreatedBy:           attribution.DetectAgent(),
		SessionID:           s.sessionID,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		Timestamp:           time.Now(),
		Status:              types.StatusPending,
		EntityStatus:        types.EnrichmentPending,
		RelationshipStatus:  types.EnrichmentPending,
		ClassificationStatus: types.EnrichmentPending,
		SummarizationStatus: types.EnrichmentPending,
		EmbeddingStatus:     types.EnrichmentPending,
	}

	if err := store.Store(ctx, newMem); err != nil {
		return nil, fmt.Errorf("failed to store evolved memory: %w", err)
	}

	// Mark the old memory as superseded
	if err := store.UpdateState(ctx, old.ID, types.StateSuperseded); err != nil {
		return nil, fmt.Errorf("failed to mark old memory as superseded: %w", err)
	}

	// Queue enrichment for the new memory
	if s.engine != nil {
		s.engine.QueueEnrichmentForMemory(newID, args.NewContent)
	}

	return &EvolveMemoryResult{
		NewID:        newID,
		SupersededID: old.ID,
	}, nil
}

// ConsolidateMemories merges multiple memories into one consolidated memory.
// The originals are soft-deleted. The new memory supersedes them in a many-to-one pattern.
func (s *Server) ConsolidateMemories(ctx context.Context, args ConsolidateMemoriesArgs) (*ConsolidateMemoriesResult, error) {
	if len(args.IDs) == 0 && args.Query == "" {
		return nil, fmt.Errorf("either ids or query is required")
	}

	// Resolve the store and search provider
	store, searchProvider := s.resolveSearchStore(args.ConnectionID)

	// Collect memory IDs to consolidate
	var ids []string
	if len(args.IDs) > 0 {
		ids = args.IDs
	} else {
		// Search for candidates
		limit := args.Limit
		if limit <= 0 {
			limit = 5
		}
		if limit > 10 {
			limit = 10
		}

		// Use search provider if available, otherwise fail gracefully
		if searchProvider != nil {
			searchResult, err := searchProvider.FullTextSearch(ctx, storage.SearchOptions{
				Query: args.Query,
				Limit: limit,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to search for consolidation candidates: %w", err)
			}
			for _, m := range searchResult.Items {
				ids = append(ids, m.ID)
			}
		} else {
			return nil, fmt.Errorf("search is not available in this configuration; please provide explicit ids instead")
		}
	}

	if len(ids) < 2 {
		return nil, fmt.Errorf("at least 2 memories are required for consolidation, got %d", len(ids))
	}

	// Fetch all memories
	memories := make([]*types.Memory, 0, len(ids))
	for _, id := range ids {
		m, err := store.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch memory %s: %w", id, err)
		}
		memories = append(memories, m)
	}

	// Build consolidated content.
	// If the LLM engine is available, use it to synthesise a single coherent
	// paragraph. Otherwise fall back to numbered concatenation.
	var consolidatedContent string
	if s.engine != nil {
		var sourceParts []string
		for i, m := range memories {
			sourceParts = append(sourceParts, fmt.Sprintf("[%d] %s", i+1, m.Content))
		}
		titleLine := ""
		if args.Title != "" {
			titleLine = fmt.Sprintf("Title: %s\n\n", args.Title)
		}
		prompt := fmt.Sprintf(`You are consolidating related memories into one coherent record.

%sSources:
%s

Write a single concise paragraph that synthesises all of the above into one coherent memory. Remove redundancy. Resolve any contradictions by noting both views. Do not add new information. Respond with only the consolidated text, no preamble.`,
			titleLine, strings.Join(sourceParts, "\n"))

		if result, err := s.engine.Summarize(ctx, prompt); err == nil && result != "" {
			consolidatedContent = result
		}
	}

	// Fallback: deterministic concatenation (no LLM available or LLM failed)
	if consolidatedContent == "" {
		var parts []string
		if args.Title != "" {
			parts = append(parts, "# "+args.Title)
		}
		for i, m := range memories {
			parts = append(parts, fmt.Sprintf("[%d] %s", i+1, m.Content))
		}
		consolidatedContent = strings.Join(parts, "\n\n")
	}

	// Collect tags from all source memories (union)
	tagSet := make(map[string]bool)
	for _, m := range memories {
		for _, t := range m.Tags {
			tagSet[t] = true
		}
	}
	var allTags []string
	for t := range tagSet {
		allTags = append(allTags, t)
	}
	sort.Strings(allTags) // Deterministic order

	// Store the consolidated memory
	newID := s.generateMemoryID(memories[0].Domain, consolidatedContent)
	consolidated := &types.Memory{
		ID:                   newID,
		Content:              consolidatedContent,
		Source:               "consolidation",
		Domain:               memories[0].Domain,
		Tags:                 allTags,
		Status:               types.StatusPending,
		EntityStatus:         types.EnrichmentPending,
		RelationshipStatus:   types.EnrichmentPending,
		ClassificationStatus: types.EnrichmentPending,
		SummarizationStatus:  types.EnrichmentPending,
		EmbeddingStatus:      types.EnrichmentPending,
		CreatedBy:            attribution.DetectAgent(),
		SessionID:            s.sessionID,
		Timestamp:            time.Now(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	if err := store.Store(ctx, consolidated); err != nil {
		return nil, fmt.Errorf("failed to store consolidated memory: %w", err)
	}

	// Soft-delete the originals
	for _, id := range ids {
		if err := store.Delete(ctx, id); err != nil {
			// Non-fatal: log and continue
			log.Printf("consolidate_memories: failed to soft-delete %s: %v", id, err)
		}
	}

	// Queue enrichment for the consolidated memory
	if s.engine != nil {
		s.engine.QueueEnrichmentForMemory(newID, consolidatedContent)
	}

	return &ConsolidateMemoriesResult{
		NewID:           newID,
		ConsolidatedIDs: ids,
		Content:         consolidatedContent,
		Message:         fmt.Sprintf("Consolidated %d memories into %s. Originals soft-deleted.", len(ids), newID),
	}, nil
}

// UpdateMemory updates the content, tags, or metadata of an existing memory.
func (s *Server) UpdateMemory(ctx context.Context, args UpdateMemoryArgs) (*UpdateMemoryResult, error) {
	if args.ID == "" {
		return nil, errors.New("id is required")
	}
	if args.Content == "" && args.Tags == nil && args.Metadata == nil {
		return nil, errors.New("at least one of content, tags, or metadata must be provided")
	}

	// Auto-route to the connection that owns this memory ID.
	store := s.resolveStoreForID(args.ID)

	memory, err := store.Get(ctx, args.ID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("memory not found: %s", args.ID)
		}
		return nil, fmt.Errorf("failed to retrieve memory: %w", err)
	}

	if args.Content != "" {
		memory.Content = args.Content
	}
	if args.Tags != nil {
		memory.Tags = args.Tags
	}
	if args.Metadata != nil {
		memory.Metadata = args.Metadata
	}

	if err := store.Update(ctx, memory); err != nil {
		return nil, fmt.Errorf("failed to update memory: %w", err)
	}

	return &UpdateMemoryResult{
		ID:      args.ID,
		Updated: true,
		Message: "Memory updated successfully",
	}, nil
}

// GetSessionContext returns recent memories from the current or specified session.
// It is useful for answering "where did I leave off?" queries.
func (s *Server) GetSessionContext(ctx context.Context, args GetSessionContextArgs) (*GetSessionContextResult, error) {
	sessionID := args.SessionID
	if sessionID == "" {
		sessionID = s.sessionID
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}

	listStore, _ := s.resolveSearchStore(args.ConnectionID)

	opts := storage.ListOptions{
		Limit:     limit,
		SessionID: sessionID,
		SortBy:    "created_at",
		SortOrder: "desc",
	}

	// Apply time window filter if specified.
	if args.TimeWindowH > 0 {
		after := time.Now().Add(-time.Duration(args.TimeWindowH) * time.Hour)
		opts.CreatedAfter = after
	}

	opts.Normalize()

	result, err := listStore.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("get session context: %w", err)
	}

	// Build topic summary by domain.
	domainCount := make(map[string]int)
	for _, m := range result.Items {
		domain := m.Domain
		if domain == "" {
			domain = "general"
		}
		domainCount[domain]++
	}

	var topics []SessionTopicSummary
	for domain, count := range domainCount {
		topics = append(topics, SessionTopicSummary{Domain: domain, Count: count})
	}
	// Sort topics by count descending, then alphabetically for stable output.
	sort.Slice(topics, func(i, j int) bool {
		if topics[i].Count != topics[j].Count {
			return topics[i].Count > topics[j].Count
		}
		return topics[i].Domain < topics[j].Domain
	})

	// Build human-readable summary string.
	summary := fmt.Sprintf("%d memories in this session", len(result.Items))
	if len(topics) > 0 {
		topicStrs := make([]string, 0, len(topics))
		for _, t := range topics {
			topicStrs = append(topicStrs, fmt.Sprintf("%s (%d)", t.Domain, t.Count))
		}
		summary += ": " + strings.Join(topicStrs, ", ")
	}

	sessionResult := &GetSessionContextResult{
		SessionID:   sessionID,
		MemoryCount: len(result.Items),
		Memories:    result.Items,
		Topics:      topics,
		Summary:     summary,
	}

	// Onboarding hint: if the store is completely empty, guide the user.
	totalResult, totalErr := listStore.List(ctx, storage.ListOptions{Limit: 1})
	if totalErr == nil && totalResult.Total == 0 {
		sessionResult.OnboardingHint = "Your memory store is empty. Start by storing memories with store_memory. Tip: describe decisions, people, or projects you want to remember."
	}

	return sessionResult, nil
}

// RestoreMemory restores a soft-deleted memory.
func (s *Server) RestoreMemory(ctx context.Context, args RestoreMemoryArgs) (*RestoreMemoryResult, error) {
	if args.ID == "" {
		return nil, errors.New("id is required")
	}

	store := s.resolveStoreForID(args.ID)
	if err := store.Restore(ctx, args.ID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("memory not found or not soft-deleted: %s", args.ID)
		}
		return nil, fmt.Errorf("failed to restore memory: %w", err)
	}

	return &RestoreMemoryResult{ID: args.ID, Restored: true}, nil
}

// ListDeletedMemories returns soft-deleted memories.
func (s *Server) ListDeletedMemories(ctx context.Context, args ListDeletedMemoriesArgs) (*ListDeletedMemoriesResult, error) {
	listStore, _ := s.resolveSearchStore(args.ConnectionID)

	opts := storage.ListOptions{
		Page:           args.Page,
		Limit:          args.Limit,
		IncludeDeleted: true,
		OnlyDeleted:    true,
	}
	opts.Normalize()

	result, err := listStore.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list deleted memories: %w", err)
	}

	return &ListDeletedMemoriesResult{
		Memories: result.Items,
		Total:    result.Total,
		Page:     result.Page,
		HasMore:  result.HasMore,
	}, nil
}

// GetEvolutionChain retrieves the full version history for a memory.
func (s *Server) GetEvolutionChain(ctx context.Context, args GetEvolutionChainArgs) (*GetEvolutionChainResult, error) {
	if args.ID == "" {
		return nil, errors.New("id is required")
	}

	store := s.resolveStoreForID(args.ID)
	chain, err := store.GetEvolutionChain(ctx, args.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get evolution chain: %w", err)
	}

	entries := make([]EvolutionEntry, len(chain))
	for i, m := range chain {
		snippet := m.Content
		if len(snippet) > 200 {
			snippet = snippet[:200] + "…"
		}
		entries[i] = EvolutionEntry{
			Position:  i + 1,
			ID:        m.ID,
			Content:   snippet,
			State:     m.State,
			CreatedAt: m.CreatedAt.Format(time.RFC3339),
		}
	}

	currentID := ""
	if len(chain) > 0 {
		currentID = chain[len(chain)-1].ID
	}

	return &GetEvolutionChainResult{
		Chain:         entries,
		TotalVersions: len(entries),
		CurrentID:     currentID,
	}, nil
}

// CreateProject creates a new project memory and optionally pre-creates phases.
func (s *Server) CreateProject(ctx context.Context, args CreateProjectArgs) (*CreateProjectResult, error) {
	if args.Name == "" {
		return nil, errors.New("name is required")
	}

	// Resolve the store.
	effectiveConn := args.ConnectionID
	if effectiveConn == "" {
		effectiveConn = s.defaultConnection
	}

	store := s.memoryStore
	if effectiveConn != "" && s.connectionManager != nil {
		if connStore, err := s.connectionManager.GetStore(effectiveConn); err == nil {
			store = connStore
		}
	}

	domain := effectiveConn
	content := args.Name
	if args.Description != "" {
		content += "\n\n" + args.Description
	}

	projectID := s.generateMemoryID(domain, content)
	projectMem := &types.Memory{
		ID:                   projectID,
		Content:              content,
		Source:               "create_project",
		Domain:               domain,
		MemoryType:           "project",
		Status:               types.StatusPending,
		EntityStatus:         types.EnrichmentPending,
		RelationshipStatus:   types.EnrichmentPending,
		ClassificationStatus: types.EnrichmentPending,
		SummarizationStatus:  types.EnrichmentPending,
		EmbeddingStatus:      types.EnrichmentPending,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		Timestamp:            time.Now(),
	}

	if err := store.Store(ctx, projectMem); err != nil {
		return nil, fmt.Errorf("failed to store project: %w", err)
	}

	result := &CreateProjectResult{ProjectID: projectID}

	// Pre-create phases if requested.
	for _, phaseName := range args.PhaseNames {
		phaseContent := phaseName
		phaseID := s.generateMemoryID(domain, "phase:"+phaseContent)
		phaseMem := &types.Memory{
			ID:                   phaseID,
			Content:              phaseContent,
			Source:               "create_project",
			Domain:               domain,
			MemoryType:           "phase",
			Status:               types.StatusPending,
			EntityStatus:         types.EnrichmentPending,
			RelationshipStatus:   types.EnrichmentPending,
			ClassificationStatus: types.EnrichmentPending,
			SummarizationStatus:  types.EnrichmentPending,
			EmbeddingStatus:      types.EnrichmentPending,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
			Timestamp:            time.Now(),
		}
		if err := store.Store(ctx, phaseMem); err != nil {
			continue // non-fatal; still return project ID
		}

		// Create CONTAINS link via memory_links table.
		type memoryLinker interface {
			CreateMemoryLink(ctx context.Context, id, sourceID, targetID, linkType string) error
		}
		if ml, ok := store.(memoryLinker); ok {
			_ = ml.CreateMemoryLink(ctx, uuid.New().String(), projectID, phaseID, "CONTAINS")
		}

		result.PhaseIDs = append(result.PhaseIDs, phaseID)
	}

	return result, nil
}

// AddProjectItem adds a child item to a parent project/phase/epic.
func (s *Server) AddProjectItem(ctx context.Context, args AddProjectItemArgs) (*AddProjectItemResult, error) {
	if args.ParentID == "" {
		return nil, errors.New("parent_id is required")
	}
	if args.ItemType == "" {
		return nil, errors.New("item_type is required")
	}
	if args.Name == "" {
		return nil, errors.New("name is required")
	}

	validTypes := map[string]bool{
		"epic": true, "phase": true, "task": true, "step": true, "milestone": true,
	}
	if !validTypes[args.ItemType] {
		return nil, fmt.Errorf("invalid item_type %q: must be one of epic, phase, task, step, milestone", args.ItemType)
	}

	store := s.resolveStoreForID(args.ParentID)

	// Get the parent to inherit domain.
	parent, err := store.Get(ctx, args.ParentID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("parent memory not found: %s", args.ParentID)
		}
		return nil, fmt.Errorf("failed to retrieve parent: %w", err)
	}

	content := args.Name
	if args.Description != "" {
		content += "\n\n" + args.Description
	}

	itemID := s.generateMemoryID(parent.Domain, args.ItemType+":"+content)
	itemMem := &types.Memory{
		ID:                   itemID,
		Content:              content,
		Source:               "add_project_item",
		Domain:               parent.Domain,
		MemoryType:           args.ItemType,
		Status:               types.StatusPending,
		EntityStatus:         types.EnrichmentPending,
		RelationshipStatus:   types.EnrichmentPending,
		ClassificationStatus: types.EnrichmentPending,
		SummarizationStatus:  types.EnrichmentPending,
		EmbeddingStatus:      types.EnrichmentPending,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		Timestamp:            time.Now(),
	}

	if err := store.Store(ctx, itemMem); err != nil {
		return nil, fmt.Errorf("failed to store project item: %w", err)
	}

	// Create CONTAINS link from parent → item via memory_links table.
	type memoryLinker interface {
		CreateMemoryLink(ctx context.Context, id, sourceID, targetID, linkType string) error
	}
	if ml, ok := store.(memoryLinker); ok {
		_ = ml.CreateMemoryLink(ctx, uuid.New().String(), args.ParentID, itemID, "CONTAINS")
	}

	return &AddProjectItemResult{
		ID:       itemID,
		ParentID: args.ParentID,
		ItemType: args.ItemType,
	}, nil
}

// GetProjectTree retrieves a nested project tree by walking CONTAINS relationships.
func (s *Server) GetProjectTree(ctx context.Context, args GetProjectTreeArgs) (*GetProjectTreeResult, error) {
	if args.ProjectID == "" {
		return nil, errors.New("project_id is required")
	}

	depth := args.Depth
	if depth <= 0 {
		depth = 3
	}
	if depth > 6 {
		depth = 6
	}

	store := s.resolveStoreForID(args.ProjectID)

	root, err := store.Get(ctx, args.ProjectID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("project not found: %s", args.ProjectID)
		}
		return nil, fmt.Errorf("failed to retrieve project: %w", err)
	}

	// Recursive tree-building function.
	var buildTree func(mem *types.Memory, currentDepth int) ProjectTreeNode
	buildTree = func(mem *types.Memory, currentDepth int) ProjectTreeNode {
		// Use first line of content as name.
		name := mem.Content
		if idx := strings.Index(name, "\n"); idx >= 0 {
			name = name[:idx]
		}
		memType := mem.MemoryType
		if memType == "" {
			memType = "memory"
		}

		node := ProjectTreeNode{
			ID:    mem.ID,
			Name:  name,
			Type:  memType,
			State: mem.State,
		}

		if currentDepth <= 0 {
			return node
		}

		children, err := store.GetMemoriesByRelationType(ctx, mem.ID, "CONTAINS")
		if err != nil {
			return node
		}

		for _, child := range children {
			node.Children = append(node.Children, buildTree(child, currentDepth-1))
		}
		return node
	}

	tree := buildTree(root, depth-1)

	return &GetProjectTreeResult{Tree: tree}, nil
}

// ListProjects lists all project memories.
func (s *Server) ListProjects(ctx context.Context, args ListProjectsArgs) (*ListProjectsResult, error) {
	listStore, _ := s.resolveSearchStore(args.ConnectionID)

	opts := storage.ListOptions{
		Page:       args.Page,
		Limit:      args.Limit,
		State:      args.State,
		MemoryType: "project",
	}
	opts.Normalize()

	result, err := listStore.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	return &ListProjectsResult{
		Projects: result.Items,
		Total:    result.Total,
		Page:     result.Page,
		HasMore:  result.HasMore,
	}, nil
}

// handleStoreMemory handles the store_memory JSON-RPC method.
func (s *Server) handleStoreMemory(ctx context.Context, params interface{}) (interface{}, error) {
	var args StoreMemoryArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.StoreMemory(ctx, args)
}

// handleRecallMemory handles the recall_memory JSON-RPC method.
func (s *Server) handleRecallMemory(ctx context.Context, params interface{}) (interface{}, error) {
	var args RecallMemoryArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.RecallMemory(ctx, args)
}

// handleFindRelated handles the find_related JSON-RPC method.
func (s *Server) handleFindRelated(ctx context.Context, params interface{}) (interface{}, error) {
	var args FindRelatedArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.FindRelated(ctx, args)
}

// handleRetryEnrichment handles the retry_enrichment JSON-RPC method.
func (s *Server) handleRetryEnrichment(ctx context.Context, params interface{}) (interface{}, error) {
	var args RetryEnrichmentArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.RetryEnrichment(ctx, args)
}

// handleExplainReasoning handles the explain_reasoning JSON-RPC method.
func (s *Server) handleExplainReasoning(ctx context.Context, params interface{}) (interface{}, error) {
	var args ExplainReasoningArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.ExplainReasoning(ctx, args)
}

// handleUpdateMemoryState handles the update_memory_state JSON-RPC method.
func (s *Server) handleUpdateMemoryState(ctx context.Context, params interface{}) (interface{}, error) {
	var args UpdateMemoryStateArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.UpdateMemoryState(ctx, args)
}

// handleForgetMemory handles the forget_memory JSON-RPC method.
func (s *Server) handleForgetMemory(ctx context.Context, params interface{}) (interface{}, error) {
	var args ForgetMemoryArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.ForgetMemory(ctx, args)
}

// handleEvolveMemory handles the evolve_memory JSON-RPC method.
func (s *Server) handleEvolveMemory(ctx context.Context, params interface{}) (interface{}, error) {
	var args EvolveMemoryArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.EvolveMemory(ctx, args)
}

// handleConsolidateMemories handles the consolidate_memories JSON-RPC method.
func (s *Server) handleConsolidateMemories(ctx context.Context, params interface{}) (interface{}, error) {
	var args ConsolidateMemoriesArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.ConsolidateMemories(ctx, args)
}

// handleDetectContradictions handles the detect_contradictions JSON-RPC method.
func (s *Server) handleDetectContradictions(ctx context.Context, params interface{}) (interface{}, error) {
	var args DetectContradictionsArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.DetectContradictions(ctx, args)
}

// handleUpdateMemory handles the update_memory JSON-RPC method.
func (s *Server) handleUpdateMemory(ctx context.Context, params interface{}) (interface{}, error) {
	var args UpdateMemoryArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.UpdateMemory(ctx, args)
}

// handleGetSessionContext handles the get_session_context JSON-RPC method.
func (s *Server) handleGetSessionContext(ctx context.Context, params interface{}) (interface{}, error) {
	var args GetSessionContextArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.GetSessionContext(ctx, args)
}

// handleTraverseMemoryGraph handles the traverse_memory_graph JSON-RPC method.
// It performs a multi-hop BFS through the entity relationship graph starting
// from the specified memory and returns connected memories sorted by distance.
func (s *Server) handleTraverseMemoryGraph(ctx context.Context, params interface{}) (interface{}, error) {
	var raw map[string]interface{}
	if err := s.unmarshalParams(params, &raw); err != nil {
		return nil, err
	}

	memoryID, _ := raw["memory_id"].(string)
	if memoryID == "" {
		return nil, fmt.Errorf("memory_id is required")
	}

	maxHops := 2
	if v, ok := raw["max_hops"].(float64); ok && v > 0 {
		maxHops = int(v)
		if maxHops > 4 {
			maxHops = 4 // safety cap
		}
	}

	limit := 10
	if v, ok := raw["limit"].(float64); ok && v > 0 {
		limit = int(v)
		if limit > 50 {
			limit = 50 // safety cap
		}
	}

	// Resolve which store to use. Traverse always operates on the store that
	// owns the memory (inferred from the ID prefix), so we route the same way
	// as other ID-based operations.
	store := s.resolveStoreForID(memoryID)

	results, err := store.Traverse(ctx, memoryID, maxHops, limit)
	if err != nil {
		return nil, fmt.Errorf("graph traversal failed: %w", err)
	}

	// Format response items.
	type traversalItem struct {
		Memory         map[string]interface{} `json:"memory"`
		HopDistance    int                    `json:"hop_distance"`
		SharedEntities []string               `json:"shared_entities,omitempty"`
	}

	items := make([]traversalItem, 0, len(results))
	for _, r := range results {
		items = append(items, traversalItem{
			Memory:         memoryToMap(r.Memory),
			HopDistance:    r.HopDistance,
			SharedEntities: r.SharedEntities,
		})
	}

	return map[string]interface{}{
		"start_memory_id": memoryID,
		"total_found":     len(items),
		"max_hops_used":   maxHops,
		"results":         items,
	}, nil
}

// handleRestoreMemory handles the restore_memory JSON-RPC method.
func (s *Server) handleRestoreMemory(ctx context.Context, params interface{}) (interface{}, error) {
	var args RestoreMemoryArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.RestoreMemory(ctx, args)
}

// handleListDeletedMemories handles the list_deleted_memories JSON-RPC method.
func (s *Server) handleListDeletedMemories(ctx context.Context, params interface{}) (interface{}, error) {
	var args ListDeletedMemoriesArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.ListDeletedMemories(ctx, args)
}

// handleGetEvolutionChain handles the get_evolution_chain JSON-RPC method.
func (s *Server) handleGetEvolutionChain(ctx context.Context, params interface{}) (interface{}, error) {
	var args GetEvolutionChainArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.GetEvolutionChain(ctx, args)
}

// handleCreateProject handles the create_project JSON-RPC method.
func (s *Server) handleCreateProject(ctx context.Context, params interface{}) (interface{}, error) {
	var args CreateProjectArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.CreateProject(ctx, args)
}

// handleAddProjectItem handles the add_project_item JSON-RPC method.
func (s *Server) handleAddProjectItem(ctx context.Context, params interface{}) (interface{}, error) {
	var args AddProjectItemArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.AddProjectItem(ctx, args)
}

// handleGetProjectTree handles the get_project_tree JSON-RPC method.
func (s *Server) handleGetProjectTree(ctx context.Context, params interface{}) (interface{}, error) {
	var args GetProjectTreeArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.GetProjectTree(ctx, args)
}

// handleListProjects handles the list_projects JSON-RPC method.
func (s *Server) handleListProjects(ctx context.Context, params interface{}) (interface{}, error) {
	var args ListProjectsArgs
	if err := s.unmarshalParams(params, &args); err != nil {
		return nil, err
	}
	return s.ListProjects(ctx, args)
}

// memoryToMap converts a types.Memory to a plain map[string]interface{} for
// JSON serialisation in MCP responses. Only the most useful fields are included.
func memoryToMap(m *types.Memory) map[string]interface{} {
	if m == nil {
		return nil
	}
	out := map[string]interface{}{
		"id":         m.ID,
		"content":    m.Content,
		"source":     m.Source,
		"domain":     m.Domain,
		"status":     string(m.Status),
		"created_at": m.CreatedAt.Format(time.RFC3339),
		"updated_at": m.UpdatedAt.Format(time.RFC3339),
		"decay_score": m.DecayScore,
	}
	if m.Summary != "" {
		out["summary"] = m.Summary
	}
	if len(m.Tags) > 0 {
		out["tags"] = m.Tags
	}
	if m.State != "" {
		out["state"] = m.State
	}
	return out
}

// DetectContradictions detects structural contradictions in the memory graph.
// If memory_id is provided, only contradictions involving that memory are returned.
// If memory_id is empty, all contradictions in the graph are detected.
//
// Contradiction types detected:
// 1. conflicting_relationship: Same entity with multiple values for single-valued relationships
// 2. superseded_active: Superseded memories still have active relationships
// 3. temporal_impossibility: Temporal ordering violations in relationships
func (s *Server) DetectContradictions(ctx context.Context, args DetectContradictionsArgs) (*DetectContradictionsResult, error) {
	// Call the contradiction detector
	contradictions, err := s.detector.DetectContradictions(ctx, args.MemoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to detect contradictions: %w", err)
	}

	// Convert engine contradictions to API result types
	results := make([]ContradictionResult, len(contradictions))
	for i, c := range contradictions {
		results[i] = ContradictionResult{
			Type:        string(c.Type),
			MemoryIDs:   c.MemoryIDs,
			Description: c.Description,
			Confidence:  c.Confidence,
		}
	}

	var message string
	if args.MemoryID != "" {
		message = fmt.Sprintf("Detected %d contradictions involving memory %s", len(contradictions), args.MemoryID)
	} else {
		message = fmt.Sprintf("Detected %d contradictions in the memory graph", len(contradictions))
	}

	return &DetectContradictionsResult{
		Contradictions: results,
		Total:          len(contradictions),
		Message:        message,
	}, nil
}

// ---------------------------------------------------------------------------
// Standard MCP protocol handlers
// ---------------------------------------------------------------------------

// handleInitialize handles the MCP initialize handshake.
func (s *Server) handleInitialize(ctx context.Context, params interface{}) (interface{}, error) {
	return MCPInitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: MCPServerCapabilities{
			Tools: &MCPToolsCapability{},
		},
		ServerInfo: MCPServerInfo{
			Name:    "memento",
			Version: "1.0.0",
		},
	}, nil
}

// handleToolsList returns the list of all tools this server exposes.
func (s *Server) handleToolsList(ctx context.Context, params interface{}) (interface{}, error) {
	return MCPToolsListResult{Tools: s.buildToolsList()}, nil
}

// handleToolsCall dispatches a tools/call request to the appropriate handler
// and wraps the result in the MCP content envelope.
func (s *Server) handleToolsCall(ctx context.Context, params interface{}) (interface{}, error) {
	var p MCPToolCallParams
	if err := s.unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	// Re-marshal arguments so they can be passed to the existing handlers
	// which expect an interface{} produced by JSON unmarshal.
	argsJSON, err := json.Marshal(p.Arguments)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal arguments: %w", err)
	}
	var rawParams interface{}
	if err := json.Unmarshal(argsJSON, &rawParams); err != nil {
		return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
	}

	var result interface{}
	var handlerErr error

	switch p.Name {
	case "store_memory":
		result, handlerErr = s.handleStoreMemory(ctx, rawParams)
	case "recall_memory":
		result, handlerErr = s.handleRecallMemory(ctx, rawParams)
	case "find_related":
		result, handlerErr = s.handleFindRelated(ctx, rawParams)
	case "update_memory_state":
		result, handlerErr = s.handleUpdateMemoryState(ctx, rawParams)
	case "forget_memory":
		result, handlerErr = s.handleForgetMemory(ctx, rawParams)
	case "evolve_memory":
		result, handlerErr = s.handleEvolveMemory(ctx, rawParams)
	case "detect_contradictions":
		result, handlerErr = s.handleDetectContradictions(ctx, rawParams)
	case "update_memory":
		result, handlerErr = s.handleUpdateMemory(ctx, rawParams)
	case "explain_reasoning":
		result, handlerErr = s.handleExplainReasoning(ctx, rawParams)
	case "retry_enrichment":
		result, handlerErr = s.handleRetryEnrichment(ctx, rawParams)
	case "get_session_context":
		result, handlerErr = s.handleGetSessionContext(ctx, rawParams)
	case "traverse_memory_graph":
		result, handlerErr = s.handleTraverseMemoryGraph(ctx, rawParams)
	case "restore_memory":
		result, handlerErr = s.handleRestoreMemory(ctx, rawParams)
	case "list_deleted_memories":
		result, handlerErr = s.handleListDeletedMemories(ctx, rawParams)
	case "get_evolution_chain":
		result, handlerErr = s.handleGetEvolutionChain(ctx, rawParams)
	case "create_project":
		result, handlerErr = s.handleCreateProject(ctx, rawParams)
	case "add_project_item":
		result, handlerErr = s.handleAddProjectItem(ctx, rawParams)
	case "get_project_tree":
		result, handlerErr = s.handleGetProjectTree(ctx, rawParams)
	case "list_projects":
		result, handlerErr = s.handleListProjects(ctx, rawParams)
	default:
		return &MCPToolCallResult{
			Content: []MCPToolCallContent{{Type: "text", Text: fmt.Sprintf("unknown tool: %s", p.Name)}},
			IsError: true,
		}, nil
	}

	if handlerErr != nil {
		return &MCPToolCallResult{
			Content: []MCPToolCallContent{{Type: "text", Text: handlerErr.Error()}},
			IsError: true,
		}, nil
	}

	text, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	return &MCPToolCallResult{
		Content: []MCPToolCallContent{{Type: "text", Text: string(text)}},
	}, nil
}

// buildToolsList returns the canonical list of MCP tool definitions.
func (s *Server) buildToolsList() []MCPTool {
	return []MCPTool{
		{
			Name:        "store_memory",
			Description: "Store a new memory. Returns immediately with a pending status; enrichment (entity extraction, embeddings) happens asynchronously. Duplicate content is deduplicated automatically.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"content"},
				"properties": map[string]interface{}{
					"content":       map[string]interface{}{"type": "string", "description": "The memory content to store (required)"},
					"source":        map[string]interface{}{"type": "string", "description": "Where this memory came from"},
					"domain":        map[string]interface{}{"type": "string", "description": "Memory domain/category (deprecated: prefer connection_id)"},
					"connection_id": map[string]interface{}{"type": "string", "description": "Connection to store into; sets the domain automatically"},
					"tags":          map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional tags for categorization"},
					"metadata":      map[string]interface{}{"type": "object", "description": "Arbitrary key-value metadata"},
					"created_by":    map[string]interface{}{"type": "string", "description": "Name of the agent or developer storing this memory. Auto-detected if not provided."},
				},
			},
		},
		{
			Name: "recall_memory",
			Description: "Retrieve memories. Three modes: " +
				"(1) id set → direct lookup (auto-routes to the right connection from the ID); " +
				"(2) query set → full-text search (natural language); " +
				"(3) neither → paginated list with optional filters. " +
				"Pass connection_id to scope query/list mode to a specific workspace.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":             map[string]interface{}{"type": "string", "description": "Memory ID for direct lookup (connection is inferred from the ID)"},
					"query":          map[string]interface{}{"type": "string", "description": "Natural-language search query (full-text search)"},
					"connection_id":  map[string]interface{}{"type": "string", "description": "Scope search/list to this connection (workspace). Omit to use the default."},
					"state":          map[string]interface{}{"type": "string", "description": "Filter by lifecycle state: active, archived, superseded"},
					"created_by":     map[string]interface{}{"type": "string", "description": "Filter by creator"},
					"created_after":  map[string]interface{}{"type": "string", "description": "RFC-3339 lower bound for created_at"},
					"created_before": map[string]interface{}{"type": "string", "description": "RFC-3339 upper bound for created_at"},
					"limit":          map[string]interface{}{"type": "integer", "description": "Max results to return (default 10, max 100)"},
					"page":           map[string]interface{}{"type": "integer", "description": "Page number for list mode (default 1)"},
				},
			},
		},
		{
			Name:        "find_related",
			Description: "Full-text (+ semantic) search across memories. Returns ranked results. Pass connection_id to search within a specific workspace.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query":          map[string]interface{}{"type": "string", "description": "Search query (required)"},
					"connection_id":  map[string]interface{}{"type": "string", "description": "Scope search to this connection (workspace). Omit to search the default workspace."},
					"limit":          map[string]interface{}{"type": "integer", "description": "Max results (default 10)"},
					"domain":         map[string]interface{}{"type": "string", "description": "Restrict search to this domain (legacy; prefer connection_id)"},
					"created_after":  map[string]interface{}{"type": "string", "description": "RFC-3339 lower bound for created_at"},
					"created_before": map[string]interface{}{"type": "string", "description": "RFC-3339 upper bound for created_at"},
				},
			},
		},
		{
			Name:        "update_memory_state",
			Description: "Update the lifecycle state of a memory. Valid states: active, archived, superseded.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"id", "state"},
				"properties": map[string]interface{}{
					"id":    map[string]interface{}{"type": "string", "description": "Memory ID (required)"},
					"state": map[string]interface{}{"type": "string", "description": "New lifecycle state: active, archived, superseded"},
				},
			},
		},
		{
			Name:        "forget_memory",
			Description: "Soft-delete a memory (moves it to trash with a grace period). Use hard_delete=true to permanently remove. Soft-deleted memories are excluded from all searches and recalls.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]interface{}{
					"id":            map[string]interface{}{"type": "string", "description": "Memory ID to delete (required)"},
					"connection_id": map[string]interface{}{"type": "string", "description": "Connection the memory lives in (inferred from ID if omitted)"},
					"hard_delete":   map[string]interface{}{"type": "boolean", "description": "Permanently remove instead of soft-deleting (default: false)"},
				},
			},
		},
		{
			Name:        "update_memory",
			Description: "Update the content, tags, or metadata of an existing memory. Use this to correct or refine a stored memory.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]interface{}{
					"id":       map[string]interface{}{"type": "string", "description": "Memory ID to update (required)"},
					"content":  map[string]interface{}{"type": "string", "description": "New content to replace the existing content"},
					"tags":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "New tags list (replaces existing tags)"},
					"metadata": map[string]interface{}{"type": "object", "description": "New metadata map (replaces existing metadata)"},
				},
			},
		},
		{
			Name:        "detect_contradictions",
			Description: "Scan the memory graph for contradictions (conflicting relationships, superseded-but-active memories, temporal impossibilities).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"memory_id": map[string]interface{}{"type": "string", "description": "Optional: focus contradiction detection on this memory ID"},
				},
			},
		},
		{
			Name:        "explain_reasoning",
			Description: "Explain why specific memories were retrieved for a query.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"query", "memory_ids"},
				"properties": map[string]interface{}{
					"query":      map[string]interface{}{"type": "string", "description": "The original query"},
					"memory_ids": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Memory IDs to explain"},
				},
			},
		},
		{
			Name:        "retry_enrichment",
			Description: "Retry enrichment (entity extraction, embeddings) for a memory that previously failed.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Memory ID to retry enrichment for"},
				},
			},
		},
		{
			Name:        "get_session_context",
			Description: "Retrieve memories from the current or a specified session to understand what work was done. Useful for 'where did I leave off?' queries. Returns recent memories grouped by topic with a summary.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{},
				"properties": map[string]interface{}{
					"session_id": map[string]interface{}{
						"type":        "string",
						"description": "Session ID to query. Defaults to the current MCP session.",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of memories to return (default: 20).",
					},
					"time_window_hours": map[string]interface{}{
						"type":        "integer",
						"description": "Only return memories from the last N hours. 0 means all memories in the session.",
					},
					"connection_id": map[string]interface{}{
						"type":        "string",
						"description": "Connection to query. Uses default if not specified.",
					},
				},
			},
		},
		{
			Name:        "evolve_memory",
			Description: "Create a new version of a memory that supersedes the old one. The old memory is marked as 'superseded' and a new memory is stored with the updated content. Use this instead of update_memory when the change represents a meaningful evolution (not just a typo fix).",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"id", "new_content"},
				"properties": map[string]interface{}{
					"id":            map[string]interface{}{"type": "string", "description": "ID of the memory to supersede (required)"},
					"new_content":   map[string]interface{}{"type": "string", "description": "Content for the new evolved memory (required)"},
					"connection_id": map[string]interface{}{"type": "string", "description": "Connection the memory lives in (inferred from ID if omitted)"},
				},
			},
		},
		{
			Name:        "consolidate_memories",
			Description: "Merge multiple memories into one consolidated memory. The originals are soft-deleted. Use when you have several related memories that should be combined into a single, coherent record. Provide either explicit memory IDs or a search query to find candidates.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"ids":           map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Explicit list of memory IDs to consolidate (min 2)"},
					"query":         map[string]interface{}{"type": "string", "description": "Search query to find consolidation candidates automatically"},
					"connection_id": map[string]interface{}{"type": "string", "description": "Connection to use (defaults to primary)"},
					"limit":         map[string]interface{}{"type": "integer", "description": "Max memories when using query mode (default 5, max 10)"},
					"title":         map[string]interface{}{"type": "string", "description": "Optional title for the consolidated memory"},
				},
			},
		},
		{
			Name:        "traverse_memory_graph",
			Description: "Follow entity relationship connections from a memory to find related memories. Discovers memories connected through shared entities (people, organizations, concepts). Use when you want to explore what is contextually related to a memory, not just textually similar.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"memory_id"},
				"properties": map[string]interface{}{
					"memory_id": map[string]interface{}{
						"type":        "string",
						"description": "Starting memory ID to traverse from",
					},
					"max_hops": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum relationship hops to follow (1-4, default 2)",
						"default":     2,
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results to return (default 10)",
						"default":     10,
					},
				},
			},
		},
		{
			Name:        "restore_memory",
			Description: "Restore a soft-deleted memory. Clears the deleted_at timestamp so the memory is visible again in searches and recalls.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]interface{}{
					"id":            map[string]interface{}{"type": "string", "description": "Memory ID to restore (required)"},
					"connection_id": map[string]interface{}{"type": "string", "description": "Connection the memory lives in (inferred from ID if omitted)"},
				},
			},
		},
		{
			Name:        "list_deleted_memories",
			Description: "List soft-deleted memories that can be restored. Returns memories that have been forgotten (soft-deleted) but not yet permanently purged.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"connection_id": map[string]interface{}{"type": "string", "description": "Connection to query (defaults to primary)"},
					"limit":         map[string]interface{}{"type": "integer", "description": "Max results (default 10)"},
					"page":          map[string]interface{}{"type": "integer", "description": "Page number (default 1)"},
				},
			},
		},
		{
			Name:        "get_evolution_chain",
			Description: "Get the full version history of a memory. Returns an ordered list from the original version to the latest, showing how the memory has evolved over time.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]interface{}{
					"id":            map[string]interface{}{"type": "string", "description": "Memory ID to trace (required)"},
					"connection_id": map[string]interface{}{"type": "string", "description": "Connection the memory lives in (inferred from ID if omitted)"},
				},
			},
		},
		{
			Name:        "create_project",
			Description: "Create a new project memory with optional pre-created phases. Projects use the memory type 'project' and can be linked to epics, phases, tasks, steps, and milestones.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name":          map[string]interface{}{"type": "string", "description": "Project name (required)"},
					"description":   map[string]interface{}{"type": "string", "description": "Project description"},
					"connection_id": map[string]interface{}{"type": "string", "description": "Connection to store into"},
					"phase_names":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional list of phase names to pre-create under the project"},
				},
			},
		},
		{
			Name:        "add_project_item",
			Description: "Add a child item (epic, phase, task, step, or milestone) under a parent project or phase. Creates a memory with the appropriate type and links it via a CONTAINS relationship.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"parent_id", "item_type", "name"},
				"properties": map[string]interface{}{
					"parent_id":     map[string]interface{}{"type": "string", "description": "ID of the parent memory (required)"},
					"item_type":     map[string]interface{}{"type": "string", "description": "Type of item: epic, phase, task, step, or milestone (required)"},
					"name":          map[string]interface{}{"type": "string", "description": "Item name (required)"},
					"description":   map[string]interface{}{"type": "string", "description": "Item description"},
					"connection_id": map[string]interface{}{"type": "string", "description": "Connection (inferred from parent_id if omitted)"},
				},
			},
		},
		{
			Name:        "get_project_tree",
			Description: "Retrieve the full nested hierarchy of a project, including all phases, epics, tasks, steps, and milestones linked via CONTAINS relationships.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"project_id"},
				"properties": map[string]interface{}{
					"project_id":    map[string]interface{}{"type": "string", "description": "Project memory ID (required)"},
					"connection_id": map[string]interface{}{"type": "string", "description": "Connection (inferred from project_id if omitted)"},
					"depth":         map[string]interface{}{"type": "integer", "description": "Maximum depth to traverse (default 3, max 6)"},
				},
			},
		},
		{
			Name:        "list_projects",
			Description: "List all project memories. Optionally filter by lifecycle state.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"connection_id": map[string]interface{}{"type": "string", "description": "Connection to query (defaults to primary)"},
					"state":         map[string]interface{}{"type": "string", "description": "Filter by lifecycle state (e.g. 'active', 'completed')"},
					"limit":         map[string]interface{}{"type": "integer", "description": "Max results (default 10)"},
					"page":          map[string]interface{}{"type": "integer", "description": "Page number (default 1)"},
				},
			},
		},
	}
}

// validateStoreMemoryArgs validates store_memory arguments.
func (s *Server) validateStoreMemoryArgs(args StoreMemoryArgs) error {
	if args.Content == "" {
		return errors.New("content is required")
	}
	return nil
}

// validateFindRelatedArgs validates find_related arguments.
func (s *Server) validateFindRelatedArgs(args FindRelatedArgs) error {
	if args.Query == "" {
		return errors.New("query is required")
	}
	if args.Limit < 0 {
		return errors.New("limit must be non-negative")
	}
	return nil
}

// resolveStoreForID returns the MemoryStore that owns the given memory ID.
// Memory IDs follow the format "mem:<connection>:<hash>", so the connection
// name is encoded directly in the ID. When the connection matches a known
// entry in the connection manager its store is returned; otherwise the
// default store is used as a fallback.
func (s *Server) resolveStoreForID(id string) storage.MemoryStore {
	if s.connectionManager == nil {
		return s.memoryStore
	}
	parts := strings.SplitN(id, ":", 3)
	if len(parts) != 3 || parts[0] != "mem" || parts[1] == "general" {
		return s.memoryStore
	}
	if store, err := s.connectionManager.GetStore(parts[1]); err == nil {
		return store
	}
	return s.memoryStore
}

// resolveSearchStore returns the MemoryStore and SearchProvider for a given
// connection ID. Resolution priority:
//  1. connectionID argument (explicit per-call override)
//  2. s.defaultConnection (set via WithDefaultConnection / MEMENTO_DEFAULT_CONNECTION)
//  3. s.memoryStore / s.searchProvider (server-level defaults)
func (s *Server) resolveSearchStore(connectionID string) (storage.MemoryStore, storage.SearchProvider) {
	// Pick which name to look up.
	name := connectionID
	if name == "" {
		name = s.defaultConnection
	}
	if name == "" || s.connectionManager == nil {
		return s.memoryStore, s.searchProvider
	}
	store, err := s.connectionManager.GetStore(name)
	if err != nil {
		return s.memoryStore, s.searchProvider
	}
	var sp storage.SearchProvider
	if casted, ok := store.(storage.SearchProvider); ok {
		sp = casted
	} else {
		sp = s.searchProvider
	}
	return store, sp
}

// generateMemoryID generates a deterministic memory ID from the content.
// Using a content hash means duplicate stores of the same text produce the
// same ID. Since Store() has upsert semantics, the second call is a no-op
// rather than creating a duplicate row.
// Format: mem:domain:<first 16 hex chars of SHA-256(content)>
func (s *Server) generateMemoryID(domain, content string) string {
	if domain == "" {
		domain = "general"
	}
	h := sha256.Sum256([]byte(content))
	slug := fmt.Sprintf("%x", h[:8]) // 16 hex chars
	return fmt.Sprintf("mem:%s:%s", domain, slug)
}

// unmarshalParams unmarshals JSON-RPC parameters into a typed struct.
func (s *Server) unmarshalParams(params interface{}, dest interface{}) error {
	data, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("failed to unmarshal params: %w", err)
	}

	return nil
}

// successResponse creates a JSON-RPC success response.
func (s *Server) successResponse(id interface{}, result interface{}) ([]byte, error) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
	return json.Marshal(resp)
}

// errorResponse creates a JSON-RPC error response.
func (s *Server) errorResponse(id interface{}, code int, message string, data interface{}) ([]byte, error) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}
	return json.Marshal(resp)
}
