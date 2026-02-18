// Package mcp implements the Model Context Protocol (MCP) server for Memento.
// It provides JSON-RPC 2.0 based tools for storing, retrieving, and searching memories.
package mcp

import (
	"encoding/json"
	"strings"

	"github.com/scrypster/memento/pkg/types"
)

// StoreMemoryArgs contains arguments for the store_memory tool.
type StoreMemoryArgs struct {
	Content      string                 `json:"content"`                 // Memory content (required)
	Source       string                 `json:"source,omitempty"`        // Source of the memory
	Domain       string                 `json:"domain,omitempty"`        // Memory domain/category (deprecated: use connection_id)
	ConnectionID string                 `json:"connection_id,omitempty"` // Connection to store into (sets domain)
	Tags         []string               `json:"tags,omitempty"`          // User-defined tags
	Metadata     map[string]interface{} `json:"metadata,omitempty"`      // Arbitrary metadata
	CreatedBy    string                 `json:"created_by,omitempty"`    // Name of the agent or developer storing this memory. Auto-detected if not provided.
	SessionID    string                 `json:"session_id,omitempty"`    // Session ID override; uses server session ID if not provided.
}

// UnmarshalJSON handles the case where some MCP clients (e.g. Claude Code) send
// array fields like "tags" as a JSON-encoded string ("[\"a\",\"b\"]") rather than
// a proper JSON array. Both forms are accepted.
func (a *StoreMemoryArgs) UnmarshalJSON(data []byte) error {
	type Alias StoreMemoryArgs
	aux := &struct {
		Tags json.RawMessage `json:"tags,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(a),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if aux.Tags == nil {
		return nil
	}
	// Try direct array unmarshal first.
	var tags []string
	if err := json.Unmarshal(aux.Tags, &tags); err == nil {
		a.Tags = tags
		return nil
	}
	// Fall back: client sent the array as a JSON-encoded string.
	var s string
	if err := json.Unmarshal(aux.Tags, &s); err != nil {
		return nil // ignore unrecognised tag formats rather than failing
	}
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") {
		_ = json.Unmarshal([]byte(s), &tags)
		a.Tags = tags
	} else if s != "" {
		// Comma-separated fallback.
		for _, t := range strings.Split(s, ",") {
			if t = strings.TrimSpace(t); t != "" {
				a.Tags = append(a.Tags, t)
			}
		}
	}
	return nil
}

// StoreMemoryResult contains the result of storing a memory.
// In v2.0, this returns immediately with pending status.
type StoreMemoryResult struct {
	ID         string             `json:"id"`                      // Memory ID
	Status     types.MemoryStatus `json:"status"`                  // Memory status (always "pending" in v2.0)
	Message    string             `json:"message"`                 // Status message
	Duplicate  bool               `json:"duplicate,omitempty"`     // If true, content was a duplicate
	ExistingID string             `json:"existing_id,omitempty"`   // ID of existing memory if duplicate
}

// RecallMemoryArgs contains arguments for the recall_memory tool.
//
// Priority order:
//  1. ID set → direct memory lookup by ID (returns single memory)
//  2. Query set → full-text search (delegates to FTS, like find_related)
//  3. Neither → list/filter mode with optional filter fields
type RecallMemoryArgs struct {
	// ID is the memory ID to look up directly.  When set all other fields
	// are ignored.
	ID string `json:"id,omitempty"`

	// Query performs a full-text search when set. Takes priority over
	// list-filter mode. Pass a natural-language query to search memories.
	Query string `json:"query,omitempty"`

	// ConnectionID scopes the search or list to a specific connection.
	// When empty, the default connection is used. Ignored in ID-lookup mode
	// (the connection is inferred from the memory ID prefix).
	ConnectionID string `json:"connection_id,omitempty"`

	// --- Optional list-mode filter fields (used when ID and Query are both empty) ---

	// State filters by lifecycle state (e.g. "active", "archived").
	State string `json:"state,omitempty"`

	// CreatedBy filters by the agent or user that created the memory.
	CreatedBy string `json:"created_by,omitempty"`

	// CreatedAfter is an ISO-8601 / RFC-3339 timestamp.  Only memories
	// created strictly after this time are returned.
	CreatedAfter string `json:"created_after,omitempty"`

	// CreatedBefore is an ISO-8601 / RFC-3339 timestamp.  Only memories
	// created strictly before this time are returned.
	CreatedBefore string `json:"created_before,omitempty"`

	// MinDecayScore filters to memories whose decay_score is >= this value.
	// Accepts values in the range [0.0, 1.0].
	MinDecayScore float64 `json:"min_decay_score,omitempty"`

	// Limit controls how many memories to return (default 10, max 100).
	// Ignored when ID is set.
	Limit int `json:"limit,omitempty"`

	// Page is the 1-indexed page number in list mode (default 1).
	// Ignored when ID or Query is set.
	Page int `json:"page,omitempty"`
}

// RecallMemoryResult contains the result of recalling a memory.
//
// In ID-lookup mode exactly one memory is returned via the Memory field.
// In list mode the Memories slice is populated and Memory is nil.
type RecallMemoryResult struct {
	// Memory is the retrieved memory when using ID-lookup mode.
	Memory *types.Memory `json:"memory,omitempty"`

	// Found reports whether the memory was found (ID-lookup mode only).
	Found bool `json:"found"`

	// Memories contains the list of matched memories in list-filter mode.
	Memories []types.Memory `json:"memories,omitempty"`

	// Total is the total number of matching memories (list-filter mode).
	Total int `json:"total,omitempty"`

	// Page is the current page number (list-filter mode).
	Page int `json:"page,omitempty"`

	// HasMore indicates whether additional pages exist (list-filter mode).
	HasMore bool `json:"has_more,omitempty"`
}

// FindRelatedArgs contains arguments for the find_related tool.
type FindRelatedArgs struct {
	Query        string `json:"query"`                   // Search query (required)
	Limit        int    `json:"limit,omitempty"`         // Maximum number of results (default: 10)
	Domain       string `json:"domain,omitempty"`        // Filter by domain
	ConnectionID string `json:"connection_id,omitempty"` // Scope search to a specific connection (default: all)

	// CreatedAfter is an ISO-8601 / RFC-3339 timestamp. Only memories created
	// strictly after this time are considered during graph traversal.
	// Empty string means no lower bound.
	CreatedAfter string `json:"created_after,omitempty"`

	// CreatedBefore is an ISO-8601 / RFC-3339 timestamp. Only memories created
	// strictly before this time are considered during graph traversal.
	// Empty string means no upper bound.
	CreatedBefore string `json:"created_before,omitempty"`
}

// FindRelatedResult contains the result of searching for related memories.
type FindRelatedResult struct {
	Memories []types.Memory `json:"memories"` // List of related memories
	Total    int            `json:"total"`    // Total number of matches
}

// RetryEnrichmentArgs contains arguments for the retry_enrichment tool.
type RetryEnrichmentArgs struct {
	ID string `json:"id"` // Memory ID (required)
}

// RetryEnrichmentResult contains the result of retrying enrichment.
type RetryEnrichmentResult struct {
	ID      string `json:"id"`      // Memory ID
	Queued  bool   `json:"queued"`  // Whether the memory was queued for retry
	Message string `json:"message"` // Status message
}

// ExplainReasoningArgs contains arguments for the explain_reasoning tool.
type ExplainReasoningArgs struct {
	Query     string   `json:"query"`      // Original query (required)
	MemoryIDs []string `json:"memory_ids"` // List of memory IDs to explain (required)
}

// ExplainReasoningResult contains the reasoning explanation.
type ExplainReasoningResult struct {
	Explanation string          `json:"explanation"` // Human-readable explanation
	Reasoning   []ReasoningStep `json:"reasoning"`   // Detailed reasoning steps
}

// ReasoningStep represents a single step in the reasoning process.
type ReasoningStep struct {
	Step        string `json:"step"`        // Step name
	Description string `json:"description"` // Step description
}

// UpdateMemoryStateArgs contains arguments for the update_memory_state tool (Opus Issue #6).
// Validates state transitions according to the state machine defined in design doc Appendix B.
type UpdateMemoryStateArgs struct {
	ID    string `json:"id"`    // Memory ID (required)
	State string `json:"state"` // New lifecycle state (required)
}

// UpdateMemoryStateResult contains the result of updating a memory state.
type UpdateMemoryStateResult struct {
	ID            string `json:"id"`             // Memory ID
	PreviousState string `json:"previous_state"` // Previous state
	NewState      string `json:"new_state"`      // New state
	Message       string `json:"message"`        // Status message
}

// DetectContradictionsArgs contains arguments for the detect_contradictions tool.
type DetectContradictionsArgs struct {
	// MemoryID is optional. If provided, only contradictions involving this memory are returned.
	MemoryID string `json:"memory_id,omitempty"`
}

// ContradictionResult represents a single detected contradiction.
type ContradictionResult struct {
	Type        string   `json:"type"`        // Contradiction type (conflicting_relationship, superseded_active, temporal_impossibility)
	MemoryIDs   []string `json:"memory_ids"`  // Memory IDs involved in this contradiction
	Description string   `json:"description"` // Human-readable description
	Confidence  float64  `json:"confidence"`  // Confidence score (0.0-1.0)
}

// DetectContradictionsResult contains the result of contradiction detection.
type DetectContradictionsResult struct {
	Contradictions []ContradictionResult `json:"contradictions"` // List of detected contradictions
	Total          int                   `json:"total"`          // Total number of contradictions detected
	Message        string                `json:"message"`        // Status message
}

// UpdateMemoryArgs contains arguments for the update_memory tool.
type UpdateMemoryArgs struct {
	// ID is the memory ID to update (required).
	ID string `json:"id"`
	// Content replaces the memory content when non-empty.
	Content string `json:"content,omitempty"`
	// Tags replaces the tags list when non-nil.
	Tags []string `json:"tags,omitempty"`
	// Metadata replaces the metadata map when non-nil.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateMemoryResult contains the result of updating a memory.
type UpdateMemoryResult struct {
	ID      string `json:"id"`      // Memory ID
	Updated bool   `json:"updated"` // Whether the update was applied
	Message string `json:"message"` // Status message
}

// GetSessionContextArgs contains arguments for the get_session_context tool.
type GetSessionContextArgs struct {
	// SessionID is the session to query. Defaults to the current MCP session.
	SessionID string `json:"session_id,omitempty"`
	// Limit is the maximum number of memories to return (default 20).
	Limit int `json:"limit,omitempty"`
	// TimeWindowH restricts results to memories created in the last N hours.
	// Zero means no time window filter (all memories in the session).
	TimeWindowH int `json:"time_window_hours,omitempty"`
	// ConnectionID scopes the query to a specific connection.
	ConnectionID string `json:"connection_id,omitempty"`
}

// SessionTopicSummary summarises memory counts per domain within a session.
type SessionTopicSummary struct {
	Domain string `json:"domain"`
	Count  int    `json:"count"`
}

// GetSessionContextResult contains the result of get_session_context.
type GetSessionContextResult struct {
	SessionID      string                `json:"session_id"`
	MemoryCount    int                   `json:"memory_count"`
	Memories       []types.Memory        `json:"memories"`
	Topics         []SessionTopicSummary `json:"topics"`
	Summary        string                `json:"summary"`
	OnboardingHint string                `json:"onboarding_hint,omitempty"` // Set when the store is empty
}

// ForgetMemoryArgs contains arguments for the forget_memory tool.
type ForgetMemoryArgs struct {
	ID           string `json:"id"`                       // Memory ID to delete (required)
	ConnectionID string `json:"connection_id,omitempty"`  // Connection the memory lives in (inferred from ID if omitted)
	HardDelete   bool   `json:"hard_delete,omitempty"`    // if true, purge permanently
}

// ForgetMemoryResult contains the result of forgetting a memory.
type ForgetMemoryResult struct {
	ID      string `json:"id"`                // Memory ID
	Deleted bool   `json:"deleted"`           // Whether the memory was deleted
	Purged  bool   `json:"purged,omitempty"`  // Whether the memory was permanently purged
}

// EvolveMemoryArgs contains arguments for the evolve_memory tool.
type EvolveMemoryArgs struct {
	ID           string `json:"id"`                       // Existing memory to supersede (required)
	NewContent   string `json:"new_content"`              // Content for the new version (required)
	ConnectionID string `json:"connection_id,omitempty"`  // Connection the memory lives in (inferred from ID if omitted)
}

// EvolveMemoryResult contains the result of evolving a memory.
type EvolveMemoryResult struct {
	NewID        string `json:"new_id"`         // ID of the new memory
	SupersededID string `json:"superseded_id"`  // ID of the old memory (now state=superseded)
}

// ConsolidateMemoriesArgs holds arguments for consolidate_memories tool.
type ConsolidateMemoriesArgs struct {
	// Exactly one of IDs or Query must be provided.
	IDs          []string `json:"ids,omitempty"`          // explicit list of memory IDs to consolidate
	Query        string   `json:"query,omitempty"`         // search query to find candidates
	ConnectionID string   `json:"connection_id,omitempty"` // connection to use
	Limit        int      `json:"limit,omitempty"`         // max memories to consolidate when using query (default 5, max 10)
	Title        string   `json:"title,omitempty"`         // optional title for the consolidated memory
}

// ConsolidateMemoriesResult is returned by consolidate_memories.
type ConsolidateMemoriesResult struct {
	NewID           string   `json:"new_id"`             // ID of the new consolidated memory
	ConsolidatedIDs []string `json:"consolidated_ids"`   // IDs that were soft-deleted
	Content         string   `json:"content"`            // the merged content
	Message         string   `json:"message"`            // status message
}

// RestoreMemoryArgs contains arguments for the restore_memory tool.
type RestoreMemoryArgs struct {
	ID           string `json:"id"`                       // Memory ID to restore (required)
	ConnectionID string `json:"connection_id,omitempty"`  // Connection the memory lives in (inferred from ID if omitted)
}

// RestoreMemoryResult contains the result of restoring a soft-deleted memory.
type RestoreMemoryResult struct {
	ID      string `json:"id"`      // Memory ID
	Restored bool  `json:"restored"` // Whether the memory was restored
}

// ListDeletedMemoriesArgs contains arguments for the list_deleted_memories tool.
type ListDeletedMemoriesArgs struct {
	ConnectionID string `json:"connection_id,omitempty"` // Connection to query (defaults to primary)
	Limit        int    `json:"limit,omitempty"`         // Max results (default 10)
	Page         int    `json:"page,omitempty"`          // Page number (default 1)
}

// ListDeletedMemoriesResult contains the result of listing soft-deleted memories.
type ListDeletedMemoriesResult struct {
	Memories []types.Memory `json:"memories"` // Soft-deleted memories
	Total    int            `json:"total"`    // Total count
	Page     int            `json:"page"`     // Current page
	HasMore  bool           `json:"has_more"` // Whether more pages exist
}

// GetEvolutionChainArgs contains arguments for the get_evolution_chain tool.
type GetEvolutionChainArgs struct {
	ID           string `json:"id"`                       // Memory ID to trace (required)
	ConnectionID string `json:"connection_id,omitempty"`  // Connection the memory lives in (inferred from ID if omitted)
}

// EvolutionEntry represents a single version in an evolution chain.
type EvolutionEntry struct {
	Position  int    `json:"position"`            // 1-indexed position in chain (1 = oldest)
	ID        string `json:"id"`                  // Memory ID
	Content   string `json:"content"`             // First 200 chars of content
	State     string `json:"state,omitempty"`     // Lifecycle state
	CreatedAt string `json:"created_at"`          // RFC-3339 creation time
}

// GetEvolutionChainResult contains the result of getting an evolution chain.
type GetEvolutionChainResult struct {
	Chain         []EvolutionEntry `json:"chain"`          // Ordered list of versions (oldest first)
	TotalVersions int              `json:"total_versions"` // Total number of versions
	CurrentID     string           `json:"current_id"`     // ID of the most recent (current) version
}

// CreateProjectArgs contains arguments for the create_project tool.
type CreateProjectArgs struct {
	Name         string   `json:"name"`                    // Project name (required)
	Description  string   `json:"description,omitempty"`   // Project description
	ConnectionID string   `json:"connection_id,omitempty"` // Connection to store into
	PhaseNames   []string `json:"phase_names,omitempty"`   // Optional phase names to pre-create
}

// CreateProjectResult contains the result of creating a project.
type CreateProjectResult struct {
	ProjectID string   `json:"project_id"`         // ID of the created project memory
	PhaseIDs  []string `json:"phase_ids,omitempty"` // IDs of pre-created phase memories
}

// AddProjectItemArgs contains arguments for the add_project_item tool.
type AddProjectItemArgs struct {
	ParentID     string `json:"parent_id"`               // ID of the parent memory (required)
	ItemType     string `json:"item_type"`               // epic, phase, task, step, or milestone (required)
	Name         string `json:"name"`                    // Item name (required)
	Description  string `json:"description,omitempty"`   // Item description
	ConnectionID string `json:"connection_id,omitempty"` // Connection (inferred from parent_id if omitted)
}

// AddProjectItemResult contains the result of adding a project item.
type AddProjectItemResult struct {
	ID       string `json:"id"`        // ID of the created item memory
	ParentID string `json:"parent_id"` // ID of the parent memory
	ItemType string `json:"item_type"` // The item type that was created
}

// ProjectTreeNode represents a node in a project tree.
type ProjectTreeNode struct {
	ID       string            `json:"id"`                // Memory ID
	Name     string            `json:"name"`              // Item name (first line of content)
	Type     string            `json:"type"`              // Memory type (project/epic/phase/task/step/milestone)
	State    string            `json:"state,omitempty"`   // Lifecycle state
	Children []ProjectTreeNode `json:"children,omitempty"` // Nested children
}

// GetProjectTreeArgs contains arguments for the get_project_tree tool.
type GetProjectTreeArgs struct {
	ProjectID    string `json:"project_id"`              // Project memory ID (required)
	ConnectionID string `json:"connection_id,omitempty"` // Connection (inferred from project_id if omitted)
	Depth        int    `json:"depth,omitempty"`         // Max depth to traverse (default 3)
}

// GetProjectTreeResult contains the result of getting a project tree.
type GetProjectTreeResult struct {
	Tree ProjectTreeNode `json:"tree"` // Nested project tree
}

// ListProjectsArgs contains arguments for the list_projects tool.
type ListProjectsArgs struct {
	ConnectionID string `json:"connection_id,omitempty"` // Connection to query (defaults to primary)
	State        string `json:"state,omitempty"`         // Filter by lifecycle state
	Limit        int    `json:"limit,omitempty"`         // Max results (default 10)
	Page         int    `json:"page,omitempty"`          // Page number (default 1)
}

// ListProjectsResult contains the result of listing projects.
type ListProjectsResult struct {
	Projects []types.Memory `json:"projects"` // Project memories
	Total    int            `json:"total"`    // Total count
	Page     int            `json:"page"`     // Current page
	HasMore  bool           `json:"has_more"` // Whether more pages exist
}

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"` // Must be "2.0"
	Method  string      `json:"method"`  // Method name
	Params  interface{} `json:"params"`  // Method parameters
	ID      interface{} `json:"id"`      // Request ID (string, number, or null)
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`          // Must be "2.0"
	Result  interface{}   `json:"result,omitempty"` // Result (if successful)
	Error   *JSONRPCError `json:"error,omitempty"`  // Error (if failed)
	ID      interface{}   `json:"id"`               // Request ID
}

// JSONRPCError represents a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int         `json:"code"`          // Error code
	Message string      `json:"message"`       // Error message
	Data    interface{} `json:"data,omitempty"` // Additional error data
}

// JSON-RPC error codes
const (
	ErrCodeParseError     = -32700 // Invalid JSON
	ErrCodeInvalidRequest = -32600 // Invalid request object
	ErrCodeMethodNotFound = -32601 // Method not found
	ErrCodeInvalidParams  = -32602 // Invalid method parameters
	ErrCodeInternalError  = -32603 // Internal JSON-RPC error
	ErrCodeServerError    = -32000 // Server error
)

// ---------------------------------------------------------------------------
// Standard MCP protocol types (initialize / tools/list / tools/call)
// ---------------------------------------------------------------------------

// MCPInitializeParams holds the parameters sent by an MCP client in the
// initialize request.
type MCPInitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities,omitempty"`
	ClientInfo      MCPClientInfo          `json:"clientInfo"`
}

// MCPClientInfo identifies the connecting MCP client.
type MCPClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPServerInfo identifies this MCP server.
type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPServerCapabilities describes what this server supports.
type MCPServerCapabilities struct {
	Tools *MCPToolsCapability `json:"tools,omitempty"`
}

// MCPToolsCapability signals that the server exposes tools.
type MCPToolsCapability struct{}

// MCPInitializeResult is the response to the initialize request.
type MCPInitializeResult struct {
	ProtocolVersion string                `json:"protocolVersion"`
	Capabilities    MCPServerCapabilities `json:"capabilities"`
	ServerInfo      MCPServerInfo         `json:"serverInfo"`
}

// MCPTool describes a single tool exposed via the MCP tools/list endpoint.
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// MCPToolsListResult is the response to the tools/list request.
type MCPToolsListResult struct {
	Tools []MCPTool `json:"tools"`
}

// MCPToolCallParams holds the parameters sent in a tools/call request.
type MCPToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// MCPToolCallContent is a single content block in a tool call response.
type MCPToolCallContent struct {
	Type string `json:"type"` // always "text" for now
	Text string `json:"text"`
}

// MCPToolCallResult is the response to a tools/call request.
type MCPToolCallResult struct {
	Content []MCPToolCallContent `json:"content"`
	IsError bool                 `json:"isError,omitempty"`
}
