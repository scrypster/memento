# Ollama HTTP Client

## Overview

The Ollama client provides a robust HTTP interface for communicating with local Ollama LLM instances. It implements automatic circuit breaker protection, request timeouts, and comprehensive error handling for reliable LLM inference.

## Features

- **Text Completion**: Generate text completions using local LLM models
- **Embeddings**: Generate vector embeddings for semantic search
- **Health Checks**: Monitor Ollama availability and health
- **Model Discovery**: List available models on the Ollama instance
- **Circuit Breaker Protection**: Automatic failure detection and recovery
- **Request Timeouts**: 5-second timeout per request (configurable)
- **Context Support**: Full context cancellation and timeout support

## Architecture

```
┌─────────────────┐
│  OllamaClient   │
├─────────────────┤
│ - baseURL       │
│ - model         │
│ - timeout       │
│ - httpClient    │
│ - circuitBreaker│
└────────┬────────┘
         │
         ├──> Complete()  ──> Circuit Breaker ──> HTTP POST /api/generate
         │
         ├──> Embed()     ──> Circuit Breaker ──> HTTP POST /api/embeddings
         │
         ├──> HealthCheck() ──────────────────> HTTP GET /api/version
         │
         └──> ListModels() ───────────────────> HTTP GET /api/tags
```

## Usage

### Basic Setup

```go
import (
    "context"
    "github.com/scrypster/memento/internal/llm"
    "time"
)

// Create client with default configuration
client := llm.NewOllamaClient(llm.OllamaConfig{})

// Or with custom configuration
config := llm.OllamaConfig{
    BaseURL: "http://localhost:11434",
    Model:   "phi3:mini",
    Timeout: 5 * time.Second,
}
client := llm.NewOllamaClient(config)
```

### Text Completion

```go
ctx := context.Background()
response, err := client.Complete(ctx, "What is the capital of France?")
if err != nil {
    log.Fatalf("Completion failed: %v", err)
}
fmt.Println(response)
```

### Generate Embeddings

```go
ctx := context.Background()
embeddings, err := client.Embed(ctx, "Hello, world!")
if err != nil {
    log.Fatalf("Embedding failed: %v", err)
}
fmt.Printf("Generated %d-dimensional embedding\n", len(embeddings))
```

### Health Check

```go
ctx := context.Background()
if err := client.HealthCheck(ctx); err != nil {
    log.Printf("Ollama health check failed: %v", err)
}
```

### List Available Models

```go
ctx := context.Background()
models, err := client.ListModels(ctx)
if err != nil {
    log.Fatalf("Failed to list models: %v", err)
}
for _, model := range models {
    fmt.Println(model)
}
```

## Integration with Other Components

### Entity Extraction

```go
content := "John Doe works on the Memento project at Anthropic."
prompt := llm.EntityExtractionPrompt(content)
response, err := client.Complete(ctx, prompt)
// Parse JSON response containing extracted entities
```

### Content Chunking

```go
// For large content that exceeds token limits
chunker := &llm.Chunker{
    MaxChunkSize: 3000,
    Overlap:      200,
}

chunks, err := chunker.Chunk(largeContent)
for _, chunk := range chunks {
    prompt := llm.EntityExtractionPrompt(chunk)
    response, err := client.Complete(ctx, prompt)
    // Process each chunk's response
}
```

## Circuit Breaker Protection

The Ollama client automatically wraps all `Complete()` and `Embed()` calls with circuit breaker protection:

- **Closed State**: Requests pass through normally
- **Open State**: After 3 consecutive failures, circuit opens and rejects requests immediately
- **Half-Open State**: After 30 seconds, allows test requests to check if service recovered
- **Recovery**: After 2 consecutive successes in half-open state, circuit closes again

### Handling Circuit Breaker Errors

```go
response, err := client.Complete(ctx, prompt)
if err != nil {
    if errors.Is(err, llm.ErrCircuitOpen) {
        // Circuit breaker is open, service is degraded
        log.Println("Ollama service temporarily unavailable")
        // Implement fallback logic
    } else {
        // Other error (timeout, network, etc.)
        log.Printf("Request failed: %v", err)
    }
}
```

## Configuration

### Default Values

```go
type OllamaConfig struct {
    BaseURL string        // Default: "http://localhost:11434"
    Model   string        // Default: "phi3:mini"
    Timeout time.Duration // Default: 5 seconds
}
```

### Recommended Models

- **phi3:mini** (2.2GB): Fast, efficient, good for entity extraction
- **llama3:8b** (4.7GB): Better quality, slower inference
- **mistral:7b** (4.1GB): Good balance of speed and quality
- **all-MiniLM-L6-v2** (23MB): Optimized for embeddings

