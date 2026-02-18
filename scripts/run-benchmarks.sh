#!/bin/bash
# run-benchmarks.sh - Run performance benchmarks for Memento v2.0
#
# Usage:
#   ./scripts/run-benchmarks.sh              # Run all benchmarks
#   ./scripts/run-benchmarks.sh Store        # Run specific benchmark
#   ./scripts/run-benchmarks.sh -count 5     # Run 5 times for averaging

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Default values
BENCH_FILTER="."
COUNT=1
BENCHTIME="1s"

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    -count)
      COUNT="$2"
      shift 2
      ;;
    -benchtime)
      BENCHTIME="$2"
      shift 2
      ;;
    -*)
      echo "Unknown option: $1"
      echo "Usage: $0 [benchmark_name] [-count N] [-benchtime Xs]"
      exit 1
      ;;
    *)
      BENCH_FILTER="$1"
      shift
      ;;
  esac
done

echo -e "${GREEN}=== Memento Performance Benchmarks ===${NC}"
echo ""

# Check if Ollama is running
if curl -s http://localhost:11434/api/tags > /dev/null 2>&1; then
  echo -e "${GREEN}✓ Ollama detected - LLM benchmarks will run${NC}"
else
  echo -e "${YELLOW}⚠ Ollama not detected - LLM benchmarks will be skipped${NC}"
fi
echo ""

# Change to repo root
cd "$(dirname "$0")/.."

# Run benchmarks
echo -e "${GREEN}Running benchmarks (filter: $BENCH_FILTER, count: $COUNT, benchtime: $BENCHTIME)...${NC}"
echo ""

go test -bench="$BENCH_FILTER" -benchmem -benchtime="$BENCHTIME" -count="$COUNT" ./tests/benchmarks/

echo ""
echo -e "${GREEN}✓ Benchmarks completed${NC}"
echo ""
echo "To compare results, save output and use benchstat:"
echo "  go install golang.org/x/perf/cmd/benchstat@latest"
echo "  benchstat old.txt new.txt"
