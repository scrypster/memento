# Ollama Client Architecture

## Component Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         Memento Engine                           │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          │ Uses
                          ↓
         ┌────────────────────────────────────┐
         │       OllamaClient                 │
         │  ┌──────────────────────────────┐  │
         │  │ - baseURL: string            │  │
         │  │ - model: string              │  │
         │  │ - timeout: Duration          │  │
         │  │ - client: *http.Client       │  │
         │  │ - circuitBreaker: *CB        │  │
         │  └──────────────────────────────┘  │
         │                                    │
         │  Public Methods:                   │
         │  ┌──────────────────────────────┐  │
         │  │ + Complete(ctx, prompt)      │  │
         │  │ + Embed(ctx, text)           │  │
         │  │ + HealthCheck(ctx)           │  │
         │  │ + ListModels(ctx)            │  │
         │  └──────────────────────────────┘  │
         │                                    │
         │  Private Methods:                  │
         │  ┌──────────────────────────────┐  │
         │  │ - complete(ctx, prompt)      │  │
         │  │ - embed(ctx, text)           │  │
         │  └──────────────────────────────┘  │
         └──────────┬────────────┬────────────┘
                    │            │
        Uses        │            │  Uses
                    ↓            ↓
         ┌──────────────┐   ┌────────────────┐
         │ CircuitBreaker│   │  net/http      │
         │               │   │                │
         │ - Execute()   │   │ - POST /api/*  │
         │ - State()     │   │ - GET /api/*   │
         │ - Metrics()   │   │                │
         └───────────────┘   └────────────────┘
                                     │
                                     │ HTTP
                                     ↓
                          ┌──────────────────┐
                          │  Ollama Server   │
                          │  localhost:11434 │
                          └──────────────────┘
```

## Request Flow

### Complete() Flow

```
User Code
   │
   │ Complete(ctx, "What is AI?")
   ↓
OllamaClient.Complete()
   │
   │ Wrap with Circuit Breaker
   ↓
CircuitBreaker.Execute()
   │
   │ Check circuit state
   ├─→ [OPEN] Return ErrCircuitOpen
   │
   │ [CLOSED/HALF-OPEN]
   ↓
OllamaClient.complete()
   │
   │ Create timeout context (5s)
   ↓
Build HTTP Request
   │
   │ POST /api/generate
   │ {
   │   "model": "phi3:mini",
   │   "prompt": "What is AI?",
   │   "stream": false
   │ }
   ↓
http.Client.Do()
   │
   │ HTTP Request
   ↓
Ollama Server
   │
   │ HTTP Response
   ↓
Parse JSON Response
   │
   │ {
   │   "response": "AI is...",
   │   "done": true
   │ }
   ↓
CircuitBreaker records success/failure
   │
   ↓
Return response to User Code
```

### Embed() Flow

```
User Code
   │
   │ Embed(ctx, "Hello world")
   ↓
OllamaClient.Embed()
   │
   │ Wrap with Circuit Breaker
   ↓
CircuitBreaker.Execute()
   │
   │ Check circuit state
   ├─→ [OPEN] Return ErrCircuitOpen
   │
   │ [CLOSED/HALF-OPEN]
   ↓
OllamaClient.embed()
   │
   │ Create timeout context (5s)
   ↓
Build HTTP Request
   │
   │ POST /api/embeddings
   │ {
   │   "model": "phi3:mini",
   │   "input": "Hello world"
   │ }
   ↓
http.Client.Do()
   │
   │ HTTP Request
   ↓
Ollama Server
   │
   │ HTTP Response
   ↓
Parse JSON Response
   │
   │ {
   │   "embedding": [0.1, 0.2, ...]
   │ }
   ↓
CircuitBreaker records success/failure
   │
   ↓
Return []float32 to User Code
```

## Circuit Breaker State Machine

```
           ┌──────────────┐
           │   CLOSED     │
           │ (Normal ops) │
           └──────┬───────┘
                  │
                  │ 3 consecutive failures
                  ↓
           ┌──────────────┐
           │     OPEN     │◄──┐
           │ (Rejecting)  │   │
           └──────┬───────┘   │ Failure in half-open
                  │            │
                  │ After 30s  │
                  ↓            │
           ┌──────────────┐   │
           │  HALF-OPEN   │───┘
           │ (Testing)    │
           └──────┬───────┘
                  │
                  │ 2 consecutive successes
                  ↓
           ┌──────────────┐
           │   CLOSED     │
           │ (Recovered)  │
           └──────────────┘
```

## Data Structures

### OllamaConfig
```go
type OllamaConfig struct {
    BaseURL string        // "http://localhost:11434"
    Model   string        // "phi3:mini"
    Timeout time.Duration // 5 * time.Second
}
```

### Request Types
```go
// Text completion
type generateRequest struct {
    Model  string `json:"model"`
    Prompt string `json:"prompt"`
    Stream bool   `json:"stream"`
}

// Embeddings
type embedRequest struct {
    Model string `json:"model"`
    Input string `json:"input"`
}
```

### Response Types
```go
// Text completion
type generateResponse struct {
    Response string `json:"response"`
    Done     bool   `json:"done"`
}

// Embeddings
type embedResponse struct {
    Embedding []float32 `json:"embedding"`
}

// Model listing
type tagsResponse struct {
    Models []struct {
        Name string `json:"name"`
    } `json:"models"`
}
```

## Integration Points

### With Prompts

```
Content → EntityExtractionPrompt() → Prompt String
                                          ↓
                         OllamaClient.Complete(ctx, prompt)
                                          ↓
                                    JSON Response
                                          ↓
                              ParseEntityResponse()
                                          ↓
                                   []Entity
```

### With Chunker

```
Large Content → Chunker.Chunk()
                     ↓
              [chunk1, chunk2, ...]
                     ↓
            For each chunk:
              EntityExtractionPrompt(chunk)
                     ↓
              OllamaClient.Complete(ctx, prompt)
                     ↓
            Merge all entity results
```

### With Circuit Breaker

```
Every Complete() and Embed() call is wrapped:

CircuitBreaker.Execute(ctx, func() {
    // HTTP call to Ollama
})

Benefits:
- Automatic failure detection
- Fast fail when service is down
- Automatic recovery detection
- Prevents cascading failures
```

## Error Handling Strategy

```
HTTP Request Error
   │
   ├─→ Context Canceled → Return context.Canceled
   │
   ├─→ Context Timeout → Return context.DeadlineExceeded
   │
   ├─→ Network Error → Return wrapped error
   │
   ├─→ Non-200 Status → Return error with status and body
   │
   ├─→ JSON Parse Error → Return wrapped error
   │
   └─→ Circuit Breaker Open → Return ErrCircuitOpen
```

## Performance Characteristics

### Latency
- **Health Check**: 10-50ms
- **List Models**: 10-50ms
- **Complete** (phi3:mini): 100-500ms
- **Complete** (llama3:8b): 200-1000ms
- **Embed**: 50-200ms

### Resource Usage
- **Memory**: ~100KB per client
- **Goroutines**: 0 (synchronous calls)
- **HTTP Connections**: Reused via http.Client

### Timeout Behavior
```
Request Start
   │
   ├─→ After 5s → context.DeadlineExceeded
   │              Circuit breaker records failure
   │
   └─→ Success → Response returned
                  Circuit breaker records success
```

## Testing Strategy

### Unit Tests (Mock Server)
```
Test Code
   │
   ↓
httptest.NewServer()
   │
   │ Simulates Ollama API
   ↓
OllamaClient → Mock Server
   │
   ↓
Verify response, errors, timeouts
```

### Integration Tests
```
Multiple Components
   │
   ├─→ OllamaClient
   │
   ├─→ Chunker
   │
   ├─→ Prompts
   │
   └─→ CircuitBreaker
       │
       ↓
   Verify complete workflows
```

## Configuration Defaults

```go
Default Configuration:
{
    BaseURL: "http://localhost:11434",
    Model:   "phi3:mini",
    Timeout: 5 * time.Second,
}

Circuit Breaker Defaults:
{
    MaxFailures:          3,
    Timeout:              30 * time.Second,
    HalfOpenMaxSuccesses: 2,
}
```

## API Endpoints

```
Ollama Server (localhost:11434)
   │
   ├─→ POST /api/generate       → Text completion
   │
   ├─→ POST /api/embeddings     → Embedding generation
   │
   ├─→ GET  /api/tags           → List models
   │
   └─→ GET  /api/version        → Health check
```

## Thread Safety

The OllamaClient is **thread-safe**:

- `http.Client` is thread-safe (reused)
- `CircuitBreaker` has internal locking
- No shared mutable state in OllamaClient
- Safe for concurrent use from multiple goroutines

```go
// Safe to use concurrently
client := llm.NewOllamaClient(config)

go client.Complete(ctx, "prompt1")
go client.Complete(ctx, "prompt2")
go client.Embed(ctx, "text1")
```

## Extension Points

Future enhancements could add:

1. **Streaming**: Support streaming responses
2. **Batch API**: Process multiple prompts in parallel
3. **Model Loading**: Load/unload models dynamically
4. **Temperature Control**: Add temperature parameter
5. **Token Limits**: Add max_tokens parameter
6. **Custom Headers**: Support authentication headers

## Dependencies Graph

```
ollama.go
   ├─→ circuit_breaker.go (CircuitBreaker)
   ├─→ net/http (HTTP client)
   ├─→ encoding/json (JSON parsing)
   ├─→ context (Cancellation)
   └─→ time (Timeouts)

Tests depend on:
   ├─→ net/http/httptest (Mock servers)
   └─→ testing (Test framework)
```

## Monitoring Recommendations

```go
// Health monitoring
ticker := time.NewTicker(30 * time.Second)
for range ticker.C {
    if err := client.HealthCheck(ctx); err != nil {
        log.Printf("Ollama unhealthy: %v", err)
        metrics.RecordOllamaDown()
    }
}

// Circuit breaker monitoring
metrics := client.circuitBreaker.Metrics()
log.Printf("CB: %d requests, %d successes, %d failures",
    metrics.TotalRequests,
    metrics.TotalSuccesses,
    metrics.TotalFailures)
```
