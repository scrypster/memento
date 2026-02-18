# Integration Tests for Memento v2.0

This directory contains comprehensive integration tests that validate the entire Memento system works correctly.

## Test Structure

```
tests/integration/
├── setup_test.go              # Common setup and test helpers
├── e2e_workflow_test.go       # End-to-end workflow tests
├── llm_integration_test.go    # LLM integration tests
├── storage_integration_test.go # Storage layer tests
├── golden_test.go             # Golden file tests
├── load_test.go               # Load and stress tests
└── testdata/
    ├── golden/                # Golden files for regression testing
    │   ├── entity_prompt_*.txt
    │   ├── entity_response_*.json
    │   └── relationship_*.json
    └── fixtures/              # Test fixtures
```

## Running Tests

### Run All Integration Tests

```bash
make test-integration
```

Or directly:

```bash
./scripts/run-integration-tests.sh
```

### Run Specific Test Suites

```bash
# E2E workflow tests
go test -v -run TestE2E ./tests/integration/

# LLM integration tests (requires Ollama)
go test -v -run TestLLM ./tests/integration/

# Storage tests
go test -v -run TestStorage ./tests/integration/

# Load tests
go test -v -run TestLoad ./tests/integration/
```

### Skip Long-Running Tests

Use the `-short` flag to skip tests that require Ollama or take a long time:

```bash
go test -short ./tests/integration/
```

Or:

```bash
./scripts/run-integration-tests.sh -short
```

### Run with Verbose Output

```bash
./scripts/run-integration-tests.sh -v
```

### Run with Coverage

```bash
./scripts/run-integration-tests.sh -cover
```

## Test Categories

### End-to-End Workflow Tests (`e2e_workflow_test.go`)

Tests complete user workflows:
- **TestE2E_StoreAndRecall** - Basic store and retrieve
- **TestE2E_SearchWorkflow** - Search functionality
- **TestE2E_EnrichmentPipeline** - Full enrichment (requires Ollama)
- **TestE2E_ConcurrentStores** - Concurrent operations
- **TestE2E_UpdateMemory** - Update operations
- **TestE2E_DeleteMemory** - Delete operations
- **TestE2E_ListMemories** - List with pagination

### LLM Integration Tests (`llm_integration_test.go`)

Tests LLM functionality (requires Ollama):
- **TestLLM_EntityExtraction** - Entity extraction
- **TestLLM_RelationshipExtraction** - Relationship extraction
- **TestLLM_EmbeddingGeneration** - Embedding generation
- **TestLLM_CircuitBreakerIntegration** - Circuit breaker behavior
- **TestLLM_Chunking** - Content chunking
- **TestLLM_PromptGeneration** - Prompt generation

### Storage Integration Tests (`storage_integration_test.go`)

Tests storage layer:
- **TestStorage_TransactionSupport** - Transaction handling
- **TestStorage_ConcurrentWrites** - Concurrent write safety
- **TestStorage_UpdateWithConcurrency** - Concurrent updates
- **TestStorage_SearchIndexing** - Search index maintenance
- **TestStorage_EntityAndRelationshipStorage** - Entity/relationship storage
- **TestStorage_TagFiltering** - Tag-based filtering

### Golden File Tests (`golden_test.go`)

Tests for prompt and response format regression:
- **TestGolden_EntityExtractionPrompt** - Entity prompt format
- **TestGolden_RelationshipExtractionPrompt** - Relationship prompt format
- **TestGolden_EntityResponseParsing** - Entity response parsing
- **TestGolden_RelationshipResponseParsing** - Relationship response parsing

To update golden files after intentional changes:

```bash
go test -v ./tests/integration/ -update
```

Or:

```bash
make test-golden-update
```

### Load Tests (`load_test.go`)

Stress and performance tests:
- **TestLoad_ConcurrentStores** - High concurrent write load
- **TestLoad_ConcurrentReads** - High concurrent read load
- **TestLoad_MixedWorkload** - Mixed read/write operations
- **TestLoad_QueuePressure** - Enrichment queue pressure
- **TestLoad_SustainedLoad** - Sustained load over time
- **TestLoad_BurstTraffic** - Burst traffic patterns
- **TestLoad_MemoryLeak** - Memory leak detection

Run load tests separately (they take longer):

```bash
make test-load
```

## Requirements

### Always Required
- Go 1.25.0+
- SQLite3

### For LLM Tests
- Ollama running locally (`http://localhost:11434`)
- Model pulled: `ollama pull phi3:mini`

To check if Ollama is available:

```bash
curl http://localhost:11434/api/tags
```

### Starting Ollama

```bash
# Start Ollama
ollama serve

# In another terminal, pull the model
ollama pull phi3:mini
```

Or use Docker:

```bash
make up
```

## Test Helpers

The `setup_test.go` file provides helper functions:

### `NewTestEngine(t *testing.T)`
Creates a fully configured memory engine for testing with:
- Temporary SQLite database
- Migrations applied
- Worker pool started
- Cleanup function returned

Usage:
```go
func TestSomething(t *testing.T) {
    eng, cleanup := NewTestEngine(t)
    defer cleanup()

    // Use eng for testing
}
```

### `NewTestStore(t *testing.T)`
Creates a standalone memory store for storage-only tests.

## Writing New Tests

### Integration Test Template

```go
func TestIntegration_NewFeature(t *testing.T) {
    // Skip in short mode if test is slow
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    // Create test engine
    eng, cleanup := NewTestEngine(t)
    defer cleanup()

    ctx := context.Background()

    // Your test logic here
}
```

### LLM Test Template

```go
func TestLLM_NewFeature(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping LLM test in short mode")
    }

    client := llm.NewOllamaClient(llm.OllamaConfig{})
    if err := client.HealthCheck(context.Background()); err != nil {
        t.Skip("Ollama not available:", err)
    }

    // Your test logic here
}
```

## Benchmarks

See `../benchmarks/` for performance benchmarks.

Run benchmarks:
```bash
make test-bench
```

## CI/CD Integration

These tests are designed for CI/CD pipelines:

```yaml
# Example GitHub Actions
- name: Run integration tests
  run: |
    make test-integration -short  # Skip Ollama-dependent tests

- name: Run integration tests with Ollama
  run: |
    # Start Ollama
    docker run -d -p 11434:11434 ollama/ollama
    docker exec ollama ollama pull phi3:mini

    # Run full test suite
    make test-integration
```

## Troubleshooting

### Tests Fail with "Ollama not available"

**Solution**: Start Ollama or run with `-short` flag to skip LLM tests.

### Tests Fail with "Failed to run migrations"

**Solution**: Ensure you're running from the repo root or migrations directory is accessible.

### Load Tests Timeout

**Solution**: Load tests are designed to run for several minutes. Increase test timeout:
```bash
go test -timeout 30m -run TestLoad ./tests/integration/
```

### Permission Denied on Scripts

**Solution**: Make scripts executable:
```bash
chmod +x scripts/*.sh
```

## Performance Targets

Integration tests validate these performance targets:
- **Store()** < 10ms (without enrichment)
- **Get()** < 5ms
- **Search()** < 100ms (for < 10k memories)
- **Concurrent Stores** > 100 ops/sec
- **Error Rate** < 1% under load

See load tests for detailed performance validation.
