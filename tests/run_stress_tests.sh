#!/bin/bash

# Memento LLM Enrichment Pipeline Stress Test Runner
# This script runs comprehensive stress tests on the Memento system

set -e

REPO_ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"
TEST_DIR="$REPO_ROOT/tests"
RESULTS_DIR="$TEST_DIR/stress_test_results"

# Create results directory
mkdir -p "$RESULTS_DIR"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo ""
    echo "=============================================================================="
    echo "  $1"
    echo "=============================================================================="
    echo ""
}

# Check prerequisites
check_prerequisites() {
    print_header "Checking Prerequisites"

    if ! command -v go &> /dev/null; then
        log_error "Go is not installed"
        exit 1
    fi
    log_success "Go installed: $(go version)"

    if ! command -v ollama &> /dev/null; then
        log_warning "Ollama is not in PATH, but may be running as service"
    fi

    # Check if Ollama is running
    if ! curl -s http://localhost:11434/api/tags > /dev/null 2>&1; then
        log_error "Ollama is not running on localhost:11434"
        echo "Please start Ollama with: ollama serve"
        exit 1
    fi
    log_success "Ollama is running"

    # Check if qwen2.5:7b is available
    MODELS=$(curl -s http://localhost:11434/api/tags | grep -o '"name":"[^"]*"' | head -5 || true)
    if echo "$MODELS" | grep -q "qwen2.5"; then
        log_success "qwen2.5 model is available"
    else
        log_warning "qwen2.5 model not found, please pull with: ollama pull qwen2.5:7b"
        echo "Available models: $MODELS"
    fi
}

# Run a specific test category
run_test_category() {
    local category=$1
    local timeout=$2
    local description=$3

    print_header "$description"

    log_info "Running tests matching: $category"
    log_info "Timeout: $timeout"
    log_info "Timestamp: $(date)"

    RESULTS_FILE="$RESULTS_DIR/${category}_results.txt"

    if go test -v "./tests" -run "$category" -timeout "$timeout" 2>&1 | tee "$RESULTS_FILE"; then
        log_success "Category '$category' passed"
        return 0
    else
        log_error "Category '$category' failed"
        return 1
    fi
}

# Main script
main() {
    log_info "Starting Memento LLM Enrichment Pipeline Stress Tests"
    log_info "Results will be saved to: $RESULTS_DIR"
    log_info "Timestamp: $(date)"

    check_prerequisites

    # Parse arguments
    TEST_MODE=${1:-"all"}
    TEST_DURATION=${2:-"120m"}

    case $TEST_MODE in
        "quick")
            print_header "Quick Sanity Check (5 minutes)"
            run_test_category "TestStress_LoadTesting_Minimal" "5m" "Load Test: Minimal Content"
            run_test_category "TestStress_Complexity_Sparse" "5m" "Complexity Test: Sparse"
            run_test_category "TestPrompt" "5m" "Prompt Structure Validation"
            ;;

        "load")
            print_header "Full Load Testing (30 minutes)"
            run_test_category "TestStress_LoadTesting" "30m" "Load Tests: All Content Sizes"
            ;;

        "complexity")
            print_header "Full Complexity Testing (25 minutes)"
            run_test_category "TestStress_Complexity" "25m" "Complexity Tests: All Levels"
            ;;

        "concurrency")
            print_header "Full Concurrency Testing (20 minutes)"
            run_test_category "TestStress_Concurrency" "20m" "Concurrency Tests: All Load Patterns"
            ;;

        "edge")
            print_header "Edge Case Testing (10 minutes)"
            run_test_category "TestStress_EdgeCase" "10m" "Edge Case Tests"
            ;;

        "prompts")
            print_header "Prompt Validation Testing (5 minutes)"
            run_test_category "TestPrompt" "5m" "Prompt Structure Validation"
            ;;

        "all")
            print_header "Running All Stress Tests (120 minutes)"
            log_info "This will take approximately 2 hours"

            run_test_category "TestStress_LoadTesting_Minimal" "10m" "Load Test: Minimal Content"
            run_test_category "TestStress_LoadTesting_Small" "15m" "Load Test: Small Content"
            run_test_category "TestStress_LoadTesting_Medium" "20m" "Load Test: Medium Content"
            run_test_category "TestStress_LoadTesting_Large" "30m" "Load Test: Large Content"

            run_test_category "TestStress_Complexity_Sparse" "10m" "Complexity: Sparse"
            run_test_category "TestStress_Complexity_Dense" "15m" "Complexity: Dense"
            run_test_category "TestStress_Complexity_FamilyTree" "15m" "Complexity: Family Tree"
            run_test_category "TestStress_Complexity_Technical" "15m" "Complexity: Technical"

            run_test_category "TestStress_Concurrency_Sequential" "15m" "Concurrency: Sequential"
            run_test_category "TestStress_Concurrency_Concurrent5" "15m" "Concurrency: 5 Goroutines"
            run_test_category "TestStress_Concurrency_Concurrent10" "15m" "Concurrency: 10 Goroutines"
            run_test_category "TestStress_Concurrency_Concurrent20" "15m" "Concurrency: 20 Goroutines"
            run_test_category "TestStress_Concurrency_Concurrent50" "20m" "Concurrency: 50 Goroutines"
            run_test_category "TestStress_Concurrency_BurstLoad" "20m" "Concurrency: Burst Load"

            run_test_category "TestStress_EdgeCase" "10m" "Edge Cases"
            run_test_category "TestPrompt" "5m" "Prompt Validation"
            ;;

        *)
            log_error "Unknown test mode: $TEST_MODE"
            echo ""
            echo "Usage: $0 [mode] [timeout]"
            echo ""
            echo "Modes:"
            echo "  quick       - Quick sanity check (5 min)"
            echo "  load        - Load tests only (30 min)"
            echo "  complexity  - Complexity tests only (25 min)"
            echo "  concurrency - Concurrency tests only (20 min)"
            echo "  edge        - Edge case tests only (10 min)"
            echo "  prompts     - Prompt validation only (5 min)"
            echo "  all         - All tests (120 min, default)"
            echo ""
            echo "Examples:"
            echo "  $0 quick"
            echo "  $0 load"
            echo "  $0 all"
            exit 1
            ;;
    esac

    print_header "Stress Test Execution Complete"
    log_success "Results saved to: $RESULTS_DIR"
    log_info "Review results with:"
    echo "  cat $RESULTS_DIR/*.txt"
    echo "  grep -r 'PASS\|FAIL' $RESULTS_DIR/"
}

# Run main function
main "$@"
