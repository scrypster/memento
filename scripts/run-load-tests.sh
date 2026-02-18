#!/bin/bash
# run-load-tests.sh - Run load tests for Memento v2.0
#
# Usage:
#   ./scripts/run-load-tests.sh           # Run all load tests
#   ./scripts/run-load-tests.sh -v        # Verbose output

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Default values
VERBOSE=""

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    -v|--verbose)
      VERBOSE="-v"
      shift
      ;;
    *)
      echo "Unknown option: $1"
      echo "Usage: $0 [-v]"
      exit 1
      ;;
  esac
done

echo -e "${GREEN}=== Memento Load Tests ===${NC}"
echo ""
echo -e "${YELLOW}Note: Load tests may take several minutes to complete${NC}"
echo ""

# Change to repo root
cd "$(dirname "$0")/.."

# Run load tests (they're in the integration package)
echo -e "${GREEN}Running load tests...${NC}"
echo ""

go test $VERBOSE -run "^TestLoad_" ./tests/integration/

echo ""
echo -e "${GREEN}✓ Load tests completed${NC}"
