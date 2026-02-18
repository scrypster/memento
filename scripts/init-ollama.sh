#!/bin/bash
# Initialize Ollama with default model for Memento
# This script waits for Ollama to be ready and pulls the default model

set -e

# Configuration
OLLAMA_HOST=${OLLAMA_HOST:-"http://ollama:11434"}
MODEL=${MEMENTO_OLLAMA_MODEL:-"qwen2.5:7b"}
MAX_RETRIES=30
RETRY_DELAY=5

# Color codes for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')] INFO:${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] WARN:${NC} $1"
}

log_error() {
    echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] ERROR:${NC} $1"
}

log_info "=== Ollama Initialization Script ==="
log_info "Target host: ${OLLAMA_HOST}"
log_info "Model to pull: ${MODEL}"

# Wait for Ollama to be ready
log_info "Waiting for Ollama service to be ready..."
retry_count=0

while [ $retry_count -lt $MAX_RETRIES ]; do
    if curl -s "${OLLAMA_HOST}/api/tags" > /dev/null 2>&1; then
        log_info "Ollama service is ready!"
        break
    fi

    retry_count=$((retry_count + 1))
    log_warn "Ollama not ready yet (attempt ${retry_count}/${MAX_RETRIES}), retrying in ${RETRY_DELAY}s..."
    sleep ${RETRY_DELAY}

    if [ $retry_count -eq $MAX_RETRIES ]; then
        log_error "Failed to connect to Ollama after ${MAX_RETRIES} attempts"
        exit 1
    fi
done

# Check if model is already installed
log_info "Checking if model '${MODEL}' is already installed..."
if curl -s "${OLLAMA_HOST}/api/tags" | grep -q "\"${MODEL}\""; then
    log_info "Model '${MODEL}' is already installed"
    log_info "Initialization complete!"
    exit 0
fi

# Pull the model
log_info "Pulling model '${MODEL}'..."
log_info "This may take several minutes depending on model size..."

# Pull model using Ollama API
response=$(curl -s -X POST "${OLLAMA_HOST}/api/pull" \
    -H "Content-Type: application/json" \
    -d "{\"name\": \"${MODEL}\"}")

if echo "$response" | grep -q "error"; then
    log_error "Failed to pull model: $response"
    exit 1
fi

log_info "Model pull initiated successfully"

# Wait for model to be available
log_info "Waiting for model to be available..."
retry_count=0
MAX_MODEL_WAIT=60

while [ $retry_count -lt $MAX_MODEL_WAIT ]; do
    if curl -s "${OLLAMA_HOST}/api/tags" | grep -q "\"${MODEL}\""; then
        log_info "Model '${MODEL}' is now available!"
        break
    fi

    retry_count=$((retry_count + 1))
    sleep 2

    if [ $retry_count -eq $MAX_MODEL_WAIT ]; then
        log_warn "Model pull is still in progress, but initialization script is exiting"
        log_warn "The model will continue downloading in the background"
        exit 0
    fi
done

log_info "=== Ollama initialization complete ==="
log_info "Model: ${MODEL}"
log_info "Ready to serve requests at ${OLLAMA_HOST}"
