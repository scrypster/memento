# Memento MCP Server

This package implements the Model Context Protocol (MCP) server for Memento, allowing AI assistants like Claude to interact with the memory system through a standardized JSON-RPC 2.0 interface.

## Architecture

The MCP server follows a clean architecture with separation of concerns:

```
internal/api/mcp/
├── types.go       # Protocol types and constants
├── server.go      # Core server implementation
└── README.md      # This file

cmd/memento-mcp/
└── main.go        # Command-line entry point

tests/
├── mcp_test.go              # Unit tests
└── mcp_integration_test.go  # Integration tests
```

## Protocol Compliance

The server implements JSON-RPC 2.0 as specified by the Model Context Protocol:

- **Version**: JSON-RPC 2.0
- **Transport**: stdin/stdout (line-delimited JSON)
- **Error Codes**: Standard JSON-RPC error codes
- **Request ID**: Supports string, number, or null

### Request Format

```json
{
  "jsonrpc": "2.0",
  "method": "store_memory",
  "params": {
    "content": "Memory content",
    "source": "test"
  },
  "id": 1
}
```

### Response Format

Success:
```json
{
  "jsonrpc": "2.0",
  "result": {
    "id": "mem:general:123",
    "status": "pending",
    "message": "Memory stored successfully"
  },
  "id": 1
}
```

Error:
```json
{
  "jsonrpc": "2.0",
  "error": {
    "code": -32602,
    "message": "Invalid params",
    "data": null
  },
  "id": 1
}
```

## Available Tools

### 1. store_memory

Stores a new memory and returns immediately with pending status (v2.0 async behavior).

**Parameters:**
- `content` (string, required): Memory content
- `source` (string, optional): Source of the memory
- `domain` (string, optional): Domain/category
- `tags` (array, optional): List of tags
- `metadata` (object, optional): Additional metadata

**Returns:**
- `id`: Generated memory ID (format: mem:domain:timestamp)
- `status`: Always "pending" (enrichment happens asynchronously)
- `message`: Status message

**Example:**
```json
{
  "jsonrpc": "2.0",
  "method": "store_memory",
  "params": {
    "content": "Go is a compiled language created at Google",
    "source": "manual",
    "domain": "tech",
    "tags": ["programming", "go", "google"]
  },
  "id": 1
}
```

### 2. recall_memory

Retrieves a memory by ID.

**Parameters:**
- `id` (string, required): Memory ID

**Returns:**
- `memory`: Memory object (if found)
- `found`: Boolean indicating if memory exists

**Example:**
```json
{
  "jsonrpc": "2.0",
  "method": "recall_memory",
  "params": {
    "id": "mem:tech:1234567890"
  },
  "id": 2
}
```

### 3. find_related

Finds memories related to a query using text matching (v2.0) or semantic search (future).

**Parameters:**
- `query` (string, required): Search query
- `limit` (integer, optional): Max results (default: 10)
- `domain` (string, optional): Filter by domain

**Returns:**
- `memories`: Array of related memories
- `total`: Total number of matches

**Example:**
```json
{
  "jsonrpc": "2.0",
  "method": "find_related",
  "params": {
    "query": "programming languages",
    "limit": 5,
    "domain": "tech"
  },
  "id": 3
}
```

### 4. retry_enrichment

Retries enrichment for a failed memory.

**Parameters:**
- `id` (string, required): Memory ID

**Returns:**
- `id`: Memory ID
- `queued`: Boolean indicating if retry was queued
- `message`: Status message

**Example:**
```json
{
  "jsonrpc": "2.0",
  "method": "retry_enrichment",
  "params": {
    "id": "mem:tech:1234567890"
  },
  "id": 4
}
```

### 5. explain_reasoning

Explains why certain memories were retrieved for a query.

**Parameters:**
- `query` (string, required): Original query
- `memory_ids` (array, required): List of memory IDs to explain

**Returns:**
- `explanation`: Human-readable explanation
- `reasoning`: Array of reasoning steps

**Example:**
```json
{
  "jsonrpc": "2.0",
  "method": "explain_reasoning",
  "params": {
    "query": "Go programming",
    "memory_ids": ["mem:tech:1234567890", "mem:tech:1234567891"]
  },
  "id": 5
}
```

