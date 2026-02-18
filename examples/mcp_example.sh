#!/bin/bash
# Example: Testing Memento MCP Server
# This script demonstrates how to interact with the MCP server manually

set -e

# Build the server
echo "Building memento-mcp..."
go build -o /tmp/memento-mcp ./cmd/memento-mcp

echo "✓ Build successful"
echo ""

# Set up test environment
export MEMENTO_DATA_PATH="/tmp/memento-test"
mkdir -p "$MEMENTO_DATA_PATH"

echo "Testing MCP Server..."
echo "====================="
echo ""

# Test 1: Store a memory
echo "Test 1: store_memory"
echo '{"jsonrpc":"2.0","method":"store_memory","params":{"content":"Go is a statically typed programming language","source":"manual","domain":"tech","tags":["programming","go"]},"id":1}' | \
  /tmp/memento-mcp 2>/dev/null | jq '.'

echo ""

# Test 2: Store another memory
echo "Test 2: store_memory (second memory)"
echo '{"jsonrpc":"2.0","method":"store_memory","params":{"content":"Python is a dynamically typed language","source":"manual","domain":"tech"},"id":2}' | \
  /tmp/memento-mcp 2>/dev/null | jq '.'

echo ""

# Test 3: Find related memories
echo "Test 3: find_related"
echo '{"jsonrpc":"2.0","method":"find_related","params":{"query":"programming","limit":10,"domain":"tech"},"id":3}' | \
  /tmp/memento-mcp 2>/dev/null | jq '.'

echo ""

# Test 4: Test error handling
echo "Test 4: Error handling (empty content)"
echo '{"jsonrpc":"2.0","method":"store_memory","params":{"content":""},"id":4}' | \
  /tmp/memento-mcp 2>/dev/null | jq '.'

echo ""

# Test 5: Method not found
echo "Test 5: Method not found"
echo '{"jsonrpc":"2.0","method":"nonexistent","params":{},"id":5}' | \
  /tmp/memento-mcp 2>/dev/null | jq '.'

echo ""
echo "✓ All tests completed"
echo ""
echo "Note: Use 'jq' to pretty-print JSON responses"
echo "Stored data is in: $MEMENTO_DATA_PATH"
