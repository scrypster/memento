#!/bin/bash
# run-integration-tests.sh - Run integration tests for Memento v2.0
#
# Usage:
#   ./scripts/run-integration-tests.sh           # Run all integration tests
#   ./scripts/run-integration-tests.sh -short    # Skip long-running tests
#   ./scripts/run-integration-tests.sh -v        # Verbose output

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
VERBOSE=""
SHORT=""
COVERAGE=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    -v|--verbose)
      VERBOSE="-v"
      shift
      ;;
    -short|--short)
      SHORT="-short"
      shift
      ;;
    -cover|--coverage)
      COVERAGE=true
      shift
      ;;
    *)
      echo "Unknown option: $1"
      echo "Usage: $0 [-v] [-short] [-cover]"
      exit 1
      ;;
  esac
done

echo -e "${GREEN}=== Memento Integration Tests ===${NC}"
echo ""

# Check if Ollama is running (for LLM tests)
if curl -s http://localhost:11434/api/tags > /dev/null 2>&1; then
  echo -e "${GREEN}✓ Ollama detected - LLM tests will run${NC}"
else
  echo -e "${YELLOW}⚠ Ollama not detected - LLM tests will be skipped${NC}"
fi
echo ""

# Change to repo root
cd "$(dirname "$0")/.."

# Run integration tests
echo -e "${GREEN}Running integration tests...${NC}"

if [ "$COVERAGE" = true ]; then
  go test $VERBOSE $SHORT -coverprofile=coverage-integration.out ./tests/integration/
  echo ""
  echo -e "${GREEN}Coverage report:${NC}"
  go tool cover -func=coverage-integration.out | tail -n 1
else
  go test $VERBOSE $SHORT ./tests/integration/
fi

echo ""
echo -e "${GREEN}✓ Integration tests completed${NC}"