## API Endpoints

### POST /api/generate

Generate text completions.

**Request:**
```json
{
  "model": "phi3:mini",
  "prompt": "What is the capital of France?",
  "stream": false
}
```

**Response:**
```json
{
  "response": "The capital of France is Paris.",
  "done": true
}
```

### POST /api/embeddings

Generate embeddings.

**Request:**
```json
{
  "model": "phi3:mini",
  "input": "Hello, world!"
}
```

**Response:**
```json
{
  "embedding": [0.1, 0.2, 0.3, ...]
}
```

### GET /api/tags

List available models.

**Response:**
```json
{
  "models": [
    {"name": "phi3:mini"},
    {"name": "llama3:8b"}
  ]
}
```

### GET /api/version

Health check endpoint.

**Response:**
```json
{
  "version": "0.1.0"
}
```

## Error Handling

The client returns detailed errors for different failure scenarios:

- **Network Errors**: Connection failures, timeouts
- **HTTP Errors**: Non-200 status codes from Ollama
- **JSON Errors**: Malformed response parsing failures
- **Context Errors**: Cancellation or deadline exceeded
- **Circuit Breaker Errors**: Circuit breaker open state

Example error handling:

```go
response, err := client.Complete(ctx, prompt)
if err != nil {
    switch {
    case errors.Is(err, context.DeadlineExceeded):
        log.Println("Request timed out after 5 seconds")
    case errors.Is(err, context.Canceled):
        log.Println("Request was canceled")
    case errors.Is(err, llm.ErrCircuitOpen):
        log.Println("Circuit breaker is open")
    default:
        log.Printf("Request failed: %v", err)
    }
    return
}
```

## Testing

The implementation includes comprehensive tests:

### Unit Tests (`tests/ollama_test.go`)

- TestOllamaClient_Complete
- TestOllamaClient_Embed
- TestOllamaClient_HealthCheck
- TestOllamaClient_Timeout
- TestOllamaClient_CircuitBreaker
- TestOllamaClient_ListModels
- TestOllamaClient_InvalidResponse
- TestOllamaClient_ContextCancellation
- TestOllamaClient_DefaultConfig
- TestOllamaClient_EmptyPrompt

### Integration Tests (`tests/ollama_integration_test.go`)

- TestOllamaIntegration_EntityExtraction
- TestOllamaIntegration_WithChunking
- TestOllamaIntegration_CircuitBreakerRecovery
- TestOllamaIntegration_EmbeddingWorkflow
- TestOllamaIntegration_MultipleRequests
- TestOllamaIntegration_ModelListing
- TestOllamaIntegration_HealthCheckMonitoring

Run tests:
```bash
go test ./tests/ollama_test.go -v
go test ./tests/ollama_integration_test.go -v
```

## Performance Considerations

### Request Timeouts

All requests have a 5-second timeout by default. This prevents hanging requests from blocking the application.

### Circuit Breaker Overhead

The circuit breaker adds minimal overhead (< 1ms) per request but provides significant reliability benefits.

### Embedding Performance

Embedding generation is typically faster than text completion:
- Small text (< 100 tokens): ~50-100ms
- Medium text (100-500 tokens): ~100-200ms
- Large text (500+ tokens): ~200-500ms

### Recommendation

For production use:
1. Set appropriate timeout based on your model size
2. Monitor circuit breaker state
3. Implement fallback logic for when circuit is open
4. Use chunking for content > 3000 tokens

## Dependencies

- **Circuit Breaker**: `internal/llm/circuit_breaker.go`
- **Prompts**: `internal/llm/prompts.go`
- **Chunker**: `internal/llm/chunker.go`

## Future Enhancements

Potential improvements for future versions:

1. **Streaming Support**: Stream responses for long completions
2. **Batch Processing**: Process multiple prompts in parallel
3. **Retry Logic**: Configurable retry with exponential backoff
4. **Metrics**: Detailed performance and error metrics
5. **Model Caching**: Cache frequently used models
6. **Custom Headers**: Support for authentication and custom headers

## Troubleshooting

### "circuit breaker is open"

The circuit breaker opened due to repeated failures. Wait 30 seconds for it to transition to half-open state, or restart your application.

### "context deadline exceeded"

Request timed out after 5 seconds. This could be due to:
- Large prompt or response
- Slow model inference
- Network congestion

Increase timeout or use a smaller model.

### "connection refused"

Ollama is not running. Start Ollama:
```bash
ollama serve
```

### "model not found"

The specified model is not installed. List available models:
```go
models, _ := client.ListModels(ctx)
```

Or install the model:
```bash
ollama pull phi3:mini
```

## License

Part of the Memento project.