## Error Codes

The server uses standard JSON-RPC 2.0 error codes:

| Code | Description | When Used |
|------|-------------|-----------|
| -32700 | Parse error | Invalid JSON received |
| -32600 | Invalid Request | Missing required fields or invalid JSON-RPC version |
| -32601 | Method not found | Unknown method name |
| -32602 | Invalid params | Invalid method parameters |
| -32603 | Internal error | Server-side error |
| -32000 | Server error | Application-specific errors |

## Implementation Details

### Memory ID Generation

Memory IDs follow the format: `mem:domain:timestamp`

- **domain**: User-provided domain or "general" as default
- **timestamp**: Unix nanosecond timestamp for uniqueness

### v2.0 Async Enrichment

The `store_memory` method returns immediately with `status: "pending"`:

1. Memory is stored in the database with pending status
2. Response is sent immediately (< 10ms target)
3. Enrichment happens asynchronously (background worker)
4. Use `recall_memory` to check enrichment status

### Search Implementation

**v2.0**: Simple text-based filtering using `LIKE` queries
**Future**: Vector search with semantic similarity

### Validation

All inputs are validated before processing:
- Required fields must be present and non-empty
- Numeric values (limit) must be non-negative
- Memory IDs must follow correct format

## Testing

The implementation includes comprehensive tests:

### Unit Tests (`mcp_test.go`)
- TestMCPServer_StoreMemory: Verify immediate return with pending status
- TestMCPServer_RecallMemory: Test memory retrieval
- TestMCPServer_RecallMemory_NotFound: Test not found handling
- TestMCPServer_FindRelated: Test basic search
- TestMCPServer_RetryEnrichment: Test retry functionality
- TestMCPServer_ExplainReasoning: Test reasoning explanation
- TestMCPProtocol_Compliance: Verify JSON-RPC 2.0 format
- TestMCPServer_ErrorHandling: Test error cases

### Integration Tests (`mcp_integration_test.go`)
- TestMCPIntegration_FullWorkflow: End-to-end workflow
- TestMCPIntegration_RetryWorkflow: Retry enrichment workflow
- TestMCPIntegration_ErrorResponses: Protocol error handling

Run tests:
```bash
go test ./tests/mcp_test.go
go test ./tests/mcp_integration_test.go
```

## Performance

**v2.0 Targets:**
- `store_memory`: < 10ms (database write only)
- `recall_memory`: < 5ms (single SELECT)
- `find_related`: < 50ms (basic filtering)
- `retry_enrichment`: < 10ms (status update)
- `explain_reasoning`: < 10ms (static explanation)

**Future Improvements:**
- Vector search with caching (< 100ms)
- Semantic similarity scoring
- LLM-based reasoning explanations

## Security

### v2.0
- No authentication (development mode)
- Local-only access (stdin/stdout)

### Future
- API token authentication
- Rate limiting
- Request validation
- Audit logging

## Usage Example

### From Claude Desktop

Once configured, Claude can use these tools naturally:

```
User: "Remember that Go is a compiled language created at Google"
Claude: [Uses store_memory tool]
       "I've stored that memory. It's being enriched in the background."

User: "What do you know about Go?"
Claude: [Uses find_related with query "Go programming"]
       "Based on my memories, Go is a compiled language created at Google..."
```

### From CLI

```bash
# Store a memory
echo '{"jsonrpc":"2.0","method":"store_memory","params":{"content":"Test"},"id":1}' | memento-mcp

# Recall a memory
echo '{"jsonrpc":"2.0","method":"recall_memory","params":{"id":"mem:general:123"},"id":2}' | memento-mcp
```

## Future Enhancements

### Phase 3 (Vector Search)
- Semantic search with embeddings
- Hybrid search (text + vector)
- Relevance scoring

### Phase 4 (Graph Traversal)
- Related memories via relationships
- Graph-based reasoning
- Multi-hop traversal

### Phase 5 (Advanced Features)
- Streaming responses
- Batch operations
- Memory clustering
- Temporal queries

## References

- [Model Context Protocol Specification](https://modelcontextprotocol.io)
- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
- [Memento Architecture](../../../docs/ARCHITECTURE.md)
