#!/bin/bash

# Run LLM correctness tests
# Tests verify that memories, entities, and relationships are generated correctly

set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$DIR"

echo "=========================================="
echo "Running LLM Correctness Tests"
echo "=========================================="
echo ""

# Check dependencies
echo "Checking dependencies..."

if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed"
    exit 1
fi

if ! curl -s http://localhost:11434/api/tags > /dev/null; then
    echo "❌ Ollama is not running at http://localhost:11434"
    echo "   Start Ollama with: ollama serve"
    exit 1
fi

echo "✓ Go is installed"
echo "✓ Ollama is running"
echo ""

# Run tests
echo "Running integration tests..."
echo ""

go test -v -tags=integration -timeout=600s \
    ./tests/integration/llm_correctness_test.go \
    -run TestLLMEntityExtractionCorrectness

echo ""
echo "Running relationship extraction tests..."
echo ""

go test -v -tags=integration -timeout=600s \
    ./tests/integration/llm_correctness_test.go \
    -run TestLLMRelationshipExtractionCorrectness

echo ""
echo "Running full enrichment pipeline tests..."
echo ""

go test -v -tags=integration -timeout=600s \
    ./tests/integration/llm_correctness_test.go \
    -run TestMemoryEnrichmentPipeline

echo ""
echo "=========================================="
echo "Tests Complete"
echo "=========================================="
